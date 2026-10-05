package workspace

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"pebblepost/internal/types"
)

func TestEnvironmentService_MergeAndCRUD(t *testing.T) {
	tempDir := t.TempDir()
	envSvc := NewEnvironmentService()

	// 1. Save public variables for 'staging'
	publicEnv := types.EnvironmentDefinition{
		Name: "staging",
		Variables: []types.KeyValue{
			{Key: "API_HOST", Value: "https://staging.api.com", Enabled: true},
			{Key: "TIMEOUT", Value: "10000", Enabled: true},
			{Key: "API_SECRET", Value: "public-placeholder", Enabled: true},
		},
	}
	if err := envSvc.SaveEnvironment(tempDir, publicEnv, false); err != nil {
		t.Fatalf("failed to save public env: %v", err)
	}

	// 2. Save secret variables for 'staging' (API_SECRET should override public)
	secretEnv := types.EnvironmentDefinition{
		Name: "staging",
		Variables: []types.KeyValue{
			{Key: "API_SECRET", Value: "super-secret-key-999", Enabled: true},
			{Key: "PRIVATE_KEY", Value: "rsa-private-content", Enabled: true},
		},
	}
	if err := envSvc.SaveEnvironment(tempDir, secretEnv, true); err != nil {
		t.Fatalf("failed to save secret env: %v", err)
	}

	// 3. List environments and verify merged results
	envs, err := envSvc.ListEnvironments(tempDir)
	if err != nil {
		t.Fatalf("failed to list environments: %v", err)
	}

	if len(envs) != 1 {
		t.Fatalf("expected 1 environment, got %d", len(envs))
	}

	staging := envs[0]
	if staging.Name != "staging" {
		t.Errorf("expected name 'staging', got '%s'", staging.Name)
	}

	// Staging should have 4 merged variables (API_HOST, TIMEOUT, API_SECRET [overridden], PRIVATE_KEY)
	if len(staging.Variables) != 4 {
		t.Fatalf("expected 4 merged variables, got %d: %+v", len(staging.Variables), staging.Variables)
	}

	var secretValue string
	for _, v := range staging.Variables {
		if v.Key == "API_SECRET" {
			secretValue = v.Value
		}
	}
	if secretValue != "super-secret-key-999" {
		t.Errorf("expected secret override 'super-secret-key-999', got '%s'", secretValue)
	}

	// 4. Delete environment
	if err := envSvc.DeleteEnvironment(tempDir, "staging"); err != nil {
		t.Fatalf("failed to delete environment: %v", err)
	}

	envsAfterDelete, err := envSvc.ListEnvironments(tempDir)
	if err != nil {
		t.Fatalf("failed to list envs after delete: %v", err)
	}
	if len(envsAfterDelete) != 0 {
		t.Errorf("expected 0 envs after delete, got %d", len(envsAfterDelete))
	}
}

func TestEnvironmentService_SecretSplitting(t *testing.T) {
	tempDir := t.TempDir()
	envSvc := NewEnvironmentService()

	// Environment with mixed public and secret variables
	mixedEnv := types.EnvironmentDefinition{
		Name: "production",
		Variables: []types.KeyValue{
			{Key: "API_URL", Value: "https://api.prod.com", Enabled: true, Secret: false},
			{Key: "DB_PASS", Value: "super_secret_db_pass", Enabled: true, Secret: true},
			{Key: "PUBLIC_KEY", Value: "pub_12345", Enabled: true, Secret: false},
			{Key: "PRIVATE_KEY", Value: "priv_67890", Enabled: true, Secret: true},
		},
	}

	// Save environment with isSecret=false (should partition automatically)
	if err := envSvc.SaveEnvironment(tempDir, mixedEnv, false); err != nil {
		t.Fatalf("failed to save mixed env: %v", err)
	}

	// Load environments and verify that secret flags are preserved
	envs, err := envSvc.ListEnvironments(tempDir)
	if err != nil {
		t.Fatalf("failed to list environments: %v", err)
	}
	if len(envs) != 1 {
		t.Fatalf("expected 1 env, got %d", len(envs))
	}

	prod := envs[0]
	var foundDBPass, foundAPIUrl bool
	for _, kv := range prod.Variables {
		if kv.Key == "DB_PASS" {
			foundDBPass = true
			if !kv.Secret {
				t.Errorf("expected DB_PASS to have Secret: true")
			}
			if kv.Value != "super_secret_db_pass" {
				t.Errorf("unexpected DB_PASS value: %s", kv.Value)
			}
		}
		if kv.Key == "API_URL" {
			foundAPIUrl = true
			if kv.Secret {
				t.Errorf("expected API_URL to have Secret: false")
			}
		}
	}

	if !foundDBPass || !foundAPIUrl {
		t.Errorf("missing expected variables in loaded env: foundDBPass=%v, foundAPIUrl=%v", foundDBPass, foundAPIUrl)
	}
}

func TestEnvironmentService_SecretFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping POSIX file mode assertions on Windows")
	}

	tempDir := t.TempDir()
	envSvc := NewEnvironmentService()

	env := types.EnvironmentDefinition{
		Name: "secure-env",
		Variables: []types.KeyValue{
			{Key: "PUBLIC_VAR", Value: "123", Enabled: true, Secret: false},
			{Key: "SECRET_VAR", Value: "secret-token-xyz", Enabled: true, Secret: true},
		},
	}

	if err := envSvc.SaveEnvironment(tempDir, env, false); err != nil {
		t.Fatalf("SaveEnvironment failed: %v", err)
	}

	envDir := filepath.Join(tempDir, PebbleDir, EnvironmentsDir)

	// Public env file should be 0644
	publicFile := filepath.Join(envDir, "secure-env.env.json")
	pubFi, err := os.Stat(publicFile)
	if err != nil {
		t.Fatalf("stat public env file failed: %v", err)
	}
	if perm := pubFi.Mode().Perm(); perm != 0644 {
		t.Errorf("public env file mode = %o; want 0644", perm)
	}

	// Secret env file MUST be 0600
	secretFile := filepath.Join(envDir, "secure-env.secret.env.json")
	secFi, err := os.Stat(secretFile)
	if err != nil {
		t.Fatalf("stat secret env file failed: %v", err)
	}
	if perm := secFi.Mode().Perm(); perm != 0600 {
		t.Errorf("secret env file mode = %o; want 0600", perm)
	}
}
