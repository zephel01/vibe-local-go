# Go版 vibe-local-go デプロイメント実装ガイド

## 📋 実装予定項目

### ✅ 優先順位 1: Makefile（クロスプラットフォームビルド）

```makefile
# Makefile
.PHONY: build build-all test lint clean help

VERSION := 1.0.0
BINARY_NAME := vibe
DIST_DIR := dist

# Target architectures
TARGETS := \
	darwin/arm64 \
	darwin/amd64 \
	linux/amd64 \
	linux/arm64 \
	linux/riscv64 \
	windows/amd64

help:
	@echo "Usage:"
	@echo "  make build           - Build for current OS/Arch"
	@echo "  make build-all       - Cross-compile for all platforms"
	@echo "  make test            - Run unit tests"
	@echo "  make lint            - Run linter"
	@echo "  make clean           - Clean dist/"
	@echo "  make release         - Build + compress all platforms"

build:
	@mkdir -p $(DIST_DIR)
	go build -ldflags "-X main.Version=$(VERSION)" -o $(DIST_DIR)/$(BINARY_NAME) ./cmd/vibe
	@echo "✅ Built: $(DIST_DIR)/$(BINARY_NAME)"

build-all:
	@mkdir -p $(DIST_DIR)
	@for target in $(TARGETS); do \
		os=$${target%/*}; \
		arch=$${target##*/}; \
		output=$(DIST_DIR)/$(BINARY_NAME)-$${os}-$${arch}; \
		[ "$$os" = "windows" ] && output=$${output}.exe; \
		echo "📦 Building $${os}/$${arch}..."; \
		GOOS=$$os GOARCH=$$arch go build \
			-ldflags "-X main.Version=$(VERSION)" \
			-o $$output ./cmd/vibe; \
		[ "$$os" != "windows" ] && chmod +x $$output; \
	done
	@echo "✅ All binaries built in $(DIST_DIR)/"

test:
	go test -v -race ./...

lint:
	go vet ./...
	staticcheck ./... 2>/dev/null || echo "staticcheck not installed"

clean:
	rm -rf $(DIST_DIR)
	@echo "✅ Cleaned $(DIST_DIR)/"

release: build-all
	@cd $(DIST_DIR) && \
	for file in *; do \
		echo "📦 Compressing $$file..."; \
		case "$$file" in \
			*.exe) zip -q "$${file%.exe}.zip" "$$file" ;; \
			*) tar czf "$${file}.tar.gz" "$$file" ;; \
		esac; \
	done
	@echo "✅ Release artifacts created in $(DIST_DIR)/"
	@ls -lh $(DIST_DIR)/ | grep -E "\.tar\.gz|\.zip|vibe-"
```

**実装内容**:
- `make build` - ローカルOS向けビルド
- `make build-all` - 6プラットフォーム対応
  - darwin/arm64 (Apple Silicon)
  - darwin/amd64 (Intel Mac)
  - linux/amd64 (標準Linux)
  - linux/arm64 (ARM64サーバー・Raspberry Pi)
  - linux/riscv64 (RISC-V対応)
  - windows/amd64
- `make release` - 圧縮ファイル生成 (tar.gz / zip)

**推定実装**: 50-60行

---

### ✅ 優先順位 2: GitHub Actions - 自動リリース

**`.github/workflows/release.yml`**:

