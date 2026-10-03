package workspace

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"pebblepost/internal/types"
)

// ImportResult holds one or more parsed requests from an import source.
type ImportResult struct {
	Requests []*types.RequestDefinition `json:"requests"`
	Source   string                     `json:"source"`
}

// ImportService parses cURL strings, Postman v2.1 JSON, and OpenAPI 3.0 specs.
type ImportService struct{}

// NewImportService creates a new ImportService.
func NewImportService() *ImportService {
	return &ImportService{}
}

// ParseCURL parses a cURL command string into a RequestDefinition.
func (s *ImportService) ParseCURL(curlStr string) (*types.RequestDefinition, error) {
	curlStr = strings.TrimSpace(curlStr)
	if !strings.HasPrefix(curlStr, "curl") {
		return nil, fmt.Errorf("input must start with 'curl'")
	}

	tokens, err := tokenizeCURL(curlStr)
	if err != nil {
		return nil, fmt.Errorf("failed to tokenize cURL: %w", err)
	}

	req := &types.RequestDefinition{
		Method:  "GET",
		Auth:    types.AuthDefinition{Type: "none"},
		Body:    types.BodyDefinition{Type: "none"},
		Scripts: types.ScriptDefinition{},
		Settings: types.SettingDefinition{
			FollowRedirects: true,
			VerifySSL:       true,
			TimeoutMs:       30000,
		},
	}

	i := 1 // skip "curl"
	for i < len(tokens) {
		tok := tokens[i]
		switch tok {
		case "-X", "--request":
			if i+1 < len(tokens) {
				req.Method = strings.ToUpper(tokens[i+1])
				i += 2
			} else {
				i++
			}
		case "-H", "--header":
			if i+1 < len(tokens) {
				raw := tokens[i+1]
				if colonIdx := strings.Index(raw, ":"); colonIdx != -1 {
					key := strings.TrimSpace(raw[:colonIdx])
					val := strings.TrimSpace(raw[colonIdx+1:])
					// Check if Bearer token
					if strings.EqualFold(key, "Authorization") {
						parts := strings.SplitN(val, " ", 2)
						if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
							req.Auth = types.AuthDefinition{Type: "bearer", Token: parts[1]}
						} else if len(parts) == 2 && strings.EqualFold(parts[0], "Basic") {
							req.Auth = types.AuthDefinition{Type: "basic", Token: parts[1]}
						} else {
							req.Headers = append(req.Headers, types.KeyValue{Key: key, Value: val, Enabled: true})
						}
					} else {
						req.Headers = append(req.Headers, types.KeyValue{Key: key, Value: val, Enabled: true})
					}
				}
				i += 2
			} else {
				i++
			}
		case "-d", "--data", "--data-raw", "--data-binary":
			if i+1 < len(tokens) {
				body := tokens[i+1]
				body = strings.TrimPrefix(body, "$'")
				body = strings.TrimSuffix(body, "'")
				// Detect JSON
				trimmed := strings.TrimSpace(body)
				if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
					req.Body = types.BodyDefinition{Type: "json", Raw: body}
				} else {
					req.Body = types.BodyDefinition{Type: "raw", Raw: body}
				}
				if req.Method == "GET" {
					req.Method = "POST"
				}
				i += 2
			} else {
				i++
			}
		case "--form", "-F":
			if i+1 < len(tokens) {
				raw := tokens[i+1]
				if eqIdx := strings.Index(raw, "="); eqIdx != -1 {
					k := raw[:eqIdx]
					v := raw[eqIdx+1:]
					if req.Body.Type != "formData" {
						req.Body = types.BodyDefinition{Type: "formData", FormData: []types.KeyValue{}}
					}
					t := "text"
					if strings.HasPrefix(v, "@") {
						t = "file"
					}
					req.Body.FormData = append(req.Body.FormData, types.KeyValue{Key: k, Value: v, Enabled: true, Type: t})
				}
				i += 2
			} else {
				i++
			}
		case "-u", "--user":
			if i+1 < len(tokens) {
				parts := strings.SplitN(tokens[i+1], ":", 2)
				if len(parts) == 2 {
					req.Auth = types.AuthDefinition{Type: "basic", Username: parts[0], Password: parts[1]}
				}
				i += 2
			} else {
				i++
			}
		case "--compressed", "-L", "--location", "-s", "--silent", "-v", "--verbose", "-i", "--include":
			if tok == "-L" || tok == "--location" {
				req.Settings.FollowRedirects = true
			}
			i++
		case "--insecure", "-k":
			req.Settings.VerifySSL = false
			i++
		case "--max-time":
			if i+1 < len(tokens) {
				var ms int
				fmt.Sscanf(tokens[i+1], "%d", &ms)
				req.Settings.TimeoutMs = ms * 1000
				i += 2
			} else {
				i++
			}
		default:
			// URL (does not start with -)
			if !strings.HasPrefix(tok, "-") && req.URL == "" {
				req.URL = tok
				// Parse query params from URL
				if qIdx := strings.Index(req.URL, "?"); qIdx != -1 {
					query := req.URL[qIdx+1:]
					req.URL = req.URL[:qIdx]
					for _, part := range strings.Split(query, "&") {
						if eqIdx := strings.Index(part, "="); eqIdx != -1 {
							req.Params = append(req.Params, types.KeyValue{
								Key:     part[:eqIdx],
								Value:   part[eqIdx+1:],
								Enabled: true,
							})
						}
					}
				}
			}
			i++
		}
	}

	if req.URL == "" {
		return nil, fmt.Errorf("no URL found in cURL command")
	}

	// Derive name from URL
	urlParts := strings.Split(strings.TrimSuffix(req.URL, "/"), "/")
	req.Name = req.Method + " " + urlParts[len(urlParts)-1]
	if req.Name == req.Method+" " {
		req.Name = req.Method + " Request"
	}

	return req, nil
}

