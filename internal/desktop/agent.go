package desktop

import (
	"context"
	"fmt"
	"strings"
	"sync"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/svpchain/svpchain-agent/internal/agent"
	"github.com/svpchain/svpchain-agent/internal/agent/history"
	"github.com/svpchain/svpchain-agent/internal/agent/llm"
	"github.com/svpchain/svpchain-agent/internal/agent/runlog"
	"github.com/svpchain/svpchain-agent/internal/agent/skills"
	"github.com/svpchain/svpchain-agent/internal/i18n"
	"github.com/svpchain/svpchain-agent/internal/manage"
	"github.com/svpchain/svpchain-agent/internal/prefs"
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
	DisabledSkills      []string `json:"disabled_skills"`
	SkillsConfigBase    string   `json:"skills_config_base"`
	ShowToolSteps       bool     `json:"show_tool_steps"`
	AgentRunLogDisabled bool     `json:"agent_run_log_disabled"`
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
		DisabledSkills:      s.DisabledSkills,
		SkillsConfigBase:    s.SkillsConfigBase,
		ShowToolSteps:       s.ShowToolSteps,
		AgentRunLogDisabled: s.AgentRunLogDisabled,
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
		DisabledSkills:      s.DisabledSkills,
		SkillsConfigBase:    s.SkillsConfigBase,
		ShowToolSteps:       s.ShowToolSteps,
		AgentRunLogDisabled: s.AgentRunLogDisabled,
	})
	skills.ApplySkillsConfigBase(s.SkillsConfigBase)
}

// AgentDefaultRemoteURL returns the production remote MCP endpoint.
func (a *App) AgentDefaultRemoteURL() string {
	return manage.RemoteMCPURL
}

var agentMu sync.Mutex
var agentCancel context.CancelFunc

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

	remoteURL := settings.RemoteMCPURL
	if remoteURL == "" {
		remoteURL = manage.RemoteMCPURL
	}

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

		answer, err := agent.Run(ctx, agent.Config{
			ChainID:   chainID,
			RemoteURL: remoteURL,
			RunLog:    runlog.New(!settings.AgentRunLogDisabled),
			LLM:       llmCfg,
			Prior:     prior,
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
		}, message)

		if err != nil {
			emitAgentError(a.ctx, err)
			return
		}
		answer = i18n.LocalizeAgentAnswer(answer)
		wruntime.EventsEmit(a.ctx, "agent:done", map[string]string{"answer": answer})
	}()
	return nil
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
