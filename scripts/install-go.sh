#!/bin/bash
# vibe-local-go インストーラ (Go版向け)
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/zephel01/vibe-local-go/main/scripts/install-go.sh | bash
#   bash scripts/install-go.sh

set -euo pipefail

# ============================================================================
# Color codes
# ============================================================================

GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
GRAY='\033[1;30m'
NC='\033[0m' # No Color

# ============================================================================
# Helper functions
# ============================================================================

print_header() {
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${GREEN}🚀 vibe-local-go インストーラ${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
}

print_success() {
    echo -e "${GREEN}✅ $@${NC}"
}

print_info() {
    echo -e "${BLUE}💠 $@${NC}"
}

print_warn() {
    echo -e "${YELLOW}⚠️  $@${NC}"
}

print_error() {
    echo -e "${RED}❌ $@${NC}"
}

# ============================================================================
# Step 1: Detect OS/Arch
# ============================================================================

print_header

print_info "Step 1: Detecting OS and Architecture..."
echo ""

OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
    Darwin)
        case "$ARCH" in
            arm64)
                TARGET="darwin-arm64"
                PRETTY_OS="macOS Apple Silicon"
                ;;
            x86_64)
                TARGET="darwin-amd64"
                PRETTY_OS="macOS Intel"
                print_warn "Intel Mac detected - arm64 (Apple Silicon) is recommended"
                ;;
            *)
                print_error "Unsupported architecture: $ARCH"
                exit 1
                ;;
        esac
        BIN_DIR="${HOME}/.local/bin"
        ;;
    Linux)
        case "$ARCH" in
            x86_64)
                TARGET="linux-amd64"
                PRETTY_OS="Linux (x86_64)"
                ;;
            aarch64)
                TARGET="linux-arm64"
                PRETTY_OS="Linux (ARM64)"
                ;;
            riscv64)
                TARGET="linux-riscv64"
                PRETTY_OS="Linux (RISC-V)"
                ;;
            *)
                print_error "Unsupported architecture: $ARCH"
                echo "  Supported: x86_64, aarch64, riscv64"
                exit 1
                ;;
        esac
        BIN_DIR="${HOME}/.local/bin"
        ;;
    MINGW*|MSYS*|CYGWIN*)
        print_error "Windows detected (Git Bash / MSYS2)"
        echo ""
        echo "  On Windows, please use:"
        echo "  1. WSL2 (Windows Subsystem for Linux) - recommended"
        echo "  2. Or download from GitHub Releases:"
        echo "     https://github.com/zephel01/vibe-local-go/releases"
        exit 1
        ;;
    *)
        print_error "Unsupported OS: $OS"
        echo "  Supported: macOS, Linux (WSL2 on Windows)"
        exit 1
        ;;
esac

print_success "OS: $PRETTY_OS"
print_success "Target: $TARGET"
echo ""

# ============================================================================
# Step 2: Get latest release version
# ============================================================================

print_info "Step 2: Fetching latest release..."
echo ""

# Try to get latest release from GitHub API
VERSION=$(curl -s https://api.github.com/repos/zephel01/vibe-local-go/releases/latest 2>/dev/null | \
    grep '"tag_name"' | head -1 | cut -d'"' -f4)

if [ -z "$VERSION" ] || [ "$VERSION" = "null" ]; then
    print_error "Failed to fetch latest version from GitHub"
    echo ""
    echo "  Manual installation:"
    echo "  1. Visit: https://github.com/zephel01/vibe-local-go/releases"
    echo "  2. Download: vibe-${TARGET}.tar.gz (or .zip on Windows)"
    echo "  3. Extract and add to PATH"
    exit 1
fi

print_success "Latest version: $VERSION"
echo ""

# ============================================================================
# Step 3: Download binary
# ============================================================================

print_info "Step 3: Downloading binary..."
echo ""

mkdir -p "$BIN_DIR"

# Determine file extension
if [ "$OS" = "Darwin" ] || [ "$OS" = "Linux" ]; then
    ARCHIVE="vibe-${TARGET}.tar.gz"
else
    ARCHIVE="vibe-${TARGET}.zip"
fi

DOWNLOAD_URL="https://github.com/zephel01/vibe-local-go/releases/download/${VERSION}/${ARCHIVE}"

