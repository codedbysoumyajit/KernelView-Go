#!/bin/sh
# KernelView Go - Multi-OS Installation & Update Script
# Supported OS: Linux (glibc & musl/Alpine), macOS (Darwin), FreeBSD, OpenBSD, NetBSD, Android (Termux)
# Supported Arch: x86_64/amd64, aarch64/arm64, i386/386, armv7/arm, riscv64
#
# Quick Run:
#   curl -fsSL https://raw.githubusercontent.com/codedbysoumyajit/KernelView-Go/main/install.sh | sh
#
# Unattended/CI Run:
#   curl -fsSL https://raw.githubusercontent.com/codedbysoumyajit/KernelView-Go/main/install.sh | sh -s -- -y
#
# Force Reinstall:
#   curl -fsSL https://raw.githubusercontent.com/codedbysoumyajit/KernelView-Go/main/install.sh | sh -s -- --force
#
# User-local install (~/.local/bin):
#   curl -fsSL https://raw.githubusercontent.com/codedbysoumyajit/KernelView-Go/main/install.sh | sh -s -- --user

set -e

REPO="codedbysoumyajit/KernelView-Go"
FALLBACK_TAG="v1.4.0-nightly"
BINARY_NAME="kernelview"

# Parse CLI options
ASSUME_YES=false
FORCE=false
FORCE_USER=false
CHANNEL=""
CUSTOM_DIR=""
CUSTOM_VERSION=""

while [ $# -gt 0 ]; do
    case "$1" in
        -y|--yes)
            ASSUME_YES=true
            shift
            ;;
        -f|--force)
            FORCE=true
            shift
            ;;
        -b|--beta|--nightly)
            CHANNEL="beta"
            shift
            ;;
        -s|--stable)
            CHANNEL="stable"
            shift
            ;;
        -c|--channel)
            CHANNEL="$2"
            shift 2
            ;;
        -u|--user)
            FORCE_USER=true
            shift
            ;;
        -d|--dir)
            CUSTOM_DIR="$2"
            shift 2
            ;;
        -v|--version)
            CUSTOM_VERSION="$2"
            shift 2
            ;;
        -h|--help)
            echo "KernelView Go Installer"
            echo ""
            echo "Options:"
            echo "  -y, --yes          Answer yes to all prompts (non-interactive)"
            echo "  -f, --force        Force install / overwrite even if already on latest"
            echo "  -b, --beta         Install pre-release / beta (nightly) version if available"
            echo "  -s, --stable       Install stable release version"
            echo "  -c, --channel CH   Release channel: 'stable' or 'beta'/'nightly'"
            echo "  -u, --user         Install to user directory (~/.local/bin) without sudo"
            echo "  -d, --dir DIR      Specify custom installation directory"
            echo "  -v, --version V    Specific version tag to install (e.g. v1.4.0)"
            echo "  -h, --help         Display this help message"
            exit 0
            ;;
        *)
            shift
            ;;
    esac
done

# ANSI Color helpers (using printf escape sequences compatible with all POSIX shells)
if [ -t 1 ]; then
    BOLD="$(printf '\033[1m')"
    GREEN="$(printf '\033[0;32m')"
    CYAN="$(printf '\033[0;36m')"
    YELLOW="$(printf '\033[1;33m')"
    RED="$(printf '\033[0;31m')"
    RESET="$(printf '\033[0m')"
else
    BOLD=""
    GREEN=""
    CYAN=""
    YELLOW=""
    RED=""
    RESET=""
fi

info() {
    printf "%b==>%b %b%s%b\n" "$CYAN" "$RESET" "$BOLD" "$*" "$RESET"
}

success() {
    printf "%b==> SUCCESS:%b %s\n" "$GREEN" "$RESET" "$*"
}

warn() {
    printf "%b==> WARNING:%b %s\n" "$YELLOW" "$RESET" "$*"
}

error() {
    printf "%b==> ERROR:%b %s\n" "$RED" "$RESET" "$*" >&2
}

