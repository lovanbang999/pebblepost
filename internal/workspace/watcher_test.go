package workspace

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcher_BasicFileEvent(t *testing.T) {
	tmpDir := t.TempDir()

	watcher, err := NewWatcherWithConfig(tmpDir, 30*time.Millisecond, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer watcher.Close()

	events, unsubscribe := watcher.Subscribe()
	defer unsubscribe()

	testFile := filepath.Join(tmpDir, "request.pebble.json")
	if err := os.WriteFile(testFile, []byte(`{"name":"test"}`), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	select {
	case evt, ok := <-events:
		if !ok {
			t.Fatal("events channel closed unexpectedly")
		}
		if evt.Path != testFile {
			t.Errorf("expected event for %s, got %s", testFile, evt.Path)
		}
		if evt.RelPath != "request.pebble.json" {
			t.Errorf("expected relPath 'request.pebble.json', got %s", evt.RelPath)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for file event")
	}
}

func TestWatcher_SelfWriteSuppression(t *testing.T) {
	tmpDir := t.TempDir()

	watcher, err := NewWatcherWithConfig(tmpDir, 30*time.Millisecond, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer watcher.Close()

	events, unsubscribe := watcher.Subscribe()
	defer unsubscribe()

	suppressedFile := filepath.Join(tmpDir, "internal-save.pebble.json")
	// Register self-write suppression
	watcher.Suppress(suppressedFile, 500*time.Millisecond)

	if !watcher.IsSuppressed(suppressedFile) {
		t.Errorf("expected file to be suppressed")
	}

	// Write the file
	if err := os.WriteFile(suppressedFile, []byte(`{"saved": true}`), 0644); err != nil {
		t.Fatalf("failed to write suppressed file: %v", err)
	}

	// Make sure no event is received for suppressed file within 100ms
	select {
	case evt := <-events:
		if evt.Path == suppressedFile {
			t.Errorf("received event for suppressed file: %+v", evt)
		}
	case <-time.After(100 * time.Millisecond):
		// Success: suppressed event was not delivered
	}
}

func TestWatcher_DebounceCoalescesWrites(t *testing.T) {
	tmpDir := t.TempDir()

	testFile := filepath.Join(tmpDir, "debounce.pebble.json")
	if err := os.WriteFile(testFile, []byte("initial"), 0644); err != nil {
		t.Fatalf("init write failed: %v", err)
	}

	debounceDur := 200 * time.Millisecond
	watcher, err := NewWatcherWithConfig(tmpDir, debounceDur, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	defer watcher.Close()

	events, unsubscribe := watcher.Subscribe()
	defer unsubscribe()

	// Perform 5 rapid writes in a tight loop
	for i := 0; i < 5; i++ {
		if err := os.WriteFile(testFile, []byte("write"), 0644); err != nil {
			t.Fatalf("write failed: %v", err)
		}
	}

	// We should receive only 1 debounced event batch
	select {
	case evt := <-events:
		if evt.Path != testFile {
			t.Errorf("expected event path %s, got %s", testFile, evt.Path)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for debounced event")
	}

	// Verify no immediate second event comes through
	select {
	case evt := <-events:
		t.Errorf("unexpected extra event received: %+v", evt)
	case <-time.After(300 * time.Millisecond):
		// OK
	}
}
