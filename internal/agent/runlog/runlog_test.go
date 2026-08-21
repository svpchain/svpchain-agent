package runlog

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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

	raw := `{"signed_tx":"abc123payload","to":"svp1x"}`
	got := Redact(raw)
	require.Contains(t, got, `"signed_tx":"[REDACTED]"`)
	require.Contains(t, got, "svp1x")
	require.NotContains(t, got, "abc123payload")
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
	require.Equal(t, OutcomeRejected, classifyOutcome(`Signing declined — the user did not approve "Sign Cosmos transaction". No transaction was signed or broadcast.`, nil))
	require.Equal(t, OutcomeRejected, classifyOutcome(`Declined — the user did not approve "Delegate task". No further action was taken.`, nil))
	require.Equal(t, OutcomeRejected, classifyOutcome(`Write path rejected — signed_tx was altered; pass it verbatim from sign_*. No transaction was signed or broadcast.`, nil))
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
		Message: llm.Message{
			Content: "I'll check positions.",
			ToolCalls: []llm.ToolCall{{
				ID:   "c1",
				Type: "function",
				Function: llm.ToolCallFunction{
					Name:      "broadcast_signed_tx",
					Arguments: `{"signed_tx":"should-not-leak"}`,
				},
			}},
		},
		Model:     "m",
		LatencyMs: 120,
		Usage:     llm.Usage{PromptTokens: 10, CompletionTokens: 3, TotalTokens: 13},
	})
	sess.SetPrompt(PromptSHA256("system body"), []string{"base", "onchain-workflow"})
	sess.Complete("ok", nil)

	runs, err := ReadAll(dir + "/agent_runs.jsonl")
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Len(t, runs[0].LLMRounds, 1)
	require.Equal(t, int64(120), runs[0].LLMRounds[0].LatencyMs)
	require.Equal(t, 10, runs[0].Usage.PromptTokens)
	require.Equal(t, 13, runs[0].Usage.TotalTokens)
	require.Equal(t, "I'll check positions.", runs[0].LLMRounds[0].Reply)
	require.Len(t, runs[0].LLMRounds[0].ToolCalls, 1)
	require.Equal(t, "broadcast_signed_tx", runs[0].LLMRounds[0].ToolCalls[0].Name)
	require.Contains(t, runs[0].LLMRounds[0].ToolCalls[0].Args, "[REDACTED]")
	require.NotContains(t, runs[0].LLMRounds[0].ToolCalls[0].Args, "should-not-leak")
	require.Equal(t, PromptSHA256("system body"), runs[0].PromptSHA256)
	require.Equal(t, []string{"base", "onchain-workflow"}, runs[0].Skills)
	require.NotContains(t, string(mustJSON(t, runs[0])), "system body")
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
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

func TestClampRecentLimit(t *testing.T) {
	require.Equal(t, defaultRecentLimit, ClampRecentLimit(0))
	require.Equal(t, defaultRecentLimit, ClampRecentLimit(-1))
	require.Equal(t, 3, ClampRecentLimit(3))
	require.Equal(t, maxRecentLimit, ClampRecentLimit(maxRecentLimit+50))
}

func TestReadRecentNewestFirst(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/agent_runs.jsonl"
	SetPathOverride(path)
	t.Cleanup(func() { SetPathOverride("") })

	rec := New(true)
	for _, msg := range []string{"first", "second", "third"} {
		sess := rec.Begin(Meta{ChainID: "svp-2517-1", UserMessage: msg})
		sess.Complete(msg, nil)
	}

	newest, err := ReadRecentNewestFirst(2)
	require.NoError(t, err)
	require.Len(t, newest, 2)
	require.Equal(t, "third", newest[0].UserMessage)
	require.Equal(t, "second", newest[1].UserMessage)
}

func TestDelete_removesOneRun(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/agent_runs.jsonl"
	SetPathOverride(path)
	t.Cleanup(func() { SetPathOverride("") })

	rec := New(true)
	var ids []string
	for _, msg := range []string{"first", "second", "third"} {
		sess := rec.Begin(Meta{ChainID: "svp-2517-1", UserMessage: msg})
		ids = append(ids, sess.RunID())
		sess.Complete(msg, nil)
	}

	require.NoError(t, Delete(ids[1]))
	runs, err := ReadAll(path)
	require.NoError(t, err)
	require.Len(t, runs, 2)
	require.Equal(t, "first", runs[0].UserMessage)
	require.Equal(t, "third", runs[1].UserMessage)

	err = Delete(ids[1])
	require.Error(t, err)
	require.Contains(t, err.Error(), "not found")
	require.Error(t, Delete(""))
}

