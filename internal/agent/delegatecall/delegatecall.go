// Package delegatecall exposes the agent-delegation tool surface to the LLM:
// discovering registered agents from the chain's x/agent registry, managing
// the user's root delegation, and delegating tasks to remote agents under
// SVP-DT credentials.
package delegatecall

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/svpchain/svpchain-local-agent/internal/agent/llm"
	"github.com/svpchain/svpchain-local-agent/internal/delegation"
	"github.com/svpchain/svpchain-local-agent/internal/registry"
)

// ConfirmRequest is what the user is shown before anything is granted.
type ConfirmRequest struct {
	// Kind selects the dialog: "create_delegation", "resume_delegation",
	// "revoke_delegation" or "delegate_task".
	Kind  string `json:"kind"`
	Title string `json:"title"`
	// Lines are the human-readable grant terms, in display order.
	Lines []string `json:"lines"`
}

// ConfirmFunc asks the user. A nil hook, a false answer, an error or a timeout
// all deny — no credential is ever minted and no delegation is ever created
// without an explicit approval.
type ConfirmFunc func(ctx context.Context, req ConfirmRequest) bool

// Service holds what the delegation tools need for one run.
type Service struct {
	Registry *registry.Client
	// Lifecycle signs delegation transactions and mints credentials with the
	// user's key. Nil leaves only the read-only discovery tools available.
	Lifecycle *delegation.Lifecycle
	// Confirm gates every grant.
	Confirm ConfirmFunc
	// Now is the mint clock, injectable for tests.
	Now func() int64
}

// Enabled reports whether the delegation tool surface is available this run.
func (s *Service) Enabled() bool {
	return s != nil && s.Registry != nil && s.Registry.BaseURL() != ""
}

func (s *Service) canDelegate() bool {
	return s.Enabled() && s.Lifecycle != nil
}

// IsTool reports whether name belongs to this package.
func IsTool(name string) bool {
	switch name {
	case "discover_agents", "get_agent_card",
		"list_delegations", "create_root_delegation",
		"pause_delegation", "resume_delegation", "revoke_delegation",
		"delegate_task":
		return true
	default:
		return false
	}
}

// Call dispatches one tool call.
func (s *Service) Call(ctx context.Context, name string, args map[string]any) (string, error) {
	if !s.Enabled() {
		return "", fmt.Errorf("agent discovery is not configured: set the chain REST endpoint in Settings")
	}
	switch name {
	case "discover_agents":
		return s.discoverAgents(ctx, args)
	case "get_agent_card":
		return s.getAgentCard(ctx, args)
	case "list_delegations":
		return s.listDelegations(ctx)
	case "create_root_delegation":
		return s.createRootDelegation(ctx, args)
	case "pause_delegation":
		return s.pauseDelegation(ctx, args)
	case "resume_delegation":
		return s.resumeDelegation(ctx, args)
	case "revoke_delegation":
		return s.revokeDelegation(ctx, args)
	case "delegate_task":
		return s.delegateTask(ctx, args)
	default:
		return "", fmt.Errorf("unknown delegation tool %q", name)
	}
}

// confirm runs the approval gate; denial is an error the LLM sees verbatim.
func (s *Service) confirm(ctx context.Context, req ConfirmRequest) error {
	if s.Confirm == nil || !s.Confirm(ctx, req) {
		return fmt.Errorf("the user declined %q — do not retry without new instructions from the user", req.Title)
	}
	return nil
}

// agentSummary is the discovery row handed to the LLM: enough to choose an
// agent, small enough not to flood the context.
type agentSummary struct {
	AgentID      string           `json:"agent_id"`
	Endpoint     string           `json:"endpoint"`
	Capabilities []string         `json:"capabilities"`
	Pricing      registry.Pricing `json:"pricing"`
	Bond         registry.Coin    `json:"bond"`
	Metadata     string           `json:"metadata,omitempty"`
}

