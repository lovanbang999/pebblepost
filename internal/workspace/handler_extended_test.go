package workspace

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"pebblepost/internal/types"
)

func TestHandler_ExtendedEndpoints(t *testing.T) {
	tempDir := t.TempDir()
	wsSvc := NewWorkspaceService()
	envSvc := NewEnvironmentService()
	interpolator := NewInterpolator()

	handler := NewHandler(wsSvc, envSvc, interpolator)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// 1. handleInit: POST /api/workspace/init
	initBody, _ := json.Marshal(map[string]string{
		"path": tempDir,
		"name": "Extended Workspace",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/workspace/init", bytes.NewReader(initBody))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleInit failed: %d: %s", w.Code, w.Body.String())
	}

	// 2. handleInfo: GET /api/workspace/info?path=...
	req = httptest.NewRequest(http.MethodGet, "/api/workspace/info?path="+url.QueryEscape(tempDir), nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleInfo failed: %d: %s", w.Code, w.Body.String())
	}

	// 3. handleCreateFolder: POST /api/workspace/folder
	createdFolderDir := filepath.Join(tempDir, CollectionsDir, "UsersFolder")
	createFolderBody, _ := json.Marshal(map[string]string{
		"workspacePath": tempDir,
		"path":          createdFolderDir,
	})
	req = httptest.NewRequest(http.MethodPost, "/api/workspace/folder", bytes.NewReader(createFolderBody))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleCreateFolder failed: %d: %s", w.Code, w.Body.String())
	}

	// 4. handleFolder (GET) & handleSaveFolder (POST)
	req = httptest.NewRequest(http.MethodGet, "/api/folder?path="+url.QueryEscape(createdFolderDir), nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleFolder GET failed: %d: %s", w.Code, w.Body.String())
	}

	saveFolderBody, _ := json.Marshal(map[string]any{
		"path": createdFolderDir,
		"folder": types.FolderDefinition{
			Name:        "UsersFolder",
			Description: "Updated folder description",
		},
	})
	req = httptest.NewRequest(http.MethodPost, "/api/folder/save", bytes.NewReader(saveFolderBody))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleSaveFolder failed: %d: %s", w.Code, w.Body.String())
	}

	// 5. Create a request inside UsersFolder to test resolve, duplicate, rename, move, delete
	reqFilePath := filepath.Join(createdFolderDir, "get-user.pebble.json")
	saveReqBody, _ := json.Marshal(map[string]any{
		"workspacePath": tempDir,
		"path":          reqFilePath,
		"request": types.RequestDefinition{
			Name:   "GetUser",
			Method: "GET",
			URL:    "https://api.example.com/users/1",
		},
	})
	req = httptest.NewRequest(http.MethodPost, "/api/request/save", bytes.NewReader(saveReqBody))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("save request failed: %d", w.Code)
	}

	// handleResolvedRequest: GET /api/request/resolved?workspacePath=...&path=...
	req = httptest.NewRequest(http.MethodGet, "/api/request/resolved?workspacePath="+url.QueryEscape(tempDir)+"&path="+url.QueryEscape(reqFilePath), nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleResolvedRequest failed: %d: %s", w.Code, w.Body.String())
	}

	// 6. handleDuplicate: POST /api/workspace/duplicate
	dupBody, _ := json.Marshal(map[string]string{
		"workspacePath": tempDir,
		"path":          reqFilePath,
	})
	req = httptest.NewRequest(http.MethodPost, "/api/workspace/duplicate", bytes.NewReader(dupBody))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleDuplicate failed: %d: %s", w.Code, w.Body.String())
	}

	// 7. handleRename: POST /api/workspace/rename
	renamedFilePath := filepath.Join(createdFolderDir, "get-single-user.pebble.json")
	renameBody, _ := json.Marshal(map[string]string{
		"workspacePath": tempDir,
		"oldPath":       reqFilePath,
		"newPath":       renamedFilePath,
	})
	req = httptest.NewRequest(http.MethodPost, "/api/workspace/rename", bytes.NewReader(renameBody))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleRename failed: %d: %s", w.Code, w.Body.String())
	}

	// 8. handleMove: POST /api/workspace/move
	targetDir := filepath.Join(tempDir, CollectionsDir)
	moveBody, _ := json.Marshal(map[string]string{
		"workspacePath": tempDir,
		"sourcePath":    renamedFilePath,
		"targetFolder":  targetDir,
	})
	req = httptest.NewRequest(http.MethodPost, "/api/workspace/move", bytes.NewReader(moveBody))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleMove failed: %d: %s", w.Code, w.Body.String())
	}

	movedFilePath := filepath.Join(targetDir, "get-single-user.pebble.json")

	// 9. handleDeleteRequest: POST /api/request/delete
	delReqBody, _ := json.Marshal(map[string]string{
		"workspacePath": tempDir,
		"path":          movedFilePath,
	})
	req = httptest.NewRequest(http.MethodPost, "/api/request/delete", bytes.NewReader(delReqBody))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleDeleteRequest failed: %d: %s", w.Code, w.Body.String())
	}

	// 10. handleImport: POST /api/import (cURL format)
	importBody, _ := json.Marshal(map[string]any{
		"source":    "curl",
		"content":   `curl -X POST https://httpbin.org/post -H "Content-Type: application/json" -d '{"hello":"world"}'`,
		"save":      false,
		"workspace": tempDir,
	})
	req = httptest.NewRequest(http.MethodPost, "/api/import", bytes.NewReader(importBody))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleImport failed: %d: %s", w.Code, w.Body.String())
	}

	// 11. handleExport: POST /api/impexp/export (postman format)
	exportBody, _ := json.Marshal(map[string]any{
		"format":         "postman",
		"collectionName": "Exported Collection",
		"folderPath":     createdFolderDir,
	})
	req = httptest.NewRequest(http.MethodPost, "/api/impexp/export", bytes.NewReader(exportBody))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleExport failed: %d: %s", w.Code, w.Body.String())
	}
}

