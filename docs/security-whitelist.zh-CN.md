# 转账白名单

[English](security-whitelist.md) | **简体中文** · [← README](../README.zh-CN.md)

白名单条目保存在 GUI 偏好文件（应用配置目录下的 `prefs.json`）。转账校验使用完整持久化列表和内置协议地址，并在两层执行。

## 1. 助手预检门控（agent 层）

助手将转账 `build_*` 调用转发到远程 MCP 前，直接从工具参数读取收款方并与有效白名单比对。非白名单地址会被拒绝，提示
`… is not on the whitelist …`。因地址来自工具参数（非原始 calldata），也覆盖 **ERC-20/721 合约转账**。受控工具：

| 工具                                                            | 校验参数                       | 类型   |
|-----------------------------------------------------------------|--------------------------------|--------|
| `build_bank_send`                                               | `recipient`                    | Cosmos |
| `build_erc20_transfer`, `build_erc20_transfer_from`             | `to`                           | EVM    |
| `build_erc721_transfer_from`, `build_erc721_safe_transfer_from` | `to`                           | EVM    |
| `build_bridge_deposit`                                          | `recipient`（空 = 自身，允许） | EVM    |

`approve`、额度和 operator 调用不受收款方白名单限制，但仍需经过本地签名确认。

## 2. 签名器兜底（sign 层）

本地签名时，如果用户已保存至少一条白名单，则按保存的白名单二次校验；空保存列表为兼容独立 signer 仍不限制，GUI 助手的预检门控仍使用有效白名单：

- **Cosmos** — `cosmos.bank.v1beta1.MsgSend` 收款方（`to_address`）
- **EVM** — 原生转账及标准 ERC-20/721/1155 转账的最终收款方

`approve` 和未知的零 value 合约调用不受转账白名单限制。提示词中的别名列表仅作辅助，且会省略无别名地址；最终是否允许必须由 build/sign 门控读取完整白名单决定。
