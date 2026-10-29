# kaxa development tasks
#
# Quick start: make build && ./bin/kaxa --help

SHELL := /usr/bin/env bash

GO ?= go
BIN ?= bin/kaxa
CMD := ./cmd/kaxa
GOSEC ?= gosec
GOSEC_SARIF ?= gosec.sarif
SEMTAG ?= semtag
SCOPE ?=
TYPE ?= rc

# scripts/release.sh reads these from the environment.
export SEMTAG GO

VERSION ?= dev
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
	-X github.com/perylemke/kaxa/internal/version.Version=$(VERSION) \
	-X github.com/perylemke/kaxa/internal/version.Commit=$(COMMIT) \
	-X github.com/perylemke/kaxa/internal/version.Date=$(DATE)

.PHONY: help
help: ## List targets
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make \033[36m<target>\033[0m\n\nTargets:\n"} \
		/^[a-zA-Z0-9_-]+:.*##/ {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: build
build: ## Build binary to ./bin/kaxa (quick local test)
	@mkdir -p bin
	CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o $(BIN) $(CMD)

.PHONY: install
install: ## Install kaxa to $$GOBIN or $$GOPATH/bin
	CGO_ENABLED=0 $(GO) install -ldflags "$(LDFLAGS)" $(CMD)

.PHONY: run
run: build ## Build and run kaxa (pass ARGS="--help")
	./$(BIN) $(ARGS)

.PHONY: test
test: ## Unit tests (default go test ./...)
	$(GO) test -race -shuffle=on ./...

.PHONY: test-integration
test-integration: ## Integration tests (in-memory fakes, no network, no cost)
	$(GO) test -race -count=1 -tags=integration ./internal/config/... ./internal/provider/...

.PHONY: coverage
coverage: ## Coverage of unit and integration tests; writes coverage.out and prints the total
	$(GO) test -race -shuffle=on -count=1 -tags=integration -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out | tail -n 1

.PHONY: coverage-html
coverage-html: coverage ## Same as coverage, plus coverage.html with the uncovered lines marked
	$(GO) tool cover -html=coverage.out -o coverage.html

.PHONY: test-live
test-live: ## Tests against the real provider APIs; needs KAXA_* credentials and BILLS compute
	$(GO) test -race -count=1 -timeout 30m -tags=live -v ./internal/provider/...

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

.PHONY: next-version
next-version: ## Print the next final version for SCOPE=major|minor|patch (tags nothing)
	@./scripts/release.sh next "$(SCOPE)"

.PHONY: changelog-release
changelog-release: ## Move [Unreleased] into the next version of CHANGELOG.md (SCOPE=major|minor|patch)
	@version=$$(./scripts/release.sh next "$(SCOPE)") && \
		echo "Updating CHANGELOG.md for $$version" && \
		$(GO) run ./tools/changelog release "$$version"

.PHONY: release
release: ## Tag and push a final version from main (SCOPE=major|minor|patch); publishes the Homebrew cask
	@./scripts/release.sh final "$(SCOPE)"

.PHONY: prerelease
prerelease: ## Tag and push a test version (SCOPE=major|minor|patch TYPE=alpha|beta|rc); not on Homebrew
	@./scripts/release.sh pre "$(SCOPE)" "$(TYPE)"

.PHONY: ci
ci: tidy vet test test-integration test-e2e test-acceptance lint vuln gosec goreleaser-check ## Run CI-equivalent checks locally

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf bin/ dist/ $(GOSEC_SARIF) coverage.out coverage.html
