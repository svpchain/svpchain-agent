package prefs_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/prefs"
)

func TestReadAndStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prefs.json")
	t.Cleanup(func() { prefs.SetPathOverride("") })
	prefs.SetPathOverride(path)

	store := prefs.Load()
	store.SetLanguage("zh")
	store.SetAgentSettings(prefs.AgentSettings{
		ChainID:           "localsvp-1",
		SkillsConfigBase:  "/tmp/custom-config",
		DisabledSkills:    []string{"x402"},
		PhoenixOTLPURL:    "http://127.0.0.1:6006/v1/traces",
		AgentValidatorURL: "http://validator.example:7080",
	})

	got := prefs.Read()
	require.Equal(t, "zh", got.Language)
	require.Equal(t, "localsvp-1", got.AgentChainID)
	require.Equal(t, "/tmp/custom-config", got.SkillsConfigBase)
	require.Equal(t, []string{"x402"}, got.DisabledSkills)
	require.Equal(t, "http://127.0.0.1:6006/v1/traces", got.PhoenixOTLPURL)
	require.Equal(t, "http://validator.example:7080", got.AgentValidatorURL)

	reloaded := prefs.Load()
	require.Equal(t, store.File(), reloaded.File())

	store.SetAgentSettings(prefs.AgentSettings{ChainID: "localsvp-1"})
	require.Empty(t, store.AgentSettings().PhoenixOTLPURL)
	cleared := prefs.Load()
	require.Empty(t, cleared.AgentSettings().PhoenixOTLPURL)
}

func TestPathOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prefs.json")
	t.Cleanup(func() { prefs.SetPathOverride("") })
	prefs.SetPathOverride(path)
	require.Equal(t, path, prefs.Path())
}

func TestReadMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.json")
	t.Cleanup(func() { prefs.SetPathOverride("") })
	prefs.SetPathOverride(path)

	got := prefs.Read()
	require.Equal(t, prefs.File{}, got)
	_, err := os.Stat(path)
	require.True(t, os.IsNotExist(err))
}
