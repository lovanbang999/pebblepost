# PebblePost Scripting API Reference

> Complete API reference and guide for writing Pre-Request and Post-Response JavaScript scripts in PebblePost.

---

## 1. Overview

PebblePost embeds the [Goja](https://github.com/dop251/goja) JavaScript engine. Scripts run in a secure, sandboxed environment without external dependencies or Node.js runtime requirements.

Scripts can be defined in two phases:
1. **Pre-Request Script**: Runs before the HTTP request is dispatched. Use it to dynamically compute timestamps, calculate HMAC signatures, or mutate headers.
2. **Post-Response (Test) Script**: Runs immediately after receiving the response. Use it to assert response contracts, check performance thresholds, or extract tokens into environment variables.

---

## 2. Global Object: `pb`

All scripting utilities are exposed under the global `pb` namespace.

---

### 2.1. Environment: `pb.environment`

Manage active workspace environment variables.

```javascript
// Get an environment variable
const baseUrl = pb.environment.get("BASE_URL");

// Set or update an environment variable (persists for subsequent requests)
pb.environment.set("AUTH_TOKEN", "new-jwt-token");

// Check if a variable exists
if (pb.environment.has("REFRESH_TOKEN")) {
  // ...
}

// Unset a variable
pb.environment.unset("TEMPORARY_NONCE");
```

---

### 2.2. Request: `pb.request`

Inspect or modify the outgoing request during the **Pre-Request** phase.

```javascript
// Get or set request headers
pb.request.headers.set("X-Timestamp", Date.now().toString());
const auth = pb.request.headers.get("Authorization");

// Set query parameters
pb.request.params.set("page", "1");
pb.request.params.set("limit", "50");

// Access or modify URL and method
pb.request.url = pb.request.url + "?debug=true";
pb.request.method = "POST";

// Modify JSON body
if (pb.request.body.type === "json") {
  const jsonBody = JSON.parse(pb.request.body.raw);
  jsonBody.timestamp = new Date().toISOString();
  pb.request.body.raw = JSON.stringify(jsonBody);
}
```

---

### 2.3. Response: `pb.response`

Available only in the **Post-Response** phase.

| Property / Method | Type | Description |
| :--- | :--- | :--- |
| `pb.response.status` | number | HTTP status code (e.g., `200`, `404`). |
| `pb.response.statusText` | string | HTTP status text (e.g., `OK`, `Not Found`). |
| `pb.response.duration` | number | Total roundtrip response duration in milliseconds. |
| `pb.response.size` | number | Response payload size in bytes. |
| `pb.response.headers` | object | Header map (e.g., `pb.response.headers.get('content-type')`). |
| `pb.response.text()` | string | Raw response body as a string. |
| `pb.response.json()` | any | Parsed JSON object from response body. |

---

### 2.4. Test Assertions: `pb.test` & `pb.expect`

PebblePost provides Chai-compatible BDD assertion syntax:

```javascript
pb.test("Status code is 200 OK", function() {
  pb.expect(pb.response.status).to.eql(200);
});

pb.test("Response time is under 300ms", function() {
  pb.expect(pb.response.duration).to.be.below(300);
});

pb.test("Has JSON Content-Type", function() {
  pb.expect(pb.response.headers.get("content-type")).to.include("application/json");
});

pb.test("User object contains valid ID and email", function() {
  const data = pb.response.json();
  pb.expect(data).to.have.property("id");
  pb.expect(data.email).to.be.a("string");
  pb.expect(data.roles).to.be.an("array").that.includes("admin");
});
```

#### Supported Assertions:
- `.to.eql(val)` / `.to.equal(val)`
- `.to.not.eql(val)`
- `.to.be.a(type)` (e.g., `'string'`, `'number'`, `'array'`, `'object'`)
- `.to.have.property(prop)`
- `.to.include(itemOrSubstring)`
- `.to.be.below(num)` / `.to.be.above(num)`
- `.to.be.true` / `.to.be.false` / `.to.be.null`

---

### 2.5. Cryptography: `pb.crypto`

Built-in cryptographic utilities for API authentication and signing:

```javascript
// UUID v4 Generation
const traceId = pb.crypto.uuid();
pb.request.headers.set("X-Trace-Id", traceId);

// MD5 Hashing
const hash = pb.crypto.md5("input string");

// SHA-256 Hashing
const sha = pb.crypto.sha256("payload string");

// HMAC SHA-256 Signing
const secret = pb.environment.get("API_SECRET");
const signature = pb.crypto.hmacSHA256("message to sign", secret);
pb.request.headers.set("X-HMAC-Signature", signature);

// Base64 Encoding & Decoding
const encoded = pb.crypto.base64Encode("user:password");
const decoded = pb.crypto.base64Decode(encoded);
```

---

## 3. Practical Examples

### Example 1: OAuth2 Token Refresh Chain
```javascript
// Post-response script on POST /oauth/token
pb.test("Token successfully acquired", () => {
  pb.expect(pb.response.status).to.eql(200);
});

const body = pb.response.json();
if (body?.access_token) {
  pb.environment.set("ACCESS_TOKEN", body.access_token);
  pb.environment.set("REFRESH_TOKEN", body.refresh_token);
}
```

### Example 2: API Timestamp & HMAC Header
```javascript
// Pre-request script
const timestamp = Math.floor(Date.now() / 1000).toString();
const apiKey = pb.environment.get("API_KEY");
const apiSecret = pb.environment.get("API_SECRET");

const payloadToSign = `${pb.request.method}\n${pb.request.url}\n${timestamp}`;
const signature = pb.crypto.hmacSHA256(payloadToSign, apiSecret);

pb.request.headers.set("X-API-Key", apiKey);
pb.request.headers.set("X-Timestamp", timestamp);
pb.request.headers.set("X-Signature", signature);
```
