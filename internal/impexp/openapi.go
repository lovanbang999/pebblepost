package impexp

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"pebblepost/internal/types"
)

type openAPISpec struct {
	OpenAPI string                     `json:"openapi"`
	Info    struct{ Title string }     `json:"info"`
	Servers []struct{ URL string }     `json:"servers"`
	Paths   map[string]openAPIPathItem `json:"paths"`
}

type openAPIPathItem map[string]openAPIOperation

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

// ParseOpenAPI parses an OpenAPI 3.x spec JSON into an ImportResult.
func ParseOpenAPI(data []byte) (*ImportResult, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("OpenAPI data is empty")
	}

	var spec openAPISpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAPI JSON: %w", err)
	}

	if !strings.HasPrefix(spec.OpenAPI, "3.") && !strings.HasPrefix(spec.OpenAPI, "3") {
		return nil, fmt.Errorf("only OpenAPI 3.x is supported (got %q)", spec.OpenAPI)
	}

	baseURL := ""
	if len(spec.Servers) > 0 {
		baseURL = strings.TrimSuffix(spec.Servers[0].URL, "/")
	}

	title := spec.Info.Title
	if title == "" {
		title = "OpenAPI Collection"
	}

	result := &ImportResult{
		Format:         FormatOpenAPI,
		CollectionName: title,
	}

	methods := []string{"get", "post", "put", "patch", "delete", "head", "options"}
	seenNames := make(map[string]int)

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

			seenNames[name]++
			finalName := name
			if seenNames[name] > 1 {
				finalName = fmt.Sprintf("%s (%d)", name, seenNames[name])
			}

			req := &types.RequestDefinition{
				Name:   finalName,
				Method: strings.ToUpper(method),
				URL:    baseURL + path,
				Auth:   types.AuthDefinition{Type: "none"},
				Body:   types.BodyDefinition{Type: "none"},
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
					rawBytes, _ := json.MarshalIndent(skeleton, "", "  ")
					req.Body = types.BodyDefinition{Type: "json", Raw: string(rawBytes)}
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

			// Group by path segment if multiple segments exist
			segments := strings.Split(strings.Trim(path, "/"), "/")
			var destRel string
			fileName := sanitizeFilename(finalName) + ".pebble.json"
			if len(segments) > 1 {
				folderRel := sanitizeFilename(segments[0])
				destRel = filepath.Join(folderRel, fileName)
			} else {
				destRel = fileName
			}

			result.Requests = append(result.Requests, ImportedItem{
				Name:    finalName,
				RelPath: destRel,
				Request: req,
			})
		}
	}

	if len(result.Requests) == 0 {
		return nil, fmt.Errorf("no operations found in OpenAPI spec")
	}

	return result, nil
}

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
