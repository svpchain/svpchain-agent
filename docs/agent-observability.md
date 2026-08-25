# Assistant Observability & Evaluation

**English** | [简体中文](agent-observability.zh-CN.md) · [← README](../README.md)

This document describes how to **measure assistant quality**, the **observability design**, and **what is implemented
today** for the SVPChain Agent GUI assistant — without relying on cloud tracing SaaS (e.g. LangSmith) by default.

---

## 1. Context & goals

Assistant pipeline:

```
User message → LLM tool loop → remote MCP build_* → local sign_* → remote broadcast_*
```

Success is defined by **on-chain outcomes** and **correct orchestration**, not fluent prose alone.

| Goal                   | Description                                     |
|------------------------|-------------------------------------------------|
| **Observable**         | Full trace per run: tools, failures, tx hashes  |
| **Measurable**         | Success rate, rejection rate, rounds, latency   |
| **Regressable**        | Offline cases after prompt/skill/guard changes  |
| **Private by default** | Traces stay on disk; no keys or API keys logged |

Compared to LangSmith/Langfuse: those excel at cloud LLM traces and team dashboards. This design fits **local keys +
chain outcomes**.

---

## 2. Evaluation layers

### Layer A — Outcome (primary)

- Intent / tx success rate
- Parameter correctness (size, market, recipient, …)
- Abstain rate when action must be refused (whitelist, insufficient balance, …)

Ground truth is on-chain: correlate `tx_hashes` in run logs with indexer / query tools.

### Layer B — Orchestration

- Tool sequence (build → sign → broadcast)
- LLM round count (cap: 25)
- Fail-fast behavior after tool errors

### Layer C — LLM quality

- Intent classification, slot filling, hallucination rate

### Layer D — UX & cost

- End-to-end latency, tokens/cost, cancel/timeout rate

Report **security metrics** (whitelist rejections, signer cross-checks) separately from task success.

---

## 3. Implemented: local run log (JSONL)

### Code

| Piece      | Path                                                  |
|------------|-------------------------------------------------------|
| Recorder   | `internal/agent/runlog/`                              |
| Hook       | `internal/agent/runner.go` → `Config.RunLog`          |
| GUI toggle | Settings → Basic → **Save assistant run logs**        |
| GUI viewer | **Runs** tab (`AgentRecentRuns`, newest first)        |
| Pref       | `agent_run_log_disabled` (`false` = enabled, default) |
| Read API   | `AgentRunLogPath()`, `AgentRecentRuns(limit)`         |

### Log file

**`agent_runs.jsonl`** next to `prefs.json`:

- macOS: `~/Library/Application Support/com.svpchain.agent/agent_runs.jsonl`
- Linux: `$XDG_CONFIG_HOME/com.svpchain.agent/agent_runs.jsonl`
- Windows: `%AppData%\com.svpchain.agent\agent_runs.jsonl`

### Record shape (one JSON object per line)

Fields include: `run_id`, timestamps, `chain_id`, `model`, redacted `user_message`, `outcome`, `answer`, `error`,
`session_id` / `session_title` (the multi-turn conversation this run belongs to), `tx_hashes`, `tx_checks` (CometBFT
RPC `GET /tx?hash=0x…`: `confirmed` / `failed` / `pending` / `error` / `skipped`), `intent_checks` (build_* args scored against
confirmed tx events: `matched` / `mismatch` / `included` / `unobserved` / `skipped` — event fields such as
recipient or ticker, not an order-book or position snapshot; EVM txs with no comparable events score `included`),
`round_count`, `prompt_sha256` (SHA-256 of the assembled
system prompt — the body is never stored), `skills` (injected skill names), `llm_rounds[]` (latency, tokens, truncated
`reply` + `tool_calls`), and `steps[]` (think/tool/error with timing).

### `outcome` values

| Value       | Meaning                        |
|-------------|--------------------------------|
| `success`   | Completed with an answer       |
| `failed`    | Error returned                 |
| `stopped`   | Fail-fast after tool error     |
| `rejected`  | Whitelist / transfer gate      |
| `cancelled` | User cancel or context timeout |

### Privacy

Private keys, LLM API keys, and `signed_tx` payloads are **never** stored. The system prompt is hashed, not written.
Long fields are truncated; secrets are redacted.

Disable: Settings UI or `"agent_run_log_disabled": true` in `prefs.json`.

