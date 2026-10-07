package openapisync

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// RawSpec represents the parsed OpenAPI specification data structure.
type RawSpec struct {
	OpenAPI    string                           `yaml:"openapi" json:"openapi"`
	Info       SpecInfo                         `yaml:"info" json:"info"`
	Servers    []SpecServer                     `yaml:"servers" json:"servers"`
	Paths      map[string]map[string]*Operation `yaml:"paths" json:"paths"`
	Components *Components                      `yaml:"components" json:"components"`
}

type SpecInfo struct {
	Title       string `yaml:"title" json:"title"`
	Version     string `yaml:"version" json:"version"`
	Description string `yaml:"description" json:"description"`
}

type SpecServer struct {
	URL         string `yaml:"url" json:"url"`
	Description string `yaml:"description" json:"description"`
}

type Operation struct {
	Summary     string                `yaml:"summary" json:"summary"`
	Description string                `yaml:"description" json:"description"`
	OperationID string                `yaml:"operationId" json:"operationId"`
	Parameters  []*Parameter          `yaml:"parameters" json:"parameters"`
	RequestBody *RequestBody          `yaml:"requestBody" json:"requestBody"`
	Security    []map[string][]string `yaml:"security" json:"security"`
}

type Parameter struct {
	Name        string  `yaml:"name" json:"name"`
	In          string  `yaml:"in" json:"in"` // query, header, path
	Required    bool    `yaml:"required" json:"required"`
	Description string  `yaml:"description" json:"description"`
	Schema      *Schema `yaml:"schema" json:"schema"`
	Example     any     `yaml:"example" json:"example"`
}

type RequestBody struct {
	Description string                   `yaml:"description" json:"description"`
	Required    bool                     `yaml:"required" json:"required"`
	Content     map[string]*MediaTypeObj `yaml:"content" json:"content"`
}

type MediaTypeObj struct {
	Schema  *Schema `yaml:"schema" json:"schema"`
	Example any     `yaml:"example" json:"example"`
}

type Schema struct {
	Type        string             `yaml:"type" json:"type"`
	Format      string             `yaml:"format" json:"format"`
	Properties  map[string]*Schema `yaml:"properties" json:"properties"`
	Items       *Schema            `yaml:"items" json:"items"`
	Example     any                `yaml:"example" json:"example"`
	Default     any                `yaml:"default" json:"default"`
	Description string             `yaml:"description" json:"description"`
}

type Components struct {
	SecuritySchemes map[string]*SecurityScheme `yaml:"securitySchemes" json:"securitySchemes"`
}

type SecurityScheme struct {
	Type         string `yaml:"type" json:"type"`     // http, apiKey, oauth2, openIdConnect
	Scheme       string `yaml:"scheme" json:"scheme"` // bearer, basic
	BearerFormat string `yaml:"bearerFormat" json:"bearerFormat"`
	In           string `yaml:"in" json:"in"` // header, query
	Name         string `yaml:"name" json:"name"`
}

// ComputeSpecHash generates a SHA-256 hash string for specification bytes.
func ComputeSpecHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// isBlockedMetadataHost guards against SSRF to cloud instance metadata services.
func isBlockedMetadataHost(host string) bool {
	h := strings.ToLower(host)
	if strings.Contains(h, ":") {
		if sh, _, err := net.SplitHostPort(h); err == nil {
			h = sh
		}
	}
	return h == "169.254.169.254" || h == "instance-data" || h == "metadata.google.internal" || h == "100.100.100.200"
}

// LoadSpec reads OpenAPI specification bytes from a local file or remote URL.
// Returns the raw specification bytes and its SHA-256 hash.
func LoadSpec(location string, baseDir string) ([]byte, string, error) {
	location = strings.TrimSpace(location)
	if location == "" {
		return nil, "", fmt.Errorf("spec location cannot be empty")
	}

	if strings.HasPrefix(location, "http://") || strings.HasPrefix(location, "https://") {
		u, err := url.Parse(location)
		if err != nil {
			return nil, "", fmt.Errorf("invalid spec URL: %w", err)
		}
		if isBlockedMetadataHost(u.Host) {
			return nil, "", fmt.Errorf("security: requests to cloud metadata service %q are blocked", u.Host)
		}

		client := &http.Client{
			Timeout: 15 * time.Second,
		}
		req, err := http.NewRequest(http.MethodGet, location, nil)
		if err != nil {
			return nil, "", fmt.Errorf("failed to create HTTP request for spec: %w", err)
		}
		req.Header.Set("User-Agent", "PebblePost-OpenAPISync/1.0")
		req.Header.Set("Accept", "application/json, application/yaml, text/yaml, */*")

		resp, err := client.Do(req)
		if err != nil {
			return nil, "", fmt.Errorf("failed to fetch remote spec from %s: %w", location, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, "", fmt.Errorf("remote spec returned status code %d %s", resp.StatusCode, resp.Status)
		}

		// Limit to 10MB to prevent memory exhaustion
		data, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
		if err != nil {
			return nil, "", fmt.Errorf("failed to read remote spec body: %w", err)
		}

		return data, ComputeSpecHash(data), nil
	}

	// Local file path
	filePath := location
	if !filepath.IsAbs(filePath) && baseDir != "" {
		filePath = filepath.Join(baseDir, location)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read spec file %s: %w", filePath, err)
	}

	return data, ComputeSpecHash(data), nil
}

// ParseSpec parses OpenAPI specification bytes (JSON or YAML) into RawSpec.
func ParseSpec(data []byte) (*RawSpec, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("OpenAPI spec data is empty")
	}

	var spec RawSpec
	// yaml.v3.Unmarshal seamlessly parses both JSON and YAML.
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAPI spec (JSON/YAML): %w", err)
	}

	// Validate OpenAPI version (3.0.x or 3.1.x)
	if !strings.HasPrefix(spec.OpenAPI, "3.") && !strings.HasPrefix(spec.OpenAPI, "3") {
		return nil, fmt.Errorf("only OpenAPI 3.x is supported (got %q)", spec.OpenAPI)
	}

	return &spec, nil
}

// BuildJSONSkeleton constructs a representative JSON skeleton from an OpenAPI schema.
func BuildJSONSkeleton(schema *Schema) any {
	if schema == nil {
		return map[string]any{}
	}
	if schema.Example != nil {
		return schema.Example
	}
	if schema.Default != nil {
		return schema.Default
	}

	switch schema.Type {
	case "object", "":
		if len(schema.Properties) > 0 {
			obj := make(map[string]any)
			for k, v := range schema.Properties {
				obj[k] = BuildJSONSkeleton(v)
			}
			return obj
		}
		return map[string]any{}
	case "array":
		if schema.Items != nil {
			return []any{BuildJSONSkeleton(schema.Items)}
		}
		return []any{}
	case "string":
		if schema.Format == "date-time" {
			return "2026-01-01T00:00:00Z"
		}
		if schema.Format == "email" {
			return "user@example.com"
		}
		return ""
	case "integer":
		return 0
	case "number":
		return 0.0
	case "boolean":
		return false
	default:
		return nil
	}
}

// FormatSkeletonJSON serializes a skeleton structure to 2-space indented JSON string.
func FormatSkeletonJSON(skeleton any) string {
	b, err := json.MarshalIndent(skeleton, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(b)
}
