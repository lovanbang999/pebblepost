# PebblePost Roadmap – Progress Tracker

Place this file at `docs/ROADMAP.md` in the repo. Antigravity updates it at the end of every prompt.

## Legend & update rules

- `[ ]` not started
- `[~]` partially done (add a short note after the item: what is missing)
- `[x]` done: implemented **and** tests pass **and** a manual verification step was given

Rules for whoever updates this file:
1. Only touch items belonging to the prompt you just worked on.
2. Never mark `[x]` without passing tests. If tests could not be run, use `[~]` and say why.
3. Do not delete or reword items. If something new is needed, append it at the bottom of that prompt's list as a new numbered item.
4. After updating, refresh the Overview table counts and add a line to the Changelog at the bottom (date, prompt, summary, commit range).

---

## Overview

| # | Prompt | Phase | Status | Done |
|---|---|---|---|---|
| 1 | Security hardening | P0 | `[x]` | 10/10 |
| 2 | Schema v1, ordering, file watcher | P0 | `[x]` | 8/8 |
| 3 | Multi-tab & tree management | P0 | `[ ]` | 0/9 |
| 4 | Folder-level inheritance | P0 | `[ ]` | 0/7 |
| 5 | Auth, cookies, request settings | P0 | `[ ]` | 0/10 |
| 6 | Script console & `pb.*` API | P0 | `[ ]` | 0/8 |
| 7 | Test infrastructure & CI | P0 | `[ ]` | 0/7 |
| 8 | Advanced CLI | P1 | `[ ]` | 0/8 |
| 9 | Environment editor & variables | P1 | `[ ]` | 0/7 |
| 10 | History | P1 | `[ ]` | 0/6 |
| 11 | Importers/Exporters | P1 | `[ ]` | 0/7 |
| 12 | Advanced response viewer | P1 | `[ ]` | 0/7 |
| 13 | gRPC | P2 | `[ ]` | 0/7 |
| 14 | WebSocket & SSE | P2 | `[ ]` | 0/6 |
| 15 | Data-driven runs & Runner UI | P2 | `[ ]` | 0/6 |
| 16 | Examples & docs generation | P2 | `[ ]` | 0/5 |
| 17 | Packaging & distribution | P2 | `[ ]` | 0/8 |

---

# PHASE P0 – Stabilize

## Prompt 1 – Security hardening
- [x] 1.1 Audit report written (issues with file:line and severity) and approved
- [x] 1.2 Web server binds to 127.0.0.1 by default; `--host` requires a token
- [x] 1.3 Token auth on all REST and SSE endpoints
- [x] 1.4 Path traversal protection (`..`, absolute paths, symlinks, Windows names) with tests
- [x] 1.5 Strict CORS, body size limit, security headers
- [x] 1.6 Goja script timeout via `vm.Interrupt` (configurable, default 5s) with infinite-loop test
- [x] 1.7 Limits on call stack depth and console output size
- [x] 1.8 Verified scripts cannot reach filesystem/network/process outside `pb.*`
- [x] 1.9 `.gitignore` check/auto-add for `*.secret.env.json` (with user confirmation)
- [x] 1.10 Secret masking in logs/CLI/history + "trust this workspace" prompt before first script run

## Prompt 2 – Schema v1, ordering, file watcher
- [x] 2.1 `schemaVersion: 1` added to all file types
- [x] 2.2 Automatic v0 → v1 migration on read (no overwrite until Save), with tests
- [x] 2.3 Stable serialization (key order, indent, trailing newline) with golden tests
- [x] 2.4 Explicit ordering mechanism, compatible with numeric prefixes (options compared first)
- [x] 2.5 Large bodies/uploads stored as relative path references
- [x] 2.6 Watcher: debounce, self-write ignore
- [x] 2.7 Conflict dialog (Keep mine / Reload / View diff) with tests
- [x] 2.8 `docs/file-format.md` written

## Prompt 3 – Multi-tab & tree management
- [ ] 3.1 Preview tab (single click) and pinned tab (double click)
- [ ] 3.2 Unsaved indicator, Ctrl+S, confirm on close
- [ ] 3.3 Per-tab state (request, response, scroll, sub-tab)
- [ ] 3.4 Shortcuts: Ctrl+W, Ctrl+Tab, Ctrl+Shift+T, middle-click close
- [ ] 3.5 Restore tabs when reopening the workspace
- [ ] 3.6 Rename (F2), duplicate, delete with confirmation
- [ ] 3.7 Drag and drop move; open tabs follow renames/moves
- [ ] 3.8 Context menu for requests and folders
- [ ] 3.9 Atomic file operations (temp file + rename) with tests

