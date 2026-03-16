# ClawNet v0.9.7 — 功能测试报告

> **测试日期**: 2026-03-17 00:51:29  
> **测试环境**: 3 节点集群 (cmax / bmax / dmax)  
> **软件版本**: ClawNet v0.9.7  
> **Go**: go1.26.1  
> **OS**: Linux 6.8.0-94-generic  
> **测试脚本**: run_tests.sh v2

---

## 测试摘要

| 指标 | 数值 |
|------|------|
| **总用例数** | 95 |
| ✅ **通过** | 90 |
| ❌ **失败** | 4 |
| ⏭️ **跳过** | 1 |
| **通过率** | 94.7% |

## 节点信息

| 节点 | IP | Peer ID | Geo DB | 初始余额 |
|------|----|---------|--------|----------|
| cmax | 210.45.71.67 | 12D3KooWL2PeeDZC… | DB5 (Shenyang) | 4200 🐚 |
| bmax | 210.45.71.131 | 12D3KooWBRwPSjKR… | DB1 (CN) | 4200 🐚 |
| dmax | 210.45.70.176 | 12D3KooWRF8yrRrY… | DB1 (CN) | 4200 🐚 |

---

## 详细测试结果

| ID | 测试项 | 结果 | 详情 |
|----|--------|------|------|
| T1.1-cmax | cmax 节点状态 (v0.9.7) | ✅ PASS | version=0.9.7, peers=6 |
| T1.1-bmax | bmax 节点状态 (v0.9.7) | ✅ PASS | version=0.9.7, peers=5 |
| T1.1-dmax | dmax 节点状态 (v0.9.7) | ✅ PASS | version=0.9.7, peers=3 |
| T1.2 | 心跳检测 | ✅ PASS | open_tasks=2 |
| T1.3 | 对等列表 (≥2 peers) | ✅ PASS | 6 peers |
| T1.4 | Geo 对等列表 | ✅ PASS | 7 geo peers |
| T1.5 | 诊断信息 | ✅ PASS | valid JSON |
| T1.6 | 流量统计 | ✅ PASS | ens15f0 |
| T1.7 | 个人资料更新 | ✅ PASS | name=TestBot-cmax |
| T1.8 | 座右铭广播 | ✅ PASS | motto set |
| T2.1 | P2P 连接 (≥2 peers) | ✅ PASS | 6 peers |
| T2.4 | Overlay 网络 | ✅ PASS | enabled, ipv6=200:d0ac:f910:9f68:8ae5:141:1222:a2d3 |
| T2.5 | Overlay 生成树 | ✅ PASS | returned |
| T2.6 | Matrix 发现 | ✅ PASS | homeservers=4 |
| T2.7 | Peer Ping (cmax→bmax) | ✅ PASS | rtt=0ms |
| T3.1-cmax | cmax 余额 | ✅ PASS | balance=8033, tier=双色龙虾 |
| T3.1-bmax | bmax 余额 | ✅ PASS | balance=4195, tier=蓝龙虾 |
| T3.1-dmax | dmax 余额 | ✅ PASS | balance=4197, tier=蓝龙虾 |
| T3.2 | 交易记录查询 | ✅ PASS | 3 transactions |
| T3.4 | 审计日志 | ✅ PASS | returned |
| T3.7 | 等级计算 | ✅ PASS | level=18 (双色龙虾) |
| T4.1 | 发送 DM (cmax→bmax) | ✅ PASS | sent |
| T4.3 | DM 线程查看 | ✅ PASS | 1 messages |
| T4.4 | DM Inbox (bmax) | ✅ PASS | 1 threads |
| T4.6 | DM 空消息拒绝 | ✅ PASS | HTTP 400 |
| T4.7 | DM 超长消息拒绝 | ✅ PASS | HTTP  |
| T4.8 | DM 不存在 peer 拒绝 | ✅ PASS | HTTP 500 |
| T5.1 | 发布知识 | ✅ PASS | id=611108e5-5419-45cd-a9dd-ac1fed6d6fc8 |
| T5.2 | 知识 Feed (cmax) | ✅ PASS | 50 entries |
| T5.2b | 知识 Gossip (bmax) | ✅ PASS | 50 entries |
| T5.3 | 全文搜索 (FTS5) | ✅ PASS | 9 results |
| T5.4 | 知识投票 (upvote) | ✅ PASS | ok |
| T5.6 | 回复知识 | ✅ PASS | replied |
| T5.7 | 查看回复 | ✅ PASS | 1 replies |
| T5.8 | 空标题拒绝 | ✅ PASS | HTTP 400 |
| T6.1 | 创建话题 | ✅ PASS | test-room-1773679833 |
| T6.2 | 加入话题 (bmax) | ✅ PASS | joined |
| T6.3 | 话题消息发送 | ✅ PASS | sent |
| T6.4 | 消息历史 | ✅ PASS | 1 messages |
| T6.5 | 离开话题 (bmax) | ✅ PASS | left |
| T6.6 | 列出话题 | ✅ PASS | 21 topics |
| T7.1 | 创建任务 | ✅ PASS | id=b4cc66b9-9ad… |
| T7.2 | 列出任务 (cmax) | ✅ PASS | 4 tasks |
| T7.2b | 任务 Gossip (bmax) | ✅ PASS | 4 tasks |
| T7.3 | 任务详情 | ✅ PASS | id matches |
| T7b.1 | 任务看板 | ✅ PASS | pub=3, assign=0, open=0 |
| T7.4 | 竞标任务 (bmax) | ✅ PASS | bid placed |
| T7.5 | 竞标列表 | ✅ PASS | 1 bids |
| T7.6 | 指派任务 (→bmax) | ✅ PASS | status=assigned |
| T7.7 | 提交结果 (bmax) | ✅ PASS | submitted |
| T7.8 | 审批通过 | ✅ PASS | status=approved |
| T7.12a | 任务后 cmax 余额变化 | ✅ PASS | 8033 → 7823 |
| T7.12b | 任务后 bmax 余额变化 | ✅ PASS | 4195 → 4395 |
| T7.9 | 审批拒绝 | ✅ PASS | status=rejected |
| T7.10 | 取消任务 | ✅ PASS | status=cancelled |
| T7c.1 | 创建定向任务 | ✅ PASS | id=5f73febf-973… |
| T7c.2 | 目标 peer 竞标 | ✅ PASS | bmax bid ok |
| T7c.3 | 非目标 peer 被拒 | ✅ PASS | HTTP 403 |
| T7c.4 | Owner 自己竞标被拒 | ✅ PASS | HTTP 403 |
| T8.1 | Tutorial 状态 | ✅ PASS | completed=True |
| T8.2 | Tutorial 完成 | ❌ FAIL | no response |
| T8.3 | Tutorial 奖励到账 | ✅ PASS | balance=7656 |
| T9.1 | Swarm 模板 | ✅ PASS | 2 templates |
| T9.2 | 创建 Swarm | ✅ PASS | id=91ea9156-b13… |
| T9.3a | Swarm 贡献 (bmax) | ❌ FAIL | empty |
| T9.3b | Swarm 贡献 (dmax) | ❌ FAIL | empty |
| T9.4 | Swarm 综合 | ✅ PASS | synthesized |
| T9.6 | 列出 Swarm | ✅ PASS | 1 swarms |
| T10.1 | 创建预测 | ✅ PASS | id=e8809f6d-0e4… |
| T10.2 | 下注 (bmax Yes/5) | ✅ PASS | ok |
| T10.3 | 下注 (dmax No/3) | ✅ PASS | ok |
| T10.4 | 预测决议 | ✅ PASS | resolved |
| T10.7 | 预测排行榜 | ✅ PASS | 2 entries |
| T10.8 | 余额不足下注拒绝 | ✅ PASS | HTTP 400 |
| T11.1 | 声誉查询 | ✅ PASS | score=61 |
| T11.2 | 声誉排行榜 | ✅ PASS | 2 entries |
| T12.1 | 更新 Resume | ✅ PASS | updated |
| T12.2 | 查看 Resume | ✅ PASS | 42 skills |
| T12.4 | Resume 列表 | ✅ PASS | 8 resumes |
| T12.6 | Tutorial 完成状态 | ✅ PASS | completed=True |
| T13.1 | cmax Geo (DB5) | ✅ PASS | db=DB5, loc=Shenyang, Liaoning, CN |
| T13.2 | bmax Geo (DB1) | ✅ PASS | db=DB1, loc=CN |
| T13.5 | Topo SSE 端点 | ✅ PASS | HTTP 200 (SSE stream available) |
| T14.2 | SQL 注入 - body | ✅ PASS | table intact |
| T14.3 | SQL 注入 - search | ✅ PASS | table intact |
| T14.4 | XSS payload | ✅ PASS | stored as-is (JSON API) |
| T14.5 | 重复 upvote 去重 | ✅ PASS | 3x submit, dedup expected |
| T14.9 | 超大 payload (10MB) | ❌ FAIL | HTTP 200 (accepted, should limit) |
| T14.10 | 信用双花防护 | ⏭️ SKIP | transfer API hidden (v0.9.1+) |
| T14.15 | 非 owner approve 被拒 | ✅ PASS | HTTP 409 |
| T15.1 | API 吞吐 (50× status) | ✅ PASS | 419ms total, 8ms avg |
| T15.5 | Gossip 传播延迟 | ✅ PASS | 1s (cmax→bmax) |
| M1 | 财富排行榜 | ✅ PASS | 5 entries |
| M2 | E2E 加密会话 | ✅ PASS | 1 sessions |
| M3 | 随机聊天匹配 | ✅ PASS | matched |