# Interactive prompt function that safely handles piped stdin (curl ... | sh)
prompt_user() {
    _prompt="$1"
    _default="$2"

    if [ "$ASSUME_YES" = "true" ]; then
        PROMPT_REPLY="y"
        return
    fi

    # Check if a controlling terminal is available to read from
    if [ -c /dev/tty ] && ( : < /dev/tty ) 2>/dev/null; then
        printf "%s" "$_prompt"
        read -r PROMPT_REPLY </dev/tty
    else
        # No controlling tty: try reading from standard input (e.g. piped answers), else use default
        printf "%s" "$_prompt"
        if ! read -r PROMPT_REPLY 2>/dev/null || [ -z "$PROMPT_REPLY" ]; then
            PROMPT_REPLY="$_default"
            printf "%s\n" "$_default"
        fi
    fi

    if [ -z "$PROMPT_REPLY" ]; then
        PROMPT_REPLY="$_default"
    fi
}

# 1. Detect OS
OS_RAW="$(uname -s)"
case "$OS_RAW" in
    Linux)
        OS="linux"
        ;;
    Darwin)
        OS="darwin"
        ;;
    FreeBSD)
        OS="freebsd"
        ;;
    OpenBSD)
        OS="openbsd"
        ;;
    NetBSD)
        OS="netbsd"
        ;;
    DragonFly)
        OS="freebsd" # DragonFly BSD supports FreeBSD ELF binaries
        ;;
    *)
        error "Unsupported operating system: $OS_RAW"
        exit 1
        ;;
esac

# Check for Termux on Android
IS_TERMUX=false
if [ -n "$PREFIX" ] && [ -d "$PREFIX/bin" ] && [ "$OS" = "linux" ]; then
    IS_TERMUX=true
elif [ "$(uname -o 2>/dev/null)" = "Android" ]; then
    IS_TERMUX=true
fi

# 2. Detect Architecture
ARCH_RAW="$(uname -m)"
case "$ARCH_RAW" in
    x86_64|amd64)
        ARCH="amd64"
        ;;
    aarch64|arm64|armv8*)
        ARCH="arm64"
        ;;
    i386|i486|i586|i686)
        ARCH="386"
        ;;
    armv7*|armv6*|armhf|arm)
        ARCH="arm"
        ;;
    riscv64)
        ARCH="riscv64"
        ;;
    *)
        ARCH="$ARCH_RAW"
        ;;
esac

info "Detected system: $OS ($ARCH)"$( [ "$IS_TERMUX" = "true" ] && echo " [Termux environment]" )

# 3. Detect downloader (curl or wget)
if command -v curl >/dev/null 2>&1; then
    FETCH_CMD="curl -fsSL"
elif command -v wget >/dev/null 2>&1; then
    FETCH_CMD="wget -qO-"
else
    error "Neither 'curl' nor 'wget' was found. Please install one to proceed."
    exit 1
fi

# 4. Check for existing installation
EXISTING_BIN=""
if [ -n "$CUSTOM_DIR" ] && [ -x "$CUSTOM_DIR/$BINARY_NAME" ]; then
    EXISTING_BIN="$CUSTOM_DIR/$BINARY_NAME"
elif command -v "$BINARY_NAME" >/dev/null 2>&1; then
    EXISTING_BIN="$(command -v "$BINARY_NAME")"
else
    for candidate in \
        /usr/local/bin/"$BINARY_NAME" \
        /usr/bin/"$BINARY_NAME" \
        "$HOME/.local/bin/$BINARY_NAME" \
        "$HOME/bin/$BINARY_NAME" \
        "${PREFIX:-/data/data/com.termux/files/usr}/bin/$BINARY_NAME"; do
        if [ -x "$candidate" ]; then
            EXISTING_BIN="$candidate"
            break
        fi
    done
fi

