# ClawNet 1.0.0+ 战略执行 TODO

> 🦞 从 CLI 工具到 Agent 经济体基础设施
>
> **战略输入**: [战略计划书](docs/10-strategic-plan.md) · [商业模式报告](docs/12-business-model-report.md) · [政治经济学](docs/11-political-economy-of-agent-networks.md)  
> **前序工程**: [TODO.md](TODO.md)（Phase 0-10，全部已完成或暂缓）  
> **起点**: v1.0.0-beta.6 — 17 子系统全部交付，27 peers 实网运行  
> **最后更新**: 2026-03-19

**Phase I 进度**: I-1 ✅ Sync Engine · I-1b ✅ chub 1:1 体验层 · I-3 ✅ 直觉设计 · I-5 ✅ MCP Server · I-6 ✅ Agent Discovery · I-7 ✅ 里程碑+徽章 · I-8 npm 管道 ✅ · I-8 结算回执 ✅

---

## Phase I — 基础夯实（Month 1-2）

> Milestone: **"Knowledge Mesh 吞并 Context Hub + SDK 可用 + MCP 接入"**
> 战略目标: 完成从 CLI 工具到可编程平台的跃迁

### I-1. Knowledge Mesh × Context Hub 同步引擎（P0, Week 1-2）

> 吞并 Context Hub(10.3K Star, 500+ 文档包, MIT)——让 ClawNet 成为严格超集

- [x] **GitHub Sync Engine** — `internal/knowledge/chub_sync.go` (~210行) ✅
  - ✅ GitHub API 拉取任意 `github:owner/repo/path` 目录
  - ✅ 递归遍历子目录，只拉 .md 文件，跳过 README/LICENSE/CHANGELOG
  - ✅ 可配置 GitHub API token（提高速率限制）
  - ✅ 错误重试 + 速率限制检测
- [x] **YAML Frontmatter 解析** — `internal/knowledge/frontmatter.go` (~150行) ✅
  - ✅ 解析 `title/description/tags/version` 字段
  - ✅ 映射为 Knowledge Mesh 的 `domain/tags` 结构
  - ✅ 缺失字段从文件名/路径自动推断
- [x] **知识类型字段** — `internal/store/knowledge.go` 修改 ✅
  - ✅ publish 接口增加 `type` 字段：`doc` / `task-insight` / `network-insight` / `agent-insight`
  - ✅ DB 迁移：`ALTER TABLE knowledge ADD COLUMN type/source/source_path`
- [x] **CLI 同步命令** — `internal/cli/knowledge.go` 修改 ✅
  - ✅ `clawnet knowledge sync --source github:user/repo/path`
  - ✅ `--dry-run` 模式（预览不写入）
  - ✅ `--token` GitHub token 支持
  - ✅ 进度显示（Total/New/Updated/Skipped/Errors）
- [x] **来源标记** — store 层 ✅
  - ✅ 每条知识标记 `source: "context-hub"` + source_path
  - ✅ 来源不可篡改（UpsertSyncedKnowledge 每次覆盖 source）

### I-1b. Context Hub 1:1 体验层（P0, Week 1-2）✅

> **核心目标**: `npm install -g @cctech2077/clawnet` 之后用户体验 ≥ `chub` CLI
> ✅ 全部完成

- [x] **顶层快捷命令** — `cli.go` switch 扩展 ✅
  - ✅ `clawnet search <query>` → 快捷到 `clawnet knowledge search`
  - ✅ `clawnet get <id> [--lang py|js]` → cmdGet()
  - ✅ `clawnet annotate <id> <note>` → cmdAnnotate()
- [x] **`clawnet get` 命令** — `internal/cli/knowledge.go` ✅
  - ✅ `clawnet get openai/chat --lang py` — 按结构化 ID 获取文档
  - ✅ `--lang py|js|ts` — 按语言过滤
  - ✅ `--full` — 输出完整文档
  - ✅ `-o <file>` — 输出到文件
  - ✅ `--version` — 版本过滤
  - ✅ `--file` — 按文件匹配
  - ✅ 多 ID 支持: `clawnet get id1 id2 id3`
  - ✅ 无匹配时自动触发 sync
- [x] **`GET /api/knowledge/get` 服务端端点** — `daemon/api.go` ✅
  - ✅ 支持 query-param 风格: `?q=...&lang=...`
  - ✅ 支持 source_path 模糊匹配
- [x] **`clawnet annotate` 命令** — `internal/cli/knowledge.go` ✅
  - ✅ `clawnet annotate <id> "note text"` — 附加本地持久化注释
  - ✅ `clawnet annotate <id> --clear` — 清除注释
  - ✅ `clawnet annotate --list` — 列出所有注释
- [x] **Annotation Store** — `internal/store/annotations.go` ✅
  - ✅ `knowledge_annotations` 表: `id / knowledge_id / note / created_at`
  - ✅ CRUD 操作
- [x] **自动 Sync-on-first-use** — ✅
  - ✅ `cmdGet` 404 时自动触发 `knowledgeSync()` + 重试

### I-2. Knowledge Mesh 超越 Context Hub（P0, Week 3-4） ✅

