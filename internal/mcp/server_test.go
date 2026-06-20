package mcp

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"testing"
	"time"

	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/evm/crypto/ethsecp256k1"
	"github.com/cosmos/gogoproto/proto"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-mcp/internal/payload"
	"github.com/svpchain/svpchain-mcp/internal/signer"
)

var _ = signer.DeriveAddress

func newRandomPriv(t *testing.T) *ethsecp256k1.PrivKey {
	t.Helper()
	bz := make([]byte, 32)
	_, err := rand.Read(bz)
	require.NoError(t, err)
	return &ethsecp256k1.PrivKey{Key: bz}
}

func newTestPayload(t *testing.T, chainID, signerAddr string) payload.TxPayload {
	t.Helper()
	body := &txtypes.TxBody{Memo: "mcp-signer-test"}
	bodyBytes, err := proto.Marshal(body)
	require.NoError(t, err)
	return payload.TxPayload{
		Version:         payload.CurrentVersion,
		ClientID:        "test-client-id",
		ChainID:         chainID,
		SignerAddress:   signerAddr,
		AccountNumber:   "42",
		Sequence:        "17",
		IsShortTermCLOB: true,
		TxBodyBytesB64:  base64.StdEncoding.EncodeToString(bodyBytes),
		Fee: payload.Fee{
			GasLimit: "1000000",
			Amount:   []payload.Coin{},
		},
		Summary:   payload.Summary{ToolName: "test"},
		ExpiresAt: time.Now().UTC().Add(30 * time.Second),
	}
}

func TestSignTransaction_Happy(t *testing.T) {
	priv := newRandomPriv(t)
	addr := signer.DeriveAddress(priv)
	h := &Handlers{Priv: priv, ChainID: "localsvp-1"}

	_, out, err := h.SignTransaction(context.Background(), nil, SignTransactionInput{
		Payload: newTestPayload(t, "localsvp-1", addr),
	})
	require.NoError(t, err)
	require.NotEmpty(t, out.SignedTx.TxRawBytesB64)
	rawBytes, err := base64.StdEncoding.DecodeString(out.SignedTx.TxRawBytesB64)
	require.NoError(t, err)
	var raw txtypes.TxRaw
	require.NoError(t, proto.Unmarshal(rawBytes, &raw))
	require.Len(t, raw.Signatures, 1)
}

