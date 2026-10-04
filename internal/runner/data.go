package runner

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ParseDataFile reads a CSV or JSON file from disk and parses it into a slice of row maps.
func ParseDataFile(filePath string) ([]map[string]any, error) {
	if strings.TrimSpace(filePath) == "" {
		return nil, errors.New("data file path is empty")
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading data file: %w", err)
	}

	return ParseDataBytes(content, filepath.Base(filePath))
}

// ParseDataBytes parses raw file bytes based on format detected from filename or content.
func ParseDataBytes(content []byte, filename string) ([]map[string]any, error) {
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 {
		return nil, errors.New("data file is empty")
	}

	ext := strings.ToLower(filepath.Ext(filename))
	if ext == ".json" || (ext == "" && (trimmed[0] == '[' || trimmed[0] == '{')) {
		return parseJSONData(trimmed)
	}

	// Default to CSV parser
	return parseCSVData(trimmed)
}

func parseJSONData(content []byte) ([]map[string]any, error) {
	// Try parsing as array of objects
	var rows []map[string]any
	if err := json.Unmarshal(content, &rows); err == nil {
		return rows, nil
	}

	// Try parsing as a single object, e.g. {"items": [...]} or a single row
	var singleObj map[string]any
	if err := json.Unmarshal(content, &singleObj); err == nil {
		// If it has an array field named "data" or "items" or "rows", extract that
		for _, key := range []string{"data", "items", "rows", "iterations"} {
			if val, ok := singleObj[key]; ok {
				if arr, ok := val.([]any); ok {
					var extracted []map[string]any
					for _, item := range arr {
						if obj, ok := item.(map[string]any); ok {
							extracted = append(extracted, obj)
						}
					}
					if len(extracted) > 0 {
						return extracted, nil
					}
				}
			}
		}
		// Treat single object as a single-row dataset
		return []map[string]any{singleObj}, nil
	}

	return nil, errors.New("invalid JSON data: expected an array of objects or an object containing an array of records")
}

func parseCSVData(content []byte) ([]map[string]any, error) {
	// Auto-detect comma vs tab delimiter
	reader := csv.NewReader(bytes.NewReader(content))
	reader.TrimLeadingSpace = true
	reader.LazyQuotes = true

	// Detect tab delimiter if tabs appear in the first line and commas do not
	firstLineEnd := bytes.IndexByte(content, '\n')
	firstLine := content
	if firstLineEnd != -1 {
		firstLine = content[:firstLineEnd]
	}
	if bytes.Contains(firstLine, []byte{'\t'}) && !bytes.Contains(firstLine, []byte{','}) {
		reader.Comma = '\t'
	} else if bytes.Contains(firstLine, []byte{';'}) && !bytes.Contains(firstLine, []byte{','}) {
		reader.Comma = ';'
	}

	headers, err := reader.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("CSV data file is empty")
		}
		return nil, fmt.Errorf("parsing CSV header: %w", err)
	}

	for i := range headers {
		headers[i] = strings.TrimSpace(headers[i])
		// Strip UTF-8 BOM if present on first header
		if i == 0 {
			headers[i] = strings.TrimPrefix(headers[i], "\ufeff")
		}
	}

	if len(headers) == 0 {
		return nil, errors.New("CSV header row contains no column names")
	}

	var rows []map[string]any
	for lineNum := 2; ; lineNum++ {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parsing CSV row %d: %w", lineNum, err)
		}

		row := make(map[string]any)
		for i, header := range headers {
			if header == "" {
				continue
			}
			if i < len(record) {
				row[header] = strings.TrimSpace(record[i])
			} else {
				row[header] = ""
			}
		}
		rows = append(rows, row)
	}

	if len(rows) == 0 {
		return nil, errors.New("CSV data file contains headers but no data rows")
	}

	return rows, nil
}

// ComputeIterations resolves the final iteration count.
//   - If data rows exist: defaults to len(dataRows); if requestedIterations > 0 and < len(dataRows), caps at requestedIterations.
//     If requestedIterations >= len(dataRows), caps at len(dataRows) per project requirements.
//   - If no data rows exist: defaults to max(1, requestedIterations).
func ComputeIterations(requestedIterations int, dataRowsCount int) int {
	if dataRowsCount > 0 {
		if requestedIterations <= 0 || requestedIterations >= dataRowsCount {
			return dataRowsCount
		}
		return requestedIterations
	}

	if requestedIterations <= 0 {
		return 1
	}
	return requestedIterations
}
