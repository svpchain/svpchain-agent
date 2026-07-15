# Architecture & project layout

**English** | [简体中文](architecture.zh-CN.md) · [← README](../README.md)

## Architecture

```
            ┌──────────────────────────────┐
            │  LLM (OpenAI-compat / Anthropic) │   streaming tool-calling loop
            └───────────────┬──────────────┘
                            │
              ┌─────────────┴──────────────┐
   build_* / broadcast_* /        sign_transaction / sign_evm_transaction /
   market data / whoami           sign_typed_data / sign_challenge / whoami
              │                              │
   ┌──────────▼───────────┐      ┌───────────▼────────────┐
   │  Remote MCP (HTTP)   │      │  Local signer (stdio)  │
   │  builds + broadcasts │      │  holds the key locally │
   └──────────────────────┘      └────────────┬───────────┘
                                               │
                                    OS credential store
                              (Keychain / Cred Mgr / Secret Service)
```

On-chain write flow the assistant follows: remote `build_*` → local `sign_*` → remote `broadcast_*`, passing `signed_tx` fields verbatim. Authentication uses a signed `svpchain-mcp-auth-v1:` challenge (signed locally), exchanged for a bearer token. When configured, a transfer whitelist is enforced both before the assistant builds a transfer (pre-flight, covering contract transfers) and again at the local signer — see [Transfer whitelist](security-whitelist.md).

For **multi-agent** workflows, the assistant can call remote A2A agents with `a2a_send_message`, or you can run `svpchain-mcp a2a serve` to expose the same orchestration loop to other A2A clients over HTTP JSON-RPC — see [Agent-to-Agent (A2A)](a2a.md).

## Project layout

```
cmd/
  svpchain-mcp/   # stdio signing MCP CLI: serve (default) / import / delete / list / a2a serve
  svpchain-gui/   # Wails GUI: Go entry + embedded Vue frontend
internal/
  agent/          # LLM tool-calling loop: remote MCP client + in-process local signer; pre-flight whitelist gate; session memory
    skills/       # Bundled SKILL.md (+ references/*.md); system prompt + on-demand read_skill_reference
    history/      # Conversation persistence + context management (JSONL sessions, projection, LLM compaction)
    runlog/       # Local JSONL run traces (tools, outcomes, tx hashes, token usage) for debugging & eval
    eval/         # Offline regression scoring for the whitelist gate
  a2a/            # A2A client: resolve Agent Card, SendMessage, parse replies
  a2aserver/      # A2A HTTP server: Agent Card, JSON-RPC /invoke, executor → agent.Run
  mcp/            # MCP tool handlers (sign_transaction / sign_evm_transaction / sign_typed_data / sign_challenge / whoami)
  signer/         # transaction + challenge signing (eth_secp256k1); transfer policy checks
  whitelist/      # address whitelist store + recipient checks (used by the agent gate and the signer)
  manage/         # key import / list / delete, MCP config generation, remote URL
  keystore/       # OS credential store read/write
  payload/        # TxPayload / SignedTx / EvmTxPayload types
  prefs/          # prefs.json schema, load/save (single config source)
  desktop/        # Wails app bindings (keys, MCP config, settings, assistant, security, update)
  update/         # in-app update (GitHub releases, verify, install; macOS DMG + Windows zip)
packaging/
  macos/          # .app assets: Info.plist, icon, en/zh-Hans localization, user guides
  windows/        # Windows user guides (READ-BEFORE-RUN)
scripts/          # packaging (macOS DMG, Windows zip), Wails icon sync, icon generation
```