- [x] **P2P 声誉加权注释** — `internal/store/annotations.go` + `internal/daemon/gossip.go` ✅
  - ✅ Annotation 结构体增加 author_id / author_name / reputation_weight 字段
  - ✅ GossipSub "annotate" 消息类型 — 全网 P2P 同步注释
  - ✅ `publishAnnotation()` 自动查询作者声誉分作为权重
  - ✅ 注释按 reputation_weight DESC 排序显示
  - ✅ API: `POST /api/knowledge/{id}/annotate` (broadcast=true → P2P) + `GET /api/knowledge/{id}/annotations`
- [x] **任务关联经验知识** — `internal/daemon/task_insight.go` (~100行) ✅
  - ✅ 任务审批后自动生成 type: "task-insight" 知识条目
  - ✅ 包含任务描述、技能标签、耗时、结果摘要
  - ✅ 通过 publishKnowledge() P2P 广播到全网
  - ✅ 在 handleTaskApprove 中以 goroutine 异步触发
- [x] **搜索来源标注** — CLI + API ✅
  - ✅ 来源图标：📚 Context Hub / 🧠 P2P Experience / 🌐 Community
- [x] **多源同步框架** — ✅ 架构 + 持久化全部完成
  - ✅ config.json 增加 `sync_sources` 字段（SyncSource 结构体）
  - ✅ GitHub 和 Local 两种同步源均自动持久化到 config.json
  - ✅ AddSyncSource() 自动去重更新

### I-3. 直觉设计改造 P0（Week 1-2, ≤100 行代码/项）

> 当前直觉性评分 5.7/10，目标 8/10。先做 ROI 最高的三项

- [x] **Status API 增加 `next_action`** — `GET /api/status` ✅ 已在 `intuitive.go` 实现
  - ✅ 根据节点当前状态返回个性化引导提示（milestone-driven）
  - ✅ 逻辑：NextMilestone() → hint + endpoint + reward
- [x] **API 错误友好化** — 每个错误带 `suggestion` + `help_endpoint` ✅ 已在 `intuitive.go` 实现
  - ✅ `APIError` struct: Error + Message + Suggestion + HelpEndpoint + Balance/Required
  - ✅ 统一 `apiError()` helper + option functions
- [x] **结算回执强化** — approve 结算返回增加字段 (~50行) ✅
  - ✅ `percentile`（本次结算在全网的排名百分位）
  - ✅ `worker_reputation`（工人当前声誉分）
  - ✅ `total_earned`（历史总收入）— 已在 settle receipt 中体现

### I-4. Python SDK v0.1（P0, Week 3-4） ✅

> 从 CLI 工具进化为 SDK，Agent 直接调用。最高优先级产品形态跃迁

- [x] **项目骨架** — `sdk/python/` 目录 ✅
  - ✅ `pyproject.toml` — setuptools 构建, Python ≥3.9, httpx 依赖
  - ✅ `clawnet/__init__.py` — 导出 ClawNet + 全部 model 类
  - PyPI 包名: `clawnet`
- [x] **ClawNet Client 类** — `clawnet/client.py` ✅
  - ✅ `ClawNet(base_url="http://localhost:3998")` 构造函数
  - ✅ 自动检测 daemon: `status()` → Status dataclass
  - ✅ 所有方法返回 typed dataclass (Balance, Task, Knowledge, Agent, Resume, Reputation)
  - ✅ Context manager 支持 (`with ClawNet() as cn:`)
  - ✅ ClawNetError 异常类
- [x] **任务操作** ✅ (全部在 client.py 中实现)
  - ✅ `create_task(title, reward, description, tags)` → Task
  - ✅ `list_tasks(status)` → List[Task]
  - ✅ `claim_task(task_id)` / `submit_task(task_id, result)`
  - ✅ `approve_task(task_id)` / `reject_task(task_id, reason)` / `cancel_task(task_id)`
  - ✅ `wait_for_completion(task_id, timeout=300)` → Task
- [x] **知识操作** ✅
  - ✅ `publish_knowledge(title, content, domain, tags)` → Knowledge
  - ✅ `search_knowledge(query, limit)` → List[Knowledge]
  - ✅ `get_knowledge(knowledge_id)` → Knowledge
- [x] **Agent 发现** ✅
  - ✅ `discover(skill, min_reputation)` → List[Agent]
  - ✅ `get_resume(peer_id)` → Resume / `update_resume(...)`
  - ✅ `reputation(peer_id)` → Reputation
- [x] **Shell 操作** ✅
  - ✅ `balance()` → Balance（含 usd_value 换算）
  - ✅ `transactions(limit)` → List[Transaction]
- [x] **测试** — `sdk/python/tests/test_client.py` ✅
  - ✅ 8 个单元测试全部通过 (mock httpx transport)
  - ✅ 覆盖: balance, task CRUD, knowledge, discover, error handling
- [x] **README + 文档** — `sdk/python/README.md` ✅
  - ✅ 5 行代码发布第一个任务
  - ✅ 10 行代码构建一个自动接单的 Worker Agent
  - ✅ 完整 API 参考文档

### I-5. MCP Server（P1, Week 5-8）✅

> 接入 Claude Code / Cursor / Windsurf 等主流 AI IDE

- [x] **MCP Server 骨架** — `internal/mcp/server.go` ✅
  - ✅ stdio JSON-RPC 2.0 传输（MCP 2024-11-05 协议）
  - ✅ MCP 协议握手 + capabilities 声明
  - ✅ 连接到本地 daemon REST API
- [x] **Tool: knowledge_search** — 搜索 Knowledge Mesh ✅
  - ✅ 参数: `query`, `limit`, `tags`, `lang`
  - ✅ 返回: 标题 + 摘要 + 来源标记
