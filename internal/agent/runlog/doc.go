// Package runlog persists local JSONL traces of assistant runs for debugging and eval.
//
// Log file: next to prefs.json as agent_runs.jsonl
//
//	macOS: ~/Library/Application Support/com.svpchain.agent/agent_runs.jsonl
//
// Each line is one JSON Run with run_id, steps, llm_rounds (latency, tokens, truncated
// reply + tool_calls), prompt_sha256 + skill names (prompt body is never stored), usage
// totals, outcome (success|failed|stopped|rejected|cancelled), session_id,
// tx_hashes extracted from broadcast tool results, tx_checks from a CometBFT
// RPC /tx?hash= lookup, and intent_checks matching build_* args to tx events.
//
// Disable via Settings → Basic → "Save assistant run logs" (agent_run_log_disabled in prefs.json).
//
// Offline eval: testdata/agent_eval/guard_cases.json + go test ./internal/agent/eval/...
// or scripts/agent-eval.sh
package runlog