func TestSignTransaction_ChainIDMismatch(t *testing.T) {
	priv := newRandomPriv(t)
	addr := signer.DeriveAddress(priv)
	h := &Handlers{Priv: priv, ChainID: "localsvp-1"}

	_, _, err := h.SignTransaction(context.Background(), nil, SignTransactionInput{
		Payload: newTestPayload(t, "other-chain", addr),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "chain_id")
	require.Contains(t, err.Error(), "other-chain")
}

func TestSignTransaction_SignerAddressMismatch(t *testing.T) {
	priv := newRandomPriv(t)
	h := &Handlers{Priv: priv, ChainID: "localsvp-1"}
	_, _, err := h.SignTransaction(context.Background(), nil, SignTransactionInput{
		Payload: newTestPayload(t, "localsvp-1", "svp1someotherbech32stringthatwontmatch"),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not match payload.signer_address")
}

func newTestEvmPayload(evmChainID, signerAddr string) payload.EvmTxPayload {
	return payload.EvmTxPayload{
		Version:              payload.CurrentVersion,
		EVMChainID:           evmChainID,
		SignerAddress:        signerAddr,
		TxType:               payload.EVMTxTypeEIP1559,
		Nonce:                "0",
		To:                   "0x1111111111111111111111111111111111111111",
		Value:                "1000",
		Gas:                  "21000",
		MaxFeePerGas:         "2000000000",
		MaxPriorityFeePerGas: "1000000000",
		Summary:              payload.EvmSummary{ToolName: "test"},
	}
}

func TestSignEvmTransaction_Happy(t *testing.T) {
	priv := newRandomPriv(t)
	addr := signer.DeriveEvmAddress(priv)
	h := &Handlers{Priv: priv, ChainID: "localsvp_1234-1", EVMChainID: 1234}

	_, out, err := h.SignEvmTransaction(context.Background(), nil, SignEvmTransactionInput{
		Payload: newTestEvmPayload("1234", addr),
	})
	require.NoError(t, err)
	require.NotEmpty(t, out.SignedTx.RawTxHex)
	require.NotEmpty(t, out.SignedTx.TxHash)
}

func TestSignEvmTransaction_ChainIDMismatch(t *testing.T) {
	// Cross-chain replay guard, EVM side: payload targets a different EVM
	// chain than the signer is bound to.
	priv := newRandomPriv(t)
	addr := signer.DeriveEvmAddress(priv)
	h := &Handlers{Priv: priv, ChainID: "localsvp_1234-1", EVMChainID: 1234}

	_, _, err := h.SignEvmTransaction(context.Background(), nil, SignEvmTransactionInput{
		Payload: newTestEvmPayload("9999", addr),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "evm_chain_id")
	require.Contains(t, err.Error(), "9999")
}

func TestSignEvmTransaction_DisabledWhenNoChainID(t *testing.T) {
	// EVMChainID 0 means the signer couldn't determine its EVM chain id; it
	// must refuse rather than sign for an attacker-chosen chain.
	priv := newRandomPriv(t)
	addr := signer.DeriveEvmAddress(priv)
	h := &Handlers{Priv: priv, ChainID: "localsvp-1"}

	_, _, err := h.SignEvmTransaction(context.Background(), nil, SignEvmTransactionInput{
		Payload: newTestEvmPayload("1234", addr),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "EVM signing is not configured")
}

func TestSignEvmTransaction_SignerAddressMismatch(t *testing.T) {
	priv := newRandomPriv(t)
	h := &Handlers{Priv: priv, ChainID: "localsvp_1234-1", EVMChainID: 1234}
	_, _, err := h.SignEvmTransaction(context.Background(), nil, SignEvmTransactionInput{
		Payload: newTestEvmPayload("1234", "0x1111111111111111111111111111111111111111"),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not match payload.signer_address")
}

func newTestTypedData(signerAddr string) payload.EIP712TypedData {
	return payload.EIP712TypedData{
		Types: map[string][]payload.EIP712Type{
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
		},
		PrimaryType: signer.PrimaryTypeTransferWithAuthorization,
		Domain: payload.EIP712Domain{
			Name:              "USD Coin",
			Version:           "2",
			ChainId:           "1234",
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

func TestSignTypedData_Happy(t *testing.T) {
	priv := newRandomPriv(t)
	addr := signer.DeriveEvmAddress(priv)
	h := &Handlers{Priv: priv, ChainID: "localsvp_1234-1", EVMChainID: 1234}

	_, out, err := h.SignTypedData(context.Background(), nil, SignTypedDataInput{
		TypedData: newTestTypedData(addr),
	})
	require.NoError(t, err)
	require.NotEmpty(t, out.Signed.Signature)
	require.Equal(t, addr, out.Signed.Signer)
}

func TestSignTypedData_ChainIDMismatch(t *testing.T) {
	priv := newRandomPriv(t)
	addr := signer.DeriveEvmAddress(priv)
	h := &Handlers{Priv: priv, ChainID: "localsvp_1234-1", EVMChainID: 1234}

	td := newTestTypedData(addr)
	td.Domain.ChainId = "9999"
	_, _, err := h.SignTypedData(context.Background(), nil, SignTypedDataInput{TypedData: td})
	require.Error(t, err)
	require.Contains(t, err.Error(), "domain.chainId")
}

func newTestPermit2TypedData() payload.EIP712TypedData {
	return payload.EIP712TypedData{
		Types: map[string][]payload.EIP712Type{
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
		},
		PrimaryType: signer.PrimaryTypePermitWitnessTransferFrom,
		Domain: payload.EIP712Domain{
			Name:              signer.Permit2DomainName,
			ChainId:           "1234",
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

func TestSignTypedData_Permit2_Happy(t *testing.T) {
	priv := newRandomPriv(t)
	addr := signer.DeriveEvmAddress(priv)
	h := &Handlers{Priv: priv, ChainID: "localsvp_1234-1", EVMChainID: 1234}

	_, out, err := h.SignTypedData(context.Background(), nil, SignTypedDataInput{
		TypedData: newTestPermit2TypedData(),
	})
	require.NoError(t, err)
	require.NotEmpty(t, out.Signed.Signature)
	require.Equal(t, addr, out.Signed.Signer)
}

func TestSignTypedData_DisabledWhenNoChainID(t *testing.T) {
	priv := newRandomPriv(t)
	addr := signer.DeriveEvmAddress(priv)
	h := &Handlers{Priv: priv, ChainID: "localsvp-1"}

	_, _, err := h.SignTypedData(context.Background(), nil, SignTypedDataInput{
		TypedData: newTestTypedData(addr),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "EVM signing is not configured")
}

func TestSignChallenge_Happy(t *testing.T) {
	priv := newRandomPriv(t)
	h := &Handlers{Priv: priv, ChainID: "localsvp-1"}

	challenge := "svpchain-mcp-auth-v1:localsvp-1:abc123def456:1780000000"
	_, out, err := h.SignChallenge(context.Background(), nil, SignChallengeInput{
		Challenge: challenge,
	})
	require.NoError(t, err)
	require.NotEmpty(t, out.Signature)
	require.Equal(t, signer.DeriveAddress(priv), out.Owner)
	sigBytes, err := base64.StdEncoding.DecodeString(out.Signature)
	require.NoError(t, err)
	require.True(t, priv.PubKey().VerifySignature([]byte(challenge), sigBytes))
}

func TestSignChallenge_RefusesArbitraryText(t *testing.T) {
	priv := newRandomPriv(t)
	h := &Handlers{Priv: priv, ChainID: "localsvp-1"}

	cases := []string{
		"hello world",
		"give me 1 ETH",
		"svpchain-mcp-auth-v2:localsvp-1:nonce:1780000000",
		"x" + ChallengePrefix + "localsvp-1:nonce:1780000000",
		"",
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			_, _, err := h.SignChallenge(context.Background(), nil, SignChallengeInput{
				Challenge: c,
			})
			require.Error(t, err, "must refuse %q", c)
			require.Contains(t, err.Error(), "challenge")
		})
	}
}

func TestSignChallenge_ChainIDMismatch(t *testing.T) {
	priv := newRandomPriv(t)
	h := &Handlers{Priv: priv, ChainID: "localsvp-1"}

	_, _, err := h.SignChallenge(context.Background(), nil, SignChallengeInput{
		Challenge: "svpchain-mcp-auth-v1:other-chain:abc123:1780000000",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "chain_id")
	require.Contains(t, err.Error(), "other-chain")
}

func TestSignChallenge_MalformedFormat(t *testing.T) {
	priv := newRandomPriv(t)
	h := &Handlers{Priv: priv, ChainID: "localsvp-1"}
	_, _, err := h.SignChallenge(context.Background(), nil, SignChallengeInput{
		Challenge: "svpchain-mcp-auth-v1:",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "malformed")
}

func TestWhoami(t *testing.T) {
	priv := newRandomPriv(t)
	h := &Handlers{Priv: priv, ChainID: "localsvp_1234-1", EVMChainID: 1234}

	_, out, err := h.Whoami(context.Background(), nil, WhoamiInput{})
	require.NoError(t, err)
	require.Equal(t, "localsvp_1234-1", out.ChainID)
	require.Equal(t, signer.DeriveAddress(priv), out.Owner)
	require.True(t, len(out.Owner) > 4 && out.Owner[:4] == "svp1")
	// EVM identity for the same key.
	require.Equal(t, signer.DeriveEvmAddress(priv), out.EVMOwner)
	require.Equal(t, "0x", out.EVMOwner[:2])
	require.Equal(t, "1234", out.EVMChainID)
}

func TestRegister_NoSchemaPanic(t *testing.T) {
	srv := mcp.NewServer(&mcp.Implementation{
		Name: "test", Version: "v0.0.0",
	}, nil)
	require.NotPanics(t, func() {
		Register(srv, &Handlers{})
	})
}
