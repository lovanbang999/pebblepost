package docs_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pebblepost/internal/docs"
	"pebblepost/internal/types"
	"pebblepost/internal/workspace"
)

// setupSampleWorkspace sets up a test workspace with folders, requests, and examples.
func setupSampleWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	wsSvc := workspace.NewWorkspaceService()
	_, err := wsSvc.Init(dir, "PetStore API")
	if err != nil {
		t.Fatalf("failed to init workspace: %v", err)
	}

	// 1. Create a users folder with _folder.pebble.json
	usersDir := filepath.Join(dir, "collections", "01-users")
	if err := os.MkdirAll(usersDir, 0755); err != nil {
		t.Fatalf("failed to create users dir: %v", err)
	}
	folderDef := types.FolderDefinition{
		SchemaVersion: 1,
		Name:          "Users API",
		Description:   "User management and authentication endpoints.",
		ItemOrder:     []string{"get-user.pebble.json", "create-user.pebble.json"},
	}
	if err := workspace.WriteFileStable(filepath.Join(usersDir, workspace.FolderFile), folderDef); err != nil {
		t.Fatalf("failed to write folder file: %v", err)
	}

	// 2. Create get-user.pebble.json with an example response
	getUserReq := types.RequestDefinition{
		Name:        "Get User Details",
		Description: "Fetch a specific user profile by user ID.",
		Method:      "GET",
		URL:         "https://api.petstore.com/v1/users/:id",
		Params: []types.KeyValue{
			{Key: "include_orders", Value: "true", Enabled: true},
		},
		Headers: []types.KeyValue{
			{Key: "Accept", Value: "application/json", Enabled: true},
		},
		Auth: types.AuthDefinition{
			Type:  "bearer",
			Token: "sample_bearer_token",
		},
		Examples: []types.ExampleResponse{
			{
				ID:          "ex_1",
				Name:        "200 OK User Profile",
				StatusCode:  200,
				StatusText:  "OK",
				ContentType: "application/json",
				Headers: []types.KeyValue{
					{Key: "Content-Type", Value: "application/json", Enabled: true},
				},
				Body: `{"id": "usr_101", "name": "Alice Smith", "email": "alice@example.com"}`,
			},
		},
	}
	if err := wsSvc.SaveRequest(filepath.Join(usersDir, "get-user.pebble.json"), &getUserReq); err != nil {
		t.Fatalf("failed to save get-user: %v", err)
	}

	// 3. Create create-user.pebble.json
	createUserReq := types.RequestDefinition{
		Name:        "Create New User",
		Description: "Register a new pet store customer.",
		Method:      "POST",
		URL:         "https://api.petstore.com/v1/users",
		Headers: []types.KeyValue{
			{Key: "Content-Type", Value: "application/json", Enabled: true},
		},
		Body: types.BodyDefinition{
			Type: "json",
			Raw:  `{"name": "Bob Jones", "email": "bob@example.com"}`,
		},
		Examples: []types.ExampleResponse{
			{
				ID:          "ex_2",
				Name:        "201 Created Response",
				StatusCode:  201,
				StatusText:  "Created",
				ContentType: "application/json",
				Body:        `{"id": "usr_102", "name": "Bob Jones", "email": "bob@example.com", "createdAt": "2026-10-04T12:00:00Z"}`,
			},
		},
	}
	if err := wsSvc.SaveRequest(filepath.Join(usersDir, "create-user.pebble.json"), &createUserReq); err != nil {
		t.Fatalf("failed to save create-user: %v", err)
	}

	return dir
}

func TestDocs_GoldenMarkdown(t *testing.T) {
	wsDir := setupSampleWorkspace(t)
	gen := docs.NewGenerator()

	data, filename, err := gen.Generate(wsDir, filepath.Join(wsDir, "collections"), "md")
	if err != nil {
		t.Fatalf("Generate markdown failed: %v", err)
	}
	if filename != "index.md" {
		t.Errorf("expected filename index.md, got %s", filename)
	}

	content := string(data)

	// Verify essential markdown components
	if !strings.Contains(content, "# PetStore API") && !strings.Contains(content, "# collections") {
		t.Errorf("missing title in markdown output")
	}
	if !strings.Contains(content, "## Table of Contents") {
		t.Errorf("missing Table of Contents in markdown output")
	}
	if !strings.Contains(content, "Users API") {
		t.Errorf("missing folder name 'Users API' in markdown output")
	}
	if !strings.Contains(content, "Get User Details") {
		t.Errorf("missing request name in markdown output")
	}
	if !strings.Contains(content, "```bash") || !strings.Contains(content, "curl -X GET") {
		t.Errorf("missing curl code snippet in markdown output")
	}
	if !strings.Contains(content, "200 OK User Profile") {
		t.Errorf("missing saved example response in markdown output")
	}

	// Golden comparison
	goldenPath := filepath.Join("testdata", "golden.md")
	if os.Getenv("UPDATE_GOLDEN") == "true" {
		_ = os.MkdirAll("testdata", 0755)
		_ = os.WriteFile(goldenPath, data, 0644)
	} else if goldenData, err := os.ReadFile(goldenPath); err == nil {
		if string(data) != string(goldenData) {
			t.Errorf("markdown output does not match golden.md")
		}
	}
}

