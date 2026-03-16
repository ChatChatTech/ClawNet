# ClawNet v0.9.8 — 任务系统深度测试报告

- **日期**: 2026-03-17 01:30:19
- **节点**: cmax (12D3KooWL2PeeDZChvnoERrfNkZa6JENyDiNWnbPwaNxNjETpmYh) / bmax (12D3KooWBRwPSjKRVwipL2VhHVFgusN7NCBfycKoYBfJRJBryvyT) / dmax (12D3KooWRF8yrRrYo8ddEecE7v2n5wioMPqvYP1CooCoSB3GudWW)
- **版本**: cmax=0.9.8 bmax=0.9.8 dmax=0.9.8

## 测试结果

| **PASS** | **FAIL** | **SKIP** | **TOTAL** | **Pass Rate** |
|----------|----------|----------|-----------|---------------|
| 110 | 0 | 0 | 110 | 100.0% |

## 初始/最终余额

| 节点 | 初始余额 | 最终余额 | 最终冻结 |
|------|---------|---------|---------|
| cmax | 4200 | 558 | 1800 |
| bmax | 5370 | 6540 | 0 |
| dmax | 4200 | 4618 | 0 |

## 详细结果

| ID | 测试项 | 状态 | 详情 |
|----|--------|------|------|
| P0.1 | Profile & Resume setup | ✅ PASS | cmax=4200, bmax=5370, dmax=4200 |
| T1.1 | 最小参数创建任务 | ✅ PASS | id=5807cc2b-2e0…, status=open |
| T1.2 | 完整参数创建任务 | ✅ PASS | id=a6357845-5fe…, reward=500 |
| T1.3 | 无标题拒绝 | ✅ PASS | HTTP 400 |
| T1.4 | 低奖励拒绝 (50<100) | ✅ PASS | HTTP 400 |
| T1.5 | 零奖励拒绝 | ✅ PASS | HTTP 400 |
| T1.6 | 负奖励拒绝 | ✅ PASS | HTTP 400 |
| T1.7 | 5% fee + 冻结验证 | ✅ PASS | diff=630 (fee=30+frozen=600) |
| T1.8 | 冻结余额验证 | ✅ PASS | frozen_diff=600 (100+500) |
| T1.9 | 定向任务创建 | ✅ PASS | id=77faaffb-ca9…, target=bmax |
| T1.10 | 逗号字符串标签 | ✅ PASS | id=1adf90a7-444… |
| T2.1 | 列出任务 (≥4) | ✅ PASS | 5 tasks |
| T2.2 | 状态筛选 (open) | ✅ PASS | 5 open tasks |
| T2.3 | 分页 (limit=2) | ✅ PASS | page1=2, page2=2 |
| T2.4 | 任务详情 | ✅ PASS | title=Minimal Task 1773682107 |
| T2.5 | 不存在任务 404 | ✅ PASS | HTTP 404 |
| T2.6 | 看板 my_published | ✅ PASS | pub=4, open=1 |
| T2.7 | Gossip 传播 (bmax) | ✅ PASS | bmax sees 28 tasks |
| T2.8 | Gossip 传播 (dmax) | ✅ PASS | dmax sees 5 tasks |
| T3.0 | Legacy任务创建 | ✅ PASS | id=e6f7de2e-9d2…, reward=200 |
| T3.1 | bmax竞标 | ✅ PASS | bid placed |
| T3.2 | 指派 → assigned | ✅ PASS | status=assigned |
| T3.3 | 状态确认 assigned | ✅ PASS | status=assigned |
| T3.4 | bmax提交 → submitted | ✅ PASS | status=submitted |
| T3.5 | 审批通过 → approved | ✅ PASS | status=approved |
| T3.6 | 奖励到账 (bmax +200) | ✅ PASS | 5370 → 5570 |
| T3.7 | 冻结释放验证 | ✅ PASS | frozen=900 |
| T3.8 | 最终状态 approved | ✅ PASS | status=approved |
| T4.0 | 拒绝测试任务创建 | ✅ PASS | id=179ca87c-d5c… |
| T4.2 | 拒绝 → rejected | ✅ PASS | status=rejected |
| T4.3 | 拒绝后奖励返还 (+150) | ✅ PASS | refund=150 |
| T4.4 | 最终状态 rejected | ✅ PASS | status=rejected |
| T5.1 | 取消open任务 | ✅ PASS | status=cancelled |
| T5.2 | 取消后余额 (fee已扣不退) | ✅ PASS | balance=3033 (fee=5 burnt) |
| T5.3 | 取消assigned任务 | ✅ PASS | status=cancelled |
| T5.4 | 非owner取消被拒 | ✅ PASS | HTTP 403 |
| T5.5 | 已完成任务不能取消 | ✅ PASS | HTTP 409 |
| T6.1 | 不能竞标自己任务 | ✅ PASS | HTTP 403 |
| T6.2 | 目标peer竞标成功 | ✅ PASS | bid_id=00926c11-bad0-49b8-85e4-e385ada26b92 |
| T6.3 | 非目标peer竞标被拒 | ✅ PASS | HTTP 403 |
| T6.4 | 多peer竞标 | ✅ PASS | 2 bids |
| T6.5 | bid_close延长 | ✅ PASS | bid_close=2026-03-16T18:09:03Z |
| T6.6 | 竞标message | ✅ PASS | msg=bmax bid |
| T7.1 | assign_to缺失 → 400 | ✅ PASS | HTTP 400 |
| T7.2 | 不能指派给自己 | ✅ PASS | HTTP 403 |
| T7.3 | 正常指派 | ✅ PASS | status=assigned |
| T7.4 | 重复指派被拒 | ✅ PASS | HTTP 409 |
| T8.1 | open状态不能submit | ✅ PASS | HTTP 409 |
| T8.2 | open状态不能approve | ✅ PASS | HTTP 409 |
| T8.3 | open状态不能reject | ✅ PASS | HTTP 409 |
| T9.0 | AH任务创建 | ✅ PASS | id=fb7e0363-2ce…, reward=300 |
| T9.1 | bmax竞标 | ✅ PASS | bid ok |
| T9.2 | bmax提交work | ✅ PASS | submission_id=5c5ca3ca-0ae4-48f8-93f0-7ecadc335949 |
| T9.3 | submissions列表 (1) | ✅ PASS | 1 submissions |
| T9.4 | pick winner → settled | ✅ PASS | status=settled |
| T9.5 | 单提交: winner得100% | ✅ PASS | earned=300 |
| T9.6 | 最终状态 settled | ✅ PASS | status=settled |
| T10.0 | 多人AH任务创建 | ✅ PASS | id=c445d798-e05…, reward=1000 |
| T10.1 | bmax+dmax竞标 | ✅ PASS | 2 bids placed |
| T10.2 | bmax+dmax提交work | ✅ PASS | 2 submissions |
| T10.3 | submissions数量=2 | ✅ PASS | 2 |
| T10.4 | pick bmax → settled | ✅ PASS | winner=12D3KooWBRwPSjKRVwipL2VhHVFgusN7NCBfycKoYBfJRJBryvyT |
| T10.5a | Winner 80% (bmax +800) | ✅ PASS | earned=800 |
| T10.5b | Consolation 20% (dmax +200) | ✅ PASS | earned=200 |
| T11.1 | 未竞标不能submit work | ✅ PASS | HTTP 403 |
| T11.2 | 空result被拒 | ✅ PASS | HTTP 400 |
| T11.3a | 首次work提交 | ✅ PASS | ok |
| T11.3b | 重复work提交被拒 | ✅ PASS | HTTP 409 |
| T12.1 | 非author不能pick | ✅ PASS | HTTP 403 |
| T12.2 | 缺submission_id → 400 | ✅ PASS | HTTP 400 |
| T12.3 | 错误submission_id → 404 | ✅ PASS | HTTP 404 |
| T12.4 | 正常pick成功 | ✅ PASS | status=settled |
| T12.5 | 已settled不能再pick | ✅ PASS | HTTP 409 |
| T13.1 | open → approve 被拒 | ✅ PASS | HTTP 409 |
| T13.2 | open → reject 被拒 | ✅ PASS | HTTP 409 |
| T13.3 | assigned → approve 被拒 | ✅ PASS | HTTP 409 |
| T13.4 | assigned → reject 被拒 | ✅ PASS | HTTP 409 |
| T13.5 | assigned → assign again 被拒 | ✅ PASS | HTTP 409 |
| T13.6 | submitted → cancel 被拒 | ✅ PASS | HTTP 409 |
| T13.7 | submitted → assign 被拒 | ✅ PASS | HTTP 409 |
| T13.8-cancel | approved → cancel 被拒 | ✅ PASS | HTTP 409 |
| T13.8-assign | approved → assign 被拒 | ✅ PASS | HTTP 400 |
| T13.8-approve | approved → approve 被拒 | ✅ PASS | HTTP 409 |
| T13.8-reject | approved → reject 被拒 | ✅ PASS | HTTP 409 |
| T14.1 | R2: dmax创建任务 | ✅ PASS | id=3644fbc1-a45… |
| T14.2 | R2: cmax竞标 | ✅ PASS | bid ok |
| T14.3 | R2: dmax指派→cmax | ✅ PASS | status=assigned |
| T14.4 | R2: cmax提交 | ✅ PASS | status=submitted |
| T14.5 | R2: dmax审批 | ✅ PASS | status=approved |
| T14.6 | R2: cmax奖励到账 (+250) | ✅ PASS | earned=250 |
| T15.1 | R3: bmax创建AH任务 | ✅ PASS | id=25a3e834-8e1…, reward=600 |
| T15.2a | R3: cmax竞标 | ✅ PASS | bid ok |
| T15.2b | R3: dmax竞标 | ✅ PASS | bid ok |
| T15.3 | R3: 双worker提交work | ✅ PASS | 2 submissions |
| T15.4 | R3: bmax pick dmax | ✅ PASS | settled |
| T15.5a | R3: winner 80% (dmax +480) | ✅ PASS | earned=480 |
| T15.5b | R3: consolation 20% (cmax +120) | ✅ PASS | earned=120 |
| T16.1 | Match agents for task | ✅ PASS | 1 matches, top=ClawNet Node |
| T16.2 | Match tasks for agent | ✅ PASS | 21 matched tasks |
| T17.1a | cmax余额 > 0 | ✅ PASS | balance=768 |
| T17.1b | bmax余额 > 0 | ✅ PASS | balance=6340 |
| T17.1c | dmax余额 > 0 | ✅ PASS | balance=4618 |
| T17.2 | Tier等级验证 | ✅ PASS | Lv3 信号小龙虾 |
| T17.3 | Credit transactions | ✅ PASS | 5 entries |
| T17.4 | 声望验证 | ✅ PASS | entries=1, top_score=57 |
| T18.1 | R4: 快速任务 #1 | ✅ PASS | approved |
| T18.2 | R4: 快速任务 #2 | ✅ PASS | approved |
| T18.3 | R4: 累计奖励 (+200) | ✅ PASS | earned=200 |
| T19.1 | 余额不足被拒 | ✅ PASS | HTTP 400 (reward=99999999) |
| T20.1 | 最终余额汇总 | ✅ PASS | cmax=558, bmax=6540, dmax=4618 |

---
*Generated by task_tests.sh*
