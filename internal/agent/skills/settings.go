package skills

import (
	"sort"

	"github.com/svpchain/svpchain-agent/internal/prefs"
)

// Setting is one skill row for the Settings UI.
type Setting struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	Locked      bool   `json:"locked"`
	Source      string `json:"source"`
}

var disabledSkillsOverride *[]string

// SetDisabledSkillsOverride redirects disabled-skill loading for tests.
func SetDisabledSkillsOverride(names []string) {
	cp := append([]string(nil), names...)
	disabledSkillsOverride = &cp
}

// ClearDisabledSkillsOverride restores loading from prefs.json.
func ClearDisabledSkillsOverride() {
	disabledSkillsOverride = nil
}

// ListSettings returns all skills with enabled state from prefs.json.
func ListSettings() ([]Setting, error) {
	all, err := LoadAll()
	if err != nil {
		return nil, err
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Priority != all[j].Priority {
			return all[i].Priority < all[j].Priority
		}
		return all[i].Name < all[j].Name
	})
	disabled := disabledSet()
	out := make([]Setting, 0, len(all))
	for _, s := range all {
		_, off := disabled[s.Name]
		enabled := !off
		if s.Locked {
			enabled = true
		}
		out = append(out, Setting{
			Name:        s.Name,
			Description: s.Description,
			Enabled:     enabled,
			Locked:      s.Locked,
			Source:      s.Source,
		})
	}
	return out, nil
}

func disabledSet() map[string]struct{} {
	names := loadDisabledSkillNames()
	set := make(map[string]struct{}, len(names))
	for _, n := range names {
		if n != "" {
			set[n] = struct{}{}
		}
	}
	return set
}

func loadDisabledSkillNames() []string {
	if disabledSkillsOverride != nil {
		return *disabledSkillsOverride
	}
	return prefs.Read().DisabledSkills
}

func filterDisabled(skills []Skill, disabled map[string]struct{}) []Skill {
	if len(disabled) == 0 {
		return skills
	}
	out := make([]Skill, 0, len(skills))
	for _, s := range skills {
		if s.Locked {
			out = append(out, s)
			continue
		}
		if _, off := disabled[s.Name]; off {
			continue
		}
		out = append(out, s)
	}
	return out
}
