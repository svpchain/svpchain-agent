# 架构与项目结构

[English](architecture.md) | **简体中文** · [← README](../README.zh-CN.md)

## 架构

```
            ┌──────────────────────────────┐
            │  LLM (OpenAI 兼容 / Anthropic) │   流式工具调用循环
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

助手执行的链上写入流程：远端 `build_*` → 本地 `sign_*` → 远端 `broadcast_*`，`signed_tx` 字段须原样传递。鉴权使用本地签名的 `svpchain-mcp-auth-v1:` challenge，换取 bearer token。若已配置转账白名单，则在助手构建转账前（预检，含合约转账）与本地签名层均会校验 —— 见 [转账白名单](security-whitelist.zh-CN.md)。

**多 Agent** 场景下，助手可通过 `a2a_send_message` 调用远端 A2A Agent；也可运行 `svpchain-mcp a2a serve`，经 HTTP JSON-RPC 向其他 A2A 客户端暴露同一套编排循环 —— 见 [Agent-to-Agent (A2A)](a2a.zh-CN.md)。

## 项目结构

```
cmd/
  svpchain-mcp/   # stdio 签名 MCP CLI：serve（默认）/ import / delete / list / a2a serve
  svpchain-gui/   # Wails GUI：Go 入口 + 内嵌 Vue 前端
internal/
  agent/          # LLM 工具调用循环：远程 MCP 客户端 + 进程内本地签名器；转账预检白名单；会话记忆
    skills/       # 内置 SKILL.md（及 references/*.md）；system prompt + 按需 read_skill_reference
    history/      # 对话持久化 + 上下文管理（JSONL 会话、投影、LLM 压缩）
    runlog/       # 本地 JSONL 运行日志（工具、outcome、tx hash、token 用量），用于排查与评估
    eval/         # 白名单门控的离线回归打分
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
