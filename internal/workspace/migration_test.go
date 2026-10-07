package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pebblepost/internal/types"
)

func TestMigrateRequest_V0toV2(t *testing.T) {
	req := &types.RequestDefinition{
		Name:   "Get User",
		Method: "GET",
		URL:    "https://api.example.com/users",
	}

	migrated, wasMigrated := MigrateRequest(req)
	if !wasMigrated {
		t.Fatalf("expected wasMigrated to be true for v0 request")
	}
	if migrated.SchemaVersion != 2 {
		t.Errorf("expected SchemaVersion 2, got %d", migrated.SchemaVersion)
	}
	if migrated.Schema != DefaultRequestSchema {
		t.Errorf("expected Schema %s, got %s", DefaultRequestSchema, migrated.Schema)
	}
	if migrated.Version != "1.0" {
		t.Errorf("expected Version '1.0', got %s", migrated.Version)
	}

	// Migrating again should return wasMigrated = false
	_, wasMigratedAgain := MigrateRequest(migrated)
	if wasMigratedAgain {
		t.Errorf("expected wasMigrated to be false when already v2")
	}
}

func TestMigrateRequest_V1toV2(t *testing.T) {
	req := &types.RequestDefinition{
		SchemaVersion: 1,
		Schema:        "https://pebblepost.dev/schemas/v1/request.json",
		Name:          "Get User V1",
		Method:        "GET",
		URL:           "https://api.example.com/users",
		Auth:          types.AuthDefinition{Type: "bearer"},
	}

	migrated, wasMigrated := MigrateRequest(req)
	if !wasMigrated {
		t.Fatalf("expected wasMigrated to be true for v1 request")
	}
	if migrated.SchemaVersion != 2 {
		t.Errorf("expected SchemaVersion 2, got %d", migrated.SchemaVersion)
	}
	if migrated.Schema != DefaultRequestSchema {
		t.Errorf("expected Schema %s, got %s", DefaultRequestSchema, migrated.Schema)
	}
	if migrated.Extractors == nil {
		t.Errorf("expected Extractors to be initialized")
	}
}

func TestMigrateEnvironment_V0toV1(t *testing.T) {
	env := &types.EnvironmentDefinition{
		Name: "staging",
		Variables: []types.KeyValue{
			{Key: "HOST", Value: "staging.example.com", Enabled: true},
		},
	}

	migrated, wasMigrated := MigrateEnvironment(env)
	if !wasMigrated {
		t.Fatalf("expected wasMigrated to be true for v0 environment")
	}
	if migrated.SchemaVersion != 1 {
		t.Errorf("expected SchemaVersion 1, got %d", migrated.SchemaVersion)
	}
}

func TestMigrateWorkspace_V0toV1(t *testing.T) {
	ws := &types.WorkspaceDefinition{
		Name:    "My Workspace",
		Trusted: true,
	}

	migrated, wasMigrated := MigrateWorkspace(ws)
	if !wasMigrated {
		t.Fatalf("expected wasMigrated to be true for v0 workspace")
	}
	if migrated.SchemaVersion != 1 {
		t.Errorf("expected SchemaVersion 1, got %d", migrated.SchemaVersion)
	}
}

func TestMigrateFolder_V0toV2(t *testing.T) {
	folder := &types.FolderDefinition{
		Order:     2,
		ItemOrder: []string{"login.pebble.json", "logout.pebble.json"},
	}

	migrated, wasMigrated := MigrateFolder(folder)
	if !wasMigrated {
		t.Fatalf("expected wasMigrated to be true for v0 folder")
	}
	if migrated.SchemaVersion != 2 {
		t.Errorf("expected SchemaVersion 2, got %d", migrated.SchemaVersion)
	}
}

func TestMigrateFolder_V1toV2(t *testing.T) {
	folder := &types.FolderDefinition{
		SchemaVersion: 1,
		Name:          "Auth Folder",
		Headers: []types.KeyValue{
			{Key: "X-App-Env", Value: "staging", Enabled: true},
		},
		Auth: types.AuthDefinition{Type: "bearer", Token: "abc"},
	}

	migrated, wasMigrated := MigrateFolder(folder)
	if !wasMigrated {
		t.Fatalf("expected wasMigrated to be true for v1 folder")
	}
	if migrated.SchemaVersion != 2 {
		t.Errorf("expected SchemaVersion 2, got %d", migrated.SchemaVersion)
	}
	if migrated.Name != "Auth Folder" {
		t.Errorf("expected Name 'Auth Folder', got %s", migrated.Name)
	}
	if len(migrated.Headers) != 1 || migrated.Headers[0].Key != "X-App-Env" {
		t.Errorf("expected Headers preserved")
	}

	// Migrating already v2 returns false
	_, wasMigratedAgain := MigrateFolder(migrated)
	if wasMigratedAgain {
		t.Errorf("expected wasMigrated to be false when already v2")
	}
}

func TestReadRequest_DoesNotOverwriteDisk(t *testing.T) {
	tmpDir := t.TempDir()
	reqPath := filepath.Join(tmpDir, "test.pebble.json")

	// Raw v0 JSON without schemaVersion
	v0JSON := `{
  "name": "Legacy Request",
  "method": "POST",
  "url": "https://api.example.com/legacy",
  "headers": [],
  "params": [],
  "auth": { "type": "none" },
  "body": { "type": "none" },
  "scripts": { "preRequest": "", "postResponse": "" },
  "settings": { "followRedirects": true, "verifySSL": true, "timeoutMs": 30000 }
}
`
	if err := os.WriteFile(reqPath, []byte(v0JSON), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	svc := NewWorkspaceService()

	// 1. Read request: should be migrated in-memory
	readReq, err := svc.ReadRequest(reqPath)
	if err != nil {
		t.Fatalf("ReadRequest failed: %v", err)
	}
	if readReq.SchemaVersion != 2 {
		t.Errorf("expected in-memory SchemaVersion 2, got %d", readReq.SchemaVersion)
	}

	// 2. Verify disk file remains untouched (no schemaVersion on disk yet)
	rawOnDisk, err := os.ReadFile(reqPath)
	if err != nil {
		t.Fatalf("failed to read disk file: %v", err)
	}
	if string(rawOnDisk) != v0JSON {
		t.Errorf("expected disk file to remain unchanged after ReadRequest, but it was modified!\nGot:\n%s\nExpected:\n%s", string(rawOnDisk), v0JSON)
	}
	if strings.Contains(string(rawOnDisk), "schemaVersion") {
		t.Errorf("disk file must NOT contain schemaVersion before explicit save")
	}

	// 3. Explicit save should update disk file with schemaVersion
	if err := svc.SaveRequest(reqPath, readReq); err != nil {
		t.Fatalf("SaveRequest failed: %v", err)
	}

	rawAfterSave, err := os.ReadFile(reqPath)
	if err != nil {
		t.Fatalf("failed to read disk file after save: %v", err)
	}
	if !strings.Contains(string(rawAfterSave), `"schemaVersion": 2`) {
		t.Errorf("expected saved file to contain '\"schemaVersion\": 2', got:\n%s", string(rawAfterSave))
	}
}
