# PebblePost File & Schema Specification

> Complete specification of directory layout, JSON schemas, environment variables, and Git integration.

---

## 1. Directory Structure

A PebblePost workspace maps directly to a directory on your local filesystem:

```
my-workspace/
├── .pebble/
│   ├── workspace.json                  # Workspace settings & default environment
│   ├── .gitignore                      # Generated automatically to ignore secrets
│   └── environments/
│       ├── dev.env.json                # Shared/committed environment variables
│       ├── dev.secret.env.json         # Sensitive secrets (ignored by Git)
│       ├── staging.env.json
│       └── prod.env.json
│
└── collections/                        # Root folder for API requests
    ├── auth/                           # Folders map 1:1 to UI collection trees
    │   ├── login.pebble.json           # Request definitions
    │   └── refresh.pebble.json
    └── orders/
        ├── create-order.pebble.json
        └── get-order.pebble.json
```

---

## 2. Request Definition Schema (`*.pebble.json`)

Every request in PebblePost is stored as an individual JSON document:

```json
{
  "$schema": "https://pebblepost.dev/schemas/v1/request.json",
  "version": "1.0",
  "name": "Create Order",
  "description": "Submit a new purchase order with line items",
  "method": "POST",
  "url": "{{BASE_URL}}/api/v1/orders",
  "headers": [
    {
      "key": "Content-Type",
      "value": "application/json",
      "enabled": true
    },
    {
      "key": "Authorization",
      "value": "Bearer {{AUTH_TOKEN}}",
      "enabled": true
    }
  ],
  "params": [
    {
      "key": "dryRun",
      "value": "false",
      "enabled": true
    }
  ],
  "auth": {
    "type": "bearer",
    "bearer": {
      "token": "{{AUTH_TOKEN}}"
    }
  },
  "body": {
    "type": "json",
    "raw": "{\n  \"items\": [\n    {\"sku\": \"PROD-101\", \"qty\": 2}\n  ],\n  \"shippingAddress\": \"123 Tech Ave\"\n}",
    "formData": [],
    "urlEncoded": []
  },
  "scripts": {
    "preRequest": "// Compute idempotency key\npb.request.headers.set('X-Idempotency-Key', pb.crypto.uuid());",
    "postResponse": "pb.test('Order created', () => {\n  pb.expect(pb.response.status).to.eql(201);\n});\nconst data = pb.response.json();\npb.environment.set('LAST_ORDER_ID', data.orderId);"
  },
  "settings": {
    "followRedirects": true,
    "verifySSL": true,
    "timeoutMs": 30000
  }
}
```

### Field Definitions

| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `version` | string | Yes | PebblePost schema version (`1.0`). |
| `name` | string | Yes | Display name in the tree and tab header. |
| `description` | string | No | Optional notes or markdown documentation. |
| `method` | string | Yes | HTTP Method: `GET`, `POST`, `PUT`, `DELETE`, `PATCH`, `HEAD`, `OPTIONS`. |
| `url` | string | Yes | Target endpoint URL. Supports `{{VAR}}` interpolation. |
| `headers` | array | Yes | Array of `{ key, value, enabled }` objects. |
| `params` | array | Yes | Query parameters `{ key, value, enabled }`. |
| `auth` | object | Yes | Authentication config (`none`, `bearer`, `basic`, `apiKey`). |
| `body` | object | Yes | Body configuration: `type` (`none`, `json`, `formData`, `urlEncoded`, `raw`, `graphql`). |
| `scripts` | object | No | `preRequest` and `postResponse` JavaScript snippets. |
| `settings` | object | No | Request execution flags (`followRedirects`, `verifySSL`, `timeoutMs`). |

---

## 3. Environment Variable Schema

### Shared Variables (`dev.env.json`)
Committed to Git for team sharing:

```json
{
  "name": "dev",
  "variables": [
    { "key": "BASE_URL", "value": "http://localhost:8080", "enabled": true },
    { "key": "API_VERSION", "value": "v1", "enabled": true }
  ]
}
```

### Secret Variables (`dev.secret.env.json`)
Automatically blocked from Git by `.pebble/.gitignore`:

```json
{
  "name": "dev-secrets",
  "variables": [
    { "key": "API_KEY", "value": "secret_live_9a8b7c6d5e", "enabled": true },
    { "key": "AUTH_TOKEN", "value": "eyJhbGciOi...", "enabled": true }
  ]
}
```

---

## 4. Variable Precedence & Interpolation

Variables are resolved using double-curly syntax `{{VARIABLE_NAME}}`. The engine evaluates variables in the following order of precedence (highest to lowest):

1. **Runtime In-Memory Variables**: Set dynamically during script execution via `pb.environment.set()`.
2. **Secret Environment Variables**: Loaded from `*.secret.env.json`.
3. **Public Environment Variables**: Loaded from `*.env.json`.

---

## 5. Git Merge Strategy

Because each API request is an isolated file:
- **No Monolithic Conflicts**: Multiple teammates can add, modify, or delete different requests simultaneously without merge conflicts.
- **Clear Git Diffs**: Reviewing a PR clearly shows headers, payloads, or scripts changed line-by-line.