echo "  From: $DOWNLOAD_URL"
echo "  To:   $BIN_DIR/"
echo ""

# Download with progress
if ! curl -fsSL -o "/tmp/${ARCHIVE}" "$DOWNLOAD_URL"; then
    print_error "Download failed"
    echo ""
    echo "  Possible causes:"
    echo "  1. Network error - check your connection"
    echo "  2. Version doesn't exist - check GitHub releases"
    echo "  3. Platform not available - check supported platforms"
    echo ""
    echo "  Try manual download:"
    echo "  https://github.com/zephel01/vibe-local-go/releases/download/${VERSION}/"
    exit 1
fi

# Extract
cd /tmp
if [ "$OS" = "Darwin" ] || [ "$OS" = "Linux" ]; then
    tar xzf "$ARCHIVE" -O > "$BIN_DIR/vibe"
else
    unzip -q "$ARCHIVE" -d "$BIN_DIR"
fi
rm -f "$ARCHIVE"

chmod +x "$BIN_DIR/vibe"

print_success "Downloaded and installed"
echo ""

# ============================================================================
# Step 4: Add to PATH
# ============================================================================

print_info "Step 4: Configuring PATH..."
echo ""

# Check if already in PATH
if echo "$PATH" | grep -q "$BIN_DIR"; then
    print_success "Already in PATH"
else
    # Detect shell
    SHELL_RC=""
    if [ -f "$HOME/.zshrc" ]; then
        SHELL_RC="$HOME/.zshrc"
    elif [ -f "$HOME/.bashrc" ]; then
        SHELL_RC="$HOME/.bashrc"
    elif [ -f "$HOME/.profile" ]; then
        SHELL_RC="$HOME/.profile"
    fi

    if [ -n "$SHELL_RC" ]; then
        # Check if already added
        if ! grep -q "\.local/bin" "$SHELL_RC"; then
            echo '' >> "$SHELL_RC"
            echo '# vibe-local-go' >> "$SHELL_RC"
            echo "export PATH=\"${BIN_DIR}:\$PATH\"" >> "$SHELL_RC"
            print_success "PATH added to $SHELL_RC"
            print_warn "Run: source $SHELL_RC"
        else
            print_success "Already configured in $SHELL_RC"
        fi
    else
        print_warn "Could not find shell config file"
        print_warn "Add manually: export PATH=\"${BIN_DIR}:\$PATH\""
    fi
fi

echo ""

# ============================================================================
# Step 5: Verify installation
# ============================================================================

print_info "Step 5: Verifying installation..."
echo ""

# Check if vibe is executable
if [ ! -x "$BIN_DIR/vibe" ]; then
    print_error "Installation failed - vibe is not executable"
    exit 1
fi

# Try to run vibe
if "$BIN_DIR/vibe" --version > /dev/null 2>&1; then
    VERSION_OUTPUT=$("$BIN_DIR/vibe" --version 2>&1 || echo "unknown")
    print_success "Installation verified"
    echo "  $VERSION_OUTPUT"
else
    print_warn "Could not verify installation immediately"
    print_warn "Try restarting your terminal or run: source $SHELL_RC"
fi

echo ""

# ============================================================================
# Success message
# ============================================================================

echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}✅ インストール完了！${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

echo -e "${YELLOW}🚀 使用開始:${NC}"
echo ""
echo -e "  ${GREEN}対話モード${NC}"
echo "    vibe"
echo ""
echo -e "  ${GREEN}ワンショット${NC}"
echo "    vibe -p '質問'"
echo ""
echo -e "  ${GREEN}ヘルプ${NC}"
echo "    vibe --help"
echo ""

# Check if vibe is in PATH for current session
if ! command -v vibe &> /dev/null; then
    echo -e "${YELLOW}⚠️  vibe コマンドがまだ PATH に見つかりません${NC}"
    echo ""
    echo "   解決方法:"
    echo "   1. 新しいターミナルウィンドウを開く、または"
    echo "   2. このコマンドを実行:"
    if [ -n "$SHELL_RC" ]; then
        echo "      source $SHELL_RC"
    fi
fi

echo ""
print_success "Happy coding! 🎉"
echo ""
