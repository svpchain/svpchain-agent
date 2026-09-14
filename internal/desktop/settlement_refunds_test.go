package desktop

import (
	"testing"

	"github.com/stretchr/testify/require"

	agentsettlement "github.com/svpchain/svpchain-agent/internal/agent/settlement"
)

func TestSettlementRefundRowsWithholdsRefundWhileTaskActive(t *testing.T) {
	intent := agentsettlement.Intent{ID: "0xintent", Payer: "0xpayer", Available: "1000000"}
	token := agentsettlement.PaymentToken{Symbol: "USDC", Decimals: 6}

	active := settlementRefundRows("svp-2517-1", intent, []agentsettlement.OnChainTask{{ID: "0xtask", IntentID: "0xcanonical-intent", Amount: "1000000", Status: agentsettlement.TaskAssigned}}, token)
	require.Len(t, active, 1)
	require.False(t, active[0].Refundable)
	require.True(t, active[0].Cancellable)
	require.Equal(t, "0xcanonical-intent", active[0].IntentID)
	require.Equal(t, "0xcanonical-intent", active[0].DisplayIntentID)
	require.Equal(t, "0xtask", active[0].TaskID)
	require.Equal(t, "1 USDC", active[0].Available)

	failed := settlementRefundRows("svp-2517-1", intent, []agentsettlement.OnChainTask{{ID: "0xtask", IntentID: "0xcanonical-intent", Amount: "1000000", Status: agentsettlement.TaskFailed}}, token)
	require.True(t, failed[0].Refundable)
	require.False(t, failed[0].Cancellable)

	unassigned := settlementRefundRows("svp-2517-1", intent, nil, token)
	require.Equal(t, intent.ID, unassigned[0].TaskID)
}

func TestSettlementDisplayStatusUsesValidatorLifecycle(t *testing.T) {
	require.Equal(t, "submitted", settlementDisplayStatus("received", "", "assigned"))
	require.Equal(t, "retrying", settlementDisplayStatus("received", "bindExecution reverted", "assigned"))
	require.Equal(t, "validating", settlementDisplayStatus("pending", "", "bound"))
	require.Equal(t, "success", settlementDisplayStatus("succeeded", "", "assigned"))
	require.Equal(t, "failed", settlementDisplayStatus("failed", "", "assigned"))
}

func TestNormalizeSettlementPageBoundsSize(t *testing.T) {
	page, size := normalizeSettlementPage(0, 0)
	require.Equal(t, 1, page)
	require.Equal(t, defaultSettlementPageSize, size)

	page, size = normalizeSettlementPage(3, maxSettlementPageSize+1)
	require.Equal(t, 3, page)
	require.Equal(t, maxSettlementPageSize, size)
}
