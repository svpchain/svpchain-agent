package phoenix

import (
	"context"
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
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/traces", r.URL.Path)
		require.Equal(t, otlpContentType, r.Header.Get("Content-Type"))
		require.Equal(t, projectName, r.Header.Get(projectHeader))
		var err error
		body, err = io.ReadAll(r.Body)
		require.NoError(t, err)
		w.Header().Set("Content-Type", otlpContentType)
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

	got := string(body)
	require.NotEmpty(t, body)
	require.NotContains(t, got, "should-not-leak")
	require.NotContains(t, got, "payload-secret")
	require.Contains(t, got, "[REDACTED_KEY]")
	require.Contains(t, got, "[REDACTED]")
	require.Contains(t, got, "AGENT")
	require.Contains(t, got, "LLM")
	require.Contains(t, got, "TOOL")
	require.Contains(t, got, "prompt.sha256")
	require.Contains(t, got, projectName)
	require.Contains(t, got, "assistant.run")
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