### Inspect

**GUI:** sidebar **Runs** — filter by outcome, open a run for the tool timeline, LLM round reply/`tool_calls`, skill
names, tx hashes (with on-chain status), and **intent checks** (whether bank-send recipients / order tickers appear in
tx events). **Open conversation** jumps to that session in Assistant. **Recheck on
chain** polls CometBFT RPC (`/tx?hash=0x…`, testnet `https://rpc-testnet.svpchain.org`) again if the first lookup was
pending. This is not the Agent Market URL. Delete one run or clear
the log from this tab. Settings → Basic → **View runs** jumps there.

```bash
tail -1 ~/Library/Application\ Support/com.svpchain.agent/agent_runs.jsonl | jq .
jq -r .outcome ~/Library/Application\ Support/com.svpchain.agent/agent_runs.jsonl | sort | uniq -c
```

---

## 3b. Implemented: optional Phoenix OTLP (off by default)

JSONL remains the source of truth. Phoenix is a **side-channel** of the same
redacted tool/LLM spans so you can inspect a tree in a local Arize Phoenix UI.

| Piece    | Path / pref                                                                 |
|----------|-----------------------------------------------------------------------------|
| Exporter | `internal/agent/phoenix/` (OTLP HTTP protobuf — Phoenix rejects JSON)       |
| Hook     | `internal/agent/runner.go` → `Config.PhoenixOTLPURL` + `composeObservers`   |
| GUI      | Settings → Basic → **Export to Phoenix** (switch). Off hides the URL field  |
|          | and **clears** a saved URL. On with an empty field writes                    |
|          | `http://127.0.0.1:6006/v1/traces`.                                          |
| Pref     | `phoenix_otlp_url` — empty disables export.                                 |

A host without a path is rewritten to `/v1/traces`. Only `http` / `https` are
accepted. Flush failures never abort a run.

The same `runlog.Redact` rules apply: no private keys, LLM API keys, `signed_tx`,
or system-prompt body (fingerprint + skill names only).

---

## 4. Implemented: offline eval

### Cases

`testdata/agent_eval/guard_cases.json` — whitelist gate regression (no LLM/network).

### Run

```bash
./scripts/agent-eval.sh
# or
go test ./internal/agent/eval/... ./internal/agent/runlog/... -count=1
```

Package: `internal/agent/eval/`.

---

## 5. LangSmith vs local approach

| Capability         | LangSmith         | This repo                       |
|--------------------|-------------------|---------------------------------|
| LLM + tool trace   | Cloud             | `agent_runs.jsonl`              |
| Dataset regression | Yes               | `guard_cases.json` (extensible) |
| On-chain tx link   | DIY               | `tx_hashes`                     |
| Keys stay local    | Careful redaction | Default local                   |
| Team dashboard     | Yes               | Optional local Phoenix (OTLP)   |

**Recommended path:** JSONL + Runs tab (session link + RPC `tx_checks` / `intent_checks`) + guard eval → optional Phoenix for a span tree.

---

## 6. Workflow

**Daily:** run assistant scenarios → inspect JSONL → `./scripts/agent-eval.sh` after guard/skill changes.

**Weekly:** aggregate outcomes, triage failures, add cases to `testdata/agent_eval/`.

**Release:** eval tests green + testnet smoke with `tx_hashes` verification.

---

## 7. Roadmap (not yet built)

- Mock MCP replay for CI
- LLM eval cases (expected tools/args)
- JSONL aggregation scripts / weekly report
- Self-hosted Langfuse (Phoenix OTLP is the shipped optional viewer)

---

## 8. Code index

```
internal/agent/runlog/
internal/agent/phoenix/
internal/chainrpc/
internal/agent/eval/
internal/agent/runner.go
internal/desktop/agent.go
internal/desktop/runlog.go
cmd/svpchain-gui/frontend/src/tabs/RunsTab.vue
testdata/agent_eval/
scripts/agent-eval.sh
```

---

## 9. Changelog

| Date    | Notes                                               |
|---------|-----------------------------------------------------|
| 2026-08 | Optional Phoenix OTLP (`phoenix_otlp_url`; switch clears URL when off) |
| 2026-08 | GUI Runs tab; generation span; session_id; RPC tx_checks; intent_checks |
| 2026-06 | Initial: JSONL run log, guard eval, settings toggle |
