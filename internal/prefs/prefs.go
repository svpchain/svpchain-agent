package prefs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

const configDirName = "com.svpchain.agent-gui"
const prefsFileName = "prefs.json"

var pathOverride string

// SetPathOverride redirects prefs.json loading for tests.
func SetPathOverride(path string) {
	pathOverride = path
}

// Path returns the application prefs.json path.
func Path() string {
	if pathOverride != "" {
		return pathOverride
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, configDirName, prefsFileName)
}

// File is the on-disk prefs.json schema.
type File struct {
	Language         string           `json:"language,omitempty"`
	SkipVersion      string           `json:"skip_version,omitempty"`
	AgentChainID     string           `json:"agent_chain_id,omitempty"`
	LLMAPIKey        string           `json:"llm_api_key,omitempty"`
	LLMBaseURL       string           `json:"llm_base_url,omitempty"`
	LLMModel         string           `json:"llm_model,omitempty"`
	LLMProvider      string           `json:"llm_provider,omitempty"`
	RemoteMCPURL     string           `json:"remote_mcp_url,omitempty"`
	Whitelist        []WhitelistEntry `json:"whitelist,omitempty"`
	DisabledSkills   []string         `json:"disabled_skills,omitempty"`
	SkillsConfigBase string           `json:"skills_config_base,omitempty"`
}

// WhitelistEntry is one persisted whitelist row in prefs.json.
type WhitelistEntry struct {
	ChainID     string `json:"chain_id"`
	AddressType string `json:"address_type"`
	Address     string `json:"address"`
	Alias       string `json:"alias,omitempty"`
}

// Store holds prefs in memory for the GUI and persists on change.
type Store struct {
	mu   sync.Mutex
	path string
	data File
}

// Read loads prefs.json from disk without retaining an in-memory store.
func Read() File {
	path := Path()
	if path == "" {
		return File{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return File{}
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return File{}
	}
	return f
}

// Load opens or creates the in-memory prefs store for the GUI.
func Load() *Store {
	path := Path()
	s := &Store{path: path}
	if path == "" {
		return s
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	_ = json.Unmarshal(data, &s.data)
	return s
}

// File returns a snapshot of the current in-memory prefs.
func (s *Store) File() File {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneFile(s.data)
}

// Update mutates in-memory prefs and writes prefs.json.
func (s *Store) Update(fn func(*File)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.data)
	s.saveLocked()
}

// UpdateErr mutates prefs and persists when fn returns nil.
func (s *Store) UpdateErr(fn func(*File) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := fn(&s.data); err != nil {
		return err
	}
	return s.saveLocked()
}

func (s *Store) saveLocked() error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

func cloneFile(f File) File {
	out := f
	if len(f.Whitelist) > 0 {
		out.Whitelist = append([]WhitelistEntry(nil), f.Whitelist...)
	}
	if len(f.DisabledSkills) > 0 {
		out.DisabledSkills = append([]string(nil), f.DisabledSkills...)
	}
	return out
}
