package impexp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"pebblepost/internal/types"
)

const (
	defaultCollectionsDir = "collections"
	defaultFolderFile     = "_folder.pebble.json"
)

// Service coordinates importing and exporting across all supported formats.
type Service struct{}

// NewService creates a new impexp Service.
func NewService() *Service {
	return &Service{}
}

// Parse parses raw bytes into an ImportResult. It enforces size limits and handles panics safely.
func (s *Service) Parse(data []byte, filenameHint string, formatOverride Format) (res *ImportResult, err error) {
	if len(data) > MaxImportFileSize {
		return nil, fmt.Errorf("file size (%d bytes) exceeds maximum allowed size (%d bytes)", len(data), MaxImportFileSize)
	}

	// Panic safety gate: malformed inputs must never crash the server or CLI
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic during parsing %q: %v", filenameHint, r)
		}
	}()

	format := formatOverride
	if format == "" || format == FormatUnknown {
		format = DetectFormat(filenameHint, data)
	}

	switch format {
	case FormatBruno:
		base := strings.TrimSuffix(filepath.Base(filenameHint), ".bru")
		req, parseErr := ParseBrunoFile(string(data), base)
		if parseErr != nil {
			return nil, parseErr
		}
		return &ImportResult{
			Format:         FormatBruno,
			CollectionName: base,
			Requests: []ImportedItem{
				{Name: req.Name, RelPath: sanitizeFilename(req.Name) + ".pebble.json", Request: req},
			},
		}, nil

	case FormatInsomnia:
		return ParseInsomnia(data)

	case FormatHAR:
		return ParseHAR(data)

	case FormatPostman:
		return ParsePostman(data)

	case FormatOpenAPI:
		return ParseOpenAPI(data)

	case FormatCURL:
		return ParseCURL(string(data))

	default:
		// Attempt fallback parsing in order of probability
		if r, err := ParsePostman(data); err == nil {
			return r, nil
		}
		if r, err := ParseInsomnia(data); err == nil {
			return r, nil
		}
		if r, err := ParseOpenAPI(data); err == nil {
			return r, nil
		}
		if r, err := ParseHAR(data); err == nil {
			return r, nil
		}
		if r, err := ParseCURL(string(data)); err == nil {
			return r, nil
		}

		return nil, fmt.Errorf("unrecognized or unsupported import format for %q", filenameHint)
	}
}

// ParseFile reads a file from disk and parses it.
func (s *Service) ParseFile(filePath string, formatOverride Format) (*ImportResult, error) {
	fi, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}

	if fi.IsDir() {
		return s.ParseDir(filePath)
	}

	if fi.Size() > MaxImportFileSize {
		return nil, fmt.Errorf("file size (%d bytes) exceeds maximum allowed size (%d bytes)", fi.Size(), MaxImportFileSize)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %q: %w", filePath, err)
	}

	return s.Parse(data, fi.Name(), formatOverride)
}

// ParseDir parses a directory (currently Bruno collection folders).
func (s *Service) ParseDir(dirPath string) (res *ImportResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic during directory parsing %q: %v", dirPath, r)
		}
	}()

	return ParseBrunoCollection(dirPath)
}

// SaveToWorkspace writes the parsed ImportResult into the workspace filesystem.
func (s *Service) SaveToWorkspace(workspaceRoot string, outDir string, result *ImportResult) (*ImportReport, error) {
	if workspaceRoot == "" {
		workspaceRoot = "."
	}

	colFolder := outDir
	if colFolder == "" {
		colName := sanitizeFilename(result.CollectionName)
		if colName == "" {
			colName = "imported-collection"
		}
		colFolder = filepath.Join(defaultCollectionsDir, colName)
	}

	targetBase := filepath.Join(workspaceRoot, colFolder)
	if err := os.MkdirAll(targetBase, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory %q: %w", targetBase, err)
	}

	report := result.ToReport()
	var successfulReqs []ImportedItem

	// 1. Write folder definitions (_folder.pebble.json)
	for _, folder := range result.Folders {
		folderPath := targetBase
		if folder.RelPath != "." && folder.RelPath != "" {
			folderPath = filepath.Join(targetBase, folder.RelPath)
		}
		if err := os.MkdirAll(folderPath, 0755); err != nil {
			continue
		}

		folderDef := folder.Definition
		if folderDef.SchemaVersion == 0 {
			folderDef.SchemaVersion = 1
		}
		if folderDef.Name == "" {
			folderDef.Name = folder.Name
		}

		data, err := json.MarshalIndent(folderDef, "", "  ")
		if err == nil {
			_ = os.WriteFile(filepath.Join(folderPath, defaultFolderFile), data, 0644)
		}
	}

	// 2. Write request definitions (*.pebble.json)
	for _, item := range result.Requests {
		if item.Request == nil {
			continue
		}

		destPath := filepath.Join(targetBase, item.RelPath)
		parentDir := filepath.Dir(destPath)
		if err := os.MkdirAll(parentDir, 0755); err != nil {
			report.Skipped = append(report.Skipped, SkippedItem{
				Name:   item.Name,
				Path:   item.RelPath,
				Reason: fmt.Sprintf("failed to create folder: %v", err),
			})
			continue
		}

		reqDef := item.Request
		if reqDef.SchemaVersion == 0 {
			reqDef.SchemaVersion = 1
		}
		if reqDef.Name == "" {
			reqDef.Name = item.Name
		}

		data, err := json.MarshalIndent(reqDef, "", "  ")
		if err != nil {
			report.Skipped = append(report.Skipped, SkippedItem{
				Name:   item.Name,
				Path:   item.RelPath,
				Reason: fmt.Sprintf("failed to encode request JSON: %v", err),
			})
			continue
		}

		if err := os.WriteFile(destPath, data, 0644); err != nil {
			report.Skipped = append(report.Skipped, SkippedItem{
				Name:   item.Name,
				Path:   item.RelPath,
				Reason: fmt.Sprintf("failed to write file: %v", err),
			})
			continue
		}

		successfulReqs = append(successfulReqs, item)
	}

	report.Requests = successfulReqs
	report.TotalRequests = len(successfulReqs)
	report.TotalFolders = len(result.Folders)
	report.TotalVariables = len(result.Variables)

	return &report, nil
}

// Export produces an exported Postman or OpenAPI file in bytes.
func (s *Service) Export(collectionName string, format Format, items []ImportedItem, folders []ImportedFolder, variables []types.KeyValue) ([]byte, error) {
	switch format {
	case FormatPostman:
		return ExportPostman(collectionName, items, folders, variables)
	case FormatOpenAPI:
		return ExportOpenAPI(collectionName, "1.0.0", items)
	default:
		return nil, fmt.Errorf("unsupported export format %q (supported: postman, openapi)", format)
	}
}
