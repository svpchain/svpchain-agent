package signer_test

import (
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/cosmos/evm/crypto/ethsecp256k1"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/payload"
	"github.com/svpchain/svpchain-agent/internal/signer"
)

// remoteGoldenPayload is the exact JSON the svpchain remote MCP server emits
// for an EVM build_* tool (captured from the remote's payload.EVMTxPayload —
// see protocol/lib/mcp/builder + the TestEVMTxPayload_WireShapeMatchesSigner
// guard there). It is pinned here as the cross-repo wire contract: if the two
// sides ever drift, this test fails. Key + address are the deterministic
// fixture the remote used to generate it (key bytes = 0x01..0x20).
const (
	remoteGoldenKeyHex  = "0102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f20"
	remoteGoldenAddr    = "0x6370eF2f4Db3611D657b90667De398a2Cc2a370C"
	remoteGoldenPayload = `{"version":1,"client_id":"cid-golden","evm_chain_id":"262144","signer_address":"0x6370eF2f4Db3611D657b90667De398a2Cc2a370C","tx_type":"eip1559","to":"0x000000000000000000000000000000000000dEaD","nonce":"7","gas":"125000","max_fee_per_gas":"5000000000","max_priority_fee_per_gas":"1000000000","value":"0","data":"0x4e71d92d","expires_at":"0001-01-01T00:00:00Z","summary":{"tool_name":"build_faucet_claim","description":"faucet claim()"}}`
)

// TestSignEvm_DecodesRemoteGoldenPayload proves the signer consumes the remote's
// EVM payload verbatim: decode the golden JSON into our EvmTxPayload, sign it,
// and confirm the result recovers to the expected sender. This is the
// end-to-end guarantee that build_*  → sign_evm_transaction works seamlessly.
func TestSignEvm_DecodesRemoteGoldenPayload(t *testing.T) {
	var p payload.EvmTxPayload
	require.NoError(t, json.Unmarshal([]byte(remoteGoldenPayload), &p))

	// The remote's field names populated our struct (no silent zero-values).
	require.Equal(t, payload.CurrentVersion, p.Version)
	require.Equal(t, "262144", p.EVMChainID)
	require.Equal(t, payload.EVMTxTypeEIP1559, p.TxType)
	require.Equal(t, remoteGoldenAddr, p.SignerAddress)
	require.Equal(t, "7", p.Nonce)
	require.Equal(t, "125000", p.Gas)
	require.Equal(t, "5000000000", p.MaxFeePerGas)
	require.Equal(t, "0x4e71d92d", p.Data)

	keyBz, err := hex.DecodeString(remoteGoldenKeyHex)
	require.NoError(t, err)
	priv := &ethsecp256k1.PrivKey{Key: keyBz}

	signed, err := signer.SignEvm(priv, &p, "")
	require.NoError(t, err)
	require.NotEmpty(t, signed.RawTxHex)

	raw, err := hexutil.Decode(signed.RawTxHex)
	require.NoError(t, err)
	var tx ethtypes.Transaction
	require.NoError(t, tx.UnmarshalBinary(raw))

	// Recovered sender == the golden address == the key — the same check
	// broadcast_evm_tx makes against the tenant owner on the remote side.
	from, err := ethtypes.Sender(ethtypes.LatestSignerForChainID(tx.ChainId()), &tx)
	require.NoError(t, err)
	require.Equal(t, common.HexToAddress(remoteGoldenAddr), from)
	require.Equal(t, uint8(ethtypes.DynamicFeeTxType), tx.Type())
	require.Equal(t, "262144", tx.ChainId().String())
}
