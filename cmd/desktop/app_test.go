package main

import (
	"context"
	"testing"
)

func TestApp_Lifecycle(t *testing.T) {
	t.Setenv("DATA_DIR", t.TempDir())
	app := NewApp()
	if app == nil {
		t.Fatal("expected non-nil App")
	}

	if app.Mux() == nil {
		t.Fatal("expected non-nil Mux")
	}

	app.shutdown(context.Background())
}
