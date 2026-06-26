# svpchain-agent

[English](README.md) | **简体中文**

面向 svpchain 的本地密钥 **交易 Agent**，采用严格的信任分离设计：

- **本地签名 MCP 服务**（`svpchain-mcp`）—— 签名密钥仅保存在本机，永不外泄；只对通过严格交叉校验的 payload / challenge 进行签名。
- **远程构建 + 广播 MCP 服务** —— 构造未签名交易、提供行情数据、广播已签名交易。运行于远端（`https://indexer.svpchain.com/mcp`）。
- **内置 LLM 助手**（`svpchain-gui`）—— 兼容 OpenAI 的工具调用循环，协调上述两者：远端 *构建* 与 *广播*，本地 *签名*。密钥永不离开本机。可选的 **转账白名单** 与模块化 **助手 Skills** 用于约束转出行为与提示词。
- **Google A2A（Agent-to-Agent）** —— 将本 Agent 暴露为符合 A2A 规范的 HTTP 服务，或通过 `a2a_send_message` 将子任务委托给其他 A2A Agent。

签名服务通过 **stdio** 运行（无网络端口；启动它的进程即为信任边界）。远端通过 HTTP 访问，并以签名 challenge 换取 bearer token 鉴权，远端同样不持有密钥。

**快速上手（GUI）：** 导入密钥 → **设置**（语言、链 ID、LLM API Key；按需展开 **LLM** 与 **Skills**）→ 可选 **安全** 白名单 → 在 **助手** 中交易，或导出 **MCP** 配置供 Cursor 使用。

## 架构

```
            ┌──────────────────────────────┐
            │  LLM (DeepSeek / OpenAI API) │   工具调用循环
            └───────────────┬──────────────┘
                            │
              ┌─────────────┴──────────────┐
   build_* / broadcast_* /        sign_transaction / sign_evm_transaction /
   行情 / whoami                   sign_typed_data / sign_challenge / whoami
              │                              │
   ┌──────────▼───────────┐      ┌───────────▼────────────┐
   │  远程 MCP (HTTP)     │      │  本地签名器 (stdio)    │
   │  构建 + 广播         │      │  本地持有密钥          │
   └──────────────────────┘      └────────────┬───────────┘
                                               │
                                    操作系统凭据存储
                              (Keychain / 凭据管理器 / Secret Service)
```

助手执行的链上写入流程：远端 `build_*` → 本地 `sign_*` → 远端 `broadcast_*`，`signed_tx` 字段须原样传递。鉴权使用本地签名的 `svpchain-mcp-auth-v1:` challenge，换取 bearer token。若已配置转账白名单，则在助手构建转账前（预检，含合约转账）与本地签名层均会校验。

**多 Agent** 场景下，助手可通过 `a2a_send_message` 调用远端 A2A Agent；也可运行 `svpchain-mcp a2a serve`，经 HTTP JSON-RPC 向其他 A2A 客户端暴露同一套编排循环。

## 项目结构

```
cmd/
  svpchain-mcp/   # stdio 签名 MCP CLI：serve（默认）/ import / delete / list / a2a serve
  svpchain-gui/   # Wails GUI：Go 入口 + 内嵌 Vue 前端
internal/
  agent/          # LLM 工具调用循环：远程 MCP 客户端 + 进程内本地签名器；转账预检白名单；会话记忆
    skills/       # 内置 SKILL.md 模块；组装助手 system prompt
  a2a/            # A2A 客户端：解析 Agent Card、SendMessage、解析回复
  a2aserver/      # A2A HTTP 服务端：Agent Card、JSON-RPC /invoke、executor → agent.Run
  mcp/            # MCP 工具处理器（sign_transaction / sign_evm_transaction / sign_typed_data / sign_challenge / whoami）
  signer/         # 交易与 challenge 签名（eth_secp256k1）；转账策略校验
  whitelist/      # 地址白名单存储与收款方校验（供 agent 门控与 signer 使用）
  manage/         # 密钥导入 / 列表 / 删除、MCP 配置生成、远程 URL
  keystore/       # 操作系统凭据存储读写
  payload/        # TxPayload / SignedTx / EvmTxPayload 类型
  prefs/          # prefs.json  schema、加载/保存（单一配置源）
  desktop/        # Wails 应用绑定（密钥、MCP 配置、设置、助手、安全、更新）
  update/         # 应用内更新（GitHub Releases、校验、安装；macOS DMG + Windows zip）
packaging/
  macos/          # .app 资源：Info.plist、图标、中/英本地化、用户指南
  windows/        # Windows 用户指南（运行前先阅读）
scripts/          # 打包（macOS DMG、Windows zip）、Wails 图标同步、图标生成
```

