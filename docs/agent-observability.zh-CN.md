# 助手可观测性与评估方案

[English](agent-observability.md) | **简体中文** · [← README](../README.zh-CN.md)

本文档说明 SVPChain Agent 内置助手的**效果评估思路**、**可观测性设计**及**已落地实现**。适用于产品、开发与运维在不上云（无 LangSmith 等 SaaS trace）的前提下，本地调试、回归与持续改进。

---

## 1. 背景与目标

助手链路为：

```
用户自然语言 → LLM 工具循环 → 远程 MCP build_* → 本地 sign_* → 远程 broadcast_*
```

评估不能只看「模型回复是否像人话」，而要以**链上结果**与**编排是否正确**为准。本方案目标：

| 目标 | 说明 |
|------|------|
| **可观测** | 每次对话有完整 trace，可复盘工具链、失败点、tx hash |
| **可量化** | 任务成功率、拒绝率、轮次、延迟等可统计 |
| **可回归** | 改 prompt / skill / 门控逻辑后，离线用例可自动跑分 |
| **隐私优先** | trace 默认落本机，不含私钥与 LLM API Key |

与 LangSmith / Langfuse 的差异：后者擅长云端 LLM trace 与协作看板；本方案侧重**本地密钥 + 链上 outcome**，数据不出本机。

---

## 2. 评估维度（四层指标）

### Layer A — 任务完成（Outcome，最重要）

| 指标 | 定义 |
|------|------|
| Intent success rate | 用户意图是否真正完成 |
| Tx success rate | `broadcast_*` 后链上是否成功 |
| Correctness | 金额、market、方向、收款地址等是否符合意图 |
| Abstain rate | 该拒绝时是否拒绝（无白名单、余额不足等） |

**Ground truth** 在链上：需将 run log 中的 `tx_hashes` 与 indexer / 查询工具结果对照。

### Layer B — 编排质量（Orchestration）

| 指标 | 定义 |
|------|------|
| Tool sequence accuracy | 是否遵循 build → sign → broadcast，有无漏步/多余 whoami |
| Round count | LLM 轮次（上限 25） |
| Fail-fast 合理性 | tool 失败后是否恰当终止（当前策略：不喂回 LLM 重试） |

### Layer C — 模型层（LLM）

| 指标 | 定义 |
|------|------|
| Intent classification | 查询 vs 下单等是否搞混 |
| Slot filling | chain、symbol、size、price 等槽位 |
| Hallucination rate | 是否编造 tx hash / 余额 |

### Layer D — 体验与成本

| 指标 | 定义 |
|------|------|
| E2E latency | 发消息 → 首 token → 完成 |
| Token / 费用 | 按模型与任务类型 |
| Cancel / timeout rate | GUI 180s watchdog |
| 重复提问率 | 用户同一意图多问一次的代理指标 |

**安全类指标**（白名单拒绝、签名 cross-check）单独统计，不与任务成功率混报。

---

## 3. 已落地：本地 Run 日志（JSONL）

### 3.1 代码位置

| 组件 | 路径 |
|------|------|
| 记录器 | `internal/agent/runlog/` |
| 接入点 | `internal/agent/runner.go` → `Config.RunLog` |
| GUI 开关 | 设置 → 基础 → **记录助手运行日志** |
| Prefs 字段 | `agent_run_log_disabled`（`false` = 开启，默认开启） |
| 读取 API | `AgentRunLogPath()`、`AgentRecentRuns(limit)` |

### 3.2 日志文件路径

与 `prefs.json` 同目录，文件名为 **`agent_runs.jsonl`**：

| 平台 | 路径 |
|------|------|
| macOS | `~/Library/Application Support/com.svpchain.agent-gui/agent_runs.jsonl` |
| Linux | `$XDG_CONFIG_HOME/com.svpchain.agent-gui/agent_runs.jsonl` |
| Windows | `%AppData%\com.svpchain.agent-gui\agent_runs.jsonl` |

### 3.3 单条记录结构（每行一个 JSON）

```json
{
  "run_id": "uuid",
  "started_at": "2026-06-25T12:00:00Z",
  "finished_at": "2026-06-25T12:00:15Z",
  "chain_id": "svp-2517-1",
  "remote_url": "https://indexer.svpchain.com/mcp",
  "model": "deepseek-v4-flash",
  "provider": "openai",
  "user_message": "查一下 BTC 永续仓位",
  "outcome": "success",
  "answer": "...",
  "error": "",
  "tx_hashes": ["0x..."],
  "round_count": 2,
  "steps": [
    {
      "at": "2026-06-25T12:00:01Z",
      "kind": "tool",
      "round": 1,
      "tool": "get_positions",
      "args": "{...}",
      "ok": true,
      "result": "{...}",
      "elapsed_ms": 320
    }
  ]
}
```

