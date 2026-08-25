// Package agentmarket is a read-only client for the SVP Agent Market service's
// semantic search API, used to shortlist remote agents when the assistant does
// not already know which one can handle a task.
//
// # Trust
//
// The market service is a THIRD remote party, separate from the chain REST
// endpoint (Agent Hub) and the remote MCP server. It is an indexer: it derives
// its data from the chain, but nothing here can prove that. A delegated task
// carries a spending credential, so an attacker who could substitute an agent's
// endpoint could redirect that credential to themselves.
//
// This client therefore treats a search response as a RANKING HINT ONLY. It
// parses the agent id and the similarity score and discards every other field
// the service returns — endpoint, capability hash, status, owner. Callers
// re-read each shortlisted agent from the chain (registry.AgentByID) to get the
// authoritative record. The worst a hostile or buggy market service can do is
// return a useless ordering, or name agents that do not exist on chain and are
// dropped when the caller fails to resolve them.
//
// Discarding those fields is structural, not a convention: they are absent from
// the Hit type, so no caller can accidentally depend on them.
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

// DefaultURL is used when no Agent Market URL is configured, so semantic search
// works out of the box. Mirrors how the remote MCP client defaults its own
// endpoint. Clearing the setting restores this default rather than disabling
// search; search_agents is dropped only when the chain endpoint (Agent Hub) is
// itself unset, since a hit that cannot be verified against the chain must
// never be shown.
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

// Hit is one search result: which agent, and how well it matched.
//
// Deliberately minimal. See the package comment: everything else the service
// reports about an agent is unverifiable here and must come from the chain.
type Hit struct {
	AgentID    string
	Similarity float64
}

// Query narrows a search. Status is intentionally not exposed: the market's
// view of on-chain status is a cache, and the caller re-resolves every hit
// against the chain anyway.
type Query struct {
	Text       string
	Capability string
	Limit      int
}

type searchResponse struct {
	Agents []struct {
		AgentID    string  `json:"agent_id"`
		Similarity float64 `json:"similarity"`
	} `json:"agents"`
	Error string `json:"error"`
}

// Search returns agent ids ranked by semantic similarity to q.Text.
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
	// Only ACTIVE agents can accept delegated work, so never spend ranking
	// slots on ones that cannot. The chain re-check still has the final say.
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
	hits := make([]Hit, 0, len(decoded.Agents))
	for _, a := range decoded.Agents {
		if id := strings.TrimSpace(a.AgentID); id != "" {
			hits = append(hits, Hit{AgentID: id, Similarity: a.Similarity})
		}
	}
	return hits, nil
}
