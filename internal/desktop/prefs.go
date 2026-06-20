package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// prefs is a tiny JSON-backed preferences store replacing Fyne's app preferences.
type prefs struct {
	mu          sync.Mutex
	path        string
	Language    string `json:"language,omitempty"`
	SkipVersion string `json:"skip_version,omitempty"`

	// Agent assistant settings.
	AgentChainID string `json:"agent_chain_id,omitempty"`
	LLMAPIKey    string `json:"llm_api_key,omitempty"`
	LLMBaseURL   string `json:"llm_base_url,omitempty"`
	LLMModel     string `json:"llm_model,omitempty"`
	RemoteMCPURL string `json:"remote_mcp_url,omitempty"`
}

func loadPrefs() *prefs {
	p := &prefs{path: prefsPath()}
	if p.path == "" {
		return p
	}
	data, err := os.ReadFile(p.path)
	if err != nil {
		return p
	}
	_ = json.Unmarshal(data, p)
	return p
}

func prefsPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "com.svpchain.agent-gui", "prefs.json")
}

func (p *prefs) save() {
	if p.path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(p.path), 0o755); err != nil {
		return
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(p.path, data, 0o600)
}

func (p *prefs) setLanguage(lang string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Language = lang
	p.save()
}

func (p *prefs) setSkipVersion(tag string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.SkipVersion = tag
	p.save()
}

func (p *prefs) setAgentSettings(s AgentSettings) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.AgentChainID = s.ChainID
	p.LLMAPIKey = s.LLMAPIKey
	p.LLMBaseURL = s.LLMBaseURL
	p.LLMModel = s.LLMModel
	p.RemoteMCPURL = s.RemoteMCPURL
	p.save()
}
