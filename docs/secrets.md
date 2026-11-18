# Workspace Secrets

PebblePost currently stores private environment variables in `.pebble/environments/<name>.secret.env.json`. Public values use `<name>.env.json`. The Rust workspace loader merges both files by environment name and marks values from the secret file as secret.

## Protect secret files

- Keep `*.secret.env.json` out of version control. Workspace initialization adds the pattern to `.pebble/.gitignore`; the application also checks the workspace `.gitignore` rule.
- On Unix, the Rust core writes secret environment files with mode `0600` and stores them under the workspace metadata directory.
- These files are not encrypted. Use full-disk encryption and operating-system account protections for local data.
- Avoid including private values in request examples, issue reports, CI logs, or exported collections.

## Current backend support

The Rust core and CLI support file and OS keychain list/get/set/delete. Keychain credentials use the native macOS Keychain, Windows Credential Manager, or Linux Secret Service and keep a private key-name index under the user's config directory. Linux requires a running Secret Service provider. The `env` backend reads named process variables and supplies blank secret environment values during request execution; it is read-only, so list/set/delete are unavailable.

The CLI accepts `--secret-backend file|env|keychain` for runs. Secret migration copies values to the selected backend, backs up file-backed sources, and removes source keys after transfer. Rollback restores a backup and clears keys from the destination backend (`keychain` by default; choose `--backend file` for file destinations).

Environment interpolation also supports `${OS_ENV:NAME}` for values supplied by the server process. Prefer CI secret stores to committed files when running automated builds.
