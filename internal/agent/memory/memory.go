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
	"github.com/svpchain/svpchain-agent/internal/agent/step"
	"github.com/svpchain/svpchain-agent/internal/prefs"
)

const memoryFileName = "agent_memory.json"

var memoryPathOverride string

// SetPathOverride redirects agent_memory.json loading for tests.
func SetPathOverride(path string) {
	memoryPathOverride = path
}

// Session holds cached signer identity for a chain/session.
type Session struct {
	ChainID      string    `json:"chain_id"`
	LocalOwner   string    `json:"local_owner"`
	SignerWhoami string    `json:"signer_whoami"`
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

func memoryKey(chainID string) string {
	return strings.TrimSpace(chainID)
}

func loadSessionMemory(chainID, localOwner string) (Session, bool) {
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
	mem, ok := store.Entries[memoryKey(chainID)]
	if !ok {
		return Session{}, false
	}
	if !mem.validFor(chainID, localOwner) {
		return Session{}, false
	}
	if strings.TrimSpace(mem.SignerWhoami) == "" {
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
	store.Entries[memoryKey(mem.ChainID)] = mem
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func (m Session) validFor(chainID, localOwner string) bool {
	return strings.TrimSpace(m.ChainID) == strings.TrimSpace(chainID) &&
		strings.TrimSpace(m.LocalOwner) == strings.TrimSpace(localOwner)
}

func (m Session) ToolResult(name string) (string, bool) {
	switch name {
	case "signer_whoami":
		if s := strings.TrimSpace(m.SignerWhoami); s != "" {
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
	default:
		return
	}
	m.UpdatedAt = time.Now().UTC()
}

func refreshSessionMemory(ctx context.Context, chainID, localOwner string, local *localsigner.Signer) (Session, error) {
	signerJSON, err := local.CallTool(ctx, "signer_whoami", nil)
	if err != nil {
		return Session{}, fmt.Errorf("signer_whoami: %w", err)
	}
	mem := Session{
		ChainID:      chainID,
		LocalOwner:   localOwner,
		SignerWhoami: signerJSON,
	}
	if err := Save(mem); err != nil {
		return mem, err
	}
	return mem, nil
}

func Resolve(ctx context.Context, chainID, localOwner string, local *localsigner.Signer, emit func(step.Step)) (Session, error) {
	if mem, ok := loadSessionMemory(chainID, localOwner); ok {
		return mem, nil
	}
	emit(step.Step{Kind: step.Think, Title: "Loading session context…"})
	return refreshSessionMemory(ctx, chainID, localOwner, local)
}

func Prompt(mem Session) string {
	if strings.TrimSpace(mem.SignerWhoami) == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Cached session context\n\n")
	b.WriteString("The following signer identity is cached for this chain and key. ")
	b.WriteString("Do NOT call signer_whoami at the start of the conversation — use this directly.\n")
	if mem.SignerWhoami != "" {
		b.WriteString("\nLocal signer (signer_whoami):\n")
		b.WriteString(mem.SignerWhoami)
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}
