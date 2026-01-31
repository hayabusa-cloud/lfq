# Makefile for lfq with intrinsics-customized Go compiler
#
# lfq requires the intrinsics compiler for atomix operations
# to compile as single CPU instructions instead of function calls.
#
# Prerequisites:
#   - curl (for downloading pre-built compiler)
#   - tar (for extracting)
#   - Linux (or WSL2 on Windows) / macOS
#
# Usage:
#   make install-compiler        # Install latest release (recommended)
#   make install-compiler-source # Build from source (bleeding-edge)
#   make build                   # Build lfq with intrinsics compiler
#   make test                    # Test lfq with intrinsics compiler
#   make bench                   # Run benchmarks

# Configuration
COMPILER_REPO    := https://github.com/hayabusa-cloud/go
COMPILER_BRANCH  := atomix
COMPILER_DIR     := $(HOME)/sdk/go-atomix
GOROOT_INTRINSIC := $(COMPILER_DIR)
GO_INTRINSIC     := $(GOROOT_INTRINSIC)/bin/go

# Auto-detect OS/ARCH for pre-built binaries
GOOS_LOCAL   := $(shell uname -s | tr '[:upper:]' '[:lower:]')
GOARCH_LOCAL := $(shell uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/')

# Default target
.DEFAULT_GOAL := build

# Compiler check macro
define require-compiler
	@if [ ! -x "$(GO_INTRINSIC)" ]; then \
		echo "Error: Intrinsics compiler not found at $(GO_INTRINSIC)"; \
		echo "Run 'make install-compiler' first."; \
		exit 1; \
	fi
endef

# ============================================================================
# Compiler Installation (Pre-built Release - Recommended)
# ============================================================================

.PHONY: install-compiler
install-compiler:
	@echo "Fetching latest release info..."
	$(eval LATEST_TAG := $(shell curl -fsSL https://api.github.com/repos/hayabusa-cloud/go/releases/latest | grep '"tag_name"' | sed 's/.*: "\(.*\)",/\1/'))
	$(eval VERSION := $(shell echo $(LATEST_TAG) | sed 's/-atomix//'))
	$(eval TARBALL := $(VERSION).$(GOOS_LOCAL)-$(GOARCH_LOCAL).tar.gz)
	$(eval DOWNLOAD_URL := $(COMPILER_REPO)/releases/download/$(LATEST_TAG)/$(TARBALL))
	@echo "Downloading intrinsics compiler $(LATEST_TAG)..."
	@echo "  URL: $(DOWNLOAD_URL)"
	@rm -rf $(COMPILER_DIR)
	@mkdir -p $(dir $(COMPILER_DIR))
	@curl -fsSL $(DOWNLOAD_URL) | tar -xz -C $(dir $(COMPILER_DIR))
	@mv $(dir $(COMPILER_DIR))/go $(COMPILER_DIR)
	@echo "Intrinsics compiler ready at $(GOROOT_INTRINSIC)"
	@$(GO_INTRINSIC) version

# ============================================================================
# Compiler Installation (Source Build - Bleeding Edge)
# ============================================================================

.PHONY: install-compiler-source
install-compiler-source:
	@if [ -d "$(COMPILER_DIR)/.git" ]; then \
		echo "Updating intrinsics compiler from source..."; \
		cd $(COMPILER_DIR) && git fetch origin && git checkout $(COMPILER_BRANCH) && git pull origin $(COMPILER_BRANCH); \
	elif [ -d "$(COMPILER_DIR)" ]; then \
		echo "Error: $(COMPILER_DIR) exists but is not a git repository."; \
		echo "This may be from a previous 'make install-compiler' (pre-built release)."; \
		echo "Please remove it first, then rerun:"; \
		echo "  rm -rf $(COMPILER_DIR)"; \
		echo "  make install-compiler-source"; \
		exit 1; \
	else \
		echo "Cloning intrinsics compiler..."; \
		mkdir -p $(dir $(COMPILER_DIR)); \
		git clone --branch $(COMPILER_BRANCH) $(COMPILER_REPO).git $(COMPILER_DIR); \
	fi
	@echo "Building compiler (this may take several minutes)..."
	cd $(COMPILER_DIR)/src && ./make.bash
	@echo "Intrinsics compiler ready at $(GOROOT_INTRINSIC)"

# ============================================================================
# Build & Test with Intrinsics Compiler
# ============================================================================

.PHONY: build
build:
	$(require-compiler)
	@echo "Building lfq with intrinsics compiler..."
	GOROOT=$(GOROOT_INTRINSIC) $(GO_INTRINSIC) vet ./...
	GOROOT=$(GOROOT_INTRINSIC) $(GO_INTRINSIC) build ./...
	@echo "Build successful"

.PHONY: test
test:
	$(require-compiler)
	@echo "Testing lfq with intrinsics compiler..."
	GOROOT=$(GOROOT_INTRINSIC) $(GO_INTRINSIC) test -v -covermode=atomic -coverprofile=coverage.out ./...
	@echo "Tests passed"

.PHONY: bench
bench:
	$(require-compiler)
	@echo "Running benchmarks with intrinsics compiler..."
	GOROOT=$(GOROOT_INTRINSIC) $(GO_INTRINSIC) test -bench=. -benchmem ./...

# ============================================================================
# Cross-Compilation (for remote hardware testing)
# ============================================================================

.PHONY: test-binary-linux-amd64
test-binary-linux-amd64:
	$(require-compiler)
	@echo "Building test binary for linux/amd64..."
	GOROOT=$(GOROOT_INTRINSIC) GOOS=linux GOARCH=amd64 $(GO_INTRINSIC) test -c -o lfq-linux-amd64.test .
	@echo "Test binary ready: lfq-linux-amd64.test"

.PHONY: test-binary-linux-arm64
test-binary-linux-arm64:
	$(require-compiler)
	@echo "Building test binary for linux/arm64..."
	GOROOT=$(GOROOT_INTRINSIC) GOOS=linux GOARCH=arm64 $(GO_INTRINSIC) test -c -o lfq-linux-arm64.test .
	@echo "Test binary ready: lfq-linux-arm64.test"

.PHONY: test-binary-darwin-arm64
test-binary-darwin-arm64:
	$(require-compiler)
	@echo "Building test binary for darwin/arm64..."
	GOROOT=$(GOROOT_INTRINSIC) GOOS=darwin GOARCH=arm64 $(GO_INTRINSIC) test -c -o lfq-darwin-arm64.test .
	@echo "Test binary ready: lfq-darwin-arm64.test"

# ============================================================================
# Utilities
# ============================================================================

.PHONY: clean
clean:
	rm -f coverage.out
	rm -f lfq.test lfq-linux-amd64.test lfq-linux-arm64.test lfq-darwin-arm64.test
	GOROOT=$(GOROOT_INTRINSIC) $(GO_INTRINSIC) clean -cache 2>/dev/null || true

.PHONY: check-compiler
check-compiler:
	@if [ -x "$(GO_INTRINSIC)" ]; then \
		echo "Intrinsics compiler found: $(GO_INTRINSIC)"; \
		$(GO_INTRINSIC) version; \
	else \
		echo "Intrinsics compiler NOT found at $(GO_INTRINSIC)"; \
		exit 1; \
	fi

.PHONY: help
help:
	@echo "lfq Makefile - Build with intrinsics-customized Go compiler"
	@echo ""
	@echo "Compiler:"
	@echo "  install-compiler        Download latest release (recommended)"
	@echo "  install-compiler-source Build from source (bleeding-edge)"
	@echo "  check-compiler          Verify compiler installation"
	@echo ""
	@echo "Build:"
	@echo "  build                   Build with intrinsics compiler (includes vet)"
	@echo "  test                    Run tests with coverage"
	@echo "  bench                   Run benchmarks"
	@echo ""
	@echo "Cross-Compilation:"
	@echo "  test-binary-linux-amd64   Build test binary for linux/amd64"
	@echo "  test-binary-linux-arm64   Build test binary for linux/arm64"
	@echo "  test-binary-darwin-arm64  Build test binary for darwin/arm64"
	@echo ""
	@echo "Other:"
	@echo "  clean                   Remove build artifacts"
	@echo "  help                    Show this help"
	@echo ""
	@echo "Configuration:"
	@echo "  COMPILER_DIR     = $(COMPILER_DIR)"
	@echo "  GO_INTRINSIC     = $(GO_INTRINSIC)"
	@echo "  GOOS_LOCAL       = $(GOOS_LOCAL)"
	@echo "  GOARCH_LOCAL     = $(GOARCH_LOCAL)"
