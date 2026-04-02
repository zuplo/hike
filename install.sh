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

# Get latest release tag
LATEST=$(curl -sL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "$LATEST" ]; then
  echo "Error: could not determine latest release. GitHub API may be rate-limited."
  echo "Try setting GITHUB_TOKEN or run: gh release download --repo ${REPO} --pattern '*${OS}_${ARCH}*'"
  exit 1
fi

VERSION="${LATEST#v}"

echo "Installing zproj ${VERSION} (${OS}/${ARCH})..."

# Download and extract
DL_DIR=$(mktemp -d)
ARCHIVE="zproj_${VERSION}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${LATEST}/${ARCHIVE}"

curl -fSL --progress-bar "$URL" -o "${DL_DIR}/${ARCHIVE}"
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
