package skills

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed bundled/*/SKILL.md bundled/*/references/*.md
var bundled embed.FS

const configDirName = "com.svpchain.agent-gui"
const skillsSubdir = "skills"

// Skill is one LLM instruction module loaded from SKILL.md.
type Skill struct {
	Name        string
	Description string
	Tools       []string
	Priority    int
	Locked      bool
	Body        string
	Source      string // "bundled" or "user"
}

type skillMeta struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Tools       []string `yaml:"tools"`
	Priority    int      `yaml:"priority"`
	Locked      bool     `yaml:"locked"`
}

var (
	skillsDirOverride  string  // full skills dir; SetSkillsDirOverride (tests)
	appliedConfigBase  string  // config root from prefs/UI; ApplySkillsConfigBase
	skillsBaseOverride *string // optional config root for tests
)

// SetSkillsDirOverride redirects user skill loading to dir (full skills path; tests).
func SetSkillsDirOverride(dir string) {
	skillsDirOverride = dir
}

// SetSkillsConfigBaseOverride redirects the config root for tests.
func SetSkillsConfigBaseOverride(base string) {
	cp := base
	skillsBaseOverride = &cp
}

// ClearSkillsConfigBaseOverride restores config-root loading from prefs/default.
func ClearSkillsConfigBaseOverride() {
	skillsBaseOverride = nil
}

// ApplySkillsConfigBase sets the runtime config root from saved preferences.
// Pass an empty string to use the OS default (os.UserConfigDir()).
func ApplySkillsConfigBase(base string) {
	appliedConfigBase = strings.TrimSpace(base)
}

// DefaultSkillsConfigBase returns the OS user config directory.
func DefaultSkillsConfigBase() (string, error) {
	return os.UserConfigDir()
}

// ResolveUserSkillsDir returns the skills directory for a config root.
// An empty base uses DefaultSkillsConfigBase().
func ResolveUserSkillsDir(base string) (string, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		var err error
		base, err = os.UserConfigDir()
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(base, configDirName, skillsSubdir), nil
}

// UserSkillsDir returns the directory for optional user-provided skills.
func UserSkillsDir() string {
	if skillsDirOverride != "" {
		return skillsDirOverride
	}
	base := skillsConfigBase()
	if base == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return ""
		}
		base = dir
	}
	return filepath.Join(base, configDirName, skillsSubdir)
}

func skillsConfigBase() string {
	if skillsBaseOverride != nil {
		return strings.TrimSpace(*skillsBaseOverride)
	}
	if appliedConfigBase != "" {
		return appliedConfigBase
	}
	return loadSkillsConfigBaseFromPrefs()
}

// LoadAll returns bundled skills plus any user skills (user overrides same name).
func LoadAll() ([]Skill, error) {
	bundledSkills, err := loadBundled()
	if err != nil {
		return nil, err
	}
	userSkills, err := loadUser(UserSkillsDir())
	if err != nil {
		return nil, err
	}
	return mergeSkills(bundledSkills, userSkills), nil
}

// ComposeSystemPrompt builds the LLM system message from loaded skills.
// When toolNames is non-empty, skills that declare tools are included only when
// at least one pattern matches an available tool. Skills with no tools field
// are always included.
func ComposeSystemPrompt(toolNames []string) (string, error) {
	all, err := LoadAll()
	if err != nil {
		return "", err
	}
	selected := filterByTools(all, toolNames)
	selected = filterDisabled(selected, disabledSet())
	sort.SliceStable(selected, func(i, j int) bool {
		if selected[i].Priority != selected[j].Priority {
			return selected[i].Priority < selected[j].Priority
		}
		return selected[i].Name < selected[j].Name
	})
	parts := make([]string, 0, len(selected))
	for _, s := range selected {
		body := strings.TrimSpace(s.Body)
		if body != "" {
			parts = append(parts, body)
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("no skill content loaded")
	}
	return strings.Join(parts, "\n\n"), nil
}

func loadBundled() ([]Skill, error) {
	var out []Skill
	err := fs.WalkDir(bundled, "bundled", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != "SKILL.md" {
			return nil
		}
		data, err := bundled.ReadFile(path)
		if err != nil {
			return err
		}
		s, err := parseSkillFile(string(data), "bundled")
		if err != nil {
			return fmt.Errorf("parse bundled %s: %w", path, err)
		}
		out = append(out, s)
		return nil
	})
	return out, err
}

func loadUser(dir string) ([]Skill, error) {
	if dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Skill
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		path := filepath.Join(dir, ent.Name(), "SKILL.md")
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		s, err := parseSkillFile(string(data), "user")
		if err != nil {
			return nil, fmt.Errorf("parse user %s: %w", path, err)
		}
		out = append(out, s)
	}
	return out, nil
}

func parseSkillFile(content, source string) (Skill, error) {
	meta, body, err := splitFrontmatter(content)
	if err != nil {
		return Skill{}, err
	}
	var m skillMeta
	if err := yaml.Unmarshal([]byte(meta), &m); err != nil {
		return Skill{}, fmt.Errorf("frontmatter: %w", err)
	}
	name := strings.TrimSpace(m.Name)
	if name == "" {
		return Skill{}, fmt.Errorf("skill name is required")
	}
	return Skill{
		Name:        name,
		Description: strings.TrimSpace(m.Description),
		Tools:       m.Tools,
		Priority:    m.Priority,
		Locked:      m.Locked,
		Body:        strings.TrimSpace(body),
		Source:      source,
	}, nil
}

func splitFrontmatter(content string) (meta, body string, err error) {
	s := strings.TrimPrefix(content, "\ufeff")
	if !strings.HasPrefix(s, "---") {
		return "", "", fmt.Errorf("missing YAML frontmatter")
	}
	rest := s[3:]
	rest = strings.TrimPrefix(rest, "\n")
	rest = strings.TrimPrefix(rest, "\r\n")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return "", "", fmt.Errorf("unclosed YAML frontmatter")
	}
	meta = rest[:end]
	body = strings.TrimSpace(rest[end+len("\n---"):])
	body = strings.TrimPrefix(body, "\n")
	body = strings.TrimPrefix(body, "\r\n")
	return meta, body, nil
}

func mergeSkills(bundledSkills, userSkills []Skill) []Skill {
	byName := make(map[string]Skill, len(bundledSkills)+len(userSkills))
	order := make([]string, 0, len(bundledSkills)+len(userSkills))
	for _, s := range bundledSkills {
		if _, ok := byName[s.Name]; !ok {
			order = append(order, s.Name)
		}
		byName[s.Name] = s
	}
	for _, s := range userSkills {
		if _, ok := byName[s.Name]; !ok {
			order = append(order, s.Name)
		}
		byName[s.Name] = s
	}
	out := make([]Skill, 0, len(order))
	for _, name := range order {
		out = append(out, byName[name])
	}
	return out
}

func filterByTools(skills []Skill, toolNames []string) []Skill {
	if len(toolNames) == 0 {
		return skills
	}
	out := make([]Skill, 0, len(skills))
	for _, s := range skills {
		if len(s.Tools) == 0 || matchesAnyTool(s.Tools, toolNames) {
			out = append(out, s)
		}
	}
	return out
}

func matchesAnyTool(patterns, toolNames []string) bool {
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		for _, name := range toolNames {
			if toolPatternMatch(pattern, name) {
				return true
			}
		}
	}
	return false
}

func toolPatternMatch(pattern, name string) bool {
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(name, strings.TrimSuffix(pattern, "*"))
	}
	return pattern == name
}

// MatchesToolPattern reports whether name satisfies pattern (supports trailing *).
func MatchesToolPattern(pattern, name string) bool {
	return toolPatternMatch(pattern, name)
}

// ParseSkillContent parses a SKILL.md file for tests.
func ParseSkillContent(content, source string) (Skill, error) {
	return parseSkillFile(content, source)
}
