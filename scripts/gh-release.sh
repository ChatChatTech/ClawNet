#!/usr/bin/env bash
set -euo pipefail

# ============================================================
# GitHub Release Script
# 从 dist/ 中的二进制创建 GitHub Release 并上传
# 依赖: release.sh 先编译好二进制
# Usage: ./scripts/gh-release.sh [版本号]
#   版本号可选，默认从 daemon.go 中读取
# ============================================================

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST_DIR="$ROOT/dist"
TOKEN_FILE="$ROOT/GITHUB_TOKEN"
REPO="ChatChatTech/ClawNet"

GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

info()  { echo -e "${GREEN}[✓]${NC} $*"; }
error() { echo -e "${RED}[✗]${NC} $*" >&2; exit 1; }

# --- 读取版本号 ---
if [[ -n "${1:-}" ]]; then
    VER="$1"
else
    VER=$(grep 'const Version' "$ROOT/clawnet-cli/internal/daemon/daemon.go" \
        | head -1 | sed 's/.*"\(.*\)".*/\1/')
fi
TAG="v${VER}"
echo "Release: $TAG"

# --- 检查 dist/ ---
ASSETS=()
for f in clawnet-linux-amd64 clawnet-linux-arm64 clawnet-windows-amd64.exe; do
    [[ -f "$DIST_DIR/$f" ]] && ASSETS+=("$DIST_DIR/$f")
done
[[ ${#ASSETS[@]} -eq 0 ]] && error "No binaries in $DIST_DIR — run scripts/release.sh first"
info "Found ${#ASSETS[@]} binaries"

# --- 读取 Token ---
[[ -f "$TOKEN_FILE" ]] || error "GITHUB_TOKEN file not found"
TOKEN=$(cat "$TOKEN_FILE")

API="https://api.github.com/repos/$REPO"

# --- 辅助函数: GitHub API ---
gh_api() {
    local method="$1" endpoint="$2"
    shift 2
    curl -sf -X "$method" \
        -H "Authorization: token $TOKEN" \
        -H "Accept: application/vnd.github+json" \
        "$endpoint" "$@"
}

# --- 清理旧的同名 release/tag ---
EXISTING=$(gh_api GET "$API/releases/tags/$TAG" 2>/dev/null || true)
if [[ -n "$EXISTING" ]]; then
    RELEASE_ID=$(echo "$EXISTING" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
    echo "Deleting existing release $TAG (id=$RELEASE_ID)..."
    gh_api DELETE "$API/releases/$RELEASE_ID" >/dev/null
    info "Deleted old release"
fi

# 删除远程 tag（如果存在）
git -C "$ROOT" push origin ":refs/tags/$TAG" 2>/dev/null || true
git -C "$ROOT" tag -d "$TAG" 2>/dev/null || true

# --- 创建 tag ---
git -C "$ROOT" tag "$TAG"
git -C "$ROOT" push origin "$TAG"
info "Pushed tag $TAG"

# --- 生成 release notes ---
NOTES=$(cat <<'EOF'
## What's New

See [commit log](https://github.com/REPO/compare/PREV...TAG) for full changes.

### Binaries
- `clawnet-linux-amd64` — Linux x86_64 (CGO, fts5)
- `clawnet-linux-arm64` — Linux ARM64
- `clawnet-windows-amd64.exe` — Windows x86_64
EOF
)
# 替换占位符
PREV_TAG=$(git -C "$ROOT" tag --sort=-version:refname | grep -v "^${TAG}$" | head -1)
NOTES="${NOTES//REPO/$REPO}"
NOTES="${NOTES//PREV/${PREV_TAG:-main}}"
NOTES="${NOTES//TAG/$TAG}"

# --- 创建 release ---
CREATE_BODY=$(python3 -c "
import json, sys
print(json.dumps({
    'tag_name': '$TAG',
    'name': '$TAG',
    'body': sys.stdin.read(),
    'draft': False,
}))
" <<< "$NOTES")

RELEASE_JSON=$(gh_api POST "$API/releases" -d "$CREATE_BODY")
RELEASE_ID=$(echo "$RELEASE_JSON" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
HTML_URL=$(echo "$RELEASE_JSON" | python3 -c "import sys,json; print(json.load(sys.stdin)['html_url'])")
info "Created release $TAG (id=$RELEASE_ID)"

# --- 上传二进制 ---
UPLOAD_URL="https://uploads.github.com/repos/$REPO/releases/$RELEASE_ID/assets"

for asset in "${ASSETS[@]}"; do
    name=$(basename "$asset")
    size=$(stat -c%s "$asset")
    size_mb=$(python3 -c "print(f'{$size/1048576:.1f}')")
    echo -n "  Uploading $name ($size_mb MB)... "
    curl -sf \
        -H "Authorization: token $TOKEN" \
        -H "Content-Type: application/octet-stream" \
        --data-binary "@$asset" \
        "$UPLOAD_URL?name=$name" >/dev/null
    echo "done"
done

info "Release published: $HTML_URL"
