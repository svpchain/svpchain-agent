// Package discovery exposes the agent-search tool surface to the LLM: finding
// remote agents by describing a task, through the SVP Agent Market service.
//
// The market service is the sole source of these results, endpoint included —
// nothing here cross-checks it against the chain. See package agentmarket for
// what that does and does not put at risk.
package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/svpchain/svpchain-agent/internal/agent/llm"
	"github.com/svpchain/svpchain-agent/internal/agentmarket"
)

// Service holds what the discovery tools need for one run.
type Service struct {
	// Market is the agent search backend. Nil (or an empty base URL) drops
	// search_agents from the tool list.
	Market *agentmarket.Client
	mu     sync.Mutex
	byURL  map[string]string
}

// Enabled reports whether the discovery tool surface is available this run.
func (s *Service) Enabled() bool {
	return s != nil && s.Market != nil && s.Market.BaseURL() != ""
}

// IsTool reports whether name belongs to this package.
func IsTool(name string) bool {
	return name == "search_agents"
}

// Call dispatches one tool call.
func (s *Service) Call(ctx context.Context, name string, args map[string]any) (string, error) {
	if !s.Enabled() {
		return "", fmt.Errorf("agent search is not configured: set the Agent Market URL in Settings")
	}
	if name != "search_agents" {
		return "", fmt.Errorf("unknown discovery tool %q", name)
	}
	return s.searchAgents(ctx, args)
}

// searchAgents either lists the active market page or semantically shortlists
// agents for a concrete task.
func (s *Service) searchAgents(ctx context.Context, args map[string]any) (string, error) {
	mode, _ := args["mode"].(string)
	mode = strings.ToLower(strings.TrimSpace(mode))
	query, _ := args["query"].(string)
	capability, _ := args["capability"].(string)
	limit := 10
	if raw, ok := args["limit"].(float64); ok && raw > 0 {
		limit = int(raw)
	}
	cursor := 0
	if raw, ok := args["cursor"].(float64); ok && raw > 0 {
		cursor = int(raw)
	}

	if mode == "list" {
		page, err := s.Market.List(ctx, agentmarket.ListQuery{Capability: capability, Limit: limit, Cursor: cursor})
		if err != nil {
			return "", err
		}
		s.remember(page.Agents)
		return marketResult(page.Agents, "list", query, page.Cursor, page.NextCursor, s.Market.BaseURL())
	}
	if mode != "" && mode != "search" {
		return "", fmt.Errorf("mode must be list or search")
	}
	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("query is required when mode is search")
	}

	hits, err := s.Market.Search(ctx, agentmarket.Query{Text: query, Capability: capability, Limit: limit})
	if err != nil {
		return "", err
	}
	s.remember(hits)
	return marketResult(hits, "search", query, 0, 0, s.Market.BaseURL())
}

// AgentIDForEndpoint returns an agent ID emitted in this run's most recent
// search/list result. It lets connection handling bind a market endpoint to
// the exact record whose current price and owner must be settled.
func (s *Service) AgentIDForEndpoint(endpoint string) string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.byURL[endpointKey(endpoint)]
}

func (s *Service) remember(hits []agentmarket.Hit) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byURL == nil {
		s.byURL = make(map[string]string)
	}
	for _, hit := range hits {
		endpoint, agentID := endpointKey(hit.Endpoint), strings.TrimSpace(hit.AgentID)
		if endpoint != "" && agentID != "" {
			s.byURL[endpoint] = agentID
		}
	}
}

func endpointKey(endpoint string) string {
	return strings.TrimRight(strings.TrimSpace(endpoint), "/")
}

func marketResult(hits []agentmarket.Hit, mode, query string, cursor, nextCursor int, marketURL string) (string, error) {
	out, err := json.Marshal(map[string]any{
		"agents":      hits,
		"count":       len(hits),
		"mode":        mode,
		"query":       query,
		"cursor":      cursor,
		"next_cursor": nextCursor,
		// Stated so the assistant can answer "where did this come from?" from a
		// tool result instead of guessing at configuration.
		"agent_market_url": marketURL,
		"source":           "agent market; endpoints and capabilities are as the market service reports them",
	})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// ToolDefs returns the tool schemas this package serves.
func (s *Service) ToolDefs() []llm.Tool {
	if !s.Enabled() {
		return nil
	}
	return []llm.Tool{
		{
			Type: "function",
			Function: llm.Function{
				Name: "search_agents",
				Description: "List active remote agents, or find agents for a concrete task. " +
					"For a generic question such as 'what agents are available?', use mode=list " +
					"and do not provide query; this pages GET /v1/agents. For a specific task, " +
					"use mode=search with a natural-language query, ranked by semantic similarity " +
					"against each agent's published A2A card. " +
					"Runs LOCALLY against the Agent Market service configured in Settings " +
					"(reported as agent_market_url in the result) — it does not go through the " +
					"remote MCP server. Returns each agent's DID, A2A endpoint, capability tags, " +
					"pricing and bond as the market service reports them. Similarity is 0..1; " +
					"treat below ~0.4 as a weak match and say so rather than acting on it " +
					"blindly. The endpoint returned is where a2a_send_message would go, so name " +
					"the agent you are contacting when you report back.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"mode": map[string]any{
							"type":        "string",
							"enum":        []string{"list", "search"},
							"description": "list for a paginated market browse; search for a concrete task",
						},
						"query": map[string]any{
							"type":        "string",
							"description": "Required only when mode is search: the task in natural language, e.g. \"check perpetual funding rates on BTC-USD\"",
						},
						"capability": map[string]any{
							"type":        "string",
							"description": "Optional exact capability tag to narrow the search",
						},
						"limit": map[string]any{
							"type":        "integer",
							"description": "Maximum candidates to return; default 10",
						},
						"cursor": map[string]any{
							"type":        "integer",
							"description": "For mode=list, the offset from next_cursor of a prior result; default 0",
						},
					},
					"required": []string{"mode"},
				},
			},
		},
	}
}
