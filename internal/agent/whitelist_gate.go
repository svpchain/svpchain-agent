package agent

import (
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

// checkWhitelistGate rejects a guarded tool call whose recipient/spender is not
// on the whitelist for chainID. It returns nil for tools that are not gated and
// when whitelist enforcement is inactive (no entries), reusing the same
// semantics as the signer-layer checks in internal/whitelist/enforce.go.
func checkWhitelistGate(chainID, name string, args map[string]any) error {
	g, ok := transferGuardedTools[name]
	if !ok {
		return nil
	}
	addr := ""
	if args != nil {
		if v, ok := args[g.field].(string); ok {
			addr = strings.TrimSpace(v)
		}
	}
	// An empty address either means "to self" (optional, e.g. bridge deposit) or
	// is a malformed call the downstream builder will reject; either way the
	// whitelist gate has nothing to check.
	if addr == "" {
		return nil
	}
	switch g.addressType {
	case whitelist.AddressTypeCosmos:
		return whitelist.CheckCosmosRecipient(chainID, addr)
	default:
		return whitelist.CheckEVMRecipient(chainID, addr)
	}
}
