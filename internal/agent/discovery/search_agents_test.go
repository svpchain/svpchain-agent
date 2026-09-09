package discovery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

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
	market := marketStub(t, `{"agents":[{"agent_id":"did:svp:svp1real","owner":"svp1owner","similarity":0.71,`+
		`"endpoint":"https://real.example","capabilities":["trading"],"status":"AGENT_STATUS_ACTIVE",`+
		`"pricing":{"amount":"1000000","unit":"call"},`+
		`"bond":{"denom":"asvp","amount":"5000"}}]}`)
	defer market.Close()

	svc := &Service{Market: agentmarket.New(market.URL)}
	svc.PaymentToken = func(context.Context) (PaymentToken, error) {
		return PaymentToken{Symbol: "USDC", Decimals: 6}, nil
	}
	out, err := svc.Call(context.Background(), "search_agents", map[string]any{"query": "funding rates"})
	if err != nil {
		t.Fatalf("search_agents: %v", err)
	}

	var decoded struct {
		Agents []struct {
			AgentID      string   `json:"agent_id"`
			Owner        string   `json:"owner"`
			Endpoint     string   `json:"endpoint"`
			Capabilities []string `json:"capabilities"`
			Similarity   float64  `json:"similarity"`
			Pricing      struct {
				Amount string `json:"amount"`
				Token  string `json:"token"`
				Unit   string `json:"unit"`
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
	if got.AgentID != "did:svp:svp1real" || got.Owner != "svp1owner" || got.Endpoint != "https://real.example" {
		t.Fatalf("unexpected agent: %+v", got)
	}
	if len(got.Capabilities) != 1 || got.Capabilities[0] != "trading" {
		t.Fatalf("capabilities = %v", got.Capabilities)
	}
	if got.Similarity != 0.71 {
		t.Fatalf("similarity = %v, want 0.71", got.Similarity)
	}
	if got.Pricing.Amount != "1" || got.Pricing.Token != "USDC" || got.Pricing.Unit != "call" || got.Bond.Amount != "5000" {
		t.Fatalf("pricing/bond dropped: %+v", got)
	}
	// The endpoint the assistant would message must be traceable to its source.
	if decoded.MarketURL != market.URL {
		t.Fatalf("agent_market_url = %q, want %q", decoded.MarketURL, market.URL)
	}
	require.Equal(t, "did:svp:svp1real", svc.AgentIDForEndpoint("https://real.example"))
	require.Equal(t, "did:svp:svp1real", svc.AgentIDForEndpoint("https://real.example/"))
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

func TestSearchAgentsRequiresQueryForSearchMode(t *testing.T) {
	market := marketStub(t, `{"agents":[]}`)
	defer market.Close()

	svc := &Service{Market: agentmarket.New(market.URL)}
	if _, err := svc.Call(context.Background(), "search_agents", map[string]any{"mode": "search", "query": "  "}); err == nil {
		t.Fatal("expected an error for a blank search query")
	}
}

func TestSearchAgentsListsActiveAgentsWhenRequested(t *testing.T) {
	var gotPath, gotQuery string
	market := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		_, _ = w.Write([]byte(`{"agents":[{"agent_id":"did:svp:svp1listed","capabilities":["evm.swap"]}],"limit":2,"cursor":0,"next_cursor":1}`))
	}))
	defer market.Close()

	svc := &Service{Market: agentmarket.New(market.URL)}
	out, err := svc.Call(context.Background(), "search_agents", map[string]any{"mode": "list", "limit": float64(2)})
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	if gotPath != "/v1/agents" || strings.Contains(gotQuery, "q=") {
		t.Fatalf("list request = %s?%s, want /v1/agents with no q", gotPath, gotQuery)
	}
	for _, want := range []string{"limit=2", "cursor=0", "status=AGENT_STATUS_ACTIVE"} {
		if !strings.Contains(gotQuery, want) {
			t.Fatalf("list query %q missing %q", gotQuery, want)
		}
	}
	var decoded struct {
		Mode       string `json:"mode"`
		Cursor     int    `json:"cursor"`
		NextCursor int    `json:"next_cursor"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("unmarshal list result: %v", err)
	}
	if decoded.Mode != "list" || decoded.Cursor != 0 || decoded.NextCursor != 1 {
		t.Fatalf("unexpected pagination result: %s", out)
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

// The Agent Card, and the base64 SHA-256 of these exact bytes.
const (
	cardBody = `{"name":"Orderbook Analyst","description":"Analyzes order books and funding rates.","skills":[{"name":"depth","description":"Order book depth analysis."}]}`
	cardHash = "WflqJrx0K07EBJ2zNfRBdnR9QJjyATd2dWzY12niCj8="
)

func marketBody(t *testing.T, hit map[string]any) string {
	t.Helper()
	body, err := json.Marshal(map[string]any{"agents": []any{hit}})
	require.NoError(t, err)
	return string(body)
}

type cardResult struct {
	Agents []struct {
		AgentID      string   `json:"agent_id"`
		Capabilities []string `json:"capabilities"`
		CardTrust    string   `json:"card_trust"`
		HealthStatus string   `json:"health_status"`
		HealthError  string   `json:"health_error"`
		Card         *struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Skills      []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"skills"`
		} `json:"card"`
	} `json:"agents"`
}

