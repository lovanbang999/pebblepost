package security

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeJoin(t *testing.T) {
	root := t.TempDir()

	// Create a file outside root to be the symlink target.
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	_ = os.WriteFile(outsideFile, []byte("secret"), 0644)

	// Symlink inside root that points to outsideFile.
	symlink := filepath.Join(root, "escape-link")
	_ = os.Symlink(outsideFile, symlink)

	tests := []struct {
		name      string
		untrusted string
		wantErr   bool
	}{
		{"valid file", "foo/bar.pebble.json", false},
		{"valid nested", "a/b/c.pebble.json", false},
		{"current dir", ".", false},
		{"dotdot prefix", "../../etc/passwd", true},
		{"dotdot middle", "foo/../../etc/passwd", true},
		{"absolute unix", "/etc/passwd", true},
		{"absolute windows C drive", `C:\Windows\system32`, true},
		{"absolute windows UNC", `\\server\share`, true},
		{"windows CON", "CON", true},
		{"windows NUL with ext", "NUL.txt", true},
		{"windows COM1 in subdir", "a/COM1/b", true},
		{"windows LPT9", "LPT9", true},
		{"symlink escaping root", "escape-link", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SafeJoin(root, tt.untrusted)
			if (err != nil) != tt.wantErr {
				t.Errorf("SafeJoin(%q) error=%v, wantErr=%v", tt.untrusted, err, tt.wantErr)
			}
		})
	}
}

func TestSafeJoin_NewFileDoesNotExist(t *testing.T) {
	root := t.TempDir()
	// Parent dir doesn't exist yet — SafeJoin should still accept it.
	got, err := SafeJoin(root, "collections/new-request.pebble.json")
	if err != nil {
		t.Fatalf("unexpected error for non-existent new file: %v", err)
	}
	want := filepath.Join(root, "collections", "new-request.pebble.json")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSafeJoin_SymlinkedRoot(t *testing.T) {
	realTarget := t.TempDir()
	linkDir := t.TempDir()
	symlinkedRoot := filepath.Join(linkDir, "symlink-workspace")
	if err := os.Symlink(realTarget, symlinkedRoot); err != nil {
		t.Skip("symlinks not supported")
	}

	got, err := SafeJoin(symlinkedRoot, "foo/bar.pebble.json")
	if err != nil {
		t.Fatalf("unexpected error for symlinked root: %v", err)
	}
	want := filepath.Join(symlinkedRoot, "foo/bar.pebble.json")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestSafeJoin_EmptyRoot(t *testing.T) {
	_, err := SafeJoin("", "foo.json")
	if err == nil {
		t.Error("expected error for empty root")
	}
}

func TestSafeJoin_EmptyPath(t *testing.T) {
	_, err := SafeJoin("/tmp", "")
	if err == nil {
		t.Error("expected error for empty path")
	}
}

func TestSafeAbsolute(t *testing.T) {
	root := t.TempDir()

	// Create a real file inside root.
	inside := filepath.Join(root, "file.pebble.json")
	_ = os.WriteFile(inside, []byte("{}"), 0644)

	// Outside file for symlink target.
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "secret.txt")
	_ = os.WriteFile(outsideFile, []byte("secret"), 0644)

	// Symlink inside root pointing outside.
	symlink := filepath.Join(root, "bad-link")
	_ = os.Symlink(outsideFile, symlink)

	tests := []struct {
		name         string
		absolutePath string
		wantErr      bool
	}{
		{"valid inside root", inside, false},
		{"outside root", filepath.Join(outside, "other.json"), true},
		{"symlink escape", symlink, true},
		{"dotdot in absolute", root + "/a/../../../etc/passwd", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := SafeAbsolute(root, tt.absolutePath)
			if (err != nil) != tt.wantErr {
				t.Errorf("SafeAbsolute(%q) error=%v, wantErr=%v", tt.absolutePath, err, tt.wantErr)
			}
		})
	}
}

func TestValidateWorkspaceRoot(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"valid dir", tempDir, false},
		{"contains dotdot", tempDir + "/../bad", true},
		{"non-existent", filepath.Join(tempDir, "does-not-exist"), true},
		{"windows CON", filepath.Join(tempDir, "CON"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWorkspaceRoot(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWorkspaceRoot(%q) error=%v, wantErr=%v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestRejectTraversal(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"normal file", "users.pebble.json", false},
		{"normal nested", "auth/login.pebble.json", false},
		{"contains dotdot", "../secret.json", true},
		{"contains dotdot in middle", "foo/../../secret.json", true},
		{"contains null byte", "users\x00.json", true},
		{"windows NUL device", "NUL.txt", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RejectTraversal(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("RejectTraversal(%q) error=%v, wantErr=%v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestValidateSafeIdentifier(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid simple", "my-env", false},
		{"valid with underscore", "dev_env_1", false},
		{"empty string", "", true},
		{"whitespace only", "   ", true},
		{"slash", "dev/test", true},
		{"backslash", `dev\test`, true},
		{"dot", ".", true},
		{"dotdot", "..", true},
		{"windows reserved", "CON", true},
		{"windows reserved aux", "aux", true},
		{"newline", "test\nname", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSafeIdentifier(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSafeIdentifier(%q) error=%v, wantErr=%v", tt.id, err, tt.wantErr)
			}
		})
	}
}

func TestValidateFileExtension(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		allowedExts []string
		wantErr     bool
	}{
		{"valid pebble.json", "request.pebble.json", []string{".pebble.json"}, false},
		{"valid uppercase extension", "request.PEBBLE.JSON", []string{".pebble.json"}, false},
		{"invalid extension", "request.json", []string{".pebble.json"}, true},
		{"multiple allowed extensions", "dev.env.json", []string{".env.json", ".secret.env.json"}, false},
		{"multiple allowed extensions secret", "dev.secret.env.json", []string{".env.json", ".secret.env.json"}, false},
		{"exe not allowed", "evil.exe", []string{".pebble.json"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFileExtension(tt.path, tt.allowedExts...)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateFileExtension(%q) error=%v, wantErr=%v", tt.path, err, tt.wantErr)
			}
		})
	}
}
