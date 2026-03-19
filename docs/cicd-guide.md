# ClawNet CI/CD 指南

> 最后更新: 2026-03-18 · 当前版本: v1.0.0-beta.4

---

## 1. 架构总览

```
开发者本地                          GitHub                             npm
┌──────────────┐    git push     ┌────────────────┐                ┌──────────────┐
│ 写代码        │ ──────────────→ │ main branch    │                │ npmjs.org    │
│ bump version │                 │                │                │ npmmirror.com│
└──────┬───────┘                 │  ┌───────────┐ │                └──────┬───────┘
       │                         │  │ Release   │ │  workflow_dispatch    │
       │ scripts/bump-version.sh │  │ (assets)  │ │ ──→ npm-publish.yml  │
       │ scripts/gh-release.sh   │  └─────┬─────┘ │       │              │
       └─────────────────────────│────────┘       │       │ OIDC publish │
                                 │                 │       └──────────────┘
                                 └────────────────┘
```

**核心流程: 4 步发版**

```bash
# 1. 改版本号 (自动修改 11 个文件)
./scripts/bump-version.sh 1.0.1

# 2. 编译 + 创建 GitHub Release + 上传二进制
./scripts/gh-release.sh

# 3. 提交 & 推送
git add -u && git commit -m "bump: v1.0.1" && git push

# 4. 触发 npm 发布 (GitHub Actions 页面手动点, 或 CLI)
# 见下文 §5
```

> **不能** push 后自动发布。npm 需要 GitHub Release 上的二进制文件作为来源，
> 而 Release 必须在 push 之前创建好资源——否则 workflow 会下载到空文件。

---

## 2. 版本管理

### 2.1 版本号约定

| 阶段 | 格式 | npm tag | 示例 |
|------|------|---------|------|
| 正式版 | `X.Y.Z` | `latest` | `1.0.0` |
| 预发布 | `X.Y.Z-beta.N` | `beta` | `1.0.0-beta.4` |
| Alpha | `X.Y.Z-alpha.N` | `beta` | `1.0.0-alpha.1` |

### 2.2 版本文件清单

`scripts/bump-version.sh` 会自动修改以下文件:

| 文件 | 说明 |
|------|------|
| `clawnet-cli/internal/daemon/daemon.go` | **权威来源** — `const Version` |
| `clawnet-cli/Makefile` | `VERSION :=` |
| `install.sh` | 回退 TAG |
| `SKILL.md` | 元数据 |
| `README.md` | shields.io badge |
| `npm/clawnet/package.json` | 包版本 + optionalDependencies |
| `npm/clawnet-linux-x64/package.json` | 平台包版本 |
| `npm/clawnet-linux-arm64/package.json` | 平台包版本 |
| `npm/clawnet-darwin-x64/package.json` | 平台包版本 |
| `npm/clawnet-darwin-arm64/package.json` | 平台包版本 |
| `npm/clawnet-win32-x64/package.json` | 平台包版本 |

```bash
# 查看当前版本
./scripts/bump-version.sh

# 升级版本
./scripts/bump-version.sh 1.0.1

# 验证
grep -r '1.0.1' --include='*.go' --include='*.json' --include='*.sh' --include='*.md' .
```

---

## 3. 编译 & 发布二进制

### 3.1 支持平台

| npm 包名 | GOOS/GOARCH | Release 文件名 |
|----------|-------------|---------------|
| `clawnet-linux-x64` | `linux/amd64` | `clawnet-linux-amd64` |
| `clawnet-linux-arm64` | `linux/arm64` | `clawnet-linux-arm64` |
| `clawnet-darwin-x64` | `darwin/amd64` | `clawnet-darwin-amd64` |
| `clawnet-darwin-arm64` | `darwin/arm64` | `clawnet-darwin-arm64` |
| `clawnet-win32-x64` | `windows/amd64` | `clawnet-windows-amd64.exe` |

### 3.2 本地交叉编译

```bash
cd clawnet-cli
VER=1.0.1

# 5 个目标平台
CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -ldflags="-s -w" -o clawnet-linux-amd64       ./cmd/clawnet/
CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build -ldflags="-s -w" -o clawnet-linux-arm64       ./cmd/clawnet/
CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 go build -ldflags="-s -w" -o clawnet-darwin-amd64      ./cmd/clawnet/
CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -ldflags="-s -w" -o clawnet-darwin-arm64      ./cmd/clawnet/
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o clawnet-windows-amd64.exe ./cmd/clawnet/
```

