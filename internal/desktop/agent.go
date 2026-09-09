package desktop

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/svpchain/svpchain-agent/internal/agent"
	"github.com/svpchain/svpchain-agent/internal/agent/history"
	"github.com/svpchain/svpchain-agent/internal/agent/llm"
	"github.com/svpchain/svpchain-agent/internal/agent/runlog"
	agentsettlement "github.com/svpchain/svpchain-agent/internal/agent/settlement"
	"github.com/svpchain/svpchain-agent/internal/agent/skills"
	"github.com/svpchain/svpchain-agent/internal/agentmarket"
	"github.com/svpchain/svpchain-agent/internal/chainrpc"
	"github.com/svpchain/svpchain-agent/internal/i18n"
	"github.com/svpchain/svpchain-agent/internal/keystore"
	"github.com/svpchain/svpchain-agent/internal/manage"
	"github.com/svpchain/svpchain-agent/internal/prefs"
	"github.com/svpchain/svpchain-agent/internal/signer"
)

// SkillSetting is one assistant skill row for the Settings UI.
type SkillSetting = skills.Setting

// AgentSettings is persisted LLM / MCP configuration for the assistant tab.
type AgentSettings struct {
	ChainID             string   `json:"chain_id"`
	LLMAPIKey           string   `json:"llm_api_key"`
	LLMBaseURL          string   `json:"llm_base_url"`
	LLMModel            string   `json:"llm_model"`
	LLMProvider         string   `json:"llm_provider"`
	LLMContextWindow    int      `json:"llm_context_window"`
	RemoteMCPURL        string   `json:"remote_mcp_url"`
	AgentMarketURL      string   `json:"agent_market_url"`
	AgentValidatorURL   string   `json:"agent_validator_url"`
	RemoteMCPDisabled   bool     `json:"remote_mcp_disabled"`
	DisabledSkills      []string `json:"disabled_skills"`
	SkillsConfigBase    string   `json:"skills_config_base"`
	ShowToolSteps       bool     `json:"show_tool_steps"`
	AgentRunLogDisabled bool     `json:"agent_run_log_disabled"`
	PhoenixOTLPURL      string   `json:"phoenix_otlp_url"`
}

// AgentGetSettings returns saved assistant settings (API key included for local use only).
func (a *App) AgentGetSettings() AgentSettings {
	s := a.store.AgentSettings()
	return AgentSettings{
		ChainID:             s.ChainID,
		LLMAPIKey:           s.LLMAPIKey,
		LLMBaseURL:          s.LLMBaseURL,
		LLMModel:            s.LLMModel,
		LLMProvider:         s.LLMProvider,
		LLMContextWindow:    s.LLMContextWindow,
		RemoteMCPURL:        s.RemoteMCPURL,
		AgentMarketURL:      s.AgentMarketURL,
		AgentValidatorURL:   s.AgentValidatorURL,
		RemoteMCPDisabled:   s.RemoteMCPDisabled,
		DisabledSkills:      s.DisabledSkills,
		SkillsConfigBase:    s.SkillsConfigBase,
		ShowToolSteps:       s.ShowToolSteps,
		AgentRunLogDisabled: s.AgentRunLogDisabled,
		PhoenixOTLPURL:      s.PhoenixOTLPURL,
	}
}

// AgentDefaultSkillsConfigBase returns the OS default config root for skills.
func (a *App) AgentDefaultSkillsConfigBase() (string, error) {
	return skills.DefaultSkillsConfigBase()
}

// AgentListSkills returns bundled and user skills with enable flags from prefs.
func (a *App) AgentListSkills() ([]SkillSetting, error) {
	return skills.ListSettings()
}

