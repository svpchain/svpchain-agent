# Agent-to-Agent (A2A)

[English](a2a.md) | **简体中文** · [← README](../README.zh-CN.md)

本项目通过 [`a2a-go`](https://github.com/a2aproject/a2a-go) 实现 [Google A2A 协议](https://google.github.io/A2A/)。A2A 与 MCP 互补：MCP 连接助手与工具；A2A 连接 Agent 与 Agent。

## 暴露本 Agent（A2A 服务端）

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
