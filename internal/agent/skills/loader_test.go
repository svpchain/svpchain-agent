package skills_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/agent/skills"
)

// hermetic pins skill loading to the bundled set alone: an empty user skills
// dir and no disabled skills. Without this, the developer's live prefs.json
// (skill toggles flipped in the GUI) and user skill overrides leak into the
// composed prompt, and the tests pass or fail depending on whose machine runs
// them.
func hermetic(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		skills.SetSkillsDirOverride("")
		skills.ClearDisabledSkillsOverride()
	})
	skills.SetSkillsDirOverride(t.TempDir())
	skills.SetDisabledSkillsOverride(nil)
}

func TestComposeSystemPrompt_matchesLegacyWithFullToolSet(t *testing.T) {
	hermetic(t)
	// Exclude x402-only tools (http_fetch, sign_typed_data, signer_whoami, x402_*) so the
	// detailed x402 skill is not injected; signer-identity is also excluded via no signer_whoami.
	tools := []string{
		"build_bank_send", "build_swap", "build_erc20_transfer", "build_erc721_transfer",
		"sign_transaction", "sign_evm_transaction",
		"broadcast_signed_tx", "broadcast_evm_tx",
		"evm_to_bech32", "signer_whoami", "whoami",
	}
	got, err := skills.ComposeSystemPrompt(tools)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(got, "# Role"))
	require.Contains(t, got, "# Red lines")
	require.Contains(t, got, "NEVER** skip local signing")
	require.Contains(t, got, "transfer whitelist")
	require.Contains(t, got, "Workflow for on-chain writes:")
	require.Contains(t, got, "Pass signed_tx fields VERBATIM")
	require.Contains(t, got, "build_bank_send only accepts svp1")
	require.Contains(t, got, "build_erc20_* / build_erc721_*")
	require.Contains(t, got, "Cached session context")
	require.Contains(t, got, "Be concise in final answers")
}

func TestComposeSystemPrompt_includesX402SkillWhenToolsPresent(t *testing.T) {
	hermetic(t)
	tools := []string{"http_fetch", "x402_prepare_typed_data", "sign_typed_data", "signer_whoami"}
	got, err := skills.ComposeSystemPrompt(tools)
	require.NoError(t, err)
	require.Contains(t, got, "x402_prepare_typed_data")
	require.Contains(t, got, "Never invent the nonce")
}

func TestComposeSystemPrompt_alwaysIncludesBase(t *testing.T) {
	hermetic(t)
	got, err := skills.ComposeSystemPrompt([]string{"build_bank_send"})
	require.NoError(t, err)
	require.Contains(t, got, "svpchain agent")
	require.Contains(t, got, "# Red lines")
	require.Contains(t, got, "build_bank_send only accepts svp1")
	require.NotContains(t, got, "Never invent the nonce")
}

func TestComposeSystemPrompt_userSkillOverridesBundled(t *testing.T) {
	hermetic(t)
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "base")
	require.NoError(t, os.MkdirAll(skillDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: base
priority: 0
---
Custom base instructions.
`), 0o600))
	t.Cleanup(func() { skills.SetSkillsDirOverride("") })
	skills.SetSkillsDirOverride(dir)

	got, err := skills.ComposeSystemPrompt(nil)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(got, "Custom base instructions."))
	require.NotContains(t, got, "# Red lines")
}

func TestLoadAll_includesBundledSkills(t *testing.T) {
	hermetic(t)
	all, err := skills.LoadAll()
	require.NoError(t, err)
	names := make([]string, len(all))
	for i, s := range all {
		names[i] = s.Name
	}
	require.Contains(t, names, "base")
	require.Contains(t, names, "onchain-workflow")
	require.Contains(t, names, "x402")
	require.Contains(t, names, "a2a")
}

func TestToolPatternMatch(t *testing.T) {
	require.True(t, skills.MatchesToolPattern("build_*", "build_bank_send"))
	require.True(t, skills.MatchesToolPattern("http_fetch", "http_fetch"))
	require.False(t, skills.MatchesToolPattern("build_*", "sign_transaction"))
}

func TestParseSkillContent_invalid(t *testing.T) {
	_, err := skills.ParseSkillContent("no frontmatter", "bundled")
	require.Error(t, err)
}
