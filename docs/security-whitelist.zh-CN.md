# 转账白名单

[English](security-whitelist.md) | **简体中文** · [← README](../README.zh-CN.md)

白名单条目保存在 GUI 偏好文件（应用配置目录下的 `prefs.json`）。在 **两层** 执行校验，且 **空列表语义不同**：

## 1. 助手预检门控（agent 层）

助手将转账/授权的 `build_*` 调用转发到远程 MCP 前，直接从工具参数读取收款方/spender 并与白名单比对。**此处白名单为强制：未配置白名单时，一切转账/授权均被拒绝**，并提示先在「安全」标签添加 —— 不会构建、签名或广播。已有白名单时，非白名单地址同样拒绝，提示 `… is not on the whitelist …`。因地址来自工具参数（非原始 calldata），也覆盖 **ERC-20/721 合约转账**。受控工具：

| 工具 | 校验参数 | 类型 |
|------|----------|------|
| `build_bank_send` | `recipient` | Cosmos |
| `build_erc20_transfer`, `build_erc20_transfer_from` | `to` | EVM |
| `build_erc721_transfer_from`, `build_erc721_safe_transfer_from` | `to` | EVM |
| `build_bridge_deposit` | `recipient`（空 = 自身，允许） | EVM |
| `build_erc20_approve`, `build_erc721_approve` | `spender` | EVM |
| `build_erc721_set_approval_for_all` | `operator` | EVM |

## 2. 签名器兜底（sign 层）

本地签名时二次校验。此处 **空白名单表示不限制**（向后兼容）—— 上述强制白名单策略仅适用于 GUI 助手。有白名单时校验：

- **Cosmos** — `cosmos.bank.v1beta1.MsgSend` 收款方（`to_address`）
- **EVM** — 仅原生转账（`to` 非空且 `value` > 0）

签名层不解码合约调用与零 value EVM 交易，此类场景由上层预检拦截。独立 `svpchain-mcp` signer 读取同一 `prefs.json`，但不施加助手的强制白名单规则。
