# Changelog

All notable changes to **PebblePost** are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [0.2.0] - 2026-10-04

### Added
- **Automated Multi-Platform Release Pipeline**:
  - GoReleaser v2 configuration for building optimized, standalone CLI (`pebblepost`) and server (`pebblepost-server`) binaries across Linux (`amd64`, `arm64`), macOS (`amd64`, `arm64`), and Windows (`amd64`, `arm64`).
  - GitHub Actions matrix workflow (`release.yml`) triggered on version tags (`v*.*.*`) building native Wails desktop packages: macOS Universal DMG/app, Windows NSIS installer/zip, and Linux AppImage/tar.gz.
  - Automated SHA256 checksums (`checksums.txt`) signed cryptographically with Minisign (`checksums.txt.minisig`).
- **Production Multi-Stage Docker Container**:
  - Hardened Alpine Linux runtime with `dumb-init`, non-root user `pebble:pebble` (`UID 10001`), persistent `/data` volume, and native `/api/health` healthcheck.
  - Authentication support for `PEBBLEPOST_TOKEN` and configurable bind address via `PEBBLEPOST_HOST`.
- **Packaging & Distribution Manifests**:
  - Homebrew tap formula (`packaging/homebrew/pebblepost.rb`) for macOS and Linux.
  - Arch Linux AUR `PKGBUILD` (`packaging/aur/PKGBUILD`) with systemd service definition.
  - Windows Package Manager (winget) manifest (`packaging/winget/lovanbang999.pebblepost.yaml`).
  - Comprehensive packaging guide (`docs/packaging.md`) covering install channels, signature verification, and macOS Gatekeeper / Windows SmartScreen guidance.
- **Cryptographic Auto-Update & UI Notification**:
  - In-app update engine querying GitHub Releases API with semver resolution.
  - Minisign and Ed25519 signature verification endpoint (`/api/update/verify`) and update checking endpoint (`/api/update/check`).
  - UI notification banner and Update Preferences dialog allowing auto-checks to be enabled or disabled on startup.
- **Empirical Benchmarking & Performance Diagnostics**:
  - Reproducible benchmark script (`scripts/measure.sh`) measuring cold startup latency, idle RAM (RSS), and stripped binary size.
  - Published non-subjective empirical comparisons against Postman and Bruno (`docs/benchmarks.md`).
- **Collection Runner & CI/CD CLI**:
  - Command-line runner (`pebblepost run`) supporting folder filtering, data-driven CSV/JSON iterations, delay, bail-on-failure, and JUnit/JSON/human-readable reports.
  - In-app interactive Collection Runner tab with real-time step streaming.
- **API Documentation & OpenAPI Generation**:
  - Interactive documentation tab previewing markdown descriptions, schemas, request parameters, and response examples.
  - Static documentation generator CLI (`pebblepost docs <path> --format md|html|openapi --out <dir>`) with multi-language code snippets (cURL, Go, Node.js, Python, C#).
- **Streaming Protocols (gRPC, WebSocket & SSE)**:
  - gRPC client supporting Unary, Client-Streaming, Server-Streaming, and Bidirectional calls via Server Reflection and Protobuf schemas.
  - Real-time WebSocket and Server-Sent Events (SSE) streaming client with live message log and connection lifecycle diagnostics.

### Changed
- Converted version identifiers to ldflags-injectable variables (`version`, `commit`, `date`, `builtBy`) for deterministic builds.
- Upgraded server health endpoint to handle both `/api/health` and `/health` with bypass of bearer token authentication.

---

## [0.1.0] - 2026-10-03

### Initial Release
- **Local-First Architecture**: Individual `*.pebble.json` files per request, enabling branch-based Git workflows with zero merge conflicts.
- **Secret Isolation**: Clear separation between repository-committed environments (`*.env.json`) and private secrets (`*.secret.env.json`), automatically guarded by `.gitignore`.
- **Pure Go JavaScript Sandbox**: Response assertions (`pb.test`, `pb.expect`) and pre-request scripts executed inside the Goja engine with zero Node.js runtime overhead.
- **Network Tracing Diagnostics**: Microsecond-level httptrace breakdowns for DNS, TCP, TLS, TTFB, and Transfer durations.
- **Smart Importers & Exporters**: 1-click import from cURL, Postman Collections (v2.1), and OpenAPI 3.0 schemas; code generation for cURL, Go, Node.js, Python, and C#.
- **Dual Runtime**: Single Go binary operating either as a native Wails v2 Linux desktop window or as a headless web server with embedded React 19 interface.
