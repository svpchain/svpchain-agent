package history

import (
	"github.com/svpchain/svpchain-agent/internal/agent/llm"
)

// DefaultContextWindow is assumed when the user has not configured one.
const DefaultContextWindow = 64000

// ContextBudgetTokens converts a model context window into the token budget
// reserved for conversation history (the rest is left for the system prompt,
// tools schema, and the reply).
func ContextBudgetTokens(contextWindow int) int {
	if contextWindow <= 0 {
		contextWindow = DefaultContextWindow
	}
	return contextWindow * 7 / 10
}

// EstimateTokens approximates the token count of messages. bytes/3 is exact-ish
// for CJK (3 bytes ≈ 1 token) and overestimates English (safe direction).
func EstimateTokens(msgs []llm.Message) int {
	total := 0
	for _, m := range msgs {
		total += len(m.Content)
		for _, tc := range m.ToolCalls {
			total += len(tc.Function.Name) + len(tc.Function.Arguments)
		}
	}
	return total / 3
}

// turnStarts returns indexes of messages that begin a turn (real user input;
// tool results use role "tool" so they never match).
func turnStarts(msgs []llm.Message) []int {
	var starts []int
	for i, m := range msgs {
		if m.Role == "user" {
			starts = append(starts, i)
		}
	}
	return starts
}

// RepairPairing appends synthetic tool results for assistant tool calls that
// have no matching tool message, and drops leading orphan tool messages. Both
// OpenAI and Anthropic reject transcripts with unpaired tool calls.
func RepairPairing(msgs []llm.Message) []llm.Message {
	var out []llm.Message
	pending := map[string]bool{}
	var pendingOrder []string

	flushPending := func() {
		for _, id := range pendingOrder {
			if pending[id] {
				out = append(out, llm.Message{
					Role:       "tool",
					ToolCallID: id,
					Content:    "(not executed)",
				})
			}
		}
		pending = map[string]bool{}
		pendingOrder = nil
	}

	for _, m := range msgs {
		switch m.Role {
		case "assistant":
			flushPending()
			out = append(out, m)
			for _, tc := range m.ToolCalls {
				pending[tc.ID] = true
				pendingOrder = append(pendingOrder, tc.ID)
			}
		case "tool":
			if pending[m.ToolCallID] {
				pending[m.ToolCallID] = false
				out = append(out, m)
			}
			// Orphan tool results (no pending call) are dropped.
		default:
			flushPending()
			out = append(out, m)
		}
	}
	flushPending()
	return out
}
