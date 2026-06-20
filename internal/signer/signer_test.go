package signer_test

import (
	"crypto/rand"
	"encoding/base64"
	"strconv"
	"strings"
	"testing"
	"time"

	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/evm/crypto/ethsecp256k1"
	"github.com/cosmos/gogoproto/proto"
	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-mcp/internal/payload"
	"github.com/svpchain/svpchain-mcp/internal/signer"
)

func newRandomPriv(t *testing.T) *ethsecp256k1.PrivKey {
	t.Helper()
	bz := make([]byte, 32)
	_, err := rand.Read(bz)
	require.NoError(t, err)
	return &ethsecp256k1.PrivKey{Key: bz}
}

// newSyntheticPayload returns a TxPayload with a minimal parseable TxBody, enough to exercise sign/decode/verify without a real Msg.
func newSyntheticPayload(t *testing.T, signerAddr string) *payload.TxPayload {
	t.Helper()
	// Minimal proto-serialized TxBody (memo only, no msgs); the chain would reject it but sign/decode/verify paths work;
	// non-empty memo ensures Marshal produces non-nil bytes and proto.Unmarshal round-trips.
	body := &txtypes.TxBody{Memo: "signer-roundtrip"}
	bodyBytes, err := proto.Marshal(body)
	require.NoError(t, err)
	return &payload.TxPayload{
		Version:         payload.CurrentVersion,
		ClientID:        "test-client-id",
		ChainID:         "localsvp-1",
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

func TestSign_HonorsPayloadFee(t *testing.T) {
	// A non-CLOB payload carries a fee; the signer must thread it into
	// AuthInfo.Fee.Amount (rather than the old hardcoded nil) or the chain
	// rejects with code 13. Regression test for the empty-fee deposit.
	priv := newRandomPriv(t)
	addr := signer.DeriveAddress(priv)
	p := newSyntheticPayload(t, addr)
	p.IsShortTermCLOB = false
	p.Fee.Amount = []payload.Coin{{Denom: "asvp", Amount: "25000000000000000"}}

	signed, err := signer.Sign(priv, p)
	require.NoError(t, err)

	var raw txtypes.TxRaw
	rawBytes, err := base64.StdEncoding.DecodeString(signed.TxRawBytesB64)
	require.NoError(t, err)
	require.NoError(t, proto.Unmarshal(rawBytes, &raw))

	var ai txtypes.AuthInfo
	require.NoError(t, proto.Unmarshal(raw.AuthInfoBytes, &ai))
	require.NotNil(t, ai.Fee)
	require.Len(t, ai.Fee.Amount, 1)
	require.Equal(t, "asvp", ai.Fee.Amount[0].Denom)
	require.Equal(t, "25000000000000000", ai.Fee.Amount[0].Amount.String())
}

func TestSign_RoundTripDecode(t *testing.T) {
	priv := newRandomPriv(t)
	addr := signer.DeriveAddress(priv)
	p := newSyntheticPayload(t, addr)

	signed, err := signer.Sign(priv, p)
	require.NoError(t, err)
	require.NotEmpty(t, signed.TxRawBytesB64)
	require.NotEmpty(t, signed.SignatureB64)
	require.NotEmpty(t, signed.PubKeyB64)

	var raw txtypes.TxRaw
	rawBytes, err := base64.StdEncoding.DecodeString(signed.TxRawBytesB64)
	require.NoError(t, err)
	require.NoError(t, proto.Unmarshal(rawBytes, &raw))

	// Signed tx TxBody bytes must match input; TxPayload carries TxBody as base64 — decode before compare.
	wantBody, err := base64.StdEncoding.DecodeString(p.TxBodyBytesB64)
	require.NoError(t, err)
	require.Equal(t, wantBody, raw.BodyBytes, "TxBody must round-trip unchanged")

	// AuthInfo must carry this signer's sequence and a single SignerInfo.
	var ai txtypes.AuthInfo
	require.NoError(t, proto.Unmarshal(raw.AuthInfoBytes, &ai))
	require.Len(t, ai.SignerInfos, 1)
	gotSeq, err := strconv.ParseUint(p.Sequence, 10, 64)
	require.NoError(t, err)
	require.Equal(t, gotSeq, ai.SignerInfos[0].Sequence)
	require.NotNil(t, ai.Fee)
	gotGas, err := strconv.ParseUint(p.Fee.GasLimit, 10, 64)
	require.NoError(t, err)
	require.Equal(t, gotGas, ai.Fee.GasLimit)
	require.Empty(t, ai.Fee.Amount, "CLOB Fee.Amount must stay empty")

	// Exactly one signature.
	require.Len(t, raw.Signatures, 1)
	wantSig, err := base64.StdEncoding.DecodeString(signed.SignatureB64)
	require.NoError(t, err)
	require.Equal(t, wantSig, raw.Signatures[0])
}

func TestSign_SignatureVerifies(t *testing.T) {
	priv := newRandomPriv(t)
	addr := signer.DeriveAddress(priv)
	p := newSyntheticPayload(t, addr)

	signed, err := signer.Sign(priv, p)
	require.NoError(t, err)

	// Recompute sign bytes from signed tx AuthInfo + TxBody (verifier view) and confirm verification passes.
	var raw txtypes.TxRaw
	rawBytes, err := base64.StdEncoding.DecodeString(signed.TxRawBytesB64)
	require.NoError(t, err)
	require.NoError(t, proto.Unmarshal(rawBytes, &raw))
	accNum, _ := strconv.ParseUint(p.AccountNumber, 10, 64)
	signBytes, err := payload.DirectSignBytes(raw.BodyBytes, raw.AuthInfoBytes, p.ChainID, accNum)
	require.NoError(t, err)
	require.True(t, priv.PubKey().VerifySignature(signBytes, raw.Signatures[0]),
		"signature must verify against recomputed sign-bytes")
}

func TestSign_RejectsAddressMismatch(t *testing.T) {
	// Use one key but declare a different signer address in the payload — Sign must reject
	// rather than silently produce a tx the server would also reject.
	priv := newRandomPriv(t)
	p := newSyntheticPayload(t, "svp1someotherbech32stringthatwontmatch")
	_, err := signer.Sign(priv, p)
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not match payload.signer_address")
}

func TestSign_AcceptsEmptySignerAddress(t *testing.T) {
	// If the server omits signer_address (e.g. temporary demo), Sign should still sign with the caller's key.
	priv := newRandomPriv(t)
	p := newSyntheticPayload(t, "")
	signed, err := signer.Sign(priv, p)
	require.NoError(t, err)
	require.NotEmpty(t, signed.TxRawBytesB64)
}

func TestSign_RejectsUnsupportedVersion(t *testing.T) {
	priv := newRandomPriv(t)
	p := newSyntheticPayload(t, signer.DeriveAddress(priv))
	p.Version = 9999
	_, err := signer.Sign(priv, p)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported TxPayload version")
}

func TestParsePrivKey(t *testing.T) {
	priv := newRandomPriv(t)
	hexKey := ""
	for _, b := range priv.Key {
		hexKey += stringFromByte(b)
	}

	parsed, err := signer.ParsePrivKey(hexKey)
	require.NoError(t, err)
	require.Equal(t, priv.Key, parsed.Key)

	_, err = signer.ParsePrivKey("0x" + hexKey)
	require.NoError(t, err, "leading 0x should be tolerated")

	_, err = signer.ParsePrivKey("not-hex")
	require.Error(t, err)

	_, err = signer.ParsePrivKey("dead")
	require.Error(t, err, "wrong-length key must be rejected")
	require.Contains(t, err.Error(), "32 bytes")
}

func TestDeriveAddress_HasSvpPrefix(t *testing.T) {
	// init() must set bech32 prefixes or DeriveAddress returns "cosmos1..." and addresses silently mismatch.
	priv := newRandomPriv(t)
	addr := signer.DeriveAddress(priv)
	require.True(t, len(addr) > 4 && addr[:4] == "svp1", "expected svp1 prefix, got %s", addr)
}

func TestDeriveEVMAddress_Has0xPrefix(t *testing.T) {
	priv := newRandomPriv(t)
	addr := signer.DeriveEVMAddress(priv)
	require.True(t, strings.HasPrefix(addr, "0x"), "expected 0x prefix, got %s", addr)
	require.Len(t, addr, 42)
}

// stringFromByte returns a lowercase two-digit hex string for b.
func stringFromByte(b byte) string {
	const hexDigits = "0123456789abcdef"
	return string([]byte{hexDigits[b>>4], hexDigits[b&0x0f]})
}
