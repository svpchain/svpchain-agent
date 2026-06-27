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

> You have **no shell** — every HTTP call goes through the `http_fetch` tool, and
> every encode/decode/sign step is a tool call. There is no `curl`, `base64`, or
> `python`. Never claim to "run" a command; call the tool.

> ⚠️ This costs real (test) funds. Each unlock authorizes a transfer of the
> `value` to the `to` address shown in the typed data. Read those fields off the
> `x402_prepare_typed_data` output and verify them before you call `sign_typed_data`.

---

## The flow (all tool calls)

```
1. http_fetch GET (no payment)        ──► 402 + payment_required (base64) in the result
2. x402_prepare_typed_data            ──► typed_data (random 32-byte nonce + validBefore)
3. verify typed_data.message.to/value ──► confirm recipient + amount before signing
4. sign_typed_data                    ──► 65-byte signature
5. x402_build_payment                 ──► payment_b64 for X-PAYMENT / PAYMENT-SIGNATURE
6. http_fetch GET the SAME url + header ─► 200 OK + resource body
```

---

### Step 0 — Confirm the signer (do this first)

Use the **Cached session context** if present; otherwise call `signer_whoami`.
Confirm before signing anything:

- `evm_owner` is the address that will pay (it becomes the `from` field).
- `evm_chain_id` == `2517` — the signer refuses to sign for any other chain.

### Step 1 — Initial request (no payment) → 402

Call `http_fetch` with `method: "GET"` and headers that ask for the
**machine-readable** 402 instead of the HTML paywall — `Accept: application/json`
plus a non-browser `User-Agent`:

```json
{
  "url": "<resource-url the user gave you>",
  "method": "GET",
  "headers": { "Accept": "application/json", "User-Agent": "svpchain-agent/1.0" }
}
```

> Use the exact URL the user provided — do not edit, guess, or "fix" its path.
> Many sites carry a locale segment (e.g. `/zh-TW/`, `/en/`) that varies by
> language; it is **not** a fixed value, so never hard-code or substitute one.

On a 402 the result JSON includes:

- `status`: `402`
- `payment_required`: the base64 requirements string (pass this to step 2 verbatim)
- `payment_submit_header`: which header to send the payment back in —
  `PAYMENT-SIGNATURE` (direct API) or `X-PAYMENT` (via web page)

If `payment_required` is absent, the server did not return machine-readable
requirements — stop and report it rather than guessing.

### Step 2 — Prepare typed data (generates the nonce — never hand-build it)

Call `x402_prepare_typed_data`:

```json
{
  "payment_required": "<payment_required from step 1>",
  "from": "<evm_owner>"
}
```

It decodes the requirements and returns `typed_data`, `accepted`, `nonce`, and
`valid_before` with a cryptographically random 32-byte nonce. **The decoded
values in this output are authoritative** — never hard-code amount/payTo from a
cached guide.

### Step 3 — Verify what you are about to authorize

Read `typed_data.message` from step 2:

- `to` — the recipient you are paying (`payTo`).
- `value` — the exact amount being transferred.
- `validBefore` — the authorization expiry.

These are the only values that matter for safety. Confirm they match the
resource you intend to unlock before continuing.

### Step 4 — Sign

Call `sign_typed_data`, passing the `typed_data` object from step 2 **verbatim**.
Returns a 65-byte `0x…` signature (`v` normalized to 27/28). The signer refuses
any payload whose `domain.chainId` is not its configured chain (2517).

### Step 5 — Build the payment header value

Call `x402_build_payment`:

```json
{
  "accepted": { "...from x402_prepare_typed_data..." },
  "signature": "0x...from sign_typed_data...",
  "authorization": { "...= typed_data.message..." }
}
```

Returns `payment_b64` — the value for the header named by `payment_submit_header`
(`PAYMENT-SIGNATURE` for direct API, `X-PAYMENT` for web).

### Step 6 — Resubmit the SAME GET with the payment header

Call `http_fetch` on the **same url** with the same `Accept`/`User-Agent` plus
the payment header:

```json
{
  "url": "<the SAME resource-url from step 1>",
  "method": "GET",
  "headers": {
    "Accept": "application/json",
    "User-Agent": "svpchain-agent/1.0",
    "X-PAYMENT": "<payment_b64>"
  }
}
```

Use the header name from `payment_submit_header` (`X-PAYMENT` or
`PAYMENT-SIGNATURE`). A `200` returns the unlocked resource body.

---

## Reference: typed_data shape (read-only — it is generated for you)

`x402_prepare_typed_data` produces this; inspect it only to verify fields. The
`domain.name`/`version` come from the requirements' `extra`, `verifyingContract`
is the **asset (token) address itself**, and `chainId` is the numeric part of
`network`.

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

---

## Gotchas

- **No shell.** Use `http_fetch` for every request and the `x402_*` tools for
  encode/decode — there is no `curl`/`base64`/`python` available to you.
- **Never invent the nonce.** It must be exactly 32 bytes (`0x` + 64 hex chars).
  Always get it from `x402_prepare_typed_data`; a hand-built 31-byte hex string
  breaks EIP-712 hashing.
- **Treat each 402's `payment_required` as authoritative** — re-prepare per
  request; never reuse cached amount/payTo.
- **HTML vs JSON 402**: a browser-like `Accept: text/html` + `Mozilla` UA returns
  the rendered HTML paywall. Use `Accept: application/json` + a non-browser UA.
- **Short window**: `validBefore` is ~30 min with a fresh random nonce each time —
  if it expires, re-run from step 2; never reuse a nonce.
- **Chain guard**: the signer only signs for its configured `evm_chain_id`
  (2517); a payload for another chain is refused before signing.
- **EIP-3009 only**: this server's token implements EIP-3009, so no `approve` is
  needed. (A Permit2 `PermitWitnessTransferFrom` fallback exists for tokens
  lacking EIP-3009, but is not used here.)
