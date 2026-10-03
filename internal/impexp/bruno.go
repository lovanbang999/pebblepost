package impexp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pebblepost/internal/types"
)

// ParseBrunoFile parses a single .bru file content into a RequestDefinition.
func ParseBrunoFile(content string, defaultName string) (*types.RequestDefinition, error) {
	req := &types.RequestDefinition{
		Name:    defaultName,
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

	blocks := extractBrunoBlocks(content)

	// 1. Meta block
	if metaLines, ok := blocks["meta"]; ok {
		for _, line := range metaLines {
			k, v := splitKeyValue(line, ":")
			if k == "name" && v != "" {
				req.Name = v
			}
			if k == "seq" {
				var seq int
				_, _ = fmt.Sscanf(v, "%d", &seq)
				req.Order = seq
			}
		}
	}

	// 2. HTTP method block (get, post, put, delete, patch, options, head)
	for _, m := range []string{"get", "post", "put", "patch", "delete", "options", "head"} {
		if lines, ok := blocks[m]; ok {
			req.Method = strings.ToUpper(m)
			for _, line := range lines {
				k, v := splitKeyValue(line, ":")
				if k == "url" {
					req.URL = v
				} else if k == "body" && v != "" && v != "none" {
					req.Body.Type = v
				} else if k == "auth" && v != "" && v != "none" {
					req.Auth.Type = v
				}
			}
			break
		}
	}

	// 3. Headers
	if lines, ok := blocks["headers"]; ok {
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			// Bruno uses ~ for disabled headers: e.g. ~Authorization: Bearer xyz
			enabled := true
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "~") {
				enabled = false
				trimmed = strings.TrimPrefix(trimmed, "~")
			}
			k, v := splitKeyValue(trimmed, ":")
			if k != "" {
				req.Headers = append(req.Headers, types.KeyValue{
					Key:     k,
					Value:   v,
					Enabled: enabled,
				})
			}
		}
	}

	// 4. Query Params
	if lines, ok := blocks["params:query"]; ok {
		for _, line := range lines {
			enabled := true
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "~") {
				enabled = false
				trimmed = strings.TrimPrefix(trimmed, "~")
			}
			k, v := splitKeyValue(trimmed, ":")
			if k != "" {
				req.Params = append(req.Params, types.KeyValue{
					Key:     k,
					Value:   v,
					Enabled: enabled,
				})
			}
		}
	}

	// 5. Auth block (auth:bearer, auth:basic, etc.)
	if lines, ok := blocks["auth:bearer"]; ok {
		req.Auth.Type = "bearer"
		for _, line := range lines {
			k, v := splitKeyValue(line, ":")
			if k == "token" {
				req.Auth.Token = v
			}
		}
	} else if lines, ok := blocks["auth:basic"]; ok {
		req.Auth.Type = "basic"
		for _, line := range lines {
			k, v := splitKeyValue(line, ":")
			switch k {
			case "username":
				req.Auth.Username = v
			case "password":
				req.Auth.Password = v
			}
		}
	}

	// 6. Body blocks
	if lines, ok := blocks["body:json"]; ok {
		req.Body.Type = "json"
		req.Body.Raw = strings.Join(lines, "\n")
	} else if lines, ok := blocks["body:text"]; ok {
		req.Body.Type = "raw"
		req.Body.Raw = strings.Join(lines, "\n")
	} else if lines, ok := blocks["body:xml"]; ok {
		req.Body.Type = "xml"
		req.Body.Raw = strings.Join(lines, "\n")
	} else if lines, ok := blocks["body:graphql"]; ok {
		req.Body.Type = "graphql"
		req.Body.Raw = strings.Join(lines, "\n")
	} else if lines, ok := blocks["body:form-urlencoded"]; ok {
		req.Body.Type = "urlencoded"
		for _, line := range lines {
			enabled := true
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "~") {
				enabled = false
				trimmed = strings.TrimPrefix(trimmed, "~")
			}
			k, v := splitKeyValue(trimmed, ":")
			if k != "" {
				req.Body.FormData = append(req.Body.FormData, types.KeyValue{
					Key:     k,
					Value:   v,
					Enabled: enabled,
				})
			}
		}
	}

	// 7. Scripts and Tests
	if lines, ok := blocks["script:pre-request"]; ok {
		req.Scripts.PreRequest = strings.Join(lines, "\n")
	}
	if lines, ok := blocks["tests"]; ok {
		req.Scripts.PostResponse = strings.Join(lines, "\n")
	}

	return req, nil
}

