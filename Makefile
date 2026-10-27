# sbox development tasks
#
# Quick start: make build && ./bin/sbox --help

SHELL := /usr/bin/env bash

GO ?= go
BIN ?= bin/sbox
CMD := ./cmd/sbox
GOSEC ?= gosec
GOSEC_SARIF ?= gosec.sarif

VERSION ?= dev
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X github.com/perylemke/sbox/internal/version.Version=$(VERSION) \
	-X github.com/perylemke/sbox/internal/version.Commit=$(COMMIT) \
	-X github.com/perylemke/sbox/internal/version.Date=$(DATE)

.PHONY: help
help: ## List targets
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make \033[36m<target>\033[0m\n\nTargets:\n"} \
		/^[a-zA-Z0-9_-]+:.*##/ {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: build
build: ## Build binary to ./bin/sbox (quick local test)
	@mkdir -p bin
	CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o $(BIN) $(CMD)

.PHONY: install
install: ## Install sbox to $$GOBIN or $$GOPATH/bin
	CGO_ENABLED=0 $(GO) install -ldflags "$(LDFLAGS)" $(CMD)

.PHONY: run
run: build ## Build and run sbox (pass ARGS="--help")
	./$(BIN) $(ARGS)

.PHONY: test
test: ## Unit tests (default go test ./...)
	$(GO) test -race -shuffle=on ./...

.PHONY: test-integration
test-integration: ## Integration tests
	$(GO) test -race -count=1 -tags=integration ./internal/config/...

.PHONY: test-e2e
test-e2e: ## End-to-end tests (builds binary)
	$(GO) test -race -count=1 -tags=e2e ./test/e2e/...

.PHONY: test-acceptance
test-acceptance: ## Acceptance tests
	$(GO) test -race -count=1 -tags=acceptance ./test/acceptance/...

.PHONY: test-all
test-all: test test-integration test-e2e test-acceptance ## All test suites

.PHONY: vet
vet: ## go vet
	$(GO) vet ./...

.PHONY: tidy
tidy: ## go mod tidy
	$(GO) mod tidy

.PHONY: fmt
fmt: ## Format Go code with gofmt and gofumpt
	golangci-lint fmt ./...

.PHONY: lint
lint: ## golangci-lint
	golangci-lint run ./...

.PHONY: vuln
vuln: ## govulncheck
	govulncheck ./...

.PHONY: gosec
gosec: ## gosec SAST; SARIF in gosec.sarif; fails on findings
	$(GOSEC) -fmt sarif -out $(GOSEC_SARIF) -stdout -verbose=text ./...

.PHONY: goreleaser-check
goreleaser-check: ## Validate .goreleaser.yml
	goreleaser check

.PHONY: ci
ci: tidy vet test test-integration test-e2e test-acceptance lint vuln gosec goreleaser-check ## Run CI-equivalent checks locally

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf bin/ dist/ $(GOSEC_SARIF)
