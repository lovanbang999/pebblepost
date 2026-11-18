# PebblePost Import & Code Generation Guide

> Step-by-step guide to importing existing API collections and generating client code snippets.

---

## 1. Smart Importers

PebblePost's Rust import engine is in `apps/pebblepost-server/src/import_export.rs` and supports a preview-then-save flow.

### Supported Formats

| Format | Source | Description |
| :--- | :--- | :--- |
| **cURL Command** | Browser DevTools / Terminal | Parses methods, headers, URLs, query parameters, and raw/form bodies. |
| **Postman** | Postman collection JSON | Imports folders, requests, auth, headers, and query parameters. |
| **OpenAPI / Swagger** | JSON or YAML specification | Converts paths and operations into request skeletons. |
| **HAR** | HAR JSON export | Imports captured HTTP requests. |
| **Insomnia** | Insomnia JSON export | Imports request resources. |
| **Bruno** | Bruno text collection | Supports basic request definitions. |

---

### How to Import

1. Click the **Import** button in the top navigation bar.
2. Select your import source tab:
   - **cURL**: Paste any single or multi-line cURL command string.
   - **Postman**: Click **Upload .json file** or paste raw collection JSON.
   - **OpenAPI**: Click **Upload .json, .yaml, or .yml file** or paste an OpenAPI 3.0 specification.
3. Click **Parse** to preview the detected requests.
4. Click **Save Requests to Workspace**. The files will be created as `*.pebble.json` inside your active workspace.

---

## 2. 1-Click Code Generation

PebblePost generates clean, idiomatic code snippets in 5 popular programming languages directly from your active request.

### Supported Targets

#### 1. cURL
Standard terminal command with multi-line escapes:
```bash
curl -X POST 'https://api.example.com/v1/users' \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <TOKEN>' \
  -d '{"name": "Alice"}'
```

#### 2. Rust (`reqwest`)
Async Rust code using `reqwest`:
```rust
use reqwest::Client;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let client = Client::new();
    let response = client
        .post("https://api.example.com/v1/users")
        .header("Content-Type", "application/json")
        .body(r#"{"name":"Alice"}"#)
        .send()
        .await?;
    println!("{}", response.text().await?);
    Ok(())
}
```

#### 3. Node.js (`fetch`)
Modern JavaScript (Node 18+ native `fetch`):
```javascript
const response = await fetch("https://api.example.com/v1/users", {
  method: "POST",
  headers: {
    "Content-Type": "application/json"
  },
  body: JSON.stringify({ name: "Alice" })
});
const data = await response.json();
```

#### 4. Python (`requests`)
Idiomatic Python with the `requests` library:
```python
import requests

url = "https://api.example.com/v1/users"
headers = {"Content-Type": "application/json"}
data = """{"name": "Alice"}"""

response = requests.post(url, headers=headers, data=data)
print(response.json())
```

#### 5. C# (`HttpClient`)
Modern .NET 8+ C# code:
```csharp
using var client = new HttpClient();
var request = new HttpRequestMessage(HttpMethod.Post, "https://api.example.com/v1/users");
request.Content = new StringContent("{\"name\": \"Alice\"}", Encoding.UTF8, "application/json");
var response = await client.SendAsync(request);
```

---

### How to Generate Code

1. Select any request in the collection explorer.
2. Click the **`</> Code`** button in the request bar.
3. Select your desired language tab (cURL, Rust, Node.js, Python, C#).
4. Click **Copy** to copy the snippet to your clipboard.
