package desktop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/svpchain/svpchain-agent/internal/whitelist"
)

// prefs is a tiny JSON-backed preferences store replacing Fyne's app preferences.
type prefs struct {
	mu          sync.Mutex
	path        string
	Language    string `json:"language,omitempty"`
	SkipVersion string `json:"skip_version,omitempty"`

	// Agent assistant settings.
	AgentChainID   string            `json:"agent_chain_id,omitempty"`
	LLMAPIKey      string            `json:"llm_api_key,omitempty"`
	LLMBaseURL     string            `json:"llm_base_url,omitempty"`
	LLMModel       string            `json:"llm_model,omitempty"`
	RemoteMCPURL   string            `json:"remote_mcp_url,omitempty"`
	Whitelist      []whitelist.Entry `json:"whitelist,omitempty"`
	DisabledSkills []string          `json:"disabled_skills,omitempty"`
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
	return whitelist.PrefsPath()
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

func (p *prefs) whitelistStore() *whitelist.Store {
	return whitelist.NewStore(p.Whitelist)
}

func (p *prefs) listWhitelist() []whitelist.Entry {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.whitelistStore().List()
}

func (p *prefs) addWhitelist(chainID, addressType, address string) (whitelist.Entry, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	store := p.whitelistStore()
	entry, err := store.Add(chainID, addressType, address)
	if err != nil {
		return whitelist.Entry{}, err
	}
	p.Whitelist = store.List()
	p.save()
	return entry, nil
}

func (p *prefs) deleteWhitelist(chainID, addressType, address string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	store := p.whitelistStore()
	if err := store.Delete(chainID, addressType, address); err != nil {
		return err
	}
	p.Whitelist = store.List()
	p.save()
	return nil
}

func (p *prefs) setAgentSettings(s AgentSettings) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.AgentChainID = s.ChainID
	p.LLMAPIKey = s.LLMAPIKey
	p.LLMBaseURL = s.LLMBaseURL
	p.LLMModel = s.LLMModel
	p.RemoteMCPURL = s.RemoteMCPURL
	p.DisabledSkills = append([]string(nil), s.DisabledSkills...)
	p.save()
}
