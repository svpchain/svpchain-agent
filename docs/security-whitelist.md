# Transfer whitelist

**English** | [简体中文](security-whitelist.zh-CN.md) · [← README](../README.md)

Whitelist entries live in the GUI preferences file (`prefs.json` under the app config directory). Enforcement happens at **two layers** with **different empty-list semantics**:

## 1. Assistant pre-flight gate (agent layer)

Before the assistant forwards a transfer/approval `build_*` tool call to the remote MCP, the recipient/spender taken straight from the tool arguments is checked against the whitelist. **The whitelist is mandatory here: with no whitelist configured, every transfer/approval is refused** with a prompt to add one in the Security tab first — nothing is built, signed, or broadcast. When a whitelist exists, a non-whitelisted address is likewise rejected with `… is not on the whitelist …`. Because the address comes from the tool arguments (not raw calldata), this also covers **ERC-20/721 contract transfers**. Gated tools:

| Tool | Checked argument | Type |
|------|------------------|------|
| `build_bank_send` | `recipient` | Cosmos |
| `build_erc20_transfer`, `build_erc20_transfer_from` | `to` | EVM |
| `build_erc721_transfer_from`, `build_erc721_safe_transfer_from` | `to` | EVM |
| `build_bridge_deposit` | `recipient` (empty = self, allowed) | EVM |
| `build_erc20_approve`, `build_erc721_approve` | `spender` | EVM |
| `build_erc721_set_approval_for_all` | `operator` | EVM |

## 2. Signer fallback (sign layer)

As a second line of defense, the local signer also checks at sign time. Here an **empty whitelist means unrestricted** (backward compatible) — the mandatory-whitelist policy above is the GUI assistant's only. When a whitelist exists it checks:

- **Cosmos** — `cosmos.bank.v1beta1.MsgSend` recipient (`to_address`)
- **EVM** — native transfers only (`to` non-empty and `value` > 0)

At the signer layer, contract calls and zero-value EVM transactions are not decoded, so they are caught by the pre-flight gate above rather than here. The standalone `svpchain-mcp` signer reads the same `prefs.json`, but does not impose the assistant's mandatory-whitelist rule.
