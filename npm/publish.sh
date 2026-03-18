#!/bin/bash
# npm/publish.sh — Download binaries from GitHub Release and publish to npm.
# Usage: ./publish.sh [version]    e.g. ./publish.sh 0.9.10
#        ./publish.sh              (auto-detects from daemon.go)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"

# Auto-detect version from daemon.go if not provided
if [ -n "${1:-}" ]; then
  VERSION="$1"
else
  VERSION=$(grep 'const Version' "$ROOT/../clawnet-cli/internal/daemon/daemon.go" \
    | sed 's/.*"\(.*\)".*/\1/')
  [ -n "$VERSION" ] || { echo "❌ Cannot detect version from daemon.go"; exit 1; }
fi

REPO="ChatChatTech/ClawNet"

# Token
NPM_TOKEN="$(cat "$ROOT/../NPM_TOKEN" 2>/dev/null || echo "")"
GITHUB_TOKEN="$(cat "$ROOT/../GITHUB_TOKEN" 2>/dev/null || echo "")"

if [ -z "$NPM_TOKEN" ]; then
  echo "❌ NPM_TOKEN file not found"
  exit 1
fi

echo "🐚 Publishing ClawNet v${VERSION} to npm"

# ── Update all package.json versions ──────────────────────────────
echo ""
echo "📝 Bumping package.json versions to ${VERSION}..."
for pkg_json in \
  "$ROOT/clawnet/package.json" \
  "$ROOT/clawnet-linux-x64/package.json" \
  "$ROOT/clawnet-linux-arm64/package.json" \
  "$ROOT/clawnet-darwin-x64/package.json" \
  "$ROOT/clawnet-darwin-arm64/package.json" \
  "$ROOT/clawnet-win32-x64/package.json"; do
  if [ -f "$pkg_json" ]; then
    sed -i "s/\"version\": \"[^\"]*\"/\"version\": \"${VERSION}\"/" "$pkg_json"
    echo "  ✓ $(basename "$(dirname "$pkg_json")")/package.json"
  fi
done

# Also bump optionalDependencies in the main wrapper
sed -i "s/@cctech2077\/clawnet-\([^\"]*\)\": \"[^\"]*\"/@cctech2077\/clawnet-\1\": \"${VERSION}\"/g" \
  "$ROOT/clawnet/package.json"
echo "  ✓ clawnet/package.json optionalDependencies"

# Map: npm-package-dir → github-release-asset-name
declare -A ASSETS=(
  ["clawnet-linux-x64"]="clawnet-linux-amd64"
  ["clawnet-linux-arm64"]="clawnet-linux-arm64"
  ["clawnet-darwin-x64"]="clawnet-darwin-amd64"
  ["clawnet-darwin-arm64"]="clawnet-darwin-arm64"
  ["clawnet-win32-x64"]="clawnet-windows-amd64.exe"
)

# Binary names inside npm packages
declare -A BIN_NAMES=(
  ["clawnet-linux-x64"]="clawnet"
  ["clawnet-linux-arm64"]="clawnet"
  ["clawnet-darwin-x64"]="clawnet"
  ["clawnet-darwin-arm64"]="clawnet"
  ["clawnet-win32-x64"]="clawnet.exe"
)

# Download binaries from GitHub Release
echo ""
echo "📥 Downloading binaries from GitHub Release v${VERSION}..."
for dir in "${!ASSETS[@]}"; do
  asset="${ASSETS[$dir]}"
  bin_name="${BIN_NAMES[$dir]}"
  dest="$ROOT/$dir/bin/$bin_name"
  mkdir -p "$(dirname "$dest")"

  url="https://github.com/${REPO}/releases/download/v${VERSION}/${asset}"
  echo "  ${asset} → ${dir}/bin/${bin_name}"
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
echo ""
for dir in "${!ASSETS[@]}"; do
  echo "📦 Publishing @cctech2077/${dir}..."
  cd "$ROOT/$dir"
  write_npmrc .
  npm publish --access public 2>&1 | tail -3
  rm -f .npmrc
done

# Publish main wrapper package
echo ""
echo "📦 Publishing @cctech2077/clawnet..."
cd "$ROOT/clawnet"
write_npmrc .
npm publish --access public 2>&1 | tail -3
rm -f .npmrc

echo ""
echo "✅ Published @cctech2077/clawnet@${VERSION}"
echo ""
echo "Install methods:"
echo "  🌍 Global:    npm install -g @cctech2077/clawnet"
echo "  🇨🇳 China:     npm install -g @cctech2077/clawnet --registry https://registry.npmmirror.com"
echo "  🐚 Script:    curl -fsSL https://chatchat.space/releases/install.sh | bash"
echo "  🇨🇳 Script:    CLAWNET_SOURCE=npm curl -fsSL https://chatchat.space/releases/install.sh | bash"
