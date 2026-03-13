# OpenClaw 生态深度研究

## 一、OpenClaw 概览

OpenClaw（原名 Clawdbot/Moltbot）是由 Peter Steinberger 创建的**开源自主 AI 助手平台**，核心理念是"在你自己的机器上运行、通过你日常聊天工具交互的 AI 助手"。

### 核心特征

| 特征 | 详情 |
|------|------|
| **运行位置** | 本地机器（Mac/Windows/Linux），数据私有 |
| **LLM 支持** | Anthropic Claude、OpenAI GPT、本地模型（MiniMax 等） |
| **通信渠道** | WhatsApp、Telegram、Discord、Slack、Signal、iMessage |
| **核心能力** | 持久记忆、浏览器控制、文件系统访问、Shell 执行、技能扩展 |
| **扩展机制** | Skills（AgentSkills 格式）+ Plugins |
| **开源** | 完全开源，社区驱动 |

### 架构组件

```
用户聊天 App（Telegram/WhatsApp/...）
         ↕
    OpenClaw Gateway（本地运行）
         ↕
    ┌────────────────────────┐
    │  Session Manager       │ ← 持久记忆
    │  Skills Engine         │ ← SKILL.md 加载
    │  Plugin System         │ ← 插件扩展
    │  Browser Controller    │ ← 网页交互
    │  System Access Layer   │ ← 文件/Shell
    │  Heartbeat Scheduler   │ ← 定时任务
    └────────────────────────┘
         ↕
    LLM Provider（Claude/GPT/Local）
```

## 二、Skills 系统详解

### 2.1 Skill 格式（AgentSkills Spec）

每个 Skill 是一个目录，包含 `SKILL.md`：

```yaml
---
name: skill-name
description: What this skill does
metadata: {"openclaw": {"requires": {"bins": ["tool"], "env": ["API_KEY"]}}}
---

# Skill Instructions（Markdown 正文）
具体指令，告诉 Agent 如何使用这个工具...
```

### 2.2 加载优先级

1. `<workspace>/skills`（最高优先级）
2. `~/.openclaw/skills`（管理/本地）
3. Bundled skills（内置）
4. `skills.load.extraDirs`（额外目录，最低）

### 2.3 ClawHub —— Skill 市场

- 类似 npm 的 Skill 注册中心
- 支持版本管理、回滚
- 向量搜索技能
- 安装命令：`npx clawhub@latest install <skill-slug>`
- 热门 Skills：self-improving-agent（185k 下载）、find-skills、summarize、gog、github 等

### 2.4 Gating 机制

技能加载时根据以下条件过滤：
- **bins**：需要系统上有对应工具
- **env**：需要环境变量/API Key
- **config**：需要特定配置启用
- **os**：操作系统限制
- **always**：始终加载

### 2.5 运行时注入

- 每次 Agent run 时注入环境变量
- Session 启动时快照 eligible skills
- 支持 hot reload（文件监听器）

## 三、OpenClaw Heartbeat 机制

OpenClaw 的定时心跳是外部服务接入的关键入口：

```
每个心跳周期：
  1. 检查通信渠道消息
  2. 执行计划任务（cron jobs）
  3. 处理外部服务回调（如 EigenFlux feed）
  4. 更新记忆
  5. 主动推送信息给用户
```

这是 EigenFlux 能接入的技术基础——将自己的 heartbeat 指令嵌入到 OpenClaw 的 heartbeat.md 中。

## 四、OpenClaw 生态现状

### 4.1 社区规模

- GitHub 开源
- Discord 社区活跃
- 知名推荐者：Karpathy、多位知名开发者
- ClawHub 上数百个 Skills

### 4.2 典型用例

- 邮件管理（Gmail 收发、退订）
- 日程管理（CalDAV 日历、航班值机）
- 代码开发（Claude Code/Codex 集成、PR 管理）
- 智能家居（Hue、空气净化器、Sonos）
- 信息聚合（RSS、新闻摘要）
- 社交媒体管理（Twitter、Discord）
- 文件处理（PDF 摘要、知识图谱）

### 4.3 生态痛点

1. **孤岛问题**：每个 OpenClaw 实例是独立的，Agent 之间无法通信
2. **信息获取低效**：依赖搜索引擎（为人类设计），token 消耗大
3. **缺乏服务发现**：一个 Agent 无法发现另一个 Agent 能提供什么能力
4. **无协作框架**：无法实现多 Agent 任务分工
5. **Skill 安全隐患**：第三方 skill 可能含恶意代码（虽已和 VirusTotal 合作）

## 五、围绕 OpenClaw 生态的机会

### 5.1 通信层（EigenFlux 已切入）
- Agent 间广播/私信
- 信息推送替代搜索

### 5.2 服务层（未被占据）
- Agent 能力注册与发现
- 付费服务交换
- SLA 保障

### 5.3 协作层（完全空白）
- 多 Agent 任务编排
- 工作流定义与执行
- 共享上下文空间

### 5.4 信任层（完全空白）
- Agent 身份验证
- 声誉评分系统
- 交易仲裁

### 5.5 数据层（完全空白）
- 结构化知识共享
- 隐私保护数据交换
- Agent 偏好学习

## 六、技术接入要点

### 6.1 成为 OpenClaw Skill 的要求

1. 创建包含 `SKILL.md` 的目录
2. YAML frontmatter 定义名称、描述、依赖
3. Markdown 正文写清楚 Agent 应该如何使用
4. 通过 ClawHub 发布

### 6.2 利用 Heartbeat 的策略

- 在 skill 安装时将自定义心跳指令写入 `heartbeat.md`
- 心跳周期中拉取数据、推送信息、执行任务
- 这是让 Agent 具备"持续连接"能力的关键

### 6.3 跨框架兼容

虽然 OpenClaw 是主要目标，但 ClawNet 应当设计为：
- AgentSkills spec 兼容（OpenClaw 原生支持）
- MCP (Model Context Protocol) 兼容（更广泛的 Agent 支持）
- REST API（通用接入）
- SDK（Python/TypeScript，面向开发者的快速集成）
