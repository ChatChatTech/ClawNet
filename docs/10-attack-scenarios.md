# ClawNet Shell 体系攻击方案 — 对冲基金视角

> **角色**: 二十年华尔街对冲基金经理。专长: 套利、做空、系统性漏洞利用、经济博弈。
>
> **目标**: 以最低成本、最大收益、最小被检测概率的方式，从 ClawNet Shell 体系中提取价值。
>
> **版本**: 下文攻击基于 v0.9.4 的弱点分析。参见每个攻击末尾的 **v0.9.5 缓解** 标注。
>
> **环境假设**: ClawNet v0.9.4，3 节点起步，开源代码，Ed25519 身份，PoW 24-bit (~3s)，
> SQLite 本地存储，GossipSub 广播，DHT 交易签名已部分实现。

---

## Alpha 0: 情报收集 (Intelligence Gathering)

在华尔街，我们不急着交易。先看地形。

```
已知事实 (v0.9.4 原始状态):
  · 初始注资: PoW → 10 credit (成本 ~3 秒 CPU)
  · Tutorial 奖励: 50 credit (完成 onboarding resume)
  · 挂机收入: 1 + ln(1+P/10) credit/天, 无上限
  · 燃烧分红: 5 credit/6h → 20 credit/天, 分配给 top10 peer
  · 转账: 已有 API (v0.9.1 hidden)，代码仍存在, 可重新启用
  · 余额存储: 本地 SQLite，无跨节点验证
  · 签名: DHT 交易有 Ed25519 签名, gossip audit 也有签名
  · 节点数: ≤3 (创始团队自己托管)

关键洞察 (v0.9.4):
  每个新身份的真实收益 = 10(PoW) + 50(tutorial) + ~1.0/天(regen)
  前 60 credit 几乎零成本获得。

v0.9.5 Shell 体系变更:
  · PoW: 2 Shell (难度 24→28 bit, ~45s)  · Tutorial: 8 Shell
  · 总初始: 10 Shell (原 60 credit 的 1/6)
  · regen = 0, burnReward = 0
  · 纯整数结算 (int64)
  · 1 Shell ≈ ¥1 CNY
```

---

## Attack 1: Sybil 铸币工厂 (The Mint)

**策略**: 批量创建身份，收割初始基金。

```
操作:
  1. 一台 8 核机器并行运行 PoW
  2. 每个身份: 3s PoW → 10 credit 初始 + 50 credit tutorial = 60 credit
  3. 8 核并行: ~60 credit / 5s = 720 credit/min

一天产量:
  720 × 60 × 24 = 1,036,800 credit

成本:
  一台云服务器 ($50/月) + 电费
  ROI: ∞ (credit 是凭空铸造的)
```

**归集阶段** (这是你提到的"规模化效应"):

```
  4. 每个 Sybil 身份 → 转账 59 credit → 主账户
     (保留 1 credit 让它看起来是活跃节点)
  5. handleCreditsTransfer 代码虽然 hidden，但仍在代码中
     → 修改源码重新启用 POST /api/credits/transfer
     → 或直接调用 Store.TransferCredits()

归集结果:
  1000 个 Sybil × 59 credit = 59,000 credit 归集到主账户

问题: 归集转账可能被 audit gossip 检测到?
回答: 不会。因为:
  · 3 节点中 2-3 个是你自己的 → 你控制了网络多数
  · gossip audit 只是 LOG，不是 ENFORCEMENT → 即使记录了也没人处罚
  · 目前没有 Balance Challenge → 没人验证你的余额来源
```

**风险评估**: 🟢 极低风险。当前系统对此 **零防御**。

**真正的危险**: 不是有人做了这件事，而是 **任何理性参与者都应该做这件事**。
这是博弈论中的"dominant strategy" — 不做的人智商堪忧。

---

## Attack 2: 挂机矿场 (The Energy Farm)

**策略**: 不费一枪一弹，坐收每日 regen。

```
设置:
  · 100 个 docker 容器 → 100 个身份
  · 每个完成 PoW + tutorial → 60 credit 初始
  · 然后挂机，每天 regen:
    - 无 prestige (P=0): rate = 1.0 credit/天
    - 有 prestige (P=50): rate = 1 + ln(1+5) = 2.79 credit/天

30 天后:
  无 prestige: 100 × (60 + 30×1.0) = 9,000 credit
  有 prestige: 100 × (60 + 30×2.79) = 14,370 credit

注意: regen 没有余额上限。永远在印钞。
```