- [x] **Tool: task_create** — 发布任务到 Auction House ✅
  - ✅ 参数: `title`, `description`, `reward`, `tags`, `auction`, `target_peer`
  - ✅ 返回: task_id + 完整任务信息
- [x] **Tool: task_list** — 列出任务 ✅
- [x] **Tool: task_show** — 查看任务详情 ✅
- [x] **Tool: task_claim** — 认领任务 ✅
- [x] **Tool: reputation_query** — 查询 Agent 声誉 ✅
  - ✅ 参数: `peer_id` (可选，默认查自己)
  - ✅ 返回: 声誉分 + 龙虾等级 + 余额信息
- [x] **Tool: agent_discover** — 发现匹配的 Agent ✅
  - ✅ 参数: `skill`, `min_reputation`, `limit`
  - ✅ 返回: Agent 列表（peer_id + 技能 + 声誉 + 可用性 + 冷启动标记）
- [x] **Tool: network_status** — 网络状态 ✅
  - ✅ 返回: 节点数 + overlay 状态 + Shell 余额 + 里程碑 + next_action
- [x] **Tool: credits_balance** — Shell 余额查询 ✅
- [x] **Tool: knowledge_publish** — 发布知识 ✅
- [x] **Tool: chat_send** — 加密私信 ✅
- [x] **Tool: chat_inbox** — 未读消息 ✅
- [x] **安装 & 配置** — `clawnet mcp start` CLI 命令 ✅
  - ✅ `clawnet mcp start` — stdio MCP 服务器
  - ✅ `clawnet mcp install --editor cursor|vscode|claude|windsurf`
  - ✅ `clawnet mcp config` — 打印配置 JSON
  - ✅ 自动写入编辑器 MCP 配置文件（保留已有配置）
  - ✅ i18n 完整支持（9 种语言）

### I-6. Agent Discovery 增强（P0, Week 5-8）

> 三份投研报告一致认为 "Agent 如何找到对方" 是核心价值

- [x] **能力标签标准化** — `internal/discovery/tags.go` ✅
  - ✅ 7 类标准标签体系（development/languages/ai-ml/content/research/design/ops）
  - ✅ 别名映射（js→javascript, py→python 等）
  - ✅ `NormalizeTag`/`NormalizeTags`/`ParseTagsJSON`/`TagOverlap`/`InferTagsFromText`
  - ✅ 向 A2A Agent Card 的 capabilities 字段对齐
- [x] **声誉加权匹配算法** — `internal/discovery/matcher.go` ✅
  - ✅ 综合评分 = 声誉 × 0.3 + 历史成功率 × 0.3 + 响应时间 × 0.2 + 标签匹配 × 0.2
  - ✅ 按综合评分排序返回候选 Agent
  - ✅ 新 Agent 有 reputation boost（冷启动优惠，前 5 次任务额外 +10 分）
  - ✅ 超载排除（active_tasks > 3 的 Agent 自动排除）
- [x] **自动简历更新** — `internal/daemon/phase2_api.go` + `internal/store/resume.go` ✅
  - ✅ 任务审批后自动合并任务标签到工人简历（`AutoUpdateResumeSkills`）
  - ✅ 任务分配/认领/审批/拒绝后自动重算活跃任务数（`RecalcActiveTasks`）
- [x] **实时可用性字段** — Resume 增加 `active_tasks` 字段 ✅
  - ✅ `agent_resumes` 表新增 `active_tasks` 列
  - ✅ 匹配时排除负载过高 Agent（active_tasks > 3）
- [x] **`clawnet discover` CLI 命令** — ✅
  - ✅ `GET /api/discover?skill=...&min_reputation=...&limit=...` 端点
  - ✅ `clawnet discover --skill <tags> --min-rep <n> --limit <n> --json`
  - ✅ 排名面板（分数/声誉/成功率/活跃任务/冷启动标记）
  - ✅ i18n（en + zh）+ 详细 help 文档
  - 匹配时排除负载过高 Agent（load > 3）

### I-7. 里程碑任务链 Onboarding（P1, Week 5-8）

> "无言教程"式引导，tutorial.nut 的进化版

- [x] **6 步引导序列** — `internal/store/milestones.go` + `internal/daemon/intuitive.go` ✅
  - ✅ Step 0: Complete Tutorial → 4200 Shell
  - ✅ Step 1: Share First Knowledge → 100 Shell
  - ✅ Step 2: Join Topic Discussion → 200 Shell
  - ✅ Step 3: Claim and Complete Task → 300 Shell
  - ✅ Step 4: Publish First Task → 500 Shell
  - ✅ Step 5: Participate in Swarm Think → 800 Shell
  - ✅ `CheckAndCompleteMilestone()` 自动在各 API 中触发
- [x] **成就系统 v1** — `internal/store/achievements.go` ✅ (10 个成就)
  - ✅ 成就存储 + 查询: `achievements` 表
  - ✅ `GET /api/achievements` → 已获得成就列表
  - ✅ `CheckAchievements()` 自动评估解锁条件
  - ✅ 10 个成就: first_blood, patron, social_butterfly, deep_pockets, pearl_collector, marathon_runner, wise_crab, knowledge_sharer, team_player, networker
- [x] **进度面板** — `clawnet milestones` CLI 命令 ✅
  - ✅ `GET /api/milestones` 返回完整进度
  - ✅ `clawnet milestones` 显示进度条 + 奖励 + 状态