// A card that verifies against the chain's committed hash is what makes a
// result decidable: the tags say "trading", the card says what it trades.
func TestSearchAgentsSurfacesVerifiedCard(t *testing.T) {
	market := marketStub(t, marketBody(t, map[string]any{
		"agent_id":        "did:svp:svp1real",
		"endpoint":        "https://real.example",
		"capabilities":    []string{"trading"},
		"capability_hash": cardHash,
		"health_status":   "healthy",
		"card":            cardBody,
	}))
	defer market.Close()

	svc := &Service{Market: agentmarket.New(market.URL)}
	out, err := svc.Call(context.Background(), "search_agents", map[string]any{"query": "order books"})
	require.NoError(t, err)

	var decoded cardResult
	require.NoError(t, json.Unmarshal([]byte(out), &decoded))
	require.Len(t, decoded.Agents, 1)
	got := decoded.Agents[0]
	require.Equal(t, "verified", got.CardTrust)
	require.NotNil(t, got.Card)
	require.Equal(t, "Orderbook Analyst", got.Card.Name)
	require.Equal(t, "Analyzes order books and funding rates.", got.Card.Description)
	require.Len(t, got.Card.Skills, 1)
	require.Equal(t, "depth", got.Card.Skills[0].Name)

	// The raw card exists to be hashed, not read: only the projection may reach
	// the model, or a page of agents spends the context budget on card JSON.
	require.NotContains(t, out, "supportedInterfaces")
	require.NotContains(t, out, `\"skills\"`)
}

// On a failed check the market keeps the last card that DID verify. Showing its
// text would describe the agent with words it has since repudiated, so the
// prose is withheld while the tags — which come from the chain record — stay.
func TestSearchAgentsWithholdsSupersededCard(t *testing.T) {
	market := marketStub(t, marketBody(t, map[string]any{
		"agent_id":        "did:svp:svp1stale",
		"endpoint":        "https://stale.example",
		"capabilities":    []string{"trading"},
		"capability_hash": cardHash,
		"health_status":   "hash_mismatch",
		"health_error":    "agent card SHA-256 differs from on-chain capability_hash",
		"card":            cardBody,
	}))
	defer market.Close()

	svc := &Service{Market: agentmarket.New(market.URL)}
	out, err := svc.Call(context.Background(), "search_agents", map[string]any{"query": "order books"})
	require.NoError(t, err)

	var decoded cardResult
	require.NoError(t, json.Unmarshal([]byte(out), &decoded))
	require.Len(t, decoded.Agents, 1)
	got := decoded.Agents[0]
	require.Equal(t, "mismatch", got.CardTrust)
	require.Nil(t, got.Card, "a superseded card must not be shown as a description")
	require.Equal(t, []string{"trading"}, got.Capabilities)
	// The reason must survive, so the assistant can say why rather than
	// silently presenting a thinner result.
	require.Equal(t, "hash_mismatch", got.HealthStatus)
	require.Contains(t, got.HealthError, "capability_hash")
	require.NotContains(t, out, "Orderbook Analyst")
}

// An older market build serves no card at all. That is an absence of evidence,
// not a failure, and must not be reported as one.
func TestSearchAgentsWithoutCardIsUnverified(t *testing.T) {
	market := marketStub(t, marketBody(t, map[string]any{
		"agent_id":     "did:svp:svp1nocard",
		"endpoint":     "https://nocard.example",
		"capabilities": []string{"trading"},
	}))
	defer market.Close()

	svc := &Service{Market: agentmarket.New(market.URL)}
	out, err := svc.Call(context.Background(), "search_agents", map[string]any{"query": "order books"})
	require.NoError(t, err)

	var decoded cardResult
	require.NoError(t, json.Unmarshal([]byte(out), &decoded))
	require.Len(t, decoded.Agents, 1)
	require.Equal(t, "unverified", decoded.Agents[0].CardTrust)
	require.Nil(t, decoded.Agents[0].Card)
}
