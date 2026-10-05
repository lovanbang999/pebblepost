# PebblePost Security Architecture & Hardening Guide

This document describes the security model, threat defenses, and implementation details for PebblePost as audited and hardened in Round 2 (Prompt 19).

---

## 1. Workspace Boundary & Path Confinement (19.2)

### Threat Model
Collections, request definitions, and `.proto` files reference local disk paths (e.g. `@file:<path>` body references, multipart form uploads, `.proto` import paths, TLS certificates). Malicious collections could attempt to access sensitive files outside the workspace (e.g. `/etc/passwd`, `~/.ssh/id_rsa`, `..\..\Windows\System32`).

### Defense Implementation
- All file references resolved via `security.SafeJoin(workspaceRoot, relPath)` or `security.SafeAbsolute(workspaceRoot, absPath)`.
- Symlinks escaping the workspace root are resolved via `filepath.EvalSymlinks` and rejected if their target lies outside `workspaceRoot`.
- Directory traversal patterns (`..`, Windows device names `CON`, `NUL`, `COM1`, null bytes) are rejected immediately.
- When running outside an explicit workspace root, `security.RejectTraversal` rejects any path containing `..`, null bytes, or drive-escaping components.

---

## 2. Scripting Sandbox & `pb.sendRequest` Protection (19.3)

### Threat Model
Pre-request and post-response scripts run in Goja (JavaScript). Malicious scripts could attempt to execute infinite network loops, exfiltrate sensitive environment secrets, follow unbounded redirects, or make Server-Side Request Forgery (SSRF) requests against cloud metadata endpoints (`169.254.169.254`, `metadata.google.internal`).

### Defense Implementation
- **SSRF Blocklist**: Requests to link-local and cloud metadata addresses (`169.254.169.254`, `[fd00:ec2::254]`, `metadata.google.internal`) are blocked at URL parsing and before each redirect.
- **Redirect Policy**: Maximum 3 redirects followed. Redirecting from HTTPS to plain HTTP is strictly rejected to prevent credential and token downgrade attacks.
- **Loop & Recursion Bounds**: A single script execution is strictly capped at a maximum of 5 auxiliary `pb.sendRequest` calls.
- **Secret Masking**: All response bodies, headers, console logs, and errors returned to the script environment pass through `security.MaskSecrets`, replacing secret environment values with `[MASKED]`.
- **TLS Verification**: Respects per-request and environment TLS policy (`insecureSkipVerify`).

---

## 3. OAuth2 PKCE Loopback Callback Server (19.4)

### Threat Model
During OAuth2 Authorization Code with PKCE flows, PebblePost starts a local HTTP server to receive the authorization code redirect. Attackers could attempt DNS rebinding or CSRF/state injection to hijack authorization codes or disrupt user logins.

### Defense Implementation
- **Localhost Binding**: Binds exclusively to `127.0.0.1:0` (dynamic loopback port).
- **DNS Rebinding Defense**: Inspects the incoming `Host` header, allowing only `127.0.0.1` or `localhost`. External or rebound domains return `400 Bad Request`.
- **Timing-Safe State Validation**: Validates `state` using `crypto/subtle.ConstantTimeCompare`. Mismatched state probes return `400 Bad Request` without aborting in-flight user authorizations.
- **Single-Use Guard**: Uses `sync.Once` to ingest at most one valid authorization code. Once processed, the loopback server shuts down immediately.

---

## 4. Auto-Update Verification Gate (19.5)

### Threat Model
Software updaters are high-value targets for supply-chain attacks, downgrade attacks, and man-in-the-middle tampering.

### Defense Implementation
- **Cryptographic Signature Verification**: Every update artifact requires an Ed25519/Minisign cryptographic signature (`.minisig` / `.sig`). The signature MUST verify against the configured trusted public key before the existing binary is touched.
- **Downgrade Protection**: Semver comparison (`CompareSemver`) strictly requires `latestVersion > currentVersion`. Attempts to downgrade or reinstall the same version are refused.
- **HTTPS Enforcement**: In production, asset and signature URLs must strictly use `https://`. Insecure `http://` URLs are rejected.
- **Atomic Replacement & Secure Permissions**: Downloads stream to a temporary file created with mode `0600` in a directory with mode `0700`. Only after successful signature verification is the file chmodded to `0755` and atomically renamed over the target executable.

