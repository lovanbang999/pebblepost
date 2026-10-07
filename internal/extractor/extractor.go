package extractor

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"pebblepost/internal/types"
)

// ExtractionResult holds the outcome of evaluating a single extractor against a response body.
type ExtractionResult struct {
	ExtractorID string `json:"extractorId"`
	Name        string `json:"name"`
	Scope       string `json:"scope"`
	Path        string `json:"path"`
	Value       string `json:"value"`
	Success     bool   `json:"success"`
	Warning     string `json:"warning,omitempty"`
}

// Extract evaluates a single extractor definition against a response body string.
func Extract(ext types.ExtractorDefinition, responseBody string) ExtractionResult {
	res := ExtractionResult{
		ExtractorID: ext.ID,
		Name:        ext.Name,
		Scope:       ext.Scope,
		Path:        ext.Path,
		Success:     false,
	}

	if ext.Scope == "" {
		res.Scope = "runtime"
	}

	trimmedBody := strings.TrimSpace(responseBody)
	if trimmedBody == "" {
		res.Warning = fmt.Sprintf("Extractor %q: response body is empty", ext.Name)
		return res
	}

	var root any
	if err := json.Unmarshal([]byte(trimmedBody), &root); err != nil {
		res.Warning = fmt.Sprintf("Extractor %q: response body is not valid JSON: %v", ext.Name, err)
		return res
	}

	val, found, err := EvaluateJSONPath(root, ext.Path)
	if err != nil {
		res.Warning = fmt.Sprintf("Extractor %q: invalid JSONPath %q: %v", ext.Name, ext.Path, err)
		return res
	}
	if !found {
		res.Warning = fmt.Sprintf("Extractor %q: JSONPath %q not found in response", ext.Name, ext.Path)
		return res
	}

	res.Value = FormatExtractedValue(val)
	res.Success = true
	return res
}

// ExtractAll evaluates all enabled extractors and returns the detailed results,
// a map of successfully extracted variables, and a list of warnings for missing paths.
func ExtractAll(extractors []types.ExtractorDefinition, responseBody string) ([]ExtractionResult, map[string]string, []string) {
	results := make([]ExtractionResult, 0, len(extractors))
	extractedVars := make(map[string]string)
	warnings := make([]string, 0)

	for _, ext := range extractors {
		if !ext.Enabled {
			continue
		}
		r := Extract(ext, responseBody)
		results = append(results, r)
		if r.Success {
			extractedVars[r.Name] = r.Value
		}
		if r.Warning != "" {
			warnings = append(warnings, r.Warning)
		}
	}

	return results, extractedVars, warnings
}

// FormatExtractedValue formats extracted values: primitives are kept raw,
// objects/arrays are serialized to compact JSON.
func FormatExtractedValue(val any) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case float64:
		// Format without unnecessary trailing decimal zeroes
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	default:
		// Map, Slice, or other complex types -> compact JSON
		bytes, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(bytes)
	}
}

// PathSegment represents one step in a JSONPath navigation.
type PathSegment struct {
	IsWildcard bool
	Key        string
	IsIndex    bool
	Index      int
}

// ParseJSONPath parses a JSONPath string (e.g. "$.data.users[0].id" or "data['user'][*]") into segments.
func ParseJSONPath(path string) ([]PathSegment, error) {
	clean := strings.TrimSpace(path)
	if clean == "" || clean == "$" {
		return []PathSegment{}, nil
	}

	// Strip leading "$." or "$"
	if strings.HasPrefix(clean, "$.") {
		clean = clean[2:]
	} else if strings.HasPrefix(clean, "$") {
		clean = clean[1:]
	}

	var segments []PathSegment
	var buf strings.Builder
	inBracket := false
	inQuote := false
	var quoteChar rune

	flushBuf := func() {
		if buf.Len() > 0 {
			segments = append(segments, PathSegment{Key: buf.String()})
			buf.Reset()
		}
	}

	chars := []rune(clean)
	for i := 0; i < len(chars); i++ {
		ch := chars[i]

		if inBracket {
			if inQuote {
				if ch == quoteChar {
					inQuote = false
				} else {
					buf.WriteRune(ch)
				}
			} else {
				if ch == '\'' || ch == '"' {
					inQuote = true
					quoteChar = ch
				} else if ch == ']' {
					inBracket = false
					s := strings.TrimSpace(buf.String())
					buf.Reset()
					if s == "*" {
						segments = append(segments, PathSegment{IsWildcard: true})
					} else if idx, err := strconv.Atoi(s); err == nil {
						segments = append(segments, PathSegment{IsIndex: true, Index: idx})
					} else if s != "" {
						segments = append(segments, PathSegment{Key: s})
					}
				} else {
					buf.WriteRune(ch)
				}
			}
			continue
		}

		if ch == '.' {
			flushBuf()
		} else if ch == '[' {
			flushBuf()
			inBracket = true
		} else {
			buf.WriteRune(ch)
		}
	}

	if inBracket {
		return nil, fmt.Errorf("unclosed bracket in JSONPath %q", path)
	}
	flushBuf()

	return segments, nil
}

// EvaluateJSONPath walks root data with parsed path segments and returns the resolved value.
func EvaluateJSONPath(root any, path string) (any, bool, error) {
	segments, err := ParseJSONPath(path)
	if err != nil {
		return nil, false, err
	}

	if len(segments) == 0 {
		return root, true, nil
	}

	currents := []any{root}

	for _, seg := range segments {
		var nextCurrents []any

		for _, cur := range currents {
			if cur == nil {
				continue
			}

			if seg.IsWildcard {
				switch node := cur.(type) {
				case []any:
					nextCurrents = append(nextCurrents, node...)
				case map[string]any:
					for _, v := range node {
						nextCurrents = append(nextCurrents, v)
					}
				}
				continue
			}

			if seg.IsIndex {
				if slice, ok := cur.([]any); ok {
					idx := seg.Index
					if idx < 0 {
						idx = len(slice) + idx
					}
					if idx >= 0 && idx < len(slice) {
						nextCurrents = append(nextCurrents, slice[idx])
					}
				}
				continue
			}

			// Key access
			switch node := cur.(type) {
			case map[string]any:
				if val, exists := node[seg.Key]; exists {
					nextCurrents = append(nextCurrents, val)
				}
			}
		}

		if len(nextCurrents) == 0 {
			return nil, false, nil
		}
		currents = nextCurrents
	}

	if len(currents) == 0 {
		return nil, false, nil
	}
	if len(currents) == 1 {
		return currents[0], true, nil
	}
	return currents, true, nil
}
