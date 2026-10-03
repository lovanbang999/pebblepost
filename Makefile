.DEFAULT_GOAL := help

# ─── Helpers ──────────────────────────────────────────────────────────────────

.PHONY: help
help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

.PHONY: init-hooks
init-hooks: ## Configure git to use project pre-commit hooks
	git config core.hooksPath .githooks
	chmod +x .githooks/pre-commit
	@echo "✅ Pre-commit hook installed successfully"

# ─── Go ───────────────────────────────────────────────────────────────────────

.PHONY: test
test: ## Run all Go tests with race detector
	@mkdir -p web/dist && touch web/dist/.gitkeep
	go test -race -count=1 ./...

.PHONY: coverage
coverage: ## Run Go tests and show coverage report
	@mkdir -p web/dist && touch web/dist/.gitkeep
	go test -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -func=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage HTML report: coverage.html"

.PHONY: coverage-open
coverage-open: coverage ## Open HTML coverage report in browser
	@command -v xdg-open >/dev/null && xdg-open coverage.html || open coverage.html

.PHONY: lint
lint: lint-go lint-fe ## Run all linters (Go + frontend)

.PHONY: lint-go
lint-go: ## Run go vet
	@mkdir -p web/dist && touch web/dist/.gitkeep
	go vet ./...

.PHONY: lint-fe
lint-fe: ## Run ESLint on frontend
	cd web && npm run lint

.PHONY: fuzz
fuzz: ## Run fuzz tests for 30 seconds each
	@mkdir -p web/dist && touch web/dist/.gitkeep
	go test -fuzz=FuzzInterpolateString -fuzztime=30s ./internal/workspace/...
	go test -fuzz=FuzzInterpolateBroken -fuzztime=30s ./internal/workspace/...
	go test -fuzz=FuzzInterpolateAtoB   -fuzztime=30s ./internal/workspace/...

.PHONY: build
build: build-cli build-server ## Build all Go binaries

.PHONY: build-cli
build-cli: ## Build CLI binary
	go build -o bin/pebblepost ./cmd/cli

.PHONY: build-server
build-server: ## Build server binary
	@mkdir -p web/dist && touch web/dist/.gitkeep
	go build -o bin/pebblepost-server ./cmd/server

# ─── Frontend ─────────────────────────────────────────────────────────────────

.PHONY: fe-install
fe-install: ## Install frontend dependencies
	cd web && npm ci

.PHONY: fe-test
fe-test: ## Run frontend unit tests (Vitest)
	cd web && npm run test:run

.PHONY: fe-test-node
fe-test-node: ## Run frontend node:test suite (tabStore)
	cd web && npm test

.PHONY: fe-build
fe-build: ## Build frontend production bundle
	cd web && npm run build

.PHONY: fe-tsc
fe-tsc: ## TypeScript type-check
	cd web && npm run tsc

# ─── Combined ─────────────────────────────────────────────────────────────────

.PHONY: check
check: lint test fe-tsc fe-test ## Run all checks (lint + tests)
	@echo "✅ All checks passed"

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf bin/ coverage.out coverage.html web/dist/
