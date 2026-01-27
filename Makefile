# lfq Makefile
# Uses intrinsics-optimized Go compiler for atomix

# Use ATOMIX_GOROOT if set, otherwise expect 'go' in PATH
ifdef ATOMIX_GOROOT
    GOROOT := $(ATOMIX_GOROOT)
    GO := $(GOROOT)/bin/go
else
    GO := go
endif

.DEFAULT_GOAL := help

.PHONY: help check test bench

help: ## Show available targets
	@echo "lfq - Lock-Free Queue Library"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-10s %s\n", $$1, $$2}'
	@echo ""
ifdef ATOMIX_GOROOT
	@echo "Using intrinsics compiler: $(GOROOT)"
else
	@echo "Note: Set ATOMIX_GOROOT for intrinsics compiler (github.com/hayabusa-cloud/go branch:atomix)"
endif

check: ## Run all checks: fmt, vet, tidy
	@echo "==> Running gofmt..."
	@test -z "$$(gofmt -l .)" || (gofmt -l . && exit 1)
	@echo "==> Running go vet..."
	@GOROOT=$(GOROOT) $(GO) vet ./...
	@echo "==> Running go mod tidy..."
	@GOROOT=$(GOROOT) $(GO) mod tidy
	@echo "==> All checks passed"

test: ## Run tests (no race detection)
	@GOROOT=$(GOROOT) $(GO) test -v ./...

bench: ## Run benchmarks
	@GOROOT=$(GOROOT) $(GO) test -bench=. -benchmem ./...
