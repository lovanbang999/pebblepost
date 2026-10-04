package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDataBytes_CSV(t *testing.T) {
	csvContent := `email,role,isActive
alice@example.com,admin,true
bob@example.com,user,false
`
	rows, err := ParseDataBytes([]byte(csvContent), "users.csv")
	if err != nil {
		t.Fatalf("unexpected error parsing CSV: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	if rows[0]["email"] != "alice@example.com" || rows[0]["role"] != "admin" || rows[0]["isActive"] != "true" {
		t.Errorf("unexpected first row: %+v", rows[0])
	}

	if rows[1]["email"] != "bob@example.com" || rows[1]["role"] != "user" || rows[1]["isActive"] != "false" {
		t.Errorf("unexpected second row: %+v", rows[1])
	}
}

func TestParseDataBytes_TSV(t *testing.T) {
	tsvContent := "id\tname\tscore\n1\tCharlie\t95\n2\tDave\t88\n"
	rows, err := ParseDataBytes([]byte(tsvContent), "scores.tsv")
	if err != nil {
		t.Fatalf("unexpected error parsing TSV: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	if rows[0]["name"] != "Charlie" || rows[0]["score"] != "95" {
		t.Errorf("unexpected first row: %+v", rows[0])
	}
}

func TestParseDataBytes_JSON(t *testing.T) {
	jsonContent := `[
		{"email": "carol@example.com", "tier": "gold", "limit": 100},
		{"email": "dan@example.com", "tier": "silver", "limit": 50}
	]`
	rows, err := ParseDataBytes([]byte(jsonContent), "data.json")
	if err != nil {
		t.Fatalf("unexpected error parsing JSON: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	if rows[0]["email"] != "carol@example.com" || rows[0]["tier"] != "gold" {
		t.Errorf("unexpected first row: %+v", rows[0])
	}
}

func TestParseDataBytes_JSONWrapped(t *testing.T) {
	jsonContent := `{"items": [{"id": "1", "status": "ok"}]}`
	rows, err := ParseDataBytes([]byte(jsonContent), "wrapped.json")
	if err != nil {
		t.Fatalf("unexpected error parsing wrapped JSON: %v", err)
	}

	if len(rows) != 1 || rows[0]["id"] != "1" {
		t.Errorf("unexpected row result: %+v", rows)
	}
}

func TestParseDataFile_Success(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "test.csv")
	err := os.WriteFile(csvPath, []byte("username,token\nusr1,tok1\nusr2,tok2\n"), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	rows, err := ParseDataFile(csvPath)
	if err != nil {
		t.Fatalf("failed to parse data file: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
}

func TestParseDataFile_Errors(t *testing.T) {
	// Empty path
	if _, err := ParseDataFile(""); err == nil {
		t.Error("expected error on empty file path")
	}

	// Non-existent file
	if _, err := ParseDataFile("/path/does/not/exist.csv"); err == nil {
		t.Error("expected error on non-existent file")
	}

	// Empty bytes
	if _, err := ParseDataBytes([]byte("   "), "empty.csv"); err == nil {
		t.Error("expected error on empty content")
	}

	// CSV with headers only
	if _, err := ParseDataBytes([]byte("col1,col2\n"), "no_data.csv"); err == nil {
		t.Error("expected error on header-only CSV")
	}
}

func TestComputeIterations(t *testing.T) {
	// Case 1: Data rows = 5, requested = 0 -> default to 5
	if it := ComputeIterations(0, 5); it != 5 {
		t.Errorf("expected 5, got %d", it)
	}

	// Case 2: Data rows = 5, requested = 3 -> 3
	if it := ComputeIterations(3, 5); it != 3 {
		t.Errorf("expected 3, got %d", it)
	}

	// Case 3: Data rows = 5, requested = 10 -> capped at 5
	if it := ComputeIterations(10, 5); it != 5 {
		t.Errorf("expected 5, got %d", it)
	}

	// Case 4: No data rows, requested = 0 -> 1
	if it := ComputeIterations(0, 0); it != 1 {
		t.Errorf("expected 1, got %d", it)
	}

	// Case 5: No data rows, requested = 4 -> 4
	if it := ComputeIterations(4, 0); it != 4 {
		t.Errorf("expected 4, got %d", it)
	}
}
