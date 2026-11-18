# PebblePost

A local-first, Git-friendly API client for HTTP, GraphQL introspection, WebSocket, and SSE workflows. The backend migration to Rust and Tauri 2 is in progress; the interface uses React 19 and Tailwind CSS v4.

[![Release](https://img.shields.io/github/v/release/lovanbang999/pebblepost?style=flat-square)](https://github.com/lovanbang999/pebblepost/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg?style=flat-square)](./LICENSE)
[![Docker Pulls](https://img.shields.io/badge/docker-lovanbang999%2Fpebblepost-blue?logo=docker&style=flat-square)](https://hub.docker.com/r/lovanbang999/pebblepost)

---

## Table of Contents

- [Introduction](#why-pebblepost)
- [Migration Status](#migration-status)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Workspace & File Format](#workspace--file-format)
- [CLI Reference](#cli-reference)
- [Scripting & Test Assertions](#scripting--test-assertions)
- [Protocol Support](#protocol-support)
- [Security & Code Signing](#security--code-signing)
- [Contributing & License](#contributing--license)

---

## Why PebblePost?

Many API development tools require cloud accounts, store collections in proprietary formats or large monolithic JSON files, and run inside resource-heavy Electron shells. Single-file collection exports often cause merge conflicts in Git, while private API tokens risk leaking into shared cloud workspaces.

PebblePost provides an alternative:
- **Local-first and Git-native**: Every request is saved as an individual `*.pebble.json` file in your repository folders. Teams can branch, review, and merge API changes using standard Git workflows.
- **Separate secret files**: Private environment values are stored in `*.secret.env.json`, which workspace initialization adds to `.gitignore`.
- **Rust backend**: The API server and CLI are Rust applications built with Axum and Tokio; the desktop shell uses Tauri 2.
- **Local-first workspace**: Collections and environments remain readable JSON files in a directory you control.
- **HTTP and GraphQL workflows**: Send HTTP requests, inspect responses, manage cookies, and introspect GraphQL schemas.
- **Streaming workflows**: Connect to WebSocket and Server-Sent Events endpoints from the request editor.

The Rust backend migration is ongoing. The Rust server runs pre-request and post-response scripts for regular request execution and collection runs. The CLI supports workspace setup and inspection, import/export, docs generation, and serving the local UI.

---

## Migration Status

The Rust workspace contains the shared workspace core, Axum server, CLI, and Tauri desktop shell. The CLI supports `version`, `workspace init`, `workspace scan`, `import`, `export`, `docs`, and `serve`. Rust implementations exist for HTTP, GraphQL introspection, WebSocket/SSE, workspace files, history, cookies, import/export, generated docs, and mock APIs.

Parity work remains: persistent stream-script hooks, several CLI automation commands, and cross-platform comparison of the updater and secret backends. Update checks, settings, signature verification, archive extraction, and binary replacement now have Rust implementations. The frontend still exposes some unfinished features, and the legacy Go source has not yet been removed.

---

## Installation

### Build from source

```bash
cd web && npm ci && npm run build
cd ..
cargo build --release --package pebblepost-cli --package pebblepost-server
```

### Docker

```bash
docker build -t pebblepost .
docker run -d --name pebblepost -p 8080:8080 \
  -e PEBBLEPOST_TOKEN="your-secure-token" \
  -v "$(pwd)/data:/data" pebblepost
```

Open `http://localhost:8080?token=your-secure-token` to authenticate the browser UI.

---

## Quick Start

### Native Desktop Studio
Install Node.js and the platform webview build dependencies, then run:
```bash
cargo tauri dev --manifest-path apps/pebblepost-desktop/src-tauri/Cargo.toml
```

### Standalone Web Server
Start the Rust HTTP server:
```bash
# Build the UI once, then run the server on localhost (loopback needs no token)
cd web && npm ci && npm run build
cd ..
cargo run -p pebblepost-cli -- serve --host 127.0.0.1 --port 8080

# Expose server on network interface (requires PEBBLEPOST_TOKEN)
export PEBBLEPOST_TOKEN="secret-bearer-token"
pebblepost serve --host 0.0.0.0 --port 8080 --token "$PEBBLEPOST_TOKEN"
```
Open **http://localhost:8080** in a browser. To expose the server on a network interface, set `PEBBLEPOST_TOKEN` and pass `--token`.

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

The Rust CLI provides version reporting, workspace setup and inspection, collection import/export, documentation generation, and the local server.

```bash
pebblepost version
pebblepost workspace init ./my-workspace --name "My Workspace"
pebblepost workspace scan ./my-workspace
pebblepost import ./postman-collection.json --workspace ./my-workspace
pebblepost export collections --format postman --workspace ./my-workspace
pebblepost docs collections --format md --workspace ./my-workspace
pebblepost serve --host 127.0.0.1 --port 8080
```

Imports accept Postman, OpenAPI JSON, HAR, Insomnia, Bruno JSON, and cURL input. Exports currently support Postman and OpenAPI JSON. Documentation supports Markdown, HTML, and OpenAPI output.

---

## Scripting & Test Assertions

### Request scripts

Request and folder `preRequest` and `postResponse` scripts execute in the Rust server's bounded JavaScript runtime. See [docs/scripting.md](docs/scripting.md) for script ordering, supported `pb.*` APIs, trust behavior, and current limitations.

---

## Protocol Support

- **HTTP/GraphQL**: HTTP request execution and GraphQL schema introspection.
- **WebSocket/SSE**: Persistent streaming requests, WebSocket send, and message event logs.
- **gRPC**: Reflection, proto-based service inspection, unary calls, and streaming endpoints are implemented in the Rust server.

---

## Performance & Benchmarks

Rust performance measurements have not been published yet.

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
