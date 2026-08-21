package runlog

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// TxCheck is one on-chain lookup of a hash extracted from a broadcast result.
type TxCheck struct {
	Hash      string    `json:"hash"`
	Status    string    `json:"status"`
	Code      uint32    `json:"code,omitempty"`
	Height    string    `json:"height,omitempty"`
	RawLog    string    `json:"raw_log,omitempty"`
	Error     string    `json:"error,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

const (
	TxConfirmed = "confirmed" // included, code 0
	TxFailed    = "failed"    // included, code != 0
	TxPending   = "pending"   // not yet in a block / RPC not found
	TxError     = "error"     // RPC/transport failure
	TxSkipped   = "skipped"   // no chain RPC configured
)

// ChainTx is the subset of a CometBFT /tx lookup the run log needs.
type ChainTx struct {
	Code   uint32
	Height string
	RawLog string
	Events []ChainEvent
}

// ChainEvent is one flattened ABCI event from a confirmed transaction.
type ChainEvent struct {
	Type  string
	Attrs map[string]string
}

// TxQuerier looks up one transaction by hash (0x-prefixed or raw hex).
type TxQuerier func(ctx context.Context, hash string) (ChainTx, error)

var errRunIDRequired = fmt.Errorf("run id is required")

const recheckInterval = 2 * time.Second

// LCDQueryHash strips a 0x prefix and uppercases the hex, matching cosmos-sdk
// REST /cosmos/tx/v1beta1/txs/{hash} path encoding.
func LCDQueryHash(hash string) string {
	h := strings.TrimSpace(hash)
	h = strings.TrimPrefix(strings.ToLower(h), "0x")
	return strings.ToUpper(h)
}

// VerifyTxs looks up extracted hashes and matches build_* intents against
// confirmed tx events. A nil querier records skipped. Intents are still
// extracted when there are no hashes yet.
func (s *Session) VerifyTxs(ctx context.Context, lookup TxQuerier) {
	if s == nil {
		return
	}
	bundle := lookupHashes(ctx, s.run.TxHashes, lookup)
	s.run.TxChecks = bundle.checks
	s.run.IntentChecks = MatchIntents(ExtractIntents(s.run.Steps), bundle.checks, bundle.events, lookup != nil)
}

// RecheckTxs re-queries hashes for a persisted run and rewrites that line.
// Pending lookups are retried until ctx is done or every hash is terminal.
func RecheckTxs(ctx context.Context, id string, lookup TxQuerier) (Run, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Run{}, errRunIDRequired
	}
	runs, err := ReadAll("")
	if err != nil {
		return Run{}, err
	}
	var hashes []string
	found := false
	for _, run := range runs {
		if run.RunID == id {
			found = true
			hashes = append([]string(nil), run.TxHashes...)
			break
		}
	}
	if !found {
		return Run{}, fmt.Errorf("run not found")
	}
	var bundle txLookupBundle
	if len(hashes) > 0 {
		bundle = pollHashes(ctx, hashes, lookup)
	}
	return patchRun(id, func(run *Run) error {
		if len(hashes) > 0 {
			run.TxChecks = bundle.checks
		}
		run.IntentChecks = MatchIntents(ExtractIntents(run.Steps), run.TxChecks, bundle.events, lookup != nil)
		return nil
	})
}

func pollHashes(ctx context.Context, hashes []string, lookup TxQuerier) txLookupBundle {
	bundle := lookupHashes(ctx, hashes, lookup)
	if lookup == nil || ctx.Err() != nil {
		return bundle
	}
	ticker := time.NewTicker(recheckInterval)
	defer ticker.Stop()
	for {
		if txChecksDone(bundle.checks) {
			return bundle
		}
		select {
		case <-ctx.Done():
			return bundle
		case <-ticker.C:
			bundle = lookupHashes(ctx, hashes, lookup)
		}
	}
}

func txChecksDone(checks []TxCheck) bool {
	for _, c := range checks {
		if c.Status == TxPending || c.Status == TxError {
			return false
		}
	}
	return true
}

func lookupHashes(ctx context.Context, hashes []string, lookup TxQuerier) txLookupBundle {
	out := txLookupBundle{}
	if len(hashes) == 0 {
		return out
	}
	now := time.Now().UTC()
	for _, h := range hashes {
		if lookup == nil {
			out.checks = append(out.checks, TxCheck{Hash: h, Status: TxSkipped, CheckedAt: now})
			continue
		}
		if ctx.Err() != nil {
			out.checks = append(out.checks, TxCheck{
				Hash:      h,
				Status:    TxPending,
				Error:     ctx.Err().Error(),
				CheckedAt: now,
			})
			continue
		}
		tx, err := lookup(ctx, h)
		check := classifyTx(h, tx, err)
		out.checks = append(out.checks, check)
		if check.Status == TxConfirmed {
			out.events = append(out.events, tx.Events...)
		}
	}
	return out
}

type txLookupBundle struct {
	checks []TxCheck
	events []ChainEvent
}

func classifyTx(hash string, tx ChainTx, err error) TxCheck {
	check := TxCheck{Hash: hash, CheckedAt: time.Now().UTC()}
	if err != nil {
		msg := err.Error()
		if isTxNotFound(msg) {
			check.Status = TxPending
			return check
		}
		check.Status = TxError
		check.Error = Redact(msg)
		return check
	}
	check.Code = tx.Code
	check.Height = tx.Height
	check.RawLog = Redact(tx.RawLog)
	if tx.Height == "" || tx.Height == "0" {
		check.Status = TxPending
		return check
	}
	if tx.Code != 0 {
		check.Status = TxFailed
		return check
	}
	check.Status = TxConfirmed
	return check
}

func isTxNotFound(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "not found") || strings.Contains(m, "404")
}
