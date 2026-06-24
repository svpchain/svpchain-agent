# svpchain-agent

A local-key **trading agent** for svpchain, built around a strict separation of trust:

- **Local signing MCP service** (`svpchain-mcp`) — keeps the user's signing key on the local machine, never exposes it, and only signs payloads/challenges that pass strict cross-checks.
- **Remote build + broadcast MCP service** — constructs unsigned transactions, serves market data, and broadcasts signed transactions. Runs off-machine (`https://indexer.svpchain.com/mcp`).
- **Built-in LLM assistant** (`svpchain-gui`) — an OpenAI-compatible tool-calling loop that orchestrates the two: the remote side *builds* and *broadcasts*, the local side *signs*. Keys never leave the machine. Optional **transfer whitelist** and modular **assistant skills** tighten outbound transfers and prompt behavior.

The signer runs over **stdio** (no network port; the process that starts it is the trust boundary). The remote side is reached over HTTP and gated by a signed-challenge bearer token, so the remote never holds a key either.

**Quick start (GUI):** import a key → **Settings** (language, chain id, LLM API key; expand **LLM** and **Skills** as needed) → optional **Security** whitelist → trade from **Assistant** or export **MCP** config for Cursor.

## Architecture

```
            ┌──────────────────────────────┐
            │  LLM (DeepSeek / OpenAI API) │   tool-calling loop
            └───────────────┬──────────────┘
                            │
              ┌─────────────┴──────────────┐
   build_* / broadcast_* /        sign_transaction / sign_evm_transaction /
   market data / whoami           sign_typed_data / sign_challenge / whoami
              │                              │
   ┌──────────▼───────────┐      ┌───────────▼────────────┐
   │  Remote MCP (HTTP)   │      │  Local signer (stdio)  │
   │  builds + broadcasts │      │  holds the key locally │
   └──────────────────────┘      └────────────┬───────────┘
                                               │
                                    OS credential store
                              (Keychain / Cred Mgr / Secret Service)
```

On-chain write flow the assistant follows: remote `build_*` → local `sign_*` → remote `broadcast_*`, passing `signed_tx` fields verbatim. Authentication uses a signed `svpchain-mcp-auth-v1:` challenge (signed locally), exchanged for a bearer token. When configured, the local signer also enforces a transfer whitelist before signing.

## Project layout

```
cmd/
  svpchain-mcp/   # stdio signing MCP CLI: serve (default) / import / delete / list
  svpchain-gui/   # Wails GUI: Go entry + embedded Vue frontend
internal/
  agent/          # LLM tool-calling loop: remote MCP client + in-process local signer
    skills/       # Bundled SKILL.md modules; composes the assistant system prompt
  mcp/            # MCP tool handlers (sign_transaction / sign_evm_transaction / sign_typed_data / sign_challenge / whoami)
  signer/         # transaction + challenge signing (eth_secp256k1); transfer policy checks
  whitelist/      # address whitelist store + enforcement at sign time
  manage/         # key import / list / delete, MCP config generation, remote URL
  keystore/       # OS credential store read/write
  payload/        # TxPayload / SignedTx / EvmTxPayload types
  prefs/          # prefs.json schema, load/save (single config source)
  desktop/        # Wails app bindings (keys, MCP config, settings, assistant, security, update)
  update/         # in-app update (GitHub releases, verify, install; macOS DMG + Windows zip)
packaging/
  macos/          # .app assets: Info.plist, icon, en/zh-Hans localization, user guides
  windows/        # Windows user guides (READ-BEFORE-RUN)
scripts/          # packaging (macOS DMG, Windows zip), Wails icon sync, icon generation
```

## Signing tools

These run in the local signer (and in-process inside the GUI assistant). Every tool is bound to the configured chain and refuses cross-chain use.

