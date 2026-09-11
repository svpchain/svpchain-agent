package guard

import (
	"fmt"
	"strings"

	"github.com/svpchain/svpchain-agent/internal/whitelist"
)

// AliasPrompt builds a system-prompt section mapping whitelist aliases to their
// addresses for chainID. It intentionally contains aliases only; the local
// transfer gate remains the authority for the complete persisted whitelist.
//
// This is advisory context only: the recipient the LLM ultimately uses is still
// validated by the pre-flight whitelist gate (see whitelist_gate.go).
func AliasPrompt(chainID string) string {
	chainID = strings.TrimSpace(chainID)
	if chainID == "" {
		return ""
	}
	var lines []string
	for _, e := range whitelist.LoadEffectiveStore().List() {
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
			"This alias list is incomplete: it omits saved addresses without aliases. Do not decide whether a raw address is whitelisted from this list. "+
			"For a raw address the user explicitly supplied, call the appropriate build tool; the local transfer gate reads the complete whitelist and will allow or refuse it. "+
			"If the user names an unknown alias without providing an address, ask for the address instead of guessing.\n%s",
		chainID, strings.Join(lines, "\n"))
}
