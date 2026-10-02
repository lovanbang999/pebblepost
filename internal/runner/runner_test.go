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
