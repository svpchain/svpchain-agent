---
name: onchain-workflow
description: Standard build, sign, and broadcast flow for Cosmos and EVM on-chain writes.
priority: 10
tools:
  - build_*
  - broadcast_signed_tx
  - broadcast_evm_tx
---

Workflow for on-chain writes:

1. Use remote build_* tools to construct unsigned transactions (or EVM payloads).
2. Sign locally with sign_transaction / sign_evm_transaction (never skip signing). The app shows a confirmation dialog
   before the local key is used; if the user declines, stop — do not retry the same signature.
3. Broadcast with broadcast_signed_tx or broadcast_evm_tx on the remote server.
4. Pass signed_tx fields VERBATIM from sign_* to broadcast_*.

The runtime enforces this sequence. Skipping a step, signing a hand-crafted payload, or editing signed_tx stops the
run — do not work around it.

The unsigned payload is valid only within the current run. Confirm the intended asset, amount, and recipient BEFORE
calling a build tool. Once a build succeeds, do not pause for a text/chat confirmation or wait for a later user turn:
immediately call the appropriate local sign_* tool with that exact payload. Its local confirmation dialog is the
user's final authorization; after approval, immediately broadcast the returned signed_tx in the same run.
