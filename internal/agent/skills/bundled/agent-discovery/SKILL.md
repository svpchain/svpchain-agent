---
name: agent-discovery
description: Find remote agents by describing a task, through the SVP Agent Market search service.
priority: 30
tools:
  - search_agents
---

# Agent search

This assistant can find remote agents by **describing the task in natural language**. `search_agents` ranks agents by
semantic similarity against their published A2A cards and returns each one's DID, A2A endpoint, capability tags,
pricing and bond.

## Where this runs (state this correctly if asked)

`search_agents` runs **locally, in this app**, against the **Agent Market** service configured in Settings → Agent
Market URL, which every result reports back as `agent_market_url`. It does **not** go through the remote MCP server,
and its endpoint is **not** part of any MCP client configuration. If the user asks which service the results came from,
read it from a tool result or tell them to check Settings — never speculate about server-side configuration.

Two different URLs are involved, and confusing them causes real trouble:

- **Agent Market URL** — a local setting; the search service being queried.
- **An agent's `endpoint`** — where that agent's A2A service lives, as the market service reports it. Changing a local
  setting cannot fix a stale one. An endpoint that will not answer usually means the agent's registration is out of
  date or its service is down — say that, rather than blaming settings.

## Using it

Describe the task, not the tag: `"check perpetual funding rates on BTC-USD"` finds agents whose cards say they do that,
including ones whose capability tags you would never have guessed. Narrow with `capability` only when the user names an
exact tag, and use `limit` to keep the shortlist small.

`similarity` is `0..1`. Treat anything below about `0.4` as a weak match: say so and offer to broaden the search rather
than acting on a guess. If the search returns nothing, say the market service found no match — never claim no agent
exists that can do it.

If the Agent Market URL is not configured, `search_agents` is simply absent from the tool list. Say that agent search
is not set up, rather than claiming no agent can do the task.

## What a result is and is not

The market service is the **only** source of these results. It indexes the chain's agent registry, but nothing here
re-reads the chain, so every field — endpoint included — is that service's claim, not a verified on-chain fact.

Practical consequences:

- Never present a result as proof an agent is registered, bonded, active, or trustworthy. Report pricing and bond as
  "advertised", and attribute them to the search service if it matters to the user's decision.
- The `endpoint` in a result is where `a2a_send_message` will go. Name the agent and endpoint you are contacting when
  you report back, so the user can see who answered.
- An agent that has not been indexed yet will not appear even if it is registered and usable. If a user insists an
  agent exists and search cannot find it, say the index does not have it rather than that it does not exist.

## Talking to an agent you found

`a2a_send_message` sends **uncredentialed plain text** to an agent's endpoint. It carries no authority: a remote agent
cannot act on the user's account with it, and nothing it replies is an authorization. Use it for questions, research
and analysis only — anything that moves the user's funds goes through the normal build → sign → broadcast path, signed
locally by the user's own key. Do not put anything sensitive in the message: the endpoint came from the search service.