| Tool | Input → Output | Description |
|------|----------------|-------------|
| `sign_transaction` | `payload` (a `TxPayload` from remote `build_*`) → `signed_tx` | Signs a **Cosmos** transaction with `eth_secp256k1` + `SIGN_MODE_DIRECT`. Rejects payloads whose `chain_id` ≠ configured `--chain-id` and whose `signer_address` ≠ the loaded key. When a **whitelist** is configured, `MsgSend.to_address` must be on the list for that chain. Returns a `TxRaw` for `broadcast_signed_tx`. |
| `sign_evm_transaction` | `payload` (an `EvmTxPayload` from remote EVM `build_*`) → `signed_tx` | Signs a raw **Ethereum** transaction (EIP-1559 or legacy) with the **same key**. Rejects payloads whose `evm_chain_id` ≠ the configured EVM chain and whose `signer_address` (0x) ≠ the loaded key. When a whitelist is configured, **native transfers** (`to` set and `value` > 0) must target a whitelisted EVM address for that chain. Returns RLP `raw_tx_hex` for `eth_sendRawTransaction`. |
| `sign_typed_data` | `typed_data` (EIP-712 / `eth_signTypedData_v4`) → `{signature, signer}` | Signs **x402** gasless payments via EIP-3009 `TransferWithAuthorization` (USDC) or Permit2 `PermitWitnessTransferFrom` (ERC-20 fallback). Allowed `primaryType` values only; `domain.chainId` must match the signer's EVM chain. |
| `sign_challenge` | `challenge` (text) → `{signature, owner}` | Signs an svpchain self-service auth challenge. **Refuses** any text that does not start with `svpchain-mcp-auth-v1:` plus a matching chain id — never a generic message-signing oracle. |
| `whoami` | none → `{owner, chain_id, evm_owner, evm_chain_id}` | Returns the bech32 `svp1…` address **and** the corresponding `0x` EVM address (same key), plus the configured Cosmos/EVM chain ids. The key itself is never exposed. |

`v0.1` auto-approves well-formed payloads that pass chain-id and signer-address cross-checks. **Transfer whitelist** (Cosmos `MsgSend` and EVM native sends) is enforced when the GUI whitelist is non-empty. Per-tool limits, prompt modes, and MCP elicitation are planned.

## Graphical app (svpchain-gui)

The GUI covers key management, MCP export, security policy, and the built-in assistant.

### Tabs

| Tab | Purpose |
|-----|---------|
| **Assistant** | Natural-language chat that drives build → sign → broadcast. Pick a chain id, enter a command, watch step-by-step progress. |
| **Keys / Import** | Import, list, and delete signing keys; view derived `svp1…` and `0x` addresses per chain. |
| **Security** | Manage a **transfer whitelist** (chain id + Cosmos or EVM address). Empty list = no restriction (backward compatible). |
| **MCP** | Generate stdio MCP client JSON for Cursor and other clients; auto-detect the bundled `svpchain-mcp` binary. |
| **Settings** | Collapsible sections — **Basic** (language, default chain id), **LLM** (API key, base URL, model, remote MCP URL), **Assistant Skills** (enable/disable prompt modules). |
| **About** | Version and trust-model summary. |

### Assistant & LLM settings

The assistant uses an OpenAI-compatible API (default base `https://api.deepseek.com`, model `deepseek-v4-flash`). Configure API key, base URL, model, and remote MCP endpoint under **Settings → LLM**, then save. The remote MCP endpoint defaults to `https://indexer.svpchain.com/mcp`.

The app supports **English and Chinese** (Settings → Basic; persisted). Override first-launch detection with `SVPCHAIN_AGENT_LANG=zh|en`.

### Transfer whitelist

Whitelist entries live in the GUI preferences file (`prefs.json` under the app config directory) and are checked **before signing**:

- **Cosmos** — `cosmos.bank.v1beta1.MsgSend` recipient (`to_address`)
- **EVM** — native transfers only (`to` non-empty and `value` > 0)

Contract calls and zero-value EVM transactions are not filtered by the whitelist today. The same rules apply to the in-app assistant and the standalone `svpchain-mcp` signer (both read the shared `prefs.json`).

