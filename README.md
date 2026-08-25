# svpchain-agent

**English** | [简体中文](README.zh-CN.md)

A local-key **on-chain agent** for svpchain (Cosmos/EVM) that also **discovers other agents on chain**, built around a
strict separation of trust:

- **Local signing MCP service** (`svpchain-mcp`) — keeps the user's signing key on the local machine, never exposes it,
  and only signs payloads/challenges that pass strict cross-checks.
- **Remote build + broadcast MCP service** — constructs unsigned transactions, serves market data, and broadcasts signed
  transactions. Runs off-machine (`https://mcp-testnet.svpchain.org/`).
- **Built-in LLM assistant** (`svpchain-gui`) — a streaming tool-calling loop (OpenAI-compatible APIs or native
  Anthropic) that orchestrates the two: the remote side *builds* and *broadcasts*, the local side *signs*. Keys never
  leave the machine. Optional **transfer whitelist**, modular **assistant skills**, multi-turn **conversation history**,
  and local **run logs** tighten transfers, prompts, and observability.
- **Agent search** — find remote agents by describing a task, through the **Agent Market** service.
- **Google A2A (Agent-to-Agent)** — ask other A2A agents for information (client only; this agent never runs as a
  network service). A2A messages carry no authority over the user's account.

The signer runs over **stdio** (no network port; the process that starts it is the trust boundary). The remote side is
reached over HTTP and gated by a signed-challenge bearer token, so the remote never holds a key either.

The on-chain write flow is always: remote `build_*` → local `sign_*` → remote `broadcast_*`, passing `signed_tx` fields
verbatim.

## Agent search

1. **Search** — `search_agents` takes a task in natural language and returns agents ranked by semantic similarity
   against their published A2A cards, with each one's DID, A2A endpoint, capability tags, pricing and bond. It runs
   locally against the **Agent Market** service (Settings → Agent Market URL, reported back as `agent_market_url`), not
   through the remote MCP server.
2. **Talk** — `a2a_send_message` sends plain, uncredentialed text to an agent's endpoint. Nothing about it lets a remote
   agent act on the user's account: on-chain writes still go through remote `build_*` → local `sign_*` →
   remote `broadcast_*`, each signature confirmed in a dialog.

The market service is the only source of these results: it indexes the chain's `x/agent` registry, but nothing here
re-reads the chain, so an agent's endpoint and capabilities are that service's claim rather than a verified on-chain
fact. The endpoint decides where an A2A message goes — it cannot move funds, but it does decide who reads the message.

## Quick start (GUI)

Import a key → **Settings** (language, chain id, LLM API key / provider, Agent Market URL) → optional **Security**
whitelist → use **Assistant** for on-chain actions (swap, transfer, bridge, ERC-20/721, Lendora lending, x402, …) or to
find remote agents, or export **MCP** config for Cursor.

```sh
make build-all      # build/svpchain-mcp + the Wails GUI (CGO required)
make test
```

See [Build, packaging & testing](docs/build-and-packaging.md) for prerequisites and platform packages.

## Documentation

| Document                                                  | Contents                                                                                           |
|-----------------------------------------------------------|----------------------------------------------------------------------------------------------------|
| [Architecture & project layout](docs/architecture.md)     | Trust model diagram, on-chain write flow, directory map                                            |
| [Local signer (svpchain-mcp)](docs/signer.md)             | Signing tools, key storage (OS credential store), running the signer, MCP client config for Cursor |
| [Graphical app (svpchain-gui)](docs/gui.md)               | Tabs, LLM settings (OpenAI-compatible / Anthropic), assistant skills & progressive references      |
| [Assistant memory & context](docs/assistant-context.md)   | Session memory, conversation history & context management, run logs & evaluation                   |
| [Transfer whitelist](docs/security-whitelist.md)          | Two-layer enforcement (pre-flight gate + signer fallback) and their different empty-list semantics |
| [Agent-to-Agent (A2A)](docs/a2a.md)                       | A2A client (`a2a_send_message`), security notes                                                    |
| [Build, packaging & testing](docs/build-and-packaging.md) | Build prerequisites, macOS `.app`/DMG, Windows zip, in-app updates, tests                          |
| [Agent observability](docs/agent-observability.md)        | Run traces, optional Phoenix OTLP, and offline eval                                                |