INSTALLED_VERSION=""
if [ -n "$EXISTING_BIN" ]; then
    # Try getting version from the existing binary
    RAW_VER="$("$EXISTING_BIN" -v 2>/dev/null || "$EXISTING_BIN" --version 2>/dev/null || true)"
    INSTALLED_VERSION="$(echo "$RAW_VER" | grep -oE 'v?[0-9]+\.[0-9]+(\.[0-9]+)?' | head -n1)"
    if [ -n "$INSTALLED_VERSION" ] && ! echo "$INSTALLED_VERSION" | grep -q '^v'; then
        INSTALLED_VERSION="v$INSTALLED_VERSION"
    fi
fi

# 5. Determine target release tag (detect stable vs pre-release / beta / nightly)
TARGET_TAG="$CUSTOM_VERSION"
if [ -z "$TARGET_TAG" ]; then
    info "Checking for KernelView releases on GitHub..."
    if command -v curl >/dev/null 2>&1; then
        RELEASES_JSON=$(curl -s "https://api.github.com/repos/$REPO/releases?per_page=15" 2>/dev/null || true)
    else
        RELEASES_JSON=$(wget -qO- "https://api.github.com/repos/$REPO/releases?per_page=15" 2>/dev/null || true)
    fi

    # Parse both the latest stable release and the latest pre-release
    PARSED_TAGS=$(echo "$RELEASES_JSON" | awk '
    BEGIN { stable = ""; prerelease = ""; cur_tag = "" }
    /"tag_name":/ {
        split($0, parts, "\"")
        cur_tag = parts[4]
    }
    /"prerelease":[ ]*true/ {
        if (cur_tag != "" && prerelease == "") {
            prerelease = cur_tag
        }
        cur_tag = ""
    }
    /"prerelease":[ ]*false/ {
        if (cur_tag != "" && stable == "") {
            stable = cur_tag
        }
        cur_tag = ""
    }
    END {
        print stable
        print prerelease
    }')

    LATEST_STABLE=$(echo "$PARSED_TAGS" | sed -n '1p')
    LATEST_PRERELEASE=$(echo "$PARSED_TAGS" | sed -n '2p')

    # Fallback to single latest endpoint if releases list was empty or rate-limited
    if [ -z "$LATEST_STABLE" ] && [ -z "$LATEST_PRERELEASE" ]; then
        if command -v curl >/dev/null 2>&1; then
            LATEST_JSON=$(curl -s "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null || true)
        else
            LATEST_JSON=$(wget -qO- "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null || true)
        fi
        LATEST_STABLE=$(echo "$LATEST_JSON" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' | head -n1)
    fi

    if [ -z "$LATEST_STABLE" ]; then
        LATEST_STABLE="$FALLBACK_TAG"
    fi

    # If a pre-release is detected, ask user for Stable vs Beta (Nightly)
    if [ -n "$LATEST_PRERELEASE" ]; then
        if [ "$CHANNEL" = "beta" ] || [ "$CHANNEL" = "nightly" ]; then
            TARGET_TAG="$LATEST_PRERELEASE"
            info "Pre-release detected. Selected Beta/Nightly channel: $TARGET_TAG"
        elif [ "$CHANNEL" = "stable" ]; then
            TARGET_TAG="$LATEST_STABLE"
            info "Selected Stable channel: $TARGET_TAG"
        else
            echo ""
            info "Pre-release detected on GitHub!"
            echo "  [1] Stable:       ${LATEST_STABLE} (Recommended)"
            echo "  [2] Beta/Nightly: ${LATEST_PRERELEASE}"
            echo ""
            prompt_user "Which version would you like to install? [1=Stable, 2=Beta] (Default: 1): " "1"
            case "$PROMPT_REPLY" in
                2|b|B|beta|Beta|nightly|Nightly)
                    TARGET_TAG="$LATEST_PRERELEASE"
                    info "Selected Beta/Nightly release channel ($TARGET_TAG)."
                    ;;
                *)
                    TARGET_TAG="$LATEST_STABLE"
                    info "Selected Stable release channel ($TARGET_TAG)."
                    ;;
            esac
            echo ""
        fi
    else
        TARGET_TAG="$LATEST_STABLE"
    fi
fi

# Ensure TARGET_TAG begins with 'v'
if ! echo "$TARGET_TAG" | grep -q '^v'; then
    TARGET_TAG="v$TARGET_TAG"
fi

# 6. Existing installation check & prompt
if [ -n "$EXISTING_BIN" ]; then
    echo ""
    info "Found existing KernelView installation at: $EXISTING_BIN"
    if [ -n "$INSTALLED_VERSION" ]; then
        info "Currently installed version: $INSTALLED_VERSION"
    fi
    info "Latest available version:    $TARGET_TAG"
    echo ""

    if [ "$INSTALLED_VERSION" = "$TARGET_TAG" ] && [ "$FORCE" != "true" ]; then
        info "KernelView is already at the latest version ($TARGET_TAG)."
        prompt_user "Would you like to reinstall / overwrite anyway? [y/N]: " "n"
        case "$PROMPT_REPLY" in
            y|Y|yes|Yes)
                info "Proceeding with reinstall..."
                ;;
            *)
                success "No changes made. You're all set with KernelView $INSTALLED_VERSION!"
                exit 0
                ;;
        esac
    elif [ "$FORCE" != "true" ]; then
        prompt_user "Would you like to update KernelView to latest ($TARGET_TAG)? [Y/n]: " "y"
        case "$PROMPT_REPLY" in
            y|Y|yes|Yes|"")
                info "Updating KernelView to $TARGET_TAG..."
                ;;
            *)
                info "Update cancelled by user. Existing version remains unchanged."
                exit 0
                ;;
        esac
    fi
