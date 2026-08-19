package signer_test

import (
	"testing"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/chainmsgs"
	"github.com/svpchain/svpchain-agent/internal/payload"
	"github.com/svpchain/svpchain-agent/internal/signer"
)

func createDelegationAny(t *testing.T, delegator string) *codectypes.Any {
	t.Helper()
	a, err := codectypes.NewAnyWithValue(&chainmsgs.MsgCreateDelegation{
		Delegator: delegator,
		AgentId:   "did:svp:" + delegator,
		Limits:    chainmsgs.Limits{Actions: []string{"clob.place_order"}, Subaccounts: []uint32{0}},
		ExpiresAt: 1_900_000_000,
	})
	require.NoError(t, err)
	return a
}

func TestSign_AllowsOwnCreateDelegation(t *testing.T) {
	priv := newRandomPriv(t)
	owner := sdk.AccAddress(priv.PubKey().Address()).String()

	body := marshalBody(t, createDelegationAny(t, owner))
	p := payloadWithBody(body, payload.Summary{MsgTypeURL: "/dydxprotocol.agentwallet.MsgCreateDelegation"})
	p.SignerAddress = owner

	signed, err := signer.Sign(priv, p)
	require.NoError(t, err)
	require.NotEmpty(t, signed.TxRawBytesB64)
}

func TestSign_RejectsCreateDelegationForSomeoneElse(t *testing.T) {
	priv := newRandomPriv(t)
	owner := sdk.AccAddress(priv.PubKey().Address()).String()
	other := sdk.AccAddress([]byte("somebody_else_______")).String()

	body := marshalBody(t, createDelegationAny(t, other))
	p := payloadWithBody(body, payload.Summary{MsgTypeURL: "/dydxprotocol.agentwallet.MsgCreateDelegation"})
	p.SignerAddress = owner

	_, err := signer.Sign(priv, p)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not the signing key")
}

func TestSign_AllowsPauseDelegation(t *testing.T) {
	priv := newRandomPriv(t)
	owner := sdk.AccAddress(priv.PubKey().Address()).String()

	a, err := codectypes.NewAnyWithValue(&chainmsgs.MsgPauseDelegation{
		Delegator: owner,
		RootId:    make([]byte, 32),
	})
	require.NoError(t, err)

	p := payloadWithBody(marshalBody(t, a), payload.Summary{MsgTypeURL: "/dydxprotocol.agentwallet.MsgPauseDelegation"})
	p.SignerAddress = owner

	_, err = signer.Sign(priv, p)
	require.NoError(t, err)
}
