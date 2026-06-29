package guard

import (
	"fmt"
	"strings"

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
// against the whitelist BEFORE the call is forwarded to the remote MCP, so a
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
	g, ok := transferGuardedTools[name]
	if !ok {
		return nil
	}
	// Mandatory whitelist: refuse all transfers until the user configures one.
	if !whitelist.LoadStore().Enforced() {
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
	var err error
	switch g.addressType {
	case whitelist.AddressTypeCosmos:
		err = whitelist.CheckCosmosRecipient(chainID, addr)
	default:
		err = whitelist.CheckEVMRecipient(chainID, addr)
	}
	if err != nil {
		return &Rejection{Err: err}
	}
	return nil
}
