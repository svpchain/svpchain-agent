package phoenix

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/svpchain/svpchain-agent/internal/agent/llm"
	"github.com/svpchain/svpchain-agent/internal/agent/runlog"
)

const (
	kindAgent = "AGENT"
	kindLLM   = "LLM"
	kindTool  = "TOOL"
	attrKind  = "openinference.span.kind"
	attrIn    = "input.value"
	attrOut   = "output.value"
)

// Meta is the run-level context copied onto the root AGENT span.
type Meta struct {
	RunID        string
	SessionID    string
	SessionTitle string
	ChainID      string
	Model        string
	Provider     string
	UserMessage  string
}

// Session is one assistant run's OTLP trace. A nil receiver is a no-op.
type Session struct {
	endpoint string
	http     *http.Client

	mu        sync.Mutex
	traceID   string
	rootID    string
	started   time.Time
	meta      Meta
	promptSHA string
	skills    []string
	spans     []otlpSpan
	root      otlpSpan
}

// Begin starts a root AGENT span. Empty or invalid otlpURL returns nil.
func Begin(otlpURL string, meta Meta) *Session {
	endpoint := ExportURL(otlpURL)
	if endpoint == "" {
		return nil
	}
	now := time.Now()
	s := &Session{
		endpoint: endpoint,
		http:     &http.Client{Timeout: 3 * time.Second},
		traceID:  newID(16),
		rootID:   newID(8),
		started:  now,
		meta:     meta,
	}
	s.root = otlpSpan{
		TraceID:           s.traceID,
		SpanID:            s.rootID,
		Name:              "assistant.run",
		Kind:              1,
		StartTimeUnixNano: nano(now),
		Attributes: []otlpAttr{
			strAttr(attrKind, kindAgent),
			strAttr("session.id", meta.SessionID),
			strAttr("run.id", meta.RunID),
			strAttr("chain_id", meta.ChainID),
			strAttr("llm.provider", meta.Provider),
			strAttr("llm.model_name", meta.Model),
			strAttr(attrIn, runlog.Redact(meta.UserMessage)),
		},
	}
	return s
}

// SetPrompt records the system-prompt fingerprint (never the body).
func (s *Session) SetPrompt(sha256Hex string, skillNames []string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.promptSHA = strings.TrimSpace(sha256Hex)
	s.skills = append([]string(nil), skillNames...)
}

// RecordLLM appends one LLM span under the root. Safe on a nil Session.
func (s *Session) RecordLLM(round int, res llm.ChatResult) {
	if s == nil {
		return
	}
	end := time.Now()
	start := end
	if res.LatencyMs > 0 {
		start = end.Add(-time.Duration(res.LatencyMs) * time.Millisecond)
	}
	model := strings.TrimSpace(res.Model)
	attrs := []otlpAttr{
		strAttr(attrKind, kindLLM),
		strAttr("llm.model_name", model),
		intAttr("llm.round", int64(round)),
		intAttr("llm.token_count.prompt", int64(res.Usage.PromptTokens)),
		intAttr("llm.token_count.completion", int64(res.Usage.CompletionTokens)),
		strAttr(attrOut, runlog.Redact(res.Message.Content)),
	}
	if calls := toolCallSummary(res.Message.ToolCalls); calls != "" {
		attrs = append(attrs, strAttr("llm.tool_calls", calls))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.spans = append(s.spans, otlpSpan{
		TraceID:           s.traceID,
		SpanID:            newID(8),
		ParentSpanID:      s.rootID,
		Name:              "llm.round." + strconv.Itoa(round),
		Kind:              1,
		StartTimeUnixNano: nano(start),
		EndTimeUnixNano:   nano(end),
		Attributes:        attrs,
		Status:            otlpStatus{Code: 1},
	})
}

// RecordTool implements agent.ToolObserver.
func (s *Session) RecordTool(name, args string) func(ok bool, result, errDetail string) {
	if s == nil {
		return func(bool, string, string) {}
	}
	start := time.Now()
	name = strings.TrimSpace(name)
	return func(ok bool, result, errDetail string) {
		end := time.Now()
		out := result
		if !ok {
			out = errDetail
		}
		status := otlpStatus{Code: 1}
		if !ok {
			status = otlpStatus{Code: 2, Message: runlog.Redact(errDetail)}
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		s.spans = append(s.spans, otlpSpan{
			TraceID:           s.traceID,
			SpanID:            newID(8),
			ParentSpanID:      s.rootID,
			Name:              name,
			Kind:              1,
			StartTimeUnixNano: nano(start),
			EndTimeUnixNano:   nano(end),
			Attributes: []otlpAttr{
				strAttr(attrKind, kindTool),
				strAttr("tool.name", name),
				strAttr(attrIn, runlog.Redact(args)),
				strAttr(attrOut, runlog.Redact(out)),
			},
			Status: status,
		})
	}
}

// End closes the root span. outcome is success/failed/… from the runner.
func (s *Session) End(answer string, runErr error) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.root.EndTimeUnixNano = nano(time.Now())
	s.root.Attributes = append(s.root.Attributes,
		strAttr(attrOut, runlog.Redact(answer)),
	)
	if s.promptSHA != "" {
		s.root.Attributes = append(s.root.Attributes, strAttr("prompt.sha256", s.promptSHA))
	}
	if len(s.skills) > 0 {
		s.root.Attributes = append(s.root.Attributes, strAttr("skills", strings.Join(s.skills, ",")))
	}
	if s.meta.SessionTitle != "" {
		s.root.Attributes = append(s.root.Attributes, strAttr("session.title", runlog.Redact(s.meta.SessionTitle)))
	}
	if runErr != nil {
		s.root.Status = otlpStatus{Code: 2, Message: runlog.Redact(runErr.Error())}
		return
	}
	s.root.Status = otlpStatus{Code: 1}
}

// Flush POSTs collected spans to Phoenix. Errors are returned for tests but
// the runner ignores them.
func (s *Session) Flush(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	payload := otlpPayload{
		ResourceSpans: []otlpResourceSpans{{
			Resource: otlpResource{Attributes: []otlpAttr{
				strAttr("service.name", "svpchain-agent"),
			}},
			ScopeSpans: []otlpScopeSpans{{
				Scope: otlpScope{Name: "svpchain-agent", Version: "phoenix"},
				Spans: append([]otlpSpan{s.root}, s.spans...),
			}},
		}},
	}
	endpoint := s.endpoint
	client := s.http
	s.mu.Unlock()
	return postOTLP(ctx, client, endpoint, payload)
}

func toolCallSummary(calls []llm.ToolCall) string {
	if len(calls) == 0 {
		return ""
	}
	type row struct {
		ID   string `json:"id,omitempty"`
		Name string `json:"name"`
		Args string `json:"args,omitempty"`
	}
	out := make([]row, 0, len(calls))
	for _, tc := range calls {
		out = append(out, row{
			ID:   strings.TrimSpace(tc.ID),
			Name: strings.TrimSpace(tc.Function.Name),
			Args: runlog.Redact(tc.Function.Arguments),
		})
	}
	bz, err := json.Marshal(out)
	if err != nil {
		return ""
	}
	return string(bz)
}

func newID(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failure is extraordinary; fall back to time bits.
		now := uint64(time.Now().UnixNano())
		for i := range b {
			b[i] = byte(now >> (uint(i) * 8))
		}
	}
	return hex.EncodeToString(b)
}

func nano(t time.Time) string {
	return strconv.FormatInt(t.UnixNano(), 10)
}
