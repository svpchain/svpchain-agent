# 本地签名器（svpchain-mcp）

[English](signer.md) | **简体中文** · [← README](../README.zh-CN.md)

## 签名工具

以下工具运行于本地签名器（GUI 助手内为进程内调用）。每个工具均绑定到配置的链 ID，拒绝跨链使用。

| 工具 | 输入 → 输出 | 说明 |
|------|-------------|------|
| `sign_transaction` | `payload`（远端 `build_*` 返回的 `TxPayload`）→ `signed_tx` | 使用 `eth_secp256k1` + `SIGN_MODE_DIRECT` 签名 **Cosmos** 交易。拒绝 `chain_id` ≠ 配置的 `--chain-id` 或 `signer_address` ≠ 已加载密钥的 payload。若配置了 **白名单**，该链的 `MsgSend.to_address` 须在列表中。返回供 `broadcast_signed_tx` 使用的 `TxRaw`。 |
| `sign_evm_transaction` | `payload`（远端 EVM `build_*` 返回的 `EvmTxPayload`）→ `signed_tx` | 使用 **同一密钥** 签名原始 **Ethereum** 交易（EIP-1559 或 legacy）。拒绝 `evm_chain_id` ≠ 配置 EVM 链或 `signer_address`（0x）≠ 已加载密钥的 payload。若配置了白名单，**原生转账**（`to` 非空且 `value` > 0）的目标地址须在该链白名单中。返回供 `eth_sendRawTransaction` 使用的 RLP `raw_tx_hex`。 |
| `sign_typed_data` | `typed_data`（EIP-712 / `eth_signTypedData_v4`）→ `{signature, signer}` | 为 **x402** 无 gas 支付签名：EIP-3009 `TransferWithAuthorization`（USDC）或 Permit2 `PermitWitnessTransferFrom`（ERC-20 回退）。仅允许特定 `primaryType`；`domain.chainId` 须与 signer 的 EVM 链一致。 |
| `sign_challenge` | `challenge`（文本）→ `{signature, owner}` | 签名 svpchain 自助鉴权 challenge。**拒绝** 不以 `svpchain-mcp-auth-v1:` 开头或链 ID 不匹配的文本 —— 不可用作通用消息签名 oracle。 |
| `whoami` | 无 → `{owner, chain_id, evm_owner, evm_chain_id}` | 返回 bech32 `svp1…` 地址及对应 `0x` EVM 地址（同一密钥），以及配置的 Cosmos/EVM 链 ID。密钥本身永不返回。 |

GUI 助手还暴露 **仅本地** 工具（stdio MCP 服务不包含）：

| 工具 | 说明 |
|------|------|
| `evm_to_bech32` | 将 `0x` 地址转换为对应的 `svp1…` bech32 地址（向 EVM 地址 `build_bank_send` 前必须先调用）。 |
| `http_fetch` | 用于 x402 付费内容的 HTTP GET/POST。 |
| `x402_prepare_typed_data` / `x402_build_payment` | 收到 402 响应后，构建并组装 x402 v2 EIP-3009 支付头。 |
| `a2a_send_message` | 向其他 **A2A 兼容 Agent** 发送消息并返回回复（见 [Agent-to-Agent (A2A)](a2a.zh-CN.md)）。 |

`v0.1` 对通过链 ID 与 signer 地址交叉校验的合法 payload 自动批准。**转账白名单** 在助手预检层（转账、授权、跨链桥，含 ERC-20/721 合约调用 —— **强制：无白名单则禁止一切转账**）与签名层（Cosmos `MsgSend` 与 EVM 原生转账；空列表在签名层表示不限制）分别生效；详见 [转账白名单](security-whitelist.zh-CN.md)。按工具限额、提示模式与 MCP elicitation 规划中。

## 存储密钥

签名密钥保存在 **操作系统凭据存储** —— macOS Keychain、Windows 凭据管理器或 Linux Secret Service（libsecret）—— 绝不通过命令行参数或客户端配置传递。导入一次即可：

```sh
# 交互式隐藏输入
./build/svpchain-mcp import --chain-id <chain-id>
Enter private key (hidden): ********
Stored key for svp1… (<chain-id>)

# …或管道传入（如从密码管理器）
printf '%s' <32-byte-hex> | ./build/svpchain-mcp import --chain-id <chain-id>
```

密钥以服务名 `svpchain-agent`、**链 ID** 为账户名存储；多条链可共存。规则为 **每条链一把密钥** —— 主网密钥用于测试网会扩大风险面，故 `import` 在检测到同一密钥已存于其他链时会警告。再次 `import` 会覆盖（密钥轮换）。

```sh
./build/svpchain-mcp list                           # 列出已存链 ID
./build/svpchain-mcp delete --chain-id <chain-id>   # 删除密钥
```

## 运行签名器

```sh
./build/svpchain-mcp --chain-id <chain-id>
```

| 参数 | 必填 | 说明 |
|------|------|------|
| `--chain-id` | 是 | 本 signer 绑定的链 ID。拒绝其他链的 payload/challenge，并选择同名存储密钥。 |
| `--evm-chain-id` | 否 | `sign_evm_transaction` 的 EIP-155 数字链 ID。默认从 `--chain-id` 解析 —— `svp_2517-1` 与 `svp-2517-1` 均为 `2517`。若 `--chain-id` 无法解析且未设此参数，则禁用 EVM 签名（Cosmos 签名不受影响）。 |

密钥从操作系统凭据存储读取。**没有 `--key-hex` 参数** —— 否则密钥会泄露到进程参数与 shell 历史。

### 无头环境回退

无 Secret Service 守护进程的 Linux 无头主机（CI、Docker）可设置 `SIGNER_KEY_HEX`；凭据存储无密钥时使用：

```sh
SIGNER_KEY_HEX=<32-byte-hex> ./build/svpchain-mcp --chain-id <chain-id>
```

服务将密钥来源（`OS credential store` 或 `SIGNER_KEY_HEX env`）写入 stderr —— 永不输出密钥本身。

> macOS 说明：读取 Keychain 需要 CGO 构建（`make build` 与 release 二进制默认启用）。`import` 后首次运行可能弹出一次性 Keychain 访问提示。

## MCP 客户端配置

在外部 MCP 客户端（如 Cursor）中通过 stdio 指向本二进制：

```json
{
  "mcpServers": {
    "svpchain-agent": {
      "command": "/absolute/path/to/build/svpchain-mcp",
      "args": ["--chain-id", "<chain-id>"]
    }
  }
}
```

配置中不含密钥 —— 服务在 `import` 后从凭据存储读取。对外服务名为 `svpchain-agent`。（GUI 内置助手在进程内运行同一 signer，无需单独 MCP 客户端。）