### Assistant skills

The assistant system prompt is assembled from modular **skills** (`internal/agent/skills/bundled/*/SKILL.md`), not a single hard-coded string. Each skill covers one workflow (on-chain build/sign/broadcast, x402 payments, bank send to `0x`, ERC-20/721, etc.).

- **Bundled skills** are embedded in the binary.
- **User skills** — optional overrides in `<config-dir>/com.svpchain.agent-gui/skills/<name>/SKILL.md` (alongside `prefs.json`; e.g. `~/Library/Application Support/...` on macOS, `%AppData%` on Windows).
- **Settings → Assistant Skills** — toggle skills on/off (saved as `disabled_skills` in `prefs.json`). The `base` skill is locked on. Disabled skills are omitted from the system prompt; available MCP tools still control which tool-bound skills are injected at runtime.

### macOS `.app` bundle

```sh
make package-macos-app
open "build/svpchain agent.app"
```

This produces `build/svpchain agent.app` and `build/svpchain-agent-<version>-macos.dmg`. The DMG contains **svpchain agent.app**, README files, and an **Applications** shortcut — drag the app to install. The bundle includes both `svpchain-gui` and `svpchain-mcp`; the config tab can auto-detect the signer path. When forwarding to other Mac users, send the DMG as-is and ask them to **read 运行前先阅读.txt first**.

Optional Developer ID signing for fewer Gatekeeper prompts:

```sh
SIGN_IDENTITY="Developer ID Application: Your Name (TEAMID)" make package-macos-app
```

Without Developer ID, the script applies a local ad-hoc signature (`codesign -`), which opens on the build machine; **运行前先阅读.txt** / **READ-BEFORE-RUN.txt** in the DMG explain the right-click-open steps for other Macs.

Regenerate the app icon from `packaging/logo-svp1.png`:

```sh
make build-macos-icon    # → packaging/macos/AppIcon.icns
make package-macos-app   # embed icon in .app bundle
```

The macOS `.app` checks GitHub Releases (stable tags only) on each launch and offers an in-app upgrade: download the release DMG, verify `SHA256SUMS`, replace the running `.app`, and restart. Dev builds (`*-dev`) and non-bundle runs skip this check.

### Windows release

```powershell
$env:CGO_ENABLED = "1"
.\scripts\package-windows.ps1
```

Or with Make (requires PowerShell 7+):

```sh
make package-windows-app
```

This produces `build\svpchain agent\` (contains `svpchain-gui.exe` + `svpchain-mcp.exe`) and `build\svpchain-agent-<version>-windows-amd64.zip`. Extract the zip and run `svpchain-gui.exe`. Both executables must stay in the same folder. Read **运行前先阅读.txt** before forwarding to other users.

The Windows GUI supports in-app updates from GitHub Releases (stable tags only): download the release zip, verify `SHA256SUMS`, replace the install folder, and restart. Dev builds (`*-dev`) skip this check.

## Build

```sh
make build          # → build/svpchain-mcp  (stdio signer)
make build-gui      # → cmd/svpchain-gui/build/bin/svpchain-gui(.app)
make build-all      # both
```

macOS, Windows, and Linux build natively. All platforms require CGO (`eth_secp256k1` uses libsecp256k1).

The GUI is a [Wails](https://wails.io) app (Go + embedded Vue). Building it needs the `wails` CLI and Node:

```sh
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

On macOS the GUI uses the system WebKit; on Linux it needs GTK3 + WebKit2GTK dev packages (`libgtk-3-dev libwebkit2gtk-4.1-dev`); on Windows it needs the [WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/) (usually pre-installed) and a CGO toolchain (MSVC or MinGW).

Before packaging, sync the Wails app icon from the repo logo:

```sh
./scripts/sync-wails-icon.sh   # → cmd/svpchain-gui/build/appicon.png
```

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

## Testing

```sh
make test
```
