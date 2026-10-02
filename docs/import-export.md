# PebblePost Import & Code Generation Guide

> Step-by-step guide to importing existing API collections and generating client code snippets.

---

## 1. Smart Importers

PebblePost features a built-in import engine in `internal/workspace/importer.go` with a 2-step preview flow: **Parse Preview** -> **Save to Workspace**.

### Supported Formats

| Format | Source | Description |
| :--- | :--- | :--- |
| **cURL Command** | Browser DevTools / Terminal | Parses methods, headers, URLs, query parameters, and raw/form bodies. |
| **Postman v2.1** | Postman Export (`.json`) | Flattens nested folders, maps auth types, headers, query params, and scripts. |
| **OpenAPI 3.0 / Swagger** | OpenAPI JSON / YAML Spec | Converts API paths and operations (`GET`, `POST`, etc.) into request skeletons. |

---

### How to Import

1. Click the **Import** button in the top navigation bar.
2. Select your import source tab:
   - **cURL**: Paste any single or multi-line cURL command string.
   - **Postman**: Click **Upload .json file** or paste raw collection JSON.
   - **OpenAPI**: Click **Upload .json file** or paste OpenAPI 3.0 JSON specification.
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

#### 2. Go (`net/http`)
Production-ready Go code utilizing standard library `net/http`:
```go
package main

import (
    "bytes"
    "fmt"
    "io"
    "net/http"
)

func main() {
    url := "https://api.example.com/v1/users"
    payload := []byte(`{"name": "Alice"}`)
    req, err := http.NewRequest("POST", url, bytes.NewBuffer(payload))
    // ...
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
3. Select your desired language tab (cURL, Go, Node.js, Python, C#).
4. Click **Copy** to copy the snippet to your clipboard.
