package openapisync

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pebblepost/internal/types"
	"pebblepost/internal/workspace"
)

const specV1YAML = `
openapi: 3.0.3
info:
  title: Users Service API
  version: 1.0.0
servers:
  - url: https://api.example.com/v1
paths:
  /users:
    get:
      summary: List Users
      operationId: listUsers
      parameters:
        - name: limit
          in: query
          required: false
          schema:
            type: integer
          example: 20
    post:
      summary: Create User
      operationId: createUser
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                name:
                  type: string
                email:
                  type: string
  /legacy-data:
    get:
      summary: Legacy Data Endpoint
      operationId: getLegacyData
`

const specV2YAML = `
openapi: 3.0.3
info:
  title: Users Service API
  version: 2.0.0
servers:
  - url: https://api.example.com/v1
paths:
  /users:
    get:
      summary: List Users
      operationId: listUsers
      parameters:
        - name: limit
          in: query
          required: false
          schema:
            type: integer
          example: 50
        - name: page
          in: query
          required: false
          schema:
            type: integer
          example: 1
    post:
      summary: Create User
      operationId: createUser
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              properties:
                name:
                  type: string
                email:
                  type: string
                role:
                  type: string
  /users/{id}:
    delete:
      summary: Delete User
      operationId: deleteUser
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
`

