package openapisync

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"pebblepost/internal/impexp"
	"pebblepost/internal/types"
	"pebblepost/internal/workspace"
)

// CompareSpecAndFolder compares a parsed OpenAPI spec against requests in folderPath.
func CompareSpecAndFolder(folderPath string, spec *RawSpec, specHash string, specLocation string) (*SyncDiffReport, error) {
	if spec == nil {
		return nil, fmt.Errorf("OpenAPI spec is nil")
	}

	report := &SyncDiffReport{
		FolderPath:   folderPath,
		SpecLocation: specLocation,
		SpecHash:     specHash,
		Endpoints:    make([]EndpointDiff, 0),
	}

	// Determine base server URL and server base path prefix
	baseURL := ""
	serverBasePath := ""
	if len(spec.Servers) > 0 {
		baseURL = strings.TrimSuffix(spec.Servers[0].URL, "/")
		if u, err := url.Parse(baseURL); err == nil && u.Path != "" && u.Path != "/" {
			serverBasePath = strings.TrimRight(u.Path, "/")
		}
	}

	// 1. Discover all existing .pebble.json request files in the folder (ignoring _folder.pebble.json)
	type localItem struct {
		filePath string
		relPath  string
		req      *types.RequestDefinition
		normPath string
		normKey  string
		opID     string
	}

	localItems := make([]*localItem, 0)
	localByKey := make(map[string]*localItem)
	localByOpID := make(map[string]*localItem)

	entries, err := os.ReadDir(folderPath)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".pebble.json") || entry.Name() == workspace.FolderFile {
				continue
			}

			fullPath := filepath.Join(folderPath, entry.Name())
			data, readErr := os.ReadFile(fullPath)
			if readErr != nil {
				continue
			}

			var req types.RequestDefinition
			if parseErr := json.Unmarshal(data, &req); parseErr != nil {
				continue
			}

			migrated, _ := workspace.MigrateRequest(&req)
			normP := normalizePath(migrated.URL)
			strippedP := normP
			if serverBasePath != "" && strings.HasPrefix(normP, serverBasePath) {
				strippedP = normalizePath(strings.TrimPrefix(normP, serverBasePath))
			}

			key := fmt.Sprintf("%s %s", strings.ToUpper(migrated.Method), strippedP)
			fullKey := fmt.Sprintf("%s %s", strings.ToUpper(migrated.Method), normP)

			item := &localItem{
				filePath: fullPath,
				relPath:  entry.Name(),
				req:      migrated,
				normPath: strippedP,
				normKey:  key,
				opID:     migrated.ID,
			}
			localItems = append(localItems, item)
			localByKey[key] = item
			localByKey[fullKey] = item
			if item.opID != "" {
				localByOpID[strings.ToLower(item.opID)] = item
			}
			if migrated.Name != "" {
				localByOpID[strings.ToLower(migrated.Name)] = item
			}
		}
	}

	matchedLocalKeys := make(map[string]bool)
	methods := []string{"get", "post", "put", "patch", "delete", "head", "options"}

	// 2. Iterate through spec paths and operations
	var sortedPaths []string
	for p := range spec.Paths {
		sortedPaths = append(sortedPaths, p)
	}
	sort.Strings(sortedPaths)

	for _, specPath := range sortedPaths {
		pathItem := spec.Paths[specPath]
		normSpecPath := normalizePath(specPath)

		for _, method := range methods {
			op, exists := pathItem[method]
			if !exists || op == nil {
				continue
			}

			upperMethod := strings.ToUpper(method)
			key := fmt.Sprintf("%s %s", upperMethod, normSpecPath)
			specReq := buildRequestFromOperation(spec, upperMethod, specPath, baseURL, op)

			// Try matching: 1st by exact Method+Path, 2nd by OperationID fallback
			var matchedLocal *localItem
			if local, ok := localByKey[key]; ok && !matchedLocalKeys[local.normKey] {
				matchedLocal = local
			} else if op.OperationID != "" {
				if local, ok := localByOpID[strings.ToLower(op.OperationID)]; ok && !matchedLocalKeys[local.normKey] {
					matchedLocal = local
				}
			}

			if matchedLocal != nil {
				matchedLocalKeys[matchedLocal.normKey] = true

				// Compare fields: method, url, parameters, body, auth
				fieldDiffs := compareFields(matchedLocal.req, specReq, serverBasePath)
				diffType := DiffUnchanged
				if len(fieldDiffs) > 0 {
					diffType = DiffChanged
					report.ChangedCount++
				}

				endpointDiff := EndpointDiff{
					ID:               key,
					OperationID:      op.OperationID,
					Summary:          op.Summary,
					Method:           upperMethod,
					Path:             normSpecPath,
					DiffType:         diffType,
					ExistingFilePath: matchedLocal.filePath,
					ProposedFilePath: matchedLocal.filePath,
					FieldDiffs:       fieldDiffs,
					SpecRequest:      specReq,
					ExistingRequest:  matchedLocal.req,
				}
				report.Endpoints = append(report.Endpoints, endpointDiff)
			} else {
				// Added endpoint
				report.AddedCount++
				fileName := impexp.SanitizeFilename(specReq.Name) + ".pebble.json"
				proposedPath := filepath.Join(folderPath, fileName)

				endpointDiff := EndpointDiff{
					ID:               key,
					OperationID:      op.OperationID,
					Summary:          op.Summary,
					Method:           upperMethod,
					Path:             normSpecPath,
					DiffType:         DiffAdded,
					ProposedFilePath: proposedPath,
					SpecRequest:      specReq,
					FieldDiffs: []FieldDiff{
						{
							Field:       "endpoint",
							OldValue:    nil,
							NewValue:    key,
							Description: fmt.Sprintf("New endpoint %s found in specification", key),
						},
					},
				}
				report.Endpoints = append(report.Endpoints, endpointDiff)
			}
		}
	}

	// 3. Detect removed endpoints (local items not in spec)
	for _, local := range localItems {
		if !matchedLocalKeys[local.normKey] {
			report.RemovedCount++
			conflict := detectConflict(local.req)
			if conflict != nil {
				report.ConflictCount++
			}

			endpointDiff := EndpointDiff{
				ID:               local.normKey,
				OperationID:      local.opID,
				Summary:          local.req.Name,
				Method:           strings.ToUpper(local.req.Method),
				Path:             local.normPath,
				DiffType:         DiffRemoved,
				ExistingFilePath: local.filePath,
				ExistingRequest:  local.req,
				Conflict:         conflict,
				FieldDiffs: []FieldDiff{
					{
						Field:       "endpoint",
						OldValue:    local.normKey,
						NewValue:    nil,
						IsConflict:  conflict != nil,
						Description: fmt.Sprintf("Endpoint %s no longer exists in specification", local.normKey),
					},
				},
			}
			report.Endpoints = append(report.Endpoints, endpointDiff)
		}
	}

	report.TotalEndpoints = len(report.Endpoints)
	report.HasDrift = report.AddedCount > 0 || report.RemovedCount > 0 || report.ChangedCount > 0

	return report, nil
}

