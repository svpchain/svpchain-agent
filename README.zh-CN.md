# svpchain-agent

[English](README.md) | **简体中文**

面向 svpchain 的本地密钥 **链上 Agent**（Cosmos/EVM），采用严格的信任分离设计：

- **本地签名 MCP 服务**（`svpchain-mcp`）—— 签名密钥仅保存在本机，永不外泄；只对通过严格交叉校验的 payload / challenge 进行签名。
- **远程构建 + 广播 MCP 服务** ——
  构造未签名交易、提供行情数据、广播已签名交易。运行于远端（`https://mcp-testnet.svpchain.org/`）。
- **内置 LLM 助手**（`svpchain-gui`）—— 支持流式工具调用（OpenAI 兼容 API 或原生 Anthropic），协调上述两者：远端 *构建* 与
  *广播*，本地 *签名*。密钥永不离开本机。可选的 **转账白名单**、模块化 **助手 Skills**（大体积细节放在 `references/*.md`，经
  `read_skill_reference` 按需加载）、多轮 **对话历史** 与本地 **运行日志**，用于约束转出、提示词与可观测性。
- **智能体检索** —— 用自然语言描述任务，通过 **Agent Market** 服务检索远程 Agent。
- **Google A2A（Agent-to-Agent）** —— 通过 `a2a_send_message` 向其他 A2A Agent 提问（仅客户端；本 Agent 不作为网络服务运行）。该消息不携带任何账户权限。

签名服务通过 **stdio** 运行（无网络端口；启动它的进程即为信任边界）。远端通过 HTTP 访问，并以签名 challenge 换取 bearer token
鉴权，远端同样不持有密钥。

链上写入流程始终为：远端 `build_*` → 本地 `sign_*` → 远端 `broadcast_*`，`signed_tx` 字段须原样传递。

## 快速上手（GUI）

导入密钥 → **设置**（语言、链 ID、LLM API Key / 提供商、Agent Market 地址；按需展开 **LLM** 与 **Skills**）→ 可选 **安全**
白名单 → 在 **助手** 中发起链上操作（兑换、转账、跨链、ERC-20/721、Lendora 借贷、x402 等）或检索远程 Agent，或导出 **MCP**
配置供 Cursor 使用。

## 智能体检索

`search_agents` 接受一段自然语言任务描述，按语义相似度返回候选 Agent 及其 DID、A2A 服务地址、能力标签、定价与保证金；它在本机
直连 **Agent Market** 服务（设置 → Agent Market 地址，结果中回报为 `agent_market_url`），不经过 Remote MCP。随后可用
`a2a_send_message` 与对方通信 —— 该消息不携带任何凭证，链上写入仍走远端 `build_*` → 本地 `sign_*` → 远端 `broadcast_*`。

这些结果**只**来自该检索服务：它索引链上 `x/agent` 注册表，但本地不再回链核对，因此服务地址与能力属于该服务的声明而非已验证的链上
事实。服务地址决定 A2A 消息发往何处 —— 它动不了资金，但决定谁能读到这条消息。

```sh
make build-all      # build/svpchain-mcp + Wails GUI（需要 CGO）
make test
```

构建依赖与各平台环境见 [构建、打包与测试](docs/build-and-packaging.zh-CN.md)。

## 文档索引

| 文档                                                  | 内容                                                                       |
|-------------------------------------------------------|----------------------------------------------------------------------------|
| [架构与项目结构](docs/architecture.zh-CN.md)          | 信任模型架构图、链上写入流程、目录结构                                     |
| [本地签名器（svpchain-mcp）](docs/signer.zh-CN.md)    | 签名工具、密钥存储（操作系统凭据库）、运行签名器、Cursor 等 MCP 客户端配置 |
| [图形界面（svpchain-gui）](docs/gui.zh-CN.md)         | 标签页、LLM 设置（OpenAI 兼容 / Anthropic）、助手 Skills 与渐进式参考文件  |
| [助手记忆与上下文](docs/assistant-context.zh-CN.md)   | 会话记忆、对话历史与上下文管理、运行日志与评估                             |
| [转账白名单](docs/security-whitelist.zh-CN.md)        | 两层校验（助手预检 + 签名器兜底）及各自的空列表语义                        |
| [Agent-to-Agent (A2A)](docs/a2a.zh-CN.md)             | A2A 客户端（`a2a_send_message`）、安全说明                                 |
| [构建、打包与测试](docs/build-and-packaging.zh-CN.md) | 构建依赖、macOS `.app`/DMG、Windows zip、应用内更新、测试                  |
| [Agent 可观测性](docs/agent-observability.zh-CN.md)   | 运行日志、可选 Phoenix OTLP 与离线评估                                     |
