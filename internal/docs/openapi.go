package docs

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
	"pebblepost/internal/impexp"
)

// GenerateOpenAPI produces an OpenAPI 3.0.3 specification in JSON or YAML format.
func GenerateOpenAPI(col *DocCollection, format string) ([]byte, error) {
	var items []impexp.ImportedItem
	flattenDocItems(col.Items, "", &items)

	title := col.Title
	if title == "" {
		title = "API Documentation"
	}
	version := col.Version
	if version == "" {
		version = "1.0.0"
	}

	jsonBytes, err := impexp.ExportOpenAPI(title, version, items)
	if err != nil {
		return nil, fmt.Errorf("failed to export openapi: %w", err)
	}

	formatLower := strings.ToLower(strings.TrimSpace(format))
	if formatLower == "json" {
		return jsonBytes, nil
	}

	// Default to YAML format for readability
	var intermediate any
	if err := json.Unmarshal(jsonBytes, &intermediate); err != nil {
		return nil, fmt.Errorf("failed to unmarshal intermediate openapi json: %w", err)
	}

	yamlBytes, err := yaml.Marshal(intermediate)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal openapi yaml: %w", err)
	}

	return yamlBytes, nil
}

func flattenDocItems(items []*DocItem, parentFolder string, acc *[]impexp.ImportedItem) {
	for _, it := range items {
		if it.IsFolder && it.Folder != nil {
			f := it.Folder
			prefix := f.Name
			if parentFolder != "" {
				prefix = parentFolder + "/" + f.Name
			}
			flattenDocItems(f.Items, prefix, acc)
		} else if !it.IsFolder && it.Request != nil {
			r := it.Request
			req := r.RawRequest
			relPath := r.RelPath
			if relPath == "" {
				relPath = r.Name + ".pebble.json"
				if parentFolder != "" {
					relPath = parentFolder + "/" + relPath
				}
			}
			*acc = append(*acc, impexp.ImportedItem{
				RelPath: relPath,
				Request: req,
			})
		}
	}
}