// AgentSetSettings persists assistant settings.
func (a *App) AgentSetSettings(s AgentSettings) {
	a.store.SetAgentSettings(prefs.AgentSettings{
		ChainID:             s.ChainID,
		LLMAPIKey:           s.LLMAPIKey,
		LLMBaseURL:          s.LLMBaseURL,
		LLMModel:            s.LLMModel,
		LLMProvider:         s.LLMProvider,
		LLMContextWindow:    s.LLMContextWindow,
		RemoteMCPURL:        s.RemoteMCPURL,
		AgentMarketURL:      s.AgentMarketURL,
		AgentValidatorURL:   s.AgentValidatorURL,
		RemoteMCPDisabled:   s.RemoteMCPDisabled,
		DisabledSkills:      s.DisabledSkills,
		SkillsConfigBase:    s.SkillsConfigBase,
		ShowToolSteps:       s.ShowToolSteps,
		AgentRunLogDisabled: s.AgentRunLogDisabled,
		PhoenixOTLPURL:      s.PhoenixOTLPURL,
	})
	skills.ApplySkillsConfigBase(s.SkillsConfigBase)
}

// AgentDefaultRemoteURL returns the production remote MCP endpoint.
func (a *App) AgentDefaultRemoteURL() string {
	return manage.RemoteMCPURL
}

// resolveRemoteURL turns the saved settings into the URL the runner should
// use, where empty means "do not connect at all".
//
// The disable flag exists because an empty URL field already means "use the
// default endpoint" — without it there is no way to express "none" in the UI.
func resolveRemoteURL(s AgentSettings) string {
	if s.RemoteMCPDisabled {
		return ""
	}
	if strings.TrimSpace(s.RemoteMCPURL) == "" {
		return manage.RemoteMCPURL
	}
	return s.RemoteMCPURL
}

var agentMu sync.Mutex
var agentCancel context.CancelFunc

// SettlementTask identifies one selected agent's quoted execution. Owner and
// Amount come from Agent Market; intent/task IDs are generated locally when
// the user starts the paid run.
type SettlementTask struct {
	// IntentID and TaskID are populated during funding and then used only by the
	// in-process validator assignment/reporting path.
	IntentID string `json:"intent_id"`
	TaskID   string `json:"task_id"`
	AgentID  string `json:"agent_id"`
	Endpoint string `json:"endpoint"`
	Amount   string `json:"amount"`
	Owner    string `json:"owner"`
	Source   string `json:"source"`
}

// SettlementNetworkConfig is the validator-resolved network payment setup.
type SettlementNetworkConfig = agentsettlement.NetworkConfig

// AgentSettlementNetworkConfig returns the deployed settlement contract and
// its payment token. Agent owner and advertised amount come from Agent Market.
func (a *App) AgentSettlementNetworkConfig() (SettlementNetworkConfig, error) {
	validatorURL := resolveSettlementValidatorURL(a.AgentGetSettings())
	client, err := agentsettlement.NewClient(validatorURL, "")
	if err != nil {
		return SettlementNetworkConfig{}, err
	}
	return client.NetworkConfig(a.ctx)
}

func emitAgentStep(ctx context.Context, step agent.Step) {
	detail := step.Detail
	if detail != "" {
		detail = i18n.LocalizeDetail(detail)
	}
	wruntime.EventsEmit(ctx, "agent:step", map[string]string{
		"kind":   string(step.Kind),
		"title":  i18n.LocalizeStepTitle(step.Title),
		"detail": detail,
	})
}

func emitAgentError(ctx context.Context, err error) {
	wruntime.EventsEmit(ctx, "agent:error", map[string]string{"error": i18n.Localize(err)})
}

func emitAgentDelta(ctx context.Context, text string) {
	wruntime.EventsEmit(ctx, "agent:delta", map[string]string{"text": text})
}

// AgentSend starts processing a user message asynchronously.
// Progress is emitted on "agent:step"; completion on "agent:done" or "agent:error".
func (a *App) AgentSend(chainID, message string) error {
	return a.agentSend(chainID, message, nil)
}

