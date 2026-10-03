package runner

import (
	"path/filepath"
	"strings"

	"pebblepost/internal/types"
)

// FilterRequest returns true if the request should be executed given the FilterOptions.
// All non-empty filter conditions must match (AND semantics).
func FilterRequest(relPath string, req *types.RequestDefinition, f FilterOptions) bool {
	if f.Folder != "" {
		folderPart := filepath.Dir(relPath)
		if folderPart == "." {
			folderPart = ""
		}
		matched, err := filepath.Match(f.Folder, folderPart)
		if err != nil || !matched {
			// Also try matching against just the last path component
			matched2, _ := filepath.Match(f.Folder, filepath.Base(folderPart))
			if !matched2 {
				return false
			}
		}
	}

	if f.Request != "" {
		nameMatch, _ := filepath.Match(f.Request, req.Name)
		pathMatch, _ := filepath.Match(f.Request, relPath)
		baseMatch, _ := filepath.Match(f.Request, filepath.Base(relPath))
		// Also support simple substring match for convenience
		substrMatch := strings.Contains(strings.ToLower(req.Name), strings.ToLower(f.Request)) ||
			strings.Contains(strings.ToLower(relPath), strings.ToLower(f.Request))
		if !nameMatch && !pathMatch && !baseMatch && !substrMatch {
			return false
		}
	}

	if f.Tag != "" {
		if req == nil || len(req.Tags) == 0 {
			return false
		}
		found := false
		for _, t := range req.Tags {
			if strings.EqualFold(t, f.Tag) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}
