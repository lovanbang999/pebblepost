package runner_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pebblepost/internal/runner"
	"pebblepost/internal/types"
)

// setupDataWorkspace creates a temporary workspace directory with .pebble folder and sample requests.
func setupDataWorkspace(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "pebble-data-runner-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	pebbleDir := filepath.Join(dir, ".pebble")
	if err := os.MkdirAll(pebbleDir, 0o755); err != nil {
		t.Fatalf("failed to create .pebble dir: %v", err)
	}

	return dir
}

// TestRunner_DataDriven_CSV tests iterating through rows of a CSV file with {{data.var}} and {{var}}.
func TestRunner_DataDriven_CSV(t *testing.T) {
	ws := setupDataWorkspace(t)

	var receivedEmails []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		email := r.URL.Query().Get("email")
		receivedEmails = append(receivedEmails, email)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"email":  email,
		})
	}))
	defer server.Close()

	// 1. Create CSV file with 3 rows
	csvContent := `name,email,role
Alice,alice@example.com,admin
Bob,bob@example.com,member
Charlie,charlie@example.com,guest
`
	csvPath := filepath.Join(ws, "users.csv")
	if err := os.WriteFile(csvPath, []byte(csvContent), 0o644); err != nil {
		t.Fatalf("failed to write CSV: %v", err)
	}

	// 2. Create request using both {{data.email}} and {{role}}
	req := types.RequestDefinition{
		SchemaVersion: 1,
		ID:            "req-user",
		Name:          "Get User",
		Method:        "GET",
		URL:           server.URL + "/user?email={{data.email}}&role={{role}}",
		Scripts: types.ScriptDefinition{
			PostResponse: `
				pb.test("Status is 200", function() {
					pb.expect(pb.response.code).to.equal(200);
				});
				pb.test("Email matches data row", function() {
					var json = pb.response.json();
					pb.expect(json.email).to.equal(pb.data.get("email"));
				});
			`,
		},
	}
	reqData, _ := json.MarshalIndent(req, "", "  ")
	reqPath := filepath.Join(ws, "get_user.pebble.json")
	if err := os.WriteFile(reqPath, reqData, 0o644); err != nil {
		t.Fatalf("failed to write request: %v", err)
	}

	// 3. Execute with Runner
	r := runner.NewRunner()
	summary, err := r.Run(context.Background(), runner.RunOptions{
		TargetPath: ws,
		DataFile:   csvPath,
	})

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if !summary.Success {
		t.Errorf("Expected summary.Success to be true, got false (exit code %d)", summary.ExitCode)
	}
	if summary.TotalIterations != 3 {
		t.Errorf("Expected 3 iterations, got %d", summary.TotalIterations)
	}
	if summary.PassedIterations != 3 {
		t.Errorf("Expected 3 passed iterations, got %d", summary.PassedIterations)
	}
	if summary.PassRate != 100.0 {
		t.Errorf("Expected 100.0%% pass rate, got %.1f%%", summary.PassRate)
	}
	if len(receivedEmails) != 3 {
		t.Fatalf("Expected 3 HTTP requests, got %d", len(receivedEmails))
	}
	expectedEmails := []string{"alice@example.com", "bob@example.com", "charlie@example.com"}
	for i, exp := range expectedEmails {
		if receivedEmails[i] != exp {
			t.Errorf("Iteration %d: expected email %q, got %q", i+1, exp, receivedEmails[i])
		}
	}
}

