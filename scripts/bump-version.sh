#!/usr/bin/env bash
set -euo pipefail

# ============================================================
# bump-version.sh — Update version across the entire repo
# Single source of truth: daemon.go → propagate everywhere
#
# Usage:
#   ./scripts/bump-version.sh 1.0.0-beta.2
#   ./scripts/bump-version.sh          # show current version
# ============================================================

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DAEMON_GO="$ROOT/clawnet-cli/internal/daemon/daemon.go"

GREEN='\033[0;32m'
DIM='\033[2m'
NC='\033[0m'
info() { echo -e "${GREEN}[✓]${NC} $*"; }

# Read current version
CURRENT=$(grep 'const Version' "$DAEMON_GO" | sed 's/.*"\(.*\)".*/\1/')

if [[ -z "${1:-}" ]]; then
  echo "Current version: $CURRENT"
  echo "Usage: $0 <new-version>"
  exit 0
fi

NEW="$1"
echo "Bumping version: $CURRENT → $NEW"
echo ""

# 1. daemon.go — canonical source
sed -i "s/const Version = \"$CURRENT\"/const Version = \"$NEW\"/" "$DAEMON_GO"
info "daemon.go"

# 2. Makefile
sed -i "s/^VERSION := .*/VERSION := $NEW/" "$ROOT/clawnet-cli/Makefile"
info "Makefile"

# 3. install.sh fallback tag
sed -i "s/TAG=\"v$CURRENT\"/TAG=\"v$NEW\"/" "$ROOT/install.sh"
info "install.sh"

# 4. SKILL.md
sed -i "s/version: \"$CURRENT\"/version: \"$NEW\"/" "$ROOT/SKILL.md"
info "SKILL.md"

# 5. README.md badge (shields.io uses -- to escape literal hyphens)
BADGE_NEW=$(echo "$NEW" | sed 's/-/--/g')
sed -i "s|version-[0-9][0-9a-zA-Z._-]*-E63946|version-${BADGE_NEW}-E63946|" "$ROOT/README.md"
info "README.md"

# 6. npm package.json files — version field and optionalDependencies
for pj in \
  "$ROOT/npm/clawnet/package.json" \
  "$ROOT/npm/clawnet-linux-x64/package.json" \
  "$ROOT/npm/clawnet-linux-arm64/package.json" \
  "$ROOT/npm/clawnet-darwin-x64/package.json" \
  "$ROOT/npm/clawnet-darwin-arm64/package.json" \
  "$ROOT/npm/clawnet-win32-x64/package.json"
do
  if [[ -f "$pj" ]]; then
    sed -i "s/\"$CURRENT\"/\"$NEW\"/g" "$pj"
    info "$(echo "$pj" | sed "s|$ROOT/||")"
  fi
done

echo ""
echo -e "${DIM}Verify:${NC} grep -r '$NEW' --include='*.go' --include='*.json' --include='*.sh' --include='*.md' ."
echo -e "${GREEN}Done.${NC} Version is now $NEW across $(echo 10) files."
