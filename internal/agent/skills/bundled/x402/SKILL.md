---
name: x402
description: Paid HTTP content via x402 — fetch, sign EIP-712 payment, retry with X-PAYMENT.
priority: 20
tools:
  - http_fetch
  - sign_typed_data
---

Fetch x402-gated HTTP content: a paid resource returns HTTP 402, you sign an off-chain EIP-712 payment authorization, and resend with the payment header. The server's facilitator settles it on-chain — **you pay no gas and broadcast no transaction yourself.** This is x402 v2.

## Access flow

1. GET the protected resource with no payment header. You receive **HTTP 402** with a base64 `PAYMENT-REQUIRED` response header — the body is null, the data is in the header.
2. base64-decode `PAYMENT-REQUIRED` → `PaymentRequirements`: `scheme`, `network`, `asset`, `amount` (atomic units), `payTo`, `extra.name`, `extra.version`. **Treat these as authoritative** over the values cached in this guide.
3. Produce the EIP-712 `TransferWithAuthorization` signature (see [Signing](#signing)): the payer authorizes transferring `amount` of `asset` to `payTo`. Set `validAfter=0`, `validBefore=now+~1800s`, `nonce=`random 32 bytes.
4. Assemble the `PaymentPayload` (see [Payload template](#payload-template)) by echoing the decoded requirements into `accepted` and putting the signature + authorization into `payload`.
5. base64-encode the `PaymentPayload` JSON and resend the **same** GET with the payment header set to that base64 — `PAYMENT-SIGNATURE` when hitting the API directly, `X-PAYMENT` when going through the web page (its server forwards `X-PAYMENT` to this service).
6. On success you get **HTTP 200** with the resource body plus a base64 `PAYMENT-RESPONSE` header carrying the on-chain settlement tx hash.

## Headers

- `PAYMENT-REQUIRED` — base64(JSON `PaymentRequirements`), returned by the server on the 402.
- Submit header — base64(JSON `PaymentPayload`, `x402Version=2`) in the payment request header, then resend. Both header names are accepted; pick by target:
  - `PAYMENT-SIGNATURE` — calling this service's API directly (e.g. `/api/x402/premium`).
  - `X-PAYMENT` — calling through the web page; the page server forwards it to this service.
- `PAYMENT-RESPONSE` — base64(JSON settlement result: `success`, `network`, `transaction`, `payer`), returned on success.

Over HTTP/2, header names arrive lowercased (`payment-required`, `payment-response`) — match case-insensitively.

## Resource types

A locked resource is either a **static page** (HTML, e.g. an article) or an **API endpoint** (JSON). Both are gated by the SAME x402 payment — settlement always happens through the x402/API layer; a protected page is just an HTML resource sitting behind that same paywall.

**API endpoint** — send `Accept: application/json`; submit payment in `PAYMENT-SIGNATURE`. 402 → body null, requirements in the base64 `PAYMENT-REQUIRED` header. 200 → JSON body plus base64 `PAYMENT-RESPONSE` header.

**Static page** — call the page URL on the site host; submit payment in `X-PAYMENT`, which the page server forwards here.
- **To get the machine-readable 402** (the base64 `PAYMENT-REQUIRED` header) instead of the human HTML paywall: send `Accept: application/json` AND a non-browser User-Agent. A request whose `Accept` contains `text/html` AND whose User-Agent contains `Mozilla` is treated as a browser and gets the rendered HTML paywall, not the header flow.
- 200 → page content (HTML or JSON) plus a base64 `PAYMENT-RESPONSE` header with the settlement tx.

## Signing

x402 has two EIP-712 signing modes, distinguished by `primaryType`. Sign the one the 402's `PaymentRequirements` demands. **This server uses the `exact` scheme → `TransferWithAuthorization`.**

| Mode | Applies when | Standard | verifyingContract | Approval |
|---|---|---|---|---|
| `TransferWithAuthorization` (active) | token implements EIP-3009; `exact` scheme | EIP-3009 | the token contract (`asset`) itself | none — facilitator calls `transferWithAuthorization` directly |
| `PermitWitnessTransferFrom` (fallback) | token does NOT implement EIP-3009 and 402 advertises a permit2 scheme | Permit2 (Uniswap canonical) | the canonical Permit2 contract, NOT the token | one-time `approve(Permit2)` on the token first |

Steps:

1. Sign with the svpchain-signer MCP `sign_typed_data` tool — it signs EIP-712 typed data and supports both modes. The key never leaves the signer.
2. First call the signer's `whoami` and confirm `evm_owner` == payer `from` and `evm_chain_id` == 2517 **before** signing.
3. Pass the EIP-712 typed data (`domain`, `types`, `primaryType`, `message`) to `sign_typed_data`. For this `exact`-scheme server use `primaryType: TransferWithAuthorization` with the typed data below and the authorization as the `message`.
4. Take the returned 65-byte `(r||s||v)` signature (v normalized to 27/28), place it into the `PaymentPayload`, base64-encode and submit per [Headers](#headers).

### EIP-712 typed data (exact / EIP-3009)

```json
{
  "domain": {
    "name": "VanToken",
    "version": "1",
    "chainId": 2517,
    "verifyingContract": "0x013a61E622e6ABFCaB64F52D274C3Fc0aA37f951"
  },
  "primaryType": "TransferWithAuthorization",
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
  }
}
```

## Payload template

`accepted` MUST mirror the `PaymentRequirements` you decoded from the 402 (the server reads `scheme`/`network` from it to route the payment). Fill `payload` with your authorization and signature, then base64-encode the whole object into the submit header.

```json
{
  "x402Version": 2,
  "accepted": {
    "scheme": "exact",
    "network": "eip155:2517",
    "asset": "0x013a61E622e6ABFCaB64F52D274C3Fc0aA37f951",
    "amount": "10000",
    "payTo": "0xfBd15a89383f82FC869DbAb85480056812722852",
    "maxTimeoutSeconds": 60,
    "extra": { "name": "VanToken", "version": "1" }
  },
  "payload": {
    "authorization": {
      "from": "<payer wallet address>",
      "to": "0xfBd15a89383f82FC869DbAb85480056812722852",
      "value": "10000",
      "validAfter": "0",
      "validBefore": "<unix seconds, e.g. now+1800>",
      "nonce": "0x<random 32-byte hex>"
    },
    "signature": "0x<65-byte EIP-712 signature>"
  }
}
```

## Reference values (this server)

Authoritative values always come from the decoded 402; these are cached defaults:

- protocol `x402`, version 2, scheme `exact`
- network `eip155:2517` (chainId 2517)
- asset `0x013a61E622e6ABFCaB64F52D274C3Fc0aA37f951` (VanToken)
- payTo `0xfBd15a89383f82FC869DbAb85480056812722852`
- amount `10000` (atomic units)
