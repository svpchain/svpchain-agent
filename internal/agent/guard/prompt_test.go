package guard

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	appconfig "github.com/svpchain/svpchain-agent/internal/config"
)

func TestWhitelistAliasPrompt(t *testing.T) {
	appconfig.SetAddressPrefixes()

	evmA := common.HexToAddress("0x1111111111111111111111111111111111111111").Hex()
	cosmos := cosmosAddr(0x11)

	t.Run("empty whitelist yields no section", func(t *testing.T) {
		writePrefs(t, `{}`)
		require.Equal(t, "", AliasPrompt(gateChainID))
	})

	t.Run("entries without alias are skipped", func(t *testing.T) {
		writePrefs(t, `{"whitelist":[{"chain_id":"`+gateChainID+`","address_type":"evm","address":"`+evmA+`"}]}`)
		require.Equal(t, "", AliasPrompt(gateChainID))
	})

	t.Run("aliases for the chain are listed; others excluded", func(t *testing.T) {
		writePrefs(t, `{"whitelist":[`+
			`{"chain_id":"`+gateChainID+`","address_type":"evm","address":"`+evmA+`","alias":"Bob"},`+
			`{"chain_id":"`+gateChainID+`","address_type":"cosmos","address":"`+cosmos+`","alias":"Treasury"},`+
			`{"chain_id":"other-1","address_type":"evm","address":"`+evmA+`","alias":"Elsewhere"}`+
			`]}`)
		out := AliasPrompt(gateChainID)
		require.Contains(t, out, "Bob")
		require.Contains(t, out, evmA)
		require.Contains(t, out, "Treasury")
		require.Contains(t, out, cosmos)
		require.NotContains(t, out, "Elsewhere")
		require.Contains(t, out, gateChainID)
	})

	t.Run("blank chain id yields nothing", func(t *testing.T) {
		writePrefs(t, `{"whitelist":[{"chain_id":"`+gateChainID+`","address_type":"evm","address":"`+evmA+`","alias":"Bob"}]}`)
		require.Equal(t, "", AliasPrompt("  "))
		// Sanity: the same store does produce output for the right chain.
		require.True(t, strings.Contains(AliasPrompt(gateChainID), "Bob"))
	})
}
