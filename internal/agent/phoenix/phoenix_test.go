package phoenix

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/agent/llm"
)

func TestExportURL(t *testing.T) {
	require.Empty(t, ExportURL(""))
	require.Empty(t, ExportURL("   "))
	require.Equal(t, DefaultURL, ExportURL("http://127.0.0.1:6006"))
	require.Equal(t, DefaultURL, ExportURL("127.0.0.1:6006"))
	require.Equal(t, DefaultURL, ExportURL("http://127.0.0.1:6006/v1/traces"))
	require.Empty(t, ExportURL("ftp://127.0.0.1:6006"))
	require.Nil(t, Begin("", Meta{}))
	require.Nil(t, Begin("not a url", Meta{}))
}

func TestSession_exportsRedactedSpans(t *testing.T) {
	var got otlpPayload
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/traces", r.URL.Path)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(body, &got))
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	s := Begin(srv.URL, Meta{
		RunID:       "run-1",
		SessionID:   "sess-1",
		ChainID:     "svp-2517-1",
		Model:       "deepseek-v4-flash",
		UserMessage: "sign this 0x" + strings.Repeat("ab", 32),
	})
	require.NotNil(t, s)
	s.SetPrompt("abc123", []string{"base", "onchain-workflow"})
	s.RecordLLM(1, llm.ChatResult{
		Message: llm.Message{
			Content: "calling sign",
			ToolCalls: []llm.ToolCall{{
				ID: "c1",
				Function: llm.ToolCallFunction{
					Name:      "broadcast_evm_tx",
					Arguments: `{"signed_tx":"should-not-leak"}`,
				},
			}},
		},
		Usage:     llm.Usage{PromptTokens: 10, CompletionTokens: 3},
		LatencyMs: 12,
	})
	done := s.RecordTool("broadcast_evm_tx", `{"signed_tx":"payload-secret"}`)
	done(false, "", "denied")
	s.End("stopped", nil)
	require.NoError(t, s.Flush(context.Background()))

	require.Len(t, got.ResourceSpans, 1)
	spans := got.ResourceSpans[0].ScopeSpans[0].Spans
	require.GreaterOrEqual(t, len(spans), 3)
	blob, err := json.Marshal(got)
	require.NoError(t, err)
	require.NotContains(t, string(blob), "should-not-leak")
	require.NotContains(t, string(blob), "payload-secret")
	require.Contains(t, string(blob), "[REDACTED_KEY]")
	require.Contains(t, string(blob), "[REDACTED]")
	require.Contains(t, string(blob), "AGENT")
	require.Contains(t, string(blob), "LLM")
	require.Contains(t, string(blob), "TOOL")
	require.Contains(t, string(blob), "prompt.sha256")
}

func TestSession_flushErrorDoesNotPanic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)
	s := Begin(srv.URL, Meta{UserMessage: "hi"})
	s.End("ok", nil)
	err := s.Flush(context.Background())
	require.Error(t, err)
}