## Prompt 4 – Folder-level inheritance
- [ ] 4.1 `_folder.pebble.json` schema (headers, auth, vars, scripts) + migration
- [ ] 4.2 Merge logic in `internal/`, shared by desktop/server/CLI
- [ ] 4.3 Precedence: root → parent → child → request, with table tests
- [ ] 4.4 Auth modes: inherit / none / specific
- [ ] 4.5 Folder scripts run before/after request scripts
- [ ] 4.6 "Folder settings" panel UI (Headers/Auth/Vars/Scripts)
- [ ] 4.7 "Inherited from …" badges on requests; CLI uses same logic

## Prompt 5 – Auth, cookies, request settings
- [ ] 5.1 Bearer, Basic, API Key (header/query)
- [ ] 5.2 Digest auth
- [ ] 5.3 OAuth2 Client Credentials + token refresh
- [ ] 5.4 OAuth2 Authorization Code + PKCE (system browser + loopback callback)
- [ ] 5.5 AWS Signature v4
- [ ] 5.6 Cookie jar with per-domain management panel and per-request toggle
- [ ] 5.7 Cookies never written into Git-tracked files
- [ ] 5.8 Timeouts, redirect control, TLS verify toggle (with warning)
- [ ] 5.9 Proxy (HTTP/SOCKS) and client certificate (mTLS)
- [ ] 5.10 httptest-based tests for every auth type, redirects, cookies, timeouts

## Prompt 6 – Script console & `pb.*` API
- [ ] 6.1 `console.log/info/warn/error` shown in a Console panel
- [ ] 6.2 Script errors show script name, line, column, short stack
- [ ] 6.3 Utilities: uuid, base64, hash, hmac, jwt.decode, date, random
- [ ] 6.4 `pb.response`, `pb.variables`, `pb.environment.get/set/unset`
- [ ] 6.5 `pb.sendRequest` (sync, honors timeout and sandbox)
- [ ] 6.6 New matchers: status, property, have.length, deep.equal, exist, oneOf, jsonSchema
- [ ] 6.7 Unsupported-syntax detection + Goja limits documented in `docs/scripting.md`
- [ ] 6.8 `pb.*` autocomplete in CodeMirror

## Prompt 7 – Test infrastructure & CI
- [ ] 7.1 Baseline coverage report per package (real numbers recorded below)
- [ ] 7.2 Fuzz tests for the interpolator
- [ ] 7.3 Golden tests for Postman and OpenAPI importers
- [ ] 7.4 Tests for runner and scripting packages
- [ ] 7.5 Vitest for store/lib logic
- [ ] 7.6 Playwright E2E for the main flows
- [ ] 7.7 GitHub Actions on ubuntu/macos/windows (vet, lint, tsc, tests, coverage) + Makefile targets

Coverage before/after: _fill in_

---

# PHASE P1 – Team/CI ready

## Prompt 8 – Advanced CLI
- [ ] 8.1 Reporters: cli, json, junit, html (+ `--out`)
- [ ] 8.2 JUnit XML validated against real CI consumers
- [ ] 8.3 `--var`, `--env-file`, `PEBBLE_VAR_*` with documented precedence
- [ ] 8.4 Secret masking in CLI output
- [ ] 8.5 Filters: `--folder`, `--request`, `--tag`
- [ ] 8.6 `--timeout`, `--retry`, `--delay`, `--bail`
- [ ] 8.7 Exit codes 0/1/2/3 documented and tested; `--dry-run`
- [ ] 8.8 `docs/ci.md` with a GitHub Actions example

## Prompt 9 – Environment editor & variables
- [ ] 9.1 Manage Environments dialog (add/edit/delete/duplicate)
- [ ] 9.2 Secret flag → stored in `*.secret.env.json`, hidden by default
- [ ] 9.3 `{{VAR}}` highlighting (defined/undefined) in all inputs
- [ ] 9.4 Hover tooltip with resolved value and source
- [ ] 9.5 Variable autocomplete on `{{` and warning before Send on undefined vars
- [ ] 9.6 Dynamic variables (`$uuid`, `$timestamp`, `$isoTimestamp`, `$randomInt`, `$randomEmail`)
- [ ] 9.7 Recursion protection and `{{` escape syntax, with tests

