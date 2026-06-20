package signer_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-mcp/internal/payload"
	"github.com/svpchain/svpchain-mcp/internal/signer"
)

func x402TypedDataTypes() map[string][]payload.EIP712Type {
	return map[string][]payload.EIP712Type{
		"EIP712Domain": {
			{Name: "name", Type: "string"},
			{Name: "version", Type: "string"},
			{Name: "chainId", Type: "uint256"},
			{Name: "verifyingContract", Type: "address"},
		},
		"TransferWithAuthorization": {
			{Name: "from", Type: "address"},
			{Name: "to", Type: "address"},
			{Name: "value", Type: "uint256"},
			{Name: "validAfter", Type: "uint256"},
			{Name: "validBefore", Type: "uint256"},
			{Name: "nonce", Type: "bytes32"},
		},
	}
}

func newX402TypedData(signerAddr string) *payload.EIP712TypedData {
	return &payload.EIP712TypedData{
		Types:       x402TypedDataTypes(),
		PrimaryType: signer.PrimaryTypeTransferWithAuthorization,
		Domain: payload.EIP712Domain{
			Name:              "USD Coin",
			Version:           "2",
			ChainId:           testEVMChainID,
			VerifyingContract: "0x036CbD53842c5426634e7929541eC2318f3dCF7e",
		},
		Message: map[string]interface{}{
			"from":        signerAddr,
			"to":          "0x209693Bc6afc0C5328bA36FaF03C514EF312287C",
			"value":       "10000",
			"validAfter":  "1740672089",
			"validBefore": "1740672154",
			"nonce":       "0xf3746613c2d920b5fdabc0856f2aeb2d4f88ee6037b8cc5d04a71a4462f13480",
		},
	}
}

func TestSignTypedData_TransferWithAuthorization(t *testing.T) {
	priv := newRandomPriv(t)
	addr := signer.DeriveEvmAddress(priv)
	td := newX402TypedData(addr)

	signed, err := signer.SignTypedData(priv, td, 1234)
	require.NoError(t, err)
	require.Equal(t, addr, signed.Signer)
	require.True(t, len(signed.Signature) > 2 && signed.Signature[:2] == "0x")

	bz, err := json.Marshal(td)
	require.NoError(t, err)
	var apiTD apitypes.TypedData
	require.NoError(t, json.Unmarshal(bz, &apiTD))
	digest, _, err := apitypes.TypedDataAndHash(apiTD)
	require.NoError(t, err)

	sig, err := hexutil.Decode(signed.Signature)
	require.NoError(t, err)
	require.Len(t, sig, 65)
	require.GreaterOrEqual(t, sig[64], byte(27))
	sig[64] -= 27

	pub, err := ethcrypto.SigToPub(digest, sig)
	require.NoError(t, err)
	require.Equal(t, common.HexToAddress(addr), ethcrypto.PubkeyToAddress(*pub))
}

func permit2TypedDataTypes() map[string][]payload.EIP712Type {
	return map[string][]payload.EIP712Type{
		"EIP712Domain": {
			{Name: "name", Type: "string"},
			{Name: "chainId", Type: "uint256"},
			{Name: "verifyingContract", Type: "address"},
		},
		"PermitWitnessTransferFrom": {
			{Name: "permitted", Type: "TokenPermissions"},
			{Name: "spender", Type: "address"},
			{Name: "nonce", Type: "uint256"},
			{Name: "deadline", Type: "uint256"},
			{Name: "witness", Type: "Witness"},
		},
		"TokenPermissions": {
			{Name: "token", Type: "address"},
			{Name: "amount", Type: "uint256"},
		},
		"Witness": {
			{Name: "to", Type: "address"},
			{Name: "validAfter", Type: "uint256"},
		},
	}
}

func newPermit2TypedData() *payload.EIP712TypedData {
	return &payload.EIP712TypedData{
		Types:       permit2TypedDataTypes(),
		PrimaryType: signer.PrimaryTypePermitWitnessTransferFrom,
		Domain: payload.EIP712Domain{
			Name:              signer.Permit2DomainName,
			ChainId:           testEVMChainID,
			VerifyingContract: signer.CanonicalPermit2Address,
		},
		Message: map[string]interface{}{
			"permitted": map[string]interface{}{
				"token":  "0x036CbD53842c5426634e7929541eC2318f3dCF7e",
				"amount": "10000",
			},
			"spender":  "0x402085c248EeA27D92E8b30b2C58ed07f9E20001",
			"nonce":    "33247007178036348590600198031289925668252061821958005840077069883511451257277",
			"deadline": "1740672154",
			"witness": map[string]interface{}{
				"to":         "0x209693Bc6afc0C5328bA36FaF03C514EF312287C",
				"validAfter": "1740672089",
			},
		},
	}
}

func TestSignTypedData_PermitWitnessTransferFrom(t *testing.T) {
	priv := newRandomPriv(t)
	addr := signer.DeriveEvmAddress(priv)
	td := newPermit2TypedData()

	signed, err := signer.SignTypedData(priv, td, 1234)
	require.NoError(t, err)
	require.Equal(t, addr, signed.Signer)
	require.True(t, strings.HasPrefix(signed.Signature, "0x"))

	bz, err := json.Marshal(td)
	require.NoError(t, err)
	var apiTD apitypes.TypedData
	require.NoError(t, json.Unmarshal(bz, &apiTD))
	digest, _, err := apitypes.TypedDataAndHash(apiTD)
	require.NoError(t, err)

	sig, err := hexutil.Decode(signed.Signature)
	require.NoError(t, err)
	require.Len(t, sig, 65)
	sig[64] -= 27

	pub, err := ethcrypto.SigToPub(digest, sig)
	require.NoError(t, err)
	require.Equal(t, common.HexToAddress(addr), ethcrypto.PubkeyToAddress(*pub))
}

func TestSignTypedData_Permit2_RejectsWrongDomain(t *testing.T) {
	priv := newRandomPriv(t)
	td := newPermit2TypedData()
	td.Domain.VerifyingContract = "0x1111111111111111111111111111111111111111"

	_, err := signer.SignTypedData(priv, td, 1234)
	require.Error(t, err)
	require.Contains(t, err.Error(), "canonical Permit2")
}

func TestSignTypedData_RejectsWrongPrimaryType(t *testing.T) {
	priv := newRandomPriv(t)
	td := newX402TypedData(signer.DeriveEvmAddress(priv))
	td.PrimaryType = "Permit"

	_, err := signer.SignTypedData(priv, td, 1234)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not allowed")
}

func TestSignTypedData_RejectsChainIDMismatch(t *testing.T) {
	priv := newRandomPriv(t)
	td := newX402TypedData(signer.DeriveEvmAddress(priv))
	td.Domain.ChainId = "9999"

	_, err := signer.SignTypedData(priv, td, 1234)
	require.Error(t, err)
	require.Contains(t, err.Error(), "domain.chainId")
}

func TestSignTypedData_RejectsFromMismatch(t *testing.T) {
	priv := newRandomPriv(t)
	td := newX402TypedData("0x1111111111111111111111111111111111111111")

	_, err := signer.SignTypedData(priv, td, 1234)
	require.Error(t, err)
	require.Contains(t, err.Error(), "message.from")
}

func TestSignTypedData_DisabledWhenNoChainID(t *testing.T) {
	priv := newRandomPriv(t)
	td := newX402TypedData(signer.DeriveEvmAddress(priv))

	_, err := signer.SignTypedData(priv, td, 0)
	require.Error(t, err)
	require.Contains(t, err.Error(), "EVM signing is not configured")
}