fi

# 7. Determine installation directory
TARGET_DIR=""
USE_SUDO=false

if [ -n "$CUSTOM_DIR" ]; then
    TARGET_DIR="$CUSTOM_DIR"
elif [ -n "$EXISTING_BIN" ]; then
    # Update in-place in existing binary's folder
    TARGET_DIR="$(dirname "$EXISTING_BIN")"
elif [ "$IS_TERMUX" = "true" ]; then
    TARGET_DIR="${PREFIX}/bin"
elif [ "$FORCE_USER" = "true" ]; then
    TARGET_DIR="$HOME/.local/bin"
elif [ "$(id -u)" -eq 0 ]; then
    TARGET_DIR="/usr/local/bin"
elif [ -w "/usr/local/bin" ]; then
    TARGET_DIR="/usr/local/bin"
elif command -v sudo >/dev/null 2>&1; then
    TARGET_DIR="/usr/local/bin"
    USE_SUDO=true
else
    TARGET_DIR="$HOME/.local/bin"
fi

# Check permissions for target directory
if [ ! -d "$TARGET_DIR" ]; then
    if [ "$USE_SUDO" = "true" ]; then
        sudo mkdir -p "$TARGET_DIR"
    else
        mkdir -p "$TARGET_DIR"
    fi
fi

if [ ! -w "$TARGET_DIR" ] && [ "$USE_SUDO" = "false" ] && [ "$(id -u)" -ne 0 ]; then
    if command -v sudo >/dev/null 2>&1; then
        USE_SUDO=true
    else
        # Fallback to user-local directory
        TARGET_DIR="$HOME/.local/bin"
        mkdir -p "$TARGET_DIR"
    fi
fi

# 8. Download and Extract Release
TAG_CLEAN=$(echo "$TARGET_TAG" | sed 's/^v//')
ARCHIVE_NAME="kernelview_${TAG_CLEAN}_${OS}_${ARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/$REPO/releases/download/${TARGET_TAG}/${ARCHIVE_NAME}"

