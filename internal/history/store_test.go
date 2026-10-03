package history

import (
	"os"
	"testing"
	"time"

	"pebblepost/internal/types"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func makeResult(statusCode int, body string, durationMs float64) *types.ExecutionResult {
	return &types.ExecutionResult{
		StatusCode: statusCode,
		Body:       body,
		Size:       int64(len(body)),
		Timing:     types.TimingMetrics{TotalDurationMs: durationMs},
		ExecutedAt: time.Now(),
	}
}

// ─── TestHistory_WriteAndRead ───────────────────────────────────────────────

func TestHistory_WriteAndRead(t *testing.T) {
	s := newTestStore(t)

	err := s.Record(RecordInput{
		WorkspacePath: "/ws",
		RequestName:   "Get Users",
		Method:        "GET",
		URL:           "https://api.example.com/users",
		Result:        makeResult(200, `{"users":[]}`, 42),
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	entries, total, err := s.List(ListFilter{WorkspacePath: "/ws"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected total=1, got %d", total)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].StatusCode != 200 {
		t.Errorf("expected status 200, got %d", entries[0].StatusCode)
	}
	if entries[0].RequestName != "Get Users" {
		t.Errorf("expected name 'Get Users', got %q", entries[0].RequestName)
	}

	// Full detail
	entry, err := s.Get(entries[0].ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if entry.ResponseBody != `{"users":[]}` {
		t.Errorf("unexpected body: %q", entry.ResponseBody)
	}
}

// ─── TestHistory_SecretMasking ─────────────────────────────────────────────

func TestHistory_SecretMasking(t *testing.T) {
	s := newTestStore(t)

	result := makeResult(200, `{"token":"super-secret-123"}`, 10)

	err := s.Record(RecordInput{
		WorkspacePath: "/ws",
		RequestName:   "Login",
		Method:        "POST",
		URL:           "https://api.example.com/login",
		Result:        result,
		SecretsToMask: []string{"super-secret-123"},
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	entries, _, _ := s.List(ListFilter{WorkspacePath: "/ws"})
	entry, err := s.Get(entries[0].ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if contains(entry.ResponseBody, "super-secret-123") {
		t.Errorf("secret value leaked into stored response body: %q", entry.ResponseBody)
	}
}

// ─── TestHistory_ResponseBodyTruncation ────────────────────────────────────

func TestHistory_ResponseBodyTruncation(t *testing.T) {
	s := newTestStore(t)

	bigBody := string(make([]byte, 200*1024)) // 200 KB of null bytes
	result := makeResult(200, bigBody, 5)

	err := s.Record(RecordInput{
		WorkspacePath: "/ws",
		RequestName:   "BigResponse",
		Method:        "GET",
		URL:           "https://api.example.com/big",
		Result:        result,
		MaxBodyBytes:  100 * 1024,
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	entries, _, _ := s.List(ListFilter{WorkspacePath: "/ws"})
	entry, _ := s.Get(entries[0].ID)

	if len(entry.ResponseBody) > 100*1024+100 {
		t.Errorf("body not truncated: len=%d", len(entry.ResponseBody))
	}
	if !contains(entry.ResponseBody, "truncated") {
		t.Error("expected truncation marker in body")
	}
}

// ─── TestHistory_SizeLimit_500Entries ──────────────────────────────────────

func TestHistory_SizeLimit_500Entries(t *testing.T) {
	s := newTestStore(t)
	s.maxEntries = 10 // override to keep test fast

	for i := 0; i < 15; i++ {
		err := s.Record(RecordInput{
			WorkspacePath: "/ws",
			RequestName:   "req",
			Method:        "GET",
			URL:           "https://example.com",
			Result:        makeResult(200, "ok", float64(i)),
		})
		if err != nil {
			t.Fatalf("Record %d: %v", i, err)
		}
	}

	_, total, err := s.List(ListFilter{WorkspacePath: "/ws", Limit: 100})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total > 10 {
		t.Errorf("expected at most 10 entries after cleanup, got %d", total)
	}
}

// ─── TestHistory_Cleanup ───────────────────────────────────────────────────

func TestHistory_Cleanup(t *testing.T) {
	s := newTestStore(t)

	for i := 0; i < 3; i++ {
		_ = s.Record(RecordInput{
			WorkspacePath: "/ws",
			RequestName:   "r",
			Method:        "GET",
			URL:           "https://x.com",
			Result:        makeResult(200, "ok", 1),
		})
	}

	entries, total, _ := s.List(ListFilter{WorkspacePath: "/ws"})
	if total != 3 {
		t.Fatalf("expected 3 before clear, got %d", total)
	}

	// Delete one
	if err := s.Delete(entries[0].ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, total, _ = s.List(ListFilter{WorkspacePath: "/ws"})
	if total != 2 {
		t.Errorf("expected 2 after delete, got %d", total)
	}

	// Clear all
	if err := s.Clear("/ws"); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	_, total, _ = s.List(ListFilter{WorkspacePath: "/ws"})
	if total != 0 {
		t.Errorf("expected 0 after clear, got %d", total)
	}
}

// ─── TestHistory_Filter ────────────────────────────────────────────────────

func TestHistory_Filter(t *testing.T) {
	s := newTestStore(t)

	_ = s.Record(RecordInput{WorkspacePath: "/ws", RequestName: "Get Users", Method: "GET", URL: "https://api.example.com/users", Result: makeResult(200, "ok", 10)})
	_ = s.Record(RecordInput{WorkspacePath: "/ws", RequestName: "Create User", Method: "POST", URL: "https://api.example.com/users", Result: makeResult(201, "created", 15)})
	_ = s.Record(RecordInput{WorkspacePath: "/ws", RequestName: "Not Found", Method: "GET", URL: "https://api.example.com/missing", Result: makeResult(404, "not found", 5)})

	// Filter by status
	entries, total, _ := s.List(ListFilter{WorkspacePath: "/ws", StatusCode: 200})
	if total != 1 {
		t.Errorf("filter by 200: expected 1, got %d", total)
	}
	_ = entries

	// Filter by method
	entries, total, _ = s.List(ListFilter{WorkspacePath: "/ws", Method: "POST"})
	if total != 1 {
		t.Errorf("filter by POST: expected 1, got %d", total)
	}

	// Filter by since
	entries, total, _ = s.List(ListFilter{WorkspacePath: "/ws", Since: time.Now().Add(-1 * time.Hour)})
	if total != 3 {
		t.Errorf("filter by since 1h ago: expected 3, got %d", total)
	}
	entries, total, _ = s.List(ListFilter{WorkspacePath: "/ws", Since: time.Now().Add(1 * time.Hour)})
	if total != 0 {
		t.Errorf("filter by future since: expected 0, got %d", total)
	}

	// Search by name
	entries, total, _ = s.List(ListFilter{WorkspacePath: "/ws", Search: "User"})
	if total != 2 {
		t.Errorf("search 'User': expected 2, got %d", total)
	}
}

// ─── TestHistory_Isolation ─────────────────────────────────────────────────

func TestHistory_Isolation(t *testing.T) {
	s := newTestStore(t)

	_ = s.Record(RecordInput{WorkspacePath: "/ws-a", RequestName: "A", Method: "GET", URL: "https://a.com", Result: makeResult(200, "a", 1)})
	_ = s.Record(RecordInput{WorkspacePath: "/ws-b", RequestName: "B", Method: "GET", URL: "https://b.com", Result: makeResult(200, "b", 1)})

	_, totalA, _ := s.List(ListFilter{WorkspacePath: "/ws-a"})
	_, totalB, _ := s.List(ListFilter{WorkspacePath: "/ws-b"})

	if totalA != 1 || totalB != 1 {
		t.Errorf("workspace isolation failed: A=%d B=%d", totalA, totalB)
	}

	_ = s.Clear("/ws-a")

	_, totalA, _ = s.List(ListFilter{WorkspacePath: "/ws-a"})
	_, totalB, _ = s.List(ListFilter{WorkspacePath: "/ws-b"})

	if totalA != 0 {
		t.Errorf("expected 0 for ws-a after clear, got %d", totalA)
	}
	if totalB != 1 {
		t.Errorf("expected ws-b untouched, got %d", totalB)
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────

func contains(s, substr string) bool {
	return len(substr) > 0 && len(s) >= len(substr) &&
		(s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

var _ = os.Stderr // keep import
