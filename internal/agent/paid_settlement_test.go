package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/agent/llm"
)

func TestConnectEndpointReusesActiveTaskForSameAgent(t *testing.T) {
	flow := &paidAgentFlow{started: true, endpoint: "https://agent.example"}
	var attached []string
	out, err := flow.ConnectEndpoint(context.Background(), "https://agent.example/", func(_ context.Context, endpoint string) (string, error) {
		attached = append(attached, endpoint)
		return `{"attached":true}`, nil
	})
	require.NoError(t, err)
	require.JSONEq(t, `{"attached":true}`, out)
	require.Equal(t, []string{"https://agent.example"}, attached)
}

func TestConnectEndpointRejectsDifferentAgentAfterFunding(t *testing.T) {
	flow := &paidAgentFlow{started: true, endpoint: "https://agent.example"}
	_, err := flow.ConnectEndpoint(context.Background(), "https://other.example", func(context.Context, string) (string, error) {
		t.Fatal("different endpoint must not attach")
		return "", nil
	})
	require.ErrorContains(t, err, "cannot connect a different endpoint")
}

func TestSettlementResultCarriesAttachedToolList(t *testing.T) {
	out, err := settlementResult(map[string]any{
		"agent_id": "did:svp:abc:1", "endpoint": "https://agent.example", "deposit_tx_hash": "0xdead",
	}, `{"agent_url":"https://agent.example","tools_available":["build_place_market_order"],"tools_skipped":["whoami"]}`)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"agent_id": "did:svp:abc:1",
		"endpoint": "https://agent.example",
		"deposit_tx_hash": "0xdead",
		"agent_url": "https://agent.example",
		"tools_available": ["build_place_market_order"],
		"tools_skipped": ["whoami"]
	}`, out)
}

func TestSettlementResultKeepsSettlementFieldsOverAttachReport(t *testing.T) {
	out, err := settlementResult(map[string]any{"owner": "0xowner", "amount": "100"},
		`{"owner":"0xspoofed","tools_available":["build_swap"]}`)
	require.NoError(t, err)
	require.JSONEq(t, `{"owner":"0xowner","amount":"100","tools_available":["build_swap"]}`, out)
}

func TestSettlementResultCarriesNonJSONAttachOutputVerbatim(t *testing.T) {
	out, err := settlementResult(map[string]any{"task_id": "7"}, "attached, no report")
	require.NoError(t, err)
	require.JSONEq(t, `{"task_id":"7","attach_result":"attached, no report"}`, out)
}

func TestNotePaidConnectMarksOnlyTheConnectTool(t *testing.T) {
	tools := []llm.Tool{
		{Function: llm.Function{Name: "search_agents", Description: "find agents"}},
		ConnectToolDef(),
	}
	notePaidConnect(tools)
	require.Equal(t, "find agents", tools[0].Function.Description)
	require.Contains(t, tools[1].Function.Description, "connecting first pays its advertised price")
	require.NotContains(t, ConnectToolDef().Function.Description, "advertised price")
}