// AgentSendSettlement escrows the selected agent's quoted payment, asks the
// validator to assign it, then runs the request. On completion it reports the
// one execution transaction observed to that validator. The validator URL
// defaults to the local deployment and may be overridden by
// AGENT_VALIDATOR_URL; no shared validator callback token is carried by this
// open-source client.
func (a *App) AgentSendSettlement(chainID, message string, task SettlementTask) error {
	validatorURL := resolveSettlementValidatorURL(a.AgentGetSettings())
	client, err := agentsettlement.NewClient(validatorURL, "")
	if err != nil {
		return err
	}
	return a.agentSend(chainID, message, &settlementRun{task: task, client: client, validatorURL: validatorURL})
}

type settlementRun struct {
	task         SettlementTask
	client       *agentsettlement.Client
	validatorURL string
	config       *agentsettlement.Config
}

func (a *App) agentSend(chainID, message string, settlement *settlementRun) error {
	agentMu.Lock()
	if agentCancel != nil {
		agentMu.Unlock()
		return localized(i18n.ErrAgentBusy)
	}
	agentMu.Unlock()

	settings := a.AgentGetSettings()
	if chainID == "" {
		chainID = settings.ChainID
	}
	chainID = strings.TrimSpace(chainID)
	message = strings.TrimSpace(message)
	if chainID == "" {
		return localized(i18n.ErrChainIDRequired)
	}
	if message == "" {
		return localized(i18n.ErrMessageRequired)
	}
	if strings.TrimSpace(settings.LLMAPIKey) == "" {
		return localized(i18n.ErrLLMKeyRequired)
	}

	// Remote MCP is temporarily disabled at the GUI entry point. Keep the
	// configured endpoint and resolver intact so re-enabling only requires
	// restoring the line below.
	remoteURL := ""
	// remoteURL := resolveRemoteURL(settings)
	validatorURL := resolveSettlementValidatorURL(settings)

	agentMu.Lock()
	ctx, cancel := context.WithCancel(a.ctx)
	agentCancel = cancel
	agentMu.Unlock()

	emitAgentStep(a.ctx, agent.Step{Kind: agent.StepThink, Title: "Starting assistant…"})

	go func() {
		defer func() {
			agentMu.Lock()
			agentCancel = nil
			agentMu.Unlock()
			if r := recover(); r != nil {
				emitAgentError(a.ctx, fmt.Errorf("internal error: %v", r))
			}
		}()

		llmCfg := agent.LLMConfig{
			APIKey:   settings.LLMAPIKey,
			BaseURL:  settings.LLMBaseURL,
			Model:    settings.LLMModel,
			Provider: settings.LLMProvider,
		}

		hist := history.Shared()
		sess, prior := prepareHistory(ctx, hist, chainID, settings.LLMContextWindow, llmCfg, func(step agent.Step) {
			emitAgentStep(a.ctx, step)
		})

		if settlement != nil {
			if err := a.fundSettlement(ctx, chainID, settlement); err != nil {
				emitAgentError(a.ctx, err)
				return
			}
			if _, err := settlement.client.CreateTask(ctx, agentsettlement.Task{
				IntentID: settlement.task.IntentID,
				TaskID:   settlement.task.TaskID,
				Amount:   settlement.task.Amount,
				Owner:    settlement.task.Owner,
			}); err != nil {
				emitAgentError(a.ctx, fmt.Errorf("assign settlement task: %w", err))
				return
			}
		}

		answer, err := agent.Run(ctx, agent.Config{
			ChainID:                chainID,
			RemoteURL:              remoteURL,
			AgentMarketURL:         settings.AgentMarketURL,
			ChainRPCURL:            chainrpc.URLForChain(chainID),
			SettlementValidatorURL: validatorURL,
			SettlementRPCURL:       settlementRPCURL(),
			Confirm:                a.confirmHook,
			RunLog:                 runlog.New(!settings.AgentRunLogDisabled),
			PhoenixOTLPURL:         settings.PhoenixOTLPURL,
			LLM:                    llmCfg,
			Prior:                  prior,
			SessionID:              sess.ID,
			SessionTitle:           sess.Title,
			AttachedAgentURL:       settlementAgentURL(sess.AttachedAgentURL, settlement),
			AttachedAgentTools:     sess.AttachedAgentTools,
			OnAttach: func(url string, tools []string) {
				if sess.ID != "" {
					_ = hist.SetAttachedAgent(sess.ID, url, tools)
				}
			},
			OnTranscript: func(runID string, msgs []llm.Message) {
				if sess.ID != "" {
					_ = hist.Append(sess.ID, runID, msgs)
				}
			},
			OnStep: func(step agent.Step) {
				emitAgentStep(a.ctx, step)
			},
			OnDelta: func(text string) {
				emitAgentDelta(a.ctx, text)
			},
			Settlement: settlementConfig(settlement),
		}, message)

		if err != nil {
			emitAgentError(a.ctx, err)
			return
		}
		answer = i18n.LocalizeAgentAnswer(answer)
		// The step channel is reliably consumed while a streaming answer is
		// displayed. It provides a terminal fallback if Wails misses agent:done.
		emitAgentStep(a.ctx, agent.Step{Kind: agent.StepAnswer, Title: "Completed", Detail: answer})
		wruntime.EventsEmit(a.ctx, "agent:done", map[string]string{"answer": answer})
	}()
	return nil
}

