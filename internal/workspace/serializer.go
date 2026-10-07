package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// CurrentSchemaVersion is the latest schema version for workspace, environment, and folder files.
const CurrentSchemaVersion = 1

// CurrentRequestSchemaVersion is the latest schema version for request files (*.pebble.json).
const CurrentRequestSchemaVersion = 2

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
	return WriteFileStableWithMode(filePath, v, 0644)
}

// WriteFileStableWithMode serializes v using MarshalStable and writes it to filePath
// atomically with the specified file permissions. For 0600 secret files, parent directories are created with 0700.
func WriteFileStableWithMode(filePath string, v any, mode os.FileMode) error {
	if filePath == "" {
		return fmt.Errorf("file path cannot be empty")
	}

	parentDir := filepath.Dir(filePath)
	dirPerm := os.FileMode(0755)
	if mode == 0600 {
		dirPerm = 0700
	}
	if err := os.MkdirAll(parentDir, dirPerm); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", parentDir, err)
	}
	if mode == 0600 {
		_ = os.Chmod(parentDir, 0700)
	}

	data, err := MarshalStable(v)
	if err != nil {
		return err
	}

	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, mode); err != nil {
		return fmt.Errorf("failed to write temporary file: %w", err)
	}
	_ = os.Chmod(tmpPath, mode)

	if err := os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to save file %s: %w", filePath, err)
	}
	_ = os.Chmod(filePath, mode)

	return nil
}
