.DEFAULT_GOAL := help

.PHONY: help
help: ## Show available tasks
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

.PHONY: init-hooks
init-hooks: ## Configure git to use project pre-commit hooks
	git config core.hooksPath .githooks
	chmod +x .githooks/pre-commit

.PHONY: check
check: lint-rust lint-fe fe-tsc ## Run static checks for Rust and the frontend

.PHONY: lint
lint: lint-rust lint-fe ## Run Rust and frontend linters

.PHONY: lint-rust
lint-rust: ## Format and lint the Rust workspace
	cargo fmt --all -- --check
	cargo clippy --workspace --all-targets -- -D warnings

.PHONY: lint-fe
lint-fe: ## Run ESLint on the frontend
	cd web && npm run lint

.PHONY: test
test: test-rust fe-test ## Run Rust and frontend unit tests

.PHONY: test-rust
test-rust: ## Run Rust workspace tests
	cargo test --workspace

.PHONY: build
build: build-cli build-server ## Build the CLI and standalone web server

.PHONY: build-cli
build-cli: ## Build the PebblePost CLI
	cargo build --release --package pebblepost-cli

.PHONY: build-server
build-server: ## Build the Axum web server
	cargo build --release --package pebblepost-server

.PHONY: build-mcp
build-mcp: ## Build the PebblePost MCP server
	cargo build --release --package pebblepost-mcp

.PHONY: run-mcp
run-mcp: ## Run the MCP server for the current workspace over stdio
	cargo run --package pebblepost-mcp -- --workspace .

.PHONY: build-desktop
build-desktop: ## Package the Tauri desktop application
	cargo tauri build --manifest-path apps/pebblepost-desktop/src-tauri/Cargo.toml

.PHONY: run-server
run-server: ## Run the local web server
	cargo run --package pebblepost-server

.PHONY: dev-desktop
dev-desktop: ## Run the Tauri desktop application in development mode
	cargo tauri dev --manifest-path apps/pebblepost-desktop/src-tauri/Cargo.toml

.PHONY: fe-install
fe-install: ## Install frontend dependencies
	cd web && npm ci

.PHONY: fe-test
fe-test: ## Run frontend unit tests
	cd web && npm run test:run

.PHONY: fe-build
fe-build: ## Build the React/Vite frontend
	cd web && npm run build

.PHONY: fe-tsc
fe-tsc: ## Type-check the frontend
	cd web && npm run tsc

.PHONY: fe-e2e
fe-e2e: fe-build build-cli ## Run Playwright end-to-end checks
	cd web && npm run test:e2e

.PHONY: clean
clean: ## Remove generated artifacts
	rm -rf target bin coverage.out coverage.html web/dist/
