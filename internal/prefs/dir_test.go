package prefs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/brand"
)

func TestResolveConfigDir_canonicalWins(t *testing.T) {
	root := t.TempDir()
	canonical := filepath.Join(root, brand.BundleID)
	legacy := filepath.Join(root, "com.svpchain.local-agent-gui")
	require.NoError(t, os.MkdirAll(canonical, 0o755))
	require.NoError(t, os.MkdirAll(legacy, 0o755))

	require.Equal(t, canonical, resolveConfigDir(root))
}

func TestResolveConfigDir_migratesLegacyPrefsDir(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "com.svpchain.local-agent-gui")
	require.NoError(t, os.MkdirAll(legacy, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(legacy, "prefs.json"), []byte("{}"), 0o600))

	got := resolveConfigDir(root)
	canonical := filepath.Join(root, brand.BundleID)
	require.Equal(t, canonical, got)
	_, err := os.Stat(filepath.Join(got, "prefs.json"))
	require.NoError(t, err)
	_, err = os.Stat(legacy)
	require.True(t, os.IsNotExist(err))
}

func TestResolveConfigDir_migratesAgentGUIDir(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "com.svpchain.agent-gui")
	require.NoError(t, os.MkdirAll(legacy, 0o755))

	got := resolveConfigDir(root)
	require.Equal(t, filepath.Join(root, brand.BundleID), got)
	_, err := os.Stat(legacy)
	require.True(t, os.IsNotExist(err))
}

func TestResolveConfigDir_freshInstall(t *testing.T) {
	root := t.TempDir()
	require.Equal(t, filepath.Join(root, brand.BundleID), resolveConfigDir(root))
}
