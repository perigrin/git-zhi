#!/bin/sh
# ABOUTME: Platform-detecting installer for git-zhi. Downloads the correct
# ABOUTME: binary from GitHub releases, installs it, and creates companion symlinks.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/perigrin/git-zhi/pu/install.sh | sh
#   curl -fsSL ... | sh -s -- --version v0.3.0
#   curl -fsSL ... | sh -s -- --install-dir /usr/local/bin

set -e

REPO="perigrin/git-zhi"
INSTALL_DIR="${HOME}/.local/bin"
VERSION=""

# Parse arguments.
while [ $# -gt 0 ]; do
    case "$1" in
        --version)    VERSION="$2"; shift 2 ;;
        --install-dir) INSTALL_DIR="$2"; shift 2 ;;
        --help)
            echo "Usage: install.sh [--version VERSION] [--install-dir DIR]"
            echo ""
            echo "Downloads and installs the git-zhi binary and creates companion symlinks."
            echo ""
            echo "Options:"
            echo "  --version      Specific version to install (default: latest)"
            echo "  --install-dir  Directory to install into (default: ~/.local/bin)"
            exit 0
            ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

# Detect platform.
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
    x86_64)  ARCH="amd64" ;;
    aarch64) ARCH="arm64" ;;
    arm64)   ARCH="arm64" ;;
    *)
        echo "Error: unsupported architecture: $ARCH"
        exit 1
        ;;
esac

case "$OS" in
    linux|darwin) ;;
    mingw*|msys*|cygwin*)
        echo "Error: Windows detected. Download the .zip from:"
        echo "  https://github.com/${REPO}/releases"
        exit 1
        ;;
    *)
        echo "Error: unsupported operating system: $OS"
        exit 1
        ;;
esac

# Resolve version.
if [ -z "$VERSION" ]; then
    VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\(.*\)".*/\1/')"
    if [ -z "$VERSION" ]; then
        echo "Error: could not determine latest version"
        exit 1
    fi
fi

echo "Installing git-zhi ${VERSION} for ${OS}/${ARCH}..."

# Build asset URL.
ASSET="git-zhi-${VERSION#v}-${OS}-${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET}"

# Download and extract.
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

echo "Downloading ${URL}..."
curl -fsSL "$URL" -o "${TMPDIR}/${ASSET}"

echo "Extracting..."
tar xzf "${TMPDIR}/${ASSET}" -C "$TMPDIR"

# Find the binary in the tarball (named git-zhi-{os}-{arch}).
BINARY_NAME="git-zhi-${OS}-${ARCH}"
if [ ! -f "${TMPDIR}/${BINARY_NAME}" ]; then
    # Fallback: look for any git-zhi* binary.
    BINARY_NAME="$(ls "${TMPDIR}" | grep '^git-zhi' | head -1)"
    if [ -z "$BINARY_NAME" ]; then
        echo "Error: no git-zhi binary found in archive"
        exit 1
    fi
fi

# Install.
mkdir -p "$INSTALL_DIR"
cp "${TMPDIR}/${BINARY_NAME}" "${INSTALL_DIR}/git-zhi"
chmod +x "${INSTALL_DIR}/git-zhi"

echo "Installed git-zhi to ${INSTALL_DIR}/git-zhi"

# Create companion symlinks.
"${INSTALL_DIR}/git-zhi" setup

# Check PATH.
case ":${PATH}:" in
    *":${INSTALL_DIR}:"*) ;;
    *)
        echo ""
        echo "Warning: ${INSTALL_DIR} is not in your PATH."
        echo "Add it with:"
        echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
        echo ""
        echo "Or add to your shell profile (~/.bashrc, ~/.zshrc, etc.)."
        ;;
esac

echo ""
echo "git-zhi ${VERSION} installed successfully."
echo "Run 'git zhi' to get started."
