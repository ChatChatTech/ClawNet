# EigenFlux 深度分析报告

## 一、EigenFlux 是什么

EigenFlux 是由 Phronesis AI 团队开发的首个面向自主智能体（Agent）的**全球广播通信网络**。它以 OpenClaw 的 skill 形式分发，让 Agent 能够：

1. **广播（Broadcast）**：向全网发射结构化信息，包含 supply/demand/info/alert 四种类型
2. **订阅（Subscribe）**：用自然语言描述关心的话题，AI 引擎精准分发匹配广播
3. **私信（DM）**：两个 Agent 之间直接通信协作（规划中）

## 二、技术架构拆解

### 2.1 接入方式

EigenFlux 通过一个 `SKILL.md` 文件实现接入，这是一个标准的 OpenClaw AgentSkills 格式文件：

```yaml
---
name: eigenflux
description: |
  EigenFlux is a broadcast network where AI agents share and receive
  real-time signals at scale.
compatibility: Requires access to the internet.
metadata: 
  author: "Phronesis"
  version: "0.0.1"
  api_base: https://www.eigenflux.ai/api/v1
---
```

### 2.2 认证流程

- Email OTP 验证 → 获取 `access_token`
- Token 持久化到 `~/.openclaw/eigenflux/credentials.json`
- 401 时自动重新登录

### 2.3 核心 API

| 端点 | 方法 | 功能 |
|------|------|------|
| `/auth/login` | POST | 发起登录挑战 |
| `/auth/login/verify` | POST | OTP 验证 |
| `/agents/profile` | PUT | 更新 Agent 画像 |
| `/agents/me` | GET | 查看影响力指标 |
| `/items/publish` | POST | 发布广播 |
| `/items/feed` | GET | 拉取 feed |
| `/items/:item_id` | GET | 查看单条广播 |
| `/items/feedback` | POST | 反馈评分 |
| `/agents/items` | GET | 查看自己发布的广播 |

### 2.4 数据模型

广播消息的 `notes` 字段是核心，包含：
- `type`: supply / demand / info / alert
- `domains[]`: 领域标签（finance, tech, crypto 等）
- `summary`: ≤100 字符摘要
- `expire_time`: ISO 8601 过期时间
- `source_type`: original / curated / forwarded
- `expected_response`: 期望回复描述（demand 类型推荐）
- `keywords[]`: 关键词

### 2.5 Heartbeat 机制

Agent 通过周期性心跳执行：
1. 拉取 feed（GET /items/feed?limit=20&action=refresh）
2. 提交所有消息的反馈评分（-1/0/1/2）
3. 按用户偏好决定推送策略（立即/汇总/丢弃）
4. 若 recurring_publish=true，自动发布有价值发现
5. 用户画像变化时更新 bio

### 2.6 匹配引擎

- 基于 Agent bio 中的 5 维画像（Domains / Purpose / Recent work / Looking for / Country）
- AI 引擎做语义匹配，将广播推送给关心该话题的 Agent

## 三、EigenFlux 的优势

1. **概念创新**：首次定义了 Agent 原生通信协议，不是人类搜索的变种
2. **Token 效率**：结构化推送 vs 搜索爬取，声称节省 94% token
3. **冷启动策略好**：自建 1000+ 节点提供开箱即用的高价值资讯
4. **接入极简**：一句提示词安装 skill，30 秒接入
5. **利用 OpenClaw heartbeat**：巧妙利用 OpenClaw 的心跳机制实现持续通信
6. **免费策略**：低门槛吸引早期用户

## 四、EigenFlux 的不足与问题

### 4.1 架构层面

| 问题 | 分析 |
|------|------|
| **中心化单点** | 所有广播经 eigenflux.ai 服务器中转，一旦宕机全网瘫痪 |
| **纯 Pull 模型** | feed 需要 Agent 在 heartbeat 中主动拉取，非真正实时推送（没有 WebSocket/SSE） |
| **无消息路由** | 只有广播和 feed，没有 pub/sub topic 层级，无法精细订阅 |
| **无端到端加密** | 私信通道未实现，且未提及 E2E 加密方案 |
| **协议锁定** | 纯 REST API，不是开放协议，迁移成本高 |

### 4.2 产品层面

| 问题 | 分析 |
|------|------|
| **只支持 OpenClaw** | 排斥了 AutoGPT、CrewAI、LangGraph、Dify 等大量 Agent 框架 |
| **Bio 画像过于粗粒度** | 5 维文本描述不足以做精准匹配 |
| **无声誉/信任机制** | 只有简单的 -1/0/1/2 评分，没有 Agent 信誉体系 |
| **无交易/支付层** | 无法支持 Agent 间的付费服务交换 |
| **隐私模型薄弱** | 仅靠内容审查驳回隐私信息，缺乏数据最小化和用户控制 |
| **无法协商/多轮对话** | 广播是单次发射，无法实现复杂的多步骤协作 |
| **无 skill 市场** | 只是一个通信层，没有可组合的服务发现 |

### 4.3 商业层面

| 问题 | 分析 |
|------|------|
| **无商业模式** | 全免费，缺乏可持续收入来源 |
| **官方节点依赖** | 1000+ 自建节点是内容来源主力，一旦官方内容停更，网络价值骤降 |
| **网络效应冷启动难** | 用户发广播后没有响应 → 感觉没用 → 离开，死亡螺旋 |
| **垃圾信息隐患** | 没有 stake/cost 机制，免费广播易被滥用 |

### 4.4 生态层面

| 问题 | 分析 |
|------|------|
| **闭源服务端** | 开发者无法自建节点或扩展协议 |
| **无 SDK** | 要求 Agent 直接写 curl 命令，接入成本高 |
| **无事件驱动** | 不支持 webhook/callback，收到匹配广播时无法触发 Agent 行动 |
| **feed 质量无法个性化** | 似乎只基于 bio 匹配，没有基于历史行为的推荐 |

## 五、总结评价

EigenFlux 抓对了一个重要问题：**Agent 需要原生的通信网络，而不是人类搜索引擎的包装**。但其实现过于简化——本质上是一个中心化的 RSS feed + 简单的 AI 匹配引擎，披着"广播网络"的皮。

真正的 Agent 通信网络需要：
- 去中心化 / 联邦化架构
- 开放协议而非单一 API
- 多模态通信（广播 + 私信 + 群组 + 频道）
- 信任/声誉/支付层
- 多 Agent 框架支持
- 事件驱动推送

这正是 ClawNet 的机会。
