# Agent-to-Agent (A2A)

[English](a2a.md) | **简体中文** · [← README](../README.zh-CN.md)

本项目通过 [`a2a-go`](https://github.com/a2aproject/a2a-go) 实现 [Google A2A 协议](https://google.github.io/A2A/)的客户端。A2A 与 MCP 互补：MCP 连接助手与工具；A2A 连接 Agent 与 Agent。

本 Agent **只作为客户端**：由人类用户直接操作，向远端 A2A Agent 发起调用；它不提供 A2A 服务端——让一个持有用户签名密钥的进程对外监听网络，与本地钱包 Agent 的信任形态相悖。

## 调用其他 Agent（A2A 客户端）

GUI 助手可通过本地工具 `a2a_send_message` 将子任务委托给远端 A2A Agent：

| 参数 | 说明 |
|------|------|
| `agent_url` | 远端 Agent 的基础 URL（客户端从此 URL 拉取 `/.well-known/agent-card.json`）。示例：`http://localhost:9001` |
| `message` | 发送给远端 Agent 的纯文本消息 |

返回 JSON：`{ "task_id", "context_id", "state", "response" }`。

当 `a2a_send_message` 可用时注入内置 **a2a** skill。可在 **设置 → 助手 Skills** 中开关。

## 安全说明

- 远端 A2A Agent **永远** 无法获得本地签名密钥。
- 不要在 A2A 消息中发送私钥、助记词或原始密钥材料。
- 除非完全信任远端 Agent 的签名流程，否则优先委托只读或咨询类任务。
