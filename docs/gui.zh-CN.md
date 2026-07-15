# 图形界面（svpchain-gui）

[English](gui.md) | **简体中文** · [← README](../README.zh-CN.md)

GUI 涵盖密钥管理、MCP 导出、安全策略与内置助手。

## 标签页

| 标签 | 用途 |
|------|------|
| **助手** | 自然语言对话，驱动 build → sign → broadcast。选择链 ID，输入指令，查看分步进度。对话支持 **多轮上下文并持久化** —— 顶栏可切换历史对话或新建对话（见 [助手记忆与上下文](assistant-context.zh-CN.md)）。 |
| **密钥 / 导入** | 导入、列出、删除签名密钥；查看每条链对应的 `svp1…` 与 `0x` 地址。 |
| **安全** | 管理 **转账白名单**（链 ID + Cosmos 或 EVM 地址，可选别名）。GUI 助手转账前须至少有一条白名单；独立 signer 在空列表时不限制（见 [转账白名单](security-whitelist.zh-CN.md)）。 |
| **MCP** | 生成供 Cursor 等客户端使用的 stdio MCP JSON；自动检测捆绑的 `svpchain-mcp` 二进制。 |
| **设置** | 可折叠分区 —— **基本**（语言、默认链 ID、调用过程显示、运行日志）、**LLM**（API Key、Base URL、模型、上下文窗口、远程 MCP URL）、**助手 Skills**（启用/禁用提示词模块）。 |
| **关于** | 版本与信任模型摘要。 |

## 助手与 LLM 设置

助手支持 **OpenAI 兼容** API（默认 base `https://api.deepseek.com`，模型 `deepseek-v4-flash`），以及原生 **Anthropic**（当提供商 / Base URL 解析为 Anthropic 时走 `/v1/messages`）。回复以流式写入聊天界面。在 **设置 → LLM** 中配置 API Key、Base URL、模型与远程 MCP 端点并保存。远程 MCP 默认 `https://indexer.svpchain.com/mcp`。

应用支持 **中英文**（设置 → 基本；持久化）。可用 `SVPCHAIN_AGENT_LANG=zh|en` 覆盖首次启动语言检测。

## 助手 Skills

助手 system prompt 由模块化 **skills**（`internal/agent/skills/bundled/*/SKILL.md`）组装，而非单一硬编码字符串。每个 skill 覆盖一种工作流（链上 build/sign/broadcast、x402 支付、向 `0x` 银行转账、ERC-20/721、A2A 委托等）。

- **内置 skills** 嵌入二进制。skill 可将大体积细节（输出模板、错误话术目录）放在 `SKILL.md` 旁的 `references/*.md` 中；助手通过本地工具 `read_skill_reference` 按需加载，而不是把它们塞进每次的 system prompt。
- **用户 skills** — 可选覆盖：`<config-dir>/com.svpchain.agent-gui/skills/<name>/SKILL.md`（与 `prefs.json` 同目录；macOS 如 `~/Library/Application Support/...`，Windows 为 `%AppData%`）。
- **设置 → 助手 Skills** — 开关各 skill（保存为 `prefs.json` 中的 `disabled_skills`）。`base` skill 锁定开启。禁用的 skill 不会写入 system prompt；可用 MCP 工具仍决定哪些绑定工具的 skill 在运行时注入。
