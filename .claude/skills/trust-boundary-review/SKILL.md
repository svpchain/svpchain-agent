---
name: trust-boundary-review
description: Review checklist for changes to the svpchain signing trust boundary — signer cross-checks, the auth challenge guard, and the two-layer transfer whitelist. Use before editing internal/signer, internal/payload, internal/whitelist, internal/mcp, whitelist_gate.go, or chainid.go.
---

# Trust-boundary review

This repo's whole reason to exist is a **three-party trust separation**. The signing key
never leaves the local machine; the remote service builds unsigned txs and broadcasts
signed ones but holds no key; the LLM only orchestrates. The checks below are the trust
boundary — not validation niceties. Treat any change that weakens them as a defect.

## The core invariant

Every state-changing action is **remote `build_*` → local `sign_*` → remote `broadcast_*`**,
passing `signed_tx` fields verbatim. There is no shortcut and no "sign this raw bytes"
path. If a change introduces a way to sign something that didn't come from a `build_*`
payload, stop.

## What the signer must keep refusing

- **Wrong chain.** The signer is bound to one `--chain-id`. It refuses any payload or
  challenge whose chain id differs. (`internal/signer/`, EVM side checks `evm_chain_id`.)
- **Wrong signer.** It refuses any payload whose `signer_address` isn't the loaded key.
- **Non-auth challenges.** `sign_challenge` refuses any text not starting with
  `svpchain-mcp-auth-v1:` + the matching chain id. It is never a generic signing oracle.

## The two whitelist layers — opposite empty-list semantics

This is the easiest thing to get wrong.

1. **Assistant pre-flight gate** — `internal/agent/whitelist_gate.go`. Reads recipient/
   spender from the tool *arguments* before forwarding a `build_*`. **Empty whitelist =
   refuse ALL transfers/approvals.** Catches ERC-20/721 contract calls because it reads
   args, not calldata.
2. **Signer fallback** — `internal/signer/`. Re-checks at sign time. **Empty whitelist =
   unrestricted** (backward compatible). Only decodes Cosmos `MsgSend` recipients and EVM
   native sends (`to` set, `value` > 0); contract / zero-value txs pass through here and
   rely on layer 1.

The standalone `svpchain-mcp` signer reads the same `prefs.json` but does **not** impose
the mandatory-empty rule — that's the GUI assistant's policy. Keep these distinct.

## Other structural rules

- `internal/payload/` is **intentionally I/O-free** — wire types only, so the signer
  imports without chain/HTTP deps. Don't add `net/http`, file, or chain imports there.
- `internal/signer` `init()` sets the svp bech32 prefixes; import this package rather than
  blank-importing `internal/config`.
- Keys never appear in process args (no `--key-hex`); headless fallback is `SIGNER_KEY_HEX`.
  Never log, echo, or transmit key material.

## Checklist before declaring done

- [ ] Chain-id binding intact on both Cosmos and EVM paths.
- [ ] `signer_address`-matches-loaded-key check intact.
- [ ] `sign_challenge` prefix + chain-id guard intact.
- [ ] Whitelist layer 1 still refuses on empty list; layer 2 still unrestricted on empty.
- [ ] `internal/payload/` still I/O-free.
- [ ] Tests added/updated in the matching `_test.go` (signer, whitelist, gate).
- [ ] `gofmt -w` + `go vet ./...` clean.

For a deeper pass, run `/trust-check` to have the `trust-boundary-auditor`
subagent review the current diff.
