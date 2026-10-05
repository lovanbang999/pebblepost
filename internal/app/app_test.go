package app

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBootstrapWithToken(t *testing.T) {
	dataDir := t.TempDir()
	inst, err := BootstrapWithToken(dataDir, "test-token")
	if err != nil {
		t.Fatalf("failed to bootstrap app: %v", err)
	}
	defer inst.Close()

	// Health endpoint should be unauthenticated and return 200
	req := httptest.NewRequest("GET", "/api/health", nil)
	rec := httptest.NewRecorder()
	inst.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
}