// tokenizeCURL splits a cURL string into tokens respecting quotes.
func tokenizeCURL(s string) ([]string, error) {
	var tokens []string
	var current strings.Builder
	inSingle := false
	inDouble := false

	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		switch {
		case c == '\'' && !inDouble:
			inSingle = !inSingle
		case c == '"' && !inSingle:
			inDouble = !inDouble
		case c == '\\' && (inDouble || inSingle):
			if i+1 < len(runes) {
				i++
				current.WriteRune(runes[i])
			}
		case unicode.IsSpace(c) && !inSingle && !inDouble:
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
			// skip line continuation
			for i+1 < len(runes) && runes[i+1] == '\\' && i+2 < len(runes) && runes[i+2] == '\n' {
				i += 2
				for i+1 < len(runes) && unicode.IsSpace(runes[i+1]) {
					i++
				}
				break
			}
		default:
			current.WriteRune(c)
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	if inSingle || inDouble {
		return nil, fmt.Errorf("unclosed quote in cURL command")
	}
	return tokens, nil
}

// --- Postman v2.1 ---

type postmanCollection struct {
	Info postmanInfo   `json:"info"`
	Item []postmanItem `json:"item"`
}

type postmanInfo struct {
	Name string `json:"name"`
}

type postmanItem struct {
	Name    string        `json:"name"`
	Item    []postmanItem `json:"item"` // folder
	Request *postmanReq   `json:"request"`
}

type postmanReq struct {
	Method string          `json:"method"`
	URL    postmanURL      `json:"url"`
	Header []postmanHeader `json:"header"`
	Body   *postmanBody    `json:"body"`
	Auth   *postmanAuth    `json:"auth"`
}

type postmanURL struct {
	Raw   string `json:"raw"`
	Query []struct {
		Key      string `json:"key"`
		Value    string `json:"value"`
		Disabled bool   `json:"disabled"`
	} `json:"query"`
}

type postmanHeader struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Disabled bool   `json:"disabled"`
}

type postmanBody struct {
	Mode       string `json:"mode"`
	Raw        string `json:"raw"`
	URLEncoded []struct {
		Key      string `json:"key"`
		Value    string `json:"value"`
		Disabled bool   `json:"disabled"`
	} `json:"urlencoded"`
	FormData []struct {
		Key      string `json:"key"`
		Value    string `json:"value"`
		Src      string `json:"src"`
		Type     string `json:"type"`
		Disabled bool   `json:"disabled"`
	} `json:"formdata"`
}

type postmanAuth struct {
	Type   string `json:"type"`
	Bearer []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	} `json:"bearer"`
	Basic []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	} `json:"basic"`
	Apikey []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	} `json:"apikey"`
}

