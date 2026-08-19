package runlog

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/agent/llm"
)

func TestRedact_truncateAndKey(t *testing.T) {
	long := strings.Repeat("a", maxFieldLen+10)
	require.True(t, len(Redact(long)) <= maxFieldLen+3)

	key := "0x" + strings.Repeat("ab", 32)
	require.Contains(t, Redact("key="+key), "[REDACTED_KEY]")
}

func TestExtractTxHashes_json(t *testing.T) {
	hash := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	result := `{"tx_hash":"0x` + hash + `"}`
	hashes := ExtractTxHashes("broadcast_evm_tx", result)
	require.Len(t, hashes, 1)
	require.Equal(t, "0x"+hash, hashes[0])
}

func TestClassifyOutcome(t *testing.T) {
	require.Equal(t, OutcomeRejected, classifyOutcome("Transfer rejected — x", nil))
	require.Equal(t, OutcomeStopped, classifyOutcome("tool failed — err. Stopped without further action.", nil))
	require.Equal(t, OutcomeSuccess, classifyOutcome("done", nil))
}

func TestSession_RecordLLMRound(t *testing.T) {
	dir := t.TempDir()
	SetPathOverride(dir + "/agent_runs.jsonl")
	t.Cleanup(func() { SetPathOverride("") })

	rec := New(true)
	sess := rec.Begin(Meta{ChainID: "svp-2517-1", Model: "m", UserMessage: "hi"})
	sess.RecordLLMRound(1, llm.ChatResult{
		Message:   llm.Message{Content: "ok"},
		Model:     "m",
		LatencyMs: 120,
		Usage:     llm.Usage{PromptTokens: 10, CompletionTokens: 3, TotalTokens: 13},
	})
	sess.Complete("ok", nil)

	runs, err := ReadAll(dir + "/agent_runs.jsonl")
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Len(t, runs[0].LLMRounds, 1)
	require.Equal(t, int64(120), runs[0].LLMRounds[0].LatencyMs)
	require.Equal(t, 10, runs[0].Usage.PromptTokens)
	require.Equal(t, 13, runs[0].Usage.TotalTokens)
}

func TestRecorder_append(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/agent_runs.jsonl"
	SetPathOverride(path)
	t.Cleanup(func() { SetPathOverride("") })

	rec := New(true)
	sess := rec.Begin(Meta{ChainID: "svp-2517-1", UserMessage: "hello"})
	require.NotNil(t, sess)
	sess.SetRound(1)
	finish := sess.RecordTool("whoami", `{}`)
	finish(true, `{"owner":"svp1test"}`, "")
	sess.Complete("ok", nil)

	runs, err := ReadAll(path)
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, OutcomeSuccess, runs[0].Outcome)
	require.Equal(t, "svp-2517-1", runs[0].ChainID)
	require.Len(t, runs[0].Steps, 1)
}
