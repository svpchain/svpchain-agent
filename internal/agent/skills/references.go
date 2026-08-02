package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Reference files implement Claude Code style progressive disclosure: SKILL.md
// stays lean and points the model at bundled/<skill>/references/*.md, which are
// loaded on demand through the read_skill_reference local tool.

const referencesSubdir = "references"

var (
	refSkillNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	refFileNameRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*\.md$`)
)

// ReadReference returns the contents of one reference file for a skill.
// User skills override bundled ones (same lookup order as SKILL.md itself).
// Both names are strictly validated so the LLM cannot traverse the filesystem.
func ReadReference(skillName, fileName string) (string, error) {
	skillName = strings.TrimSpace(skillName)
	fileName = strings.TrimSpace(fileName)
	if !refSkillNameRe.MatchString(skillName) {
		return "", fmt.Errorf("invalid skill name %q", skillName)
	}
	if !refFileNameRe.MatchString(fileName) || strings.Contains(fileName, "..") {
		return "", fmt.Errorf("invalid reference file name %q", fileName)
	}
	if dir := UserSkillsDir(); dir != "" {
		p := filepath.Join(dir, skillName, referencesSubdir, fileName)
		if data, err := os.ReadFile(p); err == nil {
			return string(data), nil
		}
	}
	data, err := bundled.ReadFile(path.Join("bundled", skillName, referencesSubdir, fileName))
	if err == nil {
		return string(data), nil
	}
	available := ListReferences(skillName)
	if len(available) == 0 {
		return "", fmt.Errorf("skill %q has no reference files", skillName)
	}
	return "", fmt.Errorf("reference %q not found for skill %q; available: %s",
		fileName, skillName, strings.Join(available, ", "))
}

// ReferenceToolName is the local tool the LLM calls to load reference files.
const ReferenceToolName = "read_skill_reference"

// ReadReferenceFromArgs adapts ReadReference to LLM tool-call arguments.
func ReadReferenceFromArgs(args map[string]any) (string, error) {
	skill, _ := args["skill"].(string)
	file, _ := args["file"].(string)
	return ReadReference(skill, file)
}

// ListReferences returns the reference file names available for a skill,
// merging bundled and user directories (sorted, deduplicated).
func ListReferences(skillName string) []string {
	if !refSkillNameRe.MatchString(strings.TrimSpace(skillName)) {
		return nil
	}
	seen := map[string]bool{}
	if entries, err := fs.ReadDir(bundled, path.Join("bundled", skillName, referencesSubdir)); err == nil {
		for _, ent := range entries {
			if !ent.IsDir() && strings.HasSuffix(ent.Name(), ".md") {
				seen[ent.Name()] = true
			}
		}
	}
	if dir := UserSkillsDir(); dir != "" {
		if entries, err := os.ReadDir(filepath.Join(dir, skillName, referencesSubdir)); err == nil {
			for _, ent := range entries {
				if !ent.IsDir() && strings.HasSuffix(ent.Name(), ".md") {
					seen[ent.Name()] = true
				}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
