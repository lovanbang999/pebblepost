package openapisync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pebblepost/internal/types"
	"pebblepost/internal/workspace"
)

// ApplyDiff applies selected or all non-conflicting changes from a SyncDiffReport to the folder.
func ApplyDiff(wsSvc *workspace.WorkspaceService, folderPath string, report *SyncDiffReport, acceptedIDs []string, force bool) (*ApplyResult, error) {
	if report == nil {
		return nil, fmt.Errorf("diff report is nil")
	}
	if wsSvc == nil {
		wsSvc = workspace.NewWorkspaceService()
	}

	acceptedMap := make(map[string]bool)
	hasAcceptedFilter := len(acceptedIDs) > 0
	for _, id := range acceptedIDs {
		acceptedMap[id] = true
	}

	result := &ApplyResult{
		CreatedFiles: make([]string, 0),
		UpdatedFiles: make([]string, 0),
		DeletedFiles: make([]string, 0),
		NewSpecHash:  report.SpecHash,
		SyncedAt:     time.Now().UTC().Format(time.RFC3339),
	}

	for _, ep := range report.Endpoints {
		// If specific accepted IDs provided, check membership
		if hasAcceptedFilter && !acceptedMap[ep.ID] {
			result.SkippedCount++
			continue
		}

		switch ep.DiffType {
		case DiffAdded:
			if ep.SpecRequest == nil {
				result.SkippedCount++
				continue
			}
			targetPath := ep.ProposedFilePath
			if targetPath == "" {
				targetPath = filepath.Join(folderPath, fmt.Sprintf("%s.pebble.json", strings.ToLower(ep.Method)+"_"+strings.ReplaceAll(ep.Path, "/", "_")))
			}

			// Ensure directory exists
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return nil, fmt.Errorf("failed to create directory for added request: %w", err)
			}

			if err := workspace.WriteFileStable(targetPath, ep.SpecRequest); err != nil {
				return nil, fmt.Errorf("failed to write new request file %s: %w", targetPath, err)
			}
			result.CreatedFiles = append(result.CreatedFiles, targetPath)
			result.AppliedCount++

		case DiffChanged:
			if ep.ExistingRequest == nil || ep.SpecRequest == nil || ep.ExistingFilePath == "" {
				result.SkippedCount++
				continue
			}

			mergedReq := merge3Way(ep.ExistingRequest, ep.SpecRequest)
			if err := workspace.WriteFileStable(ep.ExistingFilePath, mergedReq); err != nil {
				return nil, fmt.Errorf("failed to update request file %s: %w", ep.ExistingFilePath, err)
			}
			result.UpdatedFiles = append(result.UpdatedFiles, ep.ExistingFilePath)
			result.AppliedCount++

		case DiffRemoved:
			// If conflicting (user assets) and force is not set, skip removal
			if ep.Conflict != nil && !force {
				result.SkippedCount++
				continue
			}

			if ep.ExistingFilePath != "" {
				if err := os.Remove(ep.ExistingFilePath); err != nil && !os.IsNotExist(err) {
					return nil, fmt.Errorf("failed to remove deleted request file %s: %w", ep.ExistingFilePath, err)
				}
				result.DeletedFiles = append(result.DeletedFiles, ep.ExistingFilePath)
				result.AppliedCount++
			}

		case DiffUnchanged:
			// No action needed
		}
	}

	// Update _folder.pebble.json metadata
	folderDef, err := wsSvc.ReadFolder(folderPath)
	if err == nil && folderDef != nil {
		if folderDef.OpenAPISync == nil {
			folderDef.OpenAPISync = &types.OpenAPISyncConfig{
				SpecLocation: report.SpecLocation,
			}
		}
		folderDef.OpenAPISync.SpecHash = report.SpecHash
		folderDef.OpenAPISync.LastSyncedAt = result.SyncedAt
		_ = wsSvc.SaveFolder(folderPath, folderDef)
	}

	return result, nil
}

// merge3Way merges spec fields into existing request while strictly preserving user customizations.
func merge3Way(existing, spec *types.RequestDefinition) *types.RequestDefinition {
	merged := *existing

	// Spec owns Method
	merged.Method = spec.Method

	// Spec owns URL Path; preserve user's dynamic base URL / variables if present
	merged.URL = mergeURL(existing.URL, spec.URL)

	// Merge Parameters (spec query params updated, user custom query params preserved)
	merged.Params = mergeKeyValues(existing.Params, spec.Params)

	// Merge Headers: Spec headers merged, user custom headers 100% preserved
	merged.Headers = mergeHeaders(existing.Headers, spec.Headers)

	// Merge Request Body
	merged.Body = mergeBody(existing.Body, spec.Body)

	// Auth: if existing auth was inherit or none, use spec auth; if user configured credentials, keep them
	if (existing.Auth.Type == "inherit" || existing.Auth.Type == "none" || existing.Auth.Type == "") && spec.Auth.Type != "inherit" && spec.Auth.Type != "" {
		merged.Auth.Type = spec.Auth.Type
		if spec.Auth.Key != "" {
			merged.Auth.Key = spec.Auth.Key
		}
		if spec.Auth.AddTo != "" {
			merged.Auth.AddTo = spec.Auth.AddTo
		}
	}

	// 100% PRESERVE USER-OWNED ASSETS:
	merged.Scripts = existing.Scripts
	merged.Examples = existing.Examples
	merged.Extractors = existing.Extractors
	if existing.Description != "" {
		merged.Description = existing.Description
	} else if spec.Description != "" {
		merged.Description = spec.Description
	}

	return &merged
}

