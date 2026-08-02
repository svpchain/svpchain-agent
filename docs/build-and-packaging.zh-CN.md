# 构建、打包与测试

[English](build-and-packaging.md) | **简体中文** · [← README](../README.zh-CN.md)

## 构建

```sh
make build          # → build/svpchain-mcp（stdio signer）
make build-gui      # → cmd/svpchain-gui/build/bin/svpchain-gui(.app)
make build-all      # 两者
```

macOS、Windows、Linux 均可原生构建。所有平台需要 CGO（`eth_secp256k1` 使用 libsecp256k1）。

GUI 为 [Wails](https://wails.io) 应用（Go + 内嵌 Vue）。构建需要 `wails` CLI 与 Node：

```sh
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

macOS 使用系统 WebKit；Linux 需要 GTK3 + WebKit2GTK 开发包（`libgtk-3-dev libwebkit2gtk-4.1-dev`）；Windows 需要 [WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/)（通常已预装）及 CGO 工具链（MSVC 或 MinGW）。

打包前从仓库 logo 同步 Wails 应用图标：

```sh
./scripts/sync-wails-icon.sh   # → cmd/svpchain-gui/build/appicon.png
```

## macOS `.app` 包

```sh
make package-macos-app
open "build/SVPChain Agent.app"
```

生成 `build/SVPChain Agent.app` 与 `build/svpchain-agent-<version>-macos.dmg`。DMG 内含 **SVPChain Agent.app**、README 与 **应用程序** 快捷方式 —— 拖入即可安装。包内包含 `svpchain-gui` 与 `svpchain-mcp`；配置页可自动检测 signer 路径。转发给其他 Mac 用户时，请让对方 **先阅读 运行前先阅读.txt**。

可选 Developer ID 签名以减少 Gatekeeper 提示：

```sh
SIGN_IDENTITY="Developer ID Application: Your Name (TEAMID)" make package-macos-app
```

无 Developer ID 时使用本地 ad-hoc 签名（`codesign -`），在构建机器上可直接打开；DMG 中的 **运行前先阅读.txt** / **READ-BEFORE-RUN.txt** 说明其他 Mac 的右键打开步骤。

从 `packaging/logo-svp1.png` 重新生成应用图标：

```sh
make build-macos-icon    # → packaging/macos/AppIcon.icns
make package-macos-app   # 嵌入 .app
```

macOS `.app` 每次启动检查 GitHub Releases（仅 stable 标签）并提供应用内升级：下载 release DMG、校验 `SHA256SUMS`、替换运行中的 `.app` 并重启。开发构建（`*-dev`）与非 bundle 运行跳过此检查。

## Windows 发布

```powershell
$env:CGO_ENABLED = "1"
.\scripts\package-windows.ps1
```

或使用 Make（需 PowerShell 7+）：

```sh
make package-windows-app
```

生成 `build\SVPChain Agent\`（含 `svpchain-gui.exe` + `svpchain-mcp.exe`）与 `build\svpchain-agent-<version>-windows-amd64.zip`。解压后运行 `svpchain-gui.exe`。两个 exe 须在同一目录。转发前请阅读 **运行前先阅读.txt**。

Windows GUI 支持从 GitHub Releases 应用内更新（仅 stable 标签）：下载 release zip、校验 `SHA256SUMS`、替换安装目录并重启。开发构建（`*-dev`）跳过此检查。

## 测试

```sh
make test
```
