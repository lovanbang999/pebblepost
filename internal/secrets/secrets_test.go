package secrets_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"pebblepost/internal/secrets"
)

// --- Mock backend ---

type mockStore struct {
	data    map[string]map[string]string // service → key → value
	backend secrets.SecretBackendType
}

func newMock(bt secrets.SecretBackendType) *mockStore {
	return &mockStore{data: make(map[string]map[string]string), backend: bt}
}

func (m *mockStore) Backend() secrets.SecretBackendType { return m.backend }

func (m *mockStore) Get(svc, key string) (string, error) {
	if v, ok := m.data[svc][key]; ok {
		return v, nil
	}
	return "", fmt.Errorf("%w: %s/%s", secrets.ErrNotFound, svc, key)
}

func (m *mockStore) Set(svc, key, val string) error {
	if m.data[svc] == nil {
		m.data[svc] = make(map[string]string)
	}
	m.data[svc][key] = val
	return nil
}

func (m *mockStore) Delete(svc, key string) error {
	if _, ok := m.data[svc][key]; !ok {
		return fmt.Errorf("%w: %s/%s", secrets.ErrNotFound, svc, key)
	}
	delete(m.data[svc], key)
	return nil
}

func (m *mockStore) List(svc string) ([]string, error) {
	keys := make([]string, 0, len(m.data[svc]))
	for k := range m.data[svc] {
		keys = append(keys, k)
	}
	return keys, nil
}

// --- Tests ---

func TestMockStore_CRUD(t *testing.T) {
	store := newMock(secrets.BackendFile)

	// Get on missing key returns ErrNotFound
	_, err := store.Get("env1", "TOKEN")
	if err == nil {
		t.Fatal("expected error for missing key")
	}

	// Set and Get round-trip
	if err := store.Set("env1", "TOKEN", "abc123"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	val, err := store.Get("env1", "TOKEN")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "abc123" {
		t.Errorf("got %q, want %q", val, "abc123")
	}

	// List
	keys, _ := store.List("env1")
	if len(keys) != 1 || keys[0] != "TOKEN" {
		t.Errorf("List: got %v, want [TOKEN]", keys)
	}

	// Delete
	if err := store.Delete("env1", "TOKEN"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := store.Get("env1", "TOKEN"); err == nil {
		t.Fatal("expected ErrNotFound after delete")
	}

	// Delete missing key
	if err := store.Delete("env1", "GHOST"); err == nil {
		t.Fatal("expected error deleting non-existent key")
	}
}

func TestFileStore_CRUD(t *testing.T) {
	dir := t.TempDir()
	store := secrets.NewFileStore(dir)

	// Get on missing env returns error (file not found)
	_, err := store.Get("dev", "API_KEY")
	if err == nil {
		t.Fatal("expected error for missing secret file")
	}

	// Set creates file
	if err := store.Set("dev", "API_KEY", "s3cr3t"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Check file was created with mode 0600
	path := filepath.Join(dir, "dev.secret.env.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("secret file not created: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("file mode %v, want 0600", info.Mode().Perm())
	}

	// Get round-trip
	val, err := store.Get("dev", "API_KEY")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "s3cr3t" {
		t.Errorf("got %q, want %q", val, "s3cr3t")
	}

	// Set second key
	_ = store.Set("dev", "DB_PASSWORD", "hunter2")
	keys, _ := store.List("dev")
	if len(keys) != 2 {
		t.Errorf("List: got %d keys, want 2", len(keys))
	}

	// Delete one key
	if err := store.Delete("dev", "API_KEY"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	keys, _ = store.List("dev")
	if len(keys) != 1 {
		t.Errorf("List after delete: got %d keys, want 1", len(keys))
	}

	// Delete last key removes file
	_ = store.Delete("dev", "DB_PASSWORD")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("secret file should be removed when all keys deleted")
	}
}

func TestEnvStore_ReadOnly(t *testing.T) {
	store := secrets.NewEnvStore()

	if err := store.Set("env", "KEY", "val"); err != secrets.ErrReadOnly {
		t.Errorf("Set: got %v, want ErrReadOnly", err)
	}
	if err := store.Delete("env", "KEY"); err != secrets.ErrReadOnly {
		t.Errorf("Delete: got %v, want ErrReadOnly", err)
	}
	_, err := store.List("env")
	if err != secrets.ErrReadOnly {
		t.Errorf("List: got %v, want ErrReadOnly", err)
	}
}

func TestEnvStore_Get(t *testing.T) {
	t.Setenv("PEBBLE_TEST_SECRET", "my-value")
	store := secrets.NewEnvStore()

	val, err := store.Get("any", "PEBBLE_TEST_SECRET")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "my-value" {
		t.Errorf("got %q, want %q", val, "my-value")
	}

	_, err = store.Get("any", "PEBBLE_NONEXISTENT_VAR_XYZ")
	if err == nil {
		t.Fatal("expected error for unset OS env var")
	}
}

func TestResolveOSEnvRefs(t *testing.T) {
	t.Setenv("MY_TOKEN", "tok123")

	var warnings []string
	warn := func(msg string) { warnings = append(warnings, msg) }

	// Resolved reference
	out, secs := secrets.ResolveOSEnvRefs("Bearer ${OS_ENV:MY_TOKEN}", warn)
	if out != "Bearer tok123" {
		t.Errorf("resolved: got %q, want %q", out, "Bearer tok123")
	}
	if len(secs) != 1 || secs[0] != "tok123" {
		t.Errorf("secrets: got %v, want [tok123]", secs)
	}

	// Unresolved reference leaves placeholder intact, emits warning
	warnings = nil
	out, secs = secrets.ResolveOSEnvRefs("${OS_ENV:UNDEFINED_VAR_XYZ}", warn)
	if out != "${OS_ENV:UNDEFINED_VAR_XYZ}" {
		t.Errorf("unresolved: got %q, want placeholder", out)
	}
	if len(warnings) == 0 {
		t.Error("expected a warning for unresolved OS env ref")
	}
	if len(secs) != 0 {
		t.Errorf("no secrets expected for unresolved ref, got %v", secs)
	}

	// No reference passes through unchanged
	out, secs = secrets.ResolveOSEnvRefs("plain value", warn)
	if out != "plain value" {
		t.Errorf("plain: got %q", out)
	}
	if len(secs) != 0 {
		t.Errorf("plain: unexpected secrets %v", secs)
	}
}

func TestSecretBackendConfig_BackendFor(t *testing.T) {
	cfg := secrets.SecretBackendConfig{
		Default: secrets.BackendFile,
		PerEnv: map[string]secrets.SecretBackendType{
			"prod": secrets.BackendKeychain,
		},
	}

	if got := cfg.BackendFor("dev"); got != secrets.BackendFile {
		t.Errorf("dev: got %v, want file", got)
	}
	if got := cfg.BackendFor("prod"); got != secrets.BackendKeychain {
		t.Errorf("prod: got %v, want keychain", got)
	}

	// Empty config defaults to file
	empty := secrets.SecretBackendConfig{}
	if got := empty.BackendFor("anything"); got != secrets.BackendFile {
		t.Errorf("empty: got %v, want file", got)
	}
}
