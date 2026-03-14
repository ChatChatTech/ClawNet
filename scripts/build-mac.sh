#!/usr/bin/env bash
set -euo pipefail

# ============================================================
# ClawNet macOS 编译脚本
# 编译 darwin/arm64 + darwin/amd64 → dist/
# Usage: ./scripts/build-mac.sh
# ============================================================

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CLI_DIR="$ROOT/clawnet-cli"
DIST_DIR="$ROOT/dist"

GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

info()  { echo -e "${GREEN}[✓]${NC} $*"; }
error() { echo -e "${RED}[✗]${NC} $*" >&2; exit 1; }

command -v go >/dev/null || error "go not found"

mkdir -p "$DIST_DIR"
cd "$CLI_DIR"

GOFLAGS="-ldflags=-s -w -tags fts5"

# darwin/arm64 (Apple Silicon, CGO enabled for fts5)
info "Building darwin/arm64 (CGO)..."
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 \
  go build -ldflags="-s -w" -tags fts5 \
  -o "$DIST_DIR/clawnet-darwin-arm64" ./cmd/clawnet/
info "Built clawnet-darwin-arm64"

# darwin/amd64 (Intel Mac, CGO enabled for fts5)
info "Building darwin/amd64 (CGO)..."
CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 \
  go build -ldflags="-s -w" -tags fts5 \
  -o "$DIST_DIR/clawnet-darwin-amd64" ./cmd/clawnet/
info "Built clawnet-darwin-amd64"

echo ""
ls -lh "$DIST_DIR"/clawnet-darwin-*
echo ""
info "macOS build complete!"