> `-ldflags="-s -w"` 去掉调试信息, 减小约 30% 体积。
> 如需 SQLite FTS5, linux/amd64 可用 `CGO_ENABLED=1 -tags fts5`。

### 3.3 GitHub Release

使用 `scripts/gh-release.sh`:

```bash
./scripts/gh-release.sh          # 自动读取 daemon.go 版本
./scripts/gh-release.sh 1.0.1    # 指定版本
```

脚本会:
1. 删除同名旧 Release (如果存在)
2. 创建 git tag 并推送
3. 创建 GitHub Release
4. 上传 `dist/` 中的二进制

或者手动操作:

```bash
# 创建 Release
curl -s -X POST \
  -H "Authorization: token $(cat GITHUB_TOKEN)" \
  "https://api.github.com/repos/ChatChatTech/ClawNet/releases" \
  -d '{
    "tag_name": "v1.0.1",
    "name": "v1.0.1",
    "prerelease": false
  }'

# 上传二进制 (RELEASE_ID 从上面返回)
curl -X POST \
  -H "Authorization: token $(cat GITHUB_TOKEN)" \
  -H "Content-Type: application/octet-stream" \
  "https://uploads.github.com/repos/ChatChatTech/ClawNet/releases/RELEASE_ID/assets?name=clawnet-linux-amd64" \
  --data-binary @clawnet-cli/clawnet-linux-amd64
```

---

## 4. npm 发布

### 4.1 包结构

```
npm/
├── clawnet/                     # 主包 @cctech2077/clawnet
│   ├── package.json             #   bin → bin/clawnet (JS launcher)
│   └── bin/clawnet              #   Node.js 脚本, 按平台 resolve 原生二进制
├── clawnet-linux-x64/           # @cctech2077/clawnet-linux-x64
│   ├── package.json             #   os: ["linux"], cpu: ["x64"]
│   └── bin/clawnet              #   原生 Go 二进制 (~35MB)
├── clawnet-linux-arm64/         # ...
├── clawnet-darwin-x64/
├── clawnet-darwin-arm64/
└── clawnet-win32-x64/
    └── bin/clawnet.exe
```

工作原理:
1. 用户 `npm install -g @cctech2077/clawnet`
2. npm 自动安装匹配当前系统的 `optionalDependencies` 平台包
3. 运行 `clawnet` 时, JS launcher (`npm/clawnet/bin/clawnet`) 通过 `require.resolve()` 找到平台包中的原生二进制并执行

### 4.2 认证: npm Trusted Publishers (OIDC)

**不使用 NPM_TOKEN 发布**。采用 npm Trusted Publishers, 由 GitHub Actions OIDC 自动认证。

前置条件:
- npm 账号: `cctech2077`
- npm CLI ≥ 11.5.1, Node.js ≥ 22.14.0
- 每个包在 npmjs.com → Settings → Publishing access 中配置了 Trusted Publisher:
  - Repository: `ChatChatTech/ClawNet`
  - Workflow: `npm-publish.yml`
  - Environment: (空)

Workflow 中只需:
```yaml
permissions:
  id-token: write    # 所需的唯一权限
  contents: read

steps:
  - uses: actions/setup-node@v6
    with:
      node-version: '24'
      registry-url: 'https://registry.npmjs.org'
  # 直接 npm publish, 无需 NODE_AUTH_TOKEN
```

### 4.3 Workflow: npm-publish.yml

文件: `.github/workflows/npm-publish.yml`

触发方式: **仅 `workflow_dispatch`** (手动触发)

> ⚠️ **绝对不要加 `release: [published]` 触发器！**
> Release 事件在资源上传完成前就触发, 会导致 workflow 下载到空文件。
> npm 不允许覆盖已发布的版本, 一旦发布空包, 该版本号永久作废。
> (beta.2 和 beta.3 就是这样被烧掉的。)

流程:

```
workflow_dispatch (手动)
  ↓
Checkout → Setup Node 24 → 确定版本号
  ↓
从 GitHub Release 下载 5 个二进制 (带 GITHUB_TOKEN 认证)
  ↓
验证每个二进制 > 1MB (否则 fail fast)
  ↓
发布 5 个平台包 → 发布主包
  ↓
验证所有 6 个包在 npm 上可见
```

安全措施:
- 下载二进制使用 `${{ secrets.GITHUB_TOKEN }}` (GitHub 自动生成的临时 token, 公开仓库安全)
- 3 次重试 + `< 1MB` 大小校验
- prerelease 版本自动使用 `--tag beta`

### 4.4 触发发布

**方式 A: GitHub Actions 网页**

Actions → Publish npm packages → Run workflow → 输入版本号 → Run

**方式 B: CLI**

```bash
curl -s -X POST \
  -H "Authorization: token $(cat GITHUB_TOKEN)" \
  "https://api.github.com/repos/ChatChatTech/ClawNet/actions/workflows/npm-publish.yml/dispatches" \
  -d '{"ref":"main","inputs":{"version":"1.0.1"}}'
```

版本号留空则自动从 `daemon.go` 读取。

### 4.5 验证发布结果

```bash
# 检查 npm
npm view @cctech2077/clawnet versions --json

# 下载测试
cd /tmp && npm pack @cctech2077/clawnet-linux-x64@1.0.1
tar tzf cctech2077-clawnet-linux-x64-1.0.1.tgz

# 或直接安装
npm install -g @cctech2077/clawnet@1.0.1
clawnet version
```

---

## 5. 完整发版 Checklist

```
□ 1. 代码改完, 测试通过
□ 2. 更新版本号
        ./scripts/bump-version.sh X.Y.Z
□ 3. 交叉编译 5 个二进制
        cd clawnet-cli && make cross  # 或手动 go build
□ 4. 创建 GitHub Release + 上传二进制
        ./scripts/gh-release.sh
□ 5. 确认 Release 页面 5 个资源都是 "uploaded" 状态且 >30MB
□ 6. 提交 & 推送
        git add -u && git commit -m "bump: vX.Y.Z" && git push
□ 7. 触发 npm-publish workflow
        # GitHub Actions 页面手动, 或:
        curl -s -X POST \
          -H "Authorization: token $(cat GITHUB_TOKEN)" \
          "https://api.github.com/repos/ChatChatTech/ClawNet/actions/workflows/npm-publish.yml/dispatches" \
          -d '{"ref":"main","inputs":{"version":"X.Y.Z"}}'
□ 8. 等待 workflow 完成 (约 1-2 分钟)
□ 9. 验证: npm view @cctech2077/clawnet@X.Y.Z
```

---

## 6. 安装方式

### 6.1 一键安装脚本 (推荐)

```bash
curl -fsSL https://raw.githubusercontent.com/ChatChatTech/ClawNet/main/install.sh | bash
```

下载优先级:
1. **npm (npmmirror.com)** — 中国用户首选, CDN 加速
2. **npm (npmjs.org)** — 国际
3. **GitHub Releases** — 回退

可指定来源:
```bash
CLAWNET_SOURCE=github curl -fsSL ... | bash
CLAWNET_SOURCE=npm    curl -fsSL ... | bash
```

### 6.2 npm 安装

```bash
# 国际
npm install -g @cctech2077/clawnet

# 中国镜像
npm install -g @cctech2077/clawnet --registry https://registry.npmmirror.com
```

### 6.3 直接下载

