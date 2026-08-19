package signer_test

import (
	"encoding/base64"
	"testing"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/gogoproto/proto"
	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/payload"
	"github.com/svpchain/svpchain-agent/internal/signer"
)

// payloadWithBody wraps raw TxBody bytes in a minimal signable TxPayload with an
// empty SignerAddress (so the address cross-check is skipped and the TxBody
// policy is what's exercised).
func payloadWithBody(bodyBytes []byte, summary payload.Summary) *payload.TxPayload {
	return &payload.TxPayload{
		Version:        payload.CurrentVersion,
		ChainID:        "localsvp-1",
		AccountNumber:  "42",
		Sequence:       "17",
		TxBodyBytesB64: base64.StdEncoding.EncodeToString(bodyBytes),
		Fee:            payload.Fee{GasLimit: "1000000"},
		Summary:        summary,
	}
}

func marshalBody(t *testing.T, msgs ...*codectypes.Any) []byte {
	t.Helper()
	bz, err := proto.Marshal(&txtypes.TxBody{Messages: msgs})
	require.NoError(t, err)
	return bz
}

func bankSendAny(t *testing.T, from, to string) *codectypes.Any {
	t.Helper()
	a, err := codectypes.NewAnyWithValue(&banktypes.MsgSend{
		FromAddress: from,
		ToAddress:   to,
		Amount:      sdk.NewCoins(sdk.NewInt64Coin("asvp", 1)),
	})
	require.NoError(t, err)
	return a
}

func TestSign_RejectsEmptyMessageBody(t *testing.T) {
	priv := newRandomPriv(t)
	_, err := signer.Sign(priv, payloadWithBody(marshalBody(t), payload.Summary{ToolName: "x"}))
	require.Error(t, err)
	require.Contains(t, err.Error(), "no messages")
}

func TestSign_RejectsMemoOnlyBody(t *testing.T) {
	priv := newRandomPriv(t)
	bz, err := proto.Marshal(&txtypes.TxBody{Memo: "just a memo"})
	require.NoError(t, err)
	_, err = signer.Sign(priv, payloadWithBody(bz, payload.Summary{}))
	require.Error(t, err)
	require.Contains(t, err.Error(), "no messages")
}

func TestSign_RejectsNonAllowlistedMsgType(t *testing.T) {
	priv := newRandomPriv(t)
	// A message the wallet should never sign, slipped in behind a build_* flow.
	bad := &codectypes.Any{TypeUrl: "/cosmos.authz.v1beta1.MsgExec", Value: []byte{0x01}}
	_, err := signer.Sign(priv, payloadWithBody(marshalBody(t, bad), payload.Summary{}))
	require.Error(t, err)
	require.Contains(t, err.Error(), "not on the signer allowlist")
	require.Contains(t, err.Error(), "MsgExec")
}

func TestSign_RejectsSummaryTypeMismatch(t *testing.T) {
	priv := newRandomPriv(t)
	from := signer.DeriveAddress(priv)
	// Body is a bank send, but the Summary claims a place-order — tampering.
	s := payload.Summary{MsgTypeURL: "/dydxprotocol.clob.MsgPlaceOrder"}
	_, err := signer.Sign(priv, payloadWithBody(marshalBody(t, bankSendAny(t, from, from)), s))
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not match any message")
}

func TestSign_RejectsUndecodableBody(t *testing.T) {
	priv := newRandomPriv(t)
	// Length-delimited field claiming 5 bytes that don't follow → decode error.
	_, err := signer.Sign(priv, payloadWithBody([]byte{0x0a, 0x05}, payload.Summary{}))
	require.Error(t, err)
	require.Contains(t, err.Error(), "decode tx body")
}

func TestSign_RejectsBankSendFromWrongAddress(t *testing.T) {
	priv := newRandomPriv(t)
	other := signer.DeriveAddress(newRandomPriv(t))
	// MsgSend spends from some other account, not the signing key.
	_, err := signer.Sign(priv, payloadWithBody(marshalBody(t, bankSendAny(t, other, other)), payload.Summary{}))
	require.Error(t, err)
	require.Contains(t, err.Error(), "is not the signing key")
}

func TestSign_AllowsDydxOrderByTypeURL(t *testing.T) {
	priv := newRandomPriv(t)
	// dYdX order: no Go binding, so only the type URL is checked; the opaque
	// value is signed as-is.
	order := &codectypes.Any{TypeUrl: "/dydxprotocol.clob.MsgPlaceOrder", Value: []byte{0x0a, 0x02, 0x08, 0x01}}
	s := payload.Summary{MsgTypeURL: "/dydxprotocol.clob.MsgPlaceOrder"}
	signed, err := signer.Sign(priv, payloadWithBody(marshalBody(t, order), s))
	require.NoError(t, err)
	require.NotEmpty(t, signed.TxRawBytesB64)
}
