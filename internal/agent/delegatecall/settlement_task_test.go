package delegatecall

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cosmos/gogoproto/proto"
	"github.com/stretchr/testify/require"
	"github.com/svpchain/svpdt"

	svpa2a "github.com/svpchain/svpchain-agent/internal/a2a"
	settlementmsgs "github.com/svpchain/svpchain-agent/internal/chainmsgs/settlement"
)

// stubSettlementIDHex matches the order the stub chain lists for the opener.
func stubSettlementIDHex() string {
	id := make([]byte, 32)
	id[0] = 0xC0
	return hex.EncodeToString(id)
}

func serviceTaskArgs() map[string]any {
	args := taskArgs()
	args["service_budget"] = map[string]any{"denom": "uusdc", "amount": "500000"}
	return args
}

// A paid task opens the escrow with the user's key and binds the credential to
// it: the settlement caveat names the order, the service budget matches the
// cap, and the recording grant (action + subaccount 0) is added and therefore
// shown to the user.
func TestDelegateTaskWithServicePaymentBindsTheCredential(t *testing.T) {
	svc, exec, life, capture := stubStack(t)

	out, err := svc.Call(context.Background(), "delegate_task", serviceTaskArgs())
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &result))
	require.Equal(t, stubSettlementIDHex(), result["settlement_id"],
		"the caller must learn the order id, or it can never settle")

	// The escrow was opened by the user's own key, for exactly the budget.
	var open settlementmsgs.MsgOpenSettlement
	require.NoError(t, proto.Unmarshal(
		capture.soleMsg(t, "/dydxprotocol.settlement.MsgOpenSettlement"), &open))
	require.Equal(t, life.Owner(), open.Opener)
	require.Equal(t, "uusdc", open.Cap.Denom)
	require.Equal(t, "500000", open.Cap.Amount.String())
	require.Contains(t, open.Memo, "svpchain-execution")

	// The credential is bound: settlement caveat, service budget, recording
	// grant on subaccount 0.
	exec.mu.Lock()
	deleg := exec.metadata[svpa2a.DelegationMetadataKey].(map[string]any)
	exec.mu.Unlock()
	raw, err := base64.StdEncoding.DecodeString(deleg["tokens"].([]any)[0].(string))
	require.NoError(t, err)

	resolver := svpdt.SingleKeyResolver(map[string][]byte{
		life.OwnerDID(): life.Priv.PubKey().Bytes(),
	})
	verified, err := svpdt.VerifyChain([][]byte{raw}, resolver, svpdt.VerifyOpts{
		ChainID:  testChainID,
		Now:      testNow,
		MaxDepth: 4,
		Audience: remoteDID,
	})
	require.NoError(t, err)
	require.Equal(t, stubSettlementIDHex(), verified.Effective.Settlement)
	require.True(t, verified.Effective.Actions.Has("settlement.record_spend"))
	require.True(t, verified.Effective.Subaccounts.Has(0))
	require.Equal(t, "500000", verified.Effective.SvcBudget[0].Amount.String())
}

// The failure the pre-check exists for: a root delegation that cannot pay
// agents refuses the task before the user is asked and before any escrow
// opens — not after the work is done.
func TestDelegateTaskWithServicePaymentNeedsARootThatCanPay(t *testing.T) {
	svc, exec, _, capture := stubStack(t)

	// The stub serves a second root (0x22…) that exists but grants neither the
	// settlement action nor a service allowance.
	args := serviceTaskArgs()
	args["root_id"] = strings.Repeat("22", 32)
	_, err := svc.Call(context.Background(), "delegate_task", args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "settlement.record_spend")

	exec.mu.Lock()
	defer exec.mu.Unlock()
	require.Empty(t, exec.received, "nothing may reach the remote agent")
	capture.mu.Lock()
	defer capture.mu.Unlock()
	require.Empty(t, capture.bodies, "no escrow may open for a refused task")
}

// Settling and refunding broadcast the user's own settlement messages for the
// named order.
func TestSettleAndRefundBroadcastForTheNamedOrder(t *testing.T) {
	t.Run("settle", func(t *testing.T) {
		svc, _, life, capture := stubStack(t)
		out, err := svc.Call(context.Background(), "settle_settlement",
			map[string]any{"settlement_id": stubSettlementIDHex()})
		require.NoError(t, err)
		require.Contains(t, out, "settled")

		var msg settlementmsgs.MsgSettle
		require.NoError(t, proto.Unmarshal(
			capture.soleMsg(t, "/dydxprotocol.settlement.MsgSettle"), &msg))
		require.Equal(t, life.Owner(), msg.Opener)
		require.Equal(t, stubSettlementIDHex(), hex.EncodeToString(msg.SettlementId))
	})

	t.Run("refund", func(t *testing.T) {
		svc, _, life, capture := stubStack(t)
		out, err := svc.Call(context.Background(), "refund_settlement",
			map[string]any{"settlement_id": stubSettlementIDHex()})
		require.NoError(t, err)
		require.Contains(t, out, "refunded")

		var msg settlementmsgs.MsgRefundSettlement
		require.NoError(t, proto.Unmarshal(
			capture.soleMsg(t, "/dydxprotocol.settlement.MsgRefundSettlement"), &msg))
		require.Equal(t, life.Owner(), msg.Opener)
		require.Empty(t, msg.SlashAgentId, "the wallet never attributes a slash")
	})
}
