//go:build keychain

package secrets_test

import (
	"errors"
	"testing"

	"pebblepost/internal/secrets"
)

func TestKeychainStore_Live(t *testing.T) {
	const testWorkspace = "pebblepost-test-suite"
	const testEnv = "unit-test-env"
	const testKey = "TEST_API_TOKEN"
	const testVal = "super-secret-token-xyz-123"

	store := secrets.NewKeychainStore(testWorkspace)
	if store.Backend() != secrets.BackendKeychain {
		t.Fatalf("expected backend %s, got %s", secrets.BackendKeychain, store.Backend())
	}

	// Cleanup before test in case previous run aborted
	_ = store.Delete(testEnv, testKey)

	// Verify not found initially
	_, err := store.Get(testEnv, testKey)
	if !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for unset key, got: %v", err)
	}

	// Set value
	if err := store.Set(testEnv, testKey, testVal); err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get value
	got, err := store.Get(testEnv, testKey)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got != testVal {
		t.Errorf("Get returned %q, want %q", got, testVal)
	}

	// List keys
	keys, err := store.List(testEnv)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	found := false
	for _, k := range keys {
		if k == testKey {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("List returned %v, expected %s in list", keys, testKey)
	}

	// Delete value
	if err := store.Delete(testEnv, testKey); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify not found after delete
	_, err = store.Get(testEnv, testKey)
	if !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after deletion, got: %v", err)
	}

	// Delete again should return ErrNotFound
	err = store.Delete(testEnv, testKey)
	if !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("expected ErrNotFound when deleting nonexistent key, got: %v", err)
	}
}