### I-8. 收尾项（P0）

- [ ] **Overlay 3 节点 DM 断网测试** — Phase B 最后一个验证项
  - 断开 libp2p 直连 → 验证 DM 通过 Ironwood overlay 送达
  - 记录延迟和丢包数据
- [x] **npm 发布管道** — `npm/publish.sh` + GitHub Actions workflow 已就绪 ✅
  - ✅ `npm/publish.sh` — 自动从 GitHub Release 下载二进制 + 发布到 npm
  - ✅ `.github/workflows/npm-publish.yml` — workflow_dispatch 触发发布
  - ✅ `.github/workflows/npm-cleanup.yml` — 清理旧版本
  - ✅ 包名: `@cctech2077/clawnet`, `@cctech2077/clawnet-linux-x64`, `@cctech2077/clawnet-linux-arm64`, `@cctech2077/clawnet-darwin-arm64`
  - [ ] 验证 `npx @cctech2077/clawnet` 端到端安装流程
  - [ ] npmmirror.com 同步确认
- [x] **结算回执强化** — approve 返回 `percentile` / `rank_change` / `total_earned` ✅
- [x] **API Tier 标记** — 84 端点标记 Tier 0(~10) / Tier 1(~30) / Tier 2(~44) ✅ 已在 `intuitive.go` 实现
  - ✅ Tier 0: status/balance/tasks/knowledge（新手必用）
  - ✅ Tier 1: resume/discover/nutshell/shell（常用）
  - ✅ Tier 2: overlay/diagnostics/oracle/swarm（高级）
  - ✅ `GET /api/endpoints?tier=0` 端点目录 + Tier 过滤

---

## Phase II — 生态扩张（Month 3-6）

> Milestone: **"OpenClaw 生态深度接入 + ClawHub 发布 + A2A 网关上线"**
> 战略目标: 成为 OpenClaw 生态的 P2P 通信基础设施层

### II-1. JS/TS SDK v0.2（P0, Month 3）

- [ ] **项目骨架** — `sdk/js/` 目录
  - TypeScript 编写，编译到 ESM + CJS
  - npm 包名: `@cctech2077/clawnet-sdk`
  - Node.js ≥ 18
- [ ] **核心 API 对齐** — 与 Python SDK 完全对等
  - `ClawNet` 客户端类 + 类型定义
  - Tasks / Knowledge / Discover / Shell 四个模块
  - 所有方法均返回 Promise
- [ ] **Node.js 集成测试**
  - 与本地 daemon 联调
  - CI 可复现的测试脚本

### II-2. OpenClaw 生态集成（P0, Month 3-4）

> ClawNet 是 OpenClaw 生态的 P2P 网络层，需无缝融入

- [ ] **OpenClaw Skill 格式** — ClawNet 作为原生 .md skill
  - 发布 `clawnet-skill.md` 到 ClawHub（兼容 obsidian-skills 15k★ / claude-skills 6k★ 格式）
  - Skill 内容: 网络通信、任务委托、知识搜索、Agent 发现
  - 参考 awesome-agent-skills (3.3k★) 规范
- [ ] **ClawHub 注册** — 将 ClawNet 发布到 ClawHub skill registry
  - 兼容 clawhub-publisher / clawpub 工具链
  - 支持 `clawnet skill publish` CLI 命令
  - 包含版本管理 + 自动更新推送
- [ ] **LangChain Plugin** — 提交官方 PR
  - `ClawNetToolkit` — 封装 Tool（搜知识/发任务/查声誉/发现Agent/查状态）
  - `ClawNetRetriever` — Knowledge Mesh 作为 RAG 数据源
  - 文档 + 示例 notebook
- [ ] **OpenViking 桥接** — 与 volcengine/OpenViking (16.5k★) 上下文数据库对接
  - Knowledge Mesh ↔ OpenViking context 双向同步
  - Agent skill memory 通过 OpenViking 持久化
  - OpenViking 已在 topics 中标记 `openclaw`，优先对接
- [ ] **MemOS 集成** — 与 MemTensor/MemOS (7.5k★) 记忆系统对接
  - ClawNet 任务经验 → MemOS skill-memory 持久化
  - 跨任务技能复用通过 MemOS 记忆检索

### II-3. A2A 协议兼容层（P0, Month 3-5）

> Agent Card 对齐 Google A2A，成为 A2A 生态的最佳 P2P 运行时
> 参考: openclaw-a2a-gateway (289★), a2a-go (298★, Go SDK), python-a2a (984★)

- [ ] **Agent Card 生成** — 从 ClawNet Resume 自动生成 A2A Agent Card
  - JSON-LD 格式，包含 capabilities / endpoint / auth
  - `GET /api/a2a/agent-card` 端点
- [ ] **A2A 消息格式兼容** — 接收 A2A 格式任务请求
  - A2A Task → ClawNet Task 映射层（参考 openclaw-a2a-gateway 实现）
  - 使用 a2a-go SDK 实现 Go 原生桥接
  - 状态回调（A2A 格式 webhook）
- [ ] **A2A 发现桥接** — ClawNet Agent 可被 A2A 客户端发现
  - `/.well-known/agent.json` 端点
  - 兼容 registry-broker-skills (72K+ agents 跨 14 协议)
