.PHONY: build build-all test lint coverage golangci clean help release install

# Version from git tag or default
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BINARY_NAME := vibe
DIST_DIR := dist
CMD_PATH := ./cmd/vibe

# Target platforms
PLATFORMS := \
	darwin/arm64 \
	darwin/amd64 \
	linux/amd64 \
	linux/arm64 \
	linux/riscv64 \
	windows/amd64

# Color output
RED := \033[0;31m
GREEN := \033[0;32m
YELLOW := \033[1;33m
BLUE := \033[0;34m
NC := \033[0m # No Color

help:
	@echo "$(BLUE)vibe-local-go - Makefile$(NC)"
	@echo ""
	@echo "$(YELLOW)Usage:$(NC)"
	@echo "  make build             - Build for current OS/Arch"
	@echo "  make build-all         - Cross-compile for all platforms (6 targets)"
	@echo "  make release           - Build + compress all platforms (tar.gz/zip)"
	@echo "  make test              - Run unit tests with race detector"
	@echo "  make coverage          - Run tests with coverage report"
	@echo "  make lint              - Run linter (go vet + golangci-lint)"
	@echo "  make golangci          - Run golangci-lint only (install if missing)"
	@echo "  make clean             - Clean dist/ directory"
	@echo "  make install           - Build and install to ~/.local/bin/"
	@echo "  make help              - Show this help"
	@echo ""
	@echo "$(YELLOW)Supported platforms:$(NC)"
	@echo "  • darwin/arm64         - macOS Apple Silicon (M1/M2/M3)"
	@echo "  • darwin/amd64         - macOS Intel"
	@echo "  • linux/amd64          - Linux x86_64"
	@echo "  • linux/arm64          - Linux ARM64 (Raspberry Pi)"
	@echo "  • linux/riscv64        - Linux RISC-V"
	@echo "  • windows/amd64        - Windows"
	@echo ""
	@echo "$(YELLOW)Examples:$(NC)"
	@echo "  make build             # Build for your system"
	@echo "  make build-all         # Build all 6 platforms"
	@echo "  make release           # Build + compress (ready for GitHub Release)"
	@echo ""

# === Local Build ===

build: clean
	@mkdir -p $(DIST_DIR)
	@echo "$(BLUE)📦 Building $(BINARY_NAME) for local system...$(NC)"
	@echo "   Version: $(VERSION)"
	@go build \
		-ldflags "-X main.Version=$(VERSION)" \
		-o $(DIST_DIR)/$(BINARY_NAME) \
		$(CMD_PATH)
	@echo "$(GREEN)✅ Built: $(DIST_DIR)/$(BINARY_NAME)$(NC)"
	@$(DIST_DIR)/$(BINARY_NAME) --version

# === Cross-platform Build ===

build-all: clean
	@mkdir -p $(DIST_DIR)
	@echo "$(BLUE)🌍 Cross-compiling for all platforms...$(NC)"
	@echo "   Version: $(VERSION)"
	@echo ""
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; \
		arch=$${platform##*/}; \
		output=$(DIST_DIR)/$(BINARY_NAME)-$${os}-$${arch}; \
		[ "$$os" = "windows" ] && output=$${output}.exe; \
		echo "$(YELLOW)  ⚙️  Building $${os}/$${arch}...$(NC)"; \
		GOOS=$$os GOARCH=$$arch go build \
			-ldflags "-X main.Version=$(VERSION)" \
			-o $$output \
			$(CMD_PATH) || { echo "$(RED)❌ Failed: $${os}/$${arch}$(NC)"; exit 1; }; \
		[ "$$os" != "windows" ] && chmod +x $$output; \
		stat_output=$$(stat -f%z "$$output" 2>/dev/null || stat -c%s "$$output" 2>/dev/null); \
		printf "     ✅ %-25s %s\n" "$${os}/$${arch}" "$${stat_output} bytes"; \
	done
	@echo ""
	@echo "$(GREEN)✅ All binaries built in $(DIST_DIR)/:$(NC)"
	@ls -lh $(DIST_DIR)/ | grep vibe-

# === Release (Compress) ===

release: build-all
	@echo ""
	@echo "$(BLUE)📦 Creating release artifacts...$(NC)"
	@cd $(DIST_DIR) && \
	for file in $(BINARY_NAME)-*; do \
		if [ -f "$$file" ]; then \
			echo "   Compressing $$file..."; \
			case "$$file" in \
				*.exe) \
					zip -q "$${file%.exe}.zip" "$$file"; \
					rm "$$file"; \
					echo "     → $${file%.exe}.zip"; \
					;; \
				*) \
					tar czf "$${file}.tar.gz" "$$file"; \
					rm "$$file"; \
					echo "     → $${file}.tar.gz"; \
					;; \
			esac; \
		fi; \
	done
	@echo ""
	@echo "$(GREEN)✅ Release artifacts created:$(NC)"
	@ls -lh $(DIST_DIR)/ | grep -E "\.tar\.gz|\.zip" | awk '{print "     " $$9 " (" $$5 ")"}'

# === Testing ===

test:
	@echo "$(BLUE)🧪 Running tests...$(NC)"
	@go test -v -race -timeout 30s ./...
	@echo "$(GREEN)✅ Tests passed$(NC)"

coverage:
	@echo "$(BLUE)📊 Running tests with coverage...$(NC)"
	@mkdir -p $(DIST_DIR)
	@go test -race -timeout 30s -coverprofile=$(DIST_DIR)/coverage.out ./...
	@go tool cover -func=$(DIST_DIR)/coverage.out | tail -1
	@go tool cover -html=$(DIST_DIR)/coverage.out -o $(DIST_DIR)/coverage.html
	@echo "$(GREEN)✅ Coverage report: $(DIST_DIR)/coverage.html$(NC)"

# === Linting ===

lint:
	@echo "$(BLUE)🔍 Running linter...$(NC)"
	@echo "   go vet..."
	@go vet ./...
	@$(MAKE) golangci
	@echo "$(GREEN)✅ Linting passed$(NC)"

golangci:
	@echo "$(BLUE)🔍 Running golangci-lint...$(NC)"
	@if ! command -v golangci-lint &>/dev/null; then \
		echo "   $(YELLOW)Installing golangci-lint...$(NC)"; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
			| sh -s -- -b $$(go env GOPATH)/bin latest; \
	fi
	@golangci-lint run ./...
	@echo "$(GREEN)✅ golangci-lint passed$(NC)"

# === Installation ===

install: build
	@echo "$(BLUE)📥 Installing to ~/.local/bin/$(NC)"
	@mkdir -p $$HOME/.local/bin
	@cp $(DIST_DIR)/$(BINARY_NAME) $$HOME/.local/bin/$(BINARY_NAME)
	@chmod +x $$HOME/.local/bin/$(BINARY_NAME)
	@echo "$(GREEN)✅ Installed: $$HOME/.local/bin/$(BINARY_NAME)$(NC)"
	@if ! echo "$$PATH" | grep -q "$$HOME/.local/bin"; then \
		echo "$(YELLOW)⚠️  ~/.local/bin not in PATH$(NC)"; \
		echo "   Add to your shell profile:"; \
		echo "   export PATH=\"\$$HOME/.local/bin:\$$PATH\""; \
	fi

# === Cleaning ===

clean:
	@echo "$(BLUE)🧹 Cleaning dist/...$(NC)"
	@rm -rf $(DIST_DIR)
	@echo "$(GREEN)✅ Cleaned$(NC)"

# === Development Helpers ===

fmt:
	@echo "$(BLUE)🎨 Formatting code...$(NC)"
	@go fmt ./...
	@echo "$(GREEN)✅ Formatted$(NC)"

run:
	@$(DIST_DIR)/$(BINARY_NAME)

version:
	@echo "Version: $(VERSION)"

info:
	@echo "$(BLUE)Project Information$(NC)"
	@echo "  Binary name: $(BINARY_NAME)"
	@echo "  Version: $(VERSION)"
	@echo "  Main package: $(CMD_PATH)"
	@echo "  Output dir: $(DIST_DIR)"
	@echo "  Platforms: $(PLATFORMS)"