func TestDeleteAll_removesFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/agent_runs.jsonl"
	SetPathOverride(path)
	t.Cleanup(func() { SetPathOverride("") })

	rec := New(true)
	sess := rec.Begin(Meta{ChainID: "svp-2517-1", UserMessage: "hello"})
	sess.Complete("ok", nil)
	require.NoError(t, DeleteAll())

	runs, err := ReadAll(path)
	require.NoError(t, err)
	require.Empty(t, runs)
	_, statErr := os.Stat(path)
	require.True(t, os.IsNotExist(statErr))
	require.NoError(t, DeleteAll(), "clearing an empty log must succeed")
}

func TestSession_recordsSessionID(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/agent_runs.jsonl"
	SetPathOverride(path)
	t.Cleanup(func() { SetPathOverride("") })

	rec := New(true)
	sess := rec.Begin(Meta{
		ChainID:      "svp-2517-1",
		UserMessage:  "hi",
		SessionID:    "sess-1",
		SessionTitle: "查价格",
	})
	sess.Complete("ok", nil)
	runs, err := ReadAll(path)
	require.NoError(t, err)
	require.Equal(t, "sess-1", runs[0].SessionID)
	require.Equal(t, "查价格", runs[0].SessionTitle)
}

func TestLCDQueryHash(t *testing.T) {
	h := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	require.Equal(t, strings.ToUpper(h), LCDQueryHash("0x"+h))
	require.Equal(t, strings.ToUpper(h), LCDQueryHash(h))
}

func TestVerifyTxs_andRecheck(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/agent_runs.jsonl"
	SetPathOverride(path)
	t.Cleanup(func() { SetPathOverride("") })

	hash := "0xabcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	rec := New(true)
	sess := rec.Begin(Meta{ChainID: "svp-2517-1", UserMessage: "broadcast"})
	build := sess.RecordTool("build_bank_send", `{"recipient":"svp1abc","amount":"12"}`)
	build(true, `{"summary":{"recipient":"svp1abc"}}`, "")
	done := sess.RecordTool("broadcast_signed_tx", `{}`)
	done(true, `{"tx_hash":"`+hash+`"}`, "")

	calls := 0
	lookup := func(_ context.Context, got string) (ChainTx, error) {
		calls++
		require.Equal(t, hash, got)
		if calls == 1 {
			return ChainTx{}, fmt.Errorf("chain REST: 404: not found")
		}
		return ChainTx{
			Code:   0,
			Height: "12",
			Events: []ChainEvent{{
				Type:  "transfer",
				Attrs: map[string]string{"recipient": "svp1abc"},
			}},
		}, nil
	}
	sess.VerifyTxs(context.Background(), lookup)
	require.Equal(t, TxPending, sess.run.TxChecks[0].Status)
	require.Equal(t, IntentUnobserved, sess.run.IntentChecks[0].Status)
	sess.Complete("ok", nil)

	rechecked, err := RecheckTxs(context.Background(), sess.RunID(), lookup)
	require.NoError(t, err)
	require.Equal(t, TxConfirmed, rechecked.TxChecks[0].Status)
	require.Equal(t, "12", rechecked.TxChecks[0].Height)
	require.Equal(t, IntentMatched, rechecked.IntentChecks[0].Status)

	skipped, err := RecheckTxs(context.Background(), sess.RunID(), nil)
	require.NoError(t, err)
	require.Equal(t, TxSkipped, skipped.TxChecks[0].Status)
}

func TestClassifyTx_failedAndError(t *testing.T) {
	hash := "0x" + strings.Repeat("ab", 32)
	failed := classifyTx(hash, ChainTx{Code: 5, Height: "9", RawLog: "out of gas"}, nil)
	require.Equal(t, TxFailed, failed.Status)
	require.Equal(t, uint32(5), failed.Code)

	errored := classifyTx(hash, ChainTx{}, fmt.Errorf("connection refused"))
	require.Equal(t, TxError, errored.Status)
}
