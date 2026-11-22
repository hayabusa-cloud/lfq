# Makefile for lfq with intrinsics-customized Go compiler
#
# lfq requires the intrinsics compiler for atomix operations
# to compile as single CPU instructions instead of function calls.
#
# Prerequisites:
#   - curl (for downloading pre-built compiler)
#   - tar and shasum (for extraction and integrity checks)
#   - Linux (or WSL2 on Windows) / macOS
#
# Usage:
#   make install-compiler        # Install the latest Atomix compiler release
#   make install-compiler-source # Build the release from source
#   make build                   # Build lfq with intrinsics compiler
#   make test                    # Test lfq with intrinsics compiler
#   make bench                   # Run benchmarks

# Configuration
export GOTOOLCHAIN := local
COMPILER_REPO    := https://github.com/hayabusa-cloud/go
COMPILER_TAG     ?=
COMPILER_DIR     ?= $(HOME)/sdk/$(or $(COMPILER_TAG),go-atomix)
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
# Compiler Installation
# ============================================================================

install-compiler: archive_suffix = $(GOOS_LOCAL)-$(GOARCH_LOCAL).tar.gz
install-compiler-source: archive_suffix = src.tar.gz
.PHONY: install-compiler install-compiler-source
install-compiler install-compiler-source:
	@set -eu; dir="$(COMPILER_DIR)"; tag="$(COMPILER_TAG)"; \
	parent=$$(dirname "$$dir"); dir="$$parent/$$(basename "$$dir")"; dest="$$dir"; \
	if test -z "$$tag"; then \
		release=$$(curl -fsSIL -o /dev/null -w '%{url_effective}' "$(COMPILER_REPO)/releases/latest"); \
		tag=$${release##*/}; dest="$$parent/$$tag"; \
	fi; \
	matches_tag() { test -x "$$1/bin/go" && test -f "$$1/VERSION" && test "$$(head -n 1 "$$1/VERSION")" = "$$tag"; }; \
	if matches_tag "$$dir"; then exit 0; fi; \
	if test -e "$$dir" && test ! -L "$$dir"; then echo "Compiler destination already exists: $$dir. Choose another COMPILER_DIR." >&2; exit 1; fi; \
	mkdir -p "$$parent"; \
	tmp="$$dest.installing"; mkdir -m 700 "$$tmp"; \
	trap 'rm -rf "$$tmp"' 0; \
	if test -e "$$dest" || test -L "$$dest"; then \
		matches_tag "$$dest" || { echo "Invalid compiler at $$dest" >&2; exit 1; }; \
	else \
		url="$(COMPILER_REPO)/releases/download/$$tag"; archive="$$tag.$(archive_suffix)"; \
		curl -fsSL "$$url/$$archive" -o "$$tmp/$$archive"; \
		curl -fsSL "$$url/SHA256SUMS" -o "$$tmp/SHA256SUMS"; \
		(cd "$$tmp" && awk -v file="$$archive" '$$2 == file { print; found=1 } END { if (!found) exit 1 }' SHA256SUMS > selected.sha256 && shasum -a 256 -c selected.sha256); \
		tar -xzf "$$tmp/$$archive" -C "$$tmp"; \
		test "$$(head -n 1 "$$tmp/go/VERSION")" = "$$tag"; \
		if test "$@" = install-compiler-source; then \
			bootstrap=$$(GOWORK=off go env GOROOT); (cd "$$tmp/go/src" && GOWORK=off GOROOT_BOOTSTRAP="$$bootstrap" ./make.bash); \
		fi; \
		test -x "$$tmp/go/bin/go"; mv "$$tmp/go" "$$dest"; \
	fi; \
	if test "$$dest" != "$$dir"; then ln -sfn "$$tag" "$$dir"; fi

# ============================================================================
# Build & Test with Intrinsics Compiler
# ============================================================================

.PHONY: build
build:
	$(require-compiler)
	@echo "Building lfq with intrinsics compiler..."
	GOROOT="$(GOROOT_INTRINSIC)" "$(GO_INTRINSIC)" vet ./...
	GOROOT="$(GOROOT_INTRINSIC)" "$(GO_INTRINSIC)" build ./...
	@echo "Build successful"

.PHONY: test
test:
	$(require-compiler)
	@echo "Testing lfq with intrinsics compiler..."
	GOROOT="$(GOROOT_INTRINSIC)" "$(GO_INTRINSIC)" test -timeout 10m -v -covermode=atomic -coverprofile=coverage.out ./...
	@echo "Tests passed"

.PHONY: bench
bench:
	$(require-compiler)
	@echo "Running benchmarks with intrinsics compiler..."
	GOROOT="$(GOROOT_INTRINSIC)" "$(GO_INTRINSIC)" test -timeout 30m -bench=. -benchmem ./...

# ============================================================================
# Cross-Compilation (for remote hardware testing)
# ============================================================================

.PHONY: test-binary-linux-amd64
test-binary-linux-amd64:
	$(require-compiler)
	@echo "Building test binary for linux/amd64..."
	GOROOT="$(GOROOT_INTRINSIC)" GOOS=linux GOARCH=amd64 "$(GO_INTRINSIC)" test -c -o lfq-linux-amd64.test .
	@echo "Test binary ready: lfq-linux-amd64.test"

.PHONY: test-binary-linux-arm64
test-binary-linux-arm64:
	$(require-compiler)
	@echo "Building test binary for linux/arm64..."
	GOROOT="$(GOROOT_INTRINSIC)" GOOS=linux GOARCH=arm64 "$(GO_INTRINSIC)" test -c -o lfq-linux-arm64.test .
	@echo "Test binary ready: lfq-linux-arm64.test"

.PHONY: test-binary-darwin-arm64
test-binary-darwin-arm64:
	$(require-compiler)
	@echo "Building test binary for darwin/arm64..."
	GOROOT="$(GOROOT_INTRINSIC)" GOOS=darwin GOARCH=arm64 "$(GO_INTRINSIC)" test -c -o lfq-darwin-arm64.test .
	@echo "Test binary ready: lfq-darwin-arm64.test"

# ============================================================================
# Utilities
# ============================================================================

.PHONY: clean
clean:
	rm -f coverage.out
	rm -f lfq.test lfq-linux-amd64.test lfq-linux-arm64.test lfq-darwin-arm64.test
	GOROOT="$(GOROOT_INTRINSIC)" "$(GO_INTRINSIC)" clean -cache 2>/dev/null || true

.PHONY: check-compiler
check-compiler:
	@if [ -x "$(GO_INTRINSIC)" ]; then \
		echo "Intrinsics compiler found: $(GO_INTRINSIC)"; \
		"$(GO_INTRINSIC)" version; \
	else \
		echo "Intrinsics compiler NOT found at $(GO_INTRINSIC)"; \
		exit 1; \
	fi

.PHONY: help
help:
	@echo "lfq Makefile - Build with intrinsics-customized Go compiler"
	@echo ""
	@echo "Compiler:"
	@echo "  install-compiler        Download the latest Atomix compiler or the release selected by COMPILER_TAG"
	@echo "  install-compiler-source Build the same release from source"
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
