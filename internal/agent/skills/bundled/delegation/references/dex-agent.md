# Worked example: the svpchain DEX agent's card

Reference file for the `delegation` skill, loaded on demand via
`read_skill_reference`. This is a **snapshot of one real agent's card** to
ground planning — it is documentation, not configuration. The live card from
`get_agent_card` is always authoritative; when this file and the card
disagree, the card wins, and a redeployed agent can change any of it.

## Delegated execution (`svpchain-execution` skill)

Tools that execute on the user's account under an SVP-DT credential carrying
the matching chain action and a budget sized for the task:

| Tool | Required action |
|------|-----------------|
| `execute_place_order` | `clob.place_order` (budget required) |
| `execute_cancel_order` | `clob.cancel_order` |
| `execute_batch_cancel` | `clob.batch_cancel` |
| `execute_deposit_to_subaccount` | `sending.deposit_to_subaccount` (budget required) |

## Delegated reads (`svpchain-account` skill)

Tools served under a `query.account` credential (no budget; owner pinned to
the credential's principal; subaccount must be in the caveat; reusable for
polling until expiry):

- `get_subaccount` — committed snapshot from the agent's indexer
- `get_live_subaccount` — fresh read from chain gRPC; works even when the
  agent's indexer is down
- `get_balance` — wallet (bank + known ERC-20) balances

## Not delegable on this agent

- The rest of the `svpchain-account` tools (orders, fills, transfers, PnL,
  funding) require the agent's **bearer-token handshake**
  (`whoami → auth_challenge → sign_challenge → auth_verify`). A delegated
  credential cannot unlock them: the handshake proves possession of the
  account key, which never leaves this app. If the user needs one of those,
  use the remote MCP tools directly instead of delegating.
- `svpchain-market-data` needs no credential at all — anyone may query it.

## Failure modes seen in practice

- Indexer outage: `get_subaccount`/`get_balance` may 500 while
  `get_live_subaccount` still answers — prefer the live read for balance
  checks when the indexer is flaky.
- `verified: false` on the card: the served card does not match the on-chain
  registration — stop and tell the user.