// mergeURL preserves existing host variables while updating path and path parameters.
func mergeURL(existingURL, specURL string) string {
	specPath := normalizePath(specURL)

	// If existing URL had a variable prefix like {{host}} or {{baseUrl}}
	if strings.HasPrefix(existingURL, "{{") {
		idx := strings.Index(existingURL, "}}")
		if idx != -1 {
			prefix := existingURL[:idx+2]
			return prefix + specPath
		}
	}

	// If existing URL has a scheme + host
	if strings.Contains(existingURL, "://") {
		parts := strings.SplitN(existingURL, "://", 2)
		scheme := parts[0]
		rest := parts[1]
		slashIdx := strings.Index(rest, "/")
		if slashIdx != -1 {
			host := rest[:slashIdx]
			return scheme + "://" + host + specPath
		}
		return scheme + "://" + rest + specPath
	}

	if specURL != "" {
		return specURL
	}
	return existingURL
}

// mergeKeyValues merges spec parameters with existing parameters, retaining user-added items.
func mergeKeyValues(existing, spec []types.KeyValue) []types.KeyValue {
	merged := make([]types.KeyValue, 0, len(existing)+len(spec))
	specMap := make(map[string]types.KeyValue)
	for _, kv := range spec {
		specMap[strings.ToLower(kv.Key)] = kv
	}

	seen := make(map[string]bool)
	// Process existing items
	for _, kv := range existing {
		lowerKey := strings.ToLower(kv.Key)
		seen[lowerKey] = true
		if specKV, inSpec := specMap[lowerKey]; inSpec {
			// If existing value is empty, take spec example; otherwise preserve user's entered value
			val := kv.Value
			if val == "" {
				val = specKV.Value
			}
			merged = append(merged, types.KeyValue{
				Key:     specKV.Key, // use spec casing
				Value:   val,
				Enabled: kv.Enabled,
			})
		} else {
			// User-added custom parameter - preserve!
			merged = append(merged, kv)
		}
	}

	// Add any new spec parameters not previously in existing
	for _, kv := range spec {
		lowerKey := strings.ToLower(kv.Key)
		if !seen[lowerKey] {
			merged = append(merged, kv)
		}
	}

	return merged
}

// mergeHeaders ensures spec headers are added without deleting user custom headers.
func mergeHeaders(existing, spec []types.KeyValue) []types.KeyValue {
	return mergeKeyValues(existing, spec)
}

// mergeBody merges JSON body schemas by preserving user values for existing fields and adding new fields.
func mergeBody(existing, spec types.BodyDefinition) types.BodyDefinition {
	if spec.Type == "none" || spec.Type == "" {
		return existing
	}
	if spec.Type != "json" {
		return spec
	}

	// If existing body is not json, use spec json body
	if existing.Type != "json" || strings.TrimSpace(existing.Raw) == "" {
		return spec
	}

	var existingJSON, specJSON map[string]any
	if err := json.Unmarshal([]byte(existing.Raw), &existingJSON); err != nil {
		return spec
	}
	if err := json.Unmarshal([]byte(spec.Raw), &specJSON); err != nil {
		return existing
	}

	// Deep merge: keep existing values, add any new keys from spec
	mergedJSON := deepMergeMaps(existingJSON, specJSON)
	mergedRaw, err := json.MarshalIndent(mergedJSON, "", "  ")
	if err != nil {
		return existing
	}

	res := existing
	res.Type = "json"
	res.Raw = string(mergedRaw)
	return res
}

// deepMergeMaps preserves existing values while adding missing keys from spec.
func deepMergeMaps(existing, spec map[string]any) map[string]any {
	result := make(map[string]any)
	for k, v := range existing {
		result[k] = v
	}

	for k, specVal := range spec {
		if existingVal, exists := result[k]; exists {
			// If both are maps, recursively merge
			if existingMap, ok1 := existingVal.(map[string]any); ok1 {
				if specMap, ok2 := specVal.(map[string]any); ok2 {
					result[k] = deepMergeMaps(existingMap, specMap)
				}
			}
		} else {
			// New key from spec
			result[k] = specVal
		}
	}

	return result
}
