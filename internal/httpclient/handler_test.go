package httpclient

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"pebblepost/internal/scripting"
	"pebblepost/internal/types"
	"pebblepost/internal/workspace"
)

func TestHandler_ExecuteEndpoint(t *testing.T) {
	// Create mock target API server
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret-jwt-123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if r.Header.Get("X-Dynamic-Header") != "DynamicVal" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "success",
			"query":  r.URL.Query().Get("q"),
			"code":   200,
		})
	}))
	defer targetServer.Close()

	// Setup workspace and environments
	tempDir := t.TempDir()
	wsSvc := workspace.NewWorkspaceService()
	envSvc := workspace.NewEnvironmentService()
	in := workspace.NewInterpolator()
	scriptEngine := scripting.NewEngine()

	// Save environment
	envData := types.EnvironmentDefinition{
		Name: "production",
		Variables: []types.KeyValue{
			{Key: "BASE_URL", Value: targetServer.URL, Enabled: true},
			{Key: "JWT_TOKEN", Value: "secret-jwt-123", Enabled: true},
		},
	}
	_ = envSvc.SaveEnvironment(tempDir, envData, false)

	client := NewClient()
	handler := NewHandler(client, wsSvc, envSvc, in, scriptEngine)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Prepare payload for /api/request/execute with Pre-request script and Test script
	execPayload := ExecutePayload{
		WorkspacePath:   tempDir,
		EnvironmentName: "production",
		Request: &types.RequestDefinition{
			Method: "GET",
			URL:    "{{BASE_URL}}/search",
			Params: []types.KeyValue{
				{Key: "q", Value: "golang", Enabled: true},
			},
			Auth: types.AuthDefinition{
				Type:  "bearer",
				Token: "{{JWT_TOKEN}}",
			},
			Scripts: types.ScriptDefinition{
				PreRequest: `
					pb.request.headers.add("X-Dynamic-Header", "DynamicVal");
					console.log("Pre-request added dynamic header");
				`,
				PostResponse: `
					pb.test("Response status is 200", function() {
						pb.expect(pb.response.status).to.equal(200);
					});
					pb.test("Response is success", function() {
						const json = pb.response.json();
						pb.expect(json.status).to.equal("success");
						pb.environment.set("SAVED_STATUS", json.status);
					});
				`,
			},
		},
	}

	payloadBytes, _ := json.Marshal(execPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/request/execute", bytes.NewReader(payloadBytes))
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var result types.ExecutionResult
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.StatusCode != http.StatusOK {
		t.Errorf("expected executed status 200, got %d", result.StatusCode)
	}
	if len(result.Tests) != 2 {
		t.Fatalf("expected 2 tests passed, got %d", len(result.Tests))
	}
	if !result.Tests[0].Passed || !result.Tests[1].Passed {
		t.Errorf("expected all tests to pass: %v", result.Tests)
	}
	if result.ExtractedEnvVars["SAVED_STATUS"] != "success" {
		t.Errorf("expected SAVED_STATUS env var to be 'success', got '%s'", result.ExtractedEnvVars["SAVED_STATUS"])
	}
}

func TestHandler_ExecuteEndpoint_InvalidPayload(t *testing.T) {
	client := NewClient()
	handler := NewHandler(client, nil, nil, nil, nil)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// 1. Invalid method
	req := httptest.NewRequest(http.MethodGet, "/api/request/execute", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 Method Not Allowed, got %d", w.Code)
	}

	// 2. Missing request body
	req = httptest.NewRequest(http.MethodPost, "/api/request/execute", bytes.NewReader([]byte("{}")))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request, got %d", w.Code)
	}
}