func settlementConfig(settlement *settlementRun) *agentsettlement.Config {
	if settlement == nil {
		return nil
	}
	return settlement.config
}

func settlementAgentURL(sessionURL string, settlement *settlementRun) string {
	if settlement != nil && strings.TrimSpace(settlement.task.Endpoint) != "" {
		return strings.TrimSpace(settlement.task.Endpoint)
	}
	return sessionURL
}

// fundSettlement derives the user's EVM account, reads the payment network
// from validator, confirms approve+deposit locally, and fills the generated
// task identifiers used by assignment and execution reporting.
func (a *App) fundSettlement(ctx context.Context, chainID string, settlement *settlementRun) error {
	if settlement == nil || settlement.client == nil {
		return fmt.Errorf("settlement run is not configured")
	}
	if err := a.resolveSettlementAgent(ctx, settlement); err != nil {
		return err
	}
	owner, err := agentsettlement.EVMOwner(settlement.task.Owner)
	if err != nil {
		return err
	}
	network, err := settlement.client.NetworkConfig(ctx)
	if err != nil {
		return err
	}
	ring, err := keystore.Open()
	if err != nil {
		return err
	}
	hexKey, _, err := manage.SelectKey(ring, chainID, os.Getenv("SIGNER_KEY_HEX"))
	if err != nil {
		return err
	}
	priv, err := signer.ParsePrivKey(hexKey)
	if err != nil {
		return fmt.Errorf("parse key: %w", err)
	}
	emitAgentStep(a.ctx, agent.Step{Kind: agent.StepThink, Title: "Funding agent settlement…"})
	fundingCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	funded, err := agentsettlement.Fund(fundingCtx, agentsettlement.FundingConfig{
		ChainID:            chainID,
		RPCURL:             settlementRPCURL(),
		SettlementContract: network.SettlementContract,
		PaymentToken:       network.PaymentToken,
		Amount:             settlement.task.Amount,
		PrivateKey:         priv,
		Confirm:            a.confirmHook,
	})
	if err != nil {
		return fmt.Errorf("fund agent settlement: %w", err)
	}
	settlement.task.IntentID = funded.IntentID
	settlement.task.TaskID = funded.TaskID
	settlement.task.Owner = owner.Hex()
	if strings.TrimSpace(settlement.task.Source) == "" {
		settlement.task.Source = agentsettlement.SourceEVM
	}
	settlement.config = &agentsettlement.Config{
		TaskID:       funded.TaskID,
		Owner:        owner.Hex(),
		ValidatorURL: settlement.validatorURL,
		Source:       settlement.task.Source,
	}
	if _, err := agentsettlement.New(*settlement.config); err != nil {
		return err
	}
	emitAgentStep(a.ctx, agent.Step{Kind: agent.StepThink, Title: "Agent payment funded", Detail: funded.DepositTxHash})
	return nil
}