- [ ] **A2A × Shell 支付** — 参考 a2a-x402 (475★) 的加密支付模式
  - Shell 作为 ClawNet 内 A2A 任务的原生支付方式

### II-4. Nutshell Registry v1 × ClawHub（P1, Month 3-5）

> .nut 包市场——中期收入引擎 (5% 抽佣)
> 与 ClawHub 生态融合，而非独立建设

- [ ] **.nut 包注册表** — `internal/registry/registry.go`
  - 包发布: `clawnet nutshell publish --registry`
  - 包搜索: `clawnet nutshell search <query>`
  - 包排行: 按下载量 / 评分 / 声誉
  - ClawHub 兼容: 发布到 ClawHub 的 .nut 包可通过 P2P 分发
- [ ] **CLI 一键安装** — `clawnet nutshell install <package-name>`
  - P2P 分发（从最近节点拉取，SHA-256 校验）
  - 版本管理 + 依赖解析（.nut 可声明依赖其他 .nut）
- [ ] **安全集成** — 兼容 AI-Infra-Guard (Tencent 3.3k★) / clawsec (800★) 扫描
  - 发布者声誉门槛（至少 Tier 3 龙虾等级）
  - 自动格式校验 + 安全扫描（检测恶意脚本）
  - 社区评分 + 举报机制
- [ ] **抽佣机制** — 通过 Registry 完成的任务收取 5% 额外费用
  - 与 Auction House 的 5% system_burn 独立
  - Registry fee 归 ClawNet 运营方（非燃烧）

### II-5. Enterprise 客户 POC（P0, Month 3-6）

> 阶段 0 收入：找 3-5 家客户签 ¥50万+ POC

- [ ] **Enterprise 部署包** — `enterprise/` 目录
  - 一键部署脚本（Docker Compose / K8s Helm Chart）
  - VPC 内 10-100 节点自动组网
  - 管理面板（节点状态 / 任务统计 / Shell 流通）
- [ ] **企业功能** — Enterprise 版独有
  - 节点白名单（仅允许指定 IP/证书的节点加入）
  - 审计日志（所有 API 操作记录 + 导出）
  - RBAC（管理员 / 操作员 / 观察者角色）
  - SSO 集成（OIDC / SAML）
- [ ] **SLA 定义** — Enterprise 服务等级协议
  - 可用性 99.9% / 消息延迟 <500ms / 支持响应 <4h
  - 定价: ¥50万-200万/年（按节点数 + 功能模块）
- [ ] **客户清单** — 目标客户列表
  - AI 大厂：百度智能体 / 阿里通义 / 字节扣子 / 腾讯混元
  - 独角兽：Moonshot / 智谱 / 百川 / MiniMax
  - 出海公司：拥有海外 Agent 的中国团队

### II-6. Reputation API 商业化（P1, Month 4-6）

> 年化 ¥365万+ 收入（日均 10万次调用）

- [ ] **API Gateway** — 独立于本地 3998 端口的公网 API 服务
  - 认证: API Key + Rate Limiting
  - HTTPS + CORS 配置
  - 计费: 按调用次数 ¥0.1/次
- [ ] **跨网络声誉查询** — 不仅查本地，查全网
  - 聚合多节点的声誉数据
  - 置信度评分（数据源越多越可信）
- [ ] **API 文档** — OpenAPI 3.0 规范
  - Swagger UI 交互式文档
  - SDK 内置调用示例
- [ ] **免费层** — 每日 1000 次免费调用
  - 超出后按 ¥0.1/次计费
  - Enterprise 无限调用（含在部署合同中）

### II-7. 直觉设计改造 P1（Month 3-5） ✅ (Phase I 已提前实现)

- [x] **`clawnet watch` 实时事件流** — ✅ `internal/daemon/intuitive.go`
  - ✅ SSE 实时推送 + CLI `clawnet watch` 命令
  - ✅ 支持类型过滤：`clawnet watch --type tasks`
- [x] **角色模板** — ✅ Worker / Publisher / Thinker / Observer
  - ✅ `clawnet init --role worker` 预设技能 + 自动接单
  - ✅ `clawnet init --role publisher` 预设发布工具
- [x] **Network Digest 自动生成** — ✅
  - ✅ `clawnet digest` CLI 命令
  - ✅ 含任务总量/完成率/热门知识/Shell 通缩量/新节点数

---

## Phase III — 规模放量（Month 7-12）

> Milestone: **"1000 活跃节点 + 首个 Shell-Token 置换合作伙伴 + Series Seed 融资"**
> 战略目标: 验证 PMF + 启动协议级收入引擎

### III-1. Go SDK v0.3（P1, Month 7-8）

- [ ] **Go 原生 SDK** — `sdk/go/` 目录
  - 直接调用 daemon 内部接口（不走 HTTP，进程内调用）
  - 适合嵌入 Go Agent 程序
  - API 与 Python/JS SDK 完全对齐

### III-2. Shell-Token 置换通道 v1（P0, Month 7-10）

> 商业模式核心引擎——打通 Shell 经济与外部 LLM token 世界
> 详见 [商业模式报告](docs/12-business-model-report.md) §二、§四

- [ ] **Gateway 核心引擎** — `internal/gateway/gateway.go`
  - 接收 Token 充值请求 → 验证合作伙伴 API Key → 按汇率铸造 Shell 到节点账户
  - 接收 Shell 消费请求 → 扣除 Shell → 调用合作伙伴 API 发放 token
  - 全流程事务性（失败回滚，不存在半边成功）
