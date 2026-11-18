# Contributing to PebblePost

Thanks for helping improve PebblePost. Keep changes focused, preserve the workspace file format, and update the React interface and Rust API together when their contract changes.

## Project Layout

- `crates/pebblepost-core/` contains shared workspace, environment, path-safety, and interpolation logic.
- `apps/pebblepost-server/` provides the Axum API and standalone server binary.
- `apps/pebblepost-cli/` provides command-line workflows.
- `apps/pebblepost-desktop/src-tauri/` contains the Tauri 2 shell and native commands.
- `web/src/` contains the React/Vite interface; production assets are built into `web/dist/`.

## Development Setup

Install stable Rust with `rustfmt` and `clippy`, Node.js 22, and npm. Tauri builds also need the platform webview libraries; Linux requires GTK 3 and WebKitGTK 4.1 development packages. Install the Tauri CLI with `cargo install tauri-cli --version '^2'`.

```sh
cd web && npm ci && npm run dev
cargo run --package pebblepost-cli -- serve --port 8080
cargo tauri dev --manifest-path apps/pebblepost-desktop/src-tauri/Cargo.toml
```

The UI is available at `http://localhost:5173` during web development. The standalone server serves the built frontend from `web/dist/`.

## Build and Checks

Run `cargo fmt --all`, `cargo clippy --workspace --all-targets -- -D warnings`, and `cargo test --workspace` for Rust changes. Run `cd web && npm run lint && npm run tsc && npm run test:run` for frontend changes. `make build` builds the CLI and server; `make build-desktop` packages the Tauri app.

## Style and Pull Requests

Use Rust's standard formatting, `snake_case` for Rust modules and functions, and descriptive React component names in PascalCase. Keep API JSON fields camelCase and reuse core functions instead of duplicating filesystem logic in handlers. Name Rust tests after the behavior they cover (`scan_rejects_paths_outside_root`).

Use concise Conventional Commit subjects such as `feat(server): add request history`. Pull requests should explain the behavior change, link related issues, list validation commands, and include screenshots for visible UI changes. Never commit credentials, private environment files, or generated build artifacts.
