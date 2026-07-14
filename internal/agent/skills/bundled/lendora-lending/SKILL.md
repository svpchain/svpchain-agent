---
name: lendora-lending
description: Query and operate Lendora Protocol (Compound V2 fork on SVP Chain). Use when users ask about SVP Chain lending markets, supply/borrow APY, health factor, position management, or "can I safely borrow X" style questions. Tools are prefixed with lendora_.
tools:
  - lendora_*
---

# Lendora Lending

Risk-first analysis and operations for Lendora Protocol on SVP Chain. Query tools are read-only; operation tools return a simulation + unsigned tx(s) to be signed locally (`sign_transaction`) and broadcast (`broadcast_signed_tx`).

## References (load on demand)

Detailed material lives in reference files. Read them with `read_skill_reference(skill="lendora-lending", file=...)` ONLY when needed — do not preload:

| File | When to read |
|------|--------------|
| `output-templates.md` | Before formatting your FIRST user-facing reply for an intent (market scan, position check, risk assessment, what-if, balances, protocol overview, execution confirm/complete). Follow the matching template. |
| `error-responses.md` | When any `lendora_*` tool call fails or a build/sign/broadcast step errors. Pick the matching category and use its wording. |

## Tools

Query: `lendora_get_all_markets`, `lendora_get_market_details(asset)`, `lendora_get_protocol_dashboard`, `lendora_get_account_summary`, `lendora_get_account_positions`, `lendora_get_balances`, `lendora_assess_risk` — `address` is optional everywhere (omit = current user from session).

Operations: `lendora_build_supply_tx`, `lendora_build_withdraw_tx`, `lendora_build_borrow_tx`, `lendora_build_repay_tx` (each: `asset`, `amount`), `lendora_build_collateral_tx(asset, action=enable|disable)` — all return simulation + unsigned tx(s).

Parameter notes: `asset` is a symbol (e.g. "USDC") or contract address; `amount` is a string in human-readable units (e.g. "1000", not raw decimals).

## Workflow

### 1) Classify intent → tool chain

| Intent | Trigger examples | Default tools | Enrich |
|--------|------------------|---------------|--------|
| Market scan | "利率多少" / "哪个收益高" | `lendora_get_all_markets` | specific asset → + `lendora_get_market_details` |
| Position check | "我的仓位" / "安全吗" | `lendora_get_account_summary` + `lendora_get_account_positions` + `lendora_assess_risk` (always full) | — |
| Risk assessment | "评估风险" / "还能借多少" | `lendora_assess_risk` | findings name an asset → + `lendora_get_account_positions` |
| What-if simulation | "如果我借 1000 会怎样" | `lendora_build_*_tx` (dry-run via simulate) | — |
| Balance check | "余额" / "Gas 够吗" | `lendora_get_balances` | — |
| Protocol overview | "TVL" / "协议数据" | `lendora_get_protocol_dashboard` | per-market breakdown → + `lendora_get_all_markets` |
| Execution | "帮我存/取/借/还 X" | `lendora_build_*_tx` → `sign_transaction` → `broadcast_signed_tx` | — |

### 2) Ask-or-answer

- **Query intents — never ask clarifying questions.** Ambiguous query → return ALL relevant data, conclusion first, then data table. Markets ≤ 5: show all; > 10: Top 5 + "ask me for more".
- **Action intents — must confirm asset and amount** before building any transaction ("帮我存 1000" → ask which asset, show APY to help decide).
- **Empty position** → state the fact + show highest APY + offer to explain how to start.

### 3) Risk-first

`lendora_assess_risk` triggers automatically on position checks and before what-if/execution (build_tx internal simulate enforces it); NOT on pure market-data or balance queries.

Response intensity by riskLevel: 🟢 Low → one line "仓位健康 ✅"; 🟡 Medium → state findings + safe action space; 🟠 High → strong warning + specific risk-reducing actions + highlight dangerous numbers; 🔴 Critical → top-priority display + urgent recommendation + block risk-increasing actions.

### 4) Execute

```
lendora_build_*_tx → present simulation, ask confirmation
  → user confirms → sign_transaction → broadcast_signed_tx → report tx hash
```

Multi-transaction operations (e.g. Approve + Supply): present all steps upfront before the first signature; sign and broadcast sequentially; if any step fails, stop and report — never auto-retry write operations.

## Output formatting

All numbers 2 decimal places with thousands separators (`$1,200.00`); ≥ 3 items of same type → table, otherwise prose; status markers as emoji + text (`🟢 Low`); layered structure conclusion → data → suggested actions; mark block number ("Based on data at block #X") on risk assessment and simulation outputs ONLY. Full per-intent examples: read `output-templates.md`.

## Error handling

1. **Never guess** — if data is unavailable, say so; never answer from stale/cached data.
2. **Never auto-retry writes** — reads may retry once; writes wait for user instruction after failure.
3. **Always give specific numbers** — shortfalls, gas needed, max borrowable.
4. **Always suggest a next step.**

Category-specific wording (connection/auth, data query, build_tx, signing, broadcast): read `error-responses.md`.

## Guardrails

- Never skip simulate — always show impact before execution.
- Never call `sign_transaction` without explicit user confirmation.
- Never promise safety or returns — this is a tool, not investment advice.
- Never suggest all-in operations — always recommend a safety buffer.
- HF < 1.0 → block the operation; HF 1.0–1.2 → warn and require explicit confirmation; proactively warn when HF approaches the danger zone.
