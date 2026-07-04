# Build, packaging & testing

**English** | [简体中文](build-and-packaging.zh-CN.md) · [← README](../README.md)

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

## macOS `.app` bundle

```sh
make package-macos-app
open "build/SVPChain Agent.app"
```

This produces `build/SVPChain Agent.app` and `build/svpchain-agent-<version>-macos.dmg`. The DMG contains **SVPChain Agent.app**, README files, and an **Applications** shortcut — drag the app to install. The bundle includes both `svpchain-gui` and `svpchain-mcp`; the config tab can auto-detect the signer path. When forwarding to other Mac users, send the DMG as-is and ask them to **read 运行前先阅读.txt first**.

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

## Windows release

```powershell
$env:CGO_ENABLED = "1"
.\scripts\package-windows.ps1
```

Or with Make (requires PowerShell 7+):

```sh
make package-windows-app
```

This produces `build\SVPChain Agent\` (contains `svpchain-gui.exe` + `svpchain-mcp.exe`) and `build\svpchain-agent-<version>-windows-amd64.zip`. Extract the zip and run `svpchain-gui.exe`. Both executables must stay in the same folder. Read **运行前先阅读.txt** before forwarding to other users.

The Windows GUI supports in-app updates from GitHub Releases (stable tags only): download the release zip, verify `SHA256SUMS`, replace the install folder, and restart. Dev builds (`*-dev`) skip this check.

## Testing

```sh
make test
```
