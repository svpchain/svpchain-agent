package skills_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/agent/skills"
)

func TestReadReference_bundledLendora(t *testing.T) {
	got, err := skills.ReadReference("lendora-lending", "output-templates.md")
	require.NoError(t, err)
	require.Contains(t, got, "Output Templates")

	got, err = skills.ReadReference("lendora-lending", "error-responses.md")
	require.NoError(t, err)
	require.Contains(t, got, "Error Response Templates")
}

func TestReadReference_rejectsBadNames(t *testing.T) {
	cases := []struct{ skill, file string }{
		{"../base", "output-templates.md"},
		{"lendora-lending", "../../prefs.json"},
		{"lendora-lending", "output-templates.txt"},
		{"lendora-lending", ".hidden.md"},
		{"Lendora", "output-templates.md"},
		{"", "output-templates.md"},
		{"lendora-lending", ""},
		{"lendora-lending", "a/b.md"},
	}
	for _, c := range cases {
		_, err := skills.ReadReference(c.skill, c.file)
		require.Error(t, err, "skill=%q file=%q", c.skill, c.file)
	}
}

func TestReadReference_notFoundListsAvailable(t *testing.T) {
	_, err := skills.ReadReference("lendora-lending", "nope.md")
	require.Error(t, err)
	require.Contains(t, err.Error(), "output-templates.md")
	require.Contains(t, err.Error(), "error-responses.md")
}

func TestReadReference_noReferencesSkill(t *testing.T) {
	_, err := skills.ReadReference("base", "anything.md")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no reference files")
}

func TestReadReference_userOverride(t *testing.T) {
	dir := t.TempDir()
	refDir := filepath.Join(dir, "lendora-lending", "references")
	require.NoError(t, os.MkdirAll(refDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(refDir, "output-templates.md"), []byte("custom templates"), 0o600))
	t.Cleanup(func() { skills.SetSkillsDirOverride("") })
	skills.SetSkillsDirOverride(dir)

	got, err := skills.ReadReference("lendora-lending", "output-templates.md")
	require.NoError(t, err)
	require.Equal(t, "custom templates", got)
}

func TestListReferences(t *testing.T) {
	require.Equal(t, []string{"error-responses.md", "output-templates.md"}, skills.ListReferences("lendora-lending"))
	require.Empty(t, skills.ListReferences("base"))
	require.Empty(t, skills.ListReferences("../etc"))
}

func TestComposeSystemPrompt_lendoraGatedOnTools(t *testing.T) {
	withLendora, err := skills.ComposeSystemPrompt([]string{"build_bank_send", "lendora_get_all_markets"})
	require.NoError(t, err)
	require.Contains(t, withLendora, "Lendora Lending")
	require.Contains(t, withLendora, "read_skill_reference")

	without, err := skills.ComposeSystemPrompt([]string{"build_bank_send"})
	require.NoError(t, err)
	require.NotContains(t, without, "Lendora Lending")
}

func TestReadReferenceFromArgs(t *testing.T) {
	got, err := skills.ReadReferenceFromArgs(map[string]any{
		"skill": "lendora-lending",
		"file":  "error-responses.md",
	})
	require.NoError(t, err)
	require.Contains(t, got, "Broadcast Errors")

	_, err = skills.ReadReferenceFromArgs(map[string]any{"skill": "lendora-lending"})
	require.Error(t, err)
}
