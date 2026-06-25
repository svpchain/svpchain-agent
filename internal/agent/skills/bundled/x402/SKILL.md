---
name: x402
description: Paid HTTP content via x402 — fetch, sign EIP-712 payment, retry with X-PAYMENT.
priority: 20
tools:
  - http_fetch
  - sign_typed_data
---

For x402 paid HTTP content: use http_fetch; on 402, parse payment requirements from the body/headers, build EIP-712 typed_data, sign with sign_typed_data, encode payment in X-PAYMENT header, retry http_fetch.
