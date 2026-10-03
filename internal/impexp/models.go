package impexp

import (
	"pebblepost/internal/types"
)

// MaxImportFileSize defines the maximum allowed file size for imports (50 MB).
const MaxImportFileSize = 50 * 1024 * 1024

// Format identifies the format of an import or export.
type Format string

const (
	FormatUnknown  Format = "unknown"
	FormatBruno    Format = "bruno"
	FormatInsomnia Format = "insomnia"
	FormatHAR      Format = "har"
	FormatPostman  Format = "postman"
	FormatOpenAPI  Format = "openapi"
	FormatCURL     Format = "curl"
)

// SkippedItem tracks requests or resources that could not be converted, with the reason.
type SkippedItem struct {
	Name   string `json:"name"`
	Path   string `json:"path,omitempty"`
	Reason string `json:"reason"`
}

// ImportedItem represents an imported request with its relative destination path.
type ImportedItem struct {
	Name    string                   `json:"name"`
	RelPath string                   `json:"relPath"`
	Request *types.RequestDefinition `json:"request"`
}

// ImportedFolder represents an imported folder with optional folder settings.
type ImportedFolder struct {
	Name       string                 `json:"name"`
	RelPath    string                 `json:"relPath"`
	Definition types.FolderDefinition `json:"definition"`
}

// ImportReport summarizes the outcome of an import operation.
type ImportReport struct {
	SourceFormat   Format           `json:"sourceFormat"`
	CollectionName string           `json:"collectionName"`
	TotalRequests  int              `json:"totalRequests"`
	TotalFolders   int              `json:"totalFolders"`
	TotalVariables int              `json:"totalVariables"`
	Requests       []ImportedItem   `json:"requests"`
	Folders        []ImportedFolder `json:"folders,omitempty"`
	Variables      []types.KeyValue `json:"variables,omitempty"`
	Warnings       []string         `json:"warnings,omitempty"`
	Skipped        []SkippedItem    `json:"skipped,omitempty"`
}

// ImportResult contains all entities parsed from a collection file or folder.
type ImportResult struct {
	Format         Format
	CollectionName string
	Requests       []ImportedItem
	Folders        []ImportedFolder
	Variables      []types.KeyValue // Collection-level variables
	Warnings       []string
	Skipped        []SkippedItem
}

// ToReport converts an ImportResult into a serializable ImportReport.
func (r *ImportResult) ToReport() ImportReport {
	return ImportReport{
		SourceFormat:   r.Format,
		CollectionName: r.CollectionName,
		TotalRequests:  len(r.Requests),
		TotalFolders:   len(r.Folders),
		TotalVariables: len(r.Variables),
		Requests:       r.Requests,
		Folders:        r.Folders,
		Variables:      r.Variables,
		Warnings:       r.Warnings,
		Skipped:        r.Skipped,
	}
}