## 签名工具

以下工具运行于本地签名器（GUI 助手内为进程内调用）。每个工具均绑定到配置的链 ID，拒绝跨链使用。

| 工具 | 输入 → 输出 | 说明 |
|------|-------------|------|
| `sign_transaction` | `payload`（远端 `build_*` 返回的 `TxPayload`）→ `signed_tx` | 使用 `eth_secp256k1` + `SIGN_MODE_DIRECT` 签名 **Cosmos** 交易。拒绝 `chain_id` ≠ 配置的 `--chain-id` 或 `signer_address` ≠ 已加载密钥的 payload。若配置了 **白名单**，该链的 `MsgSend.to_address` 须在列表中。返回供 `broadcast_signed_tx` 使用的 `TxRaw`。 |
| `sign_evm_transaction` | `payload`（远端 EVM `build_*` 返回的 `EvmTxPayload`）→ `signed_tx` | 使用 **同一密钥** 签名原始 **Ethereum** 交易（EIP-1559 或 legacy）。拒绝 `evm_chain_id` ≠ 配置 EVM 链或 `signer_address`（0x）≠ 已加载密钥的 payload。若配置了白名单，**原生转账**（`to` 非空且 `value` > 0）的目标地址须在该链白名单中。返回供 `eth_sendRawTransaction` 使用的 RLP `raw_tx_hex`。 |
| `sign_typed_data` | `typed_data`（EIP-712 / `eth_signTypedData_v4`）→ `{signature, signer}` | 为 **x402** 无 gas 支付签名：EIP-3009 `TransferWithAuthorization`（USDC）或 Permit2 `PermitWitnessTransferFrom`（ERC-20 回退）。仅允许特定 `primaryType`；`domain.chainId` 须与 signer 的 EVM 链一致。 |
| `sign_challenge` | `challenge`（文本）→ `{signature, owner}` | 签名 svpchain 自助鉴权 challenge。**拒绝** 不以 `svpchain-mcp-auth-v1:` 开头或链 ID 不匹配的文本 —— 不可用作通用消息签名 oracle。 |
| `whoami` | 无 → `{owner, chain_id, evm_owner, evm_chain_id}` | 返回 bech32 `svp1…` 地址及对应 `0x` EVM 地址（同一密钥），以及配置的 Cosmos/EVM 链 ID。密钥本身永不返回。 |

GUI 助手还暴露 **仅本地** 工具（stdio MCP 服务不包含）：