func (a *App) resolveSettlementAgent(ctx context.Context, settlement *settlementRun) error {
	if settlement == nil {
		return fmt.Errorf("settlement run is not configured")
	}
	if strings.TrimSpace(settlement.task.AgentID) != "" {
		market := agentmarket.New(a.AgentGetSettings().AgentMarketURL)
		hit, err := market.Get(ctx, settlement.task.AgentID)
		if err != nil {
			return err
		}
		if hit.Status != "AGENT_STATUS_ACTIVE" {
			return fmt.Errorf("selected agent %q is not active", settlement.task.AgentID)
		}
		if strings.TrimSpace(hit.Owner) == "" || strings.TrimSpace(hit.Pricing.Amount) == "" {
			return fmt.Errorf("selected agent %q has incomplete owner or pricing data", settlement.task.AgentID)
		}
		settlement.task.Owner = hit.Owner
		settlement.task.Amount = hit.Pricing.Amount
		settlement.task.Endpoint = hit.Endpoint
	}
	if strings.TrimSpace(settlement.task.Owner) == "" {
		return fmt.Errorf("selected agent has no owner address")
	}
	if strings.TrimSpace(settlement.task.Amount) == "" {
		return fmt.Errorf("selected agent has no quoted settlement amount")
	}
	return nil
}

func settlementRPCURL() string {
	if value := strings.TrimSpace(os.Getenv("AGENT_SETTLEMENT_RPC_URL")); value != "" {
		return value
	}
	return "https://svp-devnet.svpchain.org/evm/"
}

func resolveSettlementValidatorURL(s AgentSettings) string {
	if value := strings.TrimSpace(os.Getenv("AGENT_VALIDATOR_URL")); value != "" {
		return value
	}
	if value := strings.TrimSpace(s.AgentValidatorURL); value != "" {
		return value
	}
	return "https://agent-validator-devnet.svpstars.com"
}

// prepareHistory resolves the active session (creating one on first use or
// chain switch), compacts it when over the context budget, and loads prior
// turns for the next run. Failures degrade to a stateless single-turn run.
func prepareHistory(ctx context.Context, hist *history.Store, chainID string, contextWindow int, llmCfg agent.LLMConfig, emit func(agent.Step)) (history.SessionInfo, []llm.Message) {
	if !hist.Enabled() {
		return history.SessionInfo{}, nil
	}
	sess, ok := hist.Current()
	if !ok || sess.ChainID != chainID {
		created, err := hist.Create(chainID)
		if err != nil {
			return history.SessionInfo{}, nil
		}
		return created, nil
	}

	budget := history.ContextBudgetTokens(contextWindow)
	summarize := func(sctx context.Context, text string) (string, error) {
		client := llm.NewClient(llmCfg)
		res, err := client.Chat(sctx, []llm.Message{
			{Role: "system", Content: history.SummarySystemPrompt},
			{Role: "user", Content: text},
		}, nil, nil)
		if err != nil {
			return "", err
		}
		return res.Message.Content, nil
	}
	if compacted, err := hist.CompactIfNeeded(ctx, sess.ID, budget, summarize); err == nil && compacted {
		emit(agent.Step{Kind: agent.StepThink, Title: "Compacted conversation history"})
	}

	prior, err := hist.Context(sess.ID)
	if err != nil {
		return sess, nil
	}
	return sess, history.RepairPairing(prior)
}

// AgentCancel stops the in-flight assistant run, if any.
func (a *App) AgentCancel() {
	agentMu.Lock()
	cancel := agentCancel
	agentMu.Unlock()
	if cancel != nil {
		cancel()
	}
}
