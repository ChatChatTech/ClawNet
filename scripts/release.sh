#!/usr/bin/env bash
set -euo pipefail

# ============================================================
# ClawNet Release Script
# 编译全平台二进制 → dist/ → 上传 Cloudflare R2
# Usage: ./scripts/release.sh
# ============================================================

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CLI_DIR="$ROOT/clawnet-cli"
DIST_DIR="$ROOT/dist"
CF_TOKEN="$ROOT/CLOUDFLARE_TOKEN"

R2_ENDPOINT="https://6f2afed64f4c167a7afa15663a450629.r2.cloudflarestorage.com"
R2_BUCKET="chatchat"

# --- 颜色 ---
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

info()  { echo -e "${GREEN}[✓]${NC} $*"; }
warn()  { echo -e "${YELLOW}[!]${NC} $*"; }
error() { echo -e "${RED}[✗]${NC} $*" >&2; exit 1; }

# --- 检查依赖 ---
command -v go    >/dev/null || error "go not found"
command -v python3 >/dev/null || error "python3 not found"
python3 -c "import boto3" 2>/dev/null || error "boto3 not installed: pip install boto3"

# --- 读取 R2 凭证 ---
[[ -f "$CF_TOKEN" ]] || error "Cloudflare token file not found: $CF_TOKEN"
R2_KEY_ID=$(grep '^CF_ADMIN_ACCESS_KEY_ID=' "$CF_TOKEN" | cut -d= -f2)
R2_SECRET=$(grep '^CF_ADMIN_SECRET_ACCESS_KEY=' "$CF_TOKEN" | cut -d= -f2)
[[ -n "$R2_KEY_ID" && -n "$R2_SECRET" ]] || error "Failed to parse R2 credentials from $CF_TOKEN"

# --- 清理 dist ---
mkdir -p "$DIST_DIR"
rm -f "$DIST_DIR"/*
info "Cleaned $DIST_DIR"

# --- 编译 ---
echo ""
echo "Building ClawNet binaries..."
cd "$CLI_DIR"

# linux/amd64 (CGO for fts5)
info "Building linux/amd64 (CGO)..."
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
  go build -ldflags="-s -w" -tags fts5 \
  -o "$DIST_DIR/clawnet-linux-amd64" ./cmd/clawnet/

# linux/arm64 (pure Go, no cross-compile toolchain)
info "Building linux/arm64..."
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
  go build -ldflags="-s -w" \
  -o "$DIST_DIR/clawnet-linux-arm64" ./cmd/clawnet/

# windows/amd64
info "Building windows/amd64..."
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
  go build -ldflags="-s -w" \
  -o "$DIST_DIR/clawnet-windows-amd64.exe" ./cmd/clawnet/

echo ""
ls -lh "$DIST_DIR"/
info "All binaries built"

# --- 上传 Cloudflare R2 ---
echo ""
echo "Uploading to Cloudflare R2 ($R2_BUCKET)..."

python3 << PYEOF
import boto3, os, sys

s3 = boto3.client('s3',
    endpoint_url='${R2_ENDPOINT}',
    aws_access_key_id='${R2_KEY_ID}',
    aws_secret_access_key='${R2_SECRET}',
)

bucket = '${R2_BUCKET}'
dist = '${DIST_DIR}'

uploads = {
    'clawnet-linux-amd64':       'releases/clawnet-linux-amd64',
    'clawnet-linux-arm64':       'releases/clawnet-linux-arm64',
    'clawnet-windows-amd64.exe': 'releases/clawnet-windows-amd64.exe',
}

for local_name, remote_key in uploads.items():
    local_path = os.path.join(dist, local_name)
    if not os.path.exists(local_path):
        print(f'  skip {local_name} (not found)')
        continue
    size_mb = os.path.getsize(local_path) / (1024 * 1024)
    print(f'  uploading {local_name} ({size_mb:.1f} MB) -> {remote_key} ...', flush=True)
    s3.upload_file(local_path, bucket, remote_key,
                   ExtraArgs={'ContentType': 'application/octet-stream'})
    print(f'  done')

# Verify
print()
print('--- R2 releases/ ---')
objs = s3.list_objects_v2(Bucket=bucket, Prefix='releases/')
for obj in objs.get('Contents', []):
    size_mb = obj['Size'] / (1024 * 1024)
    print(f"  {obj['Key']:50s}  {size_mb:6.1f} MB")
PYEOF

echo ""
info "Release complete!"
