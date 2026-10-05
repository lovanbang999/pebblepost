package gitclient

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"pebblepost/internal/types"
)

// initTestRepo creates a temporary directory initialized with git and user identity.
func initTestRepo(t *testing.T) (string, *Service) {
	t.Helper()
	dir := t.TempDir()

	cmd := exec.Command("git", "init", "-b", "main", dir)
	if err := cmd.Run(); err != nil {
		// Fallback for older git versions that don't support -b in init
		if err := exec.Command("git", "init", dir).Run(); err != nil {
			t.Fatalf("failed to git init: %v", err)
		}
		_ = exec.Command("git", "-C", dir, "checkout", "-b", "main").Run()
	}

	_ = exec.Command("git", "-C", dir, "config", "user.name", "Test User").Run()
	_ = exec.Command("git", "-C", dir, "config", "user.email", "test@example.com").Run()

	svc := NewService()
	return dir, svc
}

func TestService_NonRepo(t *testing.T) {
	dir := t.TempDir()
	svc := NewService()

	status, err := svc.Status(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error for non-repo: %v", err)
	}
	if status.IsRepo {
		t.Errorf("expected isRepo=false for empty temp dir, got true")
	}
}

func TestService_CleanRepoAndCommit(t *testing.T) {
	dir, svc := initTestRepo(t)
	ctx := context.Background()

	// Initial commit
	readmePath := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test Repo\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := svc.Stage(ctx, dir, []string{"README.md"}); err != nil {
		t.Fatalf("failed to stage: %v", err)
	}

	if err := svc.Commit(ctx, dir, "initial commit"); err != nil {
		t.Fatalf("failed to commit: %v", err)
	}

	status, err := svc.Status(ctx, dir)
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}
	if !status.IsRepo {
		t.Errorf("expected isRepo=true")
	}
	if status.Dirty {
		t.Errorf("expected dirty=false after commit, got true")
	}
	if status.Branch != "main" {
		t.Errorf("expected branch main, got %q", status.Branch)
	}

	logs, err := svc.Log(ctx, dir, 5)
	if err != nil {
		t.Fatalf("failed to get log: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(logs))
	}
	if logs[0].Subject != "initial commit" {
		t.Errorf("expected subject 'initial commit', got %q", logs[0].Subject)
	}
}

func TestService_DirtyStatusAndUnstage(t *testing.T) {
	dir, svc := initTestRepo(t)
	ctx := context.Background()

	// Commit initial file
	f := filepath.Join(dir, "req.json")
	_ = os.WriteFile(f, []byte("v1"), 0644)
	_ = svc.Stage(ctx, dir, []string{"req.json"})
	_ = svc.Commit(ctx, dir, "init")

	// Modify file
	_ = os.WriteFile(f, []byte("v2"), 0644)

	status, err := svc.Status(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Dirty {
		t.Errorf("expected dirty=true after modifying file")
	}
	if len(status.Files) == 0 {
		t.Fatalf("expected at least 1 modified file")
	}

	// Stage modified file
	if err := svc.Stage(ctx, dir, []string{"req.json"}); err != nil {
		t.Fatalf("failed to stage: %v", err)
	}

	// Unstage file
	if err := svc.Unstage(ctx, dir, []string{"req.json"}); err != nil {
		t.Fatalf("failed to unstage: %v", err)
	}

	statusAfter, _ := svc.Status(ctx, dir)
	for _, f := range statusAfter.Files {
		if f.Path == "req.json" && f.Staged {
			t.Errorf("expected req.json to be unstaged")
		}
	}
}

func TestService_StageGuard_RejectsSecretFiles(t *testing.T) {
	dir, svc := initTestRepo(t)
	ctx := context.Background()

	secretFile := "prod.secret.env.json"
	_ = os.WriteFile(filepath.Join(dir, secretFile), []byte(`{"apiKey":"secret"}`), 0644)

	err := svc.Stage(ctx, dir, []string{secretFile})
	if err == nil {
		t.Fatalf("expected error staging secret file, got nil")
	}
	if err.Error() == "" {
		t.Errorf("expected informative error message")
	}
}

func TestService_StageGuard_RejectsGitignoredFiles(t *testing.T) {
	dir, svc := initTestRepo(t)
	ctx := context.Background()

	// Write .gitignore
	_ = os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("ignored.txt\n"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "ignored.txt"), []byte("data"), 0644)

	err := svc.Stage(ctx, dir, []string{"ignored.txt"})
	if err == nil {
		t.Fatalf("expected error staging gitignored file, got nil")
	}
}

