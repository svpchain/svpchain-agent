package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/cosmos/evm/crypto/ethsecp256k1"

	"github.com/svpchain/svpchain-agent/internal/agent/hitl"
	"github.com/svpchain/svpchain-agent/internal/agent/llm"
	agentsettlement "github.com/svpchain/svpchain-agent/internal/agent/settlement"
	"github.com/svpchain/svpchain-agent/internal/agentmarket"
)

// BeginSettlementTool escrows the selected market agent's advertised price for
// one user-requested behavior, assigns its task through validator, and attaches
// that exact agent. The behavior may need several sequential transactions.
const BeginSettlementTool = "begin_agent_settlement"

type paidAgentFlow struct {
	validatorURL string
	rpcURL       string
	chainID      string
	priv         *ethsecp256k1.PrivKey
	confirm      hitl.Func
	market       *agentmarket.Client
	validator    *agentsettlement.Client
	reporter     *agentsettlement.Reporter
	mu           sync.Mutex
	started      bool
	endpoint     string
}

func newPaidAgentFlow(validatorURL, rpcURL, chainID string, priv *ethsecp256k1.PrivKey, confirm hitl.Func, market *agentmarket.Client, reporter *agentsettlement.Reporter) (*paidAgentFlow, error) {
	validator, err := agentsettlement.NewClient(validatorURL, "")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(rpcURL) == "" {
		rpcURL = "https://svp-devnet.svpchain.org/evm/"
	}
	return &paidAgentFlow{
		validatorURL: strings.TrimSpace(validatorURL), rpcURL: rpcURL, chainID: chainID,
		priv: priv, confirm: confirm, market: market, validator: validator, reporter: reporter,
	}, nil
}

func (f *paidAgentFlow) ToolDef() llm.Tool {
	return llm.Tool{Type: "function", Function: llm.Function{
		Name:        BeginSettlementTool,
		Description: "Pay the selected active market agent's advertised price through AgentSettlement for one user-requested behavior before using its execution tools. Requires the agent_id returned by search_agents. This opens local confirmations for ERC-20 approval and deposit, assigns one validator task for the behavior, then attaches that exact agent. The behavior may require several sequential transactions; its final successful broadcast is reported to the validator.",
		Parameters: map[string]any{"type": "object", "properties": map[string]any{
			"agent_id": map[string]any{"type": "string", "description": "The exact agent_id from search_agents"},
		}, "required": []string{"agent_id"}},
	}}
}

// PaymentToken reads the token configured by the validator and is used only to
// make the Agent Market's base-unit quotes understandable before settlement.
func (f *paidAgentFlow) PaymentToken(ctx context.Context) (agentsettlement.PaymentToken, error) {
	if f == nil || f.validator == nil {
		return agentsettlement.PaymentToken{}, fmt.Errorf("agent settlement is not configured")
	}
	network, err := f.validator.NetworkConfig(ctx)
	if err != nil {
		return agentsettlement.PaymentToken{}, err
	}
	return agentsettlement.ReadPaymentToken(ctx, f.rpcURL, network.PaymentToken)
}

