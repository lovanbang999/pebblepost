package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildVarMap_Precedence(t *testing.T) {
	// Create a temp env file
	dir := t.TempDir()
	envFile := filepath.Join(dir, "test.env")
	_ = os.WriteFile(envFile, []byte("FILE_KEY=from-file\nSHARED=from-file\n"), 0644)

	// Set PEBBLE_VAR_* env var
	t.Setenv("PEBBLE_VAR_ENV_KEY", "from-env")
	t.Setenv("PEBBLE_VAR_SHARED", "from-pebble-env")

	baseVars := map[string]string{
		"BASE_KEY": "from-base",
		"SHARED":   "from-base",
	}
	extraVars := map[string]string{
		"VAR_KEY": "from-var",
		"SHARED":  "from-var-flag",
	}

	result := BuildVarMap(baseVars, extraVars, envFile, nil, nil)

	// 1. Base env should be present
	if result["BASE_KEY"] != "from-base" {
		t.Errorf("BASE_KEY: expected 'from-base', got %q", result["BASE_KEY"])
	}
	// 2. env-file should be present
	if result["FILE_KEY"] != "from-file" {
		t.Errorf("FILE_KEY: expected 'from-file', got %q", result["FILE_KEY"])
	}
	// 3. PEBBLE_VAR_* should override env-file
	if result["ENV_KEY"] != "from-env" {
		t.Errorf("ENV_KEY: expected 'from-env', got %q", result["ENV_KEY"])
	}
	// 4. --var should override everything (SHARED wins with "from-var-flag")
	if result["SHARED"] != "from-var-flag" {
		t.Errorf("SHARED precedence: expected 'from-var-flag', got %q", result["SHARED"])
	}
	// 5. --var top-level key
	if result["VAR_KEY"] != "from-var" {
		t.Errorf("VAR_KEY: expected 'from-var', got %q", result["VAR_KEY"])
	}
}

func TestBuildVarMap_NoEnvFile(t *testing.T) {
	base := map[string]string{"KEY": "base"}
	result := BuildVarMap(base, nil, "", nil, nil)
	if result["KEY"] != "base" {
		t.Errorf("expected 'base', got %q", result["KEY"])
	}
}

func TestBuildVarMap_MissingEnvFileIgnored(t *testing.T) {
	base := map[string]string{"KEY": "base"}
	// Should not panic or error if env file doesn't exist
	result := BuildVarMap(base, nil, "/nonexistent/path.env", nil, nil)
	if result["KEY"] != "base" {
		t.Errorf("expected base var to remain when env file missing, got %q", result["KEY"])
	}
}

func TestParseVarFlags(t *testing.T) {
	tests := []struct {
		input   []string
		want    map[string]string
		wantErr bool
	}{
		{
			input: []string{"KEY=value", "HOST=localhost"},
			want:  map[string]string{"KEY": "value", "HOST": "localhost"},
		},
		{
			input: []string{"URL=https://api.example.com/v1?q=1"},
			want:  map[string]string{"URL": "https://api.example.com/v1?q=1"},
		},
		{
			input:   []string{"INVALID"},
			wantErr: true,
		},
		{
			input:   []string{"=value"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		got, err := ParseVarFlags(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("input %v: expected error but got nil", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("input %v: unexpected error: %v", tt.input, err)
			continue
		}
		for k, want := range tt.want {
			if got[k] != want {
				t.Errorf("key %q: expected %q, got %q", k, want, got[k])
			}
		}
	}
}

func TestLoadEnvFile(t *testing.T) {
	dir := t.TempDir()
	content := `
# this is a comment
KEY1=value1
KEY2=value2
EMPTY_LINE=

# another comment
SECRET_TOKEN=my-secret-value
`
	path := filepath.Join(dir, "vars.env")
	_ = os.WriteFile(path, []byte(content), 0644)

	vars, secrets, err := loadEnvFile(path, nil)
	if err != nil {
		t.Fatalf("loadEnvFile error: %v", err)
	}

	if vars["KEY1"] != "value1" {
		t.Errorf("KEY1: expected 'value1', got %q", vars["KEY1"])
	}
	if vars["KEY2"] != "value2" {
		t.Errorf("KEY2: expected 'value2', got %q", vars["KEY2"])
	}
	if vars["EMPTY_LINE"] != "" {
		t.Errorf("EMPTY_LINE: expected '', got %q", vars["EMPTY_LINE"])
	}
	// SECRET_TOKEN should be in secrets list
	found := false
	for _, s := range secrets {
		if s == "my-secret-value" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected SECRET_TOKEN value in secrets list")
	}
}
