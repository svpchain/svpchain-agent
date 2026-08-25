package discovery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/svpchain/svpchain-agent/internal/agentmarket"
)

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

// The market service is the only source now: what it reports about an agent is
// what the assistant sees, endpoint included.
func TestSearchAgentsReportsMarketRecords(t *testing.T) {
	market := marketStub(t, `{"agents":[{"agent_id":"did:svp:svp1real","similarity":0.71,`+
		`"endpoint":"https://real.example","capabilities":["trading"],"status":"AGENT_STATUS_ACTIVE",`+
		`"pricing":{"per_call":[{"denom":"asvp","amount":"100"}],"unit":"call"},`+
		`"bond":{"denom":"asvp","amount":"5000"}}]}`)
	defer market.Close()

	svc := &Service{Market: agentmarket.New(market.URL)}
	out, err := svc.Call(context.Background(), "search_agents", map[string]any{"query": "funding rates"})
	if err != nil {
		t.Fatalf("search_agents: %v", err)
	}

	var decoded struct {
		Agents []struct {
			AgentID      string   `json:"agent_id"`
			Endpoint     string   `json:"endpoint"`
			Capabilities []string `json:"capabilities"`
			Similarity   float64  `json:"similarity"`
			Pricing      struct {
				Unit string `json:"unit"`
			} `json:"pricing"`
			Bond struct {
				Amount string `json:"amount"`
			} `json:"bond"`
		} `json:"agents"`
		Count     int    `json:"count"`
		MarketURL string `json:"agent_market_url"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Count != 1 {
		t.Fatalf("count = %d, want 1: %s", decoded.Count, out)
	}
	got := decoded.Agents[0]
	if got.AgentID != "did:svp:svp1real" || got.Endpoint != "https://real.example" {
		t.Fatalf("unexpected agent: %+v", got)
	}
	if len(got.Capabilities) != 1 || got.Capabilities[0] != "trading" {
		t.Fatalf("capabilities = %v", got.Capabilities)
	}
	if got.Similarity != 0.71 {
		t.Fatalf("similarity = %v, want 0.71", got.Similarity)
	}
	if got.Pricing.Unit != "call" || got.Bond.Amount != "5000" {
		t.Fatalf("pricing/bond dropped: %+v", got)
	}
	// The endpoint the assistant would message must be traceable to its source.
	if decoded.MarketURL != market.URL {
		t.Fatalf("agent_market_url = %q, want %q", decoded.MarketURL, market.URL)
	}
}

// A hit with no agent id is unusable; it must be dropped rather than shown as
// a nameless row.
func TestSearchAgentsDropsIdlessHits(t *testing.T) {
	market := marketStub(t, `{"agents":[{"agent_id":"  ","similarity":0.9},{"agent_id":"did:svp:svp1real","similarity":0.6}]}`)
	defer market.Close()

	svc := &Service{Market: agentmarket.New(market.URL)}
	out, err := svc.Call(context.Background(), "search_agents", map[string]any{"query": "anything"})
	if err != nil {
		t.Fatalf("search_agents: %v", err)
	}
	var decoded struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Count != 1 {
		t.Fatalf("count = %d, want 1: %s", decoded.Count, out)
	}
}

// With no market configured the tool must be absent and refuse clearly, rather
// than erroring in a way that reads as "no agent can do this".
func TestSearchAgentsUnconfigured(t *testing.T) {
	svc := &Service{}

	if defs := svc.ToolDefs(); len(defs) != 0 {
		t.Fatalf("tools offered without a configured market service: %+v", defs)
	}
	_, err := svc.Call(context.Background(), "search_agents", map[string]any{"query": "x"})
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("err = %v, want a 'not configured' refusal", err)
	}
}

func TestSearchAgentsRequiresQuery(t *testing.T) {
	market := marketStub(t, `{"agents":[]}`)
	defer market.Close()

	svc := &Service{Market: agentmarket.New(market.URL)}
	if _, err := svc.Call(context.Background(), "search_agents", map[string]any{"query": "  "}); err == nil {
		t.Fatal("expected an error for a blank query")
	}
}

// The tool must only ask for ACTIVE agents, and must pass the caller's
// capability filter through.
func TestSearchAgentsQueryParameters(t *testing.T) {
	var got string
	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"agents":[]}`))
	}))
	defer market.Close()

	svc := &Service{Market: agentmarket.New(market.URL)}
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
