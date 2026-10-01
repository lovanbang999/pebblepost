package workspace

import (
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
