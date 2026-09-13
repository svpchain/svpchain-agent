package settlement

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/svpchain/svpchain-agent/internal/prefs"
)

const callbackOutboxFile = "settlement_callbacks.json"

// PendingCallback is an execution callback that has been broadcast locally
// but has not yet been acknowledged by agent-validator. It contains public
// identifiers only, never a private key or signed transaction.
type PendingCallback struct {
	TaskID       string    `json:"task_id"`
	TxHash       string    `json:"tx_hash"`
	ValidatorURL string    `json:"validator_url"`
	QueuedAt     time.Time `json:"queued_at"`
	Attempts     int       `json:"attempts,omitempty"`
	LastError    string    `json:"last_error,omitempty"`
}

var (
	callbackOutboxMu     sync.Mutex
	callbackPathOverride string
)

// SetCallbackOutboxPathOverride redirects the callback outbox for tests.
func SetCallbackOutboxPathOverride(path string) {
	callbackOutboxMu.Lock()
	defer callbackOutboxMu.Unlock()
	callbackPathOverride = path
}

func callbackOutboxPath() string {
	if callbackPathOverride != "" {
		return callbackPathOverride
	}
	prefPath := prefs.Path()
	if prefPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(prefPath), callbackOutboxFile)
}

// QueueCallback durably records a callback before its first network attempt.
// Recording first means an app crash after a successful broadcast cannot lose
// the only link between that transaction and its settlement task.
func QueueCallback(cfg Config, txHash string) error {
	taskID := normalizeHash(cfg.TaskID)
	txHash = normalizeHash(txHash)
	validatorURL := strings.TrimRight(strings.TrimSpace(cfg.ValidatorURL), "/")
	if taskID == "" || txHash == "" || validatorURL == "" {
		return fmt.Errorf("settlement callback requires task_id, tx_hash, and validator URL")
	}
	callbackOutboxMu.Lock()
	defer callbackOutboxMu.Unlock()
	entries, err := loadCallbacksLocked()
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.TaskID == taskID && entry.TxHash == txHash && entry.ValidatorURL == validatorURL {
			return nil
		}
	}
	entries = append(entries, PendingCallback{TaskID: taskID, TxHash: txHash, ValidatorURL: validatorURL, QueuedAt: time.Now().UTC()})
	return saveCallbacksLocked(entries)
}

// PendingCallbacks returns callbacks that will be retried on startup and in
// the background. A corrupt file is surfaced rather than discarded because it
// may be the only record of a paid task's execution transaction.
func PendingCallbacks() ([]PendingCallback, error) {
	callbackOutboxMu.Lock()
	defer callbackOutboxMu.Unlock()
	entries, err := loadCallbacksLocked()
	if err != nil {
		return nil, err
	}
	return append([]PendingCallback(nil), entries...), nil
}

// RetryPendingCallbacks posts every durable callback once. Successful entries
// are removed; failures remain for the next periodic or post-restart retry.
func RetryPendingCallbacks(ctx context.Context) error {
	entries, err := PendingCallbacks()
	if err != nil || len(entries) == 0 {
		return err
	}
	var failures []error
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		cfg := Config{TaskID: entry.TaskID, ValidatorURL: entry.ValidatorURL}
		if err := postCallback(ctx, cfg, entry.TxHash); err != nil {
			_ = updateCallbackFailure(entry, err)
			failures = append(failures, fmt.Errorf("%s: %w", entry.TaskID, err))
			continue
		}
		if err := removeCallback(entry); err != nil {
			failures = append(failures, err)
		}
	}
	return errorsJoin(failures)
}

func updateCallbackFailure(target PendingCallback, failure error) error {
	callbackOutboxMu.Lock()
	defer callbackOutboxMu.Unlock()
	entries, err := loadCallbacksLocked()
	if err != nil {
		return err
	}
	for i := range entries {
		if sameCallback(entries[i], target) {
			entries[i].Attempts++
			entries[i].LastError = failure.Error()
			break
		}
	}
	return saveCallbacksLocked(entries)
}

func removeCallback(target PendingCallback) error {
	callbackOutboxMu.Lock()
	defer callbackOutboxMu.Unlock()
	entries, err := loadCallbacksLocked()
	if err != nil {
		return err
	}
	kept := entries[:0]
	for _, entry := range entries {
		if !sameCallback(entry, target) {
			kept = append(kept, entry)
		}
	}
	return saveCallbacksLocked(kept)
}

func sameCallback(a, b PendingCallback) bool {
	return a.TaskID == b.TaskID && a.TxHash == b.TxHash && a.ValidatorURL == b.ValidatorURL
}

func loadCallbacksLocked() ([]PendingCallback, error) {
	path := callbackOutboxPath()
	if path == "" {
		return nil, fmt.Errorf("settlement callback outbox path is unavailable")
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []PendingCallback
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("decode settlement callback outbox: %w", err)
	}
	return entries, nil
}

func saveCallbacksLocked(entries []PendingCallback) error {
	path := callbackOutboxPath()
	if path == "" {
		return fmt.Errorf("settlement callback outbox path is unavailable")
	}
	if len(entries) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".settlement-callbacks-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func errorsJoin(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("retry settlement callbacks: %v", errs)
}
