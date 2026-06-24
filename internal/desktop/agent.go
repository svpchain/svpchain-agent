package desktop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/svpchain/svpchain-agent/internal/agent"
	"github.com/svpchain/svpchain-agent/internal/agent/skills"
	"github.com/svpchain/svpchain-agent/internal/manage"
	"github.com/svpchain/svpchain-agent/internal/prefs"
)

var errAgentBusy = errors.New("assistant is already running")

// SkillSetting is one assistant skill row for the Settings UI.
type SkillSetting = skills.Setting

// AgentSettings is persisted LLM / MCP configuration for the assistant tab.
type AgentSettings struct {
	ChainID          string   `json:"chain_id"`
	LLMAPIKey        string   `json:"llm_api_key"`
	LLMBaseURL       string   `json:"llm_base_url"`
	LLMModel         string   `json:"llm_model"`
	RemoteMCPURL     string   `json:"remote_mcp_url"`
	DisabledSkills   []string `json:"disabled_skills"`
	SkillsConfigBase string   `json:"skills_config_base"`
}

// AgentGetSettings returns saved assistant settings (API key included for local use only).
func (a *App) AgentGetSettings() AgentSettings {
	s := a.store.AgentSettings()
	return AgentSettings{
		ChainID:          s.ChainID,
		LLMAPIKey:        s.LLMAPIKey,
		LLMBaseURL:       s.LLMBaseURL,
		LLMModel:         s.LLMModel,
		RemoteMCPURL:     s.RemoteMCPURL,
		DisabledSkills:   s.DisabledSkills,
		SkillsConfigBase: s.SkillsConfigBase,
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
		ChainID:          s.ChainID,
		LLMAPIKey:        s.LLMAPIKey,
		LLMBaseURL:       s.LLMBaseURL,
		LLMModel:         s.LLMModel,
		RemoteMCPURL:     s.RemoteMCPURL,
		DisabledSkills:   s.DisabledSkills,
		SkillsConfigBase: s.SkillsConfigBase,
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
	wruntime.EventsEmit(ctx, "agent:step", map[string]string{
		"kind":   string(step.Kind),
		"title":  step.Title,
		"detail": step.Detail,
	})
}

func emitAgentError(ctx context.Context, err error) {
	wruntime.EventsEmit(ctx, "agent:error", map[string]string{"error": err.Error()})
}

// AgentSend starts processing a user message asynchronously.
// Progress is emitted on "agent:step"; completion on "agent:done" or "agent:error".
func (a *App) AgentSend(chainID, message string) error {
	agentMu.Lock()
	if agentCancel != nil {
		agentMu.Unlock()
		return errAgentBusy
	}
	agentMu.Unlock()

	settings := a.AgentGetSettings()
	if chainID == "" {
		chainID = settings.ChainID
	}
	chainID = strings.TrimSpace(chainID)
	message = strings.TrimSpace(message)
	if chainID == "" {
		return fmt.Errorf("chain id is required — select one in Settings or the Assistant tab")
	}
	if message == "" {
		return fmt.Errorf("message is required")
	}
	if strings.TrimSpace(settings.LLMAPIKey) == "" {
		return fmt.Errorf("LLM API key is not configured — open Settings, enter your key, and click Save")
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

		answer, err := agent.Run(ctx, agent.Config{
			ChainID:   chainID,
			RemoteURL: remoteURL,
			LLM: agent.LLMConfig{
				APIKey:  settings.LLMAPIKey,
				BaseURL: settings.LLMBaseURL,
				Model:   settings.LLMModel,
			},
			OnStep: func(step agent.Step) {
				emitAgentStep(a.ctx, step)
			},
		}, message)

		if err != nil {
			emitAgentError(a.ctx, err)
			return
		}
		wruntime.EventsEmit(a.ctx, "agent:done", map[string]string{"answer": answer})
	}()
	return nil
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