func TestWorkspaceService_DirectOperations(t *testing.T) {
	tempDir := t.TempDir()
	wsSvc := NewWorkspaceService()

	_, err := wsSvc.Init(tempDir, "Direct WS")
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	folderPath := filepath.Join(tempDir, CollectionsDir, "DirTest")
	if err := wsSvc.CreateFolder(folderPath); err != nil {
		t.Fatalf("CreateFolder failed: %v", err)
	}

	folder, err := wsSvc.ReadFolder(folderPath)
	if err != nil {
		t.Fatalf("ReadFolder failed: %v", err)
	}
	folder.Description = "New desc"
	if err := wsSvc.SaveFolder(folderPath, folder); err != nil {
		t.Fatalf("SaveFolder failed: %v", err)
	}

	newFolderPath := filepath.Join(tempDir, CollectionsDir, "DirTestRenamed")
	if err := wsSvc.Rename(folderPath, newFolderPath); err != nil {
		t.Fatalf("Rename folder failed: %v", err)
	}

	if err := wsSvc.DeletePath(newFolderPath); err != nil {
		t.Fatalf("DeletePath failed: %v", err)
	}
	if _, err := os.Stat(newFolderPath); !os.IsNotExist(err) {
		t.Fatalf("expected folder to be deleted")
	}
}

