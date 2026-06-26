package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/prefs"
)

func TestSessionMemoryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	memPath := filepath.Join(dir, "agent_memory.json")
	prefsPath := filepath.Join(dir, "prefs.json")
	require.NoError(t, os.WriteFile(prefsPath, []byte(`{}`), 0o600))
	t.Cleanup(func() {
		prefs.SetPathOverride("")
		SetMemoryPathOverride("")
	})
	prefs.SetPathOverride(prefsPath)
	SetMemoryPathOverride(memPath)

	mem := SessionMemory{
		ChainID:      "svp-2517-1",
		RemoteURL:    "https://example.com/mcp",
		LocalOwner:   "svp1abc",
		SignerWhoami: `{"owner":"svp1abc"}`,
		RemoteWhoami: `{"tenant_id":"t1"}`,
	}
	require.NoError(t, saveSessionMemory(mem))

	got, ok := loadSessionMemory("svp-2517-1", "https://example.com/mcp", "svp1abc")
	require.True(t, ok)
	require.Equal(t, mem.SignerWhoami, got.SignerWhoami)
	require.Equal(t, mem.RemoteWhoami, got.RemoteWhoami)
}

func TestSessionMemoryInvalidatesOnOwnerChange(t *testing.T) {
	dir := t.TempDir()
	memPath := filepath.Join(dir, "agent_memory.json")
	prefsPath := filepath.Join(dir, "prefs.json")
	require.NoError(t, os.WriteFile(prefsPath, []byte(`{}`), 0o600))
	t.Cleanup(func() {
		prefs.SetPathOverride("")
		SetMemoryPathOverride("")
	})
	prefs.SetPathOverride(prefsPath)
	SetMemoryPathOverride(memPath)

	mem := SessionMemory{
		ChainID:      "svp-2517-1",
		RemoteURL:    "https://example.com/mcp",
		LocalOwner:   "svp1abc",
		SignerWhoami: `{}`,
		RemoteWhoami: `{}`,
	}
	require.NoError(t, saveSessionMemory(mem))

	_, ok := loadSessionMemory("svp-2517-1", "https://example.com/mcp", "svp1other")
	require.False(t, ok)
}

func TestSessionMemoryPrompt(t *testing.T) {
	t.Parallel()
	out := sessionMemoryPrompt(SessionMemory{
		SignerWhoami: `{"owner":"svp1x"}`,
		RemoteWhoami: `{"tenant_id":"auto-1"}`,
	})
	require.Contains(t, out, "Do NOT call signer_whoami or whoami")
	require.Contains(t, out, "svp1x")
	require.Contains(t, out, "auto-1")
}

func TestDispatchToolUsesCachedWhoami(t *testing.T) {
	mem := &SessionMemory{
		ChainID:      "svp-2517-1",
		RemoteURL:    "https://example.com/mcp",
		LocalOwner:   "svp1abc",
		SignerWhoami: `{"owner":"svp1abc","chain_id":"svp-2517-1"}`,
		RemoteWhoami: `{"tenant_id":"auto-1","owner":"svp1abc"}`,
	}
	out, err := dispatchTool(context.Background(), "svp-2517-1", nil, nil, "signer_whoami", nil, mem)
	require.NoError(t, err)
	require.JSONEq(t, mem.SignerWhoami, out)

	out, err = dispatchTool(context.Background(), "svp-2517-1", nil, nil, "whoami", nil, mem)
	require.NoError(t, err)
	require.JSONEq(t, mem.RemoteWhoami, out)
}

func TestMemoryStorePersistsMultipleChains(t *testing.T) {
	dir := t.TempDir()
	memPath := filepath.Join(dir, "agent_memory.json")
	prefsPath := filepath.Join(dir, "prefs.json")
	require.NoError(t, os.WriteFile(prefsPath, []byte(`{}`), 0o600))
	t.Cleanup(func() {
		prefs.SetPathOverride("")
		SetMemoryPathOverride("")
	})
	prefs.SetPathOverride(prefsPath)
	SetMemoryPathOverride(memPath)

	require.NoError(t, saveSessionMemory(SessionMemory{
		ChainID: "a", RemoteURL: "u1", LocalOwner: "o1",
		SignerWhoami: "{}", RemoteWhoami: `{}`,
	}))
	require.NoError(t, saveSessionMemory(SessionMemory{
		ChainID: "b", RemoteURL: "u2", LocalOwner: "o2",
		SignerWhoami: `{"x":1}`, RemoteWhoami: `{}`,
	}))

	data, err := os.ReadFile(memPath)
	require.NoError(t, err)
	var store memoryStore
	require.NoError(t, json.Unmarshal(data, &store))
	require.Len(t, store.Entries, 2)
}
