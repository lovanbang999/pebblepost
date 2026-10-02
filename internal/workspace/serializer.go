package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// CurrentSchemaVersion is the latest schema version across all PebblePost files.
const CurrentSchemaVersion = 1

// MarshalStable serializes any data structure to JSON using 2-space indentation
// and always appends a single trailing newline. Key order is deterministic and
// dictated by the struct definitions.
func MarshalStable(v any) ([]byte, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// Always ensure trailing newline
	data = append(data, '\n')
	return data, nil
}

// WriteFileStable serializes v using MarshalStable and writes it to filePath
// atomically via a temporary file, ensuring no half-written files exist on disk.
func WriteFileStable(filePath string, v any) error {
	if filePath == "" {
		return fmt.Errorf("file path cannot be empty")
	}

	parentDir := filepath.Dir(filePath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", parentDir, err)
	}

	data, err := MarshalStable(v)
	if err != nil {
		return err
	}

	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temporary file: %w", err)
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to save file %s: %w", filePath, err)
	}

	return nil
}
