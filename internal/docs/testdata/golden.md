# PetStore API

## Table of Contents

- [Users API](#users-api)
  - [`GET` Get User Details](#get-get-user-details)
  - [`POST` Create New User](#post-create-new-user)
- [example](#example)
  - [`GET` Get Started (HTTPBin)](#get-get-started-httpbin)

---

## Users API

User management and authentication endpoints.

### GET Get User Details

Fetch a specific user profile by user ID.

**Endpoint**: `GET https://api.petstore.com/v1/users/:id`

#### Query Parameters

| Parameter | Example Value |
| :--- | :--- |
| `include_orders` | `true` |

#### Headers

| Header | Value |
| :--- | :--- |
| `Accept` | `application/json` |

#### Code Samples

<details><summary><b>cURL</b></summary>

```bash
curl -X GET 'https://api.petstore.com/v1/users/:id?include_orders=true' \
  -H 'Authorization: Bearer sample_bearer_token' \
  -H 'Accept: application/json' \
  --insecure \
  --no-location
```

</details>

<details><summary><b>Go</b></summary>

```go
package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func main() {
	req, err := http.NewRequest("GET", "https://api.petstore.com/v1/users/:id?include_orders=true", nil)
	if err != nil {
		panic(err)
	}

	req.Header.Set("Authorization", "Bearer sample_bearer_token")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	fmt.Printf("Status: %s\n%s\n", resp.Status, respBody)
}
```

</details>

<details><summary><b>Node.js (Fetch)</b></summary>

```javascript
// Node.js 18+ built-in fetch
const response = await fetch("https://api.petstore.com/v1/users/:id?include_orders=true", {
  method: "GET",
  headers: {
    "Accept": "application/json",
    "Authorization": "Bearer sample_bearer_token"
  }
});

const data = await response.json();
console.log(response.status, data);
```

</details>

<details><summary><b>Python</b></summary>

```python
import requests

url = "https://api.petstore.com/v1/users/:id?include_orders=true"
headers = {
    "Authorization": "Bearer sample_bearer_token",
    "Accept": "application/json"
}

response = requests.get(url, headers=headers)

print(response.status_code)
print(response.text)
```

</details>

<details><summary><b>C#</b></summary>

```csharp
using System;
using System.Net.Http;
using System.Threading.Tasks;

var client = new HttpClient();
var url = "https://api.petstore.com/v1/users/:id?include_orders=true";
client.DefaultRequestHeaders.Add("Authorization", "Bearer sample_bearer_token");
client.DefaultRequestHeaders.Add("Accept", "application/json");

var response = await client.GetAsync(url);
var responseBody = await response.Content.ReadAsStringAsync();

Console.WriteLine($"Status: {response.StatusCode}");
Console.WriteLine(responseBody);
```

</details>

#### Example Responses

##### 200 OK User Profile (`200 OK`)

<details><summary>Response Headers</summary>

| Header | Value |
| :--- | :--- |
| `Content-Type` | `application/json` |

</details>

```json
{"id": "usr_101", "name": "Alice Smith", "email": "alice@example.com"}
```

---

### POST Create New User

Register a new pet store customer.

**Endpoint**: `POST https://api.petstore.com/v1/users`

#### Headers

| Header | Value |
| :--- | :--- |
| `Content-Type` | `application/json` |

#### Request Body

```json
{"name": "Bob Jones", "email": "bob@example.com"}
```

#### Code Samples

<details><summary><b>cURL</b></summary>

```bash
curl -X POST 'https://api.petstore.com/v1/users' \
  -H 'Content-Type: application/json' \
  -H 'Content-Type: application/json' \
  --data-raw '{"name": "Bob Jones", "email": "bob@example.com"}' \
  --insecure \
  --no-location
```

</details>

<details><summary><b>Go</b></summary>

```go
package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func main() {
	body := strings.NewReader(`{"name": "Bob Jones", "email": "bob@example.com"}`)
	req, err := http.NewRequest("POST", "https://api.petstore.com/v1/users", body)
	if err != nil {
		panic(err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	fmt.Printf("Status: %s\n%s\n", resp.Status, respBody)
}
```

</details>

<details><summary><b>Node.js (Fetch)</b></summary>

```javascript
// Node.js 18+ built-in fetch
const response = await fetch("https://api.petstore.com/v1/users", {
  method: "POST",
  headers: {
    "Content-Type": "application/json"
  },
  body: `{"name": "Bob Jones", "email": "bob@example.com"}`
});

const data = await response.json();
console.log(response.status, data);
```

</details>

<details><summary><b>Python</b></summary>

```python
import requests

url = "https://api.petstore.com/v1/users"
headers = {
    "Content-Type": "application/json"
}
data = """
{"name": "Bob Jones", "email": "bob@example.com"}
"""

response = requests.post(url, headers=headers, data=data)

print(response.status_code)
print(response.text)
```

</details>

<details><summary><b>C#</b></summary>

```csharp
using System;
using System.Net.Http;
using System.Threading.Tasks;

var client = new HttpClient();
var url = "https://api.petstore.com/v1/users";
client.DefaultRequestHeaders.Add("Content-Type", "application/json");

var content = new StringContent(@"{""name"": ""Bob Jones"", ""email"": ""bob@example.com""}", System.Text.Encoding.UTF8, "application/json");
var response = await client.PostAsync(url, content);
var responseBody = await response.Content.ReadAsStringAsync();

Console.WriteLine($"Status: {response.StatusCode}");
Console.WriteLine(responseBody);
```

</details>

#### Example Responses

##### 201 Created Response (`201 Created`)

```json
{"id": "usr_102", "name": "Bob Jones", "email": "bob@example.com", "createdAt": "2026-10-04T12:00:00Z"}
```

---

## example

### GET Get Started (HTTPBin)

Sample request demonstrating PebblePost offline collection capabilities

**Endpoint**: `GET {{BASE_URL}}/get`

#### Query Parameters

| Parameter | Example Value |
| :--- | :--- |
| `source` | `pebblepost` |

#### Headers

| Header | Value |
| :--- | :--- |
| `Accept` | `application/json` |
| `User-Agent` | `PebblePost/0.1.0` |

#### Code Samples

<details><summary><b>cURL</b></summary>

```bash
curl -X GET '{{BASE_URL}}/get?source=pebblepost' \
  -H 'Accept: application/json' \
  -H 'User-Agent: PebblePost/0.1.0'
```

</details>

<details><summary><b>Go</b></summary>

```go
package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func main() {
	req, err := http.NewRequest("GET", "{{BASE_URL}}/get?source=pebblepost", nil)
	if err != nil {
		panic(err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "PebblePost/0.1.0")

	client := &http.Client{Timeout: 30000 * time.Millisecond}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	fmt.Printf("Status: %s\n%s\n", resp.Status, respBody)
}
```

</details>

<details><summary><b>Node.js (Fetch)</b></summary>

```javascript
// Node.js 18+ built-in fetch
const response = await fetch("{{BASE_URL}}/get?source=pebblepost", {
  method: "GET",
  headers: {
    "Accept": "application/json",
    "User-Agent": "PebblePost/0.1.0"
  }
});

const data = await response.json();
console.log(response.status, data);
```

</details>

<details><summary><b>Python</b></summary>

```python
import requests

url = "{{BASE_URL}}/get?source=pebblepost"
headers = {
    "Accept": "application/json",
    "User-Agent": "PebblePost/0.1.0"
}

response = requests.get(url, headers=headers)

print(response.status_code)
print(response.text)
```

</details>

<details><summary><b>C#</b></summary>

```csharp
using System;
using System.Net.Http;
using System.Threading.Tasks;

var client = new HttpClient();
var url = "{{BASE_URL}}/get?source=pebblepost";
client.DefaultRequestHeaders.Add("Accept", "application/json");
client.DefaultRequestHeaders.Add("User-Agent", "PebblePost/0.1.0");

var response = await client.GetAsync(url);
var responseBody = await response.Content.ReadAsStringAsync();

Console.WriteLine($"Status: {response.StatusCode}");
Console.WriteLine(responseBody);
```

</details>

---

