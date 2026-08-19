package signer_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/gogoproto/proto"
	"github.com/stretchr/testify/require"

	settlementmsgs "github.com/svpchain/svpchain-agent/internal/chainmsgs/settlement"
	"github.com/svpchain/svpchain-agent/internal/payload"
	"github.com/svpchain/svpchain-agent/internal/signer"
)

func openSettlementAny(t *testing.T, opener string, amount int64) *codectypes.Any {
	t.Helper()
	a, err := codectypes.NewAnyWithValue(&settlementmsgs.MsgOpenSettlement{
		Opener: opener,
		Cap:    sdk.Coin{Denom: "uusdc", Amount: sdkmath.NewInt(amount)},
		Memo:   "task",
	})
	require.NoError(t, err)
	return a
}

func TestSign_AllowsOwnOpenSettlement(t *testing.T) {
	priv := newRandomPriv(t)
	owner := sdk.AccAddress(priv.PubKey().Address()).String()

	body := marshalBody(t, openSettlementAny(t, owner, 500))
	p := payloadWithBody(body, payload.Summary{MsgTypeURL: "/dydxprotocol.settlement.MsgOpenSettlement"})
	p.SignerAddress = owner

	signed, err := signer.Sign(priv, p)
	require.NoError(t, err)
	require.NotEmpty(t, signed.TxRawBytesB64)
}

func TestSign_RejectsOpenSettlementForSomeoneElse(t *testing.T) {
	priv := newRandomPriv(t)
	owner := sdk.AccAddress(priv.PubKey().Address()).String()
	other := sdk.AccAddress([]byte("somebody_else_______")).String()

	body := marshalBody(t, openSettlementAny(t, other, 500))
	p := payloadWithBody(body, payload.Summary{MsgTypeURL: "/dydxprotocol.settlement.MsgOpenSettlement"})
	p.SignerAddress = owner

	_, err := signer.Sign(priv, p)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not the signing key")
}

func TestSign_RejectsZeroCapOpenSettlement(t *testing.T) {
	priv := newRandomPriv(t)
	owner := sdk.AccAddress(priv.PubKey().Address()).String()

	body := marshalBody(t, openSettlementAny(t, owner, 0))
	p := payloadWithBody(body, payload.Summary{MsgTypeURL: "/dydxprotocol.settlement.MsgOpenSettlement"})
	p.SignerAddress = owner

	_, err := signer.Sign(priv, p)
	require.Error(t, err)
	require.Contains(t, err.Error(), "positive coin")
}

func TestSign_AllowsOwnSettleAndRefund(t *testing.T) {
	priv := newRandomPriv(t)
	owner := sdk.AccAddress(priv.PubKey().Address()).String()

	settle, err := codectypes.NewAnyWithValue(&settlementmsgs.MsgSettle{
		Opener: owner, SettlementId: make([]byte, 32),
	})
	require.NoError(t, err)
	p := payloadWithBody(marshalBody(t, settle), payload.Summary{MsgTypeURL: "/dydxprotocol.settlement.MsgSettle"})
	p.SignerAddress = owner
	_, err = signer.Sign(priv, p)
	require.NoError(t, err)

	refund, err := codectypes.NewAnyWithValue(&settlementmsgs.MsgRefundSettlement{
		Opener: owner, SettlementId: make([]byte, 32),
	})
	require.NoError(t, err)
	p = payloadWithBody(marshalBody(t, refund), payload.Summary{MsgTypeURL: "/dydxprotocol.settlement.MsgRefundSettlement"})
	p.SignerAddress = owner
	_, err = signer.Sign(priv, p)
	require.NoError(t, err)
}

// A refund that attributes a slash is a governance move; the wallet must not
// sign it even for the user's own order.
func TestSign_RejectsRefundThatSlashes(t *testing.T) {
	priv := newRandomPriv(t)
	owner := sdk.AccAddress(priv.PubKey().Address()).String()

	refund, err := codectypes.NewAnyWithValue(&settlementmsgs.MsgRefundSettlement{
		Opener: owner, SettlementId: make([]byte, 32), SlashAgentId: "did:svp:someone",
	})
	require.NoError(t, err)
	p := payloadWithBody(marshalBody(t, refund), payload.Summary{MsgTypeURL: "/dydxprotocol.settlement.MsgRefundSettlement"})
	p.SignerAddress = owner

	_, err = signer.Sign(priv, p)
	require.Error(t, err)
	require.Contains(t, err.Error(), "slash_agent_id must be empty")
}

// RecordSpend and Claim are the paid agent's messages, not the user's: one
// arrives inside MsgAgentExecDelegated signed by the agent's operator, the
// other withdraws the operator's earnings. Neither is on the allowlist.
func TestSign_RejectsRecordSpendAndClaim(t *testing.T) {
	priv := newRandomPriv(t)
	owner := sdk.AccAddress(priv.PubKey().Address()).String()

	for _, msg := range []proto.Message{
		&settlementmsgs.MsgRecordSpend{
			Principal: owner, SettlementId: make([]byte, 32),
			AgentId: "did:svp:worker", Amount: sdk.Coin{Denom: "uusdc", Amount: sdkmath.NewInt(1)},
		},
		&settlementmsgs.MsgClaim{
			Claimer: owner, SettlementId: make([]byte, 32), AgentId: "did:svp:worker",
		},
	} {
		a, err := codectypes.NewAnyWithValue(msg)
		require.NoError(t, err)
		p := payloadWithBody(marshalBody(t, a), payload.Summary{MsgTypeURL: a.TypeUrl})
		p.SignerAddress = owner

		_, err = signer.Sign(priv, p)
		require.Error(t, err)
		require.Contains(t, err.Error(), "not on the signer allowlist")
	}
}
