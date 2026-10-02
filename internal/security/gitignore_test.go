package security

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckGitignore_Missing(t *testing.T) {
	root := t.TempDir()
	status, err := CheckGitignore(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != GitignoreMissing {
		t.Errorf("expected GitignoreMissing, got %v", status)
	}
}

func TestCheckGitignore_NoRule(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, ".gitignore"), []byte("node_modules/\ndist/\n"), 0644)

	status, err := CheckGitignore(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != GitignoreNoRule {
		t.Errorf("expected GitignoreNoRule, got %v", status)
	}
}

func TestCheckGitignore_OK(t *testing.T) {
	patterns := []string{
		"*.secret.env.json",
		"/*.secret.env.json",
	}

	for _, pattern := range patterns {
		t.Run(pattern, func(t *testing.T) {
			root := t.TempDir()
			content := "node_modules/\n" + pattern + "\n"
			_ = os.WriteFile(filepath.Join(root, ".gitignore"), []byte(content), 0644)

			status, err := CheckGitignore(root)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if status != GitignoreOK {
				t.Errorf("pattern %q: expected GitignoreOK, got %v", pattern, status)
			}
		})
	}
}

func TestEnsureGitignoreEntry(t *testing.T) {
	root := t.TempDir()

	// Start with an existing .gitignore that has no rule.
	_ = os.WriteFile(filepath.Join(root, ".gitignore"), []byte("node_modules/\n"), 0644)

	if err := EnsureGitignoreEntry(root); err != nil {
		t.Fatalf("EnsureGitignoreEntry error: %v", err)
	}

	// Now CheckGitignore should return OK.
	status, err := CheckGitignore(root)
	if err != nil {
		t.Fatalf("CheckGitignore error after ensure: %v", err)
	}
	if status != GitignoreOK {
		t.Errorf("expected GitignoreOK after adding entry, got %v", status)
	}

	// Content should contain the pattern.
	data, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
	if !strings.Contains(string(data), "*.secret.env.json") {
		t.Error("expected *.secret.env.json in .gitignore, not found")
	}
}

func TestEnsureGitignoreEntry_CreatesFile(t *testing.T) {
	root := t.TempDir()
	// No .gitignore exists yet.
	if err := EnsureGitignoreEntry(root); err != nil {
		t.Fatalf("EnsureGitignoreEntry error: %v", err)
	}

	status, _ := CheckGitignore(root)
	if status != GitignoreOK {
		t.Error("expected GitignoreOK after creating .gitignore")
	}
}
