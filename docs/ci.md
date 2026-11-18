# Rust and Frontend Builds in CI

The Rust CLI supports workspace setup and inspection, collection import/export, documentation generation, and the local web server. The standalone collection runner and its report formats are not yet ported.

## Local checks

Run these from the repository root:

```bash
cargo fmt --all -- --check
cargo check --workspace --all-targets
cargo clippy --workspace --all-targets -- -D warnings
cargo test --workspace
cd web && npm ci && npm run tsc && npm run lint && npm run build
```

The Cargo commands validate the Rust workspace. The npm commands install frontend dependencies, type-check, lint, and create `web/dist/` for the server and desktop app.

## GitHub Actions

`.github/workflows/ci.yml` runs Rust formatting, Clippy, and workspace tests on Linux, macOS, and Windows. It also checks the frontend and runs Playwright smoke checks. Linux Tauri builds require the GTK and WebKitGTK development libraries listed in that workflow.

## Build and run the CLI

```bash
cd web && npm ci && npm run build
cd ..
cargo run -p pebblepost-cli -- version
cargo run -p pebblepost-cli -- workspace init ./my-workspace
cargo run -p pebblepost-cli -- import ./postman-collection.json --workspace ./my-workspace
cargo run -p pebblepost-cli -- export collections --format postman --workspace ./my-workspace
cargo run -p pebblepost-cli -- docs collections --format md --workspace ./my-workspace
cargo run -p pebblepost-cli -- serve --host 127.0.0.1 --port 8080
```

Loopback serving does not require an API token. For a non-loopback bind, set `PEBBLEPOST_TOKEN` or pass `--token`; the CLI saves a provided token in the data directory with restrictive Unix permissions.

## CI secrets

Keep secret values in the CI platform's secret store. Pass them through environment variables or an ignored `*.secret.env.json` workspace file. Do not print values or upload secret environment files as build artifacts. The Rust CLI does not yet include the old collection runner or its report formats.
