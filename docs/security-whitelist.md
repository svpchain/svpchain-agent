# Transfer whitelist

**English** | [简体中文](security-whitelist.zh-CN.md) · [← README](../README.md)

Whitelist entries live in the GUI preferences file (`prefs.json` under the app config directory). Transfer enforcement
uses the complete persisted list plus the built-in protocol recipients. It happens at two layers.

## 1. Assistant pre-flight gate (agent layer)

Before the assistant forwards a transfer `build_*` tool call to the remote MCP, the recipient taken straight from the
tool arguments is checked against the effective whitelist. A non-whitelisted address is rejected with
`… is not on the whitelist …`. Because the address comes from the tool arguments (not raw calldata), this also covers
**ERC-20/721 contract transfers**. Gated tools:

| Tool                                                            | Checked argument                    | Type   |
|-----------------------------------------------------------------|-------------------------------------|--------|
| `build_bank_send`                                               | `recipient`                         | Cosmos |
| `build_erc20_transfer`, `build_erc20_transfer_from`             | `to`                                | EVM    |
| `build_erc721_transfer_from`, `build_erc721_safe_transfer_from` | `to`                                | EVM    |
| `build_bridge_deposit`                                          | `recipient` (empty = self, allowed) | EVM    |

`approve`, allowance, and operator calls are deliberately not recipient-whitelisted. They still require the local
signing confirmation.

## 2. Signer fallback (sign layer)

As a second line of defense, the local signer checks the saved whitelist at sign time whenever the user has saved at
least one entry. An empty saved list remains unrestricted for standalone signer compatibility; the GUI assistant's
pre-flight gate still uses its effective whitelist.

- **Cosmos** — `cosmos.bank.v1beta1.MsgSend` recipient (`to_address`)
- **EVM** — native transfers and standard ERC-20/721/1155 transfer recipients

Approvals and unknown zero-value contract calls are not transfer-whitelisted. The prompt's alias list is advisory and
omits saved entries without aliases; the build/sign gates, not the model, decide the final whitelist result.
