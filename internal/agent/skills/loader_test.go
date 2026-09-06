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

func TestComposeSystemPrompt_discoveryStatesTheMarketIsTheOnlySource(t *testing.T) {
	hermetic(t)
	got, err := skills.ComposeSystemPrompt([]string{"search_agents"})
	require.NoError(t, err)
	require.Contains(t, got, "search_agents")
	require.Contains(t, got, "the **only** source of these results")
}

// In Agent-Market-only mode the model must not be primed with a direct-service
// capability. It should discover and attach an agent before it sees any
// execution tool name.
func TestComposeSystemPrompt_marketOnlyDoesNotMentionDirectMCP(t *testing.T) {
	hermetic(t)
	got, err := skills.ComposeSystemPrompt([]string{
		"search_agents", "begin_agent_settlement", "a2a_connect_agent", "a2a_send_message",
		"sign_transaction", "sign_evm_transaction", "signer_whoami",
	})
	require.NoError(t, err)
	require.NotContains(t, got, "MCP")
	require.NotContains(t, got, "build_swap")
	require.Contains(t, got, "Find a suitable active agent through `search_agents`")
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

func TestCompose_listsInjectedSkills(t *testing.T) {
	hermetic(t)
	_, names, err := skills.Compose([]string{"build_bank_send"})
	require.NoError(t, err)
	require.Contains(t, names, "base")
	require.Contains(t, names, "bank-send-evm")
	require.NotContains(t, names, "x402")
	require.NotContains(t, names, "a2a")
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
	require.Contains(t, names, "agent-discovery")
	require.NotContains(t, names, "a2a")
}

func TestComposeSystemPrompt_doesNotInjectA2APlaybook(t *testing.T) {
	hermetic(t)
	got, err := skills.ComposeSystemPrompt([]string{
		"a2a_send_message", "search_agents",
	})
	require.NoError(t, err)
	require.NotContains(t, got, "## Tool: a2a_send_message")
	require.Contains(t, got, "search_agents")
	require.Contains(t, got, "uncredentialed plain text")
}

// The attach playbook appears only when the attach tool does. Without it the
// assistant must not offer to use a remote agent's tools.
func TestComposeSystemPrompt_attachGatesOnTheConnectTool(t *testing.T) {
	hermetic(t)
	without, err := skills.ComposeSystemPrompt([]string{"search_agents", "a2a_send_message"})
	require.NoError(t, err)
	require.NotContains(t, without, "# Using a remote agent's tools")

	with, err := skills.ComposeSystemPrompt([]string{"search_agents", "a2a_connect_agent"})
	require.NoError(t, err)
	require.Contains(t, with, "# Using a remote agent's tools")
	require.Contains(t, with, "tools_skipped")
	require.Contains(t, with, "do not conclude the task is impossible")
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
