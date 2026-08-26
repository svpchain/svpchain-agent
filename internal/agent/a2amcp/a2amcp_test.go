package a2amcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	svpa2a "github.com/svpchain/svpchain-agent/internal/a2a"
)

// fakeAgent scripts replies by tool name and records what was sent.
type fakeAgent struct {
	replies   map[string]string // tool → reply JSON text
	sent      []request
	contexts  []string
	contextID string
}

func (f *fakeAgent) install(t *testing.T) {
	t.Helper()
	orig := sendText
	t.Cleanup(func() { sendText = orig })
	sendText = func(_ context.Context, _, contextID, text string) (svpa2a.SendResult, error) {
		var req request
		if err := json.Unmarshal([]byte(text), &req); err != nil {
			return svpa2a.SendResult{}, err
		}
		f.sent = append(f.sent, req)
		f.contexts = append(f.contexts, contextID)
		reply, ok := f.replies[req.Tool]
		if !ok {
			return svpa2a.SendResult{}, fmt.Errorf("fake agent has no reply for %q", req.Tool)
		}
		return svpa2a.SendResult{Response: reply, ContextID: f.contextID}, nil
	}
}

func listing(tools ...string) string {
	var parts []string
	for _, name := range tools {
		parts = append(parts, fmt.Sprintf(
			`{"skill":"svpchain-evm","tool":%q,"input_schema":{"type":"object","properties":{"token_in":{"type":"string","description":"input token"}}}}`,
			name))
	}
	return fmt.Sprintf(`{"skill":"svpchain-meta","tool":"list_tools","ok":true,"result":{"tools":[%s]}}`,
		strings.Join(parts, ","))
}

func connected(t *testing.T, agent *fakeAgent) *Client {
	t.Helper()
	agent.install(t)
	c := New("https://agent.example")
	require.NoError(t, c.Connect(context.Background()))
	return c
}

func TestConnectDiscoversToolsAndSchemas(t *testing.T) {
	agent := &fakeAgent{replies: map[string]string{"list_tools": listing("build_swap", "quote_swap")}}
	c := connected(t, agent)

	require.Equal(t, metaSkill, agent.sent[0].Skill)
	require.Equal(t, listToolsTool, agent.sent[0].Tool)

	tools := c.Tools()
	require.Len(t, tools, 2)
	require.Equal(t, "build_swap", tools[0].Function.Name)
	// The tool keeps its MCP name — that is what lets guard and writepath see it.
	require.True(t, c.Handles("build_swap"))
	require.False(t, c.Handles("build_bank_send"))

	params, ok := tools[0].Function.Parameters.(map[string]any)
	require.True(t, ok, "the agent's own schema must reach the model")
	props, ok := params["properties"].(map[string]any)
	require.True(t, ok)
	require.Contains(t, props, "token_in")
}

// A remote agent must never supply a signing tool: those names belong to the
// local signer.
func TestConnectRefusesSigningTools(t *testing.T) {
	agent := &fakeAgent{replies: map[string]string{
		"list_tools": listing("build_swap", "sign_evm_transaction", "sign_challenge"),
	}}
	c := connected(t, agent)

	require.False(t, c.Handles("sign_evm_transaction"))
	require.False(t, c.Handles("sign_challenge"))
	require.True(t, c.Handles("build_swap"))
	require.Len(t, c.Tools(), 1)
}

func TestConnectFailsWhenTheAgentDoesNotListTools(t *testing.T) {
	agent := &fakeAgent{replies: map[string]string{}}
	agent.install(t)
	err := New("https://agent.example").Connect(context.Background())
	require.ErrorContains(t, err, "does not serve svpchain-meta/list_tools")
}

func TestCallToolSendsTheEnvelopeAndReturnsTheResult(t *testing.T) {
	agent := &fakeAgent{replies: map[string]string{
		"list_tools": listing("build_swap"),
		"build_swap": `{"skill":"svpchain-evm","tool":"build_swap","ok":true,"result":{"payload":{"evm_chain_id":"2517"}}}`,
	}}
	c := connected(t, agent)

	out, err := c.CallTool(context.Background(), "build_swap", map[string]any{"token_in": "svp"})
	require.NoError(t, err)
	require.JSONEq(t, `{"payload":{"evm_chain_id":"2517"}}`, out)

	sent := agent.sent[len(agent.sent)-1]
	require.Equal(t, "svpchain-evm", sent.Skill)
	require.Equal(t, "build_swap", sent.Tool)
	require.Equal(t, map[string]any{"token_in": "svp"}, sent.Args)
}

// ok:false is the agent answering "no". It must reach the model as a tool
// error carrying the agent's own words, not as a success.
func TestCallToolSurfacesRefusal(t *testing.T) {
	agent := &fakeAgent{replies: map[string]string{
		"list_tools": listing("build_swap"),
		"build_swap": `{"skill":"svpchain-evm","tool":"build_swap","ok":false,"error":"authentication required: this tool needs a bearer token"}`,
	}}
	c := connected(t, agent)

	_, err := c.CallTool(context.Background(), "build_swap", nil)
	require.ErrorContains(t, err, "authentication required")
}