// ParsePostman parses a Postman Collection v2.1 JSON into multiple RequestDefinitions.
func (s *ImportService) ParsePostman(data []byte) ([]*types.RequestDefinition, error) {
	var col postmanCollection
	if err := json.Unmarshal(data, &col); err != nil {
		return nil, fmt.Errorf("invalid Postman JSON: %w", err)
	}
	var reqs []*types.RequestDefinition
	s.flattenPostmanItems(col.Item, &reqs)
	if len(reqs) == 0 {
		return nil, fmt.Errorf("no requests found in Postman collection")
	}
	return reqs, nil
}

func (s *ImportService) flattenPostmanItems(items []postmanItem, out *[]*types.RequestDefinition) {
	for _, item := range items {
		if len(item.Item) > 0 {
			s.flattenPostmanItems(item.Item, out)
			continue
		}
		if item.Request == nil {
			continue
		}
		r := item.Request
		req := &types.RequestDefinition{
			Name:    item.Name,
			Method:  strings.ToUpper(r.Method),
			URL:     r.URL.Raw,
			Auth:    types.AuthDefinition{Type: "none"},
			Body:    types.BodyDefinition{Type: "none"},
			Scripts: types.ScriptDefinition{},
			Settings: types.SettingDefinition{
				FollowRedirects: true,
				VerifySSL:       true,
				TimeoutMs:       30000,
			},
		}

		// Headers
		for _, h := range r.Header {
			req.Headers = append(req.Headers, types.KeyValue{
				Key:     h.Key,
				Value:   h.Value,
				Enabled: !h.Disabled,
			})
		}

		// Params from URL
		for _, q := range r.URL.Query {
			req.Params = append(req.Params, types.KeyValue{
				Key:     q.Key,
				Value:   q.Value,
				Enabled: !q.Disabled,
			})
		}

		// Auth
		if r.Auth != nil {
			switch r.Auth.Type {
			case "bearer":
				token := ""
				for _, kv := range r.Auth.Bearer {
					if kv.Key == "token" {
						token = kv.Value
					}
				}
				req.Auth = types.AuthDefinition{Type: "bearer", Token: token}
			case "basic":
				u, p := "", ""
				for _, kv := range r.Auth.Basic {
					if kv.Key == "username" {
						u = kv.Value
					}
					if kv.Key == "password" {
						p = kv.Value
					}
				}
				req.Auth = types.AuthDefinition{Type: "basic", Username: u, Password: p}
			case "apikey":
				k, v, addTo := "", "", "header"
				for _, kv := range r.Auth.Apikey {
					if kv.Key == "key" {
						k = kv.Value
					}
					if kv.Key == "value" {
						v = kv.Value
					}
					if kv.Key == "in" {
						addTo = kv.Value
					}
				}
				req.Auth = types.AuthDefinition{Type: "apiKey", Key: k, Value: v, AddTo: addTo}
			}
		}

		// Body
		if r.Body != nil {
			switch r.Body.Mode {
			case "raw":
				bodyType := "raw"
				trimmed := strings.TrimSpace(r.Body.Raw)
				if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
					bodyType = "json"
				}
				req.Body = types.BodyDefinition{Type: bodyType, Raw: r.Body.Raw}
			case "urlencoded":
				var kvs []types.KeyValue
				for _, kv := range r.Body.URLEncoded {
					kvs = append(kvs, types.KeyValue{Key: kv.Key, Value: kv.Value, Enabled: !kv.Disabled})
				}
				req.Body = types.BodyDefinition{Type: "urlEncoded", UrlEncoded: kvs}
			case "formdata":
				var kvs []types.KeyValue
				for _, kv := range r.Body.FormData {
					t := "text"
					if kv.Type == "file" {
						t = "file"
					}
					kvs = append(kvs, types.KeyValue{Key: kv.Key, Value: kv.Value, Enabled: !kv.Disabled, Type: t})
				}
				req.Body = types.BodyDefinition{Type: "formData", FormData: kvs}
			}
		}

		*out = append(*out, req)
	}
}

// --- OpenAPI 3.0 ---

type openAPISpec struct {
	OpenAPI string                     `json:"openapi"`
	Info    struct{ Title string }     `json:"info"`
	Servers []struct{ URL string }     `json:"servers"`
	Paths   map[string]openAPIPathItem `json:"paths"`
}

type openAPIPathItem map[string]openAPIOperation // key: get, post, put, patch, delete, head, options

