# Rust Migration Status

## Cutover

The Go backend and application entry points have been retired. PebblePost's native application, server, CLI, and shared backend logic are implemented in Rust. The web interface remains React and TypeScript and communicates with the Rust server API.

## Rust Architecture

- `crates/pebblepost-core` contains workspace operations, models, interpolation, extraction, secrets, and security helpers.
- `apps/pebblepost-server` provides the Axum API, HTTP client, runner, scripting sandbox, import/export, mock server, OpenAPI sync, streaming, gRPC, history, and update services.
- `apps/pebblepost-cli` provides command-line workflows.
- `apps/pebblepost-desktop` provides the Tauri shell.
- `web/` contains the React/TypeScript interface and browser assets.

## Build and Test

The Rust workspace uses the checked-in Cargo workspace and vendored dependencies. From the repository root, run:

```sh
cargo check --workspace --offline
cargo test --workspace --offline
```

At cutover, the workspace check completed with exit code 0 and no compiler warnings. The offline test suite passed 44 tests across core and server crates. The `Makefile`, CI, and release workflows use Cargo for Rust builds and tests; frontend commands use npm.

## Legacy Files

The Go source trees (`internal/` and `cmd/`), Go module files, and Go embedding files were removed after the Rust cutover. Keep future backend changes in the Rust crates and applications, and update this document if workspace boundaries or verification commands change.