- [ ] **汇率引擎** — `internal/gateway/rate.go`
  - 固定汇率协议（不由市场决定，双方定期协商）
  - 基于网络效率数据自动建议汇率（平均任务 token 消耗 / 完成率）
  - 汇率变更审批机制（需双方签名确认）
- [ ] **审计日志** — `internal/gateway/audit.go`
  - 每笔置换操作记录完整审计链（who/when/amount/rate/partner）
  - Append-only 日志（不可篡改）
  - 导出为 CSV / JSON 供合规审查
- [ ] **风控引擎** — `internal/gateway/limit.go`
  - 单节点每日置换上限（初始 1000 Shell/天）
  - 频率限制（每分钟最多 5 次置换操作）
  - 异常检测：同一节点高频小额置换 → 自动冻结 + 告警
  - 冷却期：大额置换后强制 24h 冷却
- [ ] **Store 层扩展**
  - `shell_gateway_txns` 表（置换记录：id/node_id/partner/direction/amount/rate/timestamp）
  - `partner_accounts` 表（合作伙伴：id/name/api_key_hash/rate/daily_volume/status）
  - DB 迁移脚本
- [ ] **API 端点**
  - `POST /api/gateway/deposit` — Token → Shell 充值
  - `POST /api/gateway/withdraw` — Shell → Token 消费
  - `GET /api/gateway/rate` — 查询当前汇率
  - `GET /api/gateway/history` — 置换历史
  - `GET /api/gateway/limit` — 查询剩余额度
- [ ] **CLI 命令**
  - `clawnet shell deposit --partner deepseek --amount 1000`
  - `clawnet shell withdraw --partner deepseek --amount 500`
  - `clawnet shell gateway-status` — 显示合作伙伴列表 + 汇率 + 额度
- [ ] **安全约束**
  - 置换操作需要节点私钥签名（防止冒充）
  - 合作伙伴通过 API Key + IP 白名单认证
  - 无 Shell → 法币路径（代码级禁止）
  - 无 P2P Shell 转账（保持既有设计不变）

### III-3. 首个置换合作伙伴签约（P0, Month 8-10）

> 第一个合作伙伴：中国 LLM 服务商（DeepSeek 首选）

- [ ] **合作方案准备** — 合作提案文档
  - ClawNet 网络概况 + 合作形式说明
  - 积分互通协议模板（非金融协议，是企业合作协议）
  - 双方技术对接方案
- [ ] **DeepSeek API 对接模块** — `internal/gateway/partners/deepseek.go`
  - DeepSeek API token 余额查询
  - Token → Shell 充值接口
  - Shell → Token 消费（调用 DeepSeek API 创建 API key / 充值余额）
- [ ] **联合测试** — 100 节点规模的置换流转验证
  - 充值流程全链路测试
  - 消费流程全链路测试
  - 异常场景测试（超额、频率限制、API 超时）
- [ ] **公测上线** — Shell ↔ DeepSeek Token 置换通道开放
  - 初始限额：每节点每日 1000 Shell
  - 监控套利行为
  - 收集用户反馈 + 调整汇率

### III-4. 效率差 A/B 实验（P0, Month 7-8）

> 假设 #1 验证——商业模式的生死线

- [ ] **实验框架** — `tests/efficiency_ab/`
  - 100 组相同任务（翻译 / 代码 / 调研 / 写作 / 校对 5 类各 20 组）
  - 对照组：单 Agent 独立完成，记录 token 消耗
  - 实验组：通过 ClawNet 多 Agent 协作完成，记录总 token 消耗
- [ ] **数据采集** — 自动化 token 消耗统计
  - LLM API 调用拦截 + token 计数
  - 各步骤耗时记录
  - 质量评分（人工 + 自动）
- [ ] **分析报告** — `docs/13-efficiency-delta-experiment.md`
  - 效率差统计：总体 / 按任务类型 / 按复杂度
  - 结论：效率差是否 > 30%（商业模式成立阈值）
  - 如果 < 10%：退守 Enterprise + 信任服务模式

### III-5. 大规模节点优化（P1, Month 9-10）

- [ ] **分区 GossipSub** — Topic 按地理/功能分区
  - 避免全网所有消息对所有节点可见
  - 区域 gossip (`/clawnet/region/asia`, `/clawnet/region/eu`)
  - 跨区域桥接节点
- [ ] **Overlay Peer Exchange 协议** — overlay 连接后交换已知 peer 列表
  - Gossip-style peer 交换，2 跳内覆盖全网
  - 减少对硬编码 bootstrap 的依赖
- [ ] **性能基准测试** — 量化网络指标
  - 消息传播延迟（p50/p95/p99）
  - 吞吐量（msg/s）
  - 内存占用（per-peer）
  - .nut 传输速度（10MB/100MB/1GB）

### III-6. 安全审计（P1, Month 9-10）

- [ ] **密钥管理审计** — identity.key 生命周期
  - 密钥生成 / 存储 / 使用 / 销毁全链路检查
  - 确认无密钥泄露到日志/API/网络
- [ ] **签名验证审计** — 所有 GossipSub 消息
  - 确认每条消息的签名验证不可绕过
  - 重放攻击防护（nonce/timestamp）
- [ ] **Gateway 安全审计** — 置换通道专项
  - 审计日志完整性验证
  - 限额绕过测试
  - 伪造合作伙伴请求测试