func (s *Service) discoverAgents(ctx context.Context, args map[string]any) (string, error) {
	capability, _ := args["capability"].(string)
	agents, err := s.Registry.Agents(ctx, capability)
	if err != nil {
		return "", err
	}
	rows := make([]agentSummary, 0, len(agents))
	for _, a := range agents {
		rows = append(rows, agentSummary{
			AgentID:      a.AgentID,
			Endpoint:     a.Endpoint,
			Capabilities: a.Capabilities,
			Pricing:      a.Pricing,
			Bond:         a.Bond,
			Metadata:     a.Metadata,
		})
	}
	out, err := json.Marshal(map[string]any{
		"agents": rows,
		"count":  len(rows),
	})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (s *Service) getAgentCard(ctx context.Context, args map[string]any) (string, error) {
	agentID, _ := args["agent_id"].(string)
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return "", fmt.Errorf("agent_id is required")
	}
	agent, err := s.Registry.AgentByID(ctx, agentID)
	if err != nil {
		return "", err
	}
	card, err := s.Registry.FetchCard(ctx, agent)
	if err != nil {
		return "", err
	}
	out, err := json.Marshal(map[string]any{
		"agent_id": agent.AgentID,
		"endpoint": agent.Endpoint,
		"card":     card.Raw,
		// verified means the served card matches the sha256 the agent
		// registered on chain; treat an unverified card with suspicion.
		"verified": card.Verified,
	})
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// coinListSchema is the JSON schema for a coin-array argument.
func coinListSchema(desc string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": desc,
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"denom":  map[string]any{"type": "string"},
				"amount": map[string]any{"type": "string", "description": "base-10 integer string in the denom's base units"},
			},
			"required": []string{"denom", "amount"},
		},
	}
}

// ToolDefs returns the tool schemas this package serves.
func (s *Service) ToolDefs() []llm.Tool {
	if !s.Enabled() {
		return nil
	}
	defs := s.delegationToolDefs()
	return append([]llm.Tool{
		{
			Type: "function",
			Function: llm.Function{
				Name: "discover_agents",
				Description: "List ACTIVE agents registered on the svpchain x/agent registry. " +
					"Optionally filter by one exact capability tag (e.g. \"trading\"). " +
					"Returns agent DIDs, A2A endpoints, capability tags, pricing and bond.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"capability": map[string]any{
							"type":        "string",
							"description": "Exact capability tag to filter by; omit for all agents",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: llm.Function{
				Name: "get_agent_card",
				Description: "Fetch an agent's A2A card from its registered endpoint. " +
					"The card lists the agent's skills and tools. \"verified\" reports whether " +
					"the card matches the hash registered on chain.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"agent_id": map[string]any{
							"type":        "string",
							"description": "Agent DID, e.g. did:svp:svp1…",
						},
					},
					"required": []string{"agent_id"},
				},
			},
		},
	}, defs...)
}

