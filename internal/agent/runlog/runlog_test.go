package runlog

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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
