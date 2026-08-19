package guard

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/svpchain/svpchain-agent/internal/payload"
	"github.com/svpchain/svpchain-agent/internal/whitelist"
)

// guardedTool describes how to extract the recipient address from a transfer-
// or approval-style build_* tool's arguments, and which whitelist address type
// it should be checked against.
type guardedTool struct {
	field       string // args key holding the destination/spender address
	addressType string // whitelist.AddressTypeCosmos or AddressTypeEVM
}

// transferGuardedTools maps remote build_* tool names to the argument carrying
// the third-party recipient/spender. Any tool here has its destination checked
// against the whitelist BEFORE any transaction-related tool is called, so a
// non-whitelisted address is rejected before any build/sign/broadcast happens.
// Tools absent from this map (queries, swaps that output to self, etc.) are not
// gated.
var transferGuardedTools = map[string]guardedTool{
	"build_bank_send":                   {field: "recipient", addressType: whitelist.AddressTypeCosmos},
	"build_erc20_transfer":              {field: "to", addressType: whitelist.AddressTypeEVM},
	"build_erc20_transfer_from":         {field: "to", addressType: whitelist.AddressTypeEVM},
	"build_erc721_transfer_from":        {field: "to", addressType: whitelist.AddressTypeEVM},
	"build_erc721_safe_transfer_from":   {field: "to", addressType: whitelist.AddressTypeEVM},
	"build_bridge_deposit":              {field: "recipient", addressType: whitelist.AddressTypeEVM},
	"build_erc20_approve":               {field: "spender", addressType: whitelist.AddressTypeEVM},
	"build_erc721_approve":              {field: "spender", addressType: whitelist.AddressTypeEVM},
	"build_erc721_set_approval_for_all": {field: "operator", addressType: whitelist.AddressTypeEVM},
}

// SignEVMTool is the local signing tool that takes a whole EVM transaction.
//
// It needs its own check because it is directly callable: nothing requires it to
// be preceded by a build_* call, so gating only build_* leaves the signer
// reachable with a hand-crafted payload. And because a token transfer or
// approval carries value 0 and addresses the token contract, the beneficiary
// appears ONLY in the call data — the argument-name checks used for build_*
// tools have nothing to read. A manipulated assistant (say, one that ingested
// attacker-controlled content) could otherwise call this directly to transfer
// ERC-20 balances out or grant an unlimited approval, with no human in the loop
// and the whitelist never consulted.
const SignEVMTool = "sign_evm_transaction"

// Rejection marks a tool call refused by the pre-flight whitelist gate.
// The agent loop detects it (via errors.As) and stops immediately instead of
// feeding the error back to the LLM, so a non-whitelisted transfer ends the run
// rather than prompting the model to retry.
type Rejection struct{ Err error }

func (e *Rejection) Error() string { return e.Err.Error() }

func (e *Rejection) Unwrap() error { return e.Err }

// Check rejects a guarded tool call before it reaches the remote
// MCP. For the GUI assistant the whitelist is mandatory: if NO whitelist is
// configured, every transfer/approval tool is refused with a prompt to add one
// first (this is stricter than the signer-layer "empty = unrestricted" default
// in internal/whitelist/enforce.go, and applies only to the assistant). When a
// whitelist exists, the recipient/spender must be on it. A rejection is wrapped
// in *Rejection so the caller terminates instead of retrying.
func Check(chainID, name string, args map[string]any) error {
	// sign_evm_transaction is checked on the transaction itself, not on named
	// arguments — see checkSignEVM.
	if name == SignEVMTool {
		return checkSignEVM(chainID, args)
	}
	g, ok := transferGuardedTools[name]
	if !ok {
		return nil
	}
	// The assistant checks against the effective whitelist: the hardcoded
	// DefaultEntries merged with the user's saved entries. The defaults keep this
	// non-empty on a fresh install, so the mandatory-whitelist guard below is
	// effectively always satisfied, but it is kept fail-closed in case the
	// defaults are ever removed.
	store := whitelist.LoadEffectiveStore()
	if !store.Enforced() {
		return &Rejection{Err: fmt.Errorf(
			"no whitelist configured for chain %q — add a recipient in the Security tab before transferring",
			chainID)}
	}
	addr := ""
	if args != nil {
		if v, ok := args[g.field].(string); ok {
			addr = strings.TrimSpace(v)
		}
	}
	// An empty address means "to self" (e.g. bridge deposit) or a malformed call
	// the downstream builder will reject; the whitelist gate has nothing to check.
	if addr == "" {
		return nil
	}
	if !store.Allows(chainID, g.addressType, addr) {
		label := "EVM"
		if g.addressType == whitelist.AddressTypeCosmos {
			label = "SVP Cosmos"
		}
		return &Rejection{Err: fmt.Errorf(
			"recipient %q is not on the whitelist for chain %q (%s)", addr, chainID, label)}
	}
	return nil
}

// checkSignEVM gates a direct sign_evm_transaction call on the transaction it
// carries: the recipient of a native (value-bearing) send, plus the
// recipient/spender/operator decoded out of standard ERC-20/721/1155 call data.
//
// Same mandatory-whitelist policy as the build_* gate above (empty whitelist =
// refuse), and the same *Rejection wrapper so a refusal ends the run instead of
// being handed back to the model to retry.
//
// Fail-closed on every parse failure. A payload whose value or data will not
// decode cannot be shown to be safe, and treating it as "nothing to check"
// would turn a malformed field into a way around the whitelist. The signer
// rejects these payloads too, so nothing legitimate is lost by stopping here.
//
// Contract calls with an unrecognized selector are NOT refused: gating those
// would break every swap, order and lending flow, since the tx is a contract
// call whose method this package does not model. They remain covered by the
// value>0 check above and by the approvals they must first obtain — both of
// which are gated.
func checkSignEVM(chainID string, args map[string]any) error {
	p, err := evmPayloadFromArgs(args)
	if err != nil {
		return &Rejection{Err: fmt.Errorf("refusing to sign EVM transaction: %w", err)}
	}

	store := whitelist.LoadEffectiveStore()
	if !store.Enforced() {
		return &Rejection{Err: fmt.Errorf(
			"no whitelist configured for chain %q — add a recipient in the Security tab before transferring",
			chainID)}
	}

	valuePositive := false
	if v := strings.TrimSpace(p.Value); v != "" {
		n, ok := new(big.Int).SetString(v, 10)
		if !ok {
			return &Rejection{Err: fmt.Errorf(
				"refusing to sign EVM transaction: value %q is not a base-10 integer", p.Value)}
		}
		valuePositive = n.Sign() > 0
	}

	var data []byte
	if d := strings.TrimSpace(p.Data); d != "" {
		data, err = hexutil.Decode(d)
		if err != nil {
			return &Rejection{Err: fmt.Errorf(
				"refusing to sign EVM transaction: cannot decode call data: %w", err)}
		}
	}

	if err := store.CheckEVMTx(chainID, strings.TrimSpace(p.To), valuePositive, data); err != nil {
		return &Rejection{Err: err}
	}
	return nil
}

// evmPayloadFromArgs re-reads the tool's "payload" argument as an EvmTxPayload,
// the same way the local signer does, so the gate inspects exactly the
// transaction that would be signed rather than a looser view of it.
func evmPayloadFromArgs(args map[string]any) (*payload.EvmTxPayload, error) {
	raw, ok := args["payload"]
	if !ok || raw == nil {
		return nil, fmt.Errorf("payload is required")
	}
	bz, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("encode payload: %w", err)
	}
	var p payload.EvmTxPayload
	if err := json.Unmarshal(bz, &p); err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}
	return &p, nil
}
