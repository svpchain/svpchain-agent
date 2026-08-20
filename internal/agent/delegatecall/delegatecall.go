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

	"github.com/svpchain/svpchain-agent/internal/agent/hitl"
	"github.com/svpchain/svpchain-agent/internal/agent/llm"
	"github.com/svpchain/svpchain-agent/internal/delegation"
	"github.com/svpchain/svpchain-agent/internal/registry"
)

// ConfirmRequest / ConfirmFunc are the HITL types used for grants. Kept as
// aliases so existing call sites and tests keep compiling.
type (
	ConfirmRequest = hitl.Request
	ConfirmFunc    = hitl.Func
)

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
		"delegate_task",
		"list_settlements", "settle_settlement", "refund_settlement":
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
	case "list_settlements":
		return s.listSettlements(ctx)
	case "settle_settlement":
		return s.settleSettlement(ctx, args)
	case "refund_settlement":
		return s.refundSettlement(ctx, args)
	default:
		return "", fmt.Errorf("unknown delegation tool %q", name)
	}
}

// confirm runs the HITL gate. Denial is *hitl.Denied so the agent loop stops
// instead of letting the model retry the grant.
func (s *Service) confirm(ctx context.Context, req ConfirmRequest) error {
	return hitl.Ask(ctx, s.Confirm, req)
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
	if refresh, _ := args["refresh"].(bool); refresh {
		s.Registry.InvalidateCache()
	}
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
		// Stated so the assistant can answer "which chain is this reading?"
		// from a tool result instead of guessing at server-side config.
		"agent_hub_url": s.Registry.BaseURL(),
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
					"Runs LOCALLY on the user's machine and reads the chain directly over the " +
					"REST endpoint configured in Settings (reported as agent_hub_url in the " +
					"result) — it does not go through the remote MCP server. " +
					"Optionally filter by one exact capability tag (e.g. \"trading\"). " +
					"Returns agent DIDs, A2A endpoints, capability tags, pricing and bond. " +
					"Results are cached briefly; pass refresh=true after a chain restart or " +
					"a registration change to bypass the cache.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"capability": map[string]any{
							"type":        "string",
							"description": "Exact capability tag to filter by; omit for all agents",
						},
						"refresh": map[string]any{
							"type":        "boolean",
							"description": "Bypass the local cache and re-read the chain",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: llm.Function{
				Name: "get_agent_card",
				Description: "Fetch an agent's A2A card from the endpoint the agent registered " +
					"ON CHAIN (not from any local setting). Runs locally. The card lists the " +
					"agent's skills and tools. \"verified\" reports whether the card matches the " +
					"hash registered on chain; an unreachable card usually means the agent's " +
					"registered endpoint is stale or its service is down.",
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
					"so every action and subaccount the user wants delegated must be listed. List only " +
					"chain-executable (write) actions here — query.* actions belong in task credentials, " +
					"not the root.",
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
						"svc_spend_limit_total": coinListSchema(
							"Lifetime cap on paying agents for their services (optional, but " +
								"required before any delegate_task with a service_budget — an " +
								"empty service allowance denies all agent payments)"),
						"svc_spend_limit_daily": coinListSchema("Per-day service payment cap (optional)"),
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
				Description: "Delegate one task to a discovered remote agent. Mints a short-lived " +
					"SVP-DT credential (after the user confirms the grant in a dialog), attaches it to the " +
					"A2A message metadata under svp.delegation/v1, and sends {skill, tool, args} to the " +
					"agent's A2A endpoint. Use get_agent_card first to learn the agent's skills/tools and " +
					"their argument shapes. Grant the minimum: only the actions, subaccounts and budget " +
					"this one task needs. For read-only tasks grant [\"query.account\"] with no budget. " +
					"Set redelegable=true plus redelegate_to ONLY when the target agent must sub-delegate " +
					"to another named agent — never \"just in case\".",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"agent_id": map[string]any{"type": "string", "description": "Target agent DID from discover_agents"},
						"skill":    map[string]any{"type": "string", "description": "A2A skill id from the agent's card"},
						"tool":     map[string]any{"type": "string", "description": "Tool name within the skill"},
						"args": map[string]any{
							"type":        "object",
							"description": "Tool arguments per the agent's card; the credential is attached to the message metadata automatically — never place it in args",
						},
						"actions": map[string]any{
							"type": "array", "items": map[string]any{"type": "string"},
							"description": "Actions the credential grants, e.g. [\"clob.place_order\"], or [\"query.account\"] for read-only access",
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
						"service_budget": map[string]any{
							"type": "object",
							"description": "Pay the agent for this task: one coin, e.g. " +
								`{"denom":"uusdc","amount":"500000"}. Opens an on-chain ` +
								"escrow order for that amount (plus the chain's payment fee), binds " +
								"the credential to it, and grants settlement.record_spend on " +
								"subaccount 0 so the agent can record its spend and be paid. After " +
								"the task, settle_settlement pays out and refunds the rest. Requires " +
								"the root delegation to grant the action and a svc_spend_limit_total.",
							"properties": map[string]any{
								"denom":  map[string]any{"type": "string"},
								"amount": map[string]any{"type": "string"},
							},
						},
						"expires_in_seconds": map[string]any{
							"type":        "integer",
							"description": "Credential TTL; default 300, clamped to the chain ceiling",
						},
						"root_id": map[string]any{
							"type":        "string",
							"description": "Root delegation to mint under (hex); defaults to the newest usable one",
						},
						"redelegable": map[string]any{
							"type": "boolean",
							"description": "Allow the target agent to re-delegate a NARROWED copy of this credential " +
								"one further hop. Requires redelegate_to. Default false; leave false unless the " +
								"workflow truly has an intermediary.",
						},
						"redelegate_to": map[string]any{
							"type": "array", "items": map[string]any{"type": "string"},
							"description": "Agent DIDs the credential may be re-delegated to (required with " +
								"redelegable; each must be a registered active agent). The credential is " +
								"useless to anyone not listed.",
						},
					},
					"required": []string{"agent_id", "skill", "tool", "actions", "subaccounts"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.Function{
				Name: "list_settlements",
				Description: "List the user's settlement escrow orders: status, cap, recorded " +
					"spend, and — for open orders — what each agent is still owed.",
				Parameters: map[string]any{"type": "object", "properties": map[string]any{}},
			},
		},
		{
			Type: "function",
			Function: llm.Function{
				Name: "settle_settlement",
				Description: "Close a settlement order normally after a paid task: the unrecorded " +
					"remainder refunds to the user, and what the agent recorded stays claimable " +
					"by it. Use after the task's work is delivered.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"settlement_id": map[string]any{"type": "string", "description": "Order id (hex) from delegate_task or list_settlements"},
					},
					"required": []string{"settlement_id"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.Function{
				Name: "refund_settlement",
				Description: "Close a settlement order by full refund: everything not yet claimed " +
					"returns to the user and the agent's unclaimed accruals are zeroed. The " +
					"Emergency-Stop companion for a failed or aborted task — use settle_settlement " +
					"for a task that succeeded.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"settlement_id": map[string]any{"type": "string", "description": "Order id (hex)"},
					},
					"required": []string{"settlement_id"},
				},
			},
		},
	}
}
