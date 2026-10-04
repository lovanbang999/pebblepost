package docs

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"pebblepost/internal/types"
)

// GenerateAllSnippets produces code samples for cURL, Go, Node.js, Python, and C#.
func GenerateAllSnippets(req *types.RequestDefinition) CodeSnippets {
	if req == nil {
		return CodeSnippets{}
	}
	return CodeSnippets{
		CURL:   GenerateCURL(req),
		Go:     GenerateGo(req),
		Node:   GenerateNodeFetch(req),
		Python: GeneratePython(req),
		CSharp: GenerateCSharp(req),
	}
}

// shellEscape escapes single quotes for safe shell command interpolation.
func shellEscape(val string) string {
	return strings.ReplaceAll(val, "'", "'\\''")
}

func enabledKVs(kvs []types.KeyValue) []types.KeyValue {
	var res []types.KeyValue
	for _, kv := range kvs {
		if kv.Enabled && strings.TrimSpace(kv.Key) != "" {
			res = append(res, kv)
		}
	}
	return res
}

func buildQueryString(req *types.RequestDefinition) string {
	params := enabledKVs(req.Params)
	if len(params) == 0 {
		return ""
	}
	vals := url.Values{}
	for _, p := range params {
		vals.Add(p.Key, p.Value)
	}
	return "?" + vals.Encode()
}

func fullURL(req *types.RequestDefinition) string {
	u := req.URL
	if u == "" {
		u = "http://localhost:8080/api"
	}
	return u + buildQueryString(req)
}

func allHeaders(req *types.RequestDefinition) []types.KeyValue {
	headers := enabledKVs(req.Headers)

	switch req.Auth.Type {
	case "bearer":
		if req.Auth.Token != "" {
			headers = append([]types.KeyValue{{Key: "Authorization", Value: "Bearer " + req.Auth.Token, Enabled: true}}, headers...)
		}
	case "basic":
		if req.Auth.Username != "" || req.Auth.Password != "" {
			enc := base64.StdEncoding.EncodeToString([]byte(req.Auth.Username + ":" + req.Auth.Password))
			headers = append([]types.KeyValue{{Key: "Authorization", Value: "Basic " + enc, Enabled: true}}, headers...)
		}
	case "apiKey":
		if req.Auth.AddTo == "header" || req.Auth.AddTo == "" {
			key := req.Auth.Key
			if key == "" {
				key = "X-API-Key"
			}
			headers = append([]types.KeyValue{{Key: key, Value: req.Auth.Value, Enabled: true}}, headers...)
		}
	}
	return headers
}

// GenerateCURL generates a shell-compatible cURL command string.
func GenerateCURL(req *types.RequestDefinition) string {
	method := req.Method
	if method == "" {
		method = "GET"
	}
	u := fullURL(req)

	var lines []string
	lines = append(lines, fmt.Sprintf("curl -X %s '%s'", method, shellEscape(u)))

	for _, h := range allHeaders(req) {
		lines = append(lines, fmt.Sprintf("  -H '%s: %s'", shellEscape(h.Key), shellEscape(h.Value)))
	}

	switch req.Body.Type {
	case "json", "raw":
		lines = append(lines, "  -H 'Content-Type: application/json'")
		lines = append(lines, fmt.Sprintf("  --data-raw '%s'", shellEscape(req.Body.Raw)))
	case "urlEncoded":
		var parts []string
		for _, kv := range enabledKVs(req.Body.UrlEncoded) {
			parts = append(parts, fmt.Sprintf("%s=%s", kv.Key, kv.Value))
		}
		if len(parts) > 0 {
			lines = append(lines, fmt.Sprintf("  --data-urlencode '%s'", shellEscape(strings.Join(parts, "&"))))
		}
	case "formData":
		for _, kv := range enabledKVs(req.Body.FormData) {
			if kv.Type == "file" {
				lines = append(lines, fmt.Sprintf("  -F '%s=@%s'", shellEscape(kv.Key), shellEscape(kv.Value)))
			} else {
				lines = append(lines, fmt.Sprintf("  -F '%s=%s'", shellEscape(kv.Key), shellEscape(kv.Value)))
			}
		}
	}

	if !req.Settings.VerifySSL {
		lines = append(lines, "  --insecure")
	}
	if !req.Settings.FollowRedirects {
		lines = append(lines, "  --no-location")
	}

	return strings.Join(lines, " \\\n")
}

// GenerateGo generates Go net/http code.
func GenerateGo(req *types.RequestDefinition) string {
	method := req.Method
	if method == "" {
		method = "GET"
	}
	u := fullURL(req)
	headers := allHeaders(req)

	var bodySetup string
	bodyArg := "nil"

	if req.Body.Type == "json" || req.Body.Type == "raw" {
		bodySetup = fmt.Sprintf("\tbody := strings.NewReader(`%s`)\n", req.Body.Raw)
		bodyArg = "body"
	}

	var headerLines []string
	for _, h := range headers {
		headerLines = append(headerLines, fmt.Sprintf("\treq.Header.Set(%q, %q)", h.Key, h.Value))
	}

	clientTimeout := "\tclient := &http.Client{}"
	if req.Settings.TimeoutMs > 0 {
		clientTimeout = fmt.Sprintf("\tclient := &http.Client{Timeout: %d * time.Millisecond}", req.Settings.TimeoutMs)
	}

	headersBlock := ""
	if len(headerLines) > 0 {
		headersBlock = strings.Join(headerLines, "\n") + "\n\n"
	}

	return fmt.Sprintf(`package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func main() {
%s	req, err := http.NewRequest(%q, %q, %s)
	if err != nil {
		panic(err)
	}

%s%s
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	fmt.Printf("Status: %%s\n%%s\n", resp.Status, respBody)
}`, bodySetup, method, u, bodyArg, headersBlock, clientTimeout)
}

// GenerateNodeFetch generates modern Node.js fetch code.
func GenerateNodeFetch(req *types.RequestDefinition) string {
	method := req.Method
	if method == "" {
		method = "GET"
	}
	u := fullURL(req)
	headers := allHeaders(req)

	headersMap := make(map[string]string)
	for _, h := range headers {
		headersMap[h.Key] = h.Value
	}

	bodyPart := ""
	if req.Body.Type == "json" || req.Body.Type == "raw" {
		escapedBody := strings.ReplaceAll(req.Body.Raw, "`", "\\`")
		bodyPart = fmt.Sprintf(",\n  body: `%s`", escapedBody)
		headersMap["Content-Type"] = "application/json"
	}

	hdrJSON, _ := json.MarshalIndent(headersMap, "  ", "  ")

	return fmt.Sprintf(`// Node.js 18+ built-in fetch
const response = await fetch(%q, {
  method: %q,
  headers: %s%s
});

const data = await response.json();
console.log(response.status, data);`, u, method, string(hdrJSON), bodyPart)
}

// GeneratePython generates Python requests code.
func GeneratePython(req *types.RequestDefinition) string {
	method := strings.ToLower(req.Method)
	if method == "" {
		method = "get"
	}
	u := fullURL(req)
	headers := allHeaders(req)

	var headerEntries []string
	for _, h := range headers {
		headerEntries = append(headerEntries, fmt.Sprintf("    %q: %q", h.Key, h.Value))
	}
	headersBlock := "{}"
	if len(headerEntries) > 0 {
		headersBlock = "{\n" + strings.Join(headerEntries, ",\n") + "\n}"
	}

	var bodyBlock string
	switch req.Body.Type {
	case "json", "raw":
		bodyBlock = fmt.Sprintf("\ndata = \"\"\"\n%s\n\"\"\"\n\nresponse = requests.%s(url, headers=headers, data=data)", req.Body.Raw, method)
	case "urlEncoded":
		pairs := make(map[string]string)
		for _, kv := range enabledKVs(req.Body.UrlEncoded) {
			pairs[kv.Key] = kv.Value
		}
		dataJSON, _ := json.MarshalIndent(pairs, "", "    ")
		bodyBlock = fmt.Sprintf("\npayload = %s\n\nresponse = requests.%s(url, headers=headers, data=payload)", string(dataJSON), method)
	default:
		bodyBlock = fmt.Sprintf("\n\nresponse = requests.%s(url, headers=headers)", method)
	}

	return fmt.Sprintf(`import requests

url = %q
headers = %s%s

print(response.status_code)
print(response.text)`, u, headersBlock, bodyBlock)
}

// GenerateCSharp generates C# HttpClient code.
func GenerateCSharp(req *types.RequestDefinition) string {
	method := req.Method
	if method == "" {
		method = "GET"
	}
	pascalMethod := strings.ToUpper(method[:1]) + strings.ToLower(method[1:])
	u := fullURL(req)
	headers := allHeaders(req)

	var headerLines []string
	for _, h := range headers {
		headerLines = append(headerLines, fmt.Sprintf("client.DefaultRequestHeaders.Add(%q, %q);", h.Key, h.Value))
	}

	headersBlock := ""
	if len(headerLines) > 0 {
		headersBlock = "\n" + strings.Join(headerLines, "\n") + "\n"
	}

	bodySetup := ""
	sendCall := fmt.Sprintf("await client.%sAsync(url);", pascalMethod)

	if req.Body.Type == "json" || req.Body.Type == "raw" {
		escapedBody := strings.ReplaceAll(req.Body.Raw, "\"", "\"\"")
		bodySetup = fmt.Sprintf("var content = new StringContent(@\"%s\", System.Text.Encoding.UTF8, \"application/json\");\n", escapedBody)
		sendCall = fmt.Sprintf("await client.%sAsync(url, content);", pascalMethod)
	}

	return fmt.Sprintf(`using System;
using System.Net.Http;
using System.Threading.Tasks;

var client = new HttpClient();
var url = %q;%s
%svar response = %s
var responseBody = await response.Content.ReadAsStringAsync();

Console.WriteLine($"Status: {response.StatusCode}");
Console.WriteLine(responseBody);`, u, headersBlock, bodySetup, sendCall)
}
