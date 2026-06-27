#!/bin/sh
# Shell script to install KernelView Go on Linux and macOS
set -e

# Detect OS and Arch
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
    x86_64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    i386|i686) ARCH="386" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

case "$OS" in
    linux) OS="linux" ;;
    darwin) OS="darwin" ;;
    *) echo "Unsupported OS: $OS"; exit 1 ;;
esac

# Get latest release tag from GitHub
echo "Fetching latest release tag..."
LATEST_TAG=$(curl -s https://api.github.com/repos/codedbysoumyajit/KernelView-Go/releases/latest | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
if [ -z "$LATEST_TAG" ]; then
    LATEST_TAG="v1.0.0" # Fallback
fi

TAG_CLEAN=$(echo "$LATEST_TAG" | sed 's/^v//')
URL="https://github.com/codedbysoumyajit/KernelView-Go/releases/download/${LATEST_TAG}/kernelview_${TAG_CLEAN}_${OS}_${ARCH}.tar.gz"

echo "Downloading KernelView Go ${LATEST_TAG} for ${OS}/${ARCH}..."
curl -fsSL "$URL" -o kernelview.tar.gz

echo "Extracting..."
tar -xzf kernelview.tar.gz kernelview

echo "Installing to /usr/local/bin (requires sudo)..."
sudo mv kernelview /usr/local/bin/

rm -f kernelview.tar.gz
echo "KernelView Go installed successfully! Run 'kernelview' in your terminal."
