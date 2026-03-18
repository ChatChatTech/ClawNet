#!/bin/sh
# ClawNet one-line installer
# Usage: curl -fsSL https://chatchat.space/releases/install.sh | bash
#
# Download sources (tried in order):
#   1. GitHub Releases  (fastest outside China)
#   2. npm registry     (npmmirror works great inside China)
#
# Override:  CLAWNET_SOURCE=npm  curl ... | bash
#            CLAWNET_SOURCE=github  curl ... | bash

set -e

REPO="ChatChatTech/ClawNet"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="clawnet"
TIMEOUT=15  # seconds per source attempt

# ── Helpers ──────────────────────────────────────────────────────

info()  { printf '\033[1;34m[info]\033[0m  %s\n' "$1"; }
ok()    { printf '\033[1;32m[ok]\033[0m    %s\n' "$1"; }
warn()  { printf '\033[1;33m[warn]\033[0m  %s\n' "$1"; }
err()   { printf '\033[1;31m[error]\033[0m %s\n' "$1" >&2; exit 1; }

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || err "Required command '$1' not found. Please install it first."
}

# ── Detect OS ────────────────────────────────────────────────────

detect_os() {
  OS="$(uname -s)"
  case "$OS" in
    Linux*)  OS="linux" ;;
    Darwin*) OS="darwin" ;;
    *)       err "Unsupported OS: $OS. ClawNet supports Linux and macOS." ;;
  esac
}

# ── Detect Architecture ─────────────────────────────────────────

detect_arch() {
  ARCH="$(uname -m)"
  case "$ARCH" in
    x86_64|amd64)  ARCH="amd64" ;;
    aarch64|arm64)  ARCH="arm64" ;;
    armv7l)         ARCH="arm64" ;;  # best-effort fallback
    *)              err "Unsupported architecture: $ARCH. ClawNet supports amd64 and arm64." ;;
  esac
}

# ── Fetch latest release tag ────────────────────────────────────

fetch_latest_tag() {
  need_cmd curl
  TAG=$(curl -fsSL --connect-timeout "$TIMEOUT" \
    "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null \
    | grep '"tag_name"' \
    | head -1 \
    | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/') || true
  if [ -z "$TAG" ]; then
    warn "Could not reach GitHub API, using fallback version"
    TAG="v1.0.0-beta.2"
  fi
  info "Target release: $TAG"
}

# ── Download: GitHub Releases ───────────────────────────────────

download_github() {
  ASSET_NAME="${BINARY_NAME}-${OS}-${ARCH}"
  URL="https://github.com/${REPO}/releases/download/${TAG}/${ASSET_NAME}"
  info "Trying GitHub Releases..."
  HTTP_CODE=$(curl -fsSL --connect-timeout "$TIMEOUT" --max-time 120 \
    -w '%{http_code}' -o "$TMP" "$URL" 2>/dev/null) || HTTP_CODE="000"
  if [ "$HTTP_CODE" = "200" ] && [ -s "$TMP" ]; then
    ok "Downloaded from GitHub ($(wc -c < "$TMP" | tr -d ' ') bytes)"
    return 0
  fi
  warn "GitHub download failed (HTTP $HTTP_CODE)"
  return 1
}

# ── Download: npm registry ──────────────────────────────────────

download_npm() {
  # Map Go arch names to npm conventions
  case "$ARCH" in
    amd64) NPM_ARCH="x64" ;;
    arm64) NPM_ARCH="arm64" ;;
    *)     NPM_ARCH="$ARCH" ;;
  esac
  NPM_PKG="@cctech2077/clawnet-${OS}-${NPM_ARCH}"
  # strip leading v from tag
  NPM_VER="${TAG#v}"

  # Try npmmirror first (great for China), then official npm
  for REGISTRY in "https://registry.npmmirror.com" "https://registry.npmjs.org"; do
    info "Trying npm: ${REGISTRY}..."
    # npm tarball URL: @scope/name/-/name-version.tgz
    PKG_BASE="clawnet-${OS}-${NPM_ARCH}"
    TARBALL_URL="${REGISTRY}/${NPM_PKG}/-/${PKG_BASE}-${NPM_VER}.tgz"

    TARBALL_TMP="$(mktemp)"
    HTTP_CODE=$(curl -fsSL --connect-timeout "$TIMEOUT" --max-time 120 \
      -w '%{http_code}' -o "$TARBALL_TMP" "$TARBALL_URL" 2>/dev/null) || HTTP_CODE="000"

    if [ "$HTTP_CODE" = "200" ] && [ -s "$TARBALL_TMP" ]; then
      # Extract binary from tarball: package/bin/clawnet
      EXTRACT_DIR="$(mktemp -d)"
      tar xzf "$TARBALL_TMP" -C "$EXTRACT_DIR" 2>/dev/null
      NPM_BIN="$EXTRACT_DIR/package/bin/clawnet"
      if [ -f "$NPM_BIN" ] && [ -s "$NPM_BIN" ]; then
        cp "$NPM_BIN" "$TMP"
        rm -rf "$EXTRACT_DIR" "$TARBALL_TMP"
        ok "Downloaded from npm registry ($(wc -c < "$TMP" | tr -d ' ') bytes)"
        return 0
      fi
      rm -rf "$EXTRACT_DIR"
    fi
    rm -f "$TARBALL_TMP"
  done

  warn "npm download failed"
  return 1
}