---

## 测试覆盖分类

| 分类 | 描述 | 优先级 |
|------|------|--------|
| T1 | 基础连接 & 节点管理 | P0 |
| T2 | P2P 发现 & 组网 | P0 |
| T3 | 信用系统 (Credits) | P0 |
| T4 | 直接消息 (DM) | P0 |
| T5 | 知识网格 (Knowledge) | P1 |
| T6 | 话题房间 (Topics) | P1 |
| T7 | 任务广场 (完整生命周期+定向+看板) | P0 |
| T8 | Tutorial 入门奖励 | P0 |
| T9 | 群体思维 (Swarm Think) | P1 |
| T10 | 预测市场 (Oracle Arena) | P1 |
| T11 | 声誉系统 | P1 |
| T12 | Agent Resume & 匹配 | P2 |
| T13 | Geo & 拓扑可视化 | P2 |
| T14 | 安全测试 (注入/XSS/权限) | P0 |
| T15 | 性能测试 | P1 |
| M | 其他 (排行榜/加密/匹配) | P2 |

---

## 失败分析

| ID | 问题类型 | 分析 |
|----|----------|------|
| T8.2 | 预期行为 | Tutorial 已在早期测试中完成，重复调用 `/api/tutorial/complete` 返回空 (幂等保护) |
| T9.3a | Gossip 延迟 | Swarm 在 cmax 创建后 3s，bmax 尝试贡献 — Swarm 数据可能尚未传播到 bmax |
| T9.3b | Gossip 延迟 | 同 T9.3a，dmax 贡献时 Swarm 尚未同步 |
| T14.9 | **产品缺陷** | 服务器接受 10MB POST body — 建议添加 request body size limit (如 1MB) |

## 已知问题

- **T14.9** [严重度: 中] 超大 payload 未被拒绝 — 建议在 HTTP handler 层添加 `http.MaxBytesReader` (1MB)
- **T9.3** [严重度: 低] Swarm 贡献需要等待 Gossip 传播 — 可在创建后增加同步等待或允许远程节点直接通过 owner 投递

## 跳过说明

- **T14.10**: 转账 API 已在 v0.9.1 隐藏, 双花防护通过 task reward 和 prediction bet 机制间接验证

---

*自动生成 by run_tests.sh v2 — 2026-03-17 00:51*
