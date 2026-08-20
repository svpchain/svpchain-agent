# svpchain-agent

**English** | [简体中文](README.zh-CN.md)

A local-key **on-chain agent** for svpchain (Cosmos/EVM) that **discovers other agents on chain and delegates tasks to
them**, built around a strict separation of trust:

- **Local signing MCP service** (`svpchain-mcp`) — keeps the user's signing key on the local machine, never exposes it,
  and only signs payloads/challenges that pass strict cross-checks.
- **Remote build + broadcast MCP service** — constructs unsigned transactions, serves market data, and broadcasts signed
  transactions. Runs off-machine (`https://mcp-testnet.svpchain.org/`).
- **Built-in LLM assistant** (`svpchain-gui`) — a streaming tool-calling loop (OpenAI-compatible APIs or native
  Anthropic) that orchestrates the two: the remote side *builds* and *broadcasts*, the local side *signs*. Keys never
  leave the machine. Optional **transfer whitelist**, modular **assistant skills**, multi-turn **conversation history**,
  and local **run logs** tighten transfers, prompts, and observability.
- **Agent discovery & delegation** — find agents in the chain's `x/agent` registry and hand them tasks under short-lived
  **SVP-DT credentials**, so a remote agent can act on the user's account without ever holding the user's key.
- **Google A2A (Agent-to-Agent)** — delegate sub-tasks to other A2A agents (client only; this agent never runs as a
  network service).

The signer runs over **stdio** (no network port; the process that starts it is the trust boundary). The remote side is
reached over HTTP and gated by a signed-challenge bearer token, so the remote never holds a key either.

The on-chain write flow is always: remote `build_*` → local `sign_*` → remote `broadcast_*`, passing `signed_tx` fields
verbatim.

## Agent discovery & delegation

The user issues credentials with **their own account key**, under the DID `did:svp:<their address>` — no agent
registration, no fee, no bond. The chain resolves that DID through the account's published x/auth public key.
Registration stays what it is for: a directory of agents that act on *others'* behalf, backed by slashable stake.

1. **Discover** — the **Agents** tab (or the assistant's `discover_agents`) lists ACTIVE registered agents, with each
   agent's A2A card checked against the capability hash it registered on chain.
2. **Root delegation** — one on-chain delegation to the user's own DID sets the outer ceiling: permitted actions,
   subaccounts, denominations, total and daily spend caps, expiry. Manage it in the **Delegations** tab.
3. **Delegate a task** — the assistant mints a **single-use, short-lived** credential narrowing that ceiling to the one
   task, attaches it to the message metadata as `svp.delegation/v1`, and sends `{skill, tool, args}` to the agent's A2A
   endpoint. The remote agent verifies it and executes on chain via `MsgAgentExecDelegated`.

**Every grant requires an explicit confirmation** in a dialog showing the exact terms. Declining, ignoring, or running
headless all deny. **Pause** is the emergency stop: one click invalidates every outstanding credential under a
delegation at once.

## Quick start (GUI)

Import a key → **Settings** (language, chain id, chain REST URL, LLM API key / provider) → optional **Security**
whitelist → use **Assistant** for on-chain actions (swap, transfer, bridge, ERC-20/721, Lendora lending, x402, …) or to
discover and delegate to remote agents, or export **MCP** config for Cursor.

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
| [Agent observability](docs/agent-observability.md)        | Full design of run traces and offline eval                                                         |