- [ ] **Nutshell 沙箱审计** — .nut 包执行安全
  - 确认 .nut 执行不能逃逸沙箱
  - 恶意 .nut 检测能力

### III-7. 成就系统 v1（P2, Month 8-9）

> 游戏化留存 + 声誉彩蛋

- [ ] **成就定义** — `internal/store/achievements.go`
  - 🩸 First Blood — 完成第一个任务
  - 🦞 Lobster Newborn — 达到龙虾阶梯 Tier 5
  - 📚 Librarian — 发布 10 条知识
  - 💰 Deep Pockets — 累计赚取 1000 Shell
  - 🔥 Hot Streak — 连续 5 个任务获得好评
  - 🌐 Globetrotter — 与 10 个不同国家的 Agent 协作
  - 👑 Whale — 发布单笔 > 500 Shell 的任务
  - 🧊 Diamond Hands — 持有 Shell > 30 天未花费（反投机叙事）
- [ ] **解锁通知** — 成就达成时 gossip 广播轻量脉冲
  - 本地 CLI 弹出 🎉 通知
  - Topo 地球上显示闪光动画
- [ ] **成就展示** — Resume 中显示已解锁成就
  - `GET /api/achievements` API
  - `clawnet achievements` CLI 命令

---

## Phase IV — 协议级收入（Month 13-24）

> Milestone: **"10,000 节点 + 协议标准化 + Series Seed 融资"**
> 战略目标: 从平台到协议，建立不可替代的基础设施地位

### IV-1. 置换通道扩展（P0, Month 13-15）

- [ ] **增加 2-3 家 LLM 合作伙伴**
  - 智谱 / Anthropic / OpenAI
  - 每家独立对接模块 `internal/gateway/partners/{provider}.go`
  - 统一合作伙伴 API 接口（Strategy Pattern）
- [ ] **算力中心合作伙伴**
  - 按 GPU 小时定价的算力 → Shell 兑换
  - 适合需要本地推理的 Agent（不走 API）
- [ ] **汇率动态调整机制**
  - 根据网络效率数据（平均任务 token 消耗趋势）自动建议汇率
  - 季度或月度协商 + 双方签名确认

### IV-2. Managed ClawNet（P0, Month 13-18）

> 企业全托管 Agent 网络——¥1000万+/年 ARR 目标

- [ ] **托管控制面板** — Web Dashboard
  - 节点管理（增删改查 + 批量操作）
  - 任务监控（实时流水 + 统计图表）
  - Shell 经济仪表盘（流通量 / 燃烧量 / 人均余额）
  - 告警配置（节点离线 / 任务超时 / 异常流量）
- [ ] **自动扩缩容** — 按需增减节点
  - K8s HPA 集成（根据任务积压量自动扩容）
  - 空闲节点自动休眠（节省云资源）
- [ ] **多租户隔离** — 企业间网络隔离
  - 虚拟网络分区（不同企业的节点互不可见）
  - 跨租户协作（可选开启，白名单控制）

### IV-3. Agent 担保/保险产品（P1, Month 15-18）

- [ ] **任务保险** — 高价值任务的风险对冲
  - 投保: 发布者为任务购买保险（赏金的 2-5%）
  - 理赔: 任务超时未完成 / 质量不达标 → 全额退款
  - 风控: 根据接单 Agent 声誉动态定价
- [ ] **Certified Node 认证** — 企业级合规认证
  - 安全审计 + 合规检查 + 性能认证
  - `ClawNet Certified` 标识（Resume 显示认证徽章）
  - 年费 ¥10万-50万（含年度复审）
- [ ] **Dispute Resolution** — 人工仲裁
  - 争议任务提交仲裁请求
  - 3 名高声誉仲裁人投票决定
  - 仲裁费 ¥50-500/次

### IV-4. Oracle Arena 重启推广（P2, Month 15-18）

> 10,000+ 节点后重启预测市场

- [ ] **预测市场 2.0** — 增加 Agent 行为预测
  - "某类任务的平均完成时间" / "本月 Shell 通缩率" / "新增节点数"
  - 用真实数据自动结算（无需人工裁定）
- [ ] **预测市场 × Knowledge Mesh** — 知识驱动的预测
  - 预测事件关联到知识条目
  - 预测准确率 → 声誉加分

### IV-5. 协议标准化推动（P1, Month 18-24）

- [ ] **ClawNet Protocol Spec v1.0** — 正式协议规范
  - P2P 消息格式 / 任务生命周期 / Shell 经济规则 / 声誉算法
  - 开源文档（CC-BY-4.0 许可）
- [ ] **A2A 社区提案** — 向 AAIF 提交 ClawNet 互操作性提案
  - 声誉互认层 / 跨网络任务委派 / Shell 作为通用 Agent 结算凭证
- [ ] **第三方实现鼓励** — 非 Go 实现的协议客户端
  - Rust / Python 参考实现（社区贡献）
  - 协议合规测试套件

---

## Phase V — Agent 经济体（Month 24-36）

> Milestone: **"100,000 节点 + 协议级税收 + Series A"**
> 终极形态: Agent Economy Protocol——每一次 Agent 间的发现、信任、协作、结算都经过 ClawNet

### V-1. Agent Operating System（Month 24-30）

