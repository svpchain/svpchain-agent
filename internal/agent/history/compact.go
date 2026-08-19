package history

import (
	"context"
	"fmt"
	"strings"

	"github.com/svpchain/svpchain-agent/internal/agent/llm"
)

// keepRecentTurns is how many most-recent turns survive compaction verbatim.
const keepRecentTurns = 4

// SummarizeFunc produces a summary for rendered conversation text.
type SummarizeFunc func(ctx context.Context, text string) (string, error)

// SummarySystemPrompt instructs the LLM how to compact old conversation turns.
// Hard facts must survive verbatim: this feeds a trading assistant.
const SummarySystemPrompt = `You compact old conversation turns of an on-chain trading assistant into a short summary.

Rules:
- Preserve VERBATIM: addresses (svp1…/0x…), transaction hashes, market symbols, order ids, amounts, prices.
- Preserve completed / failed / rejected on-chain actions and their results.
- Preserve explicit user preferences and constraints (e.g. size limits, allowed markets).
- Drop pleasantries, intermediate reasoning, and raw tool output details.
- Reply with the summary only, no preamble. Keep it under 400 words.`

// CompactIfNeeded summarizes old turns when the session context exceeds the
// token budget. Returns true when a new summary was produced.
func (s *Store) CompactIfNeeded(ctx context.Context, id string, budgetTokens int, summarize SummarizeFunc) (bool, error) {
	if !s.Enabled() || id == "" || summarize == nil {
		return false, nil
	}

	s.mu.Lock()
	entries, err := s.entriesLocked(id)
	if err != nil {
		s.mu.Unlock()
		return false, err
	}
	sum, hasSum := s.loadSummaryLocked(id)
	offset := 0
	if hasSum && sum.Upto > 0 && sum.Upto <= len(entries) {
		offset = sum.Upto
	}
	live := entries[offset:]
	msgs := make([]llm.Message, 0, len(live))
	for _, e := range live {
		msgs = append(msgs, e.Msg)
	}
	est := EstimateTokens(msgs)
	if hasSum {
		est += len(sum.Text) / 3
	}
	if budgetTokens <= 0 || est <= budgetTokens {
		s.mu.Unlock()
		return false, nil
	}

	// Compact everything except the last keepRecentTurns turns.
	starts := turnStarts(msgs)
	if len(starts) <= keepRecentTurns {
		s.mu.Unlock()
		return false, nil // nothing old enough to compact
	}
	cut := starts[len(starts)-keepRecentTurns]
	if cut <= 0 {
		s.mu.Unlock()
		return false, nil
	}
	text := renderForSummary(sum.Text, msgs[:cut])
	newUpto := offset + cut
	s.mu.Unlock()

	// LLM call happens outside the lock; a concurrent Append only adds entries
	// after newUpto, so the cut stays valid.
	summary, err := summarize(ctx, text)
	if err != nil {
		return false, fmt.Errorf("compact history: %w", err)
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return false, fmt.Errorf("compact history: empty summary")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	return true, s.saveSummaryLocked(id, summaryState{Upto: newUpto, Text: summary})
}

// renderForSummary flattens messages (plus any previous summary) into text for
// the summarizer. Keys are redacted; long tool results are clipped.
func renderForSummary(prevSummary string, msgs []llm.Message) string {
	var b strings.Builder
	if strings.TrimSpace(prevSummary) != "" {
		b.WriteString("[Previous summary]\n")
		b.WriteString(prevSummary)
		b.WriteString("\n\n[Conversation to fold in]\n")
	}
	for _, m := range msgs {
		switch m.Role {
		case "user":
			b.WriteString("User: ")
			b.WriteString(redactKeys(m.Content))
		case "assistant":
			b.WriteString("Assistant: ")
			b.WriteString(m.Content)
			for _, tc := range m.ToolCalls {
				fmt.Fprintf(&b, "\n[called %s(%s)]", tc.Function.Name, clip(tc.Function.Arguments, 300))
			}
		case "tool":
			fmt.Fprintf(&b, "Tool result: %s", clip(m.Content, 500))
		default:
			continue
		}
		b.WriteString("\n")
	}
	return b.String()
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
