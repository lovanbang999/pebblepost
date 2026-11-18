# Security Notes

The Rust migration is ongoing; this document describes current safeguards and known limitations, not a completed security audit.

## Workspace files

The shared Rust core canonicalizes workspace roots and confines request, folder, and mutation paths to those roots. It rejects traversal and symlinks that resolve outside the workspace. Secret environment files are ignored by the workspace Git rules and written with mode `0600` on Unix.

## Local API server

The server validates the Host header and limits CORS to local origins. API routes require a bearer token when one is configured. Loopback CLI serving can run without a token; non-loopback serving requires `--token` or `PEBBLEPOST_TOKEN`. A supplied token is stored as `data/.token` with mode `0600` on Unix. Do not expose a non-loopback server to an untrusted network.

## Secrets and script execution

File-backed secret environment values remain in plaintext JSON files, though Unix permissions are restricted. The Rust core also supports OS keychain storage and configurable process-environment secrets; their platform behavior still needs comparison with Go. Request scripts run in a resource-limited JavaScript runtime; `pb.sendRequest` blocks known cloud metadata hosts and caps calls and response size. Treat scripts and collections as executable code and run untrusted workspaces with script trust disabled. The previous runtime's sandbox guarantees have not been independently re-established for the Rust engine.

## Network requests

PebblePost intentionally makes outbound requests to the URLs in a workspace. The Rust server does not yet reproduce all of the previous SSRF defenses, redirect restrictions, client-certificate controls, or request timing instrumentation. Use workspaces from trusted sources and restrict the server's network access when handling untrusted collections.

## Software updates

The updater accepts HTTPS download URLs and refuses versions that are not newer than the running build. Configure `PEBBLEPOST_UPDATE_PUBLIC_KEY` with a trusted Ed25519 or Minisign public key to require signature verification for signed releases. Without a configured key and without a signature URL, the current updater permits an unsigned asset; do not use that mode for production updates. Replacement behavior still needs platform review.

## Reporting issues

Report vulnerabilities privately using the contact instructions in [`SECURITY.md`](../SECURITY.md). Do not include API keys, tokens, passwords, or private request data in reports or logs.
