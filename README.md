# PebblePost

<p align="center">
  <strong>A modern, lightweight, Git-friendly API Client and Studio for REST, GraphQL, and microservices.</strong><br>
  Available as both a native Linux desktop application (Wails v2) and a standalone web server with an embedded React 19 frontend.
</p>

<p align="center">
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat-square&logo=go" alt="Go Version"></a>
  <a href="https://react.dev/"><img src="https://img.shields.io/badge/React-19.0-61DAFB?style=flat-square&logo=react" alt="React 19"></a>
  <a href="https://tailwindcss.com/"><img src="https://img.shields.io/badge/Tailwind-v4.0-38B2AC?style=flat-square&logo=tailwind-css" alt="Tailwind v4"></a>
  <a href="https://wails.io/"><img src="https://img.shields.io/badge/Wails-v2-DF1A2A?style=flat-square&logo=wails" alt="Wails v2"></a>
</p>

---

## Overview

PebblePost is designed for developers who want a fast, privacy-first, and Git-native alternative to Postman:

- **Local-First & Git-Friendly**: API collections are saved directly as `.pebble.json` files in your project folders. Commit them to Git alongside your codebase.
- **Secret Isolation**: Sensitive keys and tokens are stored in `*.secret.env.json` files that are automatically gitignored.
- **Embedded Goja JavaScript Sandbox**: Run pre-request and test assertion scripts (`pb.test`, `pb.expect`, `pb.environment`) in pure Go without Node.js dependencies.
- **Dual Runtime Support**: Runs as a frameless native Linux desktop application or a standalone web service.
- **Detailed Network Timing**: Measure DNS Lookup, TCP Connect, TLS Handshake, TTFB, and Transfer durations with precision.
- **CLI Test Runner**: Execute automated API test suites directly in CI/CD pipelines.

---

## Quick Start (Server / Web)

### Running from source

```bash
# 1. Install dependencies
go mod download

# 2. Run backend server
go run ./cmd/server
```

Open **http://localhost:8080** in your browser.

---

## Desktop Studio (Linux)

### Building Desktop App with Wails

```bash
# 1. Build desktop binary
wails build -s -skipbindings -clean

# 2. Package installable Debian package (.deb)
bash ./scripts/package-deb.sh
```

---

## License

MIT License.
