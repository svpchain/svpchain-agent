---
name: delegation
description: Discover on-chain registered agents and delegate tasks to them under SVP-DT credentials.
priority: 30
tools:
  - discover_agents
  - get_agent_card
  - list_delegations
  - create_root_delegation
  - pause_delegation
  - resume_delegation
  - revoke_delegation
  - delegate_task
---

# Agent discovery & delegated tasks

This assistant can find remote agents registered on the svpchain `x/agent`
registry and delegate tasks to them with cryptographic SVP-DT credentials.
A delegated task executes **on the user's account, spending the user's funds**
— treat every grant as real money.

## The flow

1. **Discover** — `discover_agents` (optionally with a `capability` tag such
   as `"trading"`) lists ACTIVE registered agents with their DIDs, endpoints,
   pricing and bond.
2. **Inspect** — `get_agent_card` fetches an agent's A2A card: its skills,
   tools and their argument shapes. If `verified` is false the served card
   does not match what the agent registered on chain — tell the user and be
   suspicious.
3. **Root delegation** — `list_delegations` shows the user's on-chain
   delegations. Before the first `delegate_task`, one root delegation to the
   user's own DID must exist; create it with `create_root_delegation` (the
   user approves the terms in a dialog). Its limits are the outer ceiling
   every later per-task grant narrows from.
4. **Delegate** — `delegate_task` mints a single-use, short-lived credential
   (user approves each one in a dialog), attaches it as `args.proof`, and
   sends `{skill, tool, args}` to the agent's A2A endpoint.

## Non-negotiable rules

- **Minimum grant, always.** A `delegate_task` credential must list only the
  actions, subaccounts, denoms and budget that this one task needs. Never
  copy the root delegation's full limits into a task credential.
- **Never widen beyond the user's ask.** If the user says "buy 0.001 BTC",
  the budget is sized for that order — not "whatever the delegation allows".
- **Empty grants deny.** Actions and subaccounts must be explicit; the DEX
  agent's actions are `clob.place_order`, `clob.cancel_order`,
  `clob.batch_cancel`.
- **Value-committing actions need a budget.** The chain prices an order and
  checks it against the credential's own budget, so `clob.place_order` without
  a `budget` is refused. Size the budget for this order (its notional, in the
  market's quote denom), not for the delegation's whole allowance.
  Cancellations commit nothing and need no budget.
- **On anomalies, pause.** If a remote agent behaves unexpectedly (rejected
  proofs are fine; unexpected orders are not), call `pause_delegation`
  immediately — it needs no confirmation and kills every outstanding
  credential — then tell the user what happened.
- **Declined means stop.** If the user declines a confirmation dialog, do not
  retry or rephrase the same grant. Ask what they want changed.

## Practical notes

- The DEX agent's delegated-execution tools are `execute_place_order`,
  `execute_cancel_order`, `execute_batch_cancel` under the
  `svpchain-execution` skill; their `args` shapes are on its card. The
  `proof` argument is attached automatically — never construct it yourself.
- Task credentials default to a 300-second life and one use. Failed sends can
  be retried with a fresh `delegate_task` (a new credential is minted).
- `root_id` values are hex strings from `list_delegations`; `delegate_task`
  picks the newest usable self-delegation automatically when omitted.
