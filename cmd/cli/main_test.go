package main

import (
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