func TestHandler_ResponseBody(t *testing.T) {
	wsSvc := NewWorkspaceService()
	envSvc := NewEnvironmentService()
	interpolator := NewInterpolator()
	handler := NewHandler(wsSvc, envSvc, interpolator)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Missing file query param
	req := httptest.NewRequest(http.MethodGet, "/api/response/body", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing file param, got %d", w.Code)
	}

	// Invalid file reference prefix
	req = httptest.NewRequest(http.MethodGet, "/api/response/body?file=/tmp/invalid-name.txt", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid prefix, got %d", w.Code)
	}

	// Create a valid temp file with pebble-body- prefix
	tmpFile, err := os.CreateTemp("", "pebble-body-test-*.txt")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	_, _ = tmpFile.WriteString("hello stream body chunking")
	_ = tmpFile.Close()

	// Valid GET request
	req = httptest.NewRequest(http.MethodGet, "/api/response/body?file="+url.QueryEscape(tmpFile.Name())+"&offset=6&limit=6", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Chunk string `json:"chunk"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Chunk != "stream" {
		t.Fatalf("expected 'stream', got '%s'", resp.Chunk)
	}
}

func TestImportService_ParseCURL(t *testing.T) {
	svc := NewImportService()

	// Invalid input
	if _, err := svc.ParseCURL("wget http://example.com"); err == nil {
		t.Fatal("expected error for non-curl input")
	}

	// Comprehensive cURL command
	curlCmd := `curl -X POST https://api.example.com/v1/items \
		-H "Authorization: Bearer secret-token" \
		-H "Content-Type: application/json" \
		-u "myuser:mypass" \
		-b "session=xyz; theme=dark" \
		--data '{"name":"widget","price":9.99}'`

	req, err := svc.ParseCURL(curlCmd)
	if err != nil {
		t.Fatalf("ParseCURL failed: %v", err)
	}
	if req.Method != "POST" {
		t.Errorf("expected POST, got %s", req.Method)
	}
	if req.URL != "https://api.example.com/v1/items" {
		t.Errorf("expected url https://api.example.com/v1/items, got %s", req.URL)
	}
	if req.Body.Type != "json" {
		t.Errorf("expected json body, got %s", req.Body.Type)
	}
}

func TestInheritance_ResolveRequest(t *testing.T) {
	tempDir := t.TempDir()
	wsSvc := NewWorkspaceService()
	_, _ = wsSvc.Init(tempDir, "Inherit WS")

	folderDir := filepath.Join(tempDir, CollectionsDir, "AuthFolder")
	_ = os.MkdirAll(folderDir, 0755)
	_ = wsSvc.SaveFolder(folderDir, &types.FolderDefinition{
		Name: "AuthFolder",
		Auth: types.AuthDefinition{
			Type:  "bearer",
			Token: "folder-token",
		},
		Headers: []types.KeyValue{
			{Key: "X-Inherited", Value: "FromFolder", Enabled: true},
		},
	})

	resolver := NewInheritanceResolver(wsSvc)
	req := &types.RequestDefinition{
		Name:   "InheritedReq",
		Method: "GET",
		URL:    "https://api.example.com/data",
		Auth:   types.AuthDefinition{Type: "inherit"},
		Headers: []types.KeyValue{
			{Key: "X-Local", Value: "FromReq", Enabled: true},
		},
	}

	reqPath := filepath.Join(folderDir, "req.pebble.json")
	res, pre, post, vars, err := resolver.ResolveRequest(tempDir, reqPath, req, map[string]string{"envKey": "envVal"}, nil)
	if err != nil {
		t.Fatalf("ResolveRequest failed: %v", err)
	}
	if res.Request.Auth.Type != "bearer" || res.Request.Auth.Token != "folder-token" {
		t.Errorf("auth not inherited correctly: %+v", res.Request.Auth)
	}
	if vars["envKey"] != "envVal" {
		t.Errorf("expected envKey in vars")
	}
	_ = pre
	_ = post
}

func TestInterpolator_RequestDetails(t *testing.T) {
	interpolator := NewInterpolator()
	vars := map[string]string{
		"BASE_URL":  "https://api.test.com",
		"TOKEN":     "tok-123",
		"PARAM_VAL": "abc",
		"GRPC_HOST": "localhost:50051",
		"SUBPROTO":  "chat-v1",
	}

	req := &types.RequestDefinition{
		Name:   "TestReq",
		Method: "POST",
		URL:    "{{BASE_URL}}/items?search={{PARAM_VAL}}",
		Auth: types.AuthDefinition{
			Type:  "bearer",
			Token: "{{TOKEN}}",
		},
		Headers: []types.KeyValue{
			{Key: "Authorization", Value: "Bearer {{TOKEN}}", Enabled: true},
		},
		Params: []types.KeyValue{
			{Key: "q", Value: "{{PARAM_VAL}}", Enabled: true},
		},
		Body: types.BodyDefinition{
			Type: "raw",
			Raw:  `{"url":"{{BASE_URL}}"}`,
		},
		Grpc: &types.GrpcDefinition{
			Address: "{{GRPC_HOST}}",
			Service: "ItemService",
			Method:  "GetItem",
		},
		Stream: &types.StreamDefinition{
			Subprotocols: []string{"{{SUBPROTO}}"},
		},
	}

	resolved, err := interpolator.InterpolateRequestWithError(req, vars)
	if err != nil {
		t.Fatalf("InterpolateRequestWithError: %v", err)
	}
	if resolved.URL != "https://api.test.com/items?search=abc" {
		t.Errorf("expected interpolated URL, got %s", resolved.URL)
	}
	if resolved.Auth.Token != "tok-123" {
		t.Errorf("expected interpolated token, got %s", resolved.Auth.Token)
	}
	if resolved.Grpc.Address != "localhost:50051" {
		t.Errorf("expected interpolated grpc address, got %s", resolved.Grpc.Address)
	}

	interpStream := interpolator.InterpolateStream(req.Stream, vars)
	if interpStream == nil || len(interpStream.Subprotocols) == 0 || interpStream.Subprotocols[0] != "chat-v1" {
		t.Errorf("expected interpolated stream subprotocol, got %+v", interpStream)
	}

	interpGrpc := interpolator.InterpolateGrpc(req.Grpc, vars)
	if interpGrpc == nil || interpGrpc.Address != "localhost:50051" {
		t.Errorf("expected interpolated grpc address, got %+v", interpGrpc)
	}
}
