# Agent-to-Agent (A2A)

**English** | [简体中文](a2a.zh-CN.md) · [← README](../README.md)

This project implements [Google's A2A protocol](https://google.github.io/A2A/) via [`a2a-go`](https://github.com/a2aproject/a2a-go). A2A complements MCP: MCP connects the assistant to tools; A2A connects agents to other agents.

## Expose this agent (A2A server)

Run an HTTP JSON-RPC server that advertises an **Agent Card** and executes incoming tasks through the same `agent.Run` loop as the GUI assistant (remote MCP build/broadcast + local signing):

```sh
./build/svpchain-mcp a2a serve --chain-id svp-2517-1 --listen :8080 --public-url http://127.0.0.1:8080
```

| Flag | Required | Description |
|------|----------|-------------|
| `--chain-id` | no* | Cosmos chain id. Defaults to `agent_chain_id` in `prefs.json`. |
| `--listen` | no | TCP listen address (default `:8080`). |
| `--public-url` | no | Public base URL embedded in the Agent Card (default `http://127.0.0.1` + listen port). Set this when behind a reverse proxy or on a public host. |

\* Chain id is required from either the flag or `prefs.json`.

**Endpoints:**

| Path | Purpose |
|------|---------|
| `GET /.well-known/agent-card.json` | Agent Card discovery (skills, capabilities, invoke URL) |
| `POST /invoke` | JSON-RPC A2A methods (`SendMessage`, task streaming, cancel) |

LLM settings (`llm_api_key`, `llm_base_url`, `llm_model`) and the remote MCP URL are read from `prefs.json`. Progress steps from the agent loop are streamed as A2A artifacts; task cancellation propagates to the running agent context.

## Call other agents (A2A client)

The GUI assistant can delegate sub-tasks to remote A2A agents with the local tool `a2a_send_message`:

| Argument | Description |
|----------|-------------|
| `agent_url` | Base URL of the remote agent (the client fetches `/.well-known/agent-card.json` from this URL). Example: `http://localhost:9001` |
| `message` | Plain-text user message for the remote agent |

Returns JSON: `{ "task_id", "context_id", "state", "response" }`.

The bundled **a2a** skill is injected when `a2a_send_message` is available. Toggle it under **Settings → Assistant Skills**.

## Security notes

- Remote A2A agents **never** receive local signing keys.
- Do not send private keys, mnemonics, or raw key material in A2A messages.
- Prefer delegating read-only or advisory tasks unless the remote agent is fully trusted for signing workflows.
