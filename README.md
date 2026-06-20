# svpchain-mcp

Local **signing MCP service** for svpchain — the signing side of a dual-MCP agent architecture; the other side is a remote build + broadcast MCP service.

The service runs over **stdio** (no network port; the agent process that starts it is the trust boundary). It keeps the user's signing key on the local machine and never exposes the key itself. It only signs payloads and challenges that pass strict cross-checks (matching the configured chain id).

## Project layout

```
cmd/
  svpchain-mcp/   # MCP signing CLI entry (serve / import / delete / list)
  svpchain-gui/   # Wails graphical setup tool: Go entry + embedded Vue frontend/
internal/
  desktop/               # Wails app bindings (keys, MCP config, settings, update) exposed to the frontend
  mcp/                   # MCP tool handlers (sign_transaction / sign_evm_transaction / sign_challenge / whoami)
  manage/                # Key import, list, delete, MCP config generation
  signer/                # Transaction and challenge signing
  keystore/              # OS credential store read/write
  payload/               # TxPayload / SignedTx types
  update/                # macOS in-app update (GitHub releases, verify, install)
packaging/macos/         # .app assets (Info.plist, icon, en/zh-Hans localization, user guides, etc.)
scripts/                 # macOS packaging and icon generation scripts
```

## Tools

| Tool | Input → Output | Description |
|------|----------------|-------------|
| `sign_transaction` | `payload` (a `TxPayload` from remote `build_*` tools) → `signed_tx` | Signs a **Cosmos** transaction with `eth_secp256k1` + `SIGN_MODE_DIRECT`. Rejects payloads whose `chain_id` ≠ configured `--chain-id` (cross-chain replay guard) and payloads whose `signer_address` ≠ the loaded key. Returns a `TxRaw` ready for `broadcast_signed_tx`. |
| `sign_evm_transaction` | `payload` (an `EvmTxPayload` from remote EVM `build_*` tools) → `signed_tx` | Signs a raw **Ethereum** transaction (EIP-1559 or legacy) with the **same key**, built from structured fields. Rejects payloads whose `evm_chain_id` ≠ the signer's configured chain and payloads whose `signer_address` (0x) ≠ the loaded key. Returns RLP `raw_tx_hex` for `eth_sendRawTransaction`. |
| `sign_typed_data` | `typed_data` (EIP-712 / `eth_signTypedData_v4` shape) → `{signature, signer}` | Signs **x402** gasless payments via EIP-3009 `TransferWithAuthorization` (USDC) or Permit2 `PermitWitnessTransferFrom` (ERC-20 fallback). Allowed `primaryType` values only; `domain.chainId` must match the signer's EVM chain. Returns a `0x` signature for x402 payment payloads. |
| `sign_challenge` | `challenge` (text) → `{signature, owner}` | Signs an svpchain self-service auth challenge. **Refuses** any text that does not start with `svpchain-mcp-auth-v1:` plus a matching chain id — this signer is never a generic message-signing oracle. |
| `whoami` | none → `{owner, chain_id, evm_owner, evm_chain_id}` | Returns the bech32 `svp1…` address **and** the corresponding `0x` EVM address (same key) derived from the loaded key, plus the configured Cosmos and EVM chain ids, so the agent can confirm which key is in use. The key itself is never exposed. |

`v0.1` auto-approves: every well-formed payload that passes chain-id and signer-address cross-checks is signed. Per-tool limits, allowlists, prompt modes, MCP elicitation, and other approval/policy hooks are planned for later.

## Build

```sh
make build          # → build/svpchain-mcp
# or
go build -o build/svpchain-mcp ./cmd/svpchain-mcp
```

macOS and Linux build natively; Windows is not supported.

