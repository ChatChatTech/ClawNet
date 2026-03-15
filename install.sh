#!/bin/sh
# ClawNet one-line installer
# Usage: curl -fsSL https://chatchat.space/releases/install.sh | bash
#
# Detects OS/arch, downloads the latest binary from GitHub Releases,
# installs to /usr/local/bin/clawnet, and runs `clawnet init`.

set -e

REPO="ChatChatTech/ClawNet"
INSTALL_DIR="/usr/local/bin"
BINARY_NAME="clawnet"

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
  TAG=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' \
    | head -1 \
    | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
  [ -n "$TAG" ] || err "Could not determine latest release from GitHub."
  info "Latest release: $TAG"
}

# ── Download binary ──────────────────────────────────────────────

download_binary() {
  ASSET_NAME="${BINARY_NAME}-${OS}-${ARCH}"
  URL="https://github.com/${REPO}/releases/download/${TAG}/${ASSET_NAME}"
  TMP="$(mktemp)"

  info "Downloading ${ASSET_NAME}..."
  HTTP_CODE=$(curl -fsSL -w '%{http_code}' -o "$TMP" "$URL" 2>/dev/null) || true

  if [ "$HTTP_CODE" != "200" ] || [ ! -s "$TMP" ]; then
    rm -f "$TMP"
    err "Download failed (HTTP $HTTP_CODE). Asset not found: $URL
Available assets may not include ${OS}-${ARCH}. Check https://github.com/${REPO}/releases/tag/${TAG}"
  fi

  ok "Downloaded $(wc -c < "$TMP" | tr -d ' ') bytes"
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
