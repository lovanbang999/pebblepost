package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"pebblepost/internal/httpclient"
	"pebblepost/internal/scripting"
	"pebblepost/internal/types"
	"pebblepost/internal/workspace"
)

func TestDiscoverRequests(t *testing.T) {
	tempDir := t.TempDir()

	// Create test folder structure
	collDir := filepath.Join(tempDir, "collections")
	subDir := filepath.Join(collDir, "auth")
	_ = os.MkdirAll(subDir, 0755)

	file1 := filepath.Join(collDir, "b-request.pebble.json")
	file2 := filepath.Join(subDir, "a-login.pebble.json")
	fileOther := filepath.Join(subDir, "ignore.txt")

	_ = os.WriteFile(file1, []byte("{}"), 0644)
	_ = os.WriteFile(file2, []byte("{}"), 0644)
	_ = os.WriteFile(fileOther, []byte("ignore"), 0644)

	r := NewRunner()

	// Test 1: Discover from directory
	files, err := r.discoverRequests(collDir)
	if err != nil {
		t.Fatalf("discoverRequests error: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	// Verify sorting (a-login before b-request)
	if files[0] != file2 || files[1] != file1 {
		t.Errorf("files not sorted alphabetically: %v", files)
	}

	// Test 2: Discover single file
	singleFiles, err := r.discoverRequests(file2)
	if err != nil {
		t.Fatalf("discover single file error: %v", err)
	}
	if len(singleFiles) != 1 || singleFiles[0] != file2 {
		t.Errorf("expected [%s], got %v", file2, singleFiles)
	}

	// Test 3: Non-existent path
	_, err = r.discoverRequests(filepath.Join(tempDir, "does-not-exist"))
	if err == nil {
		t.Error("expected error for non-existent path")
	}
}

func TestRunner_ChainedExecutionAndAssertions(t *testing.T) {
	// Mock HTTP Server
	var receivedAuthHeader string
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/login":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"token":"secret-jwt-token-123"}`)
		case "/api/profile":
			receivedAuthHeader = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"id":42,"username":"admin"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockServer.Close()

	tempDir := t.TempDir()
	wsSvc := workspace.NewWorkspaceService()
	_, err := wsSvc.Init(tempDir, "Test Workspace")
	if err != nil {
		t.Fatalf("Init workspace error: %v", err)
	}

	collDir := filepath.Join(tempDir, "collections", "auth")
	_ = os.MkdirAll(collDir, 0755)

	// Request 1: Login and extract AUTH_TOKEN
	req1 := types.RequestDefinition{
		Version: "1.0",
		Name:    "1-Login",
		Method:  "POST",
		URL:     mockServer.URL + "/api/login",
		Headers: []types.KeyValue{{Key: "Accept", Value: "application/json", Enabled: true}},
		Scripts: types.ScriptDefinition{
			PostResponse: `
				pb.test("Login status 200", () => {
					pb.expect(pb.response.status).to.eql(200);
				});
				const data = pb.response.json();
				pb.environment.set("AUTH_TOKEN", data.token);
			`,
		},
	}
	_ = wsSvc.SaveRequest(filepath.Join(collDir, "01-login.pebble.json"), &req1)

	// Request 2: Profile using the dynamically chained {{AUTH_TOKEN}}
	req2 := types.RequestDefinition{
		Version: "1.0",
		Name:    "2-Profile",
		Method:  "GET",
		URL:     mockServer.URL + "/api/profile",
		Headers: []types.KeyValue{
			{Key: "Authorization", Value: "Bearer {{AUTH_TOKEN}}", Enabled: true},
		},
		Scripts: types.ScriptDefinition{
			PostResponse: `
				pb.test("Profile status 200", () => {
					pb.expect(pb.response.status).to.eql(200);
				});
				const user = pb.response.json();
				pb.test("Correct username", () => {
					pb.expect(user.username).to.eql("admin");
				});
			`,
		},
	}
	_ = wsSvc.SaveRequest(filepath.Join(collDir, "02-profile.pebble.json"), &req2)

	var out bytes.Buffer
	r := NewCustomRunner(
		wsSvc,
		workspace.NewEnvironmentService(),
		workspace.NewInterpolator(),
		httpclient.NewClient(),
		scripting.NewEngine(),
	)

	summary, err := r.Run(context.Background(), RunOptions{
		TargetPath:   collDir,
		Bail:         false,
		ReportFormat: "terminal",
		Writer:       &out,
	})

	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if !summary.Success {
		t.Errorf("expected run success, got failed: %s", out.String())
	}
	if summary.TotalRequests != 2 {
		t.Errorf("expected 2 requests, got %d", summary.TotalRequests)
	}
	if summary.PassedRequests != 2 {
		t.Errorf("expected 2 passed requests, got %d", summary.PassedRequests)
	}
	if summary.TotalTests != 3 {
		t.Errorf("expected 3 total tests, got %d", summary.TotalTests)
	}
	if summary.PassedTests != 3 {
		t.Errorf("expected 3 passed tests, got %d", summary.PassedTests)
	}

	// Verify chained variable interpolation in Request 2
	expectedAuth := "Bearer secret-jwt-token-123"
	if receivedAuthHeader != expectedAuth {
		t.Errorf("expected Authorization header '%s', got '%s'", expectedAuth, receivedAuthHeader)
	}
}

func TestRunner_BailStopsImmediately(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"status":"ok"}`)
	}))
	defer mockServer.Close()

	tempDir := t.TempDir()
	wsSvc := workspace.NewWorkspaceService()
	_, _ = wsSvc.Init(tempDir, "Test Workspace")

	collDir := filepath.Join(tempDir, "collections")

	// Request 1: Failing assertion
	req1 := types.RequestDefinition{
		Version: "1.0",
		Name:    "Req 1 - Fail",
		Method:  "GET",
		URL:     mockServer.URL,
		Scripts: types.ScriptDefinition{
			PostResponse: `
				pb.test("Intentionally Failing Assertion", () => {
					pb.expect(pb.response.status).to.eql(201);
				});
			`,
		},
	}
	_ = wsSvc.SaveRequest(filepath.Join(collDir, "01-fail.pebble.json"), &req1)

	// Request 2 & 3: Should never be reached when --bail is active
	req2 := types.RequestDefinition{
		Version: "1.0",
		Name:    "Req 2 - Should Skip",
		Method:  "GET",
		URL:     mockServer.URL,
	}
	_ = wsSvc.SaveRequest(filepath.Join(collDir, "02-skip.pebble.json"), &req2)

	var out bytes.Buffer
	r := NewRunner()

	summary, err := r.Run(context.Background(), RunOptions{
		TargetPath:   collDir,
		Bail:         true,
		ReportFormat: "terminal",
		Writer:       &out,
	})

	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	if summary.Success {
		t.Error("expected run to fail")
	}
	if !summary.Bailed {
		t.Error("expected summary.Bailed to be true")
	}
	if summary.TotalRequests != 1 {
		t.Errorf("expected exactly 1 request executed before bail, got %d", summary.TotalRequests)
	}
	if summary.FailedRequests != 1 {
		t.Errorf("expected 1 failed request, got %d", summary.FailedRequests)
	}
}

func TestRunner_JSONReportFormat(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"data":"ok"}`)
	}))
	defer mockServer.Close()

	tempDir := t.TempDir()
	wsSvc := workspace.NewWorkspaceService()
	_, _ = wsSvc.Init(tempDir, "Test Workspace")

	req := types.RequestDefinition{
		Version: "1.0",
		Name:    "Health",
		Method:  "GET",
		URL:     mockServer.URL,
		Scripts: types.ScriptDefinition{
			PostResponse: `pb.test("status ok", () => { pb.expect(pb.response.status).to.eql(200); });`,
		},
	}
	_ = wsSvc.SaveRequest(filepath.Join(tempDir, "collections", "health.pebble.json"), &req)

	var out bytes.Buffer
	r := NewRunner()

	summary, err := r.Run(context.Background(), RunOptions{
		TargetPath:   filepath.Join(tempDir, "collections", "health.pebble.json"),
		ReportFormat: "json",
		Writer:       &out,
	})

	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !summary.Success {
		t.Error("expected summary to be successful")
	}

	var parsedSummary RunSummary
	if err := json.Unmarshal(out.Bytes(), &parsedSummary); err != nil {
		t.Fatalf("failed to parse JSON report output: %v\nOutput was: %s", err, out.String())
	}

	if !parsedSummary.Success {
		t.Error("expected parsed JSON summary to be successful")
	}
	if parsedSummary.TotalRequests != 1 || parsedSummary.PassedRequests != 1 {
		t.Errorf("unexpected counts in JSON report: %+v", parsedSummary)
	}
}