func TestDocs_GoldenHTML(t *testing.T) {
	wsDir := setupSampleWorkspace(t)
	gen := docs.NewGenerator()

	data, filename, err := gen.Generate(wsDir, filepath.Join(wsDir, "collections"), "html")
	if err != nil {
		t.Fatalf("Generate html failed: %v", err)
	}
	if filename != "index.html" {
		t.Errorf("expected filename index.html, got %s", filename)
	}

	content := string(data)
	if !strings.Contains(content, "<!DOCTYPE html>") {
		t.Errorf("missing DOCTYPE in html output")
	}
	if !strings.Contains(content, "Users API") {
		t.Errorf("missing folder in html output")
	}
	if !strings.Contains(content, "Get User Details") {
		t.Errorf("missing request title in html output")
	}
	if !strings.Contains(content, "copySnippet") {
		t.Errorf("missing client-side javascript in html output")
	}
	if !strings.Contains(content, "200 OK User Profile") {
		t.Errorf("missing example response in html output")
	}

	// Golden comparison
	goldenPath := filepath.Join("testdata", "golden.html")
	if os.Getenv("UPDATE_GOLDEN") == "true" {
		_ = os.MkdirAll("testdata", 0755)
		_ = os.WriteFile(goldenPath, data, 0644)
	} else if goldenData, err := os.ReadFile(goldenPath); err == nil {
		if string(data) != string(goldenData) {
			t.Errorf("html output does not match golden.html")
		}
	}
}

func TestDocs_GoldenOpenAPI(t *testing.T) {
	wsDir := setupSampleWorkspace(t)
	gen := docs.NewGenerator()

	// 1. YAML Format
	yamlData, yamlFilename, err := gen.Generate(wsDir, filepath.Join(wsDir, "collections"), "openapi")
	if err != nil {
		t.Fatalf("Generate openapi yaml failed: %v", err)
	}
	if yamlFilename != "openapi.yaml" {
		t.Errorf("expected openapi.yaml, got %s", yamlFilename)
	}
	yamlStr := string(yamlData)
	if !strings.Contains(yamlStr, "openapi: 3.0.3") {
		t.Errorf("missing openapi: 3.0.3 header in yaml output")
	}
	if !strings.Contains(yamlStr, "/v1/users/{id}") {
		t.Errorf("missing path parameter translation in yaml output")
	}
	if !strings.Contains(yamlStr, "200 OK User Profile") && !strings.Contains(yamlStr, "usr_101") {
		t.Errorf("missing example content in yaml output")
	}

	// 2. JSON Format
	jsonData, jsonFilename, err := gen.Generate(wsDir, filepath.Join(wsDir, "collections"), "json")
	if err != nil {
		t.Fatalf("Generate openapi json failed: %v", err)
	}
	if jsonFilename != "openapi.json" {
		t.Errorf("expected openapi.json, got %s", jsonFilename)
	}
	jsonStr := string(jsonData)
	if !strings.Contains(jsonStr, `"openapi": "3.0.3"`) {
		t.Errorf("missing openapi 3.0.3 in json output")
	}

	// Golden comparison
	goldenPath := filepath.Join("testdata", "golden.openapi.yaml")
	if os.Getenv("UPDATE_GOLDEN") == "true" {
		_ = os.MkdirAll("testdata", 0755)
		_ = os.WriteFile(goldenPath, yamlData, 0644)
	} else if goldenData, err := os.ReadFile(goldenPath); err == nil {
		if string(yamlData) != string(goldenData) {
			t.Errorf("openapi output does not match golden.openapi.yaml")
		}
	}
}

func TestDocs_WriteDocs(t *testing.T) {
	wsDir := setupSampleWorkspace(t)
	gen := docs.NewGenerator()

	outDir := t.TempDir()

	// 1. Write to directory
	writtenPath, err := gen.WriteDocs(wsDir, filepath.Join(wsDir, "collections"), "md", outDir)
	if err != nil {
		t.Fatalf("WriteDocs failed: %v", err)
	}
	if filepath.Base(writtenPath) != "index.md" {
		t.Errorf("expected written path to end with index.md, got %s", writtenPath)
	}
	if fi, err := os.Stat(writtenPath); err != nil || fi.Size() == 0 {
		t.Errorf("expected non-empty output file at %s", writtenPath)
	}

	// 2. Write to specific file
	customFile := filepath.Join(outDir, "custom", "api-spec.yaml")
	writtenCustom, err := gen.WriteDocs(wsDir, filepath.Join(wsDir, "collections"), "openapi", customFile)
	if err != nil {
		t.Fatalf("WriteDocs custom failed: %v", err)
	}
	if writtenCustom != customFile {
		t.Errorf("expected written path %s, got %s", customFile, writtenCustom)
	}
	if fi, err := os.Stat(customFile); err != nil || fi.Size() == 0 {
		t.Errorf("expected non-empty output file at %s", customFile)
	}
}
