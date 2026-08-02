package guard

import (
	"os"
	"path/filepath"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	appconfig "github.com/svpchain/svpchain-local-agent/internal/config"
	"github.com/svpchain/svpchain-local-agent/internal/prefs"
	"github.com/svpchain/svpchain-local-agent/internal/whitelist"
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

// tokenCall encodes selector + left-padded 32-byte words as 0x hex call data.
func tokenCall(sig string, words ...[]byte) string {
	out := ethcrypto.Keccak256([]byte(sig))[:4]
	for _, w := range words {
		padded := make([]byte, 32)
		copy(padded[32-len(w):], w)
		out = append(out, padded...)
	}
	return hexutil.Encode(out)
}

func signEVMArgs(to, value, data string) map[string]any {
	return map[string]any{
		"payload": map[string]any{
			"version":      1,
			"evm_chain_id": "2517",
			"nonce":        "1",
			"gas":          "100000",
			"to":           to,
			"value":        value,
			"data":         data,
		},
	}
}

// sign_evm_transaction is directly callable — nothing requires a build_* call
// first — so these cases are the ones that matter if the assistant is
// manipulated into hand-crafting a payload.
func TestCheckGate_SignEVMTransaction(t *testing.T) {
	appconfig.SetAddressPrefixes()

	allowedEVM := common.HexToAddress("0x1111111111111111111111111111111111111111")
	attacker := common.HexToAddress("0x2222222222222222222222222222222222222222")
	tokenContract := common.HexToAddress("0x3333333333333333333333333333333333333333").Hex()

	activeWhitelist := func(t *testing.T) {
		writePrefs(t, `{"whitelist":[{"chain_id":"`+gateChainID+
			`","address_type":"evm","address":"`+allowedEVM.Hex()+`"}]}`)
	}

	requireRejected := func(t *testing.T, err error, contains string) {
		t.Helper()
		require.Error(t, err)
		require.Contains(t, err.Error(), contains)
		// Must be a *Rejection so the run terminates rather than the error being
		// handed back to the model to retry.
		var rej *Rejection
		require.ErrorAs(t, err, &rej)
	}

	t.Run("erc20 transfer to attacker is rejected", func(t *testing.T) {
		activeWhitelist(t)
		// value 0, addressed to the token contract: the attacker appears only
		// inside the call data.
		err := Check(gateChainID, SignEVMTool, signEVMArgs(tokenContract, "0",
			tokenCall("transfer(address,uint256)", attacker.Bytes(), []byte{0x05})))
		requireRejected(t, err, "not on the whitelist")
		require.Contains(t, err.Error(), attacker.Hex())
	})

	t.Run("infinite approval to attacker is rejected", func(t *testing.T) {
		activeWhitelist(t)
		maxUint := make([]byte, 32)
		for i := range maxUint {
			maxUint[i] = 0xff
		}
		err := Check(gateChainID, SignEVMTool, signEVMArgs(tokenContract, "0",
			tokenCall("approve(address,uint256)", attacker.Bytes(), maxUint)))
		requireRejected(t, err, "spender")
	})

	t.Run("setApprovalForAll to attacker is rejected", func(t *testing.T) {
		activeWhitelist(t)
		err := Check(gateChainID, SignEVMTool, signEVMArgs(tokenContract, "0",
			tokenCall("setApprovalForAll(address,bool)", attacker.Bytes(), []byte{0x01})))
		requireRejected(t, err, "operator")
	})

	t.Run("native send to attacker is rejected", func(t *testing.T) {
		activeWhitelist(t)
		err := Check(gateChainID, SignEVMTool, signEVMArgs(attacker.Hex(), "1000", ""))
		requireRejected(t, err, "not on the whitelist")
	})

	t.Run("transfer to a whitelisted recipient is allowed", func(t *testing.T) {
		activeWhitelist(t)
		require.NoError(t, Check(gateChainID, SignEVMTool, signEVMArgs(tokenContract, "0",
			tokenCall("transfer(address,uint256)", allowedEVM.Bytes(), []byte{0x05}))))
	})

	t.Run("revoking an approval is allowed for any spender", func(t *testing.T) {
		activeWhitelist(t)
		require.NoError(t, Check(gateChainID, SignEVMTool, signEVMArgs(tokenContract, "0",
			tokenCall("approve(address,uint256)", attacker.Bytes(), nil))))
	})

	t.Run("zero-value call with an unmodelled selector is allowed", func(t *testing.T) {
		activeWhitelist(t)
		// Swaps, orders and lending calls look like this; gating them would
		// break every such flow.
		require.NoError(t, Check(gateChainID, SignEVMTool, signEVMArgs(tokenContract, "0",
			tokenCall("mint(uint256)", []byte{0x01}))))
	})

	t.Run("malformed fields are refused, not skipped", func(t *testing.T) {
		activeWhitelist(t)
		// Truncated call data under a known token selector.
		requireRejected(t, Check(gateChainID, SignEVMTool, signEVMArgs(tokenContract, "0",
			tokenCall("transfer(address,uint256)", attacker.Bytes()))), "truncated")

		// A value that will not parse must not be read as "not a native send".
		requireRejected(t, Check(gateChainID, SignEVMTool,
			signEVMArgs(attacker.Hex(), "not-a-number", "")), "base-10")

		// Call data that is not hex must not be read as "no call data".
		requireRejected(t, Check(gateChainID, SignEVMTool,
			signEVMArgs(tokenContract, "0", "zzzz")), "call data")

		// A missing payload must not sign.
		requireRejected(t, Check(gateChainID, SignEVMTool, map[string]any{}), "payload is required")
	})

	t.Run("mandatory whitelist still applies", func(t *testing.T) {
		writePrefs(t, `{}`)
		// The hardcoded defaults keep the whitelist non-empty, so a transfer to
		// a non-default address is refused rather than waved through.
		requireRejected(t, Check(gateChainID, SignEVMTool, signEVMArgs(tokenContract, "0",
			tokenCall("transfer(address,uint256)", attacker.Bytes(), []byte{0x05}))),
			"not on the whitelist")
	})
}