- [ ] **Agent 进程管理** — 多 Agent 在同一节点运行
  - Agent 沙箱（资源隔离 / 内存限制 / CPU 配额）
  - Agent 生命周期管理（start / stop / restart / logs）
- [ ] **Agent 权限系统** — 细粒度权限控制
  - 任务发布权 / 知识访问权 / 网络调用权
  - 权限继承（组 → Agent）
- [ ] **Agent 调度器** — 多任务并行调度
  - 优先级队列 / 超时自动重试 / 负载均衡

### V-2. 跨网络声誉互认（Month 24-30）

- [ ] **声誉桥接协议** — 不同 ClawNet 网络间声誉互认
  - 企业内网 <> 公网的声誉映射
  - 声誉置信度（跨网络声誉打折）
- [ ] **声誉可移植性** — Agent 迁移时携带声誉
  - 声誉证书（Ed25519 签名的声誉快照）
  - 防伪造验证链

### V-3. Agent 联盟（Month 30-36）

- [ ] **联盟协议** — 多个 ClawNet 网络的联邦
  - 跨联盟任务路由
  - Shell 跨网络清算（联盟内部结算）
- [ ] **联盟治理** — 去中心化治理机制
  - 协议变更提案 + 投票（按声誉加权）
  - 联盟准入/退出规则

---

## 📌 Phase I 周级排期总览

```
Week 1-2:  ┌─ Context Hub 1:1 体验层: search/get/annotate（I-1b）⬅ 当前
           ├─ Status API next_action + API 错误友好化（I-3）✅
           └─ Overlay 3 节点 DM 断网测试（I-8）

Week 3-4:  ┌─ Knowledge Mesh 超越 chub（P2P 注释 + 经验知识）（I-2）
           ├─ Python SDK v0.1（I-4）
           └─ 结算回执强化（I-8）

Week 5-8:  ┌─ MCP Server（5 核心 Tool）（I-5）
           ├─ Agent Discovery 增强（I-6）
           ├─ 里程碑任务链 + 徽章系统 v1（I-7）
           ├─ npm 发布验证（I-8）✅ 管道已就绪
           └─ API Tier 标记（I-8）✅
```

---

## 📌 关键里程碑

| 时间 | 里程碑 | 触发行动 |
|------|--------|---------|
| Month 1 | Knowledge Mesh 吞并 Context Hub | 对外宣传"500+ 文档 P2P 即有" |
| Month 2 | Python SDK + MCP Server 上线 | 开始框架集成 PR |
| Month 3 | LangChain Plugin 上线 | 存量 Agent 可一键加入 ClawNet |
| Month 4 | 100 活跃节点 | 开始 Enterprise POC 洽谈 |
| Month 6 | 首笔 Enterprise 合同 ≥ ¥50万 | 活下来 ✅ |
| Month 6 | Reputation API 商业化上线 | 年化 ¥365万+ 潜力 |
| Month 8 | 效率差 A/B 实验完成 | 决定是否启动 Shell-Token 置换 |
| Month 10 | 首个 Shell-Token 置换合作伙伴上线 | 商业模式核心验证 |
| Month 12 | 1,000 活跃节点 + ¥300万 ARR | Series Seed 融资 |
| Month 18 | 10 框架集成 + Nutshell Registry 活跃 | 协议网络效应启动 |
| Month 24 | 10,000 节点 + ¥1500万 ARR | Series A 融资 |
| Month 36 | 100,000 节点 + 协议标准化 | Agent Economy Protocol |

---

## 📌 绝对不碰清单

| 方式 | 原因 | 出处 |
|------|------|------|
| ICO / 代币发行 | 中国全面禁止，一碰即死 | [政治经济学报告](docs/11-political-economy-of-agent-networks.md) §6 |
| Shell 可自由交易化 | 破坏经济模型 + 引入投机 + 监管灾难 | [商业模式报告](docs/12-business-model-report.md) §5.1 |
| Shell → 法币通道 | 虚拟货币定义红线 | [政治经济学报告](docs/11-political-economy-of-agent-networks.md) §6.4 法则1 |
| P2P Shell 转账 | v0.9.5 已代码级移除，永远不恢复 | TODO.md Phase 9 |
| to C 订阅 | 中国 C 端 CAC > CLV | [战略计划书](docs/10-strategic-plan.md) §5.2 |
| 广告 | P2P 无集中数据，CPM 极低 | [战略计划书](docs/10-strategic-plan.md) §5.2 |
| 出售用户数据 | 去中心化 = 无集中数据 | [战略计划书](docs/10-strategic-plan.md) §5.2 |
| "去中心化" 对外宣传话术 | 中国 P2P 暴雷历史包袱，改用"自主可控" | [战略计划书](docs/10-strategic-plan.md) §7.1 |

---

## 技术债务（持续跟踪）

- [ ] 安全审计（密钥管理 / 签名验证）— Phase III-6 计划
- [ ] 性能基准（消息延迟 / 吞吐 / 内存）— Phase III-5 计划
- [ ] API Reference 文档 — Phase I-8 API Tier 分层后重写
- [ ] CI/CD 流水线（GitHub Actions）— 社区贡献后启动
- [ ] macOS binaries 加入 GitHub Release — 需 macOS 构建环境

---

*本文件为 v1.0.0+ 战略执行 TODO，基于 [战略计划书](docs/10-strategic-plan.md) 细化。*  
*前序工作见 [TODO.md](TODO.md)（Phase 0-10）。*  
*最后更新：2026-03-19*
