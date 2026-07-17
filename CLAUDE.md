# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A local-key on-chain agent for svpchain (Cosmos SDK + EVM). The defining constraint is a **three-party trust separation**: the signing key never leaves the local machine, the remote service builds and broadcasts but never holds a key, and an LLM assistant orchestrates the two.

- **`svpchain-mcp`** — stdio MCP signing service. Holds the key (OS credential store), signs only payloads/challenges that pass strict cross-checks. Also hosts `a2a serve`.
- **Remote MCP** (`https://mcp-testnet.svpchain.org/`, HTTP) — builds unsigned txs, serves market data, broadcasts signed txs. Not in this repo.
- **`svpchain-gui`** — Wails app (Go + embedded Vue) with a built-in LLM tool-calling assistant that runs the signer in-process.

## Commands

```sh
make build          # → build/svpchain-mcp (stdio signer). CGO_ENABLED=1 required.
make build-gui      # → Wails GUI; needs `wails` CLI + Node
make build-all
make test           # go test ./...
make tidy           # go mod tidy
make package-macos-app      # .app + DMG (macOS only)
make package-windows-app    # zip (Windows, needs pwsh 7+)
```

Run a single test:
```sh
go test ./internal/signer/ -run TestName -v
```

Frontend (from `cmd/svpchain-gui/frontend/`): `npm run dev` / `npm run build`.