// normalizePath converts path variable patterns and extracts clean route paths.
func normalizePath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "/"
	}

	// If URL has scheme and host (e.g. http://localhost/users or {{baseUrl}}/users)
	if strings.Contains(raw, "://") {
		if u, err := url.Parse(raw); err == nil && u.Path != "" {
			raw = u.Path
		}
	}

	// Strip dynamic host variable prefix {{host}}, {{baseUrl}}, etc.
	for strings.HasPrefix(raw, "{{") {
		idx := strings.Index(raw, "}}")
		if idx != -1 {
			raw = strings.TrimPrefix(raw[idx+2:], "/")
			if !strings.HasPrefix(raw, "/") {
				raw = "/" + raw
			}
		} else {
			break
		}
	}

	// Convert :param to {param}
	parts := strings.Split(raw, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") && len(part) > 1 {
			parts[i] = "{" + part[1:] + "}"
		}
	}

	norm := strings.Join(parts, "/")
	norm = strings.TrimRight(norm, "/")
	if norm == "" {
		norm = "/"
	}
	return norm
}

// buildRequestFromOperation constructs a RequestDefinition from an OpenAPI operation.
func buildRequestFromOperation(spec *RawSpec, method, path, baseURL string, op *Operation) *types.RequestDefinition {
	name := op.Summary
	if name == "" {
		name = op.OperationID
	}
	if name == "" {
		name = fmt.Sprintf("%s %s", method, path)
	}

	req := &types.RequestDefinition{
		SchemaVersion: workspace.CurrentRequestSchemaVersion,
		Schema:        workspace.DefaultRequestSchema,
		Version:       "1.0",
		ID:            op.OperationID,
		Name:          name,
		Method:        method,
		URL:           baseURL + path,
		Auth:          types.AuthDefinition{Type: "inherit"},
		Body:          types.BodyDefinition{Type: "none"},
		Params:        make([]types.KeyValue, 0),
		Headers:       make([]types.KeyValue, 0),
		Extractors:    make([]types.ExtractorDefinition, 0),
		Settings: types.SettingDefinition{
			FollowRedirects: true,
			VerifySSL:       true,
			TimeoutMs:       30000,
		},
	}

	// Parameters
	for _, p := range op.Parameters {
		if p == nil {
			continue
		}
		val := ""
		if p.Example != nil {
			val = fmt.Sprintf("%v", p.Example)
		} else if p.Schema != nil && p.Schema.Default != nil {
			val = fmt.Sprintf("%v", p.Schema.Default)
		}

		switch p.In {
		case "query":
			req.Params = append(req.Params, types.KeyValue{
				Key:     p.Name,
				Value:   val,
				Enabled: true,
			})
		case "header":
			req.Headers = append(req.Headers, types.KeyValue{
				Key:     p.Name,
				Value:   val,
				Enabled: true,
			})
		}
	}

	// Request Body
	if op.RequestBody != nil && op.RequestBody.Content != nil {
		if ct, ok := op.RequestBody.Content["application/json"]; ok && ct != nil {
			skeleton := BuildJSONSkeleton(ct.Schema)
			rawBytes, _ := json.MarshalIndent(skeleton, "", "  ")
			req.Body = types.BodyDefinition{
				Type: "json",
				Raw:  string(rawBytes),
			}
		} else if ct, ok := op.RequestBody.Content["multipart/form-data"]; ok && ct != nil {
			var kvs []types.KeyValue
			if ct.Schema != nil {
				for k := range ct.Schema.Properties {
					kvs = append(kvs, types.KeyValue{Key: k, Value: "", Enabled: true})
				}
			}
			req.Body = types.BodyDefinition{
				Type:     "formData",
				FormData: kvs,
			}
		} else if ct, ok := op.RequestBody.Content["application/x-www-form-urlencoded"]; ok && ct != nil {
			var kvs []types.KeyValue
			if ct.Schema != nil {
				for k := range ct.Schema.Properties {
					kvs = append(kvs, types.KeyValue{Key: k, Value: "", Enabled: true})
				}
			}
			req.Body = types.BodyDefinition{
				Type:       "urlEncoded",
				UrlEncoded: kvs,
			}
		}
	}

	// Auth from Operation Security
	if len(op.Security) > 0 && spec.Components != nil && spec.Components.SecuritySchemes != nil {
		for _, secRequirement := range op.Security {
			for schemeName := range secRequirement {
				if scheme, ok := spec.Components.SecuritySchemes[schemeName]; ok && scheme != nil {
					switch strings.ToLower(scheme.Type) {
					case "http":
						if strings.ToLower(scheme.Scheme) == "bearer" {
							req.Auth = types.AuthDefinition{Type: "bearer"}
						} else if strings.ToLower(scheme.Scheme) == "basic" {
							req.Auth = types.AuthDefinition{Type: "basic"}
						}
					case "apikey":
						req.Auth = types.AuthDefinition{
							Type:  "apiKey",
							Key:   scheme.Name,
							AddTo: scheme.In,
						}
					case "oauth2":
						req.Auth = types.AuthDefinition{Type: "oauth2"}
					}
					break
				}
			}
		}
	}

	return req
}