func TestOpenAPISync_V1toV2_DiffAndApply(t *testing.T) {
	tmpDir := t.TempDir()
	folderPath := filepath.Join(tmpDir, "users-collection")
	if err := os.MkdirAll(folderPath, 0755); err != nil {
		t.Fatalf("failed to create test folder: %v", err)
	}

	wsSvc := workspace.NewWorkspaceService()

	// 1. Parse and apply v1 spec to create initial collection
	spec1, err := ParseSpec([]byte(specV1YAML))
	if err != nil {
		t.Fatalf("failed to parse spec v1: %v", err)
	}
	hash1 := ComputeSpecHash([]byte(specV1YAML))

	diff1, err := CompareSpecAndFolder(folderPath, spec1, hash1, "spec_v1.yaml")
	if err != nil {
		t.Fatalf("diff v1 failed: %v", err)
	}
	if diff1.AddedCount != 3 {
		t.Fatalf("expected 3 added endpoints in v1, got %d", diff1.AddedCount)
	}

	apply1, err := ApplyDiff(wsSvc, folderPath, diff1, nil, false)
	if err != nil {
		t.Fatalf("apply v1 failed: %v", err)
	}
	if apply1.AppliedCount != 3 {
		t.Fatalf("expected 3 applied in v1, got %d", apply1.AppliedCount)
	}

	// Verify diff against v1 now reports 0 drift
	diff1Again, err := CompareSpecAndFolder(folderPath, spec1, hash1, "spec_v1.yaml")
	if err != nil {
		t.Fatalf("diff v1 again failed: %v", err)
	}
	if diff1Again.HasDrift {
		t.Errorf("expected no drift against v1, but got drift")
	}

	// 2. Add user customizations to existing files
	// User customizes POST /users: add script, custom header, and filled body values
	createUserFile := filepath.Join(folderPath, "create-user.pebble.json")
	createReqData, err := os.ReadFile(createUserFile)
	if err != nil {
		t.Fatalf("failed to read create-user file: %v", err)
	}
	var createReq types.RequestDefinition
	_ = json.Unmarshal(createReqData, &createReq)
	createReq.Scripts.PreRequest = "console.log('preparing user');"
	createReq.Scripts.PostResponse = "expect(response.status).toBe(201);"
	createReq.Headers = append(createReq.Headers, types.KeyValue{Key: "X-Custom-Tenant", Value: "tenant-123", Enabled: true})
	createReq.Body.Raw = `{"name": "Alice", "email": "alice@example.com"}`
	_ = workspace.WriteFileStable(createUserFile, &createReq)

	// User customizes GET /legacy-data: add test script, extractor, and example
	legacyFile := filepath.Join(folderPath, "legacy-data-endpoint.pebble.json")
	legacyData, err := os.ReadFile(legacyFile)
	if err != nil {
		t.Fatalf("failed to read legacy file: %v", err)
	}
	var legacyReq types.RequestDefinition
	_ = json.Unmarshal(legacyData, &legacyReq)
	legacyReq.Scripts.PostResponse = "expect(response.status).toBe(200);"
	legacyReq.Extractors = []types.ExtractorDefinition{
		{ID: "ex1", Name: "LEGACY_TOKEN", Type: "jsonpath", Path: "$.token", Scope: "runtime", Enabled: true},
	}
	legacyReq.Examples = []types.ExampleResponse{
		{ID: "ex1", Name: "Sample Response", Body: `{"token": "xyz"}`},
	}
	_ = workspace.WriteFileStable(legacyFile, &legacyReq)

	// 3. Diff against Spec v2
	spec2, err := ParseSpec([]byte(specV2YAML))
	if err != nil {
		t.Fatalf("failed to parse spec v2: %v", err)
	}
	hash2 := ComputeSpecHash([]byte(specV2YAML))

	diff2, err := CompareSpecAndFolder(folderPath, spec2, hash2, "spec_v2.yaml")
	if err != nil {
		t.Fatalf("diff v2 failed: %v", err)
	}

	if !diff2.HasDrift {
		t.Fatalf("expected drift between v1 collection and v2 spec")
	}

	// Verify diff breakdown:
	// - 1 Added: DELETE /users/{id}
	// - 1 Removed: GET /legacy-data (must have conflict due to user scripts/extractors/examples)
	// - 2 Changed: GET /users (added query param 'page') and POST /users (new 'role' property in body)
	if diff2.AddedCount != 1 {
		t.Errorf("expected 1 added endpoint, got %d", diff2.AddedCount)
	}
	if diff2.RemovedCount != 1 {
		t.Errorf("expected 1 removed endpoint, got %d", diff2.RemovedCount)
	}
	if diff2.ChangedCount != 2 {
		t.Errorf("expected 2 changed endpoints, got %d", diff2.ChangedCount)
	}
	if diff2.ConflictCount != 1 {
		t.Errorf("expected 1 conflict (on legacy-data), got %d", diff2.ConflictCount)
	}

	// Verify conflict details on legacy endpoint
	var legacyDiff *EndpointDiff
	for i, ep := range diff2.Endpoints {
		if ep.DiffType == DiffRemoved {
			legacyDiff = &diff2.Endpoints[i]
			break
		}
	}
	if legacyDiff == nil || legacyDiff.Conflict == nil {
		t.Fatalf("expected conflict on removed legacy endpoint")
	}
	if !legacyDiff.Conflict.HasUserScripts || !legacyDiff.Conflict.HasUserExamples || !legacyDiff.Conflict.HasUserExtractors {
		t.Errorf("expected conflict to detect scripts, examples, and extractors")
	}

	// 4. Apply non-conflicting changes (force = false)
	apply2, err := ApplyDiff(wsSvc, folderPath, diff2, nil, false)
	if err != nil {
		t.Fatalf("apply v2 non-conflicting failed: %v", err)
	}

	// Should have applied 3 changes (1 add + 2 changed), skipped 1 conflicting removal
	if apply2.AppliedCount != 3 {
		t.Errorf("expected 3 applied changes, got %d", apply2.AppliedCount)
	}
	if apply2.SkippedCount != 1 {
		t.Errorf("expected 1 skipped change, got %d", apply2.SkippedCount)
	}

	// Check that legacy file was NOT deleted
	if _, err := os.Stat(legacyFile); os.IsNotExist(err) {
		t.Fatalf("legacy file was erroneously deleted despite conflict!")
	}

	// Check that new DELETE endpoint was created
	deleteUserFile := filepath.Join(folderPath, "delete-user.pebble.json")
	if _, err := os.Stat(deleteUserFile); os.IsNotExist(err) {
		t.Fatalf("expected delete-user.pebble.json to be created")
	}

	// Check that POST /users preserved user scripts, custom header, and merged new body property
	updatedCreateData, _ := os.ReadFile(createUserFile)
	var updatedCreateReq types.RequestDefinition
	_ = json.Unmarshal(updatedCreateData, &updatedCreateReq)

	if updatedCreateReq.Scripts.PreRequest != "console.log('preparing user');" {
		t.Errorf("user PreRequest script was overwritten!")
	}
	if updatedCreateReq.Scripts.PostResponse != "expect(response.status).toBe(201);" {
		t.Errorf("user PostResponse script was overwritten!")
	}

	hasCustomHeader := false
	for _, h := range updatedCreateReq.Headers {
		if h.Key == "X-Custom-Tenant" && h.Value == "tenant-123" {
			hasCustomHeader = true
			break
		}
	}
	if !hasCustomHeader {
		t.Errorf("custom header X-Custom-Tenant was wiped out!")
	}

	if !strings.Contains(updatedCreateReq.Body.Raw, "role") {
		t.Errorf("new 'role' property from v2 spec was not merged into request body: %s", updatedCreateReq.Body.Raw)
	}
	if !strings.Contains(updatedCreateReq.Body.Raw, "Alice") {
		t.Errorf("existing 'Alice' value was overwritten by spec merge: %s", updatedCreateReq.Body.Raw)
	}

	// 5. Apply with force = true removes the legacy endpoint
	applyForce, err := ApplyDiff(wsSvc, folderPath, diff2, []string{legacyDiff.ID}, true)
	if err != nil {
		t.Fatalf("apply force failed: %v", err)
	}
	if applyForce.AppliedCount != 1 {
		t.Errorf("expected 1 applied in force mode, got %d", applyForce.AppliedCount)
	}
	if _, err := os.Stat(legacyFile); !os.IsNotExist(err) {
		t.Fatalf("legacy file should have been removed with force=true")
	}
}

