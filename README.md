# svpchain-agent

**English** | [简体中文](README.zh-CN.md)

A local-key **on-chain agent** for svpchain (Cosmos/EVM), built around a strict separation of trust:

- **Local signing MCP service** (`svpchain-mcp`) — keeps the user's signing key on the local machine, never exposes it, and only signs payloads/challenges that pass strict cross-checks.
- **Remote build + broadcast MCP service** — constructs unsigned transactions, serves market data, and broadcasts signed transactions. Runs off-machine (`https://indexer.svpchain.com/mcp`).
- **Built-in LLM assistant** (`svpchain-gui`) — a streaming tool-calling loop (OpenAI-compatible APIs or native Anthropic) that orchestrates the two: the remote side *builds* and *broadcasts*, the local side *signs*. Keys never leave the machine. Optional **transfer whitelist**, modular **assistant skills** (bulky detail in `references/*.md`, loaded on demand via `read_skill_reference`), multi-turn **conversation history**, and local **run logs** tighten transfers, prompts, and observability.
- **Google A2A (Agent-to-Agent)** — expose this agent as an A2A-compliant HTTP service, or delegate sub-tasks to other A2A agents via `a2a_send_message`.

The signer runs over **stdio** (no network port; the process that starts it is the trust boundary). The remote side is reached over HTTP and gated by a signed-challenge bearer token, so the remote never holds a key either.

The on-chain write flow is always: remote `build_*` → local `sign_*` → remote `broadcast_*`, passing `signed_tx` fields verbatim.

## Quick start (GUI)

Import a key → **Settings** (language, chain id, LLM API key / provider; expand **LLM** and **Skills** as needed) → optional **Security** whitelist → use **Assistant** for on-chain actions (swap, transfer, bridge, ERC-20/721, Lendora lending, x402, …), or export **MCP** config for Cursor.

```sh
make build-all      # build/svpchain-mcp + the Wails GUI (CGO required)
make test
```

See [Build, packaging & testing](docs/build-and-packaging.md) for prerequisites and platform packages.

## Documentation

| Document | Contents |
|----------|----------|
| [Architecture & project layout](docs/architecture.md) | Trust model diagram, on-chain write flow, directory map |
| [Local signer (svpchain-mcp)](docs/signer.md) | Signing tools, key storage (OS credential store), running the signer, MCP client config for Cursor |
| [Graphical app (svpchain-gui)](docs/gui.md) | Tabs, LLM settings (OpenAI-compatible / Anthropic), assistant skills & progressive references |
| [Assistant memory & context](docs/assistant-context.md) | Session memory, conversation history & context management, run logs & evaluation |
| [Transfer whitelist](docs/security-whitelist.md) | Two-layer enforcement (pre-flight gate + signer fallback) and their different empty-list semantics |
| [Agent-to-Agent (A2A)](docs/a2a.md) | A2A server (`a2a serve`), A2A client (`a2a_send_message`), security notes |
| [Build, packaging & testing](docs/build-and-packaging.md) | Build prerequisites, macOS `.app`/DMG, Windows zip, in-app updates, tests |
| [Agent observability](docs/agent-observability.md) | Full design of run traces and offline eval |
