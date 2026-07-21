package guard

import (
	"os"
	"path/filepath"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	appconfig "github.com/svpchain/svpchain-agent/internal/config"
	"github.com/svpchain/svpchain-agent/internal/prefs"
	"github.com/svpchain/svpchain-agent/internal/whitelist"
)

const gateChainID = "localsvp-1"

// writePrefs points prefs at a temp file holding the given JSON and restores the
// override on cleanup.
func writePrefs(t *testing.T, json string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "prefs.json")
	require.NoError(t, os.WriteFile(path, []byte(json), 0o600))
	t.Cleanup(func() { prefs.SetPathOverride("") })
	prefs.SetPathOverride(path)
}

func cosmosAddr(b byte) string {
	raw := make([]byte, 20)
	for i := range raw {
		raw[i] = b
	}
	return sdk.AccAddress(raw).String()
}

func TestCheckWhitelistGate(t *testing.T) {
	appconfig.SetAddressPrefixes()

	allowedEVM := common.HexToAddress("0x1111111111111111111111111111111111111111").Hex()
	blockedEVM := common.HexToAddress("0x2222222222222222222222222222222222222222").Hex()
	allowedCosmos := cosmosAddr(0x11)
	blockedCosmos := cosmosAddr(0x22)

	t.Run("defaults enforce with empty user whitelist", func(t *testing.T) {
		writePrefs(t, `{}`)
		// The hardcoded defaults are never persisted but always active, so the
		// whitelist is never "unconfigured": a default recipient is allowed and
		// any other recipient is rejected as not-on-whitelist.
		def := whitelist.DefaultEntries()[0]
		require.NoError(t, Check(def.ChainID, "build_erc20_transfer",
			map[string]any{"to": def.Address}))

		err := Check(def.ChainID, "build_erc20_transfer",
			map[string]any{"to": blockedEVM})
		require.Error(t, err)
		require.Contains(t, err.Error(), "not on the whitelist")
		var rej *Rejection
		require.ErrorAs(t, err, &rej)

		err = Check(gateChainID, "build_bank_send",
			map[string]any{"recipient": blockedCosmos})
		require.Error(t, err)
		require.Contains(t, err.Error(), "not on the whitelist")

		// Non-transfer tools stay allowed.
		require.NoError(t, Check(gateChainID, "get_balance",
			map[string]any{"owner": blockedCosmos}))
	})

	t.Run("evm transfer enforced", func(t *testing.T) {
		writePrefs(t, `{"whitelist":[{"chain_id":"`+gateChainID+`","address_type":"evm","address":"`+allowedEVM+`"}]}`)
		require.NoError(t, Check(gateChainID, "build_erc20_transfer",
			map[string]any{"to": allowedEVM}))
		err := Check(gateChainID, "build_erc20_transfer",
			map[string]any{"to": blockedEVM})
		require.Error(t, err)
		require.Contains(t, err.Error(), "not on the whitelist")
		// Must be a *Rejection so the agent loop terminates instead of
		// feeding the error back to the LLM.
		var rej *Rejection
		require.ErrorAs(t, err, &rej)
	})

	t.Run("cosmos bank send enforced", func(t *testing.T) {
		writePrefs(t, `{"whitelist":[{"chain_id":"`+gateChainID+`","address_type":"cosmos","address":"`+allowedCosmos+`"}]}`)
		require.NoError(t, Check(gateChainID, "build_bank_send",
			map[string]any{"recipient": allowedCosmos}))
		err := Check(gateChainID, "build_bank_send",
			map[string]any{"recipient": blockedCosmos})
		require.Error(t, err)
		require.Contains(t, err.Error(), "not on the whitelist")
	})

	t.Run("approval is gated", func(t *testing.T) {
		writePrefs(t, `{"whitelist":[{"chain_id":"`+gateChainID+`","address_type":"evm","address":"`+allowedEVM+`"}]}`)
		err := Check(gateChainID, "build_erc20_approve",
			map[string]any{"spender": blockedEVM})
		require.Error(t, err)
		require.Contains(t, err.Error(), "not on the whitelist")
	})

	t.Run("bridge deposit to self is allowed", func(t *testing.T) {
		writePrefs(t, `{"whitelist":[{"chain_id":"`+gateChainID+`","address_type":"evm","address":"`+allowedEVM+`"}]}`)
		// No recipient -> defaults to self -> not checked even with an active whitelist.
		require.NoError(t, Check(gateChainID, "build_bridge_deposit",
			map[string]any{"dest_chain": "sepolia", "token": "USDC", "amount": "1"}))
	})

	t.Run("non-transfer tools are not gated", func(t *testing.T) {
		writePrefs(t, `{"whitelist":[{"chain_id":"`+gateChainID+`","address_type":"evm","address":"`+allowedEVM+`"}]}`)
		require.NoError(t, Check(gateChainID, "get_balance",
			map[string]any{"owner": blockedCosmos}))
		require.NoError(t, Check(gateChainID, "build_swap",
			map[string]any{"token_in": "svp", "token_out": "usdv", "amount_in": "1"}))
	})
}
