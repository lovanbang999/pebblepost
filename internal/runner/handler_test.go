package runner_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pebblepost/internal/runner"
	"pebblepost/internal/types"
)

func TestHandler_ParseData_CSV(t *testing.T) {
	h := runner.NewHandler(runner.NewRunner())

	payload := map[string]string{
		"filename": "users.csv",
		"content":  "id,name,role\n1,Alice,admin\n2,Bob,editor\n",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/runner/parse-data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Rows      []map[string]any `json:"rows"`
		TotalRows int              `json:"totalRows"`
		Columns   []string         `json:"columns"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if resp.TotalRows != 2 {
		t.Errorf("Expected 2 rows, got %d", resp.TotalRows)
	}
	if len(resp.Columns) != 3 {
		t.Errorf("Expected 3 columns, got %d (%v)", len(resp.Columns), resp.Columns)
	}
}

func TestHandler_ParseData_JSON(t *testing.T) {
	h := runner.NewHandler(runner.NewRunner())

	payload := map[string]string{
		"filename": "items.json",
		"content":  `[{"sku":"ABC","qty":5},{"sku":"XYZ","qty":10}]`,
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/runner/parse-data", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Rows      []map[string]any `json:"rows"`
		TotalRows int              `json:"totalRows"`
		Columns   []string         `json:"columns"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.TotalRows != 2 {
		t.Errorf("Expected 2 rows, got %d", resp.TotalRows)
	}
}

func TestHandler_Stop(t *testing.T) {
	h := runner.NewHandler(runner.NewRunner())

	payload := map[string]string{"runId": "nonexistent_run"}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/runner/stop", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d", w.Code)
	}
}

func TestHandler_Run_SSE(t *testing.T) {
	ws := setupDataWorkspace(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	reqDef := types.RequestDefinition{
		SchemaVersion: 1,
		ID:            "req-ping",
		Name:          "Ping",
		Method:        "GET",
		URL:           server.URL + "/ping",
	}
	d, _ := json.Marshal(reqDef)
	_ = os.WriteFile(filepath.Join(ws, "ping.pebble.json"), d, 0o644)

	h := runner.NewHandler(runner.NewRunner())

	payload := runner.RunPayload{
		TargetPath:    ws,
		WorkspacePath: ws,
		Stream:        true,
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/runner/run", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	w := httptest.NewRecorder()

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	respBody := w.Body.String()
	if !strings.Contains(respBody, "event: start") {
		t.Errorf("Expected SSE output to contain 'event: start', got:\n%s", respBody)
	}
	if !strings.Contains(respBody, "event: request_result") {
		t.Errorf("Expected SSE output to contain 'event: request_result', got:\n%s", respBody)
	}
	if !strings.Contains(respBody, "event: complete") {
		t.Errorf("Expected SSE output to contain 'event: complete', got:\n%s", respBody)
	}

	// Verify SSE line parsing
	scanner := bufio.NewScanner(strings.NewReader(respBody))
	var foundComplete bool
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: complete") {
			foundComplete = true
			break
		}
	}
	if !foundComplete {
		t.Errorf("Could not locate event: complete line")
	}
}
