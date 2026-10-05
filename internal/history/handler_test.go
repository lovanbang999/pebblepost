package history

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandler_RoutesAndList(t *testing.T) {
	s := newTestStore(t)
	h := NewHandler(s)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Missing workspace parameter
	req := httptest.NewRequest(http.MethodGet, "/api/history", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing workspace, got %d", w.Code)
	}

	// Add records
	_ = s.Record(RecordInput{
		WorkspacePath: "/test-ws",
		RequestName:   "Req1",
		Method:        "GET",
		URL:           "https://api.example.com/one",
		Result:        makeResult(200, `{"ok":true}`, 10),
	})
	_ = s.Record(RecordInput{
		WorkspacePath: "/test-ws",
		RequestName:   "Req2",
		Method:        "POST",
		URL:           "https://api.example.com/two",
		Result:        makeResult(404, `{"error":"not found"}`, 25),
	})

	// List with filters: search, method, status, page, limit, since
	since := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
	req = httptest.NewRequest(http.MethodGet, "/api/history?workspace=/test-ws&search=Req&method=GET&status=200&page=1&limit=10&since="+since, nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Entries []HistoryEntry `json:"entries"`
		Total   int            `json:"total"`
		Page    int            `json:"page"`
		Limit   int            `json:"limit"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 1 || len(resp.Entries) != 1 {
		t.Fatalf("expected 1 entry, got total=%d, len=%d", resp.Total, len(resp.Entries))
	}
	if resp.Entries[0].Method != "GET" {
		t.Errorf("expected GET, got %s", resp.Entries[0].Method)
	}

	// Empty list check
	req = httptest.NewRequest(http.MethodGet, "/api/history?workspace=/other-ws", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Clear workspace history via DELETE
	req = httptest.NewRequest(http.MethodDelete, "/api/history?workspace=/test-ws", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for DELETE, got %d: %s", w.Code, w.Body.String())
	}

	// Unsupported method on /api/history
	req = httptest.NewRequest(http.MethodPatch, "/api/history?workspace=/test-ws", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandler_Entry(t *testing.T) {
	s := newTestStore(t)
	h := NewHandler(s)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	_ = s.Record(RecordInput{
		WorkspacePath: "/test-ws",
		RequestName:   "Req1",
		Method:        "GET",
		URL:           "https://api.example.com/one",
		Result:        makeResult(200, `{"ok":true}`, 10),
	})

	entries, _, _ := s.List(ListFilter{WorkspacePath: "/test-ws"})
	if len(entries) == 0 {
		t.Fatal("expected at least 1 entry")
	}
	entryID := entries[0].ID

	// Invalid ID string
	req := httptest.NewRequest(http.MethodGet, "/api/history/invalid", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid id, got %d", w.Code)
	}

	// Empty ID string -> 404
	req = httptest.NewRequest(http.MethodGet, "/api/history/", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for empty id, got %d", w.Code)
	}

	// Non-existent ID -> 404
	req = httptest.NewRequest(http.MethodGet, "/api/history/99999", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for non-existent id, got %d", w.Code)
	}

	// GET valid entry
	req = httptest.NewRequest(http.MethodGet, "/api/history/"+formatInt(entryID), nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var entry HistoryEntry
	if err := json.NewDecoder(w.Body).Decode(&entry); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if entry.ID != entryID {
		t.Fatalf("expected id %d, got %d", entryID, entry.ID)
	}

	// DELETE valid entry
	req = httptest.NewRequest(http.MethodDelete, "/api/history/"+formatInt(entryID), nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on DELETE, got %d", w.Code)
	}

	// Method not allowed on /api/history/{id}
	req = httptest.NewRequest(http.MethodPost, "/api/history/"+formatInt(entryID), nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func formatInt(n int64) string {
	return strconvFormatInt(n)
}

func strconvFormatInt(n int64) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}