**All builds require CGO** — `eth_secp256k1` uses libsecp256k1. The Wails GUI CLI: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`.

## The on-chain write flow (core invariant)

Every state-changing action follows: **remote `build_*` → local `sign_*` → remote `broadcast_*`**, passing `signed_tx` fields verbatim. The signer is bound to one `--chain-id` and **refuses any payload/challenge for a different chain** or a `signer_address` that isn't the loaded key. When changing signing or dispatch logic, preserve these cross-checks — they are the trust boundary, not validation niceties.

Auth: a `svpchain-mcp-auth-v1:` challenge is signed locally and exchanged for a bearer token. `sign_challenge` refuses any text not starting with that prefix + matching chain id (never a generic signing oracle).

## Architecture map

- `cmd/svpchain-mcp/` — CLI: `serve` (default), `import`/`list`/`delete` (key mgmt), `a2a serve`.
- `cmd/svpchain-gui/` — Go entry + `frontend/` (Vue 3 + naive-ui, vue-i18n en/zh). `wailsjs/` is generated bindings.
- `internal/agent/` — the LLM tool-calling loop (`runner.go` `Run`, `dispatchTool`). Holds a remote MCP client + in-process `LocalSigner` (`local.go`). Local-only tools (`signer_whoami`, `evm_to_bech32`, `http_fetch`, `x402_*`, `a2a_send_message`) live here and are **not** exposed by the stdio server.
  - `skills/bundled/*/SKILL.md` — the system prompt is **assembled from modular skills**, not hardcoded. `base` is always on; others gate on available tools and `disabled_skills`. Bulky detail lives in `bundled/<name>/references/*.md`, loaded on demand by the LLM via the local `read_skill_reference` tool (`skills/references.go`).
  - `whitelist_gate.go` — assistant pre-flight transfer gate (see below).
  - `memory.go` — session memory caching `whoami`/`signer_whoami` to `agent_memory.json`.
 - `history/` — multi-turn conversation persistence (`sessions/*.jsonl` next to `prefs.json`) + context management: tool-result projection to blobs, LLM compaction of old turns, tool-call pairing repair. Wired via `Config.Prior` / `Config.OnTranscript`.
 - `runlog/` — local JSONL run traces (`agent_runs.jsonl`): tools, outcomes, tx hashes, per-round LLM latency + token usage.
- `internal/signer/` — `eth_secp256k1` + `SIGN_MODE_DIRECT` signing, EVM tx signing, EIP-712 typed data, signer-layer whitelist checks. `init()` sets svp bech32 prefixes — import this package rather than blank-importing `internal/config`.
- `internal/mcp/` — stdio MCP tool handlers (the 5 signing tools + `whoami`).
- `internal/payload/` — wire types (`TxPayload`, `SignedTx`, `EvmTxPayload`). **Intentionally no I/O** so the signer can be imported without chain/HTTP deps.
- `internal/whitelist/` — address store + recipient checks; `internal/keystore/` — OS credential store; `internal/prefs/` — `prefs.json` (single config source); `internal/manage/` — key import/list/delete + MCP config gen; `internal/desktop/` — Wails bindings; `internal/a2a/` + `internal/a2aserver/` — A2A client/server; `internal/update/` — in-app GitHub-release updates.

## Transfer whitelist — two layers, different empty-list semantics

This trips people up. There are **two** enforcement points:

1. **Assistant pre-flight gate** (`internal/agent/whitelist_gate.go`) — checks the recipient/spender from tool *arguments* before forwarding a `build_*` call. **Mandatory: an empty whitelist refuses ALL transfers/approvals.** Covers ERC-20/721 contract calls because it reads args, not calldata.
2. **Signer fallback** (`internal/signer/`) — re-checks at sign time. **Empty whitelist = unrestricted** (backward compatible). Only decodes Cosmos `MsgSend` recipients and EVM native sends (`to` set, `value` > 0); contract/zero-value txs pass through here and rely on layer 1.

The standalone `svpchain-mcp` signer reads the same `prefs.json` but does **not** impose the mandatory-whitelist rule — that's the GUI assistant's policy only.

## Keys & config

- Keys: OS credential store (Keychain / Cred Manager / Secret Service), service `svpchain-agent`, account = chain id. **One key per chain.** No `--key-hex` flag by design (would leak into process args). Headless fallback: `SIGNER_KEY_HEX`.
- Config: `prefs.json` in the app config dir (`~/Library/Application Support/com.svpchain.agent-gui/` on macOS, `%AppData%` on Windows). Holds LLM settings, remote MCP URL, whitelist, `disabled_skills`. `agent_memory.json` sits alongside it.
- EVM chain id: parsed from `--chain-id` (`svp_2517-1` → `2517`) unless `--evm-chain-id` overrides. No chain number + no flag = EVM signing disabled, Cosmos unaffected.

## Conventions for code changes

- **CGO always.** Build/test with `CGO_ENABLED=1` (libsecp256k1). Use `make build` / `make test`; a single test is `go test ./internal/signer/ -run TestName -v`.
- **Format and vet.** Run `gofmt -w` on every edited `.go` file and `go vet ./...` before considering work done.
- **Keep `internal/payload/` I/O-free.** It carries wire types only so the signer can be imported without chain/HTTP deps — no `net/http`, file, or chain imports there.
- **Trust-boundary files need extra care.** Changes under `internal/signer/`, `internal/payload/`, `internal/whitelist/`, `internal/mcp/`, `internal/agent/whitelist_gate.go`, or `internal/agent/chainid.go` touch the security boundary. Preserve the chain-id binding and `signer_address` cross-checks, the `svpchain-mcp-auth-v1:` challenge prefix guard, and the two-layer whitelist semantics (gate: empty = refuse all; signer: empty = unrestricted). Add or extend tests in the matching `_test.go`.
- **Bundled agent skills are data, not code.** A new runtime skill is a new `internal/agent/skills/bundled/<name>/SKILL.md` with `name`/`description` frontmatter, gated on tool availability — don't hardcode prompt text in Go.

This repo ships Claude Code developer config under `.claude/` (active automatically, no install): `/verify` (build + test + gofmt -l + vet) and `/trust-check` (audit the current diff) commands, a `trust-boundary-auditor` subagent, dev skills (`build-and-test`, `trust-boundary-review`, `authoring-agent-skills`), and a PostToolUse hook (`.claude/hooks/go-postedit.sh`) that runs `gofmt -w` on saved Go files and flags edits to trust-boundary files.