### 3.4 outcome 取值

| 值 | 含义 |
|----|------|
| `success` | 正常结束并返回答案 |
| `failed` | 返回 error（含 LLM、远程 MCP、配置缺失等） |
| `stopped` | tool 失败后的 fail-fast 停止 |
| `rejected` | 白名单 / 转账门控拒绝 |
| `cancelled` | 用户取消或 context 超时 |

### 3.5 隐私与脱敏

- **不记录**：私钥、LLM API Key、完整 32 字节 hex 密钥材料
- **截断**：过长字段（默认 2000 字符）
- **用户消息**经 redact 后写入

关闭记录：设置页关闭「记录助手运行日志」，或于 `prefs.json` 设 `"agent_run_log_disabled": true`。

### 3.6 查看示例

```bash
# 美化输出最近一条
tail -1 ~/Library/Application\ Support/com.svpchain.agent-gui/agent_runs.jsonl | jq .

# 统计 outcome 分布
jq -r .outcome ~/Library/Application\ Support/com.svpchain.agent-gui/agent_runs.jsonl | sort | uniq -c
```

---

## 4. 已落地：离线 Eval 回归集

### 4.1 用例文件

`testdata/agent_eval/guard_cases.json` — 当前覆盖 **转账白名单门控**（`internal/agent/guard`），无需 LLM 与网络。

### 4.2 运行方式

```bash
./scripts/agent-eval.sh

# 或
go test ./internal/agent/eval/... ./internal/agent/runlog/... -count=1
```

### 4.3 扩展用例格式

```json
{
  "id": "guard_no_whitelist_bank_send",
  "description": "未配置白名单时 Cosmos 转账必须拒绝",
  "chain_id": "svp-2517-1",
  "tool": "build_bank_send",
  "args": { "recipient": "svp1..." },
  "expect_outcome": "rejected"
}
```

代码：`internal/agent/eval/` — `LoadGuardCases`、`ScoreGuardCases`。

---

## 5. 与 LangSmith 等方案的选型建议

| 能力 | LangSmith / Langfuse | 本地方案（已实现 / 规划） |
|------|----------------------|---------------------------|
| LLM + tool trace | ✅ 云端 | ✅ `agent_runs.jsonl` |
| Dataset 回归 | ✅ | ✅ `guard_cases.json`（可扩展） |
| 链上 tx 关联 | ❌ 需自建 | ✅ `tx_hashes` 字段 |
| 私钥不出本机 | ⚠️ 需脱敏与合规 | ✅ 默认本地 |
| 团队协作看板 | ✅ | 可选自托管 Langfuse |

**建议路径：**

1. **现阶段**：JSONL + guard 回归 + 人工 weekly review 失败 run  
2. **中期**：补充 mock remote MCP 的 LLM 用例；broadcast 后 indexer 校验  
3. **可选**：自托管 Langfuse，仅同步脱敏后的 span  

---

## 6. 推荐工作流

### 日常开发

1. 助手完成几笔典型操作（查询、下单、转账拒绝等）  
2. 打开 `agent_runs.jsonl` 检查 tool 序列与 outcome  
3. 修改 guard / skill 后执行 `./scripts/agent-eval.sh`  

### 每周质量复盘

1. 导出近 7 天 JSONL，按 `outcome` 分组  
2. 抽检 `failed` / `stopped`：分类为意图理解、工具参数、远程 MCP、签名、白名单、用户配置  
3. 将典型失败补进 `testdata/agent_eval/`  

### 发版前

- `go test ./internal/agent/eval/...` 必须通过  
- 测试网冒烟 5–10 条核心场景（下单、撤单、查仓位）并核对 `tx_hashes`  

---

## 7. 后续规划（未实现）

| 项 | 说明 |
|----|------|
| Mock MCP replay | CI 不连真网，录制 tool 响应回放 |
| LLM eval 用例 | 自然语言 → 期望 tool 名/参数（需 mock LLM 或固定 seed） |
| GUI run 历史 | 设置页展示 `AgentRecentRuns` |
| 链上 outcome 校验 | broadcast 成功后自动 query 订单/持仓 delta |
| 指标看板 | 本地脚本聚合 JSONL 生成周报 |

---

## 8. 相关代码索引

```
internal/agent/runlog/     # JSONL 记录、脱敏、tx hash 提取、读取
internal/agent/eval/       # 离线回归加载与打分
internal/agent/runner.go   # RunLog 接入
internal/desktop/agent.go  # GUI 启用记录
internal/desktop/runlog.go # AgentRunLogPath / AgentRecentRuns
testdata/agent_eval/       # 回归用例
scripts/agent-eval.sh      # 一键跑 eval 测试
```

---

## 9. 修订记录

| 日期 | 说明 |
|------|------|
| 2026-06 | 初版：JSONL run log、guard eval、设置开关 |
