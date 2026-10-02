package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pebblepost/internal/types"
)

func TestWorkspaceService_DuplicateRequest(t *testing.T) {
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "get-user.pebble.json")

	svc := NewWorkspaceService()
	req := &types.RequestDefinition{
		Name:   "Get User",
		Method: "GET",
		URL:    "https://api.example.com/users/1",
	}

	if err := svc.SaveRequest(sourcePath, req); err != nil {
		t.Fatalf("failed to save initial request: %v", err)
	}

	// 1. First duplicate: should produce "get-user (Copy).pebble.json"
	dup1, err := svc.DuplicateRequest(sourcePath)
	if err != nil {
		t.Fatalf("DuplicateRequest 1 failed: %v", err)
	}
	expected1 := filepath.Join(tmpDir, "get-user (Copy).pebble.json")
	if dup1 != expected1 {
		t.Errorf("expected duplicate path %s, got %s", expected1, dup1)
	}

	dup1Req, err := svc.ReadRequest(dup1)
	if err != nil {
		t.Fatalf("failed to read duplicate 1: %v", err)
	}
	if dup1Req.Name != "Get User (Copy)" {
		t.Errorf("expected name 'Get User (Copy)', got %s", dup1Req.Name)
	}

	// 2. Second duplicate: should produce "get-user (Copy 2).pebble.json"
	dup2, err := svc.DuplicateRequest(sourcePath)
	if err != nil {
		t.Fatalf("DuplicateRequest 2 failed: %v", err)
	}
	expected2 := filepath.Join(tmpDir, "get-user (Copy 2).pebble.json")
	if dup2 != expected2 {
		t.Errorf("expected duplicate path %s, got %s", expected2, dup2)
	}
}

func TestWorkspaceService_MovePath(t *testing.T) {
	tmpDir := t.TempDir()
	sourcePath := filepath.Join(tmpDir, "login.pebble.json")
	targetFolder := filepath.Join(tmpDir, "collections", "auth")

	svc := NewWorkspaceService()
	req := &types.RequestDefinition{
		Name:   "Login",
		Method: "POST",
		URL:    "https://api.example.com/login",
	}

	if err := svc.SaveRequest(sourcePath, req); err != nil {
		t.Fatalf("failed to save request: %v", err)
	}

	// Move file into targetFolder
	newPath, err := svc.MovePath(sourcePath, targetFolder)
	if err != nil {
		t.Fatalf("MovePath failed: %v", err)
	}

	expectedPath := filepath.Join(targetFolder, "login.pebble.json")
	if newPath != expectedPath {
		t.Errorf("expected newPath %s, got %s", expectedPath, newPath)
	}

	// Original should no longer exist
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Errorf("source file %s still exists after move", sourcePath)
	}

	// New path must exist and be readable
	readReq, err := svc.ReadRequest(newPath)
	if err != nil {
		t.Fatalf("failed to read moved request: %v", err)
	}
	if readReq.Name != "Login" {
		t.Errorf("expected name 'Login', got %s", readReq.Name)
	}
}

func TestWorkspaceService_TrashPath(t *testing.T) {
	tmpDir := t.TempDir()
	reqPath := filepath.Join(tmpDir, "delete-me.pebble.json")

	svc := NewWorkspaceService()
	req := &types.RequestDefinition{
		Name:   "Delete Me",
		Method: "DELETE",
		URL:    "https://api.example.com/item",
	}

	if err := svc.SaveRequest(reqPath, req); err != nil {
		t.Fatalf("failed to save request: %v", err)
	}

	// Move to trash
	if err := svc.TrashPath(tmpDir, reqPath); err != nil {
		t.Fatalf("TrashPath failed: %v", err)
	}

	// Original file must no longer exist at reqPath
	if _, err := os.Stat(reqPath); !os.IsNotExist(err) {
		t.Errorf("file still exists at %s after TrashPath", reqPath)
	}

	// Trash directory must exist and contain the backed up file
	trashDir := filepath.Join(tmpDir, PebbleDir, "trash")
	entries, err := os.ReadDir(trashDir)
	if err != nil {
		t.Fatalf("failed to read trash directory: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 file in trash, got %d", len(entries))
	}
	if !strings.HasSuffix(entries[0].Name(), "delete-me.pebble.json") {
		t.Errorf("unexpected trashed filename: %s", entries[0].Name())
	}
}
