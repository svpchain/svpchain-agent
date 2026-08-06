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

## Where these tools run (state this correctly if asked)

All the tools in this skill run **locally, in this app**. They read the chain
directly over the REST endpoint configured in Settings → Agent Hub URL, which
every `discover_agents` result reports back as `agent_hub_url`. They do **not**
go through the remote MCP server, and their chain endpoint is **not** part of
any MCP client configuration. If the user asks which chain endpoint is in use,
read it from a tool result or tell them to check Settings — never speculate
about server-side configuration.

Two different endpoints are involved, and confusing them causes real trouble:

- **Agent Hub URL** — a local setting; the REST API of the chain that hosts
  the agent registry, i.e. which node to read the registry from.
- **An agent's `endpoint`** — stored **on chain** by that agent; where its A2A
  service lives. Changing a local setting cannot fix a stale one. A card that
  will not load usually means the agent registered an endpoint that is now
  wrong, or its service is down — say that, rather than blaming settings.

Discovery results are cached for about a minute. After a chain restart or a
registration change, call `discover_agents` with `refresh: true`; never answer
from an earlier result in the conversation, because registrations change.

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
4. **Delegate** — `delegate_task` mints a short-lived credential (user
   approves each one in a dialog), attaches it to the A2A message metadata
   under `svp.delegation/v1`, and sends `{skill, tool, args}` to the agent's
   A2A endpoint.

## Read-only delegation

To let an agent *read* the user's account without any spending power, grant
`["query.account"]` with **no budget** (a budget on a read-only grant is
refused). Subaccounts must still be listed explicitly. An agent that supports
delegated reads answers its covered read tools for the user's own account
only — the owner argument defaults to the user — and the credential stays
reusable for polling until it expires. Which read tools accept a credential
varies per agent; its card lists them. `query.account` is an off-chain
action: never list it in `create_root_delegation`'s actions, which accept
only chain-executable write actions.

## Re-delegation (intermediary agents)

By default a credential dies with the agent it names. When a workflow truly
has an intermediary — agent A must pass the task on to agent B — set
`redelegable: true` and list the permitted final executor(s) in
`redelegate_to`. Then:

- The intermediary may hand a **narrowed** copy of the credential one further
  hop, only to the DIDs listed. Anyone else in `aud` is refused.
- The user sees and must approve the target list in the confirmation dialog.
- The grant can only shrink down the chain: budgets, actions, subaccounts and
  expiry all narrow, never widen.
- Never set `redelegable` "just in case" — it widens the blast radius, and a
  non-redelegable credential is the safe default.
- Caveat: the target list is enforced by the verifying agent; on the chain
  itself a redelegated **write** is currently bounded by budget, epoch and
  nonce rather than the target list, so keep redelegable write grants tightly
  budgeted.

## Non-negotiable rules

- **Minimum grant, always.** A `delegate_task` credential must list only the
  actions, subaccounts, denoms and budget that this one task needs. Never
  copy the root delegation's full limits into a task credential.
- **Never widen beyond the user's ask.** If the user says "buy 0.001 BTC",
  the budget is sized for that order — not "whatever the delegation allows".
- **Empty grants deny.** Actions and subaccounts must be explicit. The
  action namespace is defined by the chain: `clob.place_order`,
  `clob.cancel_order`, `clob.batch_cancel`, `sending.deposit_to_subaccount`,
  and the read-only `query.account`.
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

- An agent's **card is the only authority** on which tools accept a
  delegated credential — inspect it with `get_agent_card` before promising
  anything; never assert a tool roster from memory. Some of an agent's tools
  may instead require the agent's own authentication (e.g. a bearer-token
  handshake); a delegated credential cannot unlock those, because the
  handshake needs the account key this app never releases. A worked example
  of one real card lives in the `dex-agent.md` reference — read it with
  `read_skill_reference(skill="delegation", file="dex-agent.md")` when
  planning against the DEX agent, but the live card always wins.
- The credential proof is attached automatically to the message metadata —
  never construct or pass one yourself.
- Task credentials default to a 300-second life. Write credentials are
  consumed on use (the chain burns the nonce); read credentials stay valid
  for polling until expiry. Failed sends can be retried with a fresh
  `delegate_task` (a new credential is minted).
- `root_id` values are hex strings from `list_delegations`; `delegate_task`
  picks the newest usable self-delegation automatically when omitted.
