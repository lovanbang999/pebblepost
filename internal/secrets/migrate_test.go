package secrets_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pebblepost/internal/secrets"
)

func TestMigrate_HappyPath(t *testing.T) {
	src := newMock(secrets.BackendFile)
	_ = src.Set("dev", "API_KEY", "key1")
	_ = src.Set("dev", "DB_PASS", "pass1")

	dst := newMock(secrets.BackendKeychain)

	confirmed := false
	opts := secrets.MigrateOptions{
		Confirm: func(plan secrets.MigrationPlan) bool {
			confirmed = true
			if plan.EnvName != "dev" {
				t.Errorf("plan env: got %q, want dev", plan.EnvName)
			}
			if len(plan.Keys) != 2 {
				t.Errorf("plan keys: got %d, want 2", len(plan.Keys))
			}
			return true
		},
	}

	_, err := secrets.Migrate("dev", src, dst, opts)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if !confirmed {
		t.Error("confirm callback was not called")
	}

	// All keys must now exist in dst
	for _, key := range []string{"API_KEY", "DB_PASS"} {
		if _, err := dst.Get("dev", key); err != nil {
			t.Errorf("dst.Get(%q): %v", key, err)
		}
	}

	// All keys must be removed from src
	keys, _ := src.List("dev")
	if len(keys) != 0 {
		t.Errorf("src still has %d keys after migration: %v", len(keys), keys)
	}
}

func TestMigrate_UserDeclines(t *testing.T) {
	src := newMock(secrets.BackendFile)
	_ = src.Set("dev", "API_KEY", "key1")
	dst := newMock(secrets.BackendKeychain)

	opts := secrets.MigrateOptions{
		Confirm: func(_ secrets.MigrationPlan) bool { return false },
	}

	_, err := secrets.Migrate("dev", src, dst, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// src must be untouched
	keys, _ := src.List("dev")
	if len(keys) != 1 {
		t.Errorf("src was modified despite user declining; keys: %v", keys)
	}
	// dst must be empty
	dstKeys, _ := dst.List("dev")
	if len(dstKeys) != 0 {
		t.Errorf("dst has keys despite user declining: %v", dstKeys)
	}
}

func TestMigrate_EmptySource_NoOp(t *testing.T) {
	src := newMock(secrets.BackendFile)
	dst := newMock(secrets.BackendKeychain)

	_, err := secrets.Migrate("dev", src, dst, secrets.MigrateOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMigrate_SameBackend_Error(t *testing.T) {
	src := newMock(secrets.BackendFile)
	dst := newMock(secrets.BackendFile)

	_, err := secrets.Migrate("dev", src, dst, secrets.MigrateOptions{})
	if err == nil {
		t.Fatal("expected error migrating to the same backend")
	}
}

// errStore simulates a destination that fails on Set.
type errStore struct {
	*mockStore
	failOn string
}

func (e *errStore) Set(svc, key, val string) error {
	if key == e.failOn {
		return errors.New("simulated keychain failure")
	}
	return e.mockStore.Set(svc, key, val)
}

func TestMigrate_PartialFailure_Rollback(t *testing.T) {
	src := newMock(secrets.BackendFile)
	_ = src.Set("dev", "KEY_A", "val_a")
	_ = src.Set("dev", "KEY_B", "val_b")

	dst := &errStore{
		mockStore: newMock(secrets.BackendKeychain),
		failOn:    "KEY_B",
	}

	opts := secrets.MigrateOptions{
		Confirm: func(_ secrets.MigrationPlan) bool { return true },
	}

	_, err := secrets.Migrate("dev", src, dst, opts)
	if err == nil {
		t.Fatal("expected error on partial failure")
	}

	// dst should have no keys (rollback cleaned up KEY_A)
	dstKeys, _ := dst.mockStore.List("dev")
	if len(dstKeys) != 0 {
		t.Errorf("dst was not rolled back: %v", dstKeys)
	}
}

func TestRollback(t *testing.T) {
	dir := t.TempDir()
	origFile := filepath.Join(dir, "dev.secret.env.json")
	backupFile := filepath.Join(dir, "dev.secret.env.json.bak.123456")

	_ = os.WriteFile(backupFile, []byte(`{"variables":[{"key":"TOKEN","value":"restored-val"}]}`), 0600)

	dst := newMock(secrets.BackendKeychain)
	_ = dst.Set("dev", "TOKEN", "keychain-val")

	if err := secrets.Rollback("dev", backupFile, dst); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	// Verify file was restored
	data, err := os.ReadFile(origFile)
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if !strings.Contains(string(data), "restored-val") {
		t.Errorf("expected restored content, got: %s", string(data))
	}

	// Verify dst key was removed
	keys, _ := dst.List("dev")
	if len(keys) != 0 {
		t.Errorf("dst still has keys after rollback: %v", keys)
	}
}