func TestRunner_FolderInheritance(t *testing.T) {
	var receivedAuth string
	var receivedHeaders http.Header
	var receivedPath string

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		receivedHeaders = r.Header.Clone()
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"status":"authenticated"}`)
	}))
	defer mockServer.Close()

	tempDir := t.TempDir()
	wsSvc := workspace.NewWorkspaceService()
	_, _ = wsSvc.Init(tempDir, "Inheritance Workspace")

	// 1. Root collection folder with headers & variables
	collDir := filepath.Join(tempDir, "collections")
	rootFolder := types.FolderDefinition{
		SchemaVersion: 1,
		Name:          "Root Collection",
		Headers: []types.KeyValue{
			{Key: "X-Root-Trace", Value: "trace-999", Enabled: true},
		},
		Variables: []types.KeyValue{
			{Key: "API_VERSION", Value: "v2", Enabled: true},
		},
	}
	_ = wsSvc.SaveFolder(collDir, &rootFolder)

	// 2. Child folder '01-auth' with bearer auth, headers, variables, pre & post scripts
	authDir := filepath.Join(collDir, "01-auth")
	_ = os.MkdirAll(authDir, 0755)
	childFolder := types.FolderDefinition{
		SchemaVersion: 1,
		Name:          "01-auth",
		Auth: types.AuthDefinition{
			Type:  "bearer",
			Token: "token-from-folder",
		},
		Headers: []types.KeyValue{
			{Key: "X-Module", Value: "auth-module", Enabled: true},
		},
		Scripts: types.ScriptDefinition{
			PreRequest:   "pb.request.headers.set('X-Folder-Hook', 'hook-active');",
			PostResponse: "pb.test('folder test passed', () => { pb.expect(pb.response.status).to.eql(200); });",
		},
	}
	_ = wsSvc.SaveFolder(authDir, &childFolder)

	// 3. Request in '01-auth' with auth type "inherit"
	req := types.RequestDefinition{
		SchemaVersion: 1,
		Name:          "Login Test",
		Method:        "GET",
		URL:           mockServer.URL + "/api/{{API_VERSION}}",
		Auth: types.AuthDefinition{
			Type: "inherit",
		},
		Headers: []types.KeyValue{
			{Key: "X-Req-Custom", Value: "custom-val", Enabled: true},
		},
		Scripts: types.ScriptDefinition{
			PostResponse: "pb.test('req test passed', () => { pb.expect(pb.response.status).to.eql(200); });",
		},
	}
	reqPath := filepath.Join(authDir, "login.pebble.json")
	_ = wsSvc.SaveRequest(reqPath, &req)

	var out bytes.Buffer
	r := NewRunner()

	summary, err := r.Run(context.Background(), RunOptions{
		TargetPath: reqPath,
		Writer:     &out,
	})

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !summary.Success {
		t.Fatalf("expected test suite to succeed, results: %+v, out:\n%s", summary, out.String())
	}

	// Verify Auth was inherited
	if receivedAuth != "Bearer token-from-folder" {
		t.Errorf("expected Authorization 'Bearer token-from-folder', got %q", receivedAuth)
	}

	// Verify headers from root, child folder, request, and pre-request script
	if receivedHeaders.Get("X-Root-Trace") != "trace-999" {
		t.Errorf("expected X-Root-Trace='trace-999', got %q", receivedHeaders.Get("X-Root-Trace"))
	}
	if receivedHeaders.Get("X-Module") != "auth-module" {
		t.Errorf("expected X-Module='auth-module', got %q", receivedHeaders.Get("X-Module"))
	}
	if receivedHeaders.Get("X-Req-Custom") != "custom-val" {
		t.Errorf("expected X-Req-Custom='custom-val', got %q", receivedHeaders.Get("X-Req-Custom"))
	}
	if receivedHeaders.Get("X-Folder-Hook") != "hook-active" {
		t.Errorf("expected X-Folder-Hook='hook-active' from pre-request script, got %q", receivedHeaders.Get("X-Folder-Hook"))
	}

	// Verify variable interpolation from folder variable
	if receivedPath != "/api/v2" {
		t.Errorf("expected URL path '/api/v2', got %q", receivedPath)
	}

	// Verify test assertions: 1 from request + 1 from folder = 2 tests
	if summary.PassedTests != 2 {
		t.Errorf("expected 2 passed tests (1 req + 1 folder), got %d", summary.PassedTests)
	}
}

func TestRunner_UntrustedScriptsSkipped(t *testing.T) {
	var receivedHeaders http.Header
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok": true}`))
	}))
	defer ts.Close()

	workspaceDir := t.TempDir()
	reqDir := filepath.Join(workspaceDir, "test-col")
	_ = os.MkdirAll(reqDir, 0755)

	reqFile := filepath.Join(reqDir, "get-user.pebble.json")
	reqObj := types.RequestDefinition{
		ID:     "req-trust-1",
		Name:   "Get User",
		Method: "GET",
		URL:    ts.URL + "/test",
		Scripts: types.ScriptDefinition{
			PreRequest: `pb.request.headers.set("X-Script-Ran", "true");`,
		},
	}
	reqData, _ := json.Marshal(reqObj)
	_ = os.WriteFile(reqFile, reqData, 0644)

	r := NewRunner()

	// Case 1: Untrusted (TrustScripts: &false)
	untrusted := false
	summary1, err := r.Run(context.Background(), RunOptions{
		TargetPath:   workspaceDir,
		TrustScripts: &untrusted,
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if !summary1.Success {
		t.Fatalf("expected run to succeed")
	}
	if receivedHeaders.Get("X-Script-Ran") != "" {
		t.Fatalf("expected pre-request script NOT to run when untrusted, got %s", receivedHeaders.Get("X-Script-Ran"))
	}

	// Case 2: Trusted (TrustScripts: &true)
	trusted := true
	summary2, err := r.Run(context.Background(), RunOptions{
		TargetPath:   workspaceDir,
		TrustScripts: &trusted,
	})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if !summary2.Success {
		t.Fatalf("expected run to succeed")
	}
	if receivedHeaders.Get("X-Script-Ran") != "true" {
		t.Fatalf("expected pre-request script to run when trusted, got %s", receivedHeaders.Get("X-Script-Ran"))
	}
}