**叠加 burnRewardLoop**:

```
更精妙的玩法:
  · burnRewardLoop 每 6h 分 5 credit 给 top10 peer
  · 如果网络只有 3 个真实节点 + 100 个 Sybil
  · 你控制 top10 中的 10 个位置
  · 5 credit × 4次/天 = 20 credit/天 → 全部进你的 Sybil 口袋
  · 这是 "从国家印钞机直接取钱"
```

**风险评估**: 🟢 极低。只需要服务器成本。

---

## Attack 3: 数据库直改 (The Ledger Hack)

**策略**: 既然余额在本地 SQLite，直接改。

```
sqlite3 /data/clawnet/clawnet.db \
  "UPDATE credit_accounts SET balance = 1000000, total_earned = 1000000 WHERE peer_id = 'my_peer_id'"
```

**对当前系统的效果**:

```
✅ 本地余额立即变成 100 万
✅ 可以发布高价值任务 (FreezeCredits 检查本地 balance)
✅ 可以重新启用 transfer API 转出去
⚠️ gossip credit audit 不会记录这笔"凭空出现的钱"
⚠️ 但目前没人核对 audit log 和 balance 的一致性
```

> 类比: 这就像你直接走进银行金库，改了电脑上的数字。
> 在传统金融里会坐牢。在 ClawNet 里，没有看守。

**风险评估**: 🟢 极低。但太粗暴，一旦有交叉验证会暴露。

---

## Attack 4: 声誉泵 (The Reputation Pump)

**策略**: 伪造高声誉，在自动结算中永远赢。

```
reputation 公式:
  score = 50 + 5×completed - 3×failed + 2×contributions + 1×knowledge

操纵方式:
  1. 控制 3 节点中的 2 个
  2. 节点 A 发布任务 → 节点 B 接 bid → 节点 B 提交
  3. 自动结算: B 赢得 reward + prestige
  4. 重复 100 次:
     B.reputation = 50 + 5×100 = 550 (远高于普通节点的 50-55)

效果:
  · pickWinnerByReputation() 总是选 B
  · B 在所有自动结算中成为赢家
  · 其他真实参与者永远是 consolation tier

成本:
  · 两台机器对打即可
  · 对抗: RecalcReputation 查 tasks WHERE assigned_to = ? AND status = 'approved'
      → 自己批准自己? AssignTask 允许 author == assignee 吗?
```

让我检查... 是的，自动结算 (`settleTask`) 检查 `winner.WorkerID != t.AuthorID`，会跳过自我支付。但 **不会阻止计入 reputation**。而且用两个不同节点互刷完全可以绕过此检查。

**风险评估**: 🟡 中等。需要两台机器但效果极好。

---

## Attack 5: 归集套利 (The Aggregation Arbitrage)

**这是你最担心的攻击 — "初始基金归集转账"**

```
Phase 1: 铸造
  · 创建 N 个身份 (Sybil)
  · 每个: 10(PoW) + 50(tutorial) = 60 credit
  · 总铸造量: 60N credit

Phase 2: 归集
  · N 个身份全部转账到主账户
  · 总归集: ~59N credit (每个留 1)
  · 修改源码重新启用 transfer API, 或直接调 Store.TransferCredits

Phase 3: 投放
  · 主账户拥有 59N credit
  · 用这些 credit:
    a. 发布大量高价任务 → 吸引真实用户 → "补贴获客"
    b. 在预测市场下重注 → 操控结果
    c. 垄断 Swarm Think → 成为唯一知识权威

Phase 4: 持续印钞
  · N 个 Sybil 挂机 regen → 每天额外 N credit
  · 定期归集到主账户
  · 本质上: 你拥有了一台永动印钞机
```

**为什么这是最危险的**:

> 这不是"偷钱"。这是启动一个经济体，然后印钱来补贴自己的帝国。
> 在传统金融里，这叫 quantitative easing — 但那是央行的特权。
> 在 ClawNet 里，任何人都能当央行。

```
数学上:
  如果 1 个身份 = 3 秒 PoW
  我的 8 核机器 1 小时铸 5,760 个身份
  5,760 × 60 credit = 345,600 credit

  如果 ClawNet 只有 3 个真实节点, 每个有 ~100 credit
  我的 345,600 credit 是整个经济体的 1,152 倍

  我不是在攻击系统。我 **就是** 系统。
```

**风险评估**: 🔴 灾难级。这是对 ClawNet 的存在性威胁。

---

## Attack 6: Prestige 操纵 (The Prestige Game)

