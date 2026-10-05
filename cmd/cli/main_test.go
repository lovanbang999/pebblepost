package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMultiFlag(t *testing.T) {
	var f multiFlag
	if f.String() != "" {
		t.Fatalf("expected empty string, got %s", f.String())
	}

	if err := f.Set("foo=bar"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := f.Set("baz=qux"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(f) != 2 {
		t.Fatalf("expected 2 items, got %d", len(f))
	}
	if f[0] != "foo=bar" || f[1] != "baz=qux" {
		t.Fatalf("unexpected values: %v", f)
	}
}

func TestPrintUsage(t *testing.T) {
	// Ensure printUsage runs without panic
	printUsage()
}

func TestIsCollectionTrusted(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "sub", "col")
	_ = os.MkdirAll(subDir, 0755)

	if isCollectionTrusted(subDir) {
		t.Fatalf("expected untrusted directory when no .pebbletrust file exists")
	}

	// Create .pebbletrust in parent dir
	trustFile := filepath.Join(dir, ".pebbletrust")
	if err := os.WriteFile(trustFile, []byte("trusted"), 0644); err != nil {
		t.Fatalf("failed to write trust file: %v", err)
	}

	if !isCollectionTrusted(subDir) {
		t.Fatalf("expected subDir to be trusted via parent .pebbletrust")
	}
}
