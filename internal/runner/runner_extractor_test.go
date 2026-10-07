package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"pebblepost/internal/types"
)

func TestRunner_ExtractorChaining(t *testing.T) {
	// Fake API server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token":  "jwt_secret_token_123",
				"userId": 55,
			})
		case "/user/55":
			auth := r.Header.Get("Authorization")
			if auth != "Bearer jwt_secret_token_123" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "ok",
				"name":   "Test User",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// Temporary workspace
	tmpDir := t.TempDir()

	// Request 1: Login (extracts token and userId)
	req1 := &types.RequestDefinition{
		SchemaVersion: 2,
		Name:          "01-login",
		Method:        "POST",
		URL:           server.URL + "/login",
		Order:         1,
		Extractors: []types.ExtractorDefinition{
			{
				ID:      "ext-token",
				Name:    "TOKEN",
				Path:    "$.token",
				Scope:   "runtime",
				Enabled: true,
			},
			{
				ID:      "ext-uid",
				Name:    "USER_ID",
				Path:    "$.userId",
				Scope:   "runtime",
				Enabled: true,
			},
			{
				ID:      "ext-missing",
				Name:    "MISSING_VAR",
				Path:    "$.missing.field",
				Scope:   "runtime",
				Enabled: true,
			},
		},
	}
	req1Data, _ := json.Marshal(req1)
	_ = os.WriteFile(filepath.Join(tmpDir, "01-login.pebble.json"), req1Data, 0644)

	// Request 2: Get User Profile (uses {{TOKEN}} and {{USER_ID}})
	req2 := &types.RequestDefinition{
		SchemaVersion: 2,
		Name:          "02-profile",
		Method:        "GET",
		URL:           server.URL + "/user/{{USER_ID}}",
		Order:         2,
		Headers: []types.KeyValue{
			{Key: "Authorization", Value: "Bearer {{TOKEN}}", Enabled: true},
		},
		Scripts: types.ScriptDefinition{
			PostResponse: `
pb.test("profile retrieved successfully", function() {
    pb.expect(pb.response.status).to.eql(200);
});
`,
		},
	}
	req2Data, _ := json.Marshal(req2)
	_ = os.WriteFile(filepath.Join(tmpDir, "02-profile.pebble.json"), req2Data, 0644)

	runner := NewRunner()

	trustTrue := true
	summary, err := runner.Run(context.Background(), RunOptions{
		TargetPath:   tmpDir,
		TrustScripts: &trustTrue,
	})
	if err != nil {
		t.Fatalf("Runner execution failed: %v", err)
	}

	if summary.TotalRequests != 2 {
		t.Errorf("expected 2 requests, got %d", summary.TotalRequests)
	}
	if summary.FailedRequests != 0 {
		t.Errorf("expected 0 failed requests, got %d", summary.FailedRequests)
	}
	if summary.PassedTests != 1 {
		t.Errorf("expected 1 passed test assertion in chained request, got %d", summary.PassedTests)
	}

	// Verify missing path warning was logged without failing the run
	res1 := summary.Results[0].Result
	hasMissingWarning := false
	for _, l := range res1.Logs {
		if l != "" && (len(l) > 6 && l[:6] == "[WARN]") {
			hasMissingWarning = true
			break
		}
	}
	if !hasMissingWarning {
		t.Errorf("expected missing path warning in request logs")
	}
}

func TestRunner_DataDriven_RuntimeVariableResetPerIteration(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"iterationCall": callCount,
			"sessionToken":  "token_for_row",
		})
	}))
	defer server.Close()

	tmpDir := t.TempDir()

	req := &types.RequestDefinition{
		SchemaVersion: 2,
		Name:          "echo",
		Method:        "GET",
		URL:           server.URL + "/test",
		Extractors: []types.ExtractorDefinition{
			{
				ID:      "ext-token",
				Name:    "SESSION_TOKEN",
				Path:    "$.sessionToken",
				Scope:   "runtime",
				Enabled: true,
			},
		},
	}
	reqData, _ := json.Marshal(req)
	_ = os.WriteFile(filepath.Join(tmpDir, "echo.pebble.json"), reqData, 0644)

	runner := NewRunner()

	summary, err := runner.Run(context.Background(), RunOptions{
		TargetPath: tmpDir,
		DataRows: []map[string]any{
			{"user": "alice"},
			{"user": "bob"},
		},
	})
	if err != nil {
		t.Fatalf("Runner failed: %v", err)
	}

	if summary.TotalIterations != 2 {
		t.Errorf("expected 2 iterations, got %d", summary.TotalIterations)
	}
	if summary.PassedRequests != 2 {
		t.Errorf("expected 2 passed requests, got %d", summary.PassedRequests)
	}
}