TMP_DIR="$(mktemp -d 2>/dev/null || mktemp -d -t 'kernelview')"
cleanup() {
    rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

info "Downloading KernelView $TARGET_TAG ($OS/$ARCH)..."
DOWNLOAD_OK=false

if command -v curl >/dev/null 2>&1; then
    if curl -fSL "$DOWNLOAD_URL" -o "$TMP_DIR/$ARCHIVE_NAME" 2>/dev/null; then
        DOWNLOAD_OK=true
    fi
elif command -v wget >/dev/null 2>&1; then
    if wget -q "$DOWNLOAD_URL" -O "$TMP_DIR/$ARCHIVE_NAME" 2>/dev/null; then
        DOWNLOAD_OK=true
    fi
fi

# Fallback to Go build/install if prebuilt binary is not found for this architecture
if [ "$DOWNLOAD_OK" = "false" ]; then
    warn "Prebuilt archive not found or download failed for $OS/$ARCH."
    if command -v go >/dev/null 2>&1; then
        info "Found Go compiler. Building and installing KernelView from source..."
        GOBIN="$TARGET_DIR" go install "github.com/$REPO@$TARGET_TAG" || GOBIN="$TARGET_DIR" go install "github.com/$REPO@latest"
        success "KernelView built and installed successfully to $TARGET_DIR/$BINARY_NAME"
        exit 0
    else
        error "Could not download prebuilt binary ($DOWNLOAD_URL) and Go compiler is not installed."
        error "Please install Go to build from source or check release assets at:"
        error "https://github.com/$REPO/releases/tag/$TARGET_TAG"
        exit 1
    fi
fi

# Unpack archive
info "Extracting binary..."
tar -xzf "$TMP_DIR/$ARCHIVE_NAME" -C "$TMP_DIR"

if [ ! -f "$TMP_DIR/$BINARY_NAME" ]; then
    # Look for binary recursively inside extracted files
    FOUND_BIN="$(find "$TMP_DIR" -type f -name "$BINARY_NAME" | head -n1)"
    if [ -n "$FOUND_BIN" ]; then
        mv "$FOUND_BIN" "$TMP_DIR/$BINARY_NAME"
    else
        error "Could not find '$BINARY_NAME' executable inside downloaded archive."
        exit 1
    fi
fi

chmod +x "$TMP_DIR/$BINARY_NAME"

# 9. Install to Target Directory
info "Installing to $TARGET_DIR/$BINARY_NAME..."
if [ "$USE_SUDO" = "true" ]; then
    sudo cp -f "$TMP_DIR/$BINARY_NAME" "$TARGET_DIR/$BINARY_NAME"
    sudo chmod +x "$TARGET_DIR/$BINARY_NAME"
else
    cp -f "$TMP_DIR/$BINARY_NAME" "$TARGET_DIR/$BINARY_NAME"
    chmod +x "$TARGET_DIR/$BINARY_NAME"
fi

# 10. Verify Installation
INSTALLED_BIN="$TARGET_DIR/$BINARY_NAME"
if [ -x "$INSTALLED_BIN" ]; then
    VER_OUTPUT="$("$INSTALLED_BIN" -v 2>/dev/null || "$INSTALLED_BIN" --version 2>/dev/null || echo "$TARGET_TAG")"
    echo ""
    success "KernelView ($VER_OUTPUT) installed successfully!"
else
    error "Installation verification failed. Executable not found at $INSTALLED_BIN"
    exit 1
fi

# 11. Check PATH
case ":$PATH:" in
    *:"$TARGET_DIR":*)
        ;;
    *)
        echo ""
        warn "$TARGET_DIR is not currently in your PATH environment variable."
        echo "To run 'kernelview' directly from any terminal, add it to your shell configuration:"
        if [ -f "$HOME/.zshrc" ]; then
            echo "  echo 'export PATH=\"\$PATH:$TARGET_DIR\"' >> ~/.zshrc && source ~/.zshrc"
        elif [ -f "$HOME/.bashrc" ]; then
            echo "  echo 'export PATH=\"\$PATH:$TARGET_DIR\"' >> ~/.bashrc && source ~/.bashrc"
        else
            echo "  export PATH=\"\$PATH:$TARGET_DIR\""
        fi
        ;;
esac

echo ""
printf "%bQuick start:%b Run %bkernelview%b in your terminal.\n" "$BOLD" "$RESET" "$GREEN" "$RESET"
printf "%bLive mode:  %b Run %bkernelview -l%b for real-time monitoring.\n" "$BOLD" "$RESET" "$GREEN" "$RESET"
