package skills_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/agent/skills"
)

// legacyTail is the composed prompt suffix when the full trading tool set is available
// (all skills except x402/a2a, which are tool-gated).
const legacyTail = `Workflow for on-chain writes:
1. Use remote build_* tools to construct unsigned transactions (or EVM payloads).
2. Sign locally with sign_transaction / sign_evm_transaction (never skip signing).
3. Broadcast with broadcast_signed_tx or broadcast_evm_tx on the remote server.
4. Pass signed_tx fields VERBATIM from sign_* to broadcast_*.

Sending SVP (or any bank denom) to a 0x EVM address: build_bank_send only accepts svp1… recipients. When the user gives a 0x address, FIRST call evm_to_bech32 to convert it, then use the returned svp1… owner as build_bank_send.recipient (denom "asvp" for SVP). Never pass a 0x address straight to build_bank_send.

For ERC20/ERC721 contract calls (transfer, approve, transferFrom, safeTransferFrom, setApprovalForAll): use the remote build_erc20_* / build_erc721_* tools — they return a ready-to-sign EVMTxPayload (nonce/gas/fees filled). ERC20 amounts are human units; ERC721 uses token_id. Then sign_evm_transaction and broadcast_evm_tx, exactly like build_swap.

When **Cached session context** is present in the system prompt, use that data directly — do not call signer_whoami or whoami unless the user changed chain, signing key, or remote MCP endpoint. Otherwise call signer_whoami for the local key and whoami for remote tenant policy after auth.

Be concise in final answers. Show tx hashes and key numbers when operations succeed.`

func TestComposeSystemPrompt_matchesLegacyWithFullToolSet(t *testing.T) {
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
	require.Contains(t, got, legacyTail)
}

func TestComposeSystemPrompt_includesX402SkillWhenToolsPresent(t *testing.T) {
	tools := []string{"http_fetch", "x402_prepare_typed_data", "sign_typed_data", "signer_whoami"}
	got, err := skills.ComposeSystemPrompt(tools)
	require.NoError(t, err)
	require.Contains(t, got, "x402_prepare_typed_data")
	require.Contains(t, got, "Never invent `nonce` by hand")
}

func TestComposeSystemPrompt_alwaysIncludesBase(t *testing.T) {
	got, err := skills.ComposeSystemPrompt([]string{"build_bank_send"})
	require.NoError(t, err)
	require.Contains(t, got, "svpchain trading assistant")
	require.Contains(t, got, "# Red lines")
	require.Contains(t, got, "build_bank_send only accepts svp1")
	require.NotContains(t, got, "Never invent `nonce` by hand")
}

func TestComposeSystemPrompt_userSkillOverridesBundled(t *testing.T) {
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
