---
name: x402
description: Access x402-protected paid HTTP content — fetch, sign an EIP-3009 payment off-chain, resubmit with the payment header. No gas, no manual broadcast.
version: 1
network: eip155:2517 (SVP Chain)
scheme: exact (EIP-3009 TransferWithAuthorization)
tools:
  - http_fetch
  - x402_prepare_typed_data
  - x402_build_payment
  - sign_typed_data
---

# Accessing x402-protected content (SVP Chain)

This skill documents the end-to-end flow for unlocking an **x402 v2** paywalled
resource. You sign an **EIP-3009 `TransferWithAuthorization`** off-chain; the
server's facilitator settles it on-chain — so you **pay no gas and broadcast no
transaction yourself**.

Worked example used throughout:
- Resource: `https://www.svpchain.org/zh-TW/x402/article`
- Guide:    `https://pre-faucet.svpchain.org/api/x402/guide`

> ⚠️ This costs real (test) funds. Each unlock transfers the `amount` of `asset`
> stated in the payment requirements. Decode and verify those values before signing.

---

## The 6-step flow

```
1. GET resource (no payment)        ──► 402 + PAYMENT-REQUIRED header
2. base64-decode PAYMENT-REQUIRED   ──► authoritative PaymentRequirements
3. x402_prepare_typed_data          ──► typed_data (random 32-byte nonce + validBefore)
4. sign_typed_data                  ──► 65-byte signature
5. x402_build_payment              ──► payment_b64 for X-PAYMENT / PAYMENT-SIGNATURE
6. Resend SAME GET with header      ──► 200 OK + resource body
```

> **Never invent `nonce` by hand.** LLMs often produce 31-byte hex strings that
> break EIP-712 hashing. Always use `x402_prepare_typed_data` for step 3.

---

### Step 0 — Confirm the signer (do this first)

```text
tool: signer_whoami   (local svpchain-agent signer)
```
Verify before signing anything:
- `evm_owner` is the address that will pay (becomes the `from` field).
- `evm_chain_id` == `2517` — the signer refuses to sign for any other chain.

Example output:
```json
{ "evm_owner": "0x18cE6b725D5Fa498210bC1788DAcfA5bc14dbadc", "evm_chain_id": "2517" }
```

---

### Step 1 — Initial request (no payment) → 402

To get the **machine-readable** 402 (not the HTML paywall), send
`Accept: application/json` **and** a non-browser `User-Agent`.

```bash
curl -s -D - -o /dev/null \
  -H "Accept: application/json" \
  -H "User-Agent: svpchain-agent/1.0" \
  "https://www.svpchain.org/zh-TW/x402/article"
```

Response headers of interest:
```
HTTP/2 402
payment-required: <base64...>        # the requirements (body is null)
x-payment-submit-header: PAYMENT-SIGNATURE (direct API) or X-PAYMENT (via web page)
link: </api/x402/premium>; rel="payment"
```
> HTTP/2 lowercases header names — match case-insensitively.

---

### Step 2 — Decode the requirements (authoritative)

```bash
echo "<payment-required-base64>" | base64 -d | python3 -m json.tool
```
```json
{
  "x402Version": 2,
  "accepts": [{
    "scheme": "exact",
    "network": "eip155:2517",
    "asset": "0x013a61E622e6ABFCaB64F52D274C3Fc0aA37f951",
    "amount": "10000",
    "payTo": "0xfBd15a89383f82FC869DbAb85480056812722852",
    "maxTimeoutSeconds": 60,
    "extra": { "name": "VanToken", "version": "1" }
  }]
}
```
**The decoded values win over anything cached in a guide.** Read `amount` and
`payTo` carefully — that is exactly what you're authorizing.

---

### Step 3 — Prepare typed data (use the tool — do not hand-build nonce)

```text
tool: x402_prepare_typed_data
```
```json
{
  "payment_required": "<base64 from http_fetch.payment_required>",
  "from": "0x18cE6b725D5Fa498210bC1788DAcfA5bc14dbadc"
}
```
Returns `typed_data`, `accepted`, `nonce`, and `valid_before` with a
cryptographically random 32-byte nonce.

### Step 4 — Sign

```text
tool: sign_typed_data
```
Pass the `typed_data` object from step 3 verbatim.

### Step 5 — Build payment header value

```text
tool: x402_build_payment
```
```json
{
  "accepted": { "...from x402_prepare_typed_data..." },
  "signature": "0x...from sign_typed_data...",
  "authorization": { "...typed_data.message..." }
}
```
Returns `payment_b64` — use as `X-PAYMENT` (web) or `PAYMENT-SIGNATURE` (API).

### Step 6 — Resubmit the SAME GET with the payment header

```bash
curl -s -D - \
  -H "Accept: application/json" \
  -H "User-Agent: svpchain-agent/1.0" \
  -H "X-PAYMENT: $PAYHDR" \
  "https://www.svpchain.org/zh-TW/x402/article"
```

---

## Reference: manual typed_data shape (for debugging only)

If you must inspect the EIP-712 fields, they look like this — but **always**
generate nonce via `x402_prepare_typed_data`, not manually:
```json
{
  "types": {
    "EIP712Domain": [
      { "name": "name", "type": "string" },
      { "name": "version", "type": "string" },
      { "name": "chainId", "type": "uint256" },
      { "name": "verifyingContract", "type": "address" }
    ],
    "TransferWithAuthorization": [
      { "name": "from", "type": "address" },
      { "name": "to", "type": "address" },
      { "name": "value", "type": "uint256" },
      { "name": "validAfter", "type": "uint256" },
      { "name": "validBefore", "type": "uint256" },
      { "name": "nonce", "type": "bytes32" }
    ]
  },
  "primaryType": "TransferWithAuthorization",
  "domain": {
    "name": "VanToken",
    "version": "1",
    "chainId": 2517,
    "verifyingContract": "0x013a61E622e6ABFCaB64F52D274C3Fc0aA37f951"
  },
  "message": {
    "from": "0x18cE6b725D5Fa498210bC1788DAcfA5bc14dbadc",
    "to": "0xfBd15a89383f82FC869DbAb85480056812722852",
    "value": "10000",
    "validAfter": "0",
    "validBefore": "1782371578",
    "nonce": "0x6824a62c6c7712fdde705cf7fee92673e28a6af59e9db4f6e278dbd15b05d41a"
  }
}
```
> `domain.name`/`version` come from `extra`. `verifyingContract` is the **asset
> (token) address itself**. `chainId` is the numeric part of `network`.

Returns a 65-byte `0x…` signature (`v` normalized to 27/28).

---

## Gotchas

- **Nonce must be exactly 32 bytes** (`0x` + 64 hex chars). Use
  `x402_prepare_typed_data` — never let the LLM invent a nonce.
- **Always decode `PAYMENT-REQUIRED` per request** — treat its values as
  authoritative; never hard-code amount/payTo from a cached guide.
- **HTML vs JSON 402**: a browser-like `Accept: text/html` + `Mozilla` UA gets
  the rendered HTML paywall. Use JSON `Accept` + non-browser UA for automation.
- **Window is short**: `validBefore` ~30 min and a fresh random `nonce` each
  time — re-sign if it expires; never reuse a nonce.
- **Chain guard**: the signer only signs for its configured `evm_chain_id`
  (2517). A payload for another chain is refused before signing.
- **EIP-3009 only**: this server's token implements EIP-3009, so no `approve`
  is needed. (A Permit2 `PermitWitnessTransferFrom` fallback exists for tokens
  lacking EIP-3009, but is not used here.)
