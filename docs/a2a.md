# Agent-to-Agent (A2A)

**English** | [简体中文](a2a.zh-CN.md) · [← README](../README.md)

This project implements the client side of [Google's A2A protocol](https://google.github.io/A2A/) via [
`a2a-go`](https://github.com/a2aproject/a2a-go). A2A complements MCP: MCP connects the assistant to tools; A2A connects
agents to other agents.

This agent is a **client only**. It is operated directly by its human owner and calls out to remote A2A agents; it does
not expose an A2A server — a network-reachable service holding the owner's signing key is the wrong trust shape for a
local wallet agent.

## Call other agents (A2A client)

The GUI assistant can delegate sub-tasks to remote A2A agents with the local tool `a2a_send_message`:

| Argument    | Description                                                                                                                      |
|-------------|----------------------------------------------------------------------------------------------------------------------------------|
| `agent_url` | Base URL of the remote agent (the client fetches `/.well-known/agent-card.json` from this URL). Example: `http://localhost:9001` |
| `message`   | Plain-text user message for the remote agent                                                                                     |

Returns JSON: `{ "task_id", "context_id", "state", "response" }`.

The bundled **a2a** skill is injected when `a2a_send_message` is available. Toggle it under **Settings → Assistant
Skills**.

## Security notes

- Remote A2A agents **never** receive local signing keys.
- Do not send private keys, mnemonics, or raw key material in A2A messages.
- Prefer delegating read-only or advisory tasks unless the remote agent is fully trusted for signing workflows.
