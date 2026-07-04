# 助手记忆、对话历史与运行日志

[English](assistant-context.md) | **简体中文** · [← README](../README.zh-CN.md)

## 会话记忆（Session memory）

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

## 对话历史与上下文管理

GUI 助手支持 **多轮对话**：上一轮的问题、回答与工具调用会在下一条消息时回传给 LLM，且对话在应用重启后仍然保留。实现位于 `internal/agent/history/`。

**持久化** —— 每个对话是应用配置目录下的一个 JSONL 会话文件：

```
sessions/
  index.json          # 对话列表 + 当前对话 id
  <id>.jsonl          # 每行一条消息（user / assistant / tool）
  <id>.summary.json   # 压缩状态（见下）
  blobs/              # 大体积工具结果的完整存档（投影）
```

助手顶栏新增 **历史对话下拉框**、**新对话** 按钮与删除按钮。重新打开应用会恢复当前对话。切换链 ID 发消息时会自动新建对话（其他链的上下文会产生误导）。

**上下文管理** —— 三个机制把长对话控制在模型上下文窗口内（**设置 → 大模型 → 上下文窗口** 可配，默认 64000 tokens；历史约占其 70%）：

1. **投影（Projection）** —— 超过 4 KB 的工具结果归档到 `sessions/blobs/` 并在会话中截断。当轮运行看到的始终是完整结果，只有后续轮次看到截断版。
2. **LLM 压缩（Compaction）** —— 估算历史超出预算时，除最近 4 轮外的旧对话由 LLM 总结为一个摘要块并替换原文。总结 prompt 强制逐字保留地址、tx hash、金额、订单号与用户明确的约束。再次压缩会把旧摘要一并折叠，状态大小有界。
3. **配对修复（Pairing repair）** —— 持久化前将每个 assistant 工具调用补齐对应的 tool 结果（中断的运行补 `(not executed)`）；OpenAI 与 Anthropic 都会拒绝未配对的工具调用。

**隐私** —— 用户消息落盘前会做私钥形状（64 位 hex）脱敏，会话文件权限 `0600`，system prompt 不持久化（每轮由 skills 重新组装）。删除对话会连带删除其文件。

Desktop bindings：`AgentSessions`、`AgentNewSession`、`AgentSwitchSession`、`AgentDeleteSession`、`AgentTranscript`、`AgentCurrentSessionID`。

## 运行日志与评估

每次助手运行会向 `agent_runs.jsonl`（同配置目录）追加一条 JSONL trace：带耗时的工具调用、outcome（`success | failed | stopped | rejected | cancelled`）、提取的 tx hash，以及 **每轮 LLM 延迟与 token 用量**（`llm_rounds`、`usage`）。私钥与 API Key 均脱敏。开关位于 **设置 → 基础 → 记录助手运行日志**。离线评估用例在 `testdata/agent_eval/`，运行 `./scripts/agent-eval.sh`。完整设计见 [Agent 可观测性](agent-observability.zh-CN.md)。