# ── Multi-source download orchestrator ──────────────────────────

download_binary() {
  TMP="$(mktemp)"

  # If user specifies a source, only try that one
  case "${CLAWNET_SOURCE:-auto}" in
    github) download_github || err "GitHub download failed" ;;
    npm)    download_npm    || err "npm download failed" ;;
    auto)
      # Try in order: GitHub → npm (npmmirror + npmjs)
      download_github && return 0
      download_npm    && return 0
      err "All download sources failed. Check your network connection.
  
  Manual alternatives:
    GitHub:  https://github.com/${REPO}/releases
    npm:     npm install -g @cctech2077/clawnet
    npm(CN): npm install -g @cctech2077/clawnet --registry https://registry.npmmirror.com"
      ;;
    *)
      err "Unknown CLAWNET_SOURCE='${CLAWNET_SOURCE}'. Use: github, npm, or auto"
      ;;
  esac
}

# ── Install ──────────────────────────────────────────────────────

install_binary() {
  TARGET="${INSTALL_DIR}/${BINARY_NAME}"

  # Check if we can write directly
  if [ -w "$INSTALL_DIR" ]; then
    mv "$TMP" "$TARGET"
    chmod +x "$TARGET"
  else
    info "Elevated permissions required to install to $INSTALL_DIR"
    if command -v sudo >/dev/null 2>&1; then
      sudo mv "$TMP" "$TARGET"
      sudo chmod +x "$TARGET"
    else
      err "Cannot write to $INSTALL_DIR and sudo is not available. Run as root or set INSTALL_DIR."
    fi
  fi

  ok "Installed to ${TARGET}"
}

# ── Init ─────────────────────────────────────────────────────────

init_clawnet() {
  if [ ! -f "$HOME/.openclaw/clawnet/config.json" ]; then
    info "Running 'clawnet init'..."
    "${INSTALL_DIR}/${BINARY_NAME}" init 2>/dev/null && ok "Initialized successfully" || warn "Init failed — run 'clawnet init' manually"
  else
    info "Config already exists, skipping init"
  fi
}

# ── Main ─────────────────────────────────────────────────────────

main() {
  info "ClawNet installer"
  detect_os
  detect_arch
  info "Detected: ${OS}/${ARCH}"
  fetch_latest_tag
  download_binary
  install_binary
  init_clawnet

  echo ""
  ok "ClawNet installed! 🦞"
  echo ""
  echo "  Start the daemon:    clawnet start"
  echo "  Check status:        clawnet status"
  echo "  View topology:       clawnet topo"
  echo "  Get help:            clawnet help"
  echo ""
}

main
