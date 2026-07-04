package history

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/svpchain/svpchain-agent/internal/agent/llm"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	SetDirOverride(dir)
	t.Cleanup(func() { SetDirOverride("") })
	return Shared()
}

func TestStore_CreateAppendContext(t *testing.T) {
	s := newTestStore(t)

	sess, err := s.Create("svp-2517-1")
	require.NoError(t, err)

	err = s.Append(sess.ID, "run-1", []llm.Message{
		{Role: "user", Content: "check BTC price"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "c1", Type: "function", Function: llm.ToolCallFunction{Name: "get_price", Arguments: `{}`}}}},
		{Role: "tool", ToolCallID: "c1", Content: `{"price":"64000"}`},
		{Role: "assistant", Content: "BTC is 64000"},
	})
	require.NoError(t, err)

	prior, err := s.Context(sess.ID)
	require.NoError(t, err)
	require.Len(t, prior, 4)
	require.Equal(t, "user", prior[0].Role)
	require.Equal(t, "BTC is 64000", prior[3].Content)

	// Title derived from the first user message.
	list := s.List()
	require.Len(t, list, 1)
	require.Equal(t, "check BTC price", list[0].Title)

	cur, ok := s.Current()
	require.True(t, ok)
	require.Equal(t, sess.ID, cur.ID)
}

func TestStore_UserKeyRedaction(t *testing.T) {
	s := newTestStore(t)
	sess, err := s.Create("svp-2517-1")
	require.NoError(t, err)

	key := "0x" + strings.Repeat("ab", 32)
	require.NoError(t, s.Append(sess.ID, "", []llm.Message{{Role: "user", Content: "my key is " + key}}))

	prior, err := s.Context(sess.ID)
	require.NoError(t, err)
	require.NotContains(t, prior[0].Content, key)
	require.Contains(t, prior[0].Content, "[REDACTED_KEY]")
}

func TestStore_ToolResultProjection(t *testing.T) {
	s := newTestStore(t)
	sess, err := s.Create("svp-2517-1")
	require.NoError(t, err)

	big := strings.Repeat("x", toolResultKeepLen+500)
	require.NoError(t, s.Append(sess.ID, "run-9", []llm.Message{
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "c1", Type: "function", Function: llm.ToolCallFunction{Name: "get_candles"}}}},
		{Role: "tool", ToolCallID: "c1", Content: big},
	}))

	entries, err := s.Entries(sess.ID)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	require.Less(t, len(entries[1].Msg.Content), len(big))
	require.Contains(t, entries[1].Msg.Content, "[truncated")
	require.NotEmpty(t, entries[1].FullRef)
}

func TestStore_SwitchAndDelete(t *testing.T) {
	s := newTestStore(t)
	s1, err := s.Create("svp-2517-1")
	require.NoError(t, err)
	s2, err := s.Create("svp-2517-1")
	require.NoError(t, err)

	cur, _ := s.Current()
	require.Equal(t, s2.ID, cur.ID)

	require.NoError(t, s.SetCurrent(s1.ID))
	cur, _ = s.Current()
	require.Equal(t, s1.ID, cur.ID)

	require.NoError(t, s.Delete(s1.ID))
	_, ok := s.Current()
	require.False(t, ok)
	require.Len(t, s.List(), 1)
}

func TestRepairPairing(t *testing.T) {
	msgs := []llm.Message{
		{Role: "user", Content: "do it"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{
			{ID: "a", Type: "function", Function: llm.ToolCallFunction{Name: "t1"}},
			{ID: "b", Type: "function", Function: llm.ToolCallFunction{Name: "t2"}},
		}},
		{Role: "tool", ToolCallID: "a", Content: "ok"},
		// "b" has no result (run stopped mid-way).
	}
	out := RepairPairing(msgs)
	require.Len(t, out, 4)
	require.Equal(t, "b", out[3].ToolCallID)
	require.Equal(t, "(not executed)", out[3].Content)

	// Orphan tool result gets dropped.
	orphan := RepairPairing([]llm.Message{{Role: "tool", ToolCallID: "zz", Content: "x"}})
	require.Empty(t, orphan)
}

func TestCompactIfNeeded(t *testing.T) {
	s := newTestStore(t)
	sess, err := s.Create("svp-2517-1")
	require.NoError(t, err)

	// 8 turns of filler so the estimate exceeds a tiny budget.
	filler := strings.Repeat("w ", 300)
	for i := 0; i < 8; i++ {
		require.NoError(t, s.Append(sess.ID, "", []llm.Message{
			{Role: "user", Content: "question " + filler},
			{Role: "assistant", Content: "answer " + filler},
		}))
	}

	var summarizedInput string
	summarize := func(_ context.Context, text string) (string, error) {
		summarizedInput = text
		return "SUMMARY: traded BTC, tx 0xabc", nil
	}

	compacted, err := s.CompactIfNeeded(context.Background(), sess.ID, 100, summarize)
	require.NoError(t, err)
	require.True(t, compacted)
	require.Contains(t, summarizedInput, "question")

	prior, err := s.Context(sess.ID)
	require.NoError(t, err)
	require.Contains(t, prior[0].Content, "SUMMARY: traded BTC")
	require.Equal(t, "assistant", prior[1].Role) // alternation ack
	// The last keepRecentTurns turns survive verbatim (2 msgs each).
	require.Len(t, prior, 2+keepRecentTurns*2)

	// Second compaction folds the previous summary in.
	compacted, err = s.CompactIfNeeded(context.Background(), sess.ID, 100, summarize)
	require.NoError(t, err)
	if compacted {
		require.Contains(t, summarizedInput, "[Previous summary]")
	}
}

func TestCompactIfNeeded_underBudgetNoop(t *testing.T) {
	s := newTestStore(t)
	sess, err := s.Create("svp-2517-1")
	require.NoError(t, err)
	require.NoError(t, s.Append(sess.ID, "", []llm.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
	}))
	compacted, err := s.CompactIfNeeded(context.Background(), sess.ID, 100000, func(context.Context, string) (string, error) {
		t.Fatal("summarize must not be called under budget")
		return "", nil
	})
	require.NoError(t, err)
	require.False(t, compacted)
}