// delegationToolDefs returns the grant-making tools, present only when a
// signing key is loaded.
func (s *Service) delegationToolDefs() []llm.Tool {
	if !s.canDelegate() {
		return nil
	}
	return []llm.Tool{
		{
			Type: "function",
			Function: llm.Function{
				Name: "list_delegations",
				Description: "List the user's on-chain root delegations with their limits, epoch, " +
					"pause state and consumed spend.",
				Parameters: map[string]any{"type": "object", "properties": map[string]any{}},
			},
		},
		{
			Type: "function",
			Function: llm.Function{
				Name: "create_root_delegation",
				Description: "Create the user's on-chain root delegation (to their own DID). Required once " +
					"before any delegate_task. The user confirms the terms in a dialog. Empty lists DENY, " +
					"so every action and subaccount the user wants delegated must be listed.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"actions": map[string]any{
							"type": "array", "items": map[string]any{"type": "string"},
							"description": "Permitted actions, e.g. [\"clob.place_order\",\"clob.cancel_order\"]",
						},
						"skills": map[string]any{
							"type": "array", "items": map[string]any{"type": "string"},
							"description": "Permitted A2A skills; optional",
						},
						"subaccounts": map[string]any{
							"type": "array", "items": map[string]any{"type": "integer"},
							"description": "Subaccount numbers the delegation may touch, e.g. [0]",
						},
						"denoms": map[string]any{
							"type": "array", "items": map[string]any{"type": "string"},
							"description": "Denominations the delegation may move; optional",
						},
						"spend_limit_total": coinListSchema("Lifetime spend cap (required)"),
						"spend_limit_daily": coinListSchema("Per-day spend cap (optional)"),
						"expires_at": map[string]any{
							"type":        "integer",
							"description": "Unix seconds when the delegation expires; default 30 days",
						},
					},
					"required": []string{"actions", "subaccounts", "spend_limit_total"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.Function{
				Name:        "pause_delegation",
				Description: "Emergency stop: suspend a delegation immediately. Every outstanding credential dies with it. No confirmation needed.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"root_id": map[string]any{"type": "string", "description": "Delegation root id (hex)"},
					},
					"required": []string{"root_id"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.Function{
				Name:        "resume_delegation",
				Description: "Lift a pause on a delegation. Credentials minted before the pause stay dead. The user confirms.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"root_id": map[string]any{"type": "string", "description": "Delegation root id (hex)"},
					},
					"required": []string{"root_id"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.Function{
				Name:        "revoke_delegation",
				Description: "Permanently revoke a delegation. The user confirms.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"root_id": map[string]any{"type": "string", "description": "Delegation root id (hex)"},
					},
					"required": []string{"root_id"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.Function{
				Name: "delegate_task",
				Description: "Delegate one task to a discovered remote agent. Mints a single-use, short-lived " +
					"SVP-DT credential (after the user confirms the grant in a dialog), injects it as args.proof, " +
					"and sends {skill, tool, args} to the agent's A2A endpoint. Use get_agent_card first to learn " +
					"the agent's skills/tools and their argument shapes. Grant the minimum: only the actions, " +
					"subaccounts and budget this one task needs.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"agent_id": map[string]any{"type": "string", "description": "Target agent DID from discover_agents"},
						"skill":    map[string]any{"type": "string", "description": "A2A skill id from the agent's card"},
						"tool":     map[string]any{"type": "string", "description": "Tool name within the skill"},
						"args": map[string]any{
							"type":        "object",
							"description": "Tool arguments per the agent's card; proof is injected automatically",
						},
						"actions": map[string]any{
							"type": "array", "items": map[string]any{"type": "string"},
							"description": "Actions the credential grants, e.g. [\"clob.place_order\"]",
						},
						"skills": map[string]any{
							"type": "array", "items": map[string]any{"type": "string"},
							"description": "A2A skills the credential grants; optional",
						},
						"subaccounts": map[string]any{
							"type": "array", "items": map[string]any{"type": "integer"},
							"description": "Subaccount numbers the task may touch",
						},
						"denoms": map[string]any{
							"type": "array", "items": map[string]any{"type": "string"},
							"description": "Denoms the task may move; optional",
						},
						"budget": coinListSchema(
							"Cap on what this one task may commit. REQUIRED for value-committing " +
								"actions such as clob.place_order — the chain prices the action and " +
								"refuses it against an empty budget. Size it for this task, not for " +
								"the delegation's whole allowance.",
						),
						"expires_in_seconds": map[string]any{
							"type":        "integer",
							"description": "Credential TTL; default 300, clamped to the chain ceiling",
						},
						"root_id": map[string]any{
							"type":        "string",
							"description": "Root delegation to mint under (hex); defaults to the newest usable one",
						},
					},
					"required": []string{"agent_id", "skill", "tool", "actions", "subaccounts"},
				},
			},
		},
	}
}
