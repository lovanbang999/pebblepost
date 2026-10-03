# PebblePost Scripting Guide & `pb.*` API Reference

PebblePost provides an embedded JavaScript runtime powered by [Goja](https://github.com/dop251/goja). Scripts execute in an isolated sandbox before requests are dispatched (**Pre-request scripts**) and after responses are received (**Post-response test scripts**).

---

## 1. Engine Architecture & Runtime Limits

### Supported JavaScript Standards
PebblePost runs on **ECMAScript 5.1** with substantial **ES6 (ES2015+)** language support:
- `let` and `const` block-scoped declarations
- Arrow functions: `(x) => x * 2`
- Template literals: `` `Bearer ${token}` ``
- Classes and object shorthand methods
- Destructuring assignment: `const { id, user } = data;`
- Rest parameters and spread syntax: `...args`, `[...items]`
- Native data structures: `Map`, `Set`, `Symbol`, `Promise`
- Standard built-in objects: `JSON`, `Math`, `Date`, `RegExp`

### Goja Engine Limits & Unsupported Syntax
Scripts run in a deterministic, synchronous sandbox without external Node.js binaries:
1. **No `async` / `await`**:
   Scripts in PebblePost execute synchronously from start to finish. Functions declared with `async` and expressions using `await` are rejected. For auxiliary API requests, use synchronous `pb.sendRequest()`.
2. **No CommonJS `require()` or ESM `import` / `export`**:
   External modules cannot be imported via `require(...)` or `import ...`. All cryptography, utility, encoding, and networking functions are provided directly via the global `pb` and `console` objects.
3. **No Generator Functions**:
   `function*` and `yield` statements are not supported in the Goja runtime.
4. **No Operating System or Process Access**:
   Variables like `process`, `child_process`, `fs`, `window`, and `document` are intentionally absent for sandbox security.

### Static Syntax Pre-Check
Before any script executes, PebblePost runs a static syntax analysis. If unsupported syntax (such as `async function`, `await`, `require()`, or `import`) is detected, execution aborts immediately with a clear error specifying the script name, line, and column instead of failing silently.

---

## 2. Console API & Diagnostics

Scripts support all standard console logging methods:
- `console.log(...args)` - standard output
- `console.info(...args)` - informational messages
- `console.warn(...args)` - warning notices
- `console.error(...args)` - error reports

### Output & Error Formatting
1. Logs are displayed in the **Console** tab in the response panel.
2. Each log entry records:
   - **Timestamp**: e.g., `14:23:05.120`
   - **Level**: `LOG`, `INFO`, `WARN`, `ERROR`
   - **Source**: `Pre-request` or `Post-response`, including folder provenance if inherited from `_folder.pebble.json`
   - **Message**: Stringified arguments (objects are formatted cleanly)
3. If an uncaught error or assertion failure occurs, the console shows the script name, line, column, and a concise stack trace.

---

## 3. Global `pb.*` API Reference

All PebblePost APIs are exposed under the global `pb` object (and aliased to `pm` for Postman script compatibility).

### 3.1 `pb.uuid()`
Generates a cryptographically random RFC 4122 v4 UUID string.

```javascript
const correlationId = pb.uuid();
pb.request.headers.set("X-Correlation-ID", correlationId);
console.log("Generated Correlation ID:", correlationId);
```

---

### 3.2 `pb.base64`
Encode and decode standard and URL-safe Base64 strings.

- `pb.base64.encode(string): string`
- `pb.base64.decode(string): string`

```javascript
const encoded = pb.base64.encode("admin:secretPassword");
pb.request.headers.set("Authorization", "Basic " + encoded);

const decoded = pb.base64.decode(encoded);
console.log("Decoded credentials:", decoded);
```

---

### 3.3 `pb.hash`
Fast cryptographic hashing returning lowercase hex strings.

- `pb.hash.sha256(string): string`
- `pb.hash.md5(string): string`

```javascript
const bodyHash = pb.hash.sha256(pb.request.body.raw || "");
pb.request.headers.set("X-Payload-SHA256", bodyHash);
```

---

### 3.4 `pb.hmac`
Keyed-hash message authentication code generator.

- `pb.hmac.sha256(secret: string, message: string): string`

```javascript
const secret = pb.environment.get("API_SECRET");
const timestamp = pb.date.now().toString();
const signature = pb.hmac.sha256(secret, timestamp + ":" + pb.request.url);

pb.request.headers.set("X-Timestamp", timestamp);
pb.request.headers.set("X-Signature", signature);
```

---

### 3.5 `pb.jwt`
JWT (JSON Web Token) parser that extracts unverified headers and claims.

- `pb.jwt.decode(token: string): { header: object, payload: object }`

```javascript
const authHeader = pb.request.headers.get("Authorization") || "";
const token = authHeader.replace(/^Bearer\s+/i, "");

if (token) {
  const jwt = pb.jwt.decode(token);
  console.info("Token subject:", jwt.payload.sub);
  console.info("Token expires at:", new Date(jwt.payload.exp * 1000).toISOString());
}
```

---

### 3.6 `pb.date`
Date arithmetic and formatting utilities.

- `pb.date.now(): number` - Unix epoch time in milliseconds.
- `pb.date.nowISO(): string` - Current UTC time in RFC 3339 format.
- `pb.date.format(dateVal, formatStr): string` - Formats date values (epoch ms, ISO string, or `time.Time`). Supports tokens `YYYY`, `MM`, `DD`, `HH`, `mm`, `ss`, `SSS`.
- `pb.date.add(dateVal, amount: number, unit: string): string` - Adds duration. Supported units: `ms`, `s`, `m`, `h`, `d`.

```javascript
// Current timestamp
const nowMs = pb.date.now();
const isoNow = pb.date.nowISO();

// Format date
const today = pb.date.format(nowMs, "YYYY-MM-DD");

// Add 7 days to date
const expiry = pb.date.add(isoNow, 7, "days");
console.log("Calculated license expiry:", expiry);
```

---

### 3.7 `pb.random`
Cryptographically secure random generators.

- `pb.random.int(min: number, max: number): number` - Random integer between `min` and `max` inclusive.
- `pb.random.string(length: number, charset?: string): string` - Random string of specified length. Default charset: alphanumeric `[a-zA-Z0-9]`.

```javascript
const nonce = pb.random.string(16);
const randomPort = pb.random.int(8000, 9000);
pb.request.headers.set("X-Nonce", nonce);
```

---

### 3.8 Variables Scoping: `pb.variables` vs `pb.environment`

PebblePost distinguishes between **run-local** variables and **persistent environment** variables:

| Scope | Object | Lifespan | Description |
| :--- | :--- | :--- | :--- |
| **Run-Local** | `pb.variables` | Single request / Collection run | In-memory variables for passing data between folder scripts and request scripts. Does not overwrite disk files. |
| **Environment** | `pb.environment` | Active Workspace Environment | Reads and updates workspace environment variables. Updated values persist to active `*.env.json`. |

#### Methods on both `pb.variables` and `pb.environment`:
- `.get(key: string): string`
- `.set(key: string, value: any): void`
- `.has(key: string): boolean`
- `.unset(key: string): void`
- `.toObject(): Record<string, string>`

```javascript
// Run-local variable
pb.variables.set("tempNonce", pb.random.string(8));

// Persistent environment variable
pb.environment.set("ACCESS_TOKEN", "new-token-12345");
```

---

### 3.9 Synchronous Auxiliary Requests: `pb.sendRequest`
Performs an auxiliary synchronous HTTP call inside a script (useful for pre-fetching CSRF tokens, rotating OAuth2 tokens, or calling health endpoints before running tests).

- `pb.sendRequest(config: object | string): object`

**Config parameters:**
- `url` (string, required): Target URL
- `method` (string, optional, default: `"GET"`)
- `headers` (object, optional): Key-value request headers
- `body` (string | object, optional): Raw string or JSON payload
- `timeoutMs` (number, optional): Custom timeout (cannot exceed the script's remaining timeout budget)

**Returned Response Object:**
- `status`: HTTP status code (number, e.g. `200`)
- `statusText`: Status text (string, e.g. `"200 OK"`)
- `headers`: Response headers dictionary
- `body`: Raw string response body
- `text()`: Returns response body string
- `json()`: Parses response body as JSON
- `duration`: Roundtrip time in milliseconds

```javascript
// Example: Pre-request token exchange
const tokenRes = pb.sendRequest({
  url: "https://auth.example.com/oauth/token",
  method: "POST",
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({
    grant_type: "client_credentials",
    client_id: pb.environment.get("CLIENT_ID"),
    client_secret: pb.environment.get("CLIENT_SECRET")
  }),
  timeoutMs: 3000
});

if (tokenRes.status === 200) {
  const tokenData = tokenRes.json();
  pb.request.headers.set("Authorization", "Bearer " + tokenData.access_token);
  console.info("Auxiliary token acquired successfully");
} else {
  console.error("Token exchange failed with status:", tokenRes.status);
}
```

---

### 3.10 `pb.request` (Pre-request Script)
Inspect or mutate the outgoing HTTP request:

```javascript
// URL & Method
pb.request.url = pb.request.url + "?page=1";
pb.request.method = "POST";

// Headers
pb.request.headers.set("X-Api-Key", "secret-key");
pb.request.headers.add("X-Trace-Id", pb.uuid());
const auth = pb.request.headers.get("Authorization");
pb.request.headers.remove("Unwanted-Header");

// Body mutation
if (pb.request.body.raw) {
  const payload = JSON.parse(pb.request.body.raw);
  payload.updatedAt = pb.date.nowISO();
  pb.request.body.setRaw(JSON.stringify(payload));
}
```

---

### 3.11 `pb.response` (Post-response Test Script)
Access response properties and payload:

- `pb.response.status` / `pb.response.code`: Status code (number)
- `pb.response.statusText`: Status line text (string)
- `pb.response.headers.get(name)`: Get response header
- `pb.response.headers.has(name)`: Check if response header exists
- `pb.response.duration` / `pb.response.time`: Total duration in milliseconds
- `pb.response.size`: Payload size in bytes
- `pb.response.text()`: Raw body string
- `pb.response.json()`: Parsed JSON body

---

## 4. Assertion Library: `pb.test` & `pb.expect`

Tests are defined with `pb.test(name, callback)` and assertions are constructed using Chai BDD syntax via `pb.expect(value)`.

### 4.1 Status Matchers
```javascript
pb.test("Status code is 200 OK", function() {
  pb.expect(pb.response).to.have.status(200);
  pb.expect(pb.response.status).to.have.status(200);
  pb.expect(pb.response).to.not.have.status(500);
});
```

### 4.2 Property Matchers
```javascript
pb.test("Response contains user profile", function() {
  const data = pb.response.json();
  pb.expect(data).to.have.property("id");
  pb.expect(data).to.have.property("role", "admin");
});
```

### 4.3 Length Matchers
```javascript
pb.test("Items list has correct count", function() {
  const data = pb.response.json();
  pb.expect(data.items).to.have.length(10);
  pb.expect(data.tags).to.have.lengthOf(3);
});
```

### 4.4 Deep Equality
```javascript
pb.test("Config matches expected object", function() {
  const config = pb.response.json().config;
  pb.expect(config).to.deep.equal({
    retries: 3,
    debug: false,
    endpoints: ["us-east", "eu-west"]
  });
});
```

### 4.5 Existence & Type Checks
```javascript
pb.test("Token exists and is string", function() {
  const data = pb.response.json();
  pb.expect(data.token).to.exist;
  pb.expect(data.token).to.be.a("string");
  pb.expect(data.error).to.not.exist;
});
```

### 4.6 One Of Checks
```javascript
pb.test("Status is accepted or OK", function() {
  pb.expect(pb.response.status).to.be.oneOf([200, 201, 202]);
});
```

### 4.7 JSON Schema Validation
Validate structured JSON against a schema:

```javascript
pb.test("Response body matches user JSON schema", function() {
  const schema = {
    type: "object",
    required: ["id", "username", "email", "roles"],
    properties: {
      id: { type: "integer", minimum: 1 },
      username: { type: "string", minLength: 3, maxLength: 30 },
      email: { type: "string", pattern: "^[^@]+@[^@]+\\.[^@]+$" },
      roles: {
        type: "array",
        minItems: 1,
        items: { type: "string", enum: ["admin", "user", "editor"] }
      }
    }
  };

  pb.expect(pb.response.json()).to.have.jsonSchema(schema);
});
```
If schema validation fails, PebblePost reports a human-readable diff:
```
JSON schema validation failed:
  • root.email: string does not match pattern '^[^@]+@[^@]+\.[^@]+$'
  • root.roles[0]: value "guest" is not in enum ["admin","user","editor"]
```
