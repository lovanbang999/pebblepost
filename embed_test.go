package pebblepost

import (
	"testing"
)

func TestFrontendFS(t *testing.T) {
	fsys, err := FrontendFS()
	if err != nil {
		t.Fatalf("FrontendFS failed: %v", err)
	}
	if fsys == nil {
		t.Fatal("expected non-nil fs.FS")
	}
}
