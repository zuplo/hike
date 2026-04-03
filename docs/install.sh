#!/bin/sh
set -e

REPO="zuplo/hike"
BIN_HOME="$HOME/.hike/bin"
LINK_DIR="${LINK_DIR:-/usr/local/bin}"

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
  echo "Error: could not determine latest release."
  exit 1
fi

VERSION="${LATEST#v}"

echo "Installing hike ${VERSION} (${OS}/${ARCH})..."

# Download and extract
DL_DIR=$(mktemp -d)
ARCHIVE="hike_${VERSION}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${LATEST}/${ARCHIVE}"

curl -fSL --progress-bar "$URL" -o "${DL_DIR}/${ARCHIVE}"
tar -xzf "${DL_DIR}/${ARCHIVE}" -C "$DL_DIR"

# Install binary to ~/.hike/bin/ (no sudo needed)
mkdir -p "$BIN_HOME"
mv "${DL_DIR}/hike" "${BIN_HOME}/hike"
chmod +x "${BIN_HOME}/hike"
rm -rf "$DL_DIR"

# Create symlinks in /usr/local/bin (sudo only needed once)
create_link() {
  name=$1
  if [ -L "${LINK_DIR}/${name}" ] && [ "$(readlink "${LINK_DIR}/${name}")" = "${BIN_HOME}/hike" ]; then
    return
  elif [ -w "$LINK_DIR" ]; then
    ln -sf "${BIN_HOME}/hike" "${LINK_DIR}/${name}"
  else
    sudo ln -sf "${BIN_HOME}/hike" "${LINK_DIR}/${name}"
  fi
}

echo "Creating symlinks (requires sudo)..."
create_link hike
create_link hk

echo "hike ${VERSION} installed to ${BIN_HOME}/hike"
echo "Available as: hike, hk"
echo ""
echo "Future updates need no sudo — just run: hike update"