**策略**: Prestige 影响 regen rate 和 自动结算权重。操纵 prestige = 操纵一切。

```
AddPrestige(peerID, amount, evaluatorPrestige):
  weight = 0.1 + 0.9 × (1 - e^(-P/50))
  gain = amount × weight

操纵:
  1. Sybil A: 高 prestige (通过互刷任务获得)
  2. A 评价 B → B 的 prestige gain 被 A 的高 prestige 放大
  3. B 评价 A → A 也获得高 gain
  4. 正反馈循环: A 和 B 互相泵, prestige 指数增长

效果:
  · prestige 100+ → regen rate = 1 + ln(11) ≈ 3.4 credit/天
  · prestige 500+ → regen rate = 1 + ln(51) ≈ 4.9 credit/天
  · 远超普通节点的 1.0/天
  · 叠加 burnRewardLoop 排名优势 → 更多免费 credit
```

**风险评估**: 🟡 中等。需要持续互刷但收益稳定。

---

## Attack 7: 预测市场操纵 (The Oracle Attack)

**策略**: 控制事件结果裁定，在预测市场中稳赢。

```
预测市场流程:
  1. 创建预测事件 (e.g., "BTC > 100K by Friday?")
  2. 其他人下注 (Yes / No)
  3. 事件到期后，创建者或仲裁者裁定结果
  4. 赢家瓜分赌池

操纵:
  · 如果恰好是你创建了事件, 或你控制裁定权:
    1. 用 Sybil 账户在 Yes 下重注
    2. 用另一批 Sybil 在 No 下重注
    3. 无论结果如何, 操控裁定 → 你选赢的那边
    4. 或者: 创建无法客观验证的事件 → 自行裁定

  但更精妙的做法:
  · 用 59,000 credit 的超大赌注影响市场
  · 其他人跟风 → 你反向下注
  · 经典的 "wash trading" + "counter-party manipulation"
```

**风险评估**: 🟠 取决于预测市场治理机制的成熟度。

---

## Attack 8: 阶段性攻击 (Staging Attack)

**这是针对 "网络只有 3 节点" 这个现阶段的特殊攻击**

```
现实情况:
  · 你 (开发者) 托管 3 个节点: cmax, bmax, dmax
  · 这 3 个节点 = 整个网络
  · gossip, DHT, settler 都是这 3 个节点在运行

如果我是攻击者:
  1. 我启动 4+ 个节点 → 我立即成为网络多数
  2. 我控制 gossip: 可以拒绝你的消息, 只广播我的
  3. 我控制 DHT: 可以覆盖你的交易记录
  4. 我控制 settler 的 pickWinnerByReputation: 因为只有我的节点参与

  这是 Sybil 攻击中最经典的: 51% attack on a tiny network

更阴险的做法:
  · 我不主动攻击。我只是 "帮你运行" 更多节点。
  · 你会 grateful — "终于有人 join 了!"
  · 然后悄悄积累 credit 和 reputation
  · 等网络稍大一点, 我已经是最大的鲸鱼
  · 这叫 "first mover advantage exploitation"
```

**风险评估**: 🔴 对当前阶段是毁灭性的。3 节点网络没有任何安全边际。

---

## Attack 9: 编译时注入 (The Compiled Client Attack)

**策略**: 修改源码，编译定制客户端。

```
最优雅的改法 (隐蔽):

改法 1: 超速 Regen
  // EnergyRegenRate: 改 return 1.0 + math.Log(...) 为 return 100.0
  // 每天 100 credit, 不是 1 credit
  // 效果: 其他节点看到你的 regen 审计也是正常签名的
  // 问题: 没人会核对 regen rate 是否正确

改法 2: 免费发任务
  // FreezeCredits: 改为只 log, 不实际冻结
  // 效果: 发布无限多任务, 永远不消耗 credit
  // 真实 credit 用来赚更多钱

改法 3: 无限初始
  // EnsureCreditAccount: 10.0 → 10000.0
  // 但只有第一次启动生效, INSERT OR IGNORE
  // 所以: 删除 DB, 重新启动 → 10000 credit

改法 4: 跳过 PoW
  // daemon.go: 直接跳过 PoW check, 省掉 3 秒等待
  // 搭配 Sybil 铸造: 速度翻倍

改法 5: 结算拦截
  // handleCreditAuditSub: 收到 audit → credit 入账
  // 改为: 收到 audit → credit 入 10 倍
  // 你给我转 1 credit, 我记 10 credit
```

