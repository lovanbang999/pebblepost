package impexp

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
)

// DetectFormat inspects the filename and content bytes to identify the import format.
func DetectFormat(filename string, data []byte) Format {
	ext := strings.ToLower(filepath.Ext(filename))
	base := strings.ToLower(filepath.Base(filename))

	// File extension or filename hints
	if ext == ".bru" || base == "bruno.json" {
		return FormatBruno
	}
	if ext == ".har" {
		return FormatHAR
	}

	trimmed := strings.TrimSpace(string(data))

	// cURL commands
	if strings.HasPrefix(trimmed, "curl") || strings.HasPrefix(trimmed, "curl.exe") {
		return FormatCURL
	}

	// Bruno text file signature
	if strings.HasPrefix(trimmed, "meta {") || strings.Contains(trimmed, "\nmeta {") {
		return FormatBruno
	}

	// Inspect JSON structures
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		var raw map[string]any
		decoder := json.NewDecoder(bytes.NewReader(data))
		if err := decoder.Decode(&raw); err == nil {
			// Postman Collection
			if info, ok := raw["info"].(map[string]any); ok {
				if schema, ok := info["schema"].(string); ok && strings.Contains(schema, "getpostman.com") {
					return FormatPostman
				}
				if _, ok := raw["item"]; ok {
					return FormatPostman
				}
			}

			// Insomnia v4 Export
			if expFormat, ok := raw["__export_format"]; ok {
				if num, ok := expFormat.(float64); ok && num == 4 {
					return FormatInsomnia
				}
			}
			if typ, ok := raw["_type"].(string); ok && typ == "export" {
				return FormatInsomnia
			}

			// HAR 1.2
			if log, ok := raw["log"].(map[string]any); ok {
				if _, ok := log["entries"]; ok {
					return FormatHAR
				}
			}

			// OpenAPI 3.0 / Swagger
			if openapi, ok := raw["openapi"].(string); ok && strings.HasPrefix(openapi, "3.") {
				return FormatOpenAPI
			}
			if _, ok := raw["swagger"]; ok {
				return FormatOpenAPI
			}
		}
	}

	// OpenAPI in YAML format
	if strings.HasPrefix(trimmed, "openapi: 3.") || strings.HasPrefix(trimmed, "openapi: '3.") || strings.HasPrefix(trimmed, "openapi: \"3.") {
		return FormatOpenAPI
	}

	return FormatUnknown
}
