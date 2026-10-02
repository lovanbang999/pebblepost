package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"pebblepost/internal/types"
)

func TestMarshalStable_FormattingAndKeyOrder(t *testing.T) {
	req := types.RequestDefinition{
		Schema:        "https://pebblepost.dev/schemas/v1/request.json",
		SchemaVersion: 1,
		Version:       "1.0",
		Name:          "Golden Request",
		Order:         1,
		Method:        "GET",
		URL:           "https://api.example.com/items",
		Auth: types.AuthDefinition{
			Type: "none",
		},
		Body: types.BodyDefinition{
			Type: "none",
		},
		Scripts: types.ScriptDefinition{},
		Settings: types.SettingDefinition{
			FollowRedirects: true,
			VerifySSL:       true,
			TimeoutMs:       30000,
		},
	}

	data, err := MarshalStable(req)
	if err != nil {
		t.Fatalf("MarshalStable failed: %v", err)
	}

	// Must end with a single newline
	if len(data) == 0 || data[len(data)-1] != '\n' {
		t.Errorf("expected trailing newline")
	}

	expectedGolden := `{
  "$schema": "https://pebblepost.dev/schemas/v1/request.json",
  "schemaVersion": 1,
  "version": "1.0",
  "name": "Golden Request",
  "order": 1,
  "method": "GET",
  "url": "https://api.example.com/items",
  "auth": {
    "type": "none"
  },
  "body": {
    "type": "none"
  },
  "scripts": {},
  "settings": {
    "followRedirects": true,
    "verifySSL": true,
    "timeoutMs": 30000
  }
}
`
	if string(data) != expectedGolden {
		t.Errorf("MarshalStable output does not match golden format.\nGot:\n%s\nExpected:\n%s", string(data), expectedGolden)
	}
}

func TestMarshalStable_RoundTripDeterminism(t *testing.T) {
	original := types.RequestDefinition{
		Schema:        "https://pebblepost.dev/schemas/v1/request.json",
		SchemaVersion: 1,
		Version:       "1.0",
		Name:          "Deterministic Request",
		Method:        "POST",
		URL:           "https://api.example.com/login",
		Headers: []types.KeyValue{
			{Key: "Content-Type", Value: "application/json", Enabled: true},
		},
		Auth: types.AuthDefinition{Type: "bearer", Token: "secret123"},
		Body: types.BodyDefinition{Type: "json", Raw: `{"user":"admin"}`},
		Settings: types.SettingDefinition{
			FollowRedirects: false,
			VerifySSL:       true,
			TimeoutMs:       5000,
		},
	}

	pass1, err := MarshalStable(original)
	if err != nil {
		t.Fatalf("first marshal failed: %v", err)
	}

	var parsed types.RequestDefinition
	if err := json.Unmarshal(pass1, &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	pass2, err := MarshalStable(parsed)
	if err != nil {
		t.Fatalf("second marshal failed: %v", err)
	}

	if string(pass1) != string(pass2) {
		t.Errorf("round trip serialization is not deterministic.\nPass 1:\n%s\nPass 2:\n%s", string(pass1), string(pass2))
	}
}

func TestWriteFileStable_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "sub", "deep", "saved.pebble.json")

	env := types.EnvironmentDefinition{
		SchemaVersion: 1,
		Name:          "prod",
		Variables: []types.KeyValue{
			{Key: "API_URL", Value: "https://api.production.com", Enabled: true},
		},
	}

	if err := WriteFileStable(filePath, env); err != nil {
		t.Fatalf("WriteFileStable failed: %v", err)
	}

	// Verify temp file does not remain
	tmpFile := filePath + ".tmp"
	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Errorf("temporary file %s was not cleaned up", tmpFile)
	}

	// Verify target file was written properly
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}

	expectedGolden := `{
  "schemaVersion": 1,
  "name": "prod",
  "variables": [
    {
      "key": "API_URL",
      "value": "https://api.production.com",
      "enabled": true
    }
  ]
}
`
	if string(content) != expectedGolden {
		t.Errorf("WriteFileStable content mismatch.\nGot:\n%s\nExpected:\n%s", string(content), expectedGolden)
	}
}