// ParseBrunoCollection parses an entire directory containing Bruno files (bruno.json and .bru files).
func ParseBrunoCollection(dirPath string) (*ImportResult, error) {
	info, err := os.Stat(dirPath)
	if err != nil {
		return nil, fmt.Errorf("bruno collection directory not found: %w", err)
	}
	if !info.IsDir() {
		// Single .bru file
		data, err := os.ReadFile(dirPath)
		if err != nil {
			return nil, err
		}
		base := strings.TrimSuffix(filepath.Base(dirPath), ".bru")
		req, err := ParseBrunoFile(string(data), base)
		if err != nil {
			return nil, err
		}
		return &ImportResult{
			Format:         FormatBruno,
			CollectionName: base,
			Requests: []ImportedItem{
				{Name: req.Name, RelPath: base + ".pebble.json", Request: req},
			},
		}, nil
	}

	collectionName := filepath.Base(dirPath)
	brunoJsonPath := filepath.Join(dirPath, "bruno.json")
	if data, err := os.ReadFile(brunoJsonPath); err == nil {
		var cfg struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(data, &cfg); err == nil && cfg.Name != "" {
			collectionName = cfg.Name
		}
	}

	result := &ImportResult{
		Format:         FormatBruno,
		CollectionName: collectionName,
	}

	err = filepath.Walk(dirPath, func(path string, f os.FileInfo, err error) error {
		if err != nil || f == nil {
			return nil
		}
		if f.IsDir() {
			if strings.HasPrefix(f.Name(), ".") || f.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		if strings.HasSuffix(f.Name(), ".bru") {
			rel, _ := filepath.Rel(dirPath, path)
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				result.Skipped = append(result.Skipped, SkippedItem{
					Name:   f.Name(),
					Path:   rel,
					Reason: readErr.Error(),
				})
				return nil
			}

			reqName := strings.TrimSuffix(f.Name(), ".bru")
			req, parseErr := ParseBrunoFile(string(data), reqName)
			if parseErr != nil {
				result.Skipped = append(result.Skipped, SkippedItem{
					Name:   reqName,
					Path:   rel,
					Reason: parseErr.Error(),
				})
				return nil
			}

			// Destination relative path
			relDir := filepath.Dir(rel)
			var destRel string
			if relDir == "." {
				destRel = sanitizeFilename(req.Name) + ".pebble.json"
			} else {
				destRel = filepath.Join(relDir, sanitizeFilename(req.Name)+".pebble.json")
			}

			result.Requests = append(result.Requests, ImportedItem{
				Name:    req.Name,
				RelPath: destRel,
				Request: req,
			})
		}
		return nil
	})

	return result, err
}

// extractBrunoBlocks splits a .bru file into named blocks (e.g. meta, get, headers, etc.).
func extractBrunoBlocks(content string) map[string][]string {
	blocks := make(map[string][]string)
	scanner := bufio.NewScanner(strings.NewReader(content))

	var currentBlock string
	var currentLines []string
	inBlock := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if !inBlock {
			// Look for block start: e.g. "meta {" or "headers {"
			if strings.HasSuffix(trimmed, "{") {
				currentBlock = strings.TrimSpace(strings.TrimSuffix(trimmed, "{"))
				currentLines = nil
				inBlock = true
			}
		} else {
			if trimmed == "}" {
				// Block end
				blocks[currentBlock] = currentLines
				inBlock = false
				currentBlock = ""
			} else {
				currentLines = append(currentLines, line)
			}
		}
	}

	return blocks
}

func splitKeyValue(line string, delimiter string) (string, string) {
	parts := strings.SplitN(line, delimiter, 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return strings.TrimSpace(line), ""
}

// SanitizeFilename cleans a string so it can be safely used as a filename.
func SanitizeFilename(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var sb strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			sb.WriteRune(r)
		} else if r == ' ' || r == '.' || r == '/' || r == '\\' {
			sb.WriteRune('-')
		}
	}
	res := strings.Trim(sb.String(), "-")
	if res == "" {
		return "request"
	}
	return res
}

func sanitizeFilename(s string) string {
	return SanitizeFilename(s)
}
