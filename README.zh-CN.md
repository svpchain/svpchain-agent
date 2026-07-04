# svpchain-agent

[English](README.md) | **简体中文**

面向 svpchain 的本地密钥 **链上 Agent**（Cosmos/EVM），采用严格的信任分离设计：

- **本地签名 MCP 服务**（`svpchain-mcp`）—— 签名密钥仅保存在本机，永不外泄；只对通过严格交叉校验的 payload / challenge 进行签名。
- **远程构建 + 广播 MCP 服务** —— 构造未签名交易、提供行情数据、广播已签名交易。运行于远端（`https://indexer.svpchain.com/mcp`）。
- **内置 LLM 助手**（`svpchain-gui`）—— 兼容 OpenAI 的工具调用循环，协调上述两者：远端 *构建* 与 *广播*，本地 *签名*。密钥永不离开本机。可选的 **转账白名单** 与模块化 **助手 Skills** 用于约束转出行为与提示词。
- **Google A2A（Agent-to-Agent）** —— 将本 Agent 暴露为符合 A2A 规范的 HTTP 服务，或通过 `a2a_send_message` 将子任务委托给其他 A2A Agent。

签名服务通过 **stdio** 运行（无网络端口；启动它的进程即为信任边界）。远端通过 HTTP 访问，并以签名 challenge 换取 bearer token 鉴权，远端同样不持有密钥。

链上写入流程始终为：远端 `build_*` → 本地 `sign_*` → 远端 `broadcast_*`，`signed_tx` 字段须原样传递。

## 快速上手（GUI）

导入密钥 → **设置**（语言、链 ID、LLM API Key；按需展开 **LLM** 与 **Skills**）→ 可选 **安全** 白名单 → 在 **助手** 中发起链上操作，或导出 **MCP** 配置供 Cursor 使用。

```sh
make build-all      # build/svpchain-mcp + Wails GUI（需要 CGO）
make test
```

构建依赖与各平台环境见 [构建、打包与测试](docs/build-and-packaging.zh-CN.md)。

## 文档索引

| 文档 | 内容 |
|------|------|
| [架构与项目结构](docs/architecture.zh-CN.md) | 信任模型架构图、链上写入流程、目录结构 |
| [本地签名器（svpchain-mcp）](docs/signer.zh-CN.md) | 签名工具、密钥存储（操作系统凭据库）、运行签名器、Cursor 等 MCP 客户端配置 |
| [图形界面（svpchain-gui）](docs/gui.zh-CN.md) | 标签页、助手与 LLM 设置、助手 Skills |
| [助手记忆与上下文](docs/assistant-context.zh-CN.md) | 会话记忆、对话历史与上下文管理、运行日志与评估 |
| [转账白名单](docs/security-whitelist.zh-CN.md) | 两层校验（助手预检 + 签名器兜底）及各自的空列表语义 |
| [Agent-to-Agent (A2A)](docs/a2a.zh-CN.md) | A2A 服务端（`a2a serve`）、A2A 客户端（`a2a_send_message`）、安全说明 |
| [构建、打包与测试](docs/build-and-packaging.zh-CN.md) | 构建依赖、macOS `.app`/DMG、Windows zip、应用内更新、测试 |
| [Agent 可观测性](docs/agent-observability.zh-CN.md) | 运行日志与离线评估的完整设计 |