func TestService_BranchesAndAutoStashOnCheckout(t *testing.T) {
	dir, svc := initTestRepo(t)
	ctx := context.Background()

	// Initial commit
	_ = os.WriteFile(filepath.Join(dir, "file.txt"), []byte("base"), 0644)
	_ = svc.Stage(ctx, dir, []string{"file.txt"})
	_ = svc.Commit(ctx, dir, "base commit")

	// Create feature branch
	if err := svc.CreateBranch(ctx, dir, "feat/login"); err != nil {
		t.Fatalf("failed to create branch: %v", err)
	}

	branches, err := svc.Branches(ctx, dir)
	if err != nil {
		t.Fatalf("failed to list branches: %v", err)
	}
	if len(branches) < 2 {
		t.Errorf("expected at least 2 branches, got %d", len(branches))
	}

	// Dirty the working copy on feat/login
	_ = os.WriteFile(filepath.Join(dir, "file.txt"), []byte("dirty work"), 0644)

	// Checkout main with dirty tree -> should auto-stash and restore
	resp, err := svc.Checkout(ctx, dir, "main")
	if err != nil {
		t.Fatalf("checkout failed: %v", err)
	}
	if !resp.Stashed {
		t.Errorf("expected stashed=true for dirty checkout")
	}

	// Verify working tree still has the dirty change restored
	data, _ := os.ReadFile(filepath.Join(dir, "file.txt"))
	if string(data) != "dirty work" {
		t.Errorf("expected dirty changes restored, got %q", string(data))
	}
}

func TestService_BranchNameValidation_ShellInjectionGuard(t *testing.T) {
	dir, svc := initTestRepo(t)
	ctx := context.Background()

	maliciousNames := []string{
		"feat;rm -rf /",
		"feat|cat /etc/passwd",
		"feat`touch pwn`",
		"feat$(whoami)",
	}

	for _, name := range maliciousNames {
		if err := svc.CreateBranch(ctx, dir, name); err == nil {
			t.Errorf("expected error for malicious branch name %q, got nil", name)
		}
		if _, err := svc.Checkout(ctx, dir, name); err == nil {
			t.Errorf("expected error for malicious checkout branch name %q, got nil", name)
		}
	}
}

func TestService_DiffRequest_FieldLevelDiff(t *testing.T) {
	dir, svc := initTestRepo(t)
	ctx := context.Background()

	relPath := "login.pebble.json"

	reqV1 := types.RequestDefinition{
		Name:   "Login V1",
		Method: "GET",
		URL:    "https://api.example.com/v1/auth",
		Body: types.BodyDefinition{
			Type: "json",
			Raw:  `{"user":"alice"}`,
		},
	}
	b1, _ := json.MarshalIndent(reqV1, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, relPath), b1, 0644)
	_ = svc.Stage(ctx, dir, []string{relPath})
	_ = svc.Commit(ctx, dir, "add login v1")

	// Update to V2
	reqV2 := types.RequestDefinition{
		Name:   "Login V2",
		Method: "POST",
		URL:    "https://api.example.com/v2/auth",
		Body: types.BodyDefinition{
			Type: "json",
			Raw:  `{"user":"alice","role":"admin"}`,
		},
	}
	b2, _ := json.MarshalIndent(reqV2, "", "  ")
	_ = os.WriteFile(filepath.Join(dir, relPath), b2, 0644)

	diffs, err := svc.DiffRequest(ctx, dir, relPath)
	if err != nil {
		t.Fatalf("failed to diff request: %v", err)
	}

	fieldMap := make(map[string]DiffEntry)
	for _, d := range diffs {
		fieldMap[d.Field] = d
	}

	if d, ok := fieldMap["name"]; !ok || d.Before != "Login V1" || d.After != "Login V2" {
		t.Errorf("name diff mismatch: %+v", d)
	}
	if d, ok := fieldMap["method"]; !ok || d.Before != "GET" || d.After != "POST" {
		t.Errorf("method diff mismatch: %+v", d)
	}
	if d, ok := fieldMap["url"]; !ok || d.Before != "https://api.example.com/v1/auth" || d.After != "https://api.example.com/v2/auth" {
		t.Errorf("url diff mismatch: %+v", d)
	}
	if _, ok := fieldMap["body"]; !ok {
		t.Errorf("expected body diff entry")
	}
}
