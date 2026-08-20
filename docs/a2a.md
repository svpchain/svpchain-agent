# Agent-to-Agent (A2A)

**English** | [简体中文](a2a.zh-CN.md) · [← README](../README.md)

This project implements the client side of [Google's A2A protocol](https://google.github.io/A2A/) via [
`a2a-go`](https://github.com/a2aproject/a2a-go). A2A complements MCP: MCP connects the assistant to tools; A2A connects
agents to other agents.

This agent is a **client only**. It is operated directly by its human owner and calls out to remote A2A agents; it does
not expose an A2A server — a network-reachable service holding the owner's signing key is the wrong trust shape for a
local wallet agent.

## Call other agents (A2A client)

On-chain work uses **`delegate_task`** (SVP-DT credential on the A2A message). See the README section on agent
discovery. There is no bundled **a2a** skill.

The GUI still exposes an uncredentialed local tool `a2a_send_message` for a raw A2A URL and a plain-text message (no
spend on the user's account):

| Argument    | Description                                                                                                                      |
|-------------|----------------------------------------------------------------------------------------------------------------------------------|
| `agent_url` | Base URL of the remote agent (the client fetches `/.well-known/agent-card.json` from this URL). Example: `http://localhost:9001` |
| `message`   | Plain-text user message for the remote agent                                                                                     |

Returns JSON: `{ "task_id", "context_id", "state", "response" }`.

## Security notes

- Remote A2A agents **never** receive local signing keys.
- Do not send private keys, mnemonics, or raw key material in A2A messages.
- Prefer delegating read-only or advisory tasks unless the remote agent is fully trusted for signing workflows.