func TestCallToolRejectsNonEnvelopeReply(t *testing.T) {
	agent := &fakeAgent{replies: map[string]string{
		"list_tools": listing("build_swap"),
		"build_swap": `error: request must be JSON naming a skill`,
	}}
	c := connected(t, agent)

	_, err := c.CallTool(context.Background(), "build_swap", nil)
	require.ErrorContains(t, err, "did not answer build_swap in its envelope")
	require.ErrorContains(t, err, "must be JSON naming a skill")
}

func TestCallToolRefusesUnknownTool(t *testing.T) {
	agent := &fakeAgent{replies: map[string]string{"list_tools": listing("build_swap")}}
	c := connected(t, agent)

	_, err := c.CallTool(context.Background(), "build_bank_send", nil)
	require.ErrorContains(t, err, `does not serve "build_bank_send"`)
}

// The bearer binds to the A2A context, so every call after the first must
// carry the id the agent assigned — losing it silently de-authenticates.
func TestClientPinsTheAgentAssignedContext(t *testing.T) {
	agent := &fakeAgent{
		contextID: "ctx-42",
		replies: map[string]string{
			"list_tools": listing("build_swap"),
			"build_swap": `{"skill":"svpchain-evm","tool":"build_swap","ok":true,"result":{}}`,
		},
	}
	c := connected(t, agent)
	_, err := c.CallTool(context.Background(), "build_swap", nil)
	require.NoError(t, err)

	require.Equal(t, "", agent.contexts[0], "the first call opens a new context")
	require.Equal(t, "ctx-42", agent.contexts[1], "later calls must stay on it")
}

func TestEnsureAuthRunsTheHandshakeAndCarriesTheBearer(t *testing.T) {
	agent := &fakeAgent{
		contextID: "ctx-7",
		replies: map[string]string{
			"list_tools":     listing("build_swap", "auth_challenge", "auth_verify"),
			"auth_challenge": `{"skill":"svpchain-auth","tool":"auth_challenge","ok":true,"result":{"challenge":"svpchain-mcp-auth-v1:svp-2517-1:abc:99","nonce":"abc","expires_at":4000000000}}`,
			"auth_verify":    `{"skill":"svpchain-auth","tool":"auth_verify","ok":true,"result":{"bearer_token":"tok-1","owner":"svp1abc","expires_at":4000000000}}`,
			"build_swap":     `{"skill":"svpchain-evm","tool":"build_swap","ok":true,"result":{}}`,
		},
	}
	c := connected(t, agent)

	var signed string
	err := c.EnsureAuth(context.Background(), "svp1abc", func(ch string) (string, error) {
		signed = ch
		return "sig-b64", nil
	})
	require.NoError(t, err)
	require.True(t, c.BearerValid())
	// The signer only accepts this prefix; the handshake must pass it through
	// untouched rather than reshaping it.
	require.True(t, strings.HasPrefix(signed, "svpchain-mcp-auth-v1:"))

	_, err = c.CallTool(context.Background(), "build_swap", nil)
	require.NoError(t, err)
	require.Equal(t, "tok-1", agent.sent[len(agent.sent)-1].Bearer)
}

func TestEnsureAuthNeedsALocalSigner(t *testing.T) {
	agent := &fakeAgent{replies: map[string]string{"list_tools": listing("auth_challenge", "auth_verify")}}
	c := connected(t, agent)

	require.ErrorContains(t, c.EnsureAuth(context.Background(), "svp1abc", nil), "no local signer")
	require.ErrorContains(t, c.EnsureAuth(context.Background(), "", func(string) (string, error) {
		return "", nil
	}), "owner address is required")
}

func TestEnsureAuthFailsWhenTheAgentHasNoHandshake(t *testing.T) {
	agent := &fakeAgent{replies: map[string]string{"list_tools": listing("build_swap")}}
	c := connected(t, agent)
	err := c.EnsureAuth(context.Background(), "svp1abc", func(string) (string, error) { return "sig", nil })
	require.ErrorContains(t, err, "serves no auth handshake")
}

// svpchain-meta is the transport's control channel, not a capability. An agent
// that lists its own list_tools must not have it attached as a callable tool —
// the model would treat plumbing as a way to introspect its tool list.
func TestConnectRefusesMetaSkillTools(t *testing.T) {
	agent := &fakeAgent{replies: map[string]string{
		"list_tools": `{"skill":"svpchain-meta","tool":"list_tools","ok":true,"result":{"tools":[` +
			`{"skill":"svpchain-meta","tool":"list_tools","input_schema":{"type":"object"}},` +
			`{"skill":"svpchain-evm","tool":"build_swap","input_schema":{"type":"object"}}]}}`,
	}}
	c := connected(t, agent)

	require.False(t, c.Handles("list_tools"))
	require.True(t, c.Handles("build_swap"))
	require.Len(t, c.Tools(), 1)
}
