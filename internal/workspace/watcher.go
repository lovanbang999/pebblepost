package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// FileChangeOp describes the type of filesystem change.
type FileChangeOp string

const (
	OpCreate FileChangeOp = "create"
	OpWrite  FileChangeOp = "write"
	OpRemove FileChangeOp = "remove"
	OpRename FileChangeOp = "rename"
)

// FileChangeEvent is broadcast to listeners when a watched file is modified.
type FileChangeEvent struct {
	Path    string       `json:"path"`
	RelPath string       `json:"relPath"`
	Op      FileChangeOp `json:"op"`
	MTime   int64        `json:"mtime"`
}

// Watcher watches a workspace directory recursively for external changes,
// with debouncing (~200ms) and self-write suppression.
type Watcher struct {
	rootPath     string
	fsWatcher    *fsnotify.Watcher
	debounceDur  time.Duration
	suppressTTL  time.Duration
	suppressMap  sync.Map // normalized path -> time.Time (expiry)
	listenersMu  sync.RWMutex
	listeners    map[chan FileChangeEvent]struct{}
	pendingMu    sync.Mutex
	pending      map[string]FileChangeEvent
	timer        *time.Timer
	stopChan     chan struct{}
	closeOnce    sync.Once
}

// NewWatcher creates a new Watcher monitoring rootPath with default 200ms debounce and 1000ms suppression TTL.
func NewWatcher(rootPath string) (*Watcher, error) {
	return NewWatcherWithConfig(rootPath, 200*time.Millisecond, 1000*time.Millisecond)
}

// NewWatcherWithConfig creates a new Watcher with customized debounce and suppression TTL.
func NewWatcherWithConfig(rootPath string, debounceDur, suppressTTL time.Duration) (*Watcher, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}

	w := &Watcher{
		rootPath:    rootPath,
		fsWatcher:   fsWatcher,
		debounceDur: debounceDur,
		suppressTTL: suppressTTL,
		listeners:   make(map[chan FileChangeEvent]struct{}),
		pending:     make(map[string]FileChangeEvent),
		stopChan:    make(chan struct{}),
	}

	if err := w.watchRecursive(rootPath); err != nil {
		_ = fsWatcher.Close()
		return nil, err
	}

	go w.eventLoop()

	return w, nil
}

// Suppress registers a path as written internally by PebblePost.
// Any watcher events for this path within duration d will be ignored.
func (w *Watcher) Suppress(path string, d time.Duration) {
	if d <= 0 {
		d = w.suppressTTL
	}
	clean := filepath.Clean(path)
	w.suppressMap.Store(clean, time.Now().Add(d))
}

// IsSuppressed returns true if the path is currently suppressed.
func (w *Watcher) IsSuppressed(path string) bool {
	clean := filepath.Clean(path)
	val, ok := w.suppressMap.Load(clean)
	if !ok {
		return false
	}
	expiry, ok := val.(time.Time)
	if !ok || time.Now().After(expiry) {
		w.suppressMap.Delete(clean)
		return false
	}
	return true
}

// Subscribe returns a channel that receives debounced file change events.
// Call the returned cleanup function when done.
func (w *Watcher) Subscribe() (<-chan FileChangeEvent, func()) {
	ch := make(chan FileChangeEvent, 32)
	w.listenersMu.Lock()
	w.listeners[ch] = struct{}{}
	w.listenersMu.Unlock()

	unsubscribe := func() {
		w.listenersMu.Lock()
		delete(w.listeners, ch)
		w.listenersMu.Unlock()
		close(ch)
	}

	return ch, unsubscribe
}

// Close stops the watcher and frees all resources.
func (w *Watcher) Close() error {
	var err error
	w.closeOnce.Do(func() {
		close(w.stopChan)
		w.pendingMu.Lock()
		if w.timer != nil {
			w.timer.Stop()
		}
		w.pendingMu.Unlock()
		err = w.fsWatcher.Close()
	})
	return err
}

func (w *Watcher) watchRecursive(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if shouldIgnoreName(name) {
				return filepath.SkipDir
			}
			return w.fsWatcher.Add(path)
		}
		return nil
	})
}

func (w *Watcher) eventLoop() {
	for {
		select {
		case <-w.stopChan:
			return

		case event, ok := <-w.fsWatcher.Events:
			if !ok {
				return
			}
			w.handleFSEvent(event)

		case _, ok := <-w.fsWatcher.Errors:
			if !ok {
				return
			}
		}
	}
}

func (w *Watcher) handleFSEvent(event fsnotify.Event) {
	name := filepath.Base(event.Name)
	if shouldIgnoreName(name) || strings.HasSuffix(name, ".tmp") {
		return
	}

	// Auto-watch newly created subdirectories
	if event.Has(fsnotify.Create) {
		if fi, err := os.Stat(event.Name); err == nil && fi.IsDir() {
			_ = w.watchRecursive(event.Name)
		}
	}

	if w.IsSuppressed(event.Name) {
		return
	}

	var op FileChangeOp
	switch {
	case event.Has(fsnotify.Create):
		op = OpCreate
	case event.Has(fsnotify.Write):
		op = OpWrite
	case event.Has(fsnotify.Remove):
		op = OpRemove
	case event.Has(fsnotify.Rename):
		op = OpRename
	default:
		return
	}

	relPath, err := filepath.Rel(w.rootPath, event.Name)
	if err != nil {
		relPath = event.Name
	}

	mtime := time.Now().UnixMilli()
	if fi, err := os.Stat(event.Name); err == nil {
		mtime = fi.ModTime().UnixMilli()
	}

	change := FileChangeEvent{
		Path:    event.Name,
		RelPath: relPath,
		Op:      op,
		MTime:   mtime,
	}

	w.pendingMu.Lock()
	defer w.pendingMu.Unlock()

	w.pending[event.Name] = change

	if w.timer != nil {
		w.timer.Stop()
	}
	w.timer = time.AfterFunc(w.debounceDur, w.flushPending)
}

func (w *Watcher) flushPending() {
	w.pendingMu.Lock()
	batch := make([]FileChangeEvent, 0, len(w.pending))
	for _, evt := range w.pending {
		batch = append(batch, evt)
	}
	w.pending = make(map[string]FileChangeEvent)
	w.pendingMu.Unlock()

	if len(batch) == 0 {
		return
	}

	w.listenersMu.RLock()
	defer w.listenersMu.RUnlock()

	for _, evt := range batch {
		for ch := range w.listeners {
			select {
			case ch <- evt:
			default:
				// Avoid blocking if listener is slow
			}
		}
	}
}

func shouldIgnoreName(name string) bool {
	if strings.HasPrefix(name, ".") ||
		name == "node_modules" ||
		name == "dist" ||
		name == "build" ||
		name == "bin" {
		return true
	}
	return false
}
