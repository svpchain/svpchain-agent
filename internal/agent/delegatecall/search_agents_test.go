package delegatecall

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/svpchain/svpchain-agent/internal/agentmarket"
	"github.com/svpchain/svpchain-agent/internal/registry"
)

// chainStub serves the x/agent REST shape for one agent id, so a search hit can
// be resolved the way the real registry client resolves it.
func chainStub(t *testing.T, agents map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for id, body := range agents {
			if strings.Contains(r.URL.Path, id) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":5,"message":"not found"}`))
	}))
}

func agentJSON(id, endpoint string) string {
	return `{"agent":{"agent_id":"` + id + `","owner":"svp1owner","operator":"svp1op","endpoint":"` + endpoint +
		`","capability_hash":"","capabilities":["trading"],"status":"AGENT_STATUS_ACTIVE","metadata":"","pricing":{},"bond":{}}}`
}

func marketStub(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/agents/search" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
}

// The security property this whole design exists for: the market service ranks,
// the chain decides. A market response claiming a different endpoint must not
// change the endpoint the assistant sees, because delegate_task would send a
// spending credential there.
func TestSearchAgentsIgnoresMarketSuppliedEndpoint(t *testing.T) {
	const id = "did:svp:svp1real"
	chain := chainStub(t, map[string]string{id: agentJSON(id, "https://real.example")})
	defer chain.Close()
	market := marketStub(t, `{"agents":[{"agent_id":"`+id+`","similarity":0.71,"endpoint":"https://attacker.example","capabilities":["everything"],"status":"AGENT_STATUS_ACTIVE"}]}`)
	defer market.Close()

	svc := &Service{Registry: registry.New(chain.URL), Market: agentmarket.New(market.URL)}
	out, err := svc.Call(context.Background(), "search_agents", map[string]any{"query": "funding rates"})
	if err != nil {
		t.Fatalf("search_agents: %v", err)
	}
	if strings.Contains(out, "attacker.example") {
		t.Fatalf("market-supplied endpoint leaked into the result: %s", out)
	}
	if !strings.Contains(out, "real.example") {
		t.Fatalf("chain endpoint missing from result: %s", out)
	}
	if strings.Contains(out, "everything") {
		t.Fatalf("market-supplied capabilities leaked into the result: %s", out)
	}

	var decoded struct {
		Agents []struct {
			AgentID      string   `json:"agent_id"`
			Endpoint     string   `json:"endpoint"`
			Capabilities []string `json:"capabilities"`
			Similarity   float64  `json:"similarity"`
		} `json:"agents"`
		Count int `json:"count"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Count != 1 || decoded.Agents[0].Endpoint != "https://real.example" {
		t.Fatalf("expected the chain's endpoint, got %+v", decoded.Agents)
	}
	// The ranking is the one thing the market is allowed to contribute.
	if decoded.Agents[0].Similarity != 0.71 {
		t.Fatalf("similarity = %v, want 0.71", decoded.Agents[0].Similarity)
	}
}

// An agent the market knows about but the chain does not must be dropped, not
// shown, and the discrepancy surfaced.
func TestSearchAgentsDropsHitsMissingFromChain(t *testing.T) {
	const real, ghost = "did:svp:svp1real", "did:svp:svp1ghost"
	chain := chainStub(t, map[string]string{real: agentJSON(real, "https://real.example")})
	defer chain.Close()
	market := marketStub(t, `{"agents":[{"agent_id":"`+ghost+`","similarity":0.9},{"agent_id":"`+real+`","similarity":0.6}]}`)
	defer market.Close()

	svc := &Service{Registry: registry.New(chain.URL), Market: agentmarket.New(market.URL)}
	out, err := svc.Call(context.Background(), "search_agents", map[string]any{"query": "anything"})
	if err != nil {
		t.Fatalf("search_agents: %v", err)
	}
	var decoded struct {
		Count      int      `json:"count"`
		Unresolved []string `json:"unresolved_on_chain"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Count != 1 {
		t.Fatalf("count = %d, want 1 (the ghost agent must be dropped): %s", decoded.Count, out)
	}
	if len(decoded.Unresolved) != 1 || decoded.Unresolved[0] != ghost {
		t.Fatalf("unresolved_on_chain = %v, want [%s]", decoded.Unresolved, ghost)
	}
}

// With no market configured the tool must be absent and refuse clearly, rather
// than erroring in a way that reads as "no agent can do this".
func TestSearchAgentsUnconfigured(t *testing.T) {
	chain := chainStub(t, nil)
	defer chain.Close()
	svc := &Service{Registry: registry.New(chain.URL)}

	for _, def := range svc.ToolDefs() {
		if def.Function.Name == "search_agents" {
			t.Fatal("search_agents offered without a configured market service")
		}
	}
	_, err := svc.Call(context.Background(), "search_agents", map[string]any{"query": "x"})
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("err = %v, want a 'not configured' refusal", err)
	}
}

func TestSearchAgentsRequiresQuery(t *testing.T) {
	chain := chainStub(t, nil)
	defer chain.Close()
	market := marketStub(t, `{"agents":[]}`)
	defer market.Close()

	svc := &Service{Registry: registry.New(chain.URL), Market: agentmarket.New(market.URL)}
	if _, err := svc.Call(context.Background(), "search_agents", map[string]any{"query": "  "}); err == nil {
		t.Fatal("expected an error for a blank query")
	}
}

// The tool must only offer ACTIVE agents work, and must pass the caller's
// capability filter through.
func TestSearchAgentsQueryParameters(t *testing.T) {
	var got string
	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"agents":[]}`))
	}))
	defer market.Close()
	chain := chainStub(t, nil)
	defer chain.Close()

	svc := &Service{Registry: registry.New(chain.URL), Market: agentmarket.New(market.URL)}
	if _, err := svc.Call(context.Background(), "search_agents",
		map[string]any{"query": "funding rates", "capability": "trading", "limit": float64(3)}); err != nil {
		t.Fatalf("search_agents: %v", err)
	}
	for _, want := range []string{"q=funding+rates", "capability=trading", "limit=3", "status=AGENT_STATUS_ACTIVE"} {
		if !strings.Contains(got, want) {
			t.Fatalf("query %q missing %q", got, want)
		}
	}
}
