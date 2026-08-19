package skills_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/agent/skills"
	"github.com/svpchain/svpchain-agent/internal/prefs"
)

func TestComposeSystemPrompt_respectsDisabledSkills(t *testing.T) {
	t.Cleanup(func() { skills.ClearDisabledSkillsOverride() })
	skills.SetDisabledSkillsOverride([]string{"x402", "erc-tokens"})

	tools := []string{
		"build_bank_send", "build_swap", "build_erc20_transfer",
		"sign_transaction", "sign_evm_transaction", "sign_typed_data",
		"broadcast_signed_tx", "broadcast_evm_tx",
		"http_fetch", "evm_to_bech32", "signer_whoami", "whoami",
	}
	got, err := skills.ComposeSystemPrompt(tools)
	require.NoError(t, err)
	require.NotContains(t, got, "x402 paid HTTP")
	require.NotContains(t, got, "ERC20/ERC721 contract calls")
	require.Contains(t, got, "Workflow for on-chain writes")
}

func TestListSettings_lockedSkillAlwaysEnabled(t *testing.T) {
	settings, err := skills.ListSettings()
	require.NoError(t, err)
	var base *skills.Setting
	for i := range settings {
		if settings[i].Name == "base" {
			base = &settings[i]
			break
		}
	}
	require.NotNil(t, base)
	require.True(t, base.Enabled)
	require.True(t, base.Locked)
}

func TestListSettings_readsDisabledFromPrefs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prefs.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"disabled_skills":["x402"]}`), 0o600))

	t.Cleanup(func() {
		skills.ClearDisabledSkillsOverride()
		prefs.SetPathOverride("")
	})
	prefs.SetPathOverride(path)

	settings, err := skills.ListSettings()
	require.NoError(t, err)
	for _, s := range settings {
		if s.Name == "x402" {
			require.False(t, s.Enabled)
		}
	}
}
