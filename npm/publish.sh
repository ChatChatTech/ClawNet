#!/bin/bash
# npm/publish.sh — Download binaries from GitHub Release and publish to npm.
# Usage: ./publish.sh [version]    e.g. ./publish.sh 0.9.5
set -euo pipefail

VERSION="${1:-0.9.5}"
REPO="ChatChatTech/ClawNet"
ROOT="$(cd "$(dirname "$0")" && pwd)"

# Token
NPM_TOKEN="$(cat "$ROOT/../NPM_TOKEN" 2>/dev/null || echo "")"
GITHUB_TOKEN="$(cat "$ROOT/../GITHUB_TOKEN" 2>/dev/null || echo "")"

if [ -z "$NPM_TOKEN" ]; then
  echo "❌ NPM_TOKEN file not found"
  exit 1
fi

echo "🐚 Publishing ClawNet v${VERSION} to npm"

# Map: npm-package-dir → github-release-asset-name
declare -A ASSETS=(
  ["clawnet-linux-x64"]="clawnet-linux-amd64"
  ["clawnet-linux-arm64"]="clawnet-linux-arm64"
  ["clawnet-darwin-x64"]="clawnet-darwin-amd64"
  ["clawnet-darwin-arm64"]="clawnet-darwin-arm64"
)

# Download binaries from GitHub Release
echo "📥 Downloading binaries from GitHub Release v${VERSION}..."
for dir in "${!ASSETS[@]}"; do
  asset="${ASSETS[$dir]}"
  dest="$ROOT/$dir/bin/clawnet"
  mkdir -p "$(dirname "$dest")"

  url="https://github.com/${REPO}/releases/download/v${VERSION}/${asset}"
  echo "  ${asset} → ${dir}/bin/clawnet"
  curl -sL -o "$dest" \
    ${GITHUB_TOKEN:+-H "Authorization: token $GITHUB_TOKEN"} \
    "$url"
  chmod +x "$dest"

  size=$(stat -c%s "$dest" 2>/dev/null || stat -f%z "$dest" 2>/dev/null)
  echo "    $(( size / 1024 / 1024 )) MB"
done

# Write .npmrc for auth
write_npmrc() {
  echo "//registry.npmjs.org/:_authToken=${NPM_TOKEN}" > "$1/.npmrc"
}

# Publish platform packages first
for dir in "${!ASSETS[@]}"; do
  echo "📦 Publishing @chatchat/${dir}..."
  cd "$ROOT/$dir"
  write_npmrc .
  npm publish --access public 2>&1 | tail -3
  rm -f .npmrc
done

# Publish main wrapper package
echo "📦 Publishing @chatchat/clawnet..."
cd "$ROOT/clawnet"
write_npmrc .
npm publish --access public 2>&1 | tail -3
rm -f .npmrc

echo ""
echo "✅ Published @chatchat/clawnet@${VERSION}"
echo ""
echo "🇨🇳 中国用户安装:"
echo "   npm install -g @chatchat/clawnet --registry https://registry.npmmirror.com"
echo ""
echo "🌍 国際用户安装:"
echo "   npm install -g @chatchat/clawnet"