// TestRunner_DataDriven_IterationsLimit tests that opts.Iterations can truncate the data file rows.
func TestRunner_DataDriven_IterationsLimit(t *testing.T) {
	ws := setupDataWorkspace(t)

	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	csvContent := "id\n1\n2\n3\n4\n5\n"
	csvPath := filepath.Join(ws, "data.csv")
	_ = os.WriteFile(csvPath, []byte(csvContent), 0o644)

	req := types.RequestDefinition{
		SchemaVersion: 1,
		ID:            "req-1",
		Name:          "Ping",
		Method:        "GET",
		URL:           server.URL + "/ping?id={{data.id}}",
	}
	reqData, _ := json.MarshalIndent(req, "", "  ")
	_ = os.WriteFile(filepath.Join(ws, "ping.pebble.json"), reqData, 0o644)

	r := runner.NewRunner()
	// Ask for 2 iterations even though CSV has 5 rows
	summary, err := r.Run(context.Background(), runner.RunOptions{
		TargetPath: ws,
		DataFile:   csvPath,
		Iterations: 2,
	})

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if summary.TotalIterations != 2 {
		t.Errorf("Expected 2 iterations, got %d", summary.TotalIterations)
	}
	if requestCount != 2 {
		t.Errorf("Expected 2 requests to be made, got %d", requestCount)
	}
}

// TestRunner_SetNextRequest_Jump tests jumping past an intermediate request.
func TestRunner_SetNextRequest_Jump(t *testing.T) {
	ws := setupDataWorkspace(t)

	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		step := r.URL.Query().Get("step")
		calls = append(calls, step)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"step":"` + step + `"}`))
	}))
	defer server.Close()

	// Step 1: jumps to "Step 3" using pb.runner.setNextRequest("Step 3")
	r1 := types.RequestDefinition{
		SchemaVersion: 1,
		ID:            "req-step1",
		Name:          "Step 1",
		Method:        "GET",
		URL:           server.URL + "/api?step=1",
		Scripts: types.ScriptDefinition{
			PostResponse: `
				pb.runner.setNextRequest("Step 3");
			`,
		},
	}
	// Step 2: should be SKIPPED
	r2 := types.RequestDefinition{
		SchemaVersion: 1,
		ID:            "req-step2",
		Name:          "Step 2",
		Method:        "GET",
		URL:           server.URL + "/api?step=2",
	}
	// Step 3: executes
	r3 := types.RequestDefinition{
		SchemaVersion: 1,
		ID:            "req-step3",
		Name:          "Step 3",
		Method:        "GET",
		URL:           server.URL + "/api?step=3",
	}

	d1, _ := json.Marshal(r1)
	d2, _ := json.Marshal(r2)
	d3, _ := json.Marshal(r3)

	_ = os.WriteFile(filepath.Join(ws, "01_step1.pebble.json"), d1, 0o644)
	_ = os.WriteFile(filepath.Join(ws, "02_step2.pebble.json"), d2, 0o644)
	_ = os.WriteFile(filepath.Join(ws, "03_step3.pebble.json"), d3, 0o644)

	r := runner.NewRunner()
	summary, err := r.Run(context.Background(), runner.RunOptions{
		TargetPath: ws,
	})

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !summary.Success {
		t.Errorf("Expected success, got failure")
	}

	expectedCalls := []string{"1", "3"}
	if len(calls) != len(expectedCalls) {
		t.Fatalf("Expected calls %v, got %v", expectedCalls, calls)
	}
	for i := range expectedCalls {
		if calls[i] != expectedCalls[i] {
			t.Errorf("Call %d: expected %s, got %s", i, expectedCalls[i], calls[i])
		}
	}
}

// TestRunner_SetNextRequest_Null_EndsIteration tests pb.runner.setNextRequest(null).
func TestRunner_SetNextRequest_Null_EndsIteration(t *testing.T) {
	ws := setupDataWorkspace(t)

	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		step := r.URL.Query().Get("step")
		calls = append(calls, step)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"step":"` + step + `"}`))
	}))
	defer server.Close()

	// Step 1: terminates current iteration early
	r1 := types.RequestDefinition{
		SchemaVersion: 1,
		ID:            "req-step1",
		Name:          "Step 1",
		Method:        "GET",
		URL:           server.URL + "/api?step=1",
		Scripts: types.ScriptDefinition{
			PostResponse: `
				pb.runner.setNextRequest(null);
			`,
		},
	}
	// Step 2: should NEVER run in either iteration
	r2 := types.RequestDefinition{
		SchemaVersion: 1,
		ID:            "req-step2",
		Name:          "Step 2",
		Method:        "GET",
		URL:           server.URL + "/api?step=2",
	}

	d1, _ := json.Marshal(r1)
	d2, _ := json.Marshal(r2)
	_ = os.WriteFile(filepath.Join(ws, "01_step1.pebble.json"), d1, 0o644)
	_ = os.WriteFile(filepath.Join(ws, "02_step2.pebble.json"), d2, 0o644)

	r := runner.NewRunner()
	summary, err := r.Run(context.Background(), runner.RunOptions{
		TargetPath: ws,
		Iterations: 2,
	})

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if summary.TotalIterations != 2 {
		t.Errorf("Expected 2 iterations, got %d", summary.TotalIterations)
	}
	// Step 1 runs in iter 1, then stops. Step 1 runs in iter 2, then stops. Total = 2 calls to step 1.
	if len(calls) != 2 {
		t.Fatalf("Expected 2 calls, got %d (%v)", len(calls), calls)
	}
	for _, c := range calls {
		if c != "1" {
			t.Errorf("Expected call to step 1, got step %s", c)
		}
	}
}