---

## 5. Web Server & SSE Security (19.6)

### Threat Model
A web browser running malicious websites could attempt DNS rebinding to access PebblePost's REST and SSE API endpoints running on `127.0.0.1`. Plaintext API tokens in logs or URL query strings could be leaked to disk, browser history, or proxy access logs.

### Defense Implementation
- **Host Header Validation**: `security.HostHeaderMiddleware` enforces that all incoming HTTP requests target `localhost`, `127.0.0.1`, or `[::1]`. External host headers are rejected with `403 Forbidden`.
- **Token Masking**: Server startup logs mask the API token (e.g. `a1b2...c3d4`) to prevent token leakage into systemd journals, terminal scrollbacks, or container logs. The full token is written to `.token` in the data directory with `0600` permissions.
- **SSE Token Handling**: Native browser `EventSource` lacks custom header support. While `?token=` query parameters are accepted by `TokenMiddleware` for loopback EventSource connections, frontend components (`RunnerPanel.tsx`) prioritize streaming `fetch()` with `Authorization: Bearer <token>` and `Accept: text/event-stream` headers, keeping tokens completely out of URLs and web server access logs.

---

## 6. CLI Runner Script Trust Mechanism (19.7)

### Threat Model
Running untrusted collections from GitHub or external sources could silently execute arbitrary JavaScript in pre-request and post-response hooks.

### Defense Implementation
- By default, `pebblepost run` executes collections with script execution **disabled** unless explicit trust is granted.
- **Explicit Trust Flags**:
  - Command-line flag: `pebblepost run ./my-collection --trust`
  - Trust marker file: Creating a `.pebbletrust` file in the collection root or workspace root.
  - Environment variable: `PEBBLEPOST_TRUST=1` or `PEBBLEPOST_TRUST_SCRIPTS=1`
- When untrusted, requests run normally but scripts are skipped and a security warning notice is logged.

---

## 7. Output Encoding & XSS Prevention (19.8)

### Threat Model
Hostile API collections or response payloads containing `<script>` or event handlers could attempt Cross-Site Scripting (XSS) in HTML previews, generated documentation, or Markdown views.

### Defense Implementation
- **HTML Docs Generator**: `html.go` escapes all titles, descriptions, URLs, query parameters, headers, and code snippets with `html.EscapeString`.
- **Anchor Slug Sanitization**: `slugify` in `markdown.go` strictly sanitizes anchors to `[a-z0-9_-]`, preventing quote injection into `onclick` handlers or HTML attributes.
- **Response HTML Preview**: The response preview iframe in `ResponsePanel.tsx` uses `sandbox=""`, strictly disabling script execution and same-origin access.
- **Markdown Link Whitelist**: `MarkdownView.tsx` restricts link `href` targets to `http:`, `https:`, `mailto:`, or `#`. Dangerous protocols (`javascript:`, `data:`, `vbscript:`) are neutralized to `#`.
- **Importer Confinement**: Imported folders and requests use `security.SafeJoin` to guarantee imported items never escape the target directory.

---

## 8. File Modes & Windows Security (19.9)

### Unix Permissions
On Linux and macOS, all sensitive files and directories are created with restrictive permissions:
- SQLite History database (`history.db`, WAL, and SHM): `0600` (directory: `0700`)
- Cookie jar (`.pebble/cookies.json`): `0600` (directory `.pebble`: `0700`)
- Secret environment variables (`*.secret.env.json`): `0600` (directory: `0700`)
- Server API token file (`.token`): `0600` (data directory: `0700`)

### Windows Permissions & Limitations
- Windows NTFS does not use POSIX permission bits; `os.Chmod` on Windows only toggles the `FILE_ATTRIBUTE_READONLY` flag and cannot set discretionary access controls.
- On Windows, security relies on NTFS Access Control Lists (ACLs) inherited from the user's home profile (`%USERPROFILE%` / `%APPDATA%`). Under default Windows user accounts, files inside the user profile directory are readable only by the current user and elevated Administrators.
- Windows named devices (`CON`, `PRN`, `AUX`, `NUL`, `COM1`-`COM9`, `LPT1`-`LPT9`) are explicitly rejected by `security.SafeJoin` and `security.ValidateSafeIdentifier`.
