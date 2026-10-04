package workspace

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"pebblepost/internal/types"
)

func TestHandler_APIEndpoints(t *testing.T) {
	tempDir := t.TempDir()
	wsSvc := NewWorkspaceService()
	envSvc := NewEnvironmentService()
	interpolator := NewInterpolator()

	// 1. Initialize workspace via Service
	_, err := wsSvc.Init(tempDir, "Test API")
	if err != nil {
		t.Fatalf("failed to init workspace: %v", err)
	}

	handler := NewHandler(wsSvc, envSvc, interpolator)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// 2. Test GET /api/workspace/scan
	t.Run("GET /api/workspace/scan", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/workspace/scan?path="+url.QueryEscape(tempDir), nil)
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
		}

		var resp struct {
			Workspace    types.WorkspaceDefinition     `json:"workspace"`
			Tree         []*types.TreeNode             `json:"tree"`
			Environments []types.EnvironmentDefinition `json:"environments"`
		}
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.Workspace.Name != "Test API" {
			t.Errorf("expected workspace name 'Test API', got '%s'", resp.Workspace.Name)
		}
		if len(resp.Environments) == 0 {
			t.Errorf("expected at least 1 environment, got 0")
		}
	})

	// 3. Test POST /api/request/save & GET /api/request
	t.Run("Save and Get Request", func(t *testing.T) {
		reqPath := filepath.Join(tempDir, "collections", "custom", "test-api.pebble.json")
		reqData := types.RequestDefinition{
			Name:   "Custom Test API",
			Method: "POST",
			URL:    "https://httpbin.org/post",
		}

		savePayload, _ := json.Marshal(map[string]any{
			"path":    reqPath,
			"request": reqData,
		})
		saveReq := httptest.NewRequest(http.MethodPost, "/api/request/save", bytes.NewReader(savePayload))
		saveRR := httptest.NewRecorder()
		mux.ServeHTTP(saveRR, saveReq)

		if saveRR.Code != http.StatusOK {
			t.Fatalf("save request failed: %d - %s", saveRR.Code, saveRR.Body.String())
		}

		// GET request
		getReq := httptest.NewRequest(http.MethodGet, "/api/request?path="+url.QueryEscape(reqPath), nil)
		getRR := httptest.NewRecorder()
		mux.ServeHTTP(getRR, getReq)

		if getRR.Code != http.StatusOK {
			t.Fatalf("get request failed: %d - %s", getRR.Code, getRR.Body.String())
		}

		var retrieved types.RequestDefinition
		if err := json.NewDecoder(getRR.Body).Decode(&retrieved); err != nil {
			t.Fatalf("failed to decode retrieved request: %v", err)
		}
		if retrieved.Name != "Custom Test API" {
			t.Errorf("expected name 'Custom Test API', got '%s'", retrieved.Name)
		}
	})

	// 4. Test Environment APIs
	t.Run("Save and List Environments", func(t *testing.T) {
		envPayload, _ := json.Marshal(map[string]any{
			"workspacePath": tempDir,
			"environment": types.EnvironmentDefinition{
				Name: "production",
				Variables: []types.KeyValue{
					{Key: "PROD_HOST", Value: "https://api.prod.com", Enabled: true},
				},
			},
			"isSecret": false,
		})

		saveEnvReq := httptest.NewRequest(http.MethodPost, "/api/environments/save", bytes.NewReader(envPayload))
		saveEnvRR := httptest.NewRecorder()
		mux.ServeHTTP(saveEnvRR, saveEnvReq)

		if saveEnvRR.Code != http.StatusOK {
			t.Fatalf("save env failed: %d - %s", saveEnvRR.Code, saveEnvRR.Body.String())
		}

		// List envs
		listEnvReq := httptest.NewRequest(http.MethodGet, "/api/environments?workspacePath="+url.QueryEscape(tempDir), nil)
		listEnvRR := httptest.NewRecorder()
		mux.ServeHTTP(listEnvRR, listEnvReq)

		if listEnvRR.Code != http.StatusOK {
			t.Fatalf("list envs failed: %d - %s", listEnvRR.Code, listEnvRR.Body.String())
		}

		var envs []types.EnvironmentDefinition
		_ = json.NewDecoder(listEnvRR.Body).Decode(&envs)
		if len(envs) < 2 { // 'dev' + 'production'
			t.Errorf("expected at least 2 envs, got %d", len(envs))
		}
	})

	// 5. Test POST /api/request/interpolate
	t.Run("Interpolate Request API", func(t *testing.T) {
		interpPayload, _ := json.Marshal(map[string]any{
			"workspacePath":   tempDir,
			"environmentName": "dev",
			"request": types.RequestDefinition{
				Name:   "Login to {{BASE_URL}}",
				Method: "POST",
				URL:    "{{BASE_URL}}/api/v1/auth",
			},
			"overrides": map[string]string{
				"BASE_URL": "http://override.io",
			},
		})

		interpReq := httptest.NewRequest(http.MethodPost, "/api/request/interpolate", bytes.NewReader(interpPayload))
		interpRR := httptest.NewRecorder()
		mux.ServeHTTP(interpRR, interpReq)

		if interpRR.Code != http.StatusOK {
			t.Fatalf("interpolate API failed: %d - %s", interpRR.Code, interpRR.Body.String())
		}

		var interpResp struct {
			InterpolatedRequest types.RequestDefinition `json:"interpolatedRequest"`
			Variables           map[string]string       `json:"variables"`
		}
		if err := json.NewDecoder(interpRR.Body).Decode(&interpResp); err != nil {
			t.Fatalf("failed to decode interpolate response: %v", err)
		}

		if interpResp.InterpolatedRequest.URL != "http://override.io/api/v1/auth" {
			t.Errorf("expected URL 'http://override.io/api/v1/auth', got '%s'", interpResp.InterpolatedRequest.URL)
		}
	})

	// 6. Test Gitignore status and ensure endpoint
	t.Run("Gitignore Check and Ensure", func(t *testing.T) {
		// Scan initially reports gitignore status
		scanReq := httptest.NewRequest(http.MethodGet, "/api/workspace/scan?path="+url.QueryEscape(tempDir), nil)
		scanRR := httptest.NewRecorder()
		mux.ServeHTTP(scanRR, scanReq)

		var scanResp struct {
			GitignoreStatus string `json:"gitignoreStatus"`
		}
		_ = json.NewDecoder(scanRR.Body).Decode(&scanResp)
		if scanResp.GitignoreStatus == "" {
			t.Errorf("expected gitignoreStatus in scan response, got empty")
		}

		// Ensure gitignore
		ensurePayload, _ := json.Marshal(map[string]any{
			"workspacePath": tempDir,
		})
		ensureReq := httptest.NewRequest(http.MethodPost, "/api/workspace/gitignore/ensure", bytes.NewReader(ensurePayload))
		ensureRR := httptest.NewRecorder()
		mux.ServeHTTP(ensureRR, ensureReq)

		if ensureRR.Code != http.StatusOK {
			t.Fatalf("ensure gitignore failed: %d - %s", ensureRR.Code, ensureRR.Body.String())
		}
	})

	// 7. Test Path Traversal Protection
	t.Run("Path Traversal Protection", func(t *testing.T) {
		// Attempt reading traversal path
		badGetReq := httptest.NewRequest(http.MethodGet, "/api/request?path=../../etc/passwd&workspacePath="+url.QueryEscape(tempDir), nil)
		badGetRR := httptest.NewRecorder()
		mux.ServeHTTP(badGetRR, badGetReq)

		if badGetRR.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for path traversal, got %d", badGetRR.Code)
		}

		// Attempt saving traversal path
		badSavePayload, _ := json.Marshal(map[string]any{
			"workspacePath": tempDir,
			"path":          filepath.Join(tempDir, "..", "outside.pebble.json"),
			"request": types.RequestDefinition{
				Name:   "Evil",
				Method: "GET",
				URL:    "http://evil.com",
			},
		})
		badSaveReq := httptest.NewRequest(http.MethodPost, "/api/request/save", bytes.NewReader(badSavePayload))
		badSaveRR := httptest.NewRecorder()
		mux.ServeHTTP(badSaveRR, badSaveReq)

		if badSaveRR.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for saving outside workspace, got %d", badSaveRR.Code)
		}
	})

	// 8. Test Invalid Environment Name
	t.Run("Invalid Environment Name Rejection", func(t *testing.T) {
		badEnvPayload, _ := json.Marshal(map[string]any{
			"workspacePath": tempDir,
			"environment": types.EnvironmentDefinition{
				Name: "../../evil",
				Variables: []types.KeyValue{
					{Key: "K", Value: "V", Enabled: true},
				},
			},
		})
		badEnvReq := httptest.NewRequest(http.MethodPost, "/api/environments/save", bytes.NewReader(badEnvPayload))
		badEnvRR := httptest.NewRecorder()
		mux.ServeHTTP(badEnvRR, badEnvReq)

		if badEnvRR.Code != http.StatusBadRequest {
			t.Errorf("expected 400 for invalid env name, got %d", badEnvRR.Code)
		}
	})

	// 9. Test Save and Delete Examples
	t.Run("Save and Delete Example with Secret Redaction and Truncation", func(t *testing.T) {
		reqPath := filepath.Join(tempDir, "collections", "custom", "test-examples.pebble.json")
		baseReq := types.RequestDefinition{
			Name:   "Example API",
			Method: "GET",
			URL:    "http://example.com/api",
		}
		if err := wsSvc.SaveRequest(reqPath, &baseReq); err != nil {
			t.Fatalf("failed to save base request: %v", err)
		}

		// Save example with sensitive headers and large body
		largeBody := make([]byte, 600*1024) // 600 KB
		for i := range largeBody {
			largeBody[i] = 'A'
		}

		savePayload, _ := json.Marshal(map[string]any{
			"workspacePath": tempDir,
			"path":          reqPath,
			"example": types.ExampleResponse{
				Name:       "Success 200",
				StatusCode: 200,
				StatusText: "OK",
				Headers: []types.KeyValue{
					{Key: "Authorization", Value: "Bearer secret_jwt_token", Enabled: true},
					{Key: "Content-Type", Value: "application/json", Enabled: true},
					{Key: "Cookie", Value: "session=xyz123", Enabled: true},
				},
				Body: string(largeBody),
			},
		})

		saveReq := httptest.NewRequest(http.MethodPost, "/api/request/example/save", bytes.NewReader(savePayload))
		saveRR := httptest.NewRecorder()
		mux.ServeHTTP(saveRR, saveReq)

		if saveRR.Code != http.StatusOK {
			t.Fatalf("save example failed with status %d: %s", saveRR.Code, saveRR.Body.String())
		}

		var saveResp struct {
			Success  bool                    `json:"success"`
			Example  types.ExampleResponse   `json:"example"`
			Examples []types.ExampleResponse `json:"examples"`
		}
		if err := json.NewDecoder(saveRR.Body).Decode(&saveResp); err != nil {
			t.Fatalf("failed to decode save response: %v", err)
		}

		if len(saveResp.Examples) != 1 {
			t.Fatalf("expected 1 example, got %d", len(saveResp.Examples))
		}

		savedEx := saveResp.Examples[0]
		// Verify sensitive headers are redacted
		for _, hdr := range savedEx.Headers {
			if hdr.Key == "Authorization" && hdr.Value != "[REDACTED]" {
				t.Errorf("expected Authorization header to be [REDACTED], got %s", hdr.Value)
			}
			if hdr.Key == "Cookie" && hdr.Value != "[REDACTED]" {
				t.Errorf("expected Cookie header to be [REDACTED], got %s", hdr.Value)
			}
		}

		// Verify body size is capped (512 KB + truncation note)
		if len(savedEx.Body) > 513*1024 {
			t.Errorf("expected body to be truncated to ~512KB, got %d bytes", len(savedEx.Body))
		}
		if !bytes.Contains([]byte(savedEx.Body), []byte("[Truncated: response body exceeds 512KB limit]")) {
			t.Errorf("expected truncation notice in body")
		}

		// Delete the example
		delPayload, _ := json.Marshal(map[string]any{
			"workspacePath": tempDir,
			"path":          reqPath,
			"exampleId":     savedEx.ID,
		})
		delReq := httptest.NewRequest(http.MethodPost, "/api/request/example/delete", bytes.NewReader(delPayload))
		delRR := httptest.NewRecorder()
		mux.ServeHTTP(delRR, delReq)

		if delRR.Code != http.StatusOK {
			t.Fatalf("delete example failed with status %d: %s", delRR.Code, delRR.Body.String())
		}

		var delResp struct {
			Success  bool                    `json:"success"`
			Examples []types.ExampleResponse `json:"examples"`
		}
		if err := json.NewDecoder(delRR.Body).Decode(&delResp); err != nil {
			t.Fatalf("failed to decode delete response: %v", err)
		}
		if len(delResp.Examples) != 0 {
			t.Errorf("expected 0 examples after deletion, got %d", len(delResp.Examples))
		}
	})
}