// TestRunner_StopAll tests pb.runner.stop() completely ending the run across iterations.
func TestRunner_StopAll(t *testing.T) {
	ws := setupDataWorkspace(t)

	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	r1 := types.RequestDefinition{
		SchemaVersion: 1,
		ID:            "req-stop",
		Name:          "Stop Request",
		Method:        "GET",
		URL:           server.URL + "/test",
		Scripts: types.ScriptDefinition{
			PostResponse: `
				pb.runner.stop();
			`,
		},
	}
	d1, _ := json.Marshal(r1)
	_ = os.WriteFile(filepath.Join(ws, "01_stop.pebble.json"), d1, 0o644)

	r := runner.NewRunner()
	summary, err := r.Run(context.Background(), runner.RunOptions{
		TargetPath: ws,
		Iterations: 5,
	})

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	// Should stop after 1st execution of 1st iteration
	if calls != 1 {
		t.Errorf("Expected 1 call, got %d", calls)
	}
	if summary.TotalIterations != 1 {
		t.Errorf("Expected TotalIterations to be 1, got %d", summary.TotalIterations)
	}
}

// TestRunner_LoopProtection tests aborting when loop execution exceeds maxExecutionsPerIteration.
func TestRunner_LoopProtection(t *testing.T) {
	ws := setupDataWorkspace(t)

	var count int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	// Infinite loop: jumps to itself
	r1 := types.RequestDefinition{
		SchemaVersion: 1,
		ID:            "req-loop",
		Name:          "Infinite Loop",
		Method:        "GET",
		URL:           server.URL + "/loop",
		Scripts: types.ScriptDefinition{
			PostResponse: `
				pb.runner.setNextRequest("Infinite Loop");
			`,
		},
	}
	d1, _ := json.Marshal(r1)
	_ = os.WriteFile(filepath.Join(ws, "loop.pebble.json"), d1, 0o644)

	r := runner.NewRunner()
	// Set custom cap of 15 for faster test execution
	summary, err := r.Run(context.Background(), runner.RunOptions{
		TargetPath:                ws,
		MaxExecutionsPerIteration: 15,
	})

	if err != nil {
		t.Fatalf("Run failed with unexpected error: %v", err)
	}

	if count != 15 {
		t.Errorf("Expected count to be capped at 15, got %d", count)
	}
	if summary.Success {
		t.Errorf("Expected run to fail due to loop detection")
	}
	if summary.ExitCode != runner.ExitConfigError {
		t.Errorf("Expected ExitConfigError (%d), got %d", runner.ExitConfigError, summary.ExitCode)
	}
}

// TestRunner_LatencyStats tests computation of AvgDurationMs and P95DurationMs.
func TestRunner_LatencyStats(t *testing.T) {
	durations := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		100 * time.Millisecond,
	}

	p95 := runner.CalculateP95(durations)
	if p95 < 90.0 {
		t.Errorf("Expected p95 to be close to 100ms, got %.1f", p95)
	}

	emptyP95 := runner.CalculateP95(nil)
	if emptyP95 != 0 {
		t.Errorf("Expected empty p95 to be 0, got %.1f", emptyP95)
	}
}
