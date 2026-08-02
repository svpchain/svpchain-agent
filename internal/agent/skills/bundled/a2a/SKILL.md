---
name: a2a
description: Delegate tasks to other A2A-compatible agents via a2a_send_message.
priority: 50
tools:
  - a2a_send_message
---

# Agent-to-Agent (A2A)

This agent can call other agents that speak Google's [A2A protocol](https://google.github.io/A2A/).

## When to use

Use `a2a_send_message` when a sub-task is better handled by a specialized remote agent (compliance, research, formatting, etc.) rather than local tools.

## Tool: a2a_send_message

| Parameter   | Description |
|-------------|-------------|
| `agent_url` | Base URL of the remote agent. The client fetches `/.well-known/agent-card.json` from this URL. Example: `http://localhost:9001` |
| `message`   | Plain-text user message for the remote agent |

The tool returns JSON: `{ "task_id", "context_id", "state", "response" }`.

## Exposing this agent as A2A

Run from terminal (uses prefs.json for LLM + chain):

```bash
svpchain-mcp a2a serve --chain-id svp-2517-1 --listen :8080 --public-url http://127.0.0.1:8080
```

Endpoints:

- `GET /.well-known/agent-card.json` — Agent Card
- `POST /invoke` — JSON-RPC (SendMessage, etc.)

## Notes

- Remote agents do **not** have access to local signing keys.
- Do not send private keys or raw mnemonics in A2A messages.
- Prefer delegating read-only or advisory tasks unless the remote agent is trusted for signing workflows.
