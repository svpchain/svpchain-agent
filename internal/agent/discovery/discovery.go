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

	"github.com/svpchain/svpchain-agent/internal/agent/llm"
	"github.com/svpchain/svpchain-agent/internal/agentmarket"
)

// Service holds what the discovery tools need for one run.
type Service struct {
	// Market is the agent search backend. Nil (or an empty base URL) drops
	// search_agents from the tool list.
	Market *agentmarket.Client
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

// searchAgents shortlists agents by semantic similarity to a task description.
func (s *Service) searchAgents(ctx context.Context, args map[string]any) (string, error) {
	query, _ := args["query"].(string)
	if strings.TrimSpace(query) == "" {
		return "", fmt.Errorf("query is required")
	}
	capability, _ := args["capability"].(string)
	limit := 10
	if raw, ok := args["limit"].(float64); ok && raw > 0 {
		limit = int(raw)
	}

	hits, err := s.Market.Search(ctx, agentmarket.Query{Text: query, Capability: capability, Limit: limit})
	if err != nil {
		return "", err
	}

	out, err := json.Marshal(map[string]any{
		"agents": hits,
		"count":  len(hits),
		"query":  query,
		// Stated so the assistant can answer "where did this come from?" from a
		// tool result instead of guessing at configuration.
		"agent_market_url": s.Market.BaseURL(),
		"source":           "agent market semantic search; endpoints and capabilities are as the market service reports them",
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
				Description: "Find remote agents by DESCRIBING THE TASK in natural language, " +
					"ranked by semantic similarity against each agent's published A2A card. " +
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
						"query": map[string]any{
							"type":        "string",
							"description": "The task, in natural language, e.g. \"check perpetual funding rates on BTC-USD\"",
						},
						"capability": map[string]any{
							"type":        "string",
							"description": "Optional exact capability tag to narrow the search",
						},
						"limit": map[string]any{
							"type":        "integer",
							"description": "Maximum candidates to return; default 10",
						},
					},
					"required": []string{"query"},
				},
			},
		},
	}
}
