# Pluggable Secret Storage & Threat Model

PebblePost provides a pluggable secret storage architecture so sensitive environment variables (such as API keys, bearer tokens, passwords, and database credentials) do not have to reside in plain-text JSON files on disk.

---

## 1. Architecture Overview

Callers interact with secret storage through the `SecretStore` interface defined in `pebblepost/internal/secrets`:

```go
type SecretStore interface {
    Get(service, key string) (string, error)
    Set(service, key, value string) error
    Delete(service, key string) error
    List(service string) ([]string, error)
    Backend() SecretBackendType
}
```

Operations are scoped to a `service` (the environment name, e.g. `dev`, `staging`, `prod`) and a `key` (the variable identifier).

### Supported Backends

| Backend | Identifier | Description | Storage Location |
| :--- | :--- | :--- | :--- |
| **File** (Default) | `file` | Stores secrets in `*.secret.env.json` files | `.pebble/environments/<env>.secret.env.json` |
| **OS Keychain** | `keychain` | Stores secrets in the OS native credential store | OS Keychain (macOS) / Secret Service (Linux) / Credential Manager (Windows) |
| **Environment** | `env` | Resolves values dynamically from process environment variables | In-memory / OS process environment |

Configuration is saved in `.pebble/workspace.json`:

```json
{
  "schemaVersion": 1,
  "name": "my-workspace",
  "secretBackend": {
    "default": "keychain",
    "perEnv": {
      "ci": "env",
      "dev": "file"
    }
  }
}
```

---

## 2. Keyring Library Evaluation

Before adding a third-party dependency for OS keychain integration, multiple libraries were evaluated:

### Evaluated Libraries

1. **`github.com/zalando/go-keyring` (Selected)**
   - **Pros:**
     - Pure Go on macOS: invokes `/usr/bin/security` directly, eliminating the need for CGO during cross-compilation.
     - Linux support via native D-Bus Secret Service protocol (works with GNOME Keyring, KWallet, and KeePassXC).
     - Windows support via native Windows Credential Manager (`wincred`).
     - Minimal external dependencies and lightweight footprint.
     - Extensively tested and battle-proven in tools such as Kubernetes CLI extensions and Docker credential helpers.
   - **Cons:**
     - Requires a running D-Bus session bus daemon on Linux (not available in headless docker containers without extra setup).
     - Indexing: Native keychain APIs do not provide an enumeration/list API across items under a service without querying individual keys; PebblePost supplements this with an index stored in `~/.config/pebblepost/keychain-index.json`.

2. **`github.com/99designs/keyring`**
   - **Pros:**
     - Supports multiple pluggable backends (macOS Keychain, Linux Secret Service, pass, KWallet, encrypted file).
   - **Cons:**
     - Heavy dependency tree (pulls in gopass, libsecret, etc.).
     - Requires CGO on macOS for native Keychain access, breaking static `CGO_ENABLED=0` builds.
     - Significantly more complex API surface and larger binary overhead.

3. **`github.com/danieljoos/wincred`**
   - **Pros:**
     - Excellent, lightweight Windows Credential Manager wrapper.
   - **Cons:**
     - Windows only; requires maintaining separate implementations and build tags for macOS and Linux.

### Decision
`github.com/zalando/go-keyring` was selected because it delivers cross-platform coverage across macOS, Linux, and Windows while enabling **pure Go (CGO_ENABLED=0) compilation** on macOS and Linux. In headless or CI environments without an unlocked keyring daemon, the `env` backend provides native secrets resolution from process environment variables without disk dependencies.

---

## 3. Threat Model

This threat model outlines the security boundaries, capabilities of potential attackers, and what each backend protects against versus what it does not.

### Trust Boundaries & Assets
- **Protected Assets:** API keys, database passwords, OAuth tokens, private certificates, and cloud credentials.
- **Exposure Vectors:** Accidental Git commits, filesystem read access by other local processes, shared developer machine access, unencrypted backups, CI/CD artifact leakages, and execution log leaks.

---

### Backend 1: `file` (`*.secret.env.json`)

- **How it works:** Secrets are stored in JSON files formatted as `.pebble/environments/<name>.secret.env.json` with file permissions `0600` (readable only by the current user).
- **What it protects against:**
  - Accidental Git commits: `.pebble/environments/*.secret.env.json` is explicitly ignored in `.gitignore`.
  - Accidental export: PebblePost workspace exporters exclude `*.secret.env.json` files unless explicit secret export is requested.
  - Read access by other unprivileged local users (via `0600` POSIX file mode).
- **What it does NOT protect against:**
  - Any process running as the current user having direct filesystem read access.
  - Disk forensics on unencrypted filesystems.
  - Unencrypted backups or file synchronization tools (Dropbox, Google Drive) copying files in plain text.
  - Users inadvertently copying or emailing the workspace folder.

---

### Backend 2: `keychain` (OS Keychain / Secret Service / Credential Manager)

- **How it works:** Secrets are encrypted by the operating system using hardware-backed keys (Secure Enclave on macOS, TPM on Windows, or session keyring encryption on Linux).
- **What it protects against:**
  - Plain-text files at rest: No secret values ever touch the disk in the workspace repository.
  - Disk forensics: Secrets remain encrypted on disk even if an attacker gains raw disk image access.
  - Inadvertent repository sharing or public Git push: The repository folder contains zero secret tokens.
  - Read access by other users on a multi-user workstation.
