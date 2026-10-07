package mockserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"pebblepost/internal/workspace"
)

func TestMockHandler_Endpoints(t *testing.T) {
	wsDir := createTestWorkspace(t)
	wsSvc := workspace.NewWorkspaceService()
	handler := NewHandler(wsSvc)
	defer handler.Close()

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// 1. Initial Status -> not running
	req := httptest.NewRequest("GET", "/api/mock/status", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/mock/status status = %d; want 200", rr.Code)
	}
	var status ServerStatus
	if err := json.Unmarshal(rr.Body.Bytes(), &status); err != nil {
		t.Fatalf("Failed to parse status JSON: %v", err)
	}
	if status.Running {
		t.Fatal("Expected Running = false initially")
	}

	// 2. Start mock server
	startPayload := map[string]any{
		"path":          wsDir,
		"workspacePath": wsDir,
		"port":          0,
		"host":          "127.0.0.1",
	}
	bodyBytes, _ := json.Marshal(startPayload)
	req = httptest.NewRequest("POST", "/api/mock/start", bytes.NewReader(bodyBytes))
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/mock/start status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &status); err != nil {
		t.Fatalf("Failed to parse start response: %v", err)
	}
	if !status.Running || status.RoutesCount != 3 {
		t.Fatalf("Unexpected started status: %+v", status)
	}

	// 3. Status after start
	req = httptest.NewRequest("GET", "/api/mock/status", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/mock/status status = %d", rr.Code)
	}

	// 4. Update Override
	overridePayload := map[string]any{
		"routeId": "GET:/api/v1/users/profile",
		"override": map[string]any{
			"statusCode": 418,
			"delayMs":    10,
		},
	}
	bodyBytes, _ = json.Marshal(overridePayload)
	req = httptest.NewRequest("POST", "/api/mock/overrides", bytes.NewReader(bodyBytes))
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/mock/overrides status = %d", rr.Code)
	}

	// 5. Query Logs
	req = httptest.NewRequest("GET", "/api/mock/logs", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/mock/logs status = %d", rr.Code)
	}

	// 6. Stop Server
	req = httptest.NewRequest("POST", "/api/mock/stop", nil)
	rr = httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST /api/mock/stop status = %d", rr.Code)
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &status); err != nil {
		t.Fatalf("Failed to parse stop response: %v", err)
	}
	if status.Running {
		t.Fatal("Expected Running = false after stop")
	}
}