// compareFields inspects method, url, query parameters, body schema, and auth differences.
func compareFields(local, spec *types.RequestDefinition, serverBasePath string) []FieldDiff {
	diffs := make([]FieldDiff, 0)

	// 1. Method
	if !strings.EqualFold(local.Method, spec.Method) {
		diffs = append(diffs, FieldDiff{
			Field:       "method",
			OldValue:    local.Method,
			NewValue:    spec.Method,
			Description: fmt.Sprintf("HTTP method changed from %s to %s", local.Method, spec.Method),
		})
	}

	// 2. URL Path
	localNorm := normalizePath(local.URL)
	specNorm := normalizePath(spec.URL)
	strippedLocal := localNorm
	strippedSpec := specNorm
	if serverBasePath != "" {
		if strings.HasPrefix(strippedLocal, serverBasePath) {
			strippedLocal = normalizePath(strings.TrimPrefix(strippedLocal, serverBasePath))
		}
		if strings.HasPrefix(strippedSpec, serverBasePath) {
			strippedSpec = normalizePath(strings.TrimPrefix(strippedSpec, serverBasePath))
		}
	}
	if localNorm != specNorm && strippedLocal != strippedSpec {
		diffs = append(diffs, FieldDiff{
			Field:       "url",
			OldValue:    localNorm,
			NewValue:    specNorm,
			Description: fmt.Sprintf("Path changed from %s to %s", localNorm, specNorm),
		})
	}

	// 3. Query Parameters
	localParamKeys := make(map[string]bool)
	for _, p := range local.Params {
		if p.Key != "" {
			localParamKeys[p.Key] = true
		}
	}
	specParamKeys := make(map[string]bool)
	for _, p := range spec.Params {
		if p.Key != "" {
			specParamKeys[p.Key] = true
		}
	}

	var addedParams, removedParams []string
	for k := range specParamKeys {
		if !localParamKeys[k] {
			addedParams = append(addedParams, k)
		}
	}
	for k := range localParamKeys {
		if !specParamKeys[k] {
			removedParams = append(removedParams, k)
		}
	}

	if len(addedParams) > 0 || len(removedParams) > 0 {
		sort.Strings(addedParams)
		sort.Strings(removedParams)
		diffs = append(diffs, FieldDiff{
			Field:       "parameters",
			OldValue:    removedParams,
			NewValue:    addedParams,
			Description: fmt.Sprintf("Query parameters modified: +%v -%v", addedParams, removedParams),
		})
	}

	// 4. Request Body
	if local.Body.Type != spec.Body.Type {
		diffs = append(diffs, FieldDiff{
			Field:       "body.type",
			OldValue:    local.Body.Type,
			NewValue:    spec.Body.Type,
			Description: fmt.Sprintf("Body type changed from %s to %s", local.Body.Type, spec.Body.Type),
		})
	} else if spec.Body.Type == "json" && spec.Body.Raw != "" {
		// Compare JSON schemas by parsing skeletons
		var localJSON, specJSON any
		_ = json.Unmarshal([]byte(local.Body.Raw), &localJSON)
		_ = json.Unmarshal([]byte(spec.Body.Raw), &specJSON)

		if !reflect.DeepEqual(extractJSONStructure(localJSON), extractJSONStructure(specJSON)) {
			diffs = append(diffs, FieldDiff{
				Field:       "body.schema",
				OldValue:    local.Body.Raw,
				NewValue:    spec.Body.Raw,
				Description: "Request body JSON schema structure was updated in spec",
			})
		}
	}

	// 5. Auth
	if spec.Auth.Type != "inherit" && spec.Auth.Type != "" && !strings.EqualFold(local.Auth.Type, spec.Auth.Type) {
		diffs = append(diffs, FieldDiff{
			Field:       "auth",
			OldValue:    local.Auth.Type,
			NewValue:    spec.Auth.Type,
			Description: fmt.Sprintf("Auth requirement changed from %s to %s", local.Auth.Type, spec.Auth.Type),
		})
	}

	return diffs
}