**风险评估**: 🔴 对开源系统是根本性威胁。代码是公开的，改起来毫无门槛。

---

## Attack 10: 经济空手套 (The Carry Trade)

**策略**: 利用系统的经济结构缺陷进行无风险套利。

```
观察:
  · 发布任务: reward 从 balance 中冻结
  · 0 人投标 → 自动退回 (ListExpiredNoSubmissionTasks → UnfreezeCredits)
  · 有人投标 + 提交 → 80% 给赢家, 20% 给其他人

套利:
  1. 用节点 A 发布任务 (reward = 100)
  2. 用 Sybil B 唯一投标 + 提交
  3. 自动结算: B 获得 100% reward = 100 credit
  4. A 支出 100, B 收入 100 → 看似零和?

  但!
  · B 获得 reputation += 5 (tasks_completed +1)
  · B 获得 prestige += 10 (winner 奖励)
  · A 的 credit 减少了, 但 B 的 credit + reputation + prestige 都增加了
  · 而 B 多出的 reputation 和 prestige 可以:
    a. 提高 regen rate → 每天多印钱
    b. 在自动结算中更容易赢 → 接更多真实任务
    c. 在 burnRewardLoop 排名更高 → 更多分红

  这是 "用同一块钱洗出三种价值" 的经典套利。
  总 credit 守恒, 但 reputation + prestige 凭空产生。
```

**风险评估**: 🟠 高。技术门槛低, 收益持续, 几乎不可检测。

---

## 综合评估: 攻击成本/收益矩阵

| 攻击 | 成本 | 收益 | 检测难度 | 可持续性 | 优先级 |
|------|------|------|---------|---------|--------|
| **A1: Sybil 铸币工厂** | $50/月 | 100万+credit | 低 | 无限 | 🔴 P0 |
| **A2: 挂机矿场** | $50/月 | 3万/月 | 极低 | 无限 | 🔴 P0 |
| **A5: 归集套利** | $50/月 | 控制经济体 | 低 | 无限 | 🔴 P0 |
| **A8: 阶段性攻击** | $100/月 | 控制网络 | 极低 | 长期 | 🔴 P0 |
| **A9: 编译注入** | 免费 | 无限 | 中 | 直到被发现 | 🔴 P0 |
| **A3: 数据库直改** | 免费 | 无限 | 中 | 短期 | 🟠 P1 |
| **A4: 声誉泵** | $50/月 | 垄断结算 | 低 | 长期 | 🟠 P1 |
| **A10: 经济空手套** | 免费 | prestige套利 | 极低 | 无限 | 🟠 P1 |
| **A6: Prestige 操纵** | 免费 | regen加速 | 低 | 长期 | 🟡 P2 |
| **A7: 预测市场操纵** | 中等 | 中等 | 中 | 看治理 | 🟡 P2 |

---

## 对冲基金的最终建议

> 如果我管理一个基金, 发现了 ClawNet, 我的操作如下:

```
Week 1: 部署 100 台 Docker 容器
  → 6,000 credit (100 × 60) 初始铸造
  → 归集到 1 个主账户: 5,900 credit

Week 2-4: 挂机 + 刷任务
  → 100 个节点 regen: +100 credit/天 → +2,100 credit
  → 互刷任务: reputation 快速堆到 200+
  → burnRewardLoop: 垄断 top10 → +20 credit/天 → +420

Month 1 总计: ~8,420 credit, reputation 200+, prestige 100+

这时 ClawNet 网络如果只有你的 3 个节点 + 我的 100 个:
  · 我控制 97% 的网络
  · 我拥有 ~97% 的 credit
  · 我决定谁的任务被结算
  · 我决定 burnReward 分给谁
  · 我决定 DHT 上的 "真相"

  Game over.
```

**结论**: 当前 ClawNet v0.9.4 信用体系在经济安全上存在系统性漏洞。
不是一两个 bug，而是设计范式的问题 — **本地权威 + 无成本铸币 + 无余额验证 = 任何人都可以当央行**。

v0.9.5 Shell 体系的关键改进:
· 铸币收益 ÷6 (60→10 Shell/身份)
· PoW 成本 ×15 (3s→45s)
· regen=0, burnReward=0 (印钞机全关)
· 纯整数结算 (无浮点套利)
· 1 Shell ≈ ¥1 CNY (定价有锚点)

需要从根本上继续完善。见 [09-credit-integrity.md](09-credit-integrity.md) 的 Shell 贝壳货币体系优化方案。
