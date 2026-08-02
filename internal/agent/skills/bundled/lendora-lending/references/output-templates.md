# Lendora Lending — Output Templates

Reference file for the `lendora-lending` skill, loaded on demand via `read_skill_reference`. Contains response templates for each user intent — copy the structure, substitute live data.

---

## Market Scan — All Markets

```
📊 Lendora 市场概览

最高存款 APY: USDV 2.50%
最低借款 APY: USDC 4.20%

| 资产 | Supply APY | Borrow APY | Utilization | Liquidity |
|------|-----------|-----------|-------------|-----------|
| USDV | 2.50% | 5.10% | 43.00% | $680,000.00 |
| USDC | 2.10% | 4.20% | 38.00% | $510,000.00 |
| SVP  | 1.80% | 6.30% | 55.00% | $320,000.00 |

数据基于当前链上状态，利率随 utilization 实时变化。
```

---

## Market Scan — Single Asset

```
📊 USDC 市场详情

| 指标 | 值 |
|------|-----|
| Supply APY | 2.10% |
| Borrow APY | 4.20% |
| Total Supply | $1,050,000.00 |
| Total Borrow | $400,000.00 |
| Utilization | 38.00% |
| Collateral Factor | 75.00% |
| Reserve Factor | 10.00% |
| Oracle Price | $1.00 |

利率模型：
- Base Rate: 0.00%
- Kink: 80.00%（拐点前斜率 4.00%，拐点后 75.00%）

当前 utilization 38.00% < kink 80.00%，利率处于低斜率区间。
```

---

## Position Check

```
📊 你的 Lendora 仓位

Net Worth: $12,450.00
存款: $15,850.00 | 借款: $3,400.00
Net APY: 3.20% (Earn 2.50% − Pay 4.80%)
Health Factor: 2.40 🟢 Low
Borrow Limit Used: 41.50% ($3,400.00 / $8,200.00)

Supply:
| 资产 | 存入 | USD | APY | 作为抵押 |
|------|------|-----|-----|:--------:|
| USDV | 5,000.00 | $5,000.00 | 2.50% | ✅ |
| SVP  | 1,600.00 | $3,200.00 | 1.80% | ✅ |

Borrow:
| 资产 | 借入 | USD | APY | 可借余额 |
|------|------|-----|-----|---------|
| USDC | 3,400.00 | $3,400.00 | 4.20% | $800.00 |

✅ 仓位健康

安全操作空间：
- 最大安全可取: USDV 2,000.00 / SVP 800.00（保持 HF > 1.50）
- 最大安全可借: USDC 500.00 / USDV 480.00（保持 HF > 1.50）
- 清算价格: SVP 跌至 $1.12 时触发
```

---

## Risk Assessment (Medium Example)

```
⚠️ 风险评估 — 🟡 Medium

Health Factor: 1.72
Borrow Limit Used: 68.30%

发现问题：
1. ⚠️ USDC 池 utilization 92.00%（已超 kink 80.00%）— 借款利率可能骤升
2. ⚠️ SVP 预言机最近更新: 45 分钟前（阈值 60 分钟）— 接近过时

安全操作空间：
- 最大安全可取: USDV 800.00 / SVP 200.00（保持 HF > 1.50）
- 最大安全可借: USDC 150.00（保持 HF > 1.50）
- 清算价格: SVP 跌至 $1.45 时触发

建议：
- 关注 USDC 借款利率变化，当前处于高斜率区间
- 如需增加操作空间，可追加抵押或部分还款

Based on data at block #1,234,567
```

---

## What-if Simulation

```
📐 模拟结果：借入 1,000.00 USDC

| 指标 | 当前 | 操作后 | 变化 |
|------|------|------|------|
| Health Factor | 2.40 | 1.65 | ↓ 0.75 |
| Borrow Limit Used | 41.50% | 53.70% | ↑ 12.20% |
| 清算价格 (SVP) | $1.12 | $1.48 | ↑ $0.36 |
| 风险等级 | 🟢 Low | 🟡 Medium | ↑ |

⚠️ 可以执行但需注意：HF 降至 1.65，SVP 跌 26% 即触发清算。
最大安全借入量: 650.00 USDC（保持 HF > 1.50）

Based on data at block #1,234,567
```

---

## Balance Check

```
💰 你的钱包余额（Lendora 支持资产）

| 资产 | 余额 | USD |
|------|------|-----|
| SVP (Gas) | 12.50 | $25.00 |
| USDV | 1,250.00 | $1,250.00 |
| USDC | 820.00 | $820.00 |

Gas 状态: ✅ 充足（当前 12.50 SVP，单笔交易约需 0.01 SVP）
```

---

## Protocol Overview

```
📊 Lendora Protocol 总览

| 指标 | 值 |
|------|-----|
| Total Market Size | $2,450,000.00 |
| Total Available | $1,560,000.00 |
| Total Borrows | $890,000.00 |
| Markets | 3 |

各市场占比：
| 资产 | Supply 占比 | Borrow 占比 |
|------|:---:|:---:|
| USDV | 48.98% | 58.43% |
| USDC | 42.86% | 33.71% |
| SVP  | 8.16% | 7.87% |
```

---

## Execution — Confirmation Stage

```
📋 Supply 1,000.00 USDC — 模拟结果

| 指标 | 当前 | 操作后 | 变化 |
|------|------|------|------|
| Health Factor | 2.40 | 2.85 | ↑ 0.45 ✅ |
| Borrow Limit | $8,200.00 | $9,050.00 | ↑ $850.00 |
| 清算价格 (SVP) | $1.12 | $0.94 | ↓ $0.18（更安全） |

需要操作：
1. Approve USDC（首次存入需授权）
2. Supply 1,000.00 USDC

确认执行吗？（需要钱包签名）

Based on data at block #1,234,567
```

---

## Execution — Completion

```
✅ 操作完成！

| 步骤 | TX Hash | 状态 |
|------|---------|:----:|
| Approve USDC | 0xabc...123 | ✅ |
| Supply 1,000.00 USDC | 0xdef...456 | ✅ |

操作后仓位：
- Health Factor: 2.85
- Borrow Limit: $9,050.00
- 新增 Supply: 1,000.00 USDC (APY 2.10%)
```
