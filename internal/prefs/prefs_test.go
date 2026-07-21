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
		ChainID:          "localsvp-1",
		SkillsConfigBase: "/tmp/custom-config",
		DisabledSkills:   []string{"x402"},
	})

	got := prefs.Read()
	require.Equal(t, "zh", got.Language)
	require.Equal(t, "localsvp-1", got.AgentChainID)
	require.Equal(t, "/tmp/custom-config", got.SkillsConfigBase)
	require.Equal(t, []string{"x402"}, got.DisabledSkills)

	reloaded := prefs.Load()
	require.Equal(t, store.File(), reloaded.File())
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

func TestLoadSeedsDefaultWhitelistOnFirstRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prefs.json")
	t.Cleanup(func() { prefs.SetPathOverride("") })
	prefs.SetPathOverride(path)

	// First run: no file yet -> defaults are seeded and persisted to disk.
	store := prefs.Load()
	seeded := store.File().Whitelist
	require.NotEmpty(t, seeded, "fresh install should seed predefined whitelist")

	onDisk := prefs.Read()
	require.Equal(t, seeded, onDisk.Whitelist, "seed must be written to prefs.json")

	// Second load: file exists -> seed is not re-applied (same entries, no dupes).
	require.Equal(t, seeded, prefs.Load().File().Whitelist)

	// A user deletion must persist and not be re-seeded on the next launch.
	store.Update(func(f *prefs.File) { f.Whitelist = []prefs.WhitelistEntry{} })
	require.Empty(t, prefs.Load().File().Whitelist, "deletions must not be re-seeded")
}