从 [GitHub Releases](https://github.com/ChatChatTech/ClawNet/releases) 下载对应平台的二进制, 放到 PATH 中即可。

---

## 7. GitHub Secrets & Tokens

| Secret | 用途 | 类型 |
|--------|------|------|
| `GITHUB_TOKEN` | Release 下载认证 | 自动生成, 临时, 无需配置 |
| `NPM_TOKEN` | npm cleanup (dist-tag/unpublish) | Granular Access Token, 在 GitHub Secrets 中配置 |

> `GITHUB_TOKEN` 是 GitHub 为每次 workflow 运行自动创建的临时 token,
> 运行结束后自动失效, 公开仓库也完全安全。

> `NPM_TOKEN` 目前受限于 npm 的 EOTP (2FA) 要求, `dist-tag` 和 `unpublish` 操作
> 无法通过 CI 自动完成, 需要手动在浏览器操作。

---

## 8. Workflow 文件参考

### .github/workflows/npm-publish.yml

| 配置项 | 值 | 说明 |
|--------|-----|------|
| 触发 | `workflow_dispatch` only | 手动触发, 防止竞态 |
| Node | 24 | npm Trusted Publishers 需要 ≥ 22.14.0 |
| 权限 | `id-token: write, contents: read` | OIDC 发布 |
| 二进制来源 | GitHub Release | 带 auth + retry + size validation |
| npm tag | `latest` 或 `beta` | 根据版本号自动判断 |

### .github/workflows/npm-cleanup.yml

| 配置项 | 值 | 说明 |
|--------|-----|------|
| 触发 | `workflow_dispatch` | 手动 |
| 认证 | `NPM_TOKEN` | 需要 OTP, 目前受限 |
| 功能 | 删除旧版本 | 仅正式版可在网页手动操作 |

---

## 9. 踩坑记录

### 9.1 Release 事件竞态 (beta.2 & beta.3 教训)

**问题**: `release: [published]` 触发的 workflow 在 Release 资源上传完成之前就开始运行, 下载到空文件 (9 bytes "Not Found")。npm 不允许覆盖已发版本, 导致版本号永久作废。

**解决**: 移除 `release` 触发器, 仅使用 `workflow_dispatch`。先上传资源, 确认就绪, 再手动触发。

### 9.2 npm 不允许覆盖版本

npm 的设计: 一旦 `npm publish` 成功, 该版本号就被锁定, 即使 `npm unpublish` 后也不能重新用同一个版本号。

**教训**: 发布前一定要确认二进制正确。一旦发错, 只能 bump 新版本。

### 9.3 npm Trusted Publishers 限制

- 仅支持 `npm publish`,  **不支持** `npm dist-tag` 和 `npm unpublish`
- 后者仍需要 NPM_TOKEN + OTP
- 删除旧版本或修改 dist-tag 需在 npmjs.com 网页手动操作

### 9.4 npmmirror 同步延迟

npmmirror 通常在 npmjs.org 发布后 10-30 分钟内同步。如果用户反馈安装脚本下载旧版本, 可能是镜像还没同步。

### 9.5 prerelease 不更新 latest tag

npm 的 prerelease 版本 (beta/alpha/rc) 不会自动设置为 `latest` tag。这意味着如果之前有 `0.0.0` 作为 `latest`, 它会一直保留, 直到发布第一个正式版或手动执行:

```bash
npm dist-tag add @cctech2077/clawnet@1.0.0 latest --auth-type=web
```

---

## 10. 目录结构速查

```
.github/workflows/
├── npm-publish.yml           # npm 发布 (OIDC)
└── npm-cleanup.yml           # npm 清理 (受限)

scripts/
├── bump-version.sh           # 统一版本号更新
├── gh-release.sh             # 创建 GitHub Release
├── release.sh                # 编译 + 上传 Cloudflare R2 (旧流程)
└── build-mac.sh              # macOS 编译

npm/
├── clawnet/                  # 主包 (JS launcher)
│   ├── package.json
│   └── bin/clawnet
├── clawnet-linux-x64/        # 平台包 ×5
├── clawnet-linux-arm64/
├── clawnet-darwin-x64/
├── clawnet-darwin-arm64/
└── clawnet-win32-x64/

install.sh                    # 一键安装脚本
GITHUB_TOKEN                  # 个人 token (本地开发用, 不提交)
```

---

## 11. 常见操作速查

```bash
# 查看当前版本
grep 'const Version' clawnet-cli/internal/daemon/daemon.go

# 快速发版 (完整流程)
./scripts/bump-version.sh 1.0.1
cd clawnet-cli && make cross && cd ..
./scripts/gh-release.sh
git add -u && git commit -m "release: v1.0.1" && git push
# → 然后去 Actions 页面触发 npm-publish

# 查看 npm 上所有版本
npm view @cctech2077/clawnet versions --json

# 查看 workflow 运行状态
curl -s -H "Authorization: token $(cat GITHUB_TOKEN)" \
  "https://api.github.com/repos/ChatChatTech/ClawNet/actions/workflows/npm-publish.yml/runs?per_page=3" \
  | jq '.workflow_runs[] | {id, status, conclusion, created_at}'
```
