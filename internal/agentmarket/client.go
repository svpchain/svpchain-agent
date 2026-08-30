// Package agentmarket is a read-only client for the SVP Agent Market service's
// semantic search API — the assistant's only way to find remote agents.
//
// # Trust
//
// The market service is a remote party, separate from the remote MCP server.
// It indexes the chain's x/agent registry, but nothing here can prove that:
// this client reports what the service returns, including each agent's A2A
// endpoint. Nothing verifies that endpoint against the chain.
//
// What that does and does not put at risk: a search result decides where an
// `a2a_send_message` goes, so a hostile or stale index can point the assistant
// at an endpoint of its choosing and see whatever is in that message. It cannot
// move the user's funds — A2A messages carry no credential, and every on-chain
// write still goes through remote build → local sign (user-confirmed, whitelist
// checked) → remote broadcast.
package agentmarket

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client reads the market service's search API.
type Client struct {
	base string
	http *http.Client
}

// DefaultURL is used when no Agent Market URL is configured, so agent search
// works out of the box. Mirrors how the remote MCP client defaults its own
// endpoint; clearing the setting restores this default rather than disabling
// search.
const DefaultURL = "https://dev02.svpchain.org"

// New returns a client for baseURL, falling back to DefaultURL when empty.
func New(baseURL string) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = DefaultURL
	}
	return &Client{base: baseURL, http: &http.Client{Timeout: 20 * time.Second}}
}

// BaseURL reports the configured endpoint, for display in tool results.
func (c *Client) BaseURL() string {
	if c == nil {
		return ""
	}
	return c.base
}

// Coin is a chain coin with the amount kept as its decimal string.
type Coin struct {
	Denom  string `json:"denom,omitempty"`
	Amount string `json:"amount,omitempty"`
}

// Pricing is an agent's advertised price in the network AgentSettlement
// payment token's smallest unit. The token is selected by the network
// settlement deployment, not by an individual agent record.
type Pricing struct {
	Amount string `json:"amount,omitempty"`
	Unit   string `json:"unit,omitempty"`
}