- **What it does NOT protect against:**
  - Malicious applications running under the same user session once the keychain is unlocked.
  - Memory dumps of the running PebblePost process (`pebblepost run` or `pebblepost serve`).
  - Headless CI environments without an unlocked D-Bus daemon (for Linux CI, use the `env` backend).

---

### Backend 3: `env` (Process Environment & `${OS_ENV:VAR}`)

- **How it works:** Secrets are read directly from the process environment variables at runtime, or referenced via `${OS_ENV:VARIABLE_NAME}`.
- **What it protects against:**
  - Storing secrets on disk entirely: Ideal for CI/CD runners (GitHub Actions, GitLab CI, Jenkins) where secrets are injected as runner environment variables.
  - Accidental commits or workspace bundle leaks.
  - Ephemeral containers: Leaves no secret traces on disk upon container termination.
- **What it does NOT protect against:**
  - Inspection of `/proc/<pid>/environ` by processes with `ptrace` or root permissions.
  - CI step scripts dumping the environment (`env` or `printenv`) into job build logs (mitigated by PebblePost's internal masker).

---

## 4. Secret Masking Guarantees

PebblePost strictly guarantees that **resolved secret values are never logged, persisted in execution history, or emitted in test reports**:

1. **Resolution Masker Tracking:** Whenever secrets are resolved—whether from `SecretStore.Get()`, `${OS_ENV:VAR}` interpolation, or `.env` file parsing—the plaintext secret values are automatically registered with `security.MaskSecrets`.
2. **Log & Response Masking:** All execution logs, console logs, assertion failure messages, response bodies, and request history records are scanned and redacted with `***`.
3. **No Secret Leaks in History:** The internal history SQLite database stores requests with sensitive tokens masked prior to serialization.

---

## 5. Environment Variables & `.env` File Support

### `${OS_ENV:VAR}` Interpolation Syntax
Variables in PebblePost environments can reference host environment variables dynamically:

```json
{
  "key": "DATABASE_URL",
  "value": "postgres://user:${OS_ENV:DB_PASSWORD}@localhost:5432/mydb",
  "secret": true
}
```

- When evaluated, `${OS_ENV:DB_PASSWORD}` is resolved from `os.Getenv("DB_PASSWORD")`.
- The resolved value of `DB_PASSWORD` is automatically masked in all outputs.
- If the environment variable is not defined, a warning is emitted and the placeholder remains intact.

### `.env` File Support
PebblePost supports standard `.env` files via `--env-file <path>` or by placing a `.env` file in the workspace root:

```ini
# Database & API configuration
export API_URL=https://api.example.com
DB_PASSWORD="super-secret-password-123"
AUTH_TOKEN='bearer-token-abc'
PORT=8080 # default port
DYNAMIC_SECRET=${OS_ENV:SYSTEM_SECRET}
```

Features supported:
- `#` comments and inline comments (`KEY=VAL # comment`).
- `export KEY=VALUE` prefix.
- Single and double-quoted strings with escape sequences (`\n`, `\t`, `\"`, `\\`).
- Dynamic `${OS_ENV:VAR}` expansions inside `.env` values.
- Automatic secret classification: Keys containing `SECRET`, `PASSWORD`, `TOKEN`, `KEY`, `AUTH`, `CREDENTIAL`, or `PRIVATE` are automatically masked.

---

## 6. CLI Management & CI/CD Integration

### CLI Commands

```bash
# List secrets for an environment
pebblepost secrets list --env staging

# Get a decrypted secret value
pebblepost secrets get API_KEY --env staging

# Set a secret value
pebblepost secrets set API_KEY "my-secret-token" --env staging

# Delete a secret
pebblepost secrets delete API_KEY --env staging

# Migrate secrets from file to keychain (prompts for confirmation)
pebblepost secrets migrate --env staging --from file --to keychain

# Non-interactive migration for scripts
pebblepost secrets migrate --env staging --from file --to keychain --yes

# Rollback a migration using the created backup
pebblepost secrets rollback --env staging --backup .pebble/environments/staging.secret.env.json.bak.1775551200
```

### Running in CI Without Secret Files

In CI/CD environments where secrets are injected via GitHub Actions Secrets or environment variables:

```bash
# Option 1: Explicit secret backend flag
export API_TOKEN="ci-secret-token-123"
pebblepost run ./collections -e staging --secret-backend env

# Option 2: Environment variable override
export PEBBLEPOST_SECRET_BACKEND=env
pebblepost run ./collections -e staging
```

---

## 7. Build Tags & Testing
- **Cross-Platform Compilation:** Compiles seamlessly on Linux, macOS, and Windows with or without CGO (`CGO_ENABLED=0`).
- **Keychain Integration Test (`keychain`):** Tests live integration with the OS keychain:
  ```bash
  go test -tags keychain ./internal/secrets/...
  ```
- **IDE & Gopls Configuration:** To enable gopls analysis for build-tagged test files, ensure `-tags=keychain` is configured in your editor's `gopls.build.buildFlags` setting (preconfigured in `.vscode/settings.json`).