The GUI is a [Wails](https://wails.io) app (Go + an embedded Vue frontend). Building it requires the `wails` CLI and Node:

```sh
go install github.com/wailsapp/wails/v2/cmd/wails@latest
make build-gui      # → cmd/svpchain-gui/build/bin/svpchain-gui(.app)
```

On macOS the GUI uses the system WebKit (no bundled browser engine); on Linux it needs GTK3 + WebKit2GTK dev packages (`libgtk-3-dev libwebkit2gtk-4.1-dev`).

## Graphical setup (svpchain-gui)

If you prefer not to use the CLI, build and open the standalone setup app:

```sh
make build-gui
open cmd/svpchain-gui/build/bin/svpchain-gui.app    # macOS
# ./cmd/svpchain-gui/build/bin/svpchain-gui          # Linux
```

**macOS `.app` bundle (double-click to open):**

```sh
make package-macos-app
open "build/svpchain agent.app"
```

This produces `build/svpchain agent.app` and `build/svpchain-agent-<version>-macos.zip`. After unzipping, the archive root contains **svpchain agent.app** and README files (do not use an `.app.zip` suffix — macOS may create a spurious `.app` folder). The app display name is **svpchain agent**. The GUI supports **English and Chinese**: switch language on the Settings tab (preference is persisted), or override first-launch detection with `SVPCHAIN_AGENT_LANG=zh|en`. The app bundle includes both `svpchain-gui` and `svpchain-mcp`; the MCP config tab can auto-detect the signer binary path.

When distributing to other Mac users, forward the zip as-is and ask them to **read 运行前先阅读.txt inside the zip first**.

For distribution to other machines with fewer Gatekeeper prompts, optional Developer ID signing:

```sh
SIGN_IDENTITY="Developer ID Application: Your Name (TEAMID)" make package-macos-app
```

Without Developer ID, the script applies a local ad-hoc signature (`codesign -`), which opens directly on the build machine. For other Macs, **运行前先阅读.txt** / **READ-BEFORE-RUN.txt** in the zip explain right-click open and System Settings steps.

Regenerate the app icon from `packaging/logo-svp1.png`:

```sh
make build-macos-icon    # → packaging/macos/AppIcon.icns
make package-macos-app   # embed icon in .app bundle
```

The GUI provides:

- **Import** signing keys into the OS credential store (Chain ID + private key)
- **List / delete** stored keys and view derived `svp1…` addresses
- **Generate** MCP client JSON snippets (paste into Cursor settings)

The macOS `.app` checks GitHub Releases on each launch (stable tags only). When a newer version is available, it offers an optional in-app upgrade: download the release zip, verify `SHA256SUMS`, replace the running `.app`, and restart automatically. Dev builds (`*-dev`) and non-bundle runs skip this check.

The MCP signing service itself (`svpchain-mcp`) is still started by MCP clients over stdio — the GUI is for one-time setup only and does not replace running the signer.

Place `svpchain-mcp` and `svpchain-gui` in the same directory so the config tab can auto-detect the signer binary path.

## Storing keys

Signing keys live in the **OS credential store** — macOS Keychain, Windows Credential Manager, or Linux Secret Service (libsecret) — never via command-line arguments or MCP client config. Import once:

```sh
# Interactive hidden input
./build/svpchain-mcp import --chain-id <chain-id>
Enter private key (hidden): ********
Stored key for svp1… (<chain-id>)

# …or pipe it in (e.g. from a password manager)
printf '%s' <32-byte-hex> | ./build/svpchain-mcp import --chain-id <chain-id>
```

Keys are stored under service name `svpchain-agent` with **chain id** as the account name; multiple chains can coexist. The rule is **one key per chain** — sharing a mainnet key on testnet widens blast radius if testnet leaks, so `import` warns when the same key is already stored under another chain.

```sh
./build/svpchain-mcp list                           # list stored chain ids
./build/svpchain-mcp delete --chain-id <chain-id>   # delete a key
```

Running `import` again overwrites an existing key (key rotation).

## Running

```sh
./build/svpchain-mcp --chain-id <chain-id>
```

| Flag | Required | Description |
|------|----------|-------------|
| `--chain-id` | yes | Chain id this signer is bound to. Rejects payloads/challenges for other chains and selects the stored key with the same name. |
| `--evm-chain-id` | no | Numeric EIP-155 chain id for `sign_evm_transaction`. Defaults to the number parsed from `--chain-id` — both `svp_2517-1` and `svp-2517-1` → `2517`. If `--chain-id` has no chain number and this flag is unset, EVM signing is disabled (Cosmos signing unaffected). |

The key is read from the OS credential store. **There is no `--key-hex` flag** — that would leak the key into process arguments and shell history.

### Headless fallback

On headless Linux hosts without a Secret Service daemon (CI, Docker), set `SIGNER_KEY_HEX` and the service uses it when no key is in the credential store:

```sh
SIGNER_KEY_HEX=<32-byte-hex> ./build/svpchain-mcp --chain-id <chain-id>
```

The service logs the key source (`OS credential store` or `SIGNER_KEY_HEX env`) to stderr — never the key itself.

> macOS note: reading the Keychain requires a CGO build (`make build` and release binaries enable it by default). The first run after `import` may show a one-time Keychain access prompt.

## MCP client configuration

Point your MCP client at this binary over stdio. Example (`mcpServers` style):

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

The config does not contain the key — the service reads from the OS credential store (after `import`). The service name exposed to clients is `svpchain-agent`.