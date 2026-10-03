package impexp

import (
	"encoding/json"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"pebblepost/internal/types"
)

var reColonParam = regexp.MustCompile(`:([a-zA-Z0-9_]+)`)

// ExportOpenAPI generates an OpenAPI 3.0.3 specification JSON inferred from saved requests.
func ExportOpenAPI(title string, version string, items []ImportedItem) ([]byte, error) {
	if title == "" {
		title = "PebblePost API"
	}
	if version == "" {
		version = "1.0.0"
	}

	doc := map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":   title,
			"version": version,
		},
	}

	// Detect servers and group paths
	serverSet := make(map[string]bool)
	pathsMap := make(map[string]map[string]any)

	for _, it := range items {
		req := it.Request
		if req == nil || strings.TrimSpace(req.URL) == "" {
			continue
		}

		serverURL, pathStr := extractServerAndPath(req.URL)
		if serverURL != "" {
			serverSet[serverURL] = true
		}

		// Convert :id to {id}
		openAPIPath := reColonParam.ReplaceAllString(pathStr, "{$1}")
		if !strings.HasPrefix(openAPIPath, "/") {
			openAPIPath = "/" + openAPIPath
		}

		if pathsMap[openAPIPath] == nil {
			pathsMap[openAPIPath] = make(map[string]any)
		}

		method := strings.ToLower(req.Method)
		if method == "" {
			method = "get"
		}

		operation := buildOpenAPIOperation(req, openAPIPath)
		pathsMap[openAPIPath][method] = operation
	}

	// Servers list
	var servers []map[string]string
	var serverList []string
	for s := range serverSet {
		serverList = append(serverList, s)
	}
	sort.Strings(serverList)
	for _, s := range serverList {
		servers = append(servers, map[string]string{"url": s})
	}
	if len(servers) > 0 {
		doc["servers"] = servers
	} else {
		doc["servers"] = []map[string]string{{"url": "https://api.example.com"}}
	}

	doc["paths"] = pathsMap

	return json.MarshalIndent(doc, "", "  ")
}

func extractServerAndPath(rawURL string) (string, string) {
	rawURL = strings.TrimSpace(rawURL)

	// Handle template variables at start of URL, e.g. {{baseUrl}}/api/v1
	if strings.HasPrefix(rawURL, "{{") {
		idx := strings.Index(rawURL, "}}")
		if idx != -1 {
			server := rawURL[:idx+2]
			path := rawURL[idx+2:]
			// Strip query
			if qIdx := strings.Index(path, "?"); qIdx != -1 {
				path = path[:qIdx]
			}
			return server, path
		}
	}

	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		// Treat as relative path
		path := rawURL
		if qIdx := strings.Index(path, "?"); qIdx != -1 {
			path = path[:qIdx]
		}
		return "", path
	}

	server := u.Scheme + "://" + u.Host
	path := u.Path
	if path == "" {
		path = "/"
	}
	return server, path
}

func buildOpenAPIOperation(req *types.RequestDefinition, path string) map[string]any {
	op := map[string]any{
		"summary": req.Name,
		"responses": map[string]any{
			"200": map[string]any{
				"description": "Successful response",
			},
		},
	}
	if req.Description != "" {
		op["description"] = req.Description
	}

	var params []map[string]any

	// 1. Path parameters from path string
	pathParams := extractPathParams(path)
	for _, p := range pathParams {
		params = append(params, map[string]any{
			"name":     p,
			"in":       "path",
			"required": true,
			"schema": map[string]any{
				"type": "string",
			},
		})
	}

	// 2. Query parameters
	for _, p := range req.Params {
		if !p.Enabled || p.Key == "" {
			continue
		}
		params = append(params, map[string]any{
			"name":     p.Key,
			"in":       "query",
			"required": false,
			"schema": map[string]any{
				"type":    "string",
				"example": p.Value,
			},
		})
	}

	// 3. Header parameters
	for _, h := range req.Headers {
		if !h.Enabled || h.Key == "" {
			continue
		}
		// Skip standard or auto headers
		lower := strings.ToLower(h.Key)
		if lower == "content-type" || lower == "authorization" || lower == "accept" || lower == "user-agent" {
			continue
		}
		params = append(params, map[string]any{
			"name":     h.Key,
			"in":       "header",
			"required": false,
			"schema": map[string]any{
				"type":    "string",
				"example": h.Value,
			},
		})
	}

	if len(params) > 0 {
		op["parameters"] = params
	}

	// 4. Request Body
	reqBody := buildOpenAPIRequestBody(req.Body)
	if reqBody != nil {
		op["requestBody"] = reqBody
	}

	return op
}

func extractPathParams(path string) []string {
	var params []string
	seen := make(map[string]bool)

	start := -1
	for i, c := range path {
		if c == '{' {
			start = i + 1
		} else if c == '}' && start != -1 {
			param := path[start:i]
			if param != "" && !seen[param] {
				seen[param] = true
				params = append(params, param)
			}
			start = -1
		}
	}
	return params
}

func buildOpenAPIRequestBody(body types.BodyDefinition) map[string]any {
	switch body.Type {
	case "json":
		schema := inferSchemaFromJSON(body.Raw)
		return map[string]any{
			"required": true,
			"content": map[string]any{
				"application/json": map[string]any{
					"schema": schema,
				},
			},
		}

	case "urlEncoded":
		props := make(map[string]any)
		for _, kv := range body.UrlEncoded {
			if kv.Key != "" {
				props[kv.Key] = map[string]any{
					"type":    "string",
					"example": kv.Value,
				}
			}
		}
		return map[string]any{
			"content": map[string]any{
				"application/x-www-form-urlencoded": map[string]any{
					"schema": map[string]any{
						"type":       "object",
						"properties": props,
					},
				},
			},
		}

	case "formData":
		props := make(map[string]any)
		for _, kv := range body.FormData {
			if kv.Key != "" {
				if kv.Type == "file" {
					props[kv.Key] = map[string]any{
						"type":   "string",
						"format": "binary",
					}
				} else {
					props[kv.Key] = map[string]any{
						"type":    "string",
						"example": kv.Value,
					}
				}
			}
		}
		return map[string]any{
			"content": map[string]any{
				"multipart/form-data": map[string]any{
					"schema": map[string]any{
						"type":       "object",
						"properties": props,
					},
				},
			},
		}

	case "raw":
		if strings.TrimSpace(body.Raw) != "" {
			return map[string]any{
				"content": map[string]any{
					"text/plain": map[string]any{
						"schema": map[string]any{
							"type":    "string",
							"example": body.Raw,
						},
					},
				},
			}
		}
	}

	return nil
}

func inferSchemaFromJSON(rawJSON string) map[string]any {
	trimmed := strings.TrimSpace(rawJSON)
	if trimmed == "" {
		return map[string]any{"type": "object"}
	}

	var parsed any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return map[string]any{"type": "object"}
	}

	return inferSchemaFromValue(parsed)
}

func inferSchemaFromValue(v any) map[string]any {
	if v == nil {
		return map[string]any{"type": "string", "nullable": true}
	}

	switch val := v.(type) {
	case string:
		return map[string]any{"type": "string", "example": val}
	case float64:
		// Check if it's integer
		if val == float64(int64(val)) {
			return map[string]any{"type": "integer", "example": int64(val)}
		}
		return map[string]any{"type": "number", "example": val}
	case bool:
		return map[string]any{"type": "boolean", "example": val}
	case []any:
		itemSchema := map[string]any{"type": "string"}
		if len(val) > 0 {
			itemSchema = inferSchemaFromValue(val[0])
		}
		return map[string]any{
			"type":  "array",
			"items": itemSchema,
		}
	case map[string]any:
		props := make(map[string]any)
		for k, child := range val {
			props[k] = inferSchemaFromValue(child)
		}
		return map[string]any{
			"type":       "object",
			"properties": props,
		}
	default:
		return map[string]any{"type": "string"}
	}
}