```yaml
name: Build & Release

on:
  push:
    tags:
      - 'v*'

jobs:
  build:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        include:
          - { os: darwin, arch: arm64, name: darwin-arm64 }
          - { os: darwin, arch: amd64, name: darwin-amd64 }
          - { os: linux, arch: amd64, name: linux-amd64 }
          - { os: linux, arch: arm64, name: linux-arm64 }
          - { os: linux, arch: riscv64, name: linux-riscv64 }
          - { os: windows, arch: amd64, name: windows-amd64 }

    steps:
      - uses: actions/checkout@v3

      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Build ${{ matrix.name }}
        run: |
          mkdir -p dist
          GOOS=${{ matrix.os }} GOARCH=${{ matrix.arch }} \
          go build -ldflags "-X main.Version=${{ github.ref_name }}" \
          -o dist/vibe-${{ matrix.name }}${{ matrix.os == 'windows' && '.exe' || '' }} \
          ./cmd/vibe
          [ "${{ matrix.os }}" != "windows" ] && chmod +x dist/vibe-${{ matrix.name }}

      - name: Compress
        run: |
          cd dist
          if [ "${{ matrix.os }}" = "windows" ]; then
            zip -q vibe-${{ matrix.name }}.zip vibe-${{ matrix.name }}.exe
            rm vibe-${{ matrix.name }}.exe
          else
            tar czf vibe-${{ matrix.name }}.tar.gz vibe-${{ matrix.name }}
            rm vibe-${{ matrix.name }}
          fi

      - uses: actions/upload-artifact@v3
        with:
          name: vibe-${{ matrix.name }}
          path: dist/

  release:
    needs: build
    runs-on: ubuntu-latest
    steps:
      - uses: actions/download-artifact@v3

      - name: Create Release
        uses: softprops/action-gh-release@v1
        with:
          files: |
            **/vibe-*.tar.gz
            **/vibe-*.zip
          draft: false
          prerelease: false
```

**実装内容**:
- `v1.0.0` タグ push → 自動ビルド・リリース
- 6プラットフォーム並列ビルド
- GitHub Releases に自動アップロード
- tar.gz / zip で自動圧縮

**推定実装**: 60-80行

---

### ✅ 優先順位 3: Go版インストーラ (install.sh - 軽量版)

**`scripts/install-go.sh`**:

```bash
#!/bin/bash
# vibe-local-go インストーラ (Go版向け)
# 使用: curl -fsSL https://raw.githubusercontent.com/zephel01/vibe-local-go/main/scripts/install-go.sh | bash

set -euo pipefail

# Color codes
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}🚀 vibe-local-go インストーラ${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

# Step 1: Detect OS/Arch
OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS" in
  Darwin)
    [ "$ARCH" = "arm64" ] && TARGET="darwin-arm64" || TARGET="darwin-amd64"
    BIN_DIR="${HOME}/.local/bin"
    ;;
  Linux)
    case "$ARCH" in
      x86_64) TARGET="linux-amd64" ;;
      aarch64) TARGET="linux-arm64" ;;
      riscv64) TARGET="linux-riscv64" ;;
      *) echo "❌ Unsupported architecture: $ARCH"; exit 1 ;;
    esac
    BIN_DIR="${HOME}/.local/bin"
    ;;
  MINGW*|MSYS*|CYGWIN*)
    echo "❌ Windows detected. Use: https://github.com/zephel01/vibe-local-go/releases"
    exit 1
    ;;
  *)
    echo "❌ Unsupported OS: $OS"
    exit 1
    ;;
esac

echo "✅ OS: $OS / Arch: $ARCH → Target: $TARGET"
echo ""

# Step 2: Get latest release version
echo "📡 Fetching latest release..."
VERSION=$(curl -s https://api.github.com/repos/zephel01/vibe-local-go/releases/latest | grep tag_name | cut -d'"' -f4)

if [ -z "$VERSION" ]; then
  echo "❌ Failed to get latest version"
  exit 1
fi

echo "📦 Latest version: $VERSION"
echo ""

# Step 3: Download binary
mkdir -p "$BIN_DIR"

DOWNLOAD_URL="https://github.com/zephel01/vibe-local-go/releases/download/${VERSION}/vibe-${TARGET}.tar.gz"

echo "⬇️  Downloading: $DOWNLOAD_URL"
if curl -fsSL "$DOWNLOAD_URL" | tar xz -C "$BIN_DIR"; then
  chmod +x "$BIN_DIR/vibe-${TARGET}"
  ln -sf "$BIN_DIR/vibe-${TARGET}" "$BIN_DIR/vibe"
  echo "✅ Downloaded and installed"
else
  echo "❌ Download failed"
  exit 1
fi

echo ""

# Step 4: Add to PATH
if ! echo "$PATH" | grep -q "$BIN_DIR"; then
  SHELL_RC=""
  [ -f "$HOME/.zshrc" ] && SHELL_RC="$HOME/.zshrc"
  [ -f "$HOME/.bashrc" ] && SHELL_RC="$HOME/.bashrc"

  if [ -n "$SHELL_RC" ]; then
    echo '' >> "$SHELL_RC"
    echo "# vibe-local-go" >> "$SHELL_RC"
    echo "export PATH=\"${BIN_DIR}:\$PATH\"" >> "$SHELL_RC"
    echo "✅ PATH added to $SHELL_RC"
    echo "   → source $SHELL_RC を実行してください"
  fi
fi

echo ""

# Step 5: Verify installation
if command -v vibe &>/dev/null; then
  echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo -e "${GREEN}✅ インストール完了！${NC}"
  echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo ""
  echo "🚀 使用開始："
  echo "   vibe                 # 対話モード"
  echo "   vibe -p '質問'       # ワンショット"
  echo ""
  vibe --version
else
  echo "⚠️  新しいターミナルを開いてから vibe を実行してください"
fi
```

