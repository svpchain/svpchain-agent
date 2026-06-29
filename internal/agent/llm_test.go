package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// sseResponse writes a minimal text/event-stream from the given raw frames.
func sseResponse(w http.ResponseWriter, frames ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	fl, _ := w.(http.Flusher)
	for _, f := range frames {
		_, _ = io.WriteString(w, "data: "+f+"\n\n")
		if fl != nil {
			fl.Flush()
		}
	}
}

func TestChatOpenAI_streamsContentAndToolCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer k" {
			t.Errorf("auth header = %q", got)
		}
		// content split across two frames, then a tool call sharded across frames.
		sseResponse(w,
			`{"choices":[{"delta":{"content":"Hel"}}]}`,
			`{"choices":[{"delta":{"content":"lo"}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"signer_whoami"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"a\":"}}]}}]}`,
			`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"1}"}}]}}]}`,
			`[DONE]`,
		)
	}))
	defer srv.Close()

	c := NewLLMClient(LLMConfig{APIKey: "k", BaseURL: srv.URL, Model: "m", Provider: "openai"})
	var deltas strings.Builder
	msg, err := c.Chat(context.Background(), []llmMessage{{Role: "user", Content: "hi"}}, nil, func(s string) {
		deltas.WriteString(s)
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if msg.Content != "Hello" {
		t.Errorf("content = %q, want Hello", msg.Content)
	}
	if deltas.String() != "Hello" {
		t.Errorf("streamed deltas = %q, want Hello", deltas.String())
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(msg.ToolCalls))
	}
	tc := msg.ToolCalls[0]
	if tc.ID != "call_1" || tc.Function.Name != "signer_whoami" || tc.Function.Arguments != `{"a":1}` {
		t.Errorf("tool call = %+v", tc)
	}
}

func TestChatAnthropic_translatesRequestAndDecodesStream(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "k" {
			t.Errorf("x-api-key = %q", got)
		}
		if got := r.Header.Get("anthropic-version"); got != anthropicVersion {
			t.Errorf("anthropic-version = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		sseResponse(w,
			`{"type":"content_block_start","index":0,"content_block":{"type":"text"}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hi "}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"there"}}`,
			`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tu_1","name":"whoami"}}`,
			`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"x\":2}"}}`,
			`{"type":"message_stop"}`,
		)
	}))
	defer srv.Close()

	c := NewLLMClient(LLMConfig{APIKey: "k", BaseURL: srv.URL, Model: "claude-x", Provider: "anthropic"})
	tools := []llmTool{{Type: "function", Function: llmFunction{
		Name: "whoami", Description: "who", Parameters: map[string]any{"type": "object"},
	}}}
	msgs := []llmMessage{
		{Role: "system", Content: "sys-prompt"},
		{Role: "user", Content: "hi"},
		{Role: "tool", ToolCallID: "tu_0", Content: "result-data"},
	}
	var deltas strings.Builder
	msg, err := c.Chat(context.Background(), msgs, tools, func(s string) { deltas.WriteString(s) })
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}

	// Request translation: system hoisted, tools use input_schema, tool result block.
	if gotBody["system"] != "sys-prompt" {
		t.Errorf("system = %v", gotBody["system"])
	}
	if _, ok := gotBody["max_tokens"]; !ok {
		t.Errorf("max_tokens missing")
	}
	rtools, _ := gotBody["tools"].([]any)
	if len(rtools) != 1 {
		t.Fatalf("tools = %v", gotBody["tools"])
	}
	if tool0, _ := rtools[0].(map[string]any); tool0["input_schema"] == nil || tool0["name"] != "whoami" {
		t.Errorf("translated tool = %v", rtools[0])
	}
	// the tool message must become a user tool_result block
	rmsgs, _ := gotBody["messages"].([]any)
	foundToolResult := false
	for _, m := range rmsgs {
		mm, _ := m.(map[string]any)
		if mm["role"] != "user" {
			continue
		}
		if content, ok := mm["content"].([]any); ok {
			for _, b := range content {
				if bb, _ := b.(map[string]any); bb["type"] == "tool_result" && bb["tool_use_id"] == "tu_0" {
					foundToolResult = true
				}
			}
		}
	}
	if !foundToolResult {
		t.Errorf("tool_result block not found in %v", rmsgs)
	}

	// Response decode: text + tool_use → llmMessage.
	if msg.Content != "Hi there" || deltas.String() != "Hi there" {
		t.Errorf("content = %q deltas = %q", msg.Content, deltas.String())
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].ID != "tu_1" ||
		msg.ToolCalls[0].Function.Name != "whoami" || msg.ToolCalls[0].Function.Arguments != `{"x":2}` {
		t.Errorf("tool calls = %+v", msg.ToolCalls)
	}
}

func TestChat_retriesBeforeFirstDelta(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			http.Error(w, "overloaded", http.StatusServiceUnavailable) // 503 → retryable
			return
		}
		sseResponse(w, `{"choices":[{"delta":{"content":"ok"}}]}`, `[DONE]`)
	}))
	defer srv.Close()

	c := NewLLMClient(LLMConfig{APIKey: "k", BaseURL: srv.URL, Model: "m", Provider: "openai"})
	msg, err := c.Chat(context.Background(), []llmMessage{{Role: "user", Content: "hi"}}, nil, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if msg.Content != "ok" {
		t.Errorf("content = %q, want ok", msg.Content)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("server calls = %d, want 2 (one 503 + one success)", got)
	}
}

func TestChat_noRetryAfterDeltaEmitted(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		// Emit one delta, then break the stream by sending malformed/truncated data and
		// closing — the client has already surfaced a token, so it must NOT retry.
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fl, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, "data: "+`{"choices":[{"delta":{"content":"partial"}}]}`+"\n\n")
		if fl != nil {
			fl.Flush()
		}
		// Hijack and close abruptly to simulate a mid-stream connection drop.
		if hj, ok := w.(http.Hijacker); ok {
			conn, _, err := hj.Hijack()
			if err == nil {
				_ = conn.Close()
			}
		}
	}))
	defer srv.Close()

	c := NewLLMClient(LLMConfig{APIKey: "k", BaseURL: srv.URL, Model: "m", Provider: "openai"})
	var deltas strings.Builder
	_, err := c.Chat(context.Background(), []llmMessage{{Role: "user", Content: "hi"}}, nil, func(s string) {
		deltas.WriteString(s)
	})
	// The run may or may not error depending on how the drop surfaces, but the key
	// invariant is: exactly one attempt (no retry once "partial" was emitted).
	_ = err
	if deltas.String() != "partial" {
		t.Errorf("deltas = %q, want partial", deltas.String())
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("server calls = %d, want 1 (no retry after delta)", got)
	}
}

func TestNormalized_inferProviderAndDefaults(t *testing.T) {
	cases := []struct {
		in           LLMConfig
		wantProvider string
		wantBase     string
	}{
		{LLMConfig{}, providerOpenAI, defaultLLMBaseURL},
		{LLMConfig{BaseURL: "https://api.anthropic.com"}, providerAnthropic, "https://api.anthropic.com"},
		{LLMConfig{Provider: "anthropic"}, providerAnthropic, anthropicBaseURL},
		{LLMConfig{Provider: "OpenAI", BaseURL: "https://x/"}, providerOpenAI, "https://x"},
	}
	for i, tc := range cases {
		got := tc.in.normalized()
		if got.Provider != tc.wantProvider || got.BaseURL != tc.wantBase {
			t.Errorf("case %d: got provider=%q base=%q, want %q / %q",
				i, got.Provider, got.BaseURL, tc.wantProvider, tc.wantBase)
		}
	}
}
