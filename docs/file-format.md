# PebblePost File Format & Schema Specification (v1)

This document specifies the storage formats, serialization stability rules, ordering precedence, and migration behavior used in PebblePost.

---

## 1. Design Principles

1. **Git-First & Diff-Friendly**:
   - Fixed key ordering.
   - Deterministic 2-space indentation.
   - Single trailing newline (`\n`).
   - Empty or default optional fields omitted via `omitempty` to prevent noisy diffs.
2. **Backward Compatibility**:
   - All files without `schemaVersion` are treated as **v0** and automatically migrated in memory on read.
   - **Disk files are never rewritten until the user explicitly saves**, preserving repository cleanliness.
3. **No Embedded Binary Bloat**:
   - Large request payloads and file uploads are referenced via relative paths (`filePath`), never base64-encoded strings inside JSON files.
4. **Flexible Hybrid Ordering**:
   - Retains full compatibility with numeric prefixes (`01-`, `02-`), while supporting folder-level sequencing (`_folder.pebble.json`) and explicit file order attributes.

---

## 2. Request Definition (`*.pebble.json`)

Each request in a collection is saved as an individual `.pebble.json` file.

### Schema Fields

| Field | Type | Description |
|---|---|---|
| `$schema` | string (optional) | Canonical JSON Schema URI (`https://pebblepost.dev/schemas/v1/request.json`) |
| `schemaVersion` | integer (required in v1) | Schema specification version (`1`) |
| `version` | string (optional) | Semantic version format string (e.g., `"1.0"`) |
| `id` | string (optional) | Unique request ID |
| `name` | string (required) | Human-readable name of the request |
| `description` | string (optional) | Markdown or plain-text description |
| `order` | integer (optional) | Explicit ordering index (overrides numeric prefix) |
| `method` | string (required) | HTTP verb (`GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `HEAD`, `OPTIONS`) |
| `url` | string (required) | URL, optionally containing `{{VARIABLE}}` placeholders |
| `headers` | array of KeyValue | HTTP request headers |
| `params` | array of KeyValue | URL query parameters |
| `auth` | AuthDefinition | Authentication configuration |
| `body` | BodyDefinition | Request body payload or relative file reference |
| `scripts` | ScriptDefinition | Pre-request and post-response JavaScript snippets |
| `settings` | SettingDefinition | Request execution options |

### Example `login.pebble.json`

```json
{
  "$schema": "https://pebblepost.dev/schemas/v1/request.json",
  "schemaVersion": 1,
  "version": "1.0",
  "name": "User Login",
  "order": 1,
  "method": "POST",
  "url": "{{BASE_URL}}/api/v1/auth/login",
  "headers": [
    {
      "key": "Content-Type",
      "value": "application/json",
      "enabled": true
    }
  ],
  "auth": {
    "type": "none"
  },
  "body": {
    "type": "json",
    "raw": "{\n  \"email\": \"user@example.com\",\n  \"password\": \"{{PASSWORD}}\"\n}"
  },
  "scripts": {
    "postResponse": "pb.test(\"Login succeeds\", () => {\n  pb.expect(pb.response.status).to.eql(200);\n  pb.env.set(\"TOKEN\", pb.response.json().token);\n});"
  },
  "settings": {
    "followRedirects": true,
    "verifySSL": true,
    "timeoutMs": 30000
  }
}
```

---

## 3. Large Bodies & File References

To keep Git repositories performant and diffs readable, large body files or binary uploads are referenced via `filePath` relative to the workspace root:

```json
{
  "body": {
    "type": "file",
    "filePath": "fixtures/large-payload.json"
  }
}
```

At execution time, the HTTP client reads the contents directly from the specified file on disk into the request stream.

---

## 4. Environment Definitions (`*.env.json` & `*.secret.env.json`)

Environments store reusable variable maps.

```json
{
  "schemaVersion": 1,
  "name": "staging",
  "variables": [
    {
      "key": "BASE_URL",
      "value": "https://staging.api.example.com",
      "enabled": true
    },
    {
      "key": "API_KEY",
      "value": "sk_stage_12345",
      "enabled": true,
      "isSecret": true
    }
  ]
}
```

- `*.secret.env.json` files contain sensitive credentials and are automatically ignored by Git (enforced via `.gitignore`).

---

## 5. Folder Ordering Metadata (`_folder.pebble.json`)

To organize requests inside a directory without renaming files on disk, an optional `_folder.pebble.json` file can be placed inside any collection folder:

```json
{
  "schemaVersion": 1,
  "name": "Authentication",
  "description": "User login, registration, and password recovery endpoints",
  "order": 1,
  "itemOrder": [
    "01-register.pebble.json",
    "02-login.pebble.json",
    "refresh-token.pebble.json",
    "logout.pebble.json"
  ]
}
```

### Ordering Resolution (Hybrid Hierarchy)

When rendering collection items in the explorer tree, PebblePost evaluates order with the following precedence:

1. **Item Type**: Directories are grouped first, followed by files.
2. **Folder `itemOrder` Sequence**: If `_folder.pebble.json` exists in the parent directory, items listed in `itemOrder` appear in the declared sequence.
3. **Explicit `order` Field**: An explicit `order` number in a request or folder takes next precedence.
4. **Numeric Filename/Folder Prefix**: Legacy numeric prefixes such as `01-`, `02_`, or `10.` establish sequence when no explicit order is present.
5. **Alphabetical Fallback**: Remaining items are sorted alphabetically by their cleaned names (case-insensitive).

The UI strips numeric prefixes for display (`01-auth` displays cleanly as `"auth"`), while keeping files in their defined order.

---

## 6. Migration from v0 to v1

Files created in earlier PebblePost versions lack the `schemaVersion` attribute.
- **In-Memory Read Migration**:
  - `MigrateRequest`: When `req.SchemaVersion < 1`, it assigns `SchemaVersion = 1`, `$schema = DefaultRequestSchema`, and `Version = "1.0"`.
  - `MigrateEnvironment`: Assigns `SchemaVersion = 1`.
  - `MigrateWorkspace`: Assigns `SchemaVersion = 1`.
- **Disk Immutability Guarantee**:
  - Reading a file does **not** touch or rewrite the disk.
  - Disk serialization only occurs when the user explicitly triggers a save action.

---

## 7. File Watcher & Conflict Resolution

PebblePost features a real-time filesystem watcher:

1. **Debouncing**: Changes are debounced over a ~200ms window to batch rapid modifications into a single event.
2. **Self-Write Suppression**: Writes originated by PebblePost are registered in a suppression journal (`w.Suppress(path, ttl)`) so self-saves do not trigger false conflict alerts.
3. **SSE Notification**: Real-time events are streamed to the frontend via `GET /api/workspace/events`.
4. **Conflict Handling**:
   - If an open request has unsaved changes and is modified externally on disk, PebblePost displays a **Conflict Dialog**:
     - **Keep Mine**: Preserves the editor's dirty state.
     - **Reload from Disk**: Discards unsaved edits and refreshes with the new disk content.
     - **View Diff**: Shows a side-by-side comparison between the local in-memory version and the disk version.
