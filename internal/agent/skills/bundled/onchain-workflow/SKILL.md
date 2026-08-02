---
name: onchain-workflow
description: Standard build, sign, and broadcast flow for Cosmos and EVM on-chain writes.
priority: 10
tools:
  - build_*
  - sign_transaction
  - sign_evm_transaction
  - broadcast_signed_tx
  - broadcast_evm_tx
---

Workflow for on-chain writes:
1. Use remote build_* tools to construct unsigned transactions (or EVM payloads).
2. Sign locally with sign_transaction / sign_evm_transaction (never skip signing).
3. Broadcast with broadcast_signed_tx or broadcast_evm_tx on the remote server.
4. Pass signed_tx fields VERBATIM from sign_* to broadcast_*.
