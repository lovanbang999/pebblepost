package httpclient

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestHandler_CookiesEndpoint(t *testing.T) {
	tempDir := t.TempDir()
	client := NewClient()
	client.SetWorkspace(tempDir)
	handler := NewHandler(client, nil, nil, nil, nil)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// 1. POST cookie
	cookiePayload := map[string]any{
		"workspacePath": tempDir,
		"cookie": types.CookieItem{
			Name:   "auth_token",
			Value:  "val_12345",
			Domain: "example.com",
			Path:   "/",
		},
	}
	body, _ := json.Marshal(cookiePayload)
	req := httptest.NewRequest(http.MethodPost, "/api/cookies", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from POST /api/cookies, got %d: %s", w.Code, w.Body.String())
	}

	// 2. GET cookies
	req = httptest.NewRequest(http.MethodGet, "/api/cookies?workspacePath="+tempDir, nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from GET /api/cookies, got %d", w.Code)
	}

	var grouped map[string][]types.CookieItem
	if err := json.NewDecoder(w.Body).Decode(&grouped); err != nil {
		t.Fatalf("failed to decode cookies: %v", err)
	}

	if len(grouped["example.com"]) == 0 {
		t.Errorf("expected cookies for example.com")
	} else if grouped["example.com"][0].Value != "val_12345" {
		t.Errorf("expected cookie value 'val_12345', got '%s'", grouped["example.com"][0].Value)
	}

	// 3. DELETE cookie
	req = httptest.NewRequest(http.MethodDelete, "/api/cookies?workspacePath="+tempDir+"&domain=example.com&name=auth_token", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from DELETE /api/cookies, got %d", w.Code)
	}

	// Verify deleted
	req = httptest.NewRequest(http.MethodGet, "/api/cookies?workspacePath="+tempDir, nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	var groupedAfter map[string][]types.CookieItem
	_ = json.NewDecoder(w.Body).Decode(&groupedAfter)
	if len(groupedAfter["example.com"]) != 0 {
		t.Errorf("expected empty cookies after deletion, got %v", groupedAfter["example.com"])
	}
}

func TestHandler_SecretMaskingInLogs(t *testing.T) {
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer targetServer.Close()

	client := NewClient()
	scriptEngine := scripting.NewEngine()
	handler := NewHandler(client, nil, nil, workspace.NewInterpolator(), scriptEngine)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Pre-request script logs secret
	secretPass := "SuperSecretPassword123"
	payload := ExecutePayload{
		Request: &types.RequestDefinition{
			Method: "GET",
			URL:    targetServer.URL,
			Auth: types.AuthDefinition{
				Type:     "basic",
				Username: "user",
				Password: secretPass,
			},
			Scripts: types.ScriptDefinition{
				PreRequest: `pb.console.log("Connecting with password: " + pb.request.auth.password);`,
			},
		},
	}

	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/request/execute", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var result types.ExecutionResult
	_ = json.NewDecoder(w.Body).Decode(&result)

	for _, log := range result.Logs {
		if strings.Contains(log, secretPass) {
			t.Errorf("secret password leaked in logs: %q", log)
		}
	}
}

