package agent

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/agent/llm"
)

type fakeSource struct {
	tools  []string
	called []string
}

func (f *fakeSource) URL() string { return "https://agent.example" }

func (f *fakeSource) Tools() []llm.Tool {
	out := make([]llm.Tool, 0, len(f.tools))
	for _, n := range f.tools {
		out = append(out, llm.Tool{Type: "function", Function: llm.Function{Name: n}})
	}
	return out
}

func (f *fakeSource) Handles(name string) bool {
	for _, n := range f.tools {
		if n == name {
			return true
		}
	}
	return false
}

func (f *fakeSource) CallTool(_ context.Context, name string, _ map[string]any) (string, error) {
	f.called = append(f.called, name)
	return fmt.Sprintf(`{"tool":%q}`, name), nil
}

func toolList(names ...string) []llm.Tool {
	out := make([]llm.Tool, 0, len(names))
	for _, n := range names {
		out = append(out, llm.Tool{Type: "function", Function: llm.Function{Name: n}})
	}
	return out
}

// A market-discovered agent must not be able to displace the local signer or
// the remote MCP by advertising their tool names.
func TestAttachedDropsNamesAlreadyServed(t *testing.T) {
	att := newAttached(toolList("sign_evm_transaction", "build_bank_send", "search_agents"))
	src := &fakeSource{tools: []string{"build_swap", "build_bank_send", "sign_evm_transaction"}}

	skipped := att.skipped(src)
	added := att.set(src)

	require.Equal(t, []string{"build_swap"}, added)
	require.ElementsMatch(t, []string{"build_bank_send", "sign_evm_transaction"}, skipped)
	require.Len(t, att.toolDefs(), 1)
}

func TestAttachedCannotClaimTheConnectToolItself(t *testing.T) {
	att := newAttached(nil)
	require.Empty(t, att.set(&fakeSource{tools: []string{ConnectTool}}))
}

// Precedence must hold at dispatch too, not just in the advertised list.
func TestAttachedHandlerNeverTakesALocalOrRemoteTool(t *testing.T) {
	att := newAttached(toolList("sign_evm_transaction", "build_bank_send"))
	att.set(&fakeSource{tools: []string{"build_swap", "build_bank_send", "sign_evm_transaction"}})
	env := dispatchEnv{att: att}

	require.Equal(t, "localHandler", firstHandlerName(env, "sign_evm_transaction"))
	require.Equal(t, "remoteHandler", firstHandlerName(env, "build_bank_send"))
	require.Equal(t, "attachedHandler", firstHandlerName(env, "build_swap"))
}

func TestAttachedHandlerRoutesToTheAgent(t *testing.T) {
	att := newAttached(nil)
	src := &fakeSource{tools: []string{"build_swap"}}
	att.set(src)

	out, err := dispatchEnv{att: att}.dispatch(context.Background(), "build_swap", map[string]any{})
	require.NoError(t, err)
	require.JSONEq(t, `{"tool":"build_swap"}`, out)
	require.Equal(t, []string{"build_swap"}, src.called)
}

// With nothing attached, an A2A tool name is not magically routable.
func TestNothingAttachedFallsThroughToRemote(t *testing.T) {
	env := dispatchEnv{att: newAttached(nil)}
	require.False(t, env.att.handles("build_swap"))
	require.Equal(t, "remoteHandler", firstHandlerName(env, "build_swap"))
}

func TestConnectRequiresAnAgentURL(t *testing.T) {
	_, err := dispatchEnv{att: newAttached(nil)}.connect(context.Background(), map[string]any{})
	require.ErrorContains(t, err, "agent_url is required")
}

// a2a_connect_agent is offered only alongside search_agents: attaching an agent
// you cannot discover is not a flow the assistant should be shown.
func TestConnectToolRidesWithAgentSearch(t *testing.T) {
	require.Equal(t, ConnectTool, ConnectToolDef().Function.Name)
}

// A successful attach reports the endpoint so the caller can persist it and
// re-attach on the next user message; the fake source stands in for the live
// client, so only the bookkeeping around set() is exercised here.
func TestAttachedReportsEndpointOnAttach(t *testing.T) {
	var got []string
	att := newAttached(nil)
	att.onAttach = func(url string) { got = append(got, url) }
	att.noteReattachFailure("https://agent.example", fmt.Errorf("connection refused"))

	url, lostErr := att.lost()
	require.Equal(t, "https://agent.example", url)
	require.ErrorContains(t, lostErr, "connection refused")

	// Mirrors the tail of connect(): a fresh attach clears the lost marker.
	att.set(&fakeSource{tools: []string{"build_swap"}})
	att.mu.Lock()
	att.lostURL, att.lostErr = "", nil
	att.mu.Unlock()
	url, lostErr = att.lost()
	require.Empty(t, url)
	require.NoError(t, lostErr)
	require.Empty(t, got, "set() alone does not report; connect() does")
}

// When the agent attached last turn cannot be re-attached and the remote MCP
// is off, a call to one of its tools must point the model at reconnecting,
// not at the remote-MCP setting — which would not bring these tools back.
func TestLostAgentToolsExplainHowToReconnect(t *testing.T) {
	att := newAttached(nil)
	att.noteReattachFailure("https://agent.example", fmt.Errorf("connection refused"))
	env := dispatchEnv{att: att}

	_, err := env.dispatch(context.Background(), "build_swap", map[string]any{})
	require.ErrorContains(t, err, "https://agent.example")
	require.ErrorContains(t, err, "connection refused")
	require.ErrorContains(t, err, ConnectTool)
	require.NotContains(t, err.Error(), "Settings")
}
