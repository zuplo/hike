#!/bin/sh
set -e

REPO="zuplo/zproj"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

case "$OS" in
  darwin|linux) ;;
  *) echo "Unsupported OS: $OS"; exit 1 ;;
esac

# Get a GitHub token for API auth if not already set
if [ -z "$GITHUB_TOKEN" ] && command -v gh >/dev/null 2>&1; then
  GITHUB_TOKEN=$(gh auth token 2>/dev/null || true)
fi

AUTH_HEADER=""
if [ -n "$GITHUB_TOKEN" ]; then
  AUTH_HEADER="Authorization: token ${GITHUB_TOKEN}"
fi

# Get latest release tag
LATEST=$(curl -sL ${AUTH_HEADER:+-H "$AUTH_HEADER"} "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "$LATEST" ]; then
  echo "Error: could not determine latest release."
  echo "GitHub API may be rate-limited. Install gh CLI (https://cli.github.com) and run 'gh auth login', then retry."
  exit 1
fi

VERSION="${LATEST#v}"

echo "Installing zproj ${VERSION} (${OS}/${ARCH})..."

# Download and extract
DL_DIR=$(mktemp -d)
ARCHIVE="zproj_${VERSION}_${OS}_${ARCH}.tar.gz"

if [ -n "$GITHUB_TOKEN" ]; then
  # Use gh CLI for download (handles private repos correctly)
  if command -v gh >/dev/null 2>&1; then
    gh release download "$LATEST" --repo "$REPO" --pattern "$ARCHIVE" --dir "$DL_DIR"
  else
    # Fallback: API asset download for private repos
    ASSET_URL=$(curl -sL -H "$AUTH_HEADER" "https://api.github.com/repos/${REPO}/releases/tags/${LATEST}" \
      | grep -A 4 "\"name\": \"${ARCHIVE}\"" | grep '"url"' | sed -E 's/.*"(https[^"]+)".*/\1/')
    curl -fSL --progress-bar -H "$AUTH_HEADER" -H "Accept: application/octet-stream" "$ASSET_URL" -o "${DL_DIR}/${ARCHIVE}"
  fi
else
  URL="https://github.com/${REPO}/releases/download/${LATEST}/${ARCHIVE}"
  curl -fSL --progress-bar "$URL" -o "${DL_DIR}/${ARCHIVE}"
fi
tar -xzf "${DL_DIR}/${ARCHIVE}" -C "$DL_DIR"

# Install
if [ -w "$INSTALL_DIR" ]; then
  mv "${DL_DIR}/zproj" "${INSTALL_DIR}/zproj"
else
  echo "Need sudo to install to ${INSTALL_DIR}"
  sudo mv "${DL_DIR}/zproj" "${INSTALL_DIR}/zproj"
fi

rm -rf "$DL_DIR"

echo "zproj ${VERSION} installed to ${INSTALL_DIR}/zproj"
