# Local signer (svpchain-mcp)

**English** | [简体中文](signer.zh-CN.md) · [← README](../README.md)

## Signing tools

These run in the local signer (and in-process inside the GUI assistant). Every tool is bound to the configured chain and refuses cross-chain use.

| Tool | Input → Output | Description |
|------|----------------|-------------|
| `sign_transaction` | `payload` (a `TxPayload` from remote `build_*`) → `signed_tx` | Signs a **Cosmos** transaction with `eth_secp256k1` + `SIGN_MODE_DIRECT`. Rejects payloads whose `chain_id` ≠ configured `--chain-id` and whose `signer_address` ≠ the loaded key. When a **whitelist** is configured, `MsgSend.to_address` must be on the list for that chain. Returns a `TxRaw` for `broadcast_signed_tx`. |
| `sign_evm_transaction` | `payload` (an `EvmTxPayload` from remote EVM `build_*`) → `signed_tx` | Signs a raw **Ethereum** transaction (EIP-1559 or legacy) with the **same key**. Rejects payloads whose `evm_chain_id` ≠ the configured EVM chain and whose `signer_address` (0x) ≠ the loaded key. When a whitelist is configured, **native transfers** (`to` set and `value` > 0) must target a whitelisted EVM address for that chain. Returns RLP `raw_tx_hex` for `eth_sendRawTransaction`. |
| `sign_typed_data` | `typed_data` (EIP-712 / `eth_signTypedData_v4`) → `{signature, signer}` | Signs **x402** gasless payments via EIP-3009 `TransferWithAuthorization` (USDC) or Permit2 `PermitWitnessTransferFrom` (ERC-20 fallback). Allowed `primaryType` values only; `domain.chainId` must match the signer's EVM chain. |
| `sign_challenge` | `challenge` (text) → `{signature, owner}` | Signs an svpchain self-service auth challenge. **Refuses** any text that does not start with `svpchain-mcp-auth-v1:` plus a matching chain id — never a generic message-signing oracle. |
| `whoami` | none → `{owner, chain_id, evm_owner, evm_chain_id}` | Returns the bech32 `svp1…` address **and** the corresponding `0x` EVM address (same key), plus the configured Cosmos/EVM chain ids. The key itself is never exposed. |

The GUI assistant also exposes **local-only** tools that are not part of the stdio MCP server:

| Tool | Description |
|------|-------------|
| `evm_to_bech32` | Convert a `0x` address to the matching `svp1…` bech32 address (required before `build_bank_send` to an EVM address). |
| `http_fetch` | HTTP GET/POST for x402 paywalled content. |
| `x402_prepare_typed_data` / `x402_build_payment` | Build and assemble x402 v2 EIP-3009 payment headers after a 402 response. |
| `a2a_send_message` | Send a message to another **A2A-compatible agent** and return its reply (see [Agent-to-Agent (A2A)](a2a.md)). |

`v0.1` auto-approves well-formed payloads that pass chain-id and signer-address cross-checks. The **transfer whitelist** is enforced at the assistant pre-flight gate (transfers, approvals, bridge, including ERC-20/721 contract calls — and **mandatory: no whitelist means no transfers**) and again at the signer layer (Cosmos `MsgSend` and EVM native sends, where an empty list stays unrestricted); see [Transfer whitelist](security-whitelist.md). Per-tool limits, prompt modes, and MCP elicitation are planned.

## Storing keys

Signing keys live in the **OS credential store** — macOS Keychain, Windows Credential Manager, or Linux Secret Service (libsecret) — never via command-line arguments or client config. Import once:

```sh
# Interactive hidden input
./build/svpchain-mcp import --chain-id <chain-id>
Enter private key (hidden): ********
Stored key for svp1… (<chain-id>)

# …or pipe it in (e.g. from a password manager)
printf '%s' <32-byte-hex> | ./build/svpchain-mcp import --chain-id <chain-id>
```

Keys are stored under service name `svpchain-agent` with the **chain id** as the account name; multiple chains can coexist. The rule is **one key per chain** — sharing a mainnet key on testnet widens blast radius, so `import` warns when the same key is already stored under another chain. Running `import` again overwrites (key rotation).

```sh
./build/svpchain-mcp list                           # list stored chain ids
./build/svpchain-mcp delete --chain-id <chain-id>   # delete a key
```

## Running the signer

```sh
./build/svpchain-mcp --chain-id <chain-id>
```

| Flag | Required | Description |
|------|----------|-------------|
| `--chain-id` | yes | Chain id this signer is bound to. Rejects payloads/challenges for other chains and selects the stored key with the same name. |
| `--evm-chain-id` | no | Numeric EIP-155 chain id for `sign_evm_transaction`. Defaults to the number parsed from `--chain-id` — both `svp_2517-1` and `svp-2517-1` → `2517`. If `--chain-id` has no chain number and this flag is unset, EVM signing is disabled (Cosmos signing unaffected). |

The key is read from the OS credential store. **There is no `--key-hex` flag** — that would leak the key into process arguments and shell history.

### Headless fallback

On headless Linux hosts without a Secret Service daemon (CI, Docker), set `SIGNER_KEY_HEX`; the service uses it when no key is in the credential store:

```sh
SIGNER_KEY_HEX=<32-byte-hex> ./build/svpchain-mcp --chain-id <chain-id>
```

The service logs the key source (`OS credential store` or `SIGNER_KEY_HEX env`) to stderr — never the key itself.

> macOS note: reading the Keychain requires a CGO build (`make build` and release binaries enable it by default). The first run after `import` may show a one-time Keychain access prompt.

## MCP client configuration

To use the local signer from an external MCP client (e.g. Cursor), point it at the binary over stdio:

```json
{
  "mcpServers": {
    "svpchain-agent": {
      "command": "/absolute/path/to/build/svpchain-mcp",
      "args": ["--chain-id", "<chain-id>"]
    }
  }
}
```

The config does not contain the key — the service reads from the OS credential store after `import`. The service name exposed to clients is `svpchain-agent`. (The GUI's built-in assistant runs this same signer in-process, so it needs no separate MCP client.)
