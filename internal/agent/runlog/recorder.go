package runlog

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/svpchain/svpchain-agent/internal/agent/guard"
	"github.com/svpchain/svpchain-agent/internal/prefs"
)

const logFileName = "agent_runs.jsonl"

var pathOverride string

// SetPathOverride redirects the JSONL file for tests.
func SetPathOverride(path string) {
	pathOverride = path
}

// LogPath returns the agent run log file path (next to prefs.json).
func LogPath() string {
	if pathOverride != "" {
		return pathOverride
	}
	p := prefs.Path()
	if p == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(p), logFileName)
}

// Outcome classifies how a run ended.
type Outcome string

const (
	OutcomeSuccess   Outcome = "success"
	OutcomeFailed    Outcome = "failed"
	OutcomeStopped   Outcome = "stopped"
	OutcomeRejected  Outcome = "rejected"
	OutcomeCancelled Outcome = "cancelled"
)

// Step is one trace event inside a run.
type Step struct {
	At        time.Time `json:"at"`
	Kind      string    `json:"kind"`
	Round     int       `json:"round,omitempty"`
	Tool      string    `json:"tool,omitempty"`
	Args      string    `json:"args,omitempty"`
	OK        *bool     `json:"ok,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	Result    string    `json:"result,omitempty"`
	ElapsedMs int64     `json:"elapsed_ms,omitempty"`
}

// Run is one assistant execution trace.
type Run struct {
	RunID       string    `json:"run_id"`
	StartedAt   time.Time `json:"started_at"`
	FinishedAt  time.Time `json:"finished_at,omitempty"`
	ChainID     string    `json:"chain_id"`
	RemoteURL   string    `json:"remote_url"`
	Model       string    `json:"model,omitempty"`
	Provider    string    `json:"provider,omitempty"`
	UserMessage string    `json:"user_message"`
	Outcome     Outcome   `json:"outcome"`
	Answer      string    `json:"answer,omitempty"`
	Error       string    `json:"error,omitempty"`
	TxHashes    []string  `json:"tx_hashes,omitempty"`
	RoundCount  int       `json:"round_count"`
	Steps       []Step    `json:"steps"`
}

// Meta describes a run before execution starts.
type Meta struct {
	ChainID     string
	RemoteURL   string
	Model       string
	Provider    string
	UserMessage string
}

// Recorder appends completed runs to a local JSONL file.
type Recorder struct {
	path    string
	enabled bool
}

// New returns a recorder. When enabled is false, Begin is a no-op.
func New(enabled bool) *Recorder {
	return &Recorder{path: LogPath(), enabled: enabled && LogPath() != ""}
}

// Enabled reports whether runs will be persisted.
func (r *Recorder) Enabled() bool {
	return r != nil && r.enabled
}

// Begin starts tracing a run.
func (r *Recorder) Begin(meta Meta) *Session {
	if r == nil || !r.enabled {
		return nil
	}
	return &Session{
		recorder: r,
		run: Run{
			RunID:       uuid.NewString(),
			StartedAt:   time.Now().UTC(),
			ChainID:     strings.TrimSpace(meta.ChainID),
			RemoteURL:   strings.TrimSpace(meta.RemoteURL),
			Model:       strings.TrimSpace(meta.Model),
			Provider:    strings.TrimSpace(meta.Provider),
			UserMessage: Redact(meta.UserMessage),
		},
	}
}

// Session accumulates steps for one run.
type Session struct {
	recorder *Recorder
	run      Run
	round    int
}

// SetRound updates the current LLM iteration (1-based).
func (s *Session) SetRound(n int) {
	if s == nil {
		return
	}
	s.round = n
}

// RecordStep appends a non-tool progress event.
func (s *Session) RecordStep(kind, title, detail string) {
	if s == nil {
		return
	}
	msg := strings.TrimSpace(title)
	if detail != "" {
		if msg != "" {
			msg += "\n" + detail
		} else {
			msg = detail
		}
	}
	s.run.Steps = append(s.run.Steps, Step{
		At:     time.Now().UTC(),
		Kind:   kind,
		Round:  s.round,
		Detail: Redact(msg),
	})
}

// RecordTool logs a tool invocation and returns a callback to mark completion.
func (s *Session) RecordTool(name, args string) func(ok bool, result, errDetail string) {
	if s == nil {
		return func(bool, string, string) {}
	}
	start := time.Now()
	name = strings.TrimSpace(name)
	s.run.Steps = append(s.run.Steps, Step{
		At:    start,
		Kind:  "tool",
		Round: s.round,
		Tool:  name,
		Args:  Redact(args),
	})
	idx := len(s.run.Steps) - 1
	return func(ok bool, result, errDetail string) {
		elapsed := time.Since(start).Milliseconds()
		s.run.Steps[idx].OK = &ok
		s.run.Steps[idx].ElapsedMs = elapsed
		if ok {
			s.run.Steps[idx].Result = Redact(result)
			for _, h := range ExtractTxHashes(name, result) {
				s.appendTxHash(h)
			}
		} else {
			s.run.Steps[idx].Detail = Redact(errDetail)
		}
	}
}

func (s *Session) appendTxHash(h string) {
	for _, existing := range s.run.TxHashes {
		if existing == h {
			return
		}
	}
	s.run.TxHashes = append(s.run.TxHashes, h)
}

// Complete finalizes the run and appends it to the JSONL log.
func (s *Session) Complete(answer string, err error) {
	if s == nil || s.recorder == nil {
		return
	}
	s.run.FinishedAt = time.Now().UTC()
	s.run.RoundCount = s.round
	s.run.Answer = Redact(answer)
	s.run.Outcome = classifyOutcome(answer, err)
	if err != nil {
		s.run.Error = Redact(err.Error())
	}
	_ = s.recorder.append(s.run)
}

func classifyOutcome(answer string, err error) Outcome {
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return OutcomeCancelled
		}
		return OutcomeFailed
	}
	lower := strings.ToLower(answer)
	if strings.Contains(lower, "transfer rejected") {
		return OutcomeRejected
	}
	if strings.Contains(lower, "stopped without further action") || strings.Contains(answer, " failed — ") {
		return OutcomeStopped
	}
	return OutcomeSuccess
}

// ClassifyGuardError maps guard rejections for eval scoring.
func ClassifyGuardError(err error) Outcome {
	if err == nil {
		return OutcomeSuccess
	}
	var rej *guard.Rejection
	if errors.As(err, &rej) {
		return OutcomeRejected
	}
	return OutcomeFailed
}

func (r *Recorder) append(run Run) error {
	if r.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	line, err := json.Marshal(run)
	if err != nil {
		return err
	}
	_, err = f.Write(append(line, '\n'))
	return err
}
