package openapisync

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"pebblepost/internal/workspace"
)

func TestHandler_LinkDiffAndApply(t *testing.T) {
	tmpDir := t.TempDir()
	folderPath := filepath.Join(tmpDir, "my-collection")
	_ = os.MkdirAll(folderPath, 0755)

	specFile := filepath.Join(tmpDir, "spec.yaml")
	_ = os.WriteFile(specFile, []byte(specV1YAML), 0644)

	wsSvc := workspace.NewWorkspaceService()
	h := NewHandler(wsSvc)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// 1. Link Folder
	linkBody, _ := json.Marshal(LinkRequest{
		FolderPath:   folderPath,
		SpecLocation: specFile,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/sync/link", bytes.NewReader(linkBody))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("link endpoint failed with code %d: %s", rec.Code, rec.Body.String())
	}

	var linkRes map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &linkRes)
	if linkRes["success"] != true {
		t.Errorf("expected success true in link response")
	}

	// 2. GET /api/sync/diff without spec param (uses folder config)
	reqDiff := httptest.NewRequest(http.MethodGet, "/api/sync/diff?folder="+folderPath, nil)
	recDiff := httptest.NewRecorder()
	mux.ServeHTTP(recDiff, reqDiff)

	if recDiff.Code != http.StatusOK {
		t.Fatalf("diff endpoint failed with code %d: %s", recDiff.Code, recDiff.Body.String())
	}

	var diffRep SyncDiffReport
	_ = json.Unmarshal(recDiff.Body.Bytes(), &diffRep)
	if diffRep.AddedCount != 3 {
		t.Errorf("expected 3 added endpoints, got %d", diffRep.AddedCount)
	}

	// 3. POST /api/sync/apply
	applyBody, _ := json.Marshal(ApplyRequest{
		FolderPath: folderPath,
	})
	reqApply := httptest.NewRequest(http.MethodPost, "/api/sync/apply", bytes.NewReader(applyBody))
	recApply := httptest.NewRecorder()
	mux.ServeHTTP(recApply, reqApply)

	if recApply.Code != http.StatusOK {
		t.Fatalf("apply endpoint failed with code %d: %s", recApply.Code, recApply.Body.String())
	}

	var applyRes ApplyResult
	_ = json.Unmarshal(recApply.Body.Bytes(), &applyRes)
	if applyRes.AppliedCount != 3 {
		t.Errorf("expected 3 applied files, got %d", applyRes.AppliedCount)
	}

	// 4. GET /api/sync/diff again - should now report no drift
	reqDiff2 := httptest.NewRequest(http.MethodGet, "/api/sync/diff?folder="+folderPath, nil)
	recDiff2 := httptest.NewRecorder()
	mux.ServeHTTP(recDiff2, reqDiff2)

	var diffRep2 SyncDiffReport
	_ = json.Unmarshal(recDiff2.Body.Bytes(), &diffRep2)
	if diffRep2.HasDrift {
		t.Errorf("expected no drift after apply")
	}

	// 5. Verify folder definition has schemaVersion 2
	folderDef, _ := wsSvc.ReadFolder(folderPath)
	if folderDef.SchemaVersion != 2 {
		t.Errorf("expected folder schemaVersion 2, got %d", folderDef.SchemaVersion)
	}
	if folderDef.OpenAPISync == nil || folderDef.OpenAPISync.SpecLocation != specFile {
		t.Errorf("expected OpenAPISync.SpecLocation to match %s", specFile)
	}
}

func TestHandler_ErrorCases(t *testing.T) {
	wsSvc := workspace.NewWorkspaceService()
	h := NewHandler(wsSvc)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Missing folder query param
	req := httptest.NewRequest(http.MethodGet, "/api/sync/diff", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing folder, got %d", rec.Code)
	}

	// Unlinked folder
	tmpDir := t.TempDir()
	req = httptest.NewRequest(http.MethodGet, "/api/sync/diff?folder="+tmpDir, nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unlinked folder, got %d", rec.Code)
	}
}
