package signer_test

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/payload"
	"github.com/svpchain/svpchain-agent/internal/signer"
)

const testEVMChainID = "1234"

// newEvmPayload returns a minimal EIP-1559 EvmTxPayload bound to signerAddr.
func newEvmPayload(signerAddr string) *payload.EvmTxPayload {
	return &payload.EvmTxPayload{
		Version:              payload.CurrentVersion,
		EVMChainID:           testEVMChainID,
		SignerAddress:        signerAddr,
		TxType:               payload.EVMTxTypeEIP1559,
		Nonce:                "7",
		To:                   "0x1111111111111111111111111111111111111111",
		Value:                "1000000000000000000", // 1 ether in wei
		Gas:                  "21000",
		MaxFeePerGas:         "2000000000",
		MaxPriorityFeePerGas: "1000000000",
		Summary:              payload.EvmSummary{ToolName: "test"},
	}
}

// decodeRaw decodes the SignedEvmTx raw hex back into a go-ethereum tx.
func decodeRaw(t *testing.T, raw string) *ethtypes.Transaction {
	t.Helper()
	b, err := hexutil.Decode(raw)
	require.NoError(t, err)
	tx := new(ethtypes.Transaction)
	require.NoError(t, tx.UnmarshalBinary(b))
	return tx
}

func TestSignEvm_EIP1559_RoundTrip(t *testing.T) {
	priv := newRandomPriv(t)
	addr := signer.DeriveEvmAddress(priv)
	p := newEvmPayload(addr)

	signed, err := signer.SignEvm(priv, p)
	require.NoError(t, err)
	require.NotEmpty(t, signed.RawTxHex)
	require.NotEmpty(t, signed.TxHash)

	tx := decodeRaw(t, signed.RawTxHex)
	require.Equal(t, uint8(ethtypes.DynamicFeeTxType), tx.Type())
	require.Equal(t, uint64(7), tx.Nonce())
	require.Equal(t, uint64(21000), tx.Gas())
	require.Equal(t, big.NewInt(1234), tx.ChainId())
	require.Equal(t, "1000000000000000000", tx.Value().String())
	require.Equal(t, big.NewInt(2000000000), tx.GasFeeCap())
	require.Equal(t, big.NewInt(1000000000), tx.GasTipCap())
	require.Equal(t, common.HexToAddress(p.To), *tx.To())
	require.Equal(t, tx.Hash().Hex(), signed.TxHash)

	// The recovered sender must be our key's address — proves the signature is
	// valid for this chain id and binds to the expected account.
	sender, err := ethtypes.Sender(ethtypes.LatestSignerForChainID(big.NewInt(1234)), tx)
	require.NoError(t, err)
	require.Equal(t, common.HexToAddress(addr), sender)
}

func TestSignEvm_Legacy_RoundTrip(t *testing.T) {
	priv := newRandomPriv(t)
	addr := signer.DeriveEvmAddress(priv)
	p := newEvmPayload(addr)
	p.TxType = payload.EVMTxTypeLegacy
	p.MaxFeePerGas = ""
	p.MaxPriorityFeePerGas = ""
	p.GasPrice = "3000000000"

	signed, err := signer.SignEvm(priv, p)
	require.NoError(t, err)

	tx := decodeRaw(t, signed.RawTxHex)
	require.Equal(t, uint8(ethtypes.LegacyTxType), tx.Type())
	require.Equal(t, big.NewInt(3000000000), tx.GasPrice())
	require.Equal(t, big.NewInt(1234), tx.ChainId(), "EIP-155 chain id must be embedded in v")

	sender, err := ethtypes.Sender(ethtypes.LatestSignerForChainID(big.NewInt(1234)), tx)
	require.NoError(t, err)
	require.Equal(t, common.HexToAddress(addr), sender)
}

func TestSignEvm_InfersTypeFromFields(t *testing.T) {
	priv := newRandomPriv(t)
	addr := signer.DeriveEvmAddress(priv)

	// No TxType, max_fee_per_gas set => EIP-1559.
	p := newEvmPayload(addr)
	p.TxType = ""
	signed, err := signer.SignEvm(priv, p)
	require.NoError(t, err)
	require.Equal(t, uint8(ethtypes.DynamicFeeTxType), decodeRaw(t, signed.RawTxHex).Type())

	// No TxType, no max_fee_per_gas but gas_price set => legacy.
	p = newEvmPayload(addr)
	p.TxType = ""
	p.MaxFeePerGas = ""
	p.MaxPriorityFeePerGas = ""
	p.GasPrice = "3000000000"
	signed, err = signer.SignEvm(priv, p)
	require.NoError(t, err)
	require.Equal(t, uint8(ethtypes.LegacyTxType), decodeRaw(t, signed.RawTxHex).Type())
}

func TestSignEvm_ContractCreation_NilTo(t *testing.T) {
	priv := newRandomPriv(t)
	addr := signer.DeriveEvmAddress(priv)
	p := newEvmPayload(addr)
	p.To = ""
	p.Data = "0x6001600101"

	signed, err := signer.SignEvm(priv, p)
	require.NoError(t, err)
	tx := decodeRaw(t, signed.RawTxHex)
	require.Nil(t, tx.To(), "empty To must yield a contract-creation tx")
	require.Equal(t, hexutil.MustDecode("0x6001600101"), tx.Data())
}

func TestSignEvm_RejectsAddressMismatch(t *testing.T) {
	priv := newRandomPriv(t)
	p := newEvmPayload("0x1111111111111111111111111111111111111111")
	_, err := signer.SignEvm(priv, p)
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not match payload.signer_address")
}

func TestSignEvm_AcceptsEmptySignerAddress(t *testing.T) {
	priv := newRandomPriv(t)
	p := newEvmPayload("")
	signed, err := signer.SignEvm(priv, p)
	require.NoError(t, err)
	require.NotEmpty(t, signed.RawTxHex)
}

func TestSignEvm_RejectsUnsupportedVersion(t *testing.T) {
	priv := newRandomPriv(t)
	p := newEvmPayload(signer.DeriveEvmAddress(priv))
	p.Version = 9999
	_, err := signer.SignEvm(priv, p)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported EvmTxPayload version")
}

func TestSignEvm_RejectsBadChainID(t *testing.T) {
	priv := newRandomPriv(t)
	p := newEvmPayload(signer.DeriveEvmAddress(priv))
	p.EVMChainID = "0"
	_, err := signer.SignEvm(priv, p)
	require.Error(t, err)
	require.Contains(t, err.Error(), "evm_chain_id must be positive")
}

func TestDeriveEvmAddress_MatchesKey(t *testing.T) {
	// The 0x address must be the EVM form of the same key the Cosmos path uses:
	// both derive from priv.PubKey().Address().
	priv := newRandomPriv(t)
	addr := signer.DeriveEvmAddress(priv)
	require.True(t, common.IsHexAddress(addr))
	require.Equal(t, common.BytesToAddress(priv.PubKey().Address()), common.HexToAddress(addr))
}