// Hit is one search result: an agent as the market service describes it, plus
// how well it matched. Every field except Similarity is the service's claim
// about an on-chain record — see the package comment.
type Hit struct {
	AgentID      string   `json:"agent_id"`
	Owner        string   `json:"owner,omitempty"`
	Endpoint     string   `json:"endpoint,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Pricing      Pricing  `json:"pricing,omitzero"`
	Bond         Coin     `json:"bond,omitzero"`
	Status       string   `json:"status,omitempty"`
	Metadata     string   `json:"metadata,omitempty"`
	Similarity   float64  `json:"similarity"`
}

// Query narrows a search.
type Query struct {
	Text       string
	Capability string
	Limit      int
}

type searchResponse struct {
	Agents []Hit  `json:"agents"`
	Error  string `json:"error"`
}

// ListQuery narrows a paginated listing of active Agent records.
type ListQuery struct {
	Capability string
	Limit      int
	Cursor     int
}

// Page is one page from the Agent Market listing API.
type Page struct {
	Agents     []Hit `json:"agents"`
	Limit      int   `json:"limit"`
	Cursor     int   `json:"cursor"`
	NextCursor int   `json:"next_cursor"`
}

// Get returns the current on-chain-derived record for one agent.
func (c *Client) Get(ctx context.Context, agentID string) (Hit, error) {
	if c == nil {
		return Hit{}, fmt.Errorf("agent market search is not configured")
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return Hit{}, fmt.Errorf("agent id is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/v1/agents/"+url.PathEscape(agentID), nil)
	if err != nil {
		return Hit{}, err
	}
	response, err := c.http.Do(req)
	if err != nil {
		return Hit{}, fmt.Errorf("agent market unreachable: %w", err)
	}
	defer response.Body.Close()
	var hit Hit
	if err := json.NewDecoder(response.Body).Decode(&hit); err != nil && response.StatusCode == http.StatusOK {
		return Hit{}, fmt.Errorf("decode agent market response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return Hit{}, fmt.Errorf("agent market lookup failed (HTTP %d)", response.StatusCode)
	}
	hits := usableHits([]Hit{hit})
	if len(hits) != 1 {
		return Hit{}, fmt.Errorf("agent market lookup returned an invalid agent record")
	}
	return hits[0], nil
}

type listResponse struct {
	Page
	Error string `json:"error"`
}

// List returns a page of active Agent records without semantic ranking.
func (c *Client) List(ctx context.Context, q ListQuery) (Page, error) {
	if c == nil {
		return Page{}, fmt.Errorf("agent market search is not configured")
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 10
	}
	values := url.Values{}
	values.Set("limit", strconv.Itoa(limit))
	values.Set("cursor", strconv.Itoa(max(q.Cursor, 0)))
	values.Set("status", "AGENT_STATUS_ACTIVE")
	if cap := strings.TrimSpace(q.Capability); cap != "" {
		values.Set("capability", cap)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/v1/agents?"+values.Encode(), nil)
	if err != nil {
		return Page{}, err
	}
	response, err := c.http.Do(req)
	if err != nil {
		return Page{}, fmt.Errorf("agent market unreachable: %w", err)
	}
	defer response.Body.Close()

	var decoded listResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil && response.StatusCode == http.StatusOK {
		return Page{}, fmt.Errorf("decode agent market response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		if decoded.Error != "" {
			return Page{}, fmt.Errorf("agent market list failed (HTTP %d): %s", response.StatusCode, decoded.Error)
		}
		return Page{}, fmt.Errorf("agent market list failed (HTTP %d)", response.StatusCode)
	}
	decoded.Agents = usableHits(decoded.Agents)
	return decoded.Page, nil
}

// FindActiveByEndpoint resolves an active market record by its advertised A2A
// endpoint. The market currently exposes no endpoint index, so this walks its
// bounded paginated listing. A caller must not fall back to a free execution
// when this lookup fails: that would let a market agent evade settlement on a
// later chat turn, after the original search result has left process memory.
func (c *Client) FindActiveByEndpoint(ctx context.Context, endpoint string) (Hit, bool, error) {
	if c == nil {
		return Hit{}, false, fmt.Errorf("agent market search is not configured")
	}
	want := normalizeEndpoint(endpoint)
	if want == "" {
		return Hit{}, false, fmt.Errorf("agent endpoint is required")
	}

	const (
		pageSize = 200
		maxPages = 50
	)
	cursor := 0
	for pageNumber := 0; pageNumber < maxPages; pageNumber++ {
		page, err := c.List(ctx, ListQuery{Limit: pageSize, Cursor: cursor})
		if err != nil {
			return Hit{}, false, err
		}
		for _, hit := range page.Agents {
			if normalizeEndpoint(hit.Endpoint) == want {
				return hit, true, nil
			}
		}
		if len(page.Agents) == 0 || page.NextCursor <= cursor {
			return Hit{}, false, nil
		}
		cursor = page.NextCursor
	}
	return Hit{}, false, fmt.Errorf("agent market endpoint lookup exceeded %d pages", maxPages)
}

// Search returns agents ranked by semantic similarity to q.Text.
func (c *Client) Search(ctx context.Context, q Query) ([]Hit, error) {
	if c == nil {
		return nil, fmt.Errorf("agent market search is not configured")
	}
	text := strings.TrimSpace(q.Text)
	if text == "" {
		return nil, fmt.Errorf("search text is required")
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 10
	}
	values := url.Values{}
	values.Set("q", text)
	values.Set("limit", strconv.Itoa(limit))
	// Only ACTIVE agents can be acted on, so never spend ranking slots on ones
	// that cannot.
	values.Set("status", "AGENT_STATUS_ACTIVE")
	if cap := strings.TrimSpace(q.Capability); cap != "" {
		values.Set("capability", cap)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/v1/agents/search?"+values.Encode(), nil)
	if err != nil {
		return nil, err
	}
	response, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("agent market unreachable: %w", err)
	}
	defer response.Body.Close()

	var decoded searchResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil && response.StatusCode == http.StatusOK {
		return nil, fmt.Errorf("decode agent market response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		if decoded.Error != "" {
			return nil, fmt.Errorf("agent market search failed (HTTP %d): %s", response.StatusCode, decoded.Error)
		}
		return nil, fmt.Errorf("agent market search failed (HTTP %d)", response.StatusCode)
	}
	return usableHits(decoded.Agents), nil
}

func usableHits(agents []Hit) []Hit {
	hits := make([]Hit, 0, len(agents))
	for _, a := range agents {
		a.AgentID = strings.TrimSpace(a.AgentID)
		if a.AgentID == "" {
			continue
		}
		a.Endpoint = normalizeEndpoint(a.Endpoint)
		hits = append(hits, a)
	}
	return hits
}

// normalizeEndpoint makes an endpoint emitted by the market and the same
// endpoint sent back to a2a_connect_agent compare consistently. A trailing
// slash is not meaningful for the A2A base endpoint used here.
func normalizeEndpoint(endpoint string) string {
	return strings.TrimRight(strings.TrimSpace(endpoint), "/")
}
