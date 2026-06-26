package a2aserver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/prefs"
)

func TestConfigFromPrefsRequiresChain(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "prefs.json")
	require.NoError(t, os.WriteFile(path, []byte(`{}`), 0o600))
	t.Cleanup(func() { prefs.SetPathOverride("") })
	prefs.SetPathOverride(path)

	_, err := ConfigFromPrefs("", "", "")
	require.ErrorIs(t, err, errChainIDRequired)
}

func TestBuildAgentCard(t *testing.T) {
	t.Parallel()
	card := BuildAgentCard("http://127.0.0.1:8080")
	require.Equal(t, AgentName, card.Name)
	require.Contains(t, card.Description, "swap")
	require.Contains(t, card.Description, "bridge")
	require.NotEmpty(t, card.Skills)
	require.True(t, card.Capabilities.Streaming)
	require.Len(t, card.SupportedInterfaces, 1)
	require.Equal(t, "http://127.0.0.1:8080/invoke", card.SupportedInterfaces[0].URL)
	require.Equal(t, "svpchain-onchain", card.Skills[0].ID)
}
