package guard

import (
	"fmt"
	"strings"

	"github.com/svpchain/svpchain-agent/internal/whitelist"
)

// AliasPrompt builds a system-prompt section mapping whitelist aliases
// to their addresses for chainID, so the assistant can resolve "transfer to
// <alias>" to a concrete recipient. Entries without an alias, or for other
// chains, are skipped. Returns "" when there is nothing to inject.
//
// This is advisory context only: the recipient the LLM ultimately uses is still
// validated by the pre-flight whitelist gate (see whitelist_gate.go).
func AliasPrompt(chainID string) string {
	chainID = strings.TrimSpace(chainID)
	if chainID == "" {
		return ""
	}
	var lines []string
	for _, e := range whitelist.LoadStore().List() {
		if e.ChainID != chainID || strings.TrimSpace(e.Alias) == "" {
			continue
		}
		label := "SVP Cosmos"
		if e.AddressType == whitelist.AddressTypeEVM {
			label = "EVM"
		}
		lines = append(lines, fmt.Sprintf("- %q → %s (%s)", e.Alias, e.Address, label))
	}
	if len(lines) == 0 {
		return ""
	}
	return fmt.Sprintf(
		"## Whitelist aliases (chain %s)\n"+
			"When the user names a payee by one of these aliases, use the mapped address as the recipient. "+
			"Match the address type to the transfer (SVP Cosmos → build_bank_send recipient; EVM → the `to` of an EVM transfer). "+
			"Only these aliases are known — if the user names an alias not listed here, tell them it is not on the whitelist instead of guessing an address.\n%s",
		chainID, strings.Join(lines, "\n"))
}
