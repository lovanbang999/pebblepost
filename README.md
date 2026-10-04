# PebblePost

A local-first, Git-friendly API client, runner, and documentation studio for REST, GraphQL, gRPC, and WebSocket/SSE. Built with Go 1.26, React 19, Tailwind CSS v4, and Wails v2.

[![Release](https://img.shields.io/github/v/release/lovanbang999/pebblepost?style=flat-square)](https://github.com/lovanbang999/pebblepost/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg?style=flat-square)](./LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/lovanbang999/pebblepost)](https://goreportcard.com/report/github.com/lovanbang999/pebblepost)
[![Docker Pulls](https://img.shields.io/badge/docker-lovanbang999%2Fpebblepost-blue?logo=docker&style=flat-square)](https://hub.docker.com/r/lovanbang999/pebblepost)

---

## Table of Contents

- [Introduction](#why-pebblepost)
- [Objective Tool Comparison](#objective-comparison-with-other-tools)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Workspace & File Format](#workspace--file-format)
- [CLI Reference](#cli-reference)
- [Scripting & Test Assertions](#scripting--test-assertions)
- [Multi-Protocol Support](#multi-protocol-support)
- [Security & Code Signing](#security--code-signing)
- [Contributing & License](#contributing--license)

---

## Why PebblePost?

Many API development tools require cloud accounts, store collections in proprietary formats or large monolithic JSON files, and run inside resource-heavy Electron shells. Single-file collection exports often cause merge conflicts in Git, while private API tokens risk leaking into shared cloud workspaces.

PebblePost provides an alternative:
- **Local-first and Git-native**: Every request is saved as an individual `*.pebble.json` file in your repository folders. Teams can branch, review, and merge API changes using standard Git workflows.
- **Strict secret isolation**: Public environment variables are stored in `*.env.json` (tracked in Git), while private keys and tokens are stored in `*.secret.env.json` (automatically blocked by `.gitignore`).
- **Zero-runtime JavaScript sandbox**: Pre-request transformations and response assertions (`pb.test`, `pb.expect`, `pb.environment`) run directly in pure Go via Goja, requiring no Node.js runtime.
- **Dual-runtime binary**: Runs as a native desktop application via Wails v2 (macOS, Windows, Linux) or as a standalone web server with an embedded React 19 interface.
- **Multi-protocol engine**: Full support for REST, GraphQL, gRPC (with Server Reflection), and real-time streaming (WebSocket, Server-Sent Events).
- **Network timing diagnostics**: Microsecond-accurate latency breakdown for DNS Lookup, TCP Connect, TLS Handshake, TTFB, and Content Transfer.

---

## Objective Comparison with Other Tools

All performance figures below are empirically measured on Linux x86_64 using stripped production binaries (`-trimpath -ldflags="-s -w"`). Reproduction script available at [`scripts/measure.sh`](./scripts/measure.sh).

| Feature / Metric | PebblePost | Postman (Desktop v11) | Bruno (Desktop v1.38) | Insomnia (v10) |
| :--- | :--- | :--- | :--- | :--- |
| **Cold Startup Time** | **6.93 ms** (CLI) / **21.96 ms** (Server) | ~2,400 ms | ~1,100 ms | ~2,100 ms |
| **Idle Memory (RSS)** | **21.68 MB** (Server) | ~420 MB | ~170 MB | ~380 MB |
| **Executable Size** | **26.82 MB** (CLI) / **28.93 MB** (Server + UI) | ~180 MB (installer) | ~95 MB (installer) | ~160 MB (installer) |
| **Runtime Architecture** | Standalone static Go binary | Electron / Chromium / Node.js | Electron / Chromium / Node.js | Electron / Chromium / Node.js |
| **Cloud Account Required** | **No** (100% offline & local) | Yes (for full features) | No | Yes (for full features) |
| **Collection Storage Format** | Individual `*.pebble.json` files | Monolithic JSON dump | Individual `*.bru` files | Monolithic JSON dump |
| **Secret Isolation** | Built-in `*.secret.env.json` | Cloud or single file | Git-ignored file | Cloud or single file |
| **Headless Server Mode** | **Yes** (Docker / web server) | Newman CLI only | Bruno CLI only | Inso CLI only |
| **Cryptographic Signatures**| **Yes** (Minisign + Ed25519) | Vendor auto-updater | None | Vendor auto-updater |

---

## Installation

### 1. Homebrew (macOS & Linux)
```bash
brew install lovanbang999/tap/pebblepost
```

### 2. Arch Linux (AUR)
```bash
yay -S pebblepost-bin
# or build from source:
yay -S pebblepost
```

### 3. Windows (winget)
```powershell
winget install lovanbang999.pebblepost
```

### 4. Docker Container (Web Server)
Multi-stage minimal Alpine container with non-root security context (`UID 10001`):

```bash
docker run -d \
  --name pebblepost \
  -p 8080:8080 \
  -e PEBBLEPOST_TOKEN="your-secure-token" \
  -v $(pwd)/data:/data \
  lovanbang999/pebblepost:latest
```

*Note: Access the web UI at `http://localhost:8080?token=your-secure-token`.*

### 5. Pre-built Binaries & Signatures
Download release archives from [GitHub Releases](https://github.com/lovanbang999/pebblepost/releases). Verify integrity using [Minisign](https://jedisct1.github.io/minisign/):

```bash
# Verify checksums file
minisign -Vm checksums.txt -p docs/minisign.pub

# Verify binary sha256
sha256sum -c checksums.txt --ignore-missing
```

---

## Quick Start

### Native Desktop Studio
Launch the desktop application for your platform:
```bash
# On Linux
./pebblepost

# On macOS
open PebblePost.app

# On Windows
.\PebblePost.exe
```

### Standalone Web Server
Start the embedded HTTP server:
```bash
# Run server on localhost (no token required on loopback)
pebblepost-server --port 8080

# Expose server on network interface (requires PEBBLEPOST_TOKEN)
export PEBBLEPOST_TOKEN="secret-bearer-token"
pebblepost-server --host 0.0.0.0 --port 8080 --token $PEBBLEPOST_TOKEN
```
Open **http://localhost:8080** in any modern web browser.

---

## Workspace & File Format

PebblePost structures API collections as individual, human-readable JSON files:

```
my-workspace/
├── .pebble/                               # Workspace configuration
│   ├── workspace.json                     # Workspace settings & default environment
│   ├── cookies.json                       # Cookie jar (session cookies)
│   ├── environments/
│   │   ├── dev.env.json                   # Public variables (Git-tracked)
│   │   ├── dev.secret.env.json            # Private tokens (Auto-blocked in Git)
│   │   └── prod.env.json
│   └── .gitignore                         # Automatically protects *.secret.env.json
│
└── collections/                           # API request files
    ├── auth/
    │   ├── login.pebble.json              # Request definition
    │   └── refresh-token.pebble.json
    └── users/
        ├── get-profile.pebble.json
        └── update-user.pebble.json
```

### Request Definition Schema (`*.pebble.json`)
```json
{
  "$schema": "https://pebblepost.dev/schemas/v1/request.json",
  "version": "1.0",
  "name": "User Login",
  "method": "POST",
  "url": "{{BASE_URL}}/api/v1/auth/login",
  "headers": [
    { "key": "Content-Type", "value": "application/json", "enabled": true }
  ],
  "params": [],
  "auth": { "type": "none" },
  "body": {
    "type": "json",
    "raw": "{\n  \"username\": \"admin\",\n  \"password\": \"{{ADMIN_PASSWORD}}\"\n}"
  },
  "scripts": {
    "preRequest": "pb.request.headers.set('X-Timestamp', Date.now().toString());",
    "postResponse": "pb.test('Status is 200', () => {\n  pb.expect(pb.response.status).to.eql(200);\n});\nconst res = pb.response.json();\nif (res?.token) {\n  pb.environment.set('AUTH_TOKEN', res.token);\n}"
  },
  "settings": {
    "followRedirects": true,
    "verifySSL": true,
    "timeoutMs": 30000
  }
}
```

---

## CLI Reference

The `pebblepost` CLI provides automation capabilities for CI/CD pipelines and documentation generation.

### 1. Test Runner (`pebblepost run`)
Execute entire collections or specific subfolders:

```bash
# Run all requests in a collection
pebblepost run ./collections -e dev

# Run a specific subfolder with bail-on-failure
pebblepost run ./collections --folder auth --bail

# Data-driven testing with CSV or JSON iteration parameters
pebblepost run ./collections --data ./users.csv --iterations 10 --delay 200

# Generate JUnit XML or JSON reports for CI pipelines
pebblepost run ./collections -e staging --report junit --output ./test-results.xml
```

| Flag | Description | Default |
| :--- | :--- | :--- |
| `-e, --env <name>` | Environment name (`dev`, `staging`, `prod`) | `""` |
| `--folder <subpath>`| Subfolder filter within collection | `""` |
| `--data <file>` | Path to CSV or JSON data file for iterations | `""` |
| `-n, --iterations <n>`| Number of iteration passes | `1` |
| `--delay <ms>` | Delay between request executions in milliseconds | `0` |
| `--bail` | Stop execution immediately upon first failed test | `false` |
| `--report <format>`| Report output: `human`, `json`, `junit` | `human` |
| `--output <file>` | Destination file for report output | `stdout` |
| `--dry-run` | Validate and list execution plan without firing requests| `false` |

### 2. Documentation Generator (`pebblepost docs`)
Generate self-contained API documentation or OpenAPI specifications:

```bash
# Generate Markdown documentation
pebblepost docs ./collections --format md --out ./docs/api.md

# Generate responsive HTML documentation
pebblepost docs ./collections --format html --out ./dist/docs.html

# Export OpenAPI 3.0 specification from collection
pebblepost docs ./collections --format openapi --out ./openapi.yaml
```

---

## Scripting & Test Assertions

PebblePost embeds an ECMAScript runtime powered by [Goja](https://github.com/dop251/goja):

```javascript
// Pre-request: Inject authentication signatures
const ts = Math.floor(Date.now() / 1000);
pb.request.headers.set('X-Request-Timestamp', ts.toString());
pb.request.headers.set('X-Signature', pb.crypto.sha256('secret' + ts));

// Post-response: Validate assertions
pb.test("Status code is 200 OK", function() {
  pb.expect(pb.response.status).to.eql(200);
});

pb.test("Response time under 250ms", function() {
  pb.expect(pb.response.duration).to.be.below(250);
});

// Extract token into active environment
const body = pb.response.json();
if (body?.access_token) {
  pb.environment.set("ACCESS_TOKEN", body.access_token);
}
```

---

## Multi-Protocol Support

- **REST / HTTP**: Full HTTP/1.1 and HTTP/2 execution with TLS certificate management, proxy support, and detailed network timeline (`httptrace`).
- **GraphQL**: Query and variable editing with automatic syntax highlighting and schema queries.
- **gRPC**: Unary, Client-Streaming, Server-Streaming, and Bidirectional streaming calls using Protobuf descriptors or Server Reflection.
- **WebSocket & SSE**: Real-time event streams with message inspection, filtering, and connection lifecycle monitors.

---

## Security & Code Signing

PebblePost releases are cryptographically signed using Minisign (Ed25519). For detailed instructions on verifying signatures or bypassing macOS Gatekeeper and Windows SmartScreen without commercial enterprise certificates, see [docs/packaging.md](./docs/packaging.md).

Public Minisign Verification Key:
```
untrusted comment: pebblepost release public key
RWSL0U7j8Vf5CgEw/Xv9nC8fKx9P9M8qZ7vY2kL5mN4pQ==
```

---

## Contributing & License

- [Contributing Guidelines](./CONTRIBUTING.md)
- [Code of Conduct](./CODE_OF_CONDUCT.md)
- [Security Policy](./SECURITY.md)
- [Changelog](./CHANGELOG.md)

PebblePost is open-source software licensed under the [MIT License](./LICENSE).
