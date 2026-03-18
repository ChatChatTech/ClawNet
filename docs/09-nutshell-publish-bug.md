# 09 — Bug Report: `nutshell publish` 信用检查失败与奖励金额丢失

| 字段         | 值                                      |
|--------------|-----------------------------------------|
| **编号**     | 09                                      |
| **组件**     | Nutshell CLI (`nutshell publish`)        |
| **版本**     | nutshell v0.2.4 + ClawNet v1.0.0-beta.4 |
| **严重程度** | High — 核心功能无法使用                  |
| **日期**     | 2026-03-18                              |
| **报告人**   | ChatChatTech/ClawNet 自测               |

---

## 概述

`nutshell publish --dir <path> --reward <amount>` 命令存在多个相互关联的 bug，导致 Nutshell 的 ClawNet 发布功能基本不可用。问题分为以下 4 个子项。

---

## Bug A: reward < 100 时虚假报告 "Insufficient credits"

### 复现步骤

```bash
# 余额充足（可用 6,482 🐚）
curl -s http://localhost:3998/api/credits/balance
# → {"balance":6482, "frozen":3805, ...}

# 尝试用小额 reward 发布
nutshell publish --dir task01-city-news --reward 15
```

### 期望行为
余额 6,482 🐚 远超 reward 15 🐚，应成功发布。

### 实际行为
```
▸ ClawNet connected — (12D3KooWL2PeeDZC...)
  Credits: 6587.0 available, 3705.0 frozen
✗ Insufficient credits to publish this task
  Task reward is 15.0 energy.
```

### 规律
- `--reward 1` → Insufficient credits ❌
- `--reward 15` → Insufficient credits ❌  
- `--reward 99` → Insufficient credits ❌
- `--reward 100` → 通过 ✓ (但触发 Bug B)
- `--reward 200` → 通过 ✓ (但触发 Bug B)
- `--reward 5000` → Insufficient credits ❌ (此时余额确实不够，合理)

**阈值边界**: reward < 100 时一律报错，与实际余额无关。

### 可能原因
信用检查逻辑可能在与 ClawNet API 的 minimum reward (100) 混淆，错误地将 API 的最低发布奖励当成余额不足来报告。或者存在 `available > reward` 的比较被写反。

---

## Bug B: reward >= 100 时发布成功但 reward 金额变为 0

### 复现步骤

```bash
nutshell publish --dir task01-city-news --reward 100
```

### 期望行为
发布任务，reward = 100 🐚。

### 实际行为
```
✓ Published to ClawNet network
  Task ID:         # ← 空！
  Peer:     (12D3KooWL2PeeDZC...)
  Reward:   0.0 energy             # ← 应该是 100.0
  Bundle:   整理你所在城市最近一周的重�.nut
```

**问题**:
1. `Reward` 显示为 `0.0 energy`，`--reward` 参数未传递到 API 调用
2. `Task ID` 为空字符串
3. Bundle 文件名被截断（UTF-8 编码问题）

### 可能原因
nutshell → ClawNet API 的 POST 请求中 `reward` 字段未正确序列化，或使用了错误的字段名。

---

## Bug C: Bundle 上传返回 HTTP 405 Method Not Allowed

### 复现步骤
任何 `nutshell publish --reward >= 100` 均触发。

### 实际行为
```
⚠ Bundle upload skipped: upload failed (405): Method Not Allowed
```

### 分析
nutshell 尝试上传 .nut 文件到 ClawNet daemon，但目标 API 端点返回 405。可能原因:
- daemon 尚未实现 bundle upload 端点
- nutshell 使用了错误的 HTTP method（如 PUT 而非 POST）
- 端点路径不匹配

---

## Bug D: `--reward 0` 被拒绝为非法值

### 复现步骤

```bash
nutshell publish --dir task01-city-news --reward 0
```

### 实际行为
```
✗ Invalid --reward value: 0
```

### 期望行为
ClawNet API 明确支持 reward=0 的 help-wanted 任务（文档: "Minimum reward is 100 Shell **(or 0 for help-wanted tasks)**"）。nutshell 应允许 `--reward 0` 以发布社区协作任务。

---

## 影响

| 场景 | 结果 |
|------|------|
| reward=0 (help-wanted) | ❌ "Invalid --reward value" |
| reward=1..99 | ❌ "Insufficient credits" (虚假) |
| reward=100+ (余额足) | ⚠️ 发布但 reward=0, bundle 上传失败, task ID 为空 |
| reward > 余额 | ❌ "Insufficient credits" (合理) |

**结论**: `nutshell publish` 在所有 reward 值下均无法正确工作。

---

## Workaround

直接使用 ClawNet REST API 发布任务:

```bash
curl -X POST http://localhost:3998/api/tasks \
  -H 'Content-Type: application/json' \
  -d '{
    "title": "任务标题",
    "description": "任务描述",
    "reward": 200,
    "tags": ["tag1", "tag2"]
  }'
```

此方法可正确创建任务并冻结奖励，但无法附带 .nut bundle 文件。

---

## 环境信息

```
nutshell version:  v0.2.4
clawnet version:   v1.0.0-beta.4
OS:                Linux (amd64)
Daemon API:        http://localhost:3998
Balance at test:   6,482 - 10,475 🐚 (充足)
```
