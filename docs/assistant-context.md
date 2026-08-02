# Assistant memory, conversation history & run logs

**English** | [简体中文](assistant-context.zh-CN.md) · [← README](../README.md)

## Session memory

Every assistant run used to start with the LLM calling `signer_whoami` (local key) and `whoami` (remote tenant policy). **Session memory** caches both results on disk and injects them into the system prompt so later conversations skip those round-trips.

**How it works:**

1. After remote MCP auth, the agent loads `agent_memory.json` (same config directory as `prefs.json`).
2. If a valid entry exists for the current **chain id + remote MCP URL + local owner**, its JSON is appended to the system prompt as **Cached session context** — the LLM is instructed not to call `signer_whoami` or `whoami` again.
3. If missing or stale, the agent fetches both once (UI step: `Loading session context…`), saves to disk, then injects the prompt.
4. If the LLM still calls either tool, `dispatchTool` returns the cached JSON without a network/local repeat.

**Cache invalidation** — a refresh runs when any of these change:

- Chain id
- Remote MCP URL
- Local signing key (owner address)

**File location** (alongside `prefs.json`):

- macOS: `~/Library/Application Support/com.svpchain.agent-gui/agent_memory.json`
- Windows: `%AppData%\com.svpchain.agent-gui\agent_memory.json`
- Linux: `~/.config/com.svpchain.agent-gui/agent_memory.json`

The same mechanism applies to `svpchain-mcp a2a serve` runs.

## Conversation history & context management

The GUI assistant is **multi-turn**: earlier questions, answers, and tool round-trips are sent back to the LLM on the next message, and conversations survive app restarts. The implementation lives in `internal/agent/history/`.

**Persistence** — each conversation is a JSONL session file under the app config directory:

```
sessions/
  index.json          # conversation list + current conversation id
  <id>.jsonl          # one message per line (user / assistant / tool)
  <id>.summary.json   # compaction state (see below)
  blobs/              # archived full tool results (projection)
```

The Assistant header gains a **conversation picker**, a **New chat** button, and a delete button. Reopening the app restores the current conversation. Sending a message on a different chain id automatically starts a fresh conversation (context from another chain would be misleading).

**Context management** — three mechanisms keep long conversations inside the model's context window (configurable under **Settings → LLM → Context window**, default 64000 tokens; history is budgeted at ~70% of it):

1. **Projection** — tool results larger than 4 KB are archived to `sessions/blobs/` and truncated in the transcript. The live run always saw the full result; only later turns get the truncated view.
2. **LLM compaction** — when the estimated history exceeds the budget, all but the most recent 4 turns are summarized by the LLM into a single summary block that replaces them. The summary prompt requires addresses, tx hashes, amounts, order ids, and explicit user constraints to be preserved verbatim. Compaction folds previous summaries in, so state stays bounded.
3. **Pairing repair** — persisted transcripts are normalized so every assistant tool call has a matching tool result (interrupted runs get synthetic `(not executed)` results); both OpenAI and Anthropic reject unpaired tool calls.

**Privacy** — user messages are key-redacted (64-hex sequences) before hitting disk, session files are `0600`, and the system prompt is never persisted (it is re-assembled from skills each run). Deleting a conversation removes its files.

Desktop bindings: `AgentSessions`, `AgentNewSession`, `AgentSwitchSession`, `AgentDeleteSession`, `AgentTranscript`, `AgentCurrentSessionID`.

## Run logs & evaluation

Every assistant run appends a JSONL trace to `agent_runs.jsonl` (same config directory): tool calls with timing, outcome (`success | failed | stopped | rejected | cancelled`), extracted tx hashes, and **per-round LLM latency + token usage** (`llm_rounds`, `usage`). Private keys and API keys are redacted. Toggle under **Settings → Basic → Save assistant run logs**. Offline eval cases live in `testdata/agent_eval/`; run `./scripts/agent-eval.sh`. See [Agent observability](agent-observability.md) for the full design.