func TestLoadSpec_RemoteAndSSRF(t *testing.T) {
	// Remote HTTP server serving valid spec
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write([]byte(specV1YAML))
	}))
	defer ts.Close()

	data, hash, err := LoadSpec(ts.URL, "")
	if err != nil {
		t.Fatalf("failed to load spec from test server: %v", err)
	}
	if len(data) == 0 || hash == "" {
		t.Fatalf("expected non-empty data and hash")
	}

	// SSRF block for cloud metadata service
	_, _, err = LoadSpec("http://169.254.169.254/latest/meta-data", "")
	if err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("expected SSRF block for cloud metadata IP, got: %v", err)
	}

	_, _, err = LoadSpec("http://metadata.google.internal/computeMetadata/v1/", "")
	if err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("expected SSRF block for Google metadata host, got: %v", err)
	}
}

func TestOpenAPISync_JSONSpecAndSelectiveApply(t *testing.T) {
	jsonSpec := `{
  "openapi": "3.0.0",
  "info": { "title": "Inventory API", "version": "1.0.0" },
  "servers": [{ "url": "https://inventory.example.com" }],
  "paths": {
    "/items": {
      "get": { "summary": "Get Items", "operationId": "getItems" },
      "post": { "summary": "Add Item", "operationId": "addItem" }
    }
  }
}`

	tmpDir := t.TempDir()
	folderPath := filepath.Join(tmpDir, "inventory")
	_ = os.MkdirAll(folderPath, 0755)

	spec, err := ParseSpec([]byte(jsonSpec))
	if err != nil {
		t.Fatalf("failed to parse JSON spec: %v", err)
	}
	hash := ComputeSpecHash([]byte(jsonSpec))

	diffReport, err := CompareSpecAndFolder(folderPath, spec, hash, "inventory.json")
	if err != nil {
		t.Fatalf("diff failed: %v", err)
	}

	if diffReport.AddedCount != 2 {
		t.Fatalf("expected 2 added items, got %d", diffReport.AddedCount)
	}

	wsSvc := workspace.NewWorkspaceService()
	// Only accept "GET /items" selectively
	applyRes, err := ApplyDiff(wsSvc, folderPath, diffReport, []string{"GET /items"}, false)
	if err != nil {
		t.Fatalf("selective apply failed: %v", err)
	}
	if applyRes.AppliedCount != 1 || applyRes.SkippedCount != 1 {
		t.Errorf("expected 1 applied and 1 skipped, got applied=%d, skipped=%d", applyRes.AppliedCount, applyRes.SkippedCount)
	}

	// Verify only GET item file was created
	getItemsFile := filepath.Join(folderPath, "get-items.pebble.json")
	if _, err := os.Stat(getItemsFile); os.IsNotExist(err) {
		t.Errorf("expected get-items.pebble.json to exist")
	}

	addItemFile := filepath.Join(folderPath, "add-item.pebble.json")
	if _, err := os.Stat(addItemFile); !os.IsNotExist(err) {
		t.Errorf("add-item.pebble.json should not exist due to selective apply")
	}

	// Verify _folder.pebble.json was updated with schemaVersion 2 and openApiSync
	folderDef, err := wsSvc.ReadFolder(folderPath)
	if err != nil {
		t.Fatalf("failed to read folder: %v", err)
	}
	if folderDef.SchemaVersion != 2 {
		t.Errorf("expected folder schemaVersion 2, got %d", folderDef.SchemaVersion)
	}
	if folderDef.OpenAPISync == nil || folderDef.OpenAPISync.SpecHash != hash {
		t.Errorf("expected OpenAPISync config and specHash to be recorded")
	}
}
