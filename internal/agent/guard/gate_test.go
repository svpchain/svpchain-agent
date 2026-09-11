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

	appconfig "github.com/svpchain/svpchain-agent/internal/config"
	"github.com/svpchain/svpchain-agent/internal/prefs"
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

	t.Run("transfer recipients must be whitelisted", func(t *testing.T) {
		writePrefs(t, `{"whitelist":[`+
			`{"chain_id":"`+gateChainID+`","address_type":"evm","address":"`+allowedEVM+`"},`+
			`{"chain_id":"`+gateChainID+`","address_type":"cosmos","address":"`+allowedCosmos+`"}`+
			`]}`)
		require.NoError(t, Check(gateChainID, "build_erc20_transfer",
			map[string]any{"to": allowedEVM}))
		require.NoError(t, Check(gateChainID, "build_bank_send",
			map[string]any{"recipient": allowedCosmos}))
		require.Error(t, Check(gateChainID, "build_erc20_transfer",
			map[string]any{"to": blockedEVM}))
		require.Error(t, Check(gateChainID, "build_bank_send",
			map[string]any{"recipient": blockedCosmos}))
	})

	t.Run("approval is not recipient-whitelisted", func(t *testing.T) {
		writePrefs(t, `{"whitelist":[{"chain_id":"`+gateChainID+`","address_type":"evm","address":"`+allowedEVM+`"}]}`)
		require.NoError(t, Check(gateChainID, "build_erc20_approve",
			map[string]any{"spender": blockedEVM}))
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

	t.Run("direct transfer signing enforces recipients but permits approvals", func(t *testing.T) {
		activeWhitelist(t)
		maxUint := make([]byte, 32)
		for i := range maxUint {
			maxUint[i] = 0xff
		}
		for _, args := range []map[string]any{
			signEVMArgs(tokenContract, "0", tokenCall("approve(address,uint256)", attacker.Bytes(), maxUint)),
			signEVMArgs(tokenContract, "0", tokenCall("setApprovalForAll(address,bool)", attacker.Bytes(), []byte{0x01})),
		} {
			require.NoError(t, Check(gateChainID, SignEVMTool, args))
		}
		require.NoError(t, Check(gateChainID, SignEVMTool,
			signEVMArgs(tokenContract, "0", tokenCall("transfer(address,uint256)", allowedEVM.Bytes(), []byte{0x05}))))
		require.Error(t, Check(gateChainID, SignEVMTool,
			signEVMArgs(tokenContract, "0", tokenCall("transfer(address,uint256)", attacker.Bytes(), []byte{0x05}))))
		require.Error(t, Check(gateChainID, SignEVMTool, signEVMArgs(attacker.Hex(), "1000", "")))
		// A payable contract call uses value for the protocol operation, not a
		// plain transfer to the contract. This is the shape produced by swaps.
		require.NoError(t, Check(gateChainID, SignEVMTool,
			signEVMArgs(attacker.Hex(), "1000", "0x7ff36ab5")))

		err := Check(gateChainID, SignEVMTool, map[string]any{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "payload is required")
	})
}
