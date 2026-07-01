// Package runlog persists local JSONL traces of assistant runs for debugging and eval.
//
// Log file: next to prefs.json as agent_runs.jsonl
//
//	macOS: ~/Library/Application Support/com.svpchain.agent-gui/agent_runs.jsonl
//
// Each line is one JSON Run with run_id, steps, llm_rounds, usage totals, outcome
// (success|failed|stopped|rejected|cancelled), and tx_hashes extracted from broadcast tool results.
//
// Disable via Settings → Basic → "Save assistant run logs" (agent_run_log_disabled in prefs.json).
//
// Offline eval: testdata/agent_eval/guard_cases.json + go test ./internal/agent/eval/...
// or scripts/agent-eval.sh
package runlog
