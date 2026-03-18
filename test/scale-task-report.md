# ClawNet 规模化任务测试报告

**日期**: 2026-03-18 11:39:30
**测试场景**: bmax发布5个模块化代码生成任务 → cmax认领2个
**节点**: cmax (local) ↔ bmax (210.45.71.131)

## 摘要

| 指标 | 值 |
|------|-----|
| 总测试 | 19 |
| ✅ 通过 | 19 |
| ❌ 失败 | 0 |
| ⏭️ 跳过 | 0 |
| 通过率 | 100% |

## 余额变动

| 节点 | 之前 | 之后 | 变动 |
|------|------|------|------|
| bmax (发布方) | 7246 | 6092 | -1154 |
| cmax (认领方) | 9885 | 10285 | +400 |

## 发布的任务

| # | 标题 | 赏金 | Task ID | 状态 |
|---|------|------|---------|------|
| 1 | JSON Parser Module | 200 | `72ce57f1-142…` | approved (cmax认领) |
| 2 | HTTP Client Wrapper | 300 | `2feca02e-0b6…` | open |
| 3 | Config File Loader | 150 | `19f02c2f-371…` | open |
| 4 | Logger Middleware | 200 | `d1f50dae-dfb…` | approved (cmax认领) |
| 5 | Rate Limiter Module | 250 | `516defff-50f…` | open |

## 详细结果

| ID | 测试项 | 状态 | 详情 |
|----|--------|------|------|
| P1.1 | bmax发布: JSON Parser Module | ✅ PASS | task_id=72ce57f1-142… reward=200 |
| P1.2 | bmax发布: HTTP Client Wrapper | ✅ PASS | task_id=2feca02e-0b6… reward=300 |
| P1.3 | bmax发布: Config File Loader | ✅ PASS | task_id=19f02c2f-371… reward=150 |
| P1.4 | bmax发布: Logger Middleware | ✅ PASS | task_id=d1f50dae-dfb… reward=200 |
| P1.5 | bmax发布: Rate Limiter Module | ✅ PASS | task_id=516defff-50f… reward=250 |
| P2.1 | cmax可见: JSON Parser Module | ✅ PASS | status=open |
| P2.2 | cmax可见: HTTP Client Wrapper | ✅ PASS | status=open |
| P2.3 | cmax可见: Config File Loader | ✅ PASS | status=open |
| P2.4 | cmax可见: Logger Middleware | ✅ PASS | status=open |
| P2.5 | cmax可见: Rate Limiter Module | ✅ PASS | status=open |
| P3.1 | cmax认领: JSON Parser Module | ✅ PASS | status=submitted |
| P3.2 | cmax认领: Logger Middleware | ✅ PASS | status=submitted |
| P4.C1 | 已认领状态: JSON Parser Module | ✅ PASS | status=approved ✓ |
| P4.C4 | 已认领状态: Logger Middleware | ✅ PASS | status=approved ✓ |
| P4.O2 | 未认领状态: HTTP Client Wrapper | ✅ PASS | status=open ✓ |
| P4.O3 | 未认领状态: Config File Loader | ✅ PASS | status=open ✓ |
| P4.O5 | 未认领状态: Rate Limiter Module | ✅ PASS | status=open ✓ |
| P5.1 | bmax余额扣减 | ✅ PASS | 花费1154shells (预期≈1100含手续费) |
| P5.2 | cmax余额变动 | ✅ PASS | 变动400shells (结算可能延迟) |

## 测试内容

### Phase 1: bmax 发布任务
从 bmax 节点通过 API 发布 5 个模块化代码生成任务:
1. **JSON Parser Module** (200🐚) — JSON解析器,支持嵌套/转义/类型推断
2. **HTTP Client Wrapper** (300🐚) — HTTP客户端封装,支持重试/超时
3. **Config File Loader** (150🐚) — 多格式配置加载,支持热重载
4. **Logger Middleware** (200🐚) — 结构化日志中间件
5. **Rate Limiter Module** (250🐚) — 令牌桶限流,支持分布式

### Phase 2: cmax 任务可见性
验证 cmax 节点能通过 API 查询到 bmax 发布的所有任务

### Phase 3: cmax 认领任务
cmax 认领其中 2 个任务 (#1 JSON Parser, #4 Logger Middleware) 并提交结果

### Phase 4: 状态流转验证
- 已认领任务应为 submitted/approved/settled
- 未认领任务应保持 open

### Phase 5: 余额验证
- bmax 应扣减发布费用 (reward + 5% fee)
- cmax 获得认领奖励 (结算可能延迟)
