package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/svpchain/svpchain-agent/internal/prefs"
)

const memoryFileName = "agent_memory.json"

var memoryPathOverride string

// SetMemoryPathOverride redirects agent_memory.json loading for tests.
func SetMemoryPathOverride(path string) {
	memoryPathOverride = path
}

// SessionMemory holds cached signer_whoami and remote whoami results for a chain/session.
type SessionMemory struct {
	ChainID      string    `json:"chain_id"`
	RemoteURL    string    `json:"remote_url"`
	LocalOwner   string    `json:"local_owner"`
	SignerWhoami string    `json:"signer_whoami"`
	RemoteWhoami string    `json:"remote_whoami"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type memoryStore struct {
	Entries map[string]SessionMemory `json:"entries"`
}

func memoryFilePath() string {
	if memoryPathOverride != "" {
		return memoryPathOverride
	}
	p := prefs.Path()
	if p == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(p), memoryFileName)
}

func memoryKey(chainID, remoteURL string) string {
	return strings.TrimSpace(chainID) + "\x00" + strings.TrimSpace(remoteURL)
}

func loadSessionMemory(chainID, remoteURL, localOwner string) (SessionMemory, bool) {
	path := memoryFilePath()
	if path == "" {
		return SessionMemory{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return SessionMemory{}, false
	}
	var store memoryStore
	if err := json.Unmarshal(data, &store); err != nil || store.Entries == nil {
		return SessionMemory{}, false
	}
	mem, ok := store.Entries[memoryKey(chainID, remoteURL)]
	if !ok {
		return SessionMemory{}, false
	}
	if !mem.validFor(chainID, remoteURL, localOwner) {
		return SessionMemory{}, false
	}
	if strings.TrimSpace(mem.SignerWhoami) == "" || strings.TrimSpace(mem.RemoteWhoami) == "" {
		return SessionMemory{}, false
	}
	return mem, true
}

func saveSessionMemory(mem SessionMemory) error {
	path := memoryFilePath()
	if path == "" {
		return nil
	}
	var store memoryStore
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &store)
	}
	if store.Entries == nil {
		store.Entries = make(map[string]SessionMemory)
	}
	mem.UpdatedAt = time.Now().UTC()
	store.Entries[memoryKey(mem.ChainID, mem.RemoteURL)] = mem
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func (m SessionMemory) validFor(chainID, remoteURL, localOwner string) bool {
	return strings.TrimSpace(m.ChainID) == strings.TrimSpace(chainID) &&
		strings.TrimSpace(m.RemoteURL) == strings.TrimSpace(remoteURL) &&
		strings.TrimSpace(m.LocalOwner) == strings.TrimSpace(localOwner)
}

func (m SessionMemory) toolResult(name string) (string, bool) {
	switch name {
	case "signer_whoami":
		if s := strings.TrimSpace(m.SignerWhoami); s != "" {
			return s, true
		}
	case "whoami":
		if s := strings.TrimSpace(m.RemoteWhoami); s != "" {
			return s, true
		}
	}
	return "", false
}

func (m *SessionMemory) setToolResult(name, result string) {
	result = strings.TrimSpace(result)
	if result == "" {
		return
	}
	switch name {
	case "signer_whoami":
		m.SignerWhoami = result
	case "whoami":
		m.RemoteWhoami = result
	default:
		return
	}
	m.UpdatedAt = time.Now().UTC()
}

func refreshSessionMemory(ctx context.Context, chainID, remoteURL, localOwner string, local *LocalSigner, remote *RemoteClient) (SessionMemory, error) {
	signerJSON, err := local.CallTool(ctx, "signer_whoami", nil)
	if err != nil {
		return SessionMemory{}, fmt.Errorf("signer_whoami: %w", err)
	}
	remoteJSON, err := remote.CallTool(ctx, "whoami", nil)
	if err != nil {
		return SessionMemory{}, fmt.Errorf("whoami: %w", err)
	}
	mem := SessionMemory{
		ChainID:      chainID,
		RemoteURL:    remoteURL,
		LocalOwner:   localOwner,
		SignerWhoami: signerJSON,
		RemoteWhoami: remoteJSON,
	}
	if err := saveSessionMemory(mem); err != nil {
		return mem, err
	}
	return mem, nil
}

func resolveSessionMemory(ctx context.Context, chainID, remoteURL, localOwner string, local *LocalSigner, remote *RemoteClient, emit func(Step)) (SessionMemory, error) {
	if mem, ok := loadSessionMemory(chainID, remoteURL, localOwner); ok {
		return mem, nil
	}
	emit(Step{Kind: StepThink, Title: "Loading session context…"})
	return refreshSessionMemory(ctx, chainID, remoteURL, localOwner, local, remote)
}

func sessionMemoryPrompt(mem SessionMemory) string {
	if strings.TrimSpace(mem.SignerWhoami) == "" && strings.TrimSpace(mem.RemoteWhoami) == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Cached session context\n\n")
	b.WriteString("The following identity and tenant data is cached for this chain and key. ")
	b.WriteString("Do NOT call signer_whoami or whoami at the start of the conversation — use this directly.\n")
	if mem.SignerWhoami != "" {
		b.WriteString("\nLocal signer (signer_whoami):\n")
		b.WriteString(mem.SignerWhoami)
		b.WriteByte('\n')
	}
	if mem.RemoteWhoami != "" {
		b.WriteString("\nRemote tenant (whoami):\n")
		b.WriteString(mem.RemoteWhoami)
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}