## Prompt 10 – History
- [ ] 10.1 Record every Send (resolved request, status, duration, size, truncated response)
- [ ] 10.2 Stored outside the Git workspace; storage choice documented
- [ ] 10.3 Secrets masked before saving
- [ ] 10.4 History panel with filters and text search
- [ ] 10.5 Reopen read-only / restore request; compare two responses
- [ ] 10.6 Size limit, auto-cleanup, Clear history

## Prompt 11 – Importers/Exporters
- [ ] 11.1 Bruno import
- [ ] 11.2 Insomnia v4 import
- [ ] 11.3 HAR import
- [ ] 11.4 Improved Postman v2.1 import (inheritance, variables, `pm.*` → `pb.*`)
- [ ] 11.5 Export to Postman v2.1 and OpenAPI 3
- [ ] 11.6 Import report (imported / skipped with reasons) + `pebblepost import` CLI
- [ ] 11.7 Golden tests with real files; malformed input never panics

## Prompt 12 – Advanced response viewer
- [ ] 12.1 Large responses: virtualization/truncation + streaming to temp file
- [ ] 12.2 Previews: image, PDF, sandboxed HTML, XML, hex
- [ ] 12.3 Ctrl+F search within response
- [ ] 12.4 JSONPath filter
- [ ] 12.5 Copy as cURL (actual request sent)
- [ ] 12.6 Redirect chain and timing tooltips for reused connections
- [ ] 12.7 Improved Assertions tab

---

# PHASE P2 – Differentiate

## Prompt 13 – gRPC
- [ ] 13.1 `grpc` request type in schema
- [ ] 13.2 Server reflection and `.proto` loading (library choice documented)
- [ ] 13.3 Unary calls
- [ ] 13.4 Server/client/bidirectional streaming with Cancel
- [ ] 13.5 UI: service/method picker, sample message, status/metadata/trailers
- [ ] 13.6 Assertions and `{{variables}}` work with gRPC
- [ ] 13.7 CLI support; tests against a fake gRPC server

## Prompt 14 – WebSocket & SSE
- [ ] 14.1 `websocket` and `sse` request types
- [ ] 14.2 Two-way log UI (filter, search, timestamps)
- [ ] 14.3 Connect/Disconnect, auto-reconnect, ping/pong, close codes
- [ ] 14.4 Script/assertion on received messages (usable in CLI)
- [ ] 14.5 Ring buffer memory limit
- [ ] 14.6 Tests with fake servers

## Prompt 15 – Data-driven runs & Runner UI
- [ ] 15.1 CSV/JSON data import with `{{data.*}}` variables
- [ ] 15.2 CLI `--data` and `--iterations`
- [ ] 15.3 Runner UI with live progress and per-iteration results
- [ ] 15.4 `pb.runner.setNextRequest` with loop protection
- [ ] 15.5 Aggregate report (pass rate, avg/p95)
- [ ] 15.6 Tests

## Prompt 16 – Examples & docs generation
- [ ] 16.1 "Save as example" (masked, size-limited)
- [ ] 16.2 `description` field (Markdown) for requests/folders
- [ ] 16.3 `pebblepost docs` generating md/html
- [ ] 16.4 In-app docs preview
- [ ] 16.5 Golden tests for docs output

## Prompt 17 – Packaging & distribution
- [ ] 17.1 GoReleaser + Wails builds on version tags
- [ ] 17.2 Small non-root Docker image with healthcheck
- [ ] 17.3 Homebrew / winget / AUR (status noted per channel)
- [ ] 17.4 Checksums and signatures (cosign/minisign)
- [ ] 17.5 Auto-update with signature verification (can be disabled)
- [ ] 17.6 Measured numbers: startup time, idle RAM, binary size
- [ ] 17.7 README (install, quick start, file format, CLI, honest comparison)
- [ ] 17.8 LICENSE, CONTRIBUTING, CHANGELOG, issue/PR templates

---

## Changelog

| Date | Prompt | Summary | Commits |
|---|---|---|---|
| 2026-10-02 | init | Placed `docs/ROADMAP.md` from roadmap template; all items start at `[ ]` | — |
| 2026-10-02 | 1 | Security hardening: token auth, path guard, CORS/headers, sandbox timeout/caps, gitignore protection, secret masking, and workspace trust | `010e994..6d39171` |
| 2026-10-02 | 2 | Schema v1, ordering, file watcher: schemaVersion 1, v0->v1 migration on read, stable serialization, Option C hybrid ordering, relative bodies, debounced watcher, conflict dialog, and file-format.md | `b78444e..e16dabd` |
