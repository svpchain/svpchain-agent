package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/agent/llm"
	agentsettlement "github.com/svpchain/svpchain-agent/internal/agent/settlement"
	"github.com/svpchain/svpchain-agent/internal/agentmarket"
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

func carriedTask() agentsettlement.ActiveTask {
	return agentsettlement.ActiveTask{
		Endpoint: "https://agent.example",
		AgentID:  "did:svp:abc:1",
		TaskID:   "0x" + strings.Repeat("ab", 32),
		IntentID: "0x" + strings.Repeat("cd", 32),
		Owner:    "0x516c9637B4b26F1f62f553145A9A86F01E890f60",
		Amount:   "1000000",
	}
}

// A task funded in an earlier turn and never reported is the same behavior
// still running. Answering the assistant's clarifying question must not deposit
// a second escrow for it.
func TestAdoptsUnreportedTaskForTheSameAgent(t *testing.T) {
	task := carriedTask()
	flow := &paidAgentFlow{resume: &task}
	adopted := flow.adoptableLocked("https://agent.example/")
	require.NotNil(t, adopted)
	require.Equal(t, task.TaskID, adopted.TaskID)
}

func TestFundsWhenTheCarriedTaskBelongsToAnotherAgent(t *testing.T) {
	task := carriedTask()
	flow := &paidAgentFlow{resume: &task}
	require.Nil(t, flow.adoptableLocked("https://other.example"))
	require.Nil(t, (&paidAgentFlow{}).adoptableLocked("https://agent.example"))
}

// Reuse must never be reached through a record the reporter cannot close out:
// the execution would be unreportable and the escrow stuck.
func TestFundsWhenTheCarriedTaskIsIncomplete(t *testing.T) {
	for name, task := range map[string]agentsettlement.ActiveTask{
		"no task id": {Endpoint: "https://agent.example", Owner: "0xabc"},
		"no owner":   {Endpoint: "https://agent.example", TaskID: "0x" + strings.Repeat("ab", 32)},
		"bad hash":   {Endpoint: "https://agent.example", TaskID: "0xnothex", Owner: "0xabc"},
	} {
		t.Run(name, func(t *testing.T) {
			carried := task
			require.Nil(t, (&paidAgentFlow{resume: &carried}).adoptableLocked("https://agent.example"))
		})
	}
}

// Reuse activates the task for this run — reporter, attach, and the record the
// next turn reads — and moves no funds.
func TestReuseActivatesTheTaskWithoutDepositing(t *testing.T) {
	task := carriedTask()
	var recorded []agentsettlement.ActiveTask
	flow := &paidAgentFlow{
		validatorURL: "https://validator.example",
		reporter:     agentsettlement.NewDeferred(),
		resume:       &task,
		onFunded:     func(t agentsettlement.ActiveTask) { recorded = append(recorded, t) },
	}

	var attached []string
	out, err := flow.reuseLocked(context.Background(), task,
		agentmarket.Hit{Endpoint: "https://agent.example", Owner: "svp1owner"},
		func(_ context.Context, endpoint string) (string, error) {
			attached = append(attached, endpoint)
			return `{"tools_available":["build_example"]}`, nil
		})
	require.NoError(t, err)
	require.Equal(t, []string{"https://agent.example"}, attached)
	require.True(t, flow.started, "the run now owns the task and must not fund another")
	require.Equal(t, []agentsettlement.ActiveTask{task}, recorded)

	var report map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &report))
	require.Equal(t, true, report["reused_existing_task"])
	require.Equal(t, task.TaskID, report["task_id"])
	require.Equal(t, []any{"build_example"}, report["tools_available"])
	require.NotContains(t, report, "deposit_tx_hash", "reuse deposits nothing")

	// The reporter is bound to the adopted task, so this behavior's execution
	// closes out the escrow the user already paid.
	require.ErrorContains(t,
		flow.reporter.Configure(agentsettlement.Config{
			TaskID: task.TaskID, Owner: task.Owner, ValidatorURL: flow.validatorURL,
		}),
		"already configured")
}

// The whole point, at the level the tool loop actually enters: begin_agent_settlement
// for the agent this conversation already paid attaches it and deposits nothing.
// The validator client is nil on purpose — funding a second task must call it,
// so a fall-through fails loudly here instead of quietly charging the user.
func TestStartAdoptsTheConversationTaskInsteadOfDepositing(t *testing.T) {
	task := carriedTask()
	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/agents/"+url.PathEscape(task.AgentID), r.URL.Path)
		_ = json.NewEncoder(w).Encode(agentmarket.Hit{
			AgentID: task.AgentID, Owner: "svp1owner", Endpoint: task.Endpoint,
			Status: "AGENT_STATUS_ACTIVE", Pricing: agentmarket.Pricing{Amount: "1000000"},
		})
	}))
	defer market.Close()

	flow := &paidAgentFlow{
		validatorURL: "https://validator.example",
		market:       agentmarket.New(market.URL),
		reporter:     agentsettlement.NewDeferred(),
		resume:       &task,
	}
	out, err := flow.Start(context.Background(), map[string]any{"agent_id": task.AgentID},
		func(context.Context, string) (string, error) { return `{"tools_available":["build_example"]}`, nil })
	require.NoError(t, err)

	var report map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &report))
	require.Equal(t, true, report["reused_existing_task"])
	require.Equal(t, task.TaskID, report["task_id"])
}

// And the guard is real: a different agent is funded normally, reaching the
// validator this flow deliberately does not have.
func TestStartFundsAnAgentTheConversationHasNotPaid(t *testing.T) {
	task := carriedTask()
	other := "did:svp:other:1"
	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(agentmarket.Hit{
			AgentID: other, Owner: "svp1owner", Endpoint: "https://other.example",
			Status: "AGENT_STATUS_ACTIVE", Pricing: agentmarket.Pricing{Amount: "1000000"},
		})
	}))
	defer market.Close()

	flow := &paidAgentFlow{
		validatorURL: "https://validator.example",
		market:       agentmarket.New(market.URL),
		reporter:     agentsettlement.NewDeferred(),
		resume:       &task,
	}
	_, err := flow.Start(context.Background(), map[string]any{"agent_id": other},
		func(context.Context, string) (string, error) {
			t.Fatal("an unfunded agent must not be attached")
			return "", nil
		})
	require.ErrorContains(t, err, "settlement validator client is nil")
}