**実装内容**:
- OS/Arch 自動検出 (6プラットフォーム対応)
- GitHub Release から最新バイナリをダウンロード
- PATH 自動設定
- 5-10秒で完了（Python版の 1/100以下）

**推定実装**: 80-100行

---

### ✅ 優先順位 4: README.md 更新（インストール手順）

現在の README.md に追加する内容：

```markdown
## インストール

### 方法1: バイナリダウンロード（推奨）

```bash
curl -fsSL https://raw.githubusercontent.com/zephel01/vibe-local-go/main/scripts/install-go.sh | bash
```

**所要時間**: 5-10秒
**依存関係**: なし（バイナリ単体）

### 方法2: ソースからビルド

```bash
git clone https://github.com/zephel01/vibe-local-go.git
cd vibe-local-go
make build
./dist/vibe --version
```

**要件**: Go 1.21+

### 方法3: GitHub Releases から手動ダウンロード

https://github.com/zephel101/vibe-local-go/releases

- vibe-darwin-arm64.tar.gz (Apple Silicon)
- vibe-darwin-amd64.tar.gz (Intel Mac)
- vibe-linux-amd64.tar.gz (Linux x86_64)
- vibe-linux-arm64.tar.gz (Linux ARM64)
- vibe-linux-riscv64.tar.gz (RISC-V)
- vibe-windows-amd64.zip (Windows)

## 対応プラットフォーム

| OS | arch | 状態 | 備考 |
|---|------|------|------|
| macOS | arm64 (Apple Silicon) | ✅ | M1/M2/M3 推奨環境 |
| macOS | amd64 (Intel) | ✅ | 動作するが arm64 推奨 |
| Linux | amd64 | ✅ | 最もテスト済み |
| Linux | arm64 | ✅ | Raspberry Pi 4/5 対応 |
| Linux | riscv64 | ✅ | RISC-V ボード対応 |
| Windows | amd64 | ✅ | WSL2 推奨 |
```

---

## 📊 実装タイムライン

| 項目 | 行数 | 難易度 | 実装時間 | 効果 |
|------|------|--------|--------|------|
| **Makefile** | ~60 | ⭐ | 30分 | クロスプラットフォームビルド |
| **GitHub Actions** | ~80 | ⭐⭐ | 45分 | 自動リリース |
| **install-go.sh** | ~100 | ⭐ | 1時間 | ワンコマンドインストール |
| **README.md 更新** | ~50 | ⭐ | 30分 | ドキュメント整備 |
| **合計** | ~290 | ⭐ | 2-3時間 | 完全自動化 |

---

## 🎯 実装後のフロー

```
1. Makefile 完成
   ↓
2. make build-all で 6プラットフォーム対応バイナリ生成
   ↓
3. GitHub Actions CI/CD セットアップ
   ↓
4. v1.0.0 タグ push
   ↓
5. GitHub が自動ビルル・リリース
   ↓
6. install-go.sh が自動的に最新バイナリをダウンロード
   ↓
7. ユーザー: curl | bash でワンコマンドインストール 完了！
```

---

## ✅ チェックリスト

- [ ] Makefile 実装 (6プラットフォーム)
- [ ] `.github/workflows/release.yml` 実装
- [ ] `scripts/install-go.sh` 実装
- [ ] README.md インストール手順更新
- [ ] 各プラットフォームでテスト
- [ ] v1.0.0 タグで初回リリース

---

## 注記

- **Homebrew**: 実装なし（ユーザー要望による）
- **Docker**: 実装なし（ユーザー要望による）
- **RISC-V**: クロスコンパイル対応予定
