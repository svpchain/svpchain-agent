# Lendora Lending — Error Response Templates

Reference file for the `lendora-lending` skill, loaded on demand via `read_skill_reference`. Contains detailed error response templates by category — pick the matching row and adapt the numbers.

---

## Connection & Authentication Errors

| Error | Agent Response |
|-------|---------------|
| svpchain-remote unreachable | "Lendora 服务暂时无法连接，请稍后重试。" |
| Not authenticated (`auth_required` result) | NOT an error — the tool returned a successful `auth_required` result with handshake steps. Run `auth_challenge` → `sign_challenge` (svpchain-signer) → `auth_verify`, then RETRY the original tool. Do not show an error to the user unless the handshake itself fails. |
| Authentication failed / session expired | "会话已过期，请重新启动 svpchain-signer 认证。" |
| svpchain-signer not running | "本地签名服务未启动。请确认 svpchain-signer 已配置并运行。" |
| Signer has no key | "未找到钱包密钥。请运行 svpchain-signer 初始化流程导入或创建密钥。" |

---

## Data Query Errors

| Error | Agent Response |
|-------|---------------|
| RPC timeout | "链上数据暂时无法获取，请稍后重试。" |
| Rate limited (429) | "请求过于频繁，请等待 30 秒后重试。" |
| Tool not found (feature not yet deployed) | "该功能暂未上线，当前可提供数据查询和风险评估。" |
| Data format error / missing fields | "数据格式异常，无法给出可靠结果。请稍后重试。" |
| Address format invalid | "地址格式不正确。SVP Chain 地址应以 0x 开头，共 42 位字符。" |
| Session has no address & user didn't provide one | "无法识别你的钱包地址，请提供你的 SVP Chain 地址（0x 开头，42 位）。" |
| Asset not found / not supported | "Lendora 当前不支持该资产。支持的资产：[list from get_all_markets]。" |
| Oracle price stale (> threshold) | Include in risk findings: "⚠️ [asset] 预言机最近更新: X 分钟前（阈值 Y 分钟）" |

---

## Operation Errors (build_tx)

| Error | Agent Response |
|-------|---------------|
| Insufficient token balance | "你的 [asset] 余额为 X，不足以操作 Y，还差 Z。" |
| Insufficient Gas (SVP native) | "SVP 余额不足以支付 Gas 费。当前余额 X SVP，预估需要 Y SVP。请先获取 SVP。" |
| HF hard threshold (< 1.0) | "⛔ 该操作将导致 HF 降至 X.XX（< 1.00），进入清算区，已被阻止。建议先还款或追加抵押。" |
| HF soft threshold (1.0–1.2) | "⚠️ 操作后 HF 将降至 X.XX，接近清算风险区。确认继续吗？" |
| Exceeds single-tx limit | "单笔操作金额超过安全限额（限额 X）。请拆分为多笔较小操作。" |
| Rate limit (same address same op < 10s) | "操作过于频繁，请等待 10 秒后重试。" |
| Insufficient pool liquidity | "当前 [asset] 池可用流动性为 X，不足以满足 Y 的借款请求。最大可借: X。" |
| Market paused | "[asset] 市场当前已暂停 [supply/borrow] 操作。请关注官方公告。" |
| Allowance insufficient (auto-handled) | NOT an error. The build tool returns an `approval_required` object (no payload) with `tool=build_erc20_approve`, `token`=underlying, `spender`=cToken. Flow: `build_erc20_approve` → `sign_evm_transaction` → `broadcast_evm_tx`, then RE-CALL the original `lendora_build_*_tx` to get the payload. Present Approve + the action as two steps; never surface as an error. |

---

## Signing Errors

| Error | Agent Response |
|-------|---------------|
| User rejected signature | "签名已取消，操作未执行。如需继续请重新发起。" |
| Signer timeout / no response | "签名超时未完成，操作已取消。请检查 svpchain-signer 是否正常运行。" |
| Multi-tx interrupted (e.g. Approve signed, Supply rejected) | "Approve 交易已完成 (TX: 0x...)，但后续操作被取消。无资金损失。如需继续，请重新发起存款操作（无需再次授权）。" |

---

## Broadcast Errors

| Error | Agent Response |
|-------|---------------|
| Nonce conflict | "交易被拒绝（nonce 冲突），可能你同时在其他地方发起了交易。请稍后重试。" |
| Gas price insufficient / TX pending | "交易已提交但尚未确认（Gas 费偏低）。TX: 0x... 请等待或考虑加速。" |
| Contract revert | "交易上链失败：[revert reason]。TX: 0x... Gas 已消耗。链上状态可能已变化，建议重新查询后再操作。" |
| Broadcast timeout | "交易广播超时，状态不确定。TX: 0x... 请通过区块浏览器确认状态后再进行下一步操作。" |
