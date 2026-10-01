package workspace

import (
	"path/filepath"
	"testing"

	"pebblepost/internal/types"
)

func TestWorkspaceService_Init(t *testing.T) {
	tempDir := t.TempDir()
	svc := NewWorkspaceService()

	wsDef, err := svc.Init(tempDir, "Test API Project")
	if err != nil {
		t.Fatalf("unexpected error initializing workspace: %v", err)
	}

	if wsDef.Name != "Test API Project" {
		t.Errorf("expected workspace name 'Test API Project', got '%s'", wsDef.Name)
	}

	// Verify info can be read back
	info, err := svc.GetWorkspaceInfo(tempDir)
	if err != nil {
		t.Fatalf("failed to read workspace info: %v", err)
	}
	if info.Name != "Test API Project" {
		t.Errorf("expected workspace info name 'Test API Project', got '%s'", info.Name)
	}

	// Verify tree has the starter example
	tree, err := svc.ScanTree(tempDir)
	if err != nil {
		t.Fatalf("failed to scan tree: %v", err)
	}
	if len(tree) == 0 {
		t.Fatalf("expected non-empty tree, got 0 nodes")
	}

	// Check that the starter example is inside 'example' folder
	exampleFolder := tree[0]
	if !exampleFolder.IsDir || exampleFolder.Name != "example" {
		t.Errorf("expected 'example' folder, got %+v", exampleFolder)
	}
	if len(exampleFolder.Children) == 0 {
		t.Fatalf("expected example folder to have children, got 0")
	}
	reqNode := exampleFolder.Children[0]
	if reqNode.IsDir || reqNode.Method != "GET" {
		t.Errorf("expected GET request node, got %+v", reqNode)
	}
}

func TestWorkspaceService_ReadWriteRequest(t *testing.T) {
	tempDir := t.TempDir()
	svc := NewWorkspaceService()

	reqPath := filepath.Join(tempDir, "auth", "login.pebble.json")
	originalReq := &types.RequestDefinition{
		Name:   "User Login",
		Method: "POST",
		URL:    "https://api.example.com/v1/auth/login",
		Headers: []types.KeyValue{
			{Key: "Content-Type", Value: "application/json", Enabled: true},
		},
		Body: types.BodyDefinition{
			Type: "json",
			Raw:  `{"username":"admin"}`,
		},
		Scripts: types.ScriptDefinition{
			PostResponse: "pb.test('200 ok', () => pb.expect(pb.response.status).to.eql(200));",
		},
		Settings: types.SettingDefinition{
			FollowRedirects: true,
			TimeoutMs:       5000,
		},
	}

	// Save request
	if err := svc.SaveRequest(reqPath, originalReq); err != nil {
		t.Fatalf("failed to save request: %v", err)
	}

	// Read request back
	loadedReq, err := svc.ReadRequest(reqPath)
	if err != nil {
		t.Fatalf("failed to read saved request: %v", err)
	}

	if loadedReq.Name != originalReq.Name {
		t.Errorf("expected request name '%s', got '%s'", originalReq.Name, loadedReq.Name)
	}
	if loadedReq.Method != "POST" {
		t.Errorf("expected method 'POST', got '%s'", loadedReq.Method)
	}
	if len(loadedReq.Headers) != 1 || loadedReq.Headers[0].Key != "Content-Type" {
		t.Errorf("headers mismatch: %+v", loadedReq.Headers)
	}
}

func TestWorkspaceService_FolderAndRename(t *testing.T) {
	tempDir := t.TempDir()
	svc := NewWorkspaceService()

	folderPath := filepath.Join(tempDir, "custom-folder")
	if err := svc.CreateFolder(folderPath); err != nil {
		t.Fatalf("failed to create folder: %v", err)
	}

	reqPath := filepath.Join(folderPath, "test.pebble.json")
	req := &types.RequestDefinition{Name: "Test Req", Method: "GET", URL: "http://localhost:8080"}
	if err := svc.SaveRequest(reqPath, req); err != nil {
		t.Fatalf("failed to save req: %v", err)
	}

	// Rename folder
	newFolderPath := filepath.Join(tempDir, "renamed-folder")
	if err := svc.Rename(folderPath, newFolderPath); err != nil {
		t.Fatalf("failed to rename folder: %v", err)
	}

	// Verify request exists at new path
	newReqPath := filepath.Join(newFolderPath, "test.pebble.json")
	if _, err := svc.ReadRequest(newReqPath); err != nil {
		t.Fatalf("failed to read req at new path: %v", err)
	}

	// Delete folder
	if err := svc.DeletePath(newFolderPath); err != nil {
		t.Fatalf("failed to delete folder: %v", err)
	}

	if _, err := svc.ReadRequest(newReqPath); err == nil {
		t.Errorf("expected error reading deleted request, got nil")
	}
}