type openAPIOperation struct {
	Summary     string              `json:"summary"`
	OperationID string              `json:"operationId"`
	Parameters  []openAPIParameter  `json:"parameters"`
	RequestBody *openAPIRequestBody `json:"requestBody"`
}

type openAPIParameter struct {
	Name     string `json:"name"`
	In       string `json:"in"` // query, header, path
	Required bool   `json:"required"`
	Example  any    `json:"example"`
}

type openAPIRequestBody struct {
	Content map[string]struct {
		Schema *openAPISchema `json:"schema"`
	} `json:"content"`
}

type openAPISchema struct {
	Type       string                    `json:"type"`
	Properties map[string]*openAPISchema `json:"properties"`
	Example    any                       `json:"example"`
}

// ParseOpenAPI parses an OpenAPI 3.0 JSON or YAML (JSON only for now) spec.
func (s *ImportService) ParseOpenAPI(data []byte) ([]*types.RequestDefinition, error) {
	var spec openAPISpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("invalid OpenAPI JSON: %w", err)
	}
	if !strings.HasPrefix(spec.OpenAPI, "3.") {
		return nil, fmt.Errorf("only OpenAPI 3.x is supported (got %q)", spec.OpenAPI)
	}

	baseURL := ""
	if len(spec.Servers) > 0 {
		baseURL = strings.TrimSuffix(spec.Servers[0].URL, "/")
	}

	var reqs []*types.RequestDefinition
	methods := []string{"get", "post", "put", "patch", "delete", "head", "options"}

	for path, pathItem := range spec.Paths {
		for _, method := range methods {
			opRaw, ok := pathItem[method]
			if !ok {
				continue
			}

			name := opRaw.Summary
			if name == "" {
				name = opRaw.OperationID
			}
			if name == "" {
				name = strings.ToUpper(method) + " " + path
			}

			req := &types.RequestDefinition{
				Name:    name,
				Method:  strings.ToUpper(method),
				URL:     baseURL + path,
				Auth:    types.AuthDefinition{Type: "none"},
				Body:    types.BodyDefinition{Type: "none"},
				Scripts: types.ScriptDefinition{},
				Settings: types.SettingDefinition{
					FollowRedirects: true,
					VerifySSL:       true,
					TimeoutMs:       30000,
				},
			}

			// Parameters
			for _, p := range opRaw.Parameters {
				val := ""
				if p.Example != nil {
					val = fmt.Sprintf("%v", p.Example)
				}
				switch p.In {
				case "query":
					req.Params = append(req.Params, types.KeyValue{
						Key:     p.Name,
						Value:   val,
						Enabled: true,
					})
				case "header":
					req.Headers = append(req.Headers, types.KeyValue{
						Key:     p.Name,
						Value:   val,
						Enabled: true,
					})
				}
			}

			// Request body
			if opRaw.RequestBody != nil {
				if ct, ok := opRaw.RequestBody.Content["application/json"]; ok {
					skeleton := buildJSONSkeleton(ct.Schema)
					raw, _ := json.MarshalIndent(skeleton, "", "  ")
					req.Body = types.BodyDefinition{Type: "json", Raw: string(raw)}
				} else if ct, ok := opRaw.RequestBody.Content["multipart/form-data"]; ok {
					var kvs []types.KeyValue
					if ct.Schema != nil {
						for k := range ct.Schema.Properties {
							kvs = append(kvs, types.KeyValue{Key: k, Value: "", Enabled: true})
						}
					}
					req.Body = types.BodyDefinition{Type: "formData", FormData: kvs}
				}
			}

			reqs = append(reqs, req)
		}
	}

	if len(reqs) == 0 {
		return nil, fmt.Errorf("no operations found in OpenAPI spec")
	}
	return reqs, nil
}

// buildJSONSkeleton generates a placeholder JSON object from an OpenAPI schema.
func buildJSONSkeleton(schema *openAPISchema) any {
	if schema == nil {
		return map[string]any{}
	}
	if schema.Example != nil {
		return schema.Example
	}
	if schema.Type == "object" && len(schema.Properties) > 0 {
		obj := map[string]any{}
		for k, v := range schema.Properties {
			obj[k] = buildJSONSkeleton(v)
		}
		return obj
	}
	switch schema.Type {
	case "string":
		return ""
	case "integer", "number":
		return 0
	case "boolean":
		return false
	case "array":
		return []any{}
	}
	return nil
}