func (f *paidAgentFlow) Start(ctx context.Context, args map[string]any, attach func(context.Context, string) (string, error)) (string, error) {
	if f == nil {
		return "", fmt.Errorf("agent settlement is not configured")
	}
	agentID, _ := args["agent_id"].(string)
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return "", fmt.Errorf("agent_id is required")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.started {
		return "", fmt.Errorf("a paid agent task is already active in this run")
	}
	if f.market == nil || f.reporter == nil {
		return "", fmt.Errorf("agent settlement dependencies are unavailable")
	}
	hit, err := f.market.Get(ctx, agentID)
	if err != nil {
		return "", err
	}
	if hit.Status != "AGENT_STATUS_ACTIVE" {
		return "", fmt.Errorf("agent %q is not active", agentID)
	}
	if strings.TrimSpace(hit.Endpoint) == "" || strings.TrimSpace(hit.Owner) == "" || strings.TrimSpace(hit.Pricing.Amount) == "" {
		return "", fmt.Errorf("agent %q has incomplete endpoint, owner, or pricing data", agentID)
	}
	network, err := f.validator.NetworkConfig(ctx)
	if err != nil {
		return "", err
	}
	owner, err := agentsettlement.EVMOwner(hit.Owner)
	if err != nil {
		return "", err
	}
	funded, err := agentsettlement.Fund(ctx, agentsettlement.FundingConfig{
		ChainID: f.chainID, RPCURL: f.rpcURL, SettlementContract: network.SettlementContract,
		PaymentToken: network.PaymentToken, Amount: hit.Pricing.Amount, PrivateKey: f.priv, Confirm: f.confirm,
	})
	if err != nil {
		return "", fmt.Errorf("fund agent settlement: %w", err)
	}
	// A successful deposit has moved user funds. Do not allow this run to make a
	// second deposit if assignment or reporting setup fails afterwards.
	f.started = true
	if _, err := f.validator.CreateTask(ctx, agentsettlement.Task{
		IntentID: funded.IntentID, TaskID: funded.TaskID, Amount: hit.Pricing.Amount, Owner: owner.Hex(),
	}); err != nil {
		// The deposit above already moved user funds. Carry the on-chain
		// identifiers into the failure so the escrow can be assigned later:
		// without them the run log records no way to find the deposit again,
		// and recovering it means scanning settlement contract events.
		return "", fmt.Errorf("assign settlement task (deposit already escrowed: intent_id=%s task_id=%s deposit_tx=%s amount=%s): %w",
			funded.IntentID, funded.TaskID, funded.DepositTxHash, hit.Pricing.Amount, err)
	}
	if err := f.reporter.Configure(agentsettlement.Config{
		TaskID: funded.TaskID, Owner: owner.Hex(), ValidatorURL: f.validatorURL, Source: agentsettlement.SourceEVM,
	}); err != nil {
		return "", err
	}
	f.endpoint = normalizeAgentEndpoint(hit.Endpoint)
	// The remote A2A endpoint is not contacted until the escrow deposit has
	// mined and its validator task exists. Connecting authenticates and fetches
	// the agent card, so it belongs after payment just like build_* calls do.
	if _, err := attach(ctx, hit.Endpoint); err != nil {
		return "", fmt.Errorf("attach selected agent after settlement: %w", err)
	}
	result, err := json.Marshal(map[string]string{
		"agent_id": agentID, "endpoint": hit.Endpoint, "owner": owner.Hex(), "amount": hit.Pricing.Amount,
		"intent_id": funded.IntentID, "task_id": funded.TaskID, "approve_tx_hash": funded.ApproveTxHash, "deposit_tx_hash": funded.DepositTxHash,
	})
	if err != nil {
		return "", err
	}
	return string(result), nil
}

// StartEndpoint resolves an endpoint against the current active Agent Market
// projection and funds that record before connecting to it. It intentionally
// does not depend on whether the endpoint originated from search_agents or
// the paginated agents listing.
func (f *paidAgentFlow) StartEndpoint(ctx context.Context, endpoint string, attach func(context.Context, string) (string, error)) (string, error) {
	if f == nil || f.market == nil {
		return "", fmt.Errorf("agent settlement dependencies are unavailable")
	}
	hit, found, err := f.market.FindActiveByEndpoint(ctx, endpoint)
	if err != nil {
		return "", fmt.Errorf("verify agent against Agent Market before settlement: %w", err)
	}
	if !found {
		return "", fmt.Errorf("agent endpoint %q is not an active Agent Market record; execution requires a market agent with a quoted price", strings.TrimSpace(endpoint))
	}
	return f.Start(ctx, map[string]any{"agent_id": hit.AgentID}, attach)
}

// ConnectEndpoint is the public paid connection entry point. begin_agent_settlement
// already attaches the selected agent; a later a2a_connect_agent for that same
// endpoint must therefore be idempotent and never fund a second task.
func (f *paidAgentFlow) ConnectEndpoint(ctx context.Context, endpoint string, attach func(context.Context, string) (string, error)) (string, error) {
	if f == nil {
		return "", fmt.Errorf("agent settlement is not configured")
	}
	f.mu.Lock()
	started, activeEndpoint := f.started, f.endpoint
	f.mu.Unlock()
	if !started {
		return f.StartEndpoint(ctx, endpoint, attach)
	}
	if activeEndpoint == "" || normalizeAgentEndpoint(endpoint) != activeEndpoint {
		return "", fmt.Errorf("a paid agent task is already active for %s; cannot connect a different endpoint in this run", activeEndpoint)
	}
	return attach(ctx, activeEndpoint)
}

// EnsureEndpoint is a final execution gate for attached A2A tools. It makes
// an already attached agent safe even if a future connection path forgets to
// call StartEndpoint: no request reaches that agent until the deposit has
// completed and the validator task has been assigned.
func (f *paidAgentFlow) EnsureEndpoint(ctx context.Context, endpoint string, attach func(context.Context, string) (string, error)) error {
	if f == nil {
		return fmt.Errorf("agent settlement is not configured")
	}
	f.mu.Lock()
	started, activeEndpoint := f.started, f.endpoint
	f.mu.Unlock()
	if started {
		if activeEndpoint == "" || normalizeAgentEndpoint(endpoint) != activeEndpoint {
			return fmt.Errorf("a paid agent task is active for %s, not %s", activeEndpoint, normalizeAgentEndpoint(endpoint))
		}
		return nil
	}
	_, err := f.StartEndpoint(ctx, endpoint, attach)
	return err
}

func normalizeAgentEndpoint(endpoint string) string {
	return strings.TrimRight(strings.TrimSpace(endpoint), "/")
}