// extractJSONStructure normalizes a parsed JSON structure into key-type mappings.
func extractJSONStructure(v any) any {
	if m, ok := v.(map[string]any); ok {
		structMap := make(map[string]string)
		for k, val := range m {
			structMap[k] = fmt.Sprintf("%T", val)
		}
		return structMap
	}
	return fmt.Sprintf("%T", v)
}

// detectConflict identifies user-defined assets that make removal risky.
func detectConflict(req *types.RequestDefinition) *ConflictInfo {
	if req == nil {
		return nil
	}

	hasScripts := strings.TrimSpace(req.Scripts.PreRequest) != "" || strings.TrimSpace(req.Scripts.PostResponse) != ""
	hasExamples := len(req.Examples) > 0
	hasExtractors := len(req.Extractors) > 0
	hasCustomHeaders := len(req.Headers) > 0

	if !hasScripts && !hasExamples && !hasExtractors && !hasCustomHeaders {
		return nil
	}

	var reasons []string
	if hasScripts {
		reasons = append(reasons, "pre-request / test scripts")
	}
	if hasExamples {
		reasons = append(reasons, fmt.Sprintf("%d saved examples", len(req.Examples)))
	}
	if hasExtractors {
		reasons = append(reasons, fmt.Sprintf("%d variable extractors", len(req.Extractors)))
	}
	if hasCustomHeaders {
		reasons = append(reasons, fmt.Sprintf("%d custom headers", len(req.Headers)))
	}

	return &ConflictInfo{
		HasUserScripts:       hasScripts,
		HasUserExamples:      hasExamples,
		HasUserExtractors:    hasExtractors,
		HasUserCustomHeaders: hasCustomHeaders,
		Details:              fmt.Sprintf("Request contains user-defined assets: %s", strings.Join(reasons, ", ")),
	}
}
