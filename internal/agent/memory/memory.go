package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	localsigner "github.com/svpchain/svpchain-agent/internal/agent/local"
	remotemcp "github.com/svpchain/svpchain-agent/internal/agent/remote"
	"github.com/svpchain/svpchain-agent/internal/agent/step"
	"github.com/svpchain/svpchain-agent/internal/prefs"
)

const memoryFileName = "agent_memory.json"

var memoryPathOverride string

// SetPathOverride redirects agent_memory.json loading for tests.
func SetPathOverride(path string) {
	memoryPathOverride = path
}

// Session holds cached signer_whoami and remote whoami results for a chain/session.
type Session struct {
	ChainID      string    `json:"chain_id"`
	RemoteURL    string    `json:"remote_url"`
	LocalOwner   string    `json:"local_owner"`
	SignerWhoami string    `json:"signer_whoami"`
	RemoteWhoami string    `json:"remote_whoami"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type memoryStore struct {
	Entries map[string]Session `json:"entries"`
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

func loadSessionMemory(chainID, remoteURL, localOwner string) (Session, bool) {
	path := memoryFilePath()
	if path == "" {
		return Session{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Session{}, false
	}
	var store memoryStore
	if err := json.Unmarshal(data, &store); err != nil || store.Entries == nil {
		return Session{}, false
	}
	mem, ok := store.Entries[memoryKey(chainID, remoteURL)]
	if !ok {
		return Session{}, false
	}
	if !mem.validFor(chainID, remoteURL, localOwner) {
		return Session{}, false
	}
	if strings.TrimSpace(mem.SignerWhoami) == "" || strings.TrimSpace(mem.RemoteWhoami) == "" {
		return Session{}, false
	}
	return mem, true
}

func Save(mem Session) error {
	path := memoryFilePath()
	if path == "" {
		return nil
	}
	var store memoryStore
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &store)
	}
	if store.Entries == nil {
		store.Entries = make(map[string]Session)
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

func (m Session) validFor(chainID, remoteURL, localOwner string) bool {
	return strings.TrimSpace(m.ChainID) == strings.TrimSpace(chainID) &&
		strings.TrimSpace(m.RemoteURL) == strings.TrimSpace(remoteURL) &&
		strings.TrimSpace(m.LocalOwner) == strings.TrimSpace(localOwner)
}

func (m Session) ToolResult(name string) (string, bool) {
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

func (m *Session) SetToolResult(name, result string) {
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

func refreshSessionMemory(ctx context.Context, chainID, remoteURL, localOwner string, local *localsigner.Signer, remote *remotemcp.Client) (Session, error) {
	signerJSON, err := local.CallTool(ctx, "signer_whoami", nil)
	if err != nil {
		return Session{}, fmt.Errorf("signer_whoami: %w", err)
	}
	remoteJSON, err := remote.CallTool(ctx, "whoami", nil)
	if err != nil {
		return Session{}, fmt.Errorf("whoami: %w", err)
	}
	mem := Session{
		ChainID:      chainID,
		RemoteURL:    remoteURL,
		LocalOwner:   localOwner,
		SignerWhoami: signerJSON,
		RemoteWhoami: remoteJSON,
	}
	if err := Save(mem); err != nil {
		return mem, err
	}
	return mem, nil
}

func Resolve(ctx context.Context, chainID, remoteURL, localOwner string, local *localsigner.Signer, remote *remotemcp.Client, emit func(step.Step)) (Session, error) {
	if mem, ok := loadSessionMemory(chainID, remoteURL, localOwner); ok {
		return mem, nil
	}
	emit(step.Step{Kind: step.Think, Title: "Loading session context…"})
	return refreshSessionMemory(ctx, chainID, remoteURL, localOwner, local, remote)
}

func Prompt(mem Session) string {
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
