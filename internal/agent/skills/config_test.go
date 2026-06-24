package skills_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/agent/skills"
)

func TestUserSkillsDir_appliedConfigBase(t *testing.T) {
	t.Cleanup(func() {
		skills.SetSkillsDirOverride("")
		skills.ApplySkillsConfigBase("")
	})

	root := t.TempDir()
	skills.ApplySkillsConfigBase(root)

	got := skills.UserSkillsDir()
	want := filepath.Join(root, "com.svpchain.agent-gui", "skills")
	require.Equal(t, want, got)
}

func TestUserSkillsDir_setSkillsDirOverrideTakesPrecedence(t *testing.T) {
	t.Cleanup(func() {
		skills.SetSkillsDirOverride("")
		skills.ApplySkillsConfigBase("")
	})

	override := filepath.Join(t.TempDir(), "custom-skills")
	skills.ApplySkillsConfigBase(t.TempDir())
	skills.SetSkillsDirOverride(override)

	require.Equal(t, override, skills.UserSkillsDir())
}

func TestResolveUserSkillsDir(t *testing.T) {
	got, err := skills.ResolveUserSkillsDir("/tmp/example")
	require.NoError(t, err)
	require.Equal(t, filepath.Join("/tmp/example", "com.svpchain.agent-gui", "skills"), got)
}

func TestUserSkillsDir_emptyBaseUsesOSDefault(t *testing.T) {
	t.Cleanup(func() {
		skills.ApplySkillsConfigBase("")
	})
	skills.ApplySkillsConfigBase("")

	defaultBase, err := skills.DefaultSkillsConfigBase()
	require.NoError(t, err)
	want := filepath.Join(defaultBase, "com.svpchain.agent-gui", "skills")
	require.Equal(t, want, skills.UserSkillsDir())
}
