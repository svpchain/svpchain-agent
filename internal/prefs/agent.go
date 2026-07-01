package prefs

import "strings"

// SetLanguage persists the UI language.
func (s *Store) SetLanguage(lang string) {
	s.Update(func(f *File) {
		f.Language = lang
	})
}

// SetSkipVersion records a release tag to skip for update prompts.
func (s *Store) SetSkipVersion(tag string) {
	s.Update(func(f *File) {
		f.SkipVersion = tag
	})
}

// AgentSettings is the assistant-related subset of prefs.
type AgentSettings struct {
	ChainID             string
	LLMAPIKey           string
	LLMBaseURL          string
	LLMModel            string
	LLMProvider         string
	RemoteMCPURL        string
	DisabledSkills      []string
	SkillsConfigBase    string
	ShowToolSteps       bool
	AgentRunLogDisabled bool
}

// AgentSettings returns the saved assistant configuration.
func (s *Store) AgentSettings() AgentSettings {
	f := s.File()
	return AgentSettings{
		ChainID:             f.AgentChainID,
		LLMAPIKey:           f.LLMAPIKey,
		LLMBaseURL:          f.LLMBaseURL,
		LLMModel:            f.LLMModel,
		LLMProvider:         f.LLMProvider,
		RemoteMCPURL:        f.RemoteMCPURL,
		DisabledSkills:      append([]string(nil), f.DisabledSkills...),
		SkillsConfigBase:    f.SkillsConfigBase,
		ShowToolSteps:       f.ShowToolSteps,
		AgentRunLogDisabled: f.AgentRunLogDisabled,
	}
}

// SetAgentSettings persists assistant configuration.
func (s *Store) SetAgentSettings(in AgentSettings) {
	s.Update(func(f *File) {
		f.AgentChainID = in.ChainID
		f.LLMAPIKey = in.LLMAPIKey
		f.LLMBaseURL = in.LLMBaseURL
		f.LLMModel = in.LLMModel
		f.LLMProvider = in.LLMProvider
		f.RemoteMCPURL = in.RemoteMCPURL
		f.DisabledSkills = append([]string(nil), in.DisabledSkills...)
		f.SkillsConfigBase = strings.TrimSpace(in.SkillsConfigBase)
		f.ShowToolSteps = in.ShowToolSteps
		f.AgentRunLogDisabled = in.AgentRunLogDisabled
	})
}
