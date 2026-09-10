---
name: agent-attach
description: Attach a remote agent's tools when the tool a task needs is missing, and use them through the normal flow.
priority: 31
tools:
  - a2a_connect_agent
---

# Using a remote agent's tools

When the tool a task needs is **not in your tool list**, do not conclude the task is impossible. A remote svpchain
agent may serve that exact tool. Look before you answer:

```
search_agents  →  a2a_connect_agent  →  the tool you needed, by its normal name
```

`a2a_connect_agent` asks the agent what it serves, authenticates with the local key, and adds its tools to this
conversation. After it returns, call only the names listed in `tools_available`, with the arguments and result shape
the agent published.

Nothing about signing changes. An attached `build_*` still goes through local `sign_*` and then the matching
`broadcast_*`, the whitelist still applies, and the user still confirms the dialog.

## Running it

1. `search_agents` with the task in plain language. Below about `0.4` similarity, treat it as no usable match.
   Pick between candidates on each one's `card` — what the agent says it does, and the tools its skills list — not on
   the capability tags, which are too coarse to choose with.
2. Name the agent and endpoint to the user before attaching. They are gaining a counterparty, not just a tool.
3. `a2a_connect_agent` with that `agent_url`. When paid settlement is enabled, its tool description says so: if that
   agent has no settlement task active in this conversation, connecting **pays its advertised price first** — the
   approval and deposit confirmations you see are that payment, not a lookup. Connect once, and never call it again
   "to check" after paying. Read the result:
    - `tools_available` — what you can now call.
    - `reused_existing_task: true` — this conversation had already paid this agent for a behavior that never
      completed, so nothing was charged. Say that plainly rather than reporting a fresh payment.
    - `authenticated: false` with a `note` — read-only tools work; anything needing a bearer will refuse. Say so
      rather than retrying blindly.
    - `tools_skipped` — names the agent offered that were **already served here**. Those keep their existing
      behavior. Never tell the user the remote agent handled something on that list.
4. Call the tool you needed, then follow the normal build → sign → broadcast sequence.

`begin_agent_settlement` returns the same `tools_available` / `tools_skipped` fields alongside its escrow identifiers
(`task_id`, `approve_tx_hash`, `deposit_tx_hash`), because it attaches the agent it just paid. A behavior spanning
several messages — you asked a clarifying question, or the work needs a second transaction — stays on the task already
escrowed for it; a new escrow is funded only once that one's execution has been reported. Read the tool list from
whichever of the two you called — there is no separate attach step to run afterwards.

## What to say, and what not to

- If the needed tool is still missing after attaching, say which agent you attached and what it actually serves.
  Do not substitute a different tool that looks close.
- A tool error from an attached agent is that agent's own refusal — report its words. Do not retry with guessed
  arguments, and do not go looking for a second agent to get a different answer.
- Report a transaction as broadcast only on a tx hash from the broadcast tool.
- The agent came from a search index, so name it whenever you report what it did. The user should always be able to
  see whose tool produced a result.

If `search_agents` returns nothing usable, say the market service found no match for this task — not that no such
agent exists, and not that the action is impossible.