| 工具 | 说明 |
|------|------|
| `evm_to_bech32` | 将 `0x` 地址转换为对应的 `svp1…` bech32 地址（向 EVM 地址 `build_bank_send` 前必须先调用）。 |
| `http_fetch` | 用于 x402 付费内容的 HTTP GET/POST。 |
| `x402_prepare_typed_data` / `x402_build_payment` | 收到 402 响应后，构建并组装 x402 v2 EIP-3009 支付头。 |
| `a2a_send_message` | 向其他 **A2A 兼容 Agent** 发送消息并返回回复（见 [Agent-to-Agent (A2A)](#agent-to-agent-a2a)）。 |

`v0.1` 对通过链 ID 与 signer 地址交叉校验的合法 payload 自动批准。**转账白名单** 在助手预检层（转账、授权、跨链桥，含 ERC-20/721 合约调用 —— **强制：无白名单则禁止一切转账**）与签名层（Cosmos `MsgSend` 与 EVM 原生转账；空列表在签名层表示不限制）分别生效；详见 [转账白名单](#转账白名单)。按工具限额、提示模式与 MCP elicitation 规划中。

## 图形界面（svpchain-gui）

GUI 涵盖密钥管理、MCP 导出、安全策略与内置助手。

### 标签页

| 标签 | 用途 |
|------|------|
| **助手** | 自然语言对话，驱动 build → sign → broadcast。选择链 ID，输入指令，查看分步进度。 |
| **密钥 / 导入** | 导入、列出、删除签名密钥；查看每条链对应的 `svp1…` 与 `0x` 地址。 |
| **安全** | 管理 **转账白名单**（链 ID + Cosmos 或 EVM 地址，可选别名）。GUI 助手转账前须至少有一条白名单；独立 signer 在空列表时不限制。 |
| **MCP** | 生成供 Cursor 等客户端使用的 stdio MCP JSON；自动检测捆绑的 `svpchain-mcp` 二进制。 |
| **设置** | 可折叠分区 —— **基本**（语言、默认链 ID）、**LLM**（API Key、Base URL、模型、远程 MCP URL）、**助手 Skills**（启用/禁用提示词模块）。 |
| **关于** | 版本与信任模型摘要。 |

### 助手与 LLM 设置

助手使用兼容 OpenAI 的 API（默认 base `https://api.deepseek.com`，模型 `deepseek-v4-flash`）。在 **设置 → LLM** 中配置 API Key、Base URL、模型与远程 MCP 端点并保存。远程 MCP 默认 `https://indexer.svpchain.com/mcp`。

应用支持 **中英文**（设置 → 基本；持久化）。可用 `SVPCHAIN_AGENT_LANG=zh|en` 覆盖首次启动语言检测。

### 会话记忆（Session memory）

此前每次助手对话开始时，LLM 都会先调用 `signer_whoami`（本地密钥信息）和 `whoami`（远端租户策略）。**会话记忆** 将这两份结果持久化到文件，并在后续对话中直接注入 system prompt，从而跳过重复的 tool 调用。

**工作流程：**

1. 远程 MCP 鉴权完成后，agent 读取 `agent_memory.json`（与 `prefs.json` 同目录）。
2. 若当前 **链 ID + 远程 MCP URL + 本地 owner** 存在有效缓存，将其 JSON 以 **Cached session context** 段落追加到 system prompt —— 并指示 LLM 勿再调用 `signer_whoami` 或 `whoami`。
3. 若无缓存或已失效，agent 各调用一次（界面显示 `Loading session context…`），写入文件后再注入 prompt。
4. 若 LLM 仍调用上述工具，`dispatchTool` 直接返回缓存 JSON，不再发起网络/本地重复查询。

**缓存失效** — 以下任一变化会触发重新拉取：

- 链 ID
- 远程 MCP URL
- 本地签名密钥（owner 地址）

**文件位置**（与 `prefs.json` 同目录）：

- macOS：`~/Library/Application Support/com.svpchain.agent-gui/agent_memory.json`
- Windows：`%AppData%\com.svpchain.agent-gui\agent_memory.json`
- Linux：`~/.config/com.svpchain.agent-gui/agent_memory.json`

`svpchain-mcp a2a serve` 使用同一套机制。

### 转账白名单

白名单条目保存在 GUI 偏好文件（应用配置目录下的 `prefs.json`）。在 **两层** 执行校验，且 **空列表语义不同**：

**1. 助手预检门控（agent 层）。** 助手将转账/授权的 `build_*` 调用转发到远程 MCP 前，直接从工具参数读取收款方/spender 并与白名单比对。**此处白名单为强制：未配置白名单时，一切转账/授权均被拒绝**，并提示先在「安全」标签添加 —— 不会构建、签名或广播。已有白名单时，非白名单地址同样拒绝，提示 `… is not on the whitelist …`。因地址来自工具参数（非原始 calldata），也覆盖 **ERC-20/721 合约转账**。受控工具：

| 工具 | 校验参数 | 类型 |
|------|----------|------|
| `build_bank_send` | `recipient` | Cosmos |
| `build_erc20_transfer`, `build_erc20_transfer_from` | `to` | EVM |
| `build_erc721_transfer_from`, `build_erc721_safe_transfer_from` | `to` | EVM |
| `build_bridge_deposit` | `recipient`（空 = 自身，允许） | EVM |
| `build_erc20_approve`, `build_erc721_approve` | `spender` | EVM |
| `build_erc721_set_approval_for_all` | `operator` | EVM |

**2. 签名器兜底（sign 层）。** 本地签名时二次校验。此处 **空白名单表示不限制**（向后兼容）—— 上述强制白名单策略仅适用于 GUI 助手。有白名单时校验：

- **Cosmos** — `cosmos.bank.v1beta1.MsgSend` 收款方（`to_address`）
- **EVM** — 仅原生转账（`to` 非空且 `value` > 0）

签名层不解码合约调用与零 value EVM 交易，此类场景由上层预检拦截。独立 `svpchain-mcp` signer 读取同一 `prefs.json`，但不施加助手的强制白名单规则。

### 助手 Skills

助手 system prompt 由模块化 **skills**（`internal/agent/skills/bundled/*/SKILL.md`）组装，而非单一硬编码字符串。每个 skill 覆盖一种工作流（链上 build/sign/broadcast、x402 支付、向 `0x` 银行转账、ERC-20/721、A2A 委托等）。

- **内置 skills** 嵌入二进制。
- **用户 skills** — 可选覆盖：`<config-dir>/com.svpchain.agent-gui/skills/<name>/SKILL.md`（与 `prefs.json` 同目录；macOS 如 `~/Library/Application Support/...`，Windows 为 `%AppData%`）。
- **设置 → 助手 Skills** — 开关各 skill（保存为 `prefs.json` 中的 `disabled_skills`）。`base` skill 锁定开启。禁用的 skill 不会写入 system prompt；可用 MCP 工具仍决定哪些绑定工具的 skill 在运行时注入。

### macOS `.app` 包

```sh
make package-macos-app
open "build/svpchain agent.app"
```

生成 `build/svpchain agent.app` 与 `build/svpchain-agent-<version>-macos.dmg`。DMG 内含 **svpchain agent.app**、README 与 **应用程序** 快捷方式 —— 拖入即可安装。包内包含 `svpchain-gui` 与 `svpchain-mcp`；配置页可自动检测 signer 路径。转发给其他 Mac 用户时，请让对方 **先阅读 运行前先阅读.txt**。

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

### Windows 发布

```powershell
$env:CGO_ENABLED = "1"
.\scripts\package-windows.ps1
```

或使用 Make（需 PowerShell 7+）：

```sh
make package-windows-app
```

生成 `build\svpchain agent\`（含 `svpchain-gui.exe` + `svpchain-mcp.exe`）与 `build\svpchain-agent-<version>-windows-amd64.zip`。解压后运行 `svpchain-gui.exe`。两个 exe 须在同一目录。转发前请阅读 **运行前先阅读.txt**。

Windows GUI 支持从 GitHub Releases 应用内更新（仅 stable 标签）：下载 release zip、校验 `SHA256SUMS`、替换安装目录并重启。开发构建（`*-dev`）跳过此检查。

## Agent-to-Agent (A2A)

本项目通过 [`a2a-go`](https://github.com/a2aproject/a2a-go) 实现 [Google A2A 协议](https://google.github.io/A2A/)。A2A 与 MCP 互补：MCP 连接助手与工具；A2A 连接 Agent 与 Agent。

### 暴露本 Agent（A2A 服务端）

启动 HTTP JSON-RPC 服务，发布 **Agent Card**，并通过与 GUI 助手相同的 `agent.Run` 循环处理入站任务（远程 MCP build/broadcast + 本地签名）：

```sh
./build/svpchain-mcp a2a serve --chain-id svp-2517-1 --listen :8080 --public-url http://127.0.0.1:8080
```

| 参数 | 必填 | 说明 |
|------|------|------|
| `--chain-id` | 否* | Cosmos 链 ID。默认取 `prefs.json` 中的 `agent_chain_id`。 |
| `--listen` | 否 | TCP 监听地址（默认 `:8080`）。 |
| `--public-url` | 否 | 写入 Agent Card 的公网基础 URL（默认 `http://127.0.0.1` + 监听端口）。反向代理或公网部署时请正确设置。 |

\* 链 ID 须通过参数或 `prefs.json` 提供其一。

**端点：**

| 路径 | 用途 |
|------|------|
| `GET /.well-known/agent-card.json` | Agent Card 发现（skills、能力、invoke URL） |
| `POST /invoke` | JSON-RPC A2A 方法（`SendMessage`、任务流式推送、取消） |

LLM 设置（`llm_api_key`、`llm_base_url`、`llm_model`）与远程 MCP URL 从 `prefs.json` 读取。Agent 循环进度以 A2A artifact 流式推送；任务取消会传播到正在运行的 agent 上下文。

### 调用其他 Agent（A2A 客户端）

GUI 助手可通过本地工具 `a2a_send_message` 将子任务委托给远端 A2A Agent：

| 参数 | 说明 |
|------|------|
| `agent_url` | 远端 Agent 的基础 URL（客户端从此 URL 拉取 `/.well-known/agent-card.json`）。示例：`http://localhost:9001` |
| `message` | 发送给远端 Agent 的纯文本消息 |

返回 JSON：`{ "task_id", "context_id", "state", "response" }`。

当 `a2a_send_message` 可用时注入内置 **a2a** skill。可在 **设置 → 助手 Skills** 中开关。

### 安全说明

- 远端 A2A Agent **永远** 无法获得本地签名密钥。
- 不要在 A2A 消息中发送私钥、助记词或原始密钥材料。
- 除非完全信任远端 Agent 的签名流程，否则优先委托只读或咨询类任务。

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

## 存储密钥

签名密钥保存在 **操作系统凭据存储** —— macOS Keychain、Windows 凭据管理器或 Linux Secret Service（libsecret）—— 绝不通过命令行参数或客户端配置传递。导入一次即可：

```sh
# 交互式隐藏输入
./build/svpchain-mcp import --chain-id <chain-id>
Enter private key (hidden): ********
Stored key for svp1… (<chain-id>)

# …或管道传入（如从密码管理器）
printf '%s' <32-byte-hex> | ./build/svpchain-mcp import --chain-id <chain-id>
```

密钥以服务名 `svpchain-agent`、**链 ID** 为账户名存储；多条链可共存。规则为 **每条链一把密钥** —— 主网密钥用于测试网会扩大风险面，故 `import` 在检测到同一密钥已存于其他链时会警告。再次 `import` 会覆盖（密钥轮换）。

```sh
./build/svpchain-mcp list                           # 列出已存链 ID
./build/svpchain-mcp delete --chain-id <chain-id>   # 删除密钥
```

## 运行签名器

```sh
./build/svpchain-mcp --chain-id <chain-id>
```

| 参数 | 必填 | 说明 |
|------|------|------|
| `--chain-id` | 是 | 本 signer 绑定的链 ID。拒绝其他链的 payload/challenge，并选择同名存储密钥。 |
| `--evm-chain-id` | 否 | `sign_evm_transaction` 的 EIP-155 数字链 ID。默认从 `--chain-id` 解析 —— `svp_2517-1` 与 `svp-2517-1` 均为 `2517`。若 `--chain-id` 无法解析且未设此参数，则禁用 EVM 签名（Cosmos 签名不受影响）。 |

密钥从操作系统凭据存储读取。**没有 `--key-hex` 参数** —— 否则密钥会泄露到进程参数与 shell 历史。

### 无头环境回退

无 Secret Service 守护进程的 Linux 无头主机（CI、Docker）可设置 `SIGNER_KEY_HEX`；凭据存储无密钥时使用：

```sh
SIGNER_KEY_HEX=<32-byte-hex> ./build/svpchain-mcp --chain-id <chain-id>
```

服务将密钥来源（`OS credential store` 或 `SIGNER_KEY_HEX env`）写入 stderr —— 永不输出密钥本身。

> macOS 说明：读取 Keychain 需要 CGO 构建（`make build` 与 release 二进制默认启用）。`import` 后首次运行可能弹出一次性 Keychain 访问提示。

## MCP 客户端配置

在外部 MCP 客户端（如 Cursor）中通过 stdio 指向本二进制：

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

配置中不含密钥 —— 服务在 `import` 后从凭据存储读取。对外服务名为 `svpchain-agent`。（GUI 内置助手在进程内运行同一 signer，无需单独 MCP 客户端。）

## 测试

```sh
make test
```
