// Package security provides shared security primitives for PebblePost.
package security

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateWorkspaceRoot performs basic sanity checks on a workspace root path
// supplied by an untrusted caller:
//   - Must not contain ".." components
//   - Must not contain Windows reserved device names
//   - Must be an existing directory
func ValidateWorkspaceRoot(rootPath string) error {
	normalized := strings.ReplaceAll(rootPath, `\`, "/")
	for _, part := range strings.Split(normalized, "/") {
		if part == ".." {
			return fmt.Errorf("path traversal detected")
		}
	}
	if err := checkWindowsReservedNames(rootPath); err != nil {
		return err
	}
	fi, err := os.Stat(rootPath)
	if err != nil {
		return fmt.Errorf("workspace path does not exist: %w", err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("workspace path is not a directory")
	}
	return nil
}

// RejectTraversal is a lightweight fallback guard used when the caller does not
// supply a workspace root. It rejects any path that contains ".." components,
// null bytes, or Windows reserved device names.
func RejectTraversal(path string) error {
	if strings.Contains(path, "\x00") {
		return fmt.Errorf("null byte in path")
	}
	normalized := strings.ReplaceAll(path, `\`, "/")
	for _, part := range strings.Split(normalized, "/") {
		if part == ".." {
			return fmt.Errorf("path traversal detected")
		}
	}
	return checkWindowsReservedNames(filepath.Base(path))
}

// windowsReservedNames lists device names that are special on Windows.
// Checked on all platforms so that files created on Linux cannot cause
// problems when the workspace is later opened on Windows.
var windowsReservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true,
	"COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true,
	"LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// SafeJoin resolves a caller-supplied relative path and verifies it stays
// inside root. It rejects:
//   - bare absolute paths ("/etc/passwd")
//   - Windows-style absolute paths ("C:\...", "\\server\share")
//   - ".." components in any position
//   - Windows reserved device names in any component (CON, NUL, COM1, …)
//   - symlinks whose real target is outside root
//
// When the target file does not yet exist (save new file), the parent
// directory is resolved instead. Returns the cleaned absolute path on success.
func SafeJoin(root, untrusted string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("workspace root cannot be empty")
	}
	if untrusted == "" {
		return "", fmt.Errorf("path cannot be empty")
	}

	// Reject OS-native absolute paths.
	if filepath.IsAbs(untrusted) {
		return "", fmt.Errorf("absolute paths are not allowed: %s", untrusted)
	}

	// Reject Windows drive letters and UNC paths on any OS.
	if len(untrusted) >= 2 && (untrusted[1] == ':' ||
		(untrusted[0] == '\\' && len(untrusted) >= 2 && untrusted[1] == '\\')) {
		return "", fmt.Errorf("absolute paths are not allowed: %s", untrusted)
	}

	// Normalize backslashes and split to inspect every component.
	normalized := strings.ReplaceAll(untrusted, `\`, "/")
	for _, part := range strings.Split(normalized, "/") {
		if part == ".." {
			return "", fmt.Errorf("path traversal detected: %s", untrusted)
		}
	}

	// Reject Windows reserved names in any component.
	if err := checkWindowsReservedNames(untrusted); err != nil {
		return "", err
	}

	candidate := filepath.Join(root, untrusted)

	return validateAbsoluteUnderRoot(root, candidate)
}

// SafeAbsolute validates that an already-absolute path stays inside root
// after symlink resolution. Used when the caller supplies a full path
// (e.g. paths returned by the tree scanner).
func SafeAbsolute(root, absolutePath string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("workspace root cannot be empty")
	}
	if absolutePath == "" {
		return "", fmt.Errorf("path cannot be empty")
	}

	// Reject paths with ".." that could escape even from an absolute form.
	normalized := strings.ReplaceAll(absolutePath, `\`, "/")
	for _, part := range strings.Split(normalized, "/") {
		if part == ".." {
			return "", fmt.Errorf("path traversal detected: %s", absolutePath)
		}
	}

	if err := checkWindowsReservedNames(absolutePath); err != nil {
		return "", err
	}

	return validateAbsoluteUnderRoot(root, absolutePath)
}

// validateAbsoluteUnderRoot resolves symlinks and ensures candidate is
// rooted inside root. When the candidate does not exist yet, the parent
// directory is resolved.
func validateAbsoluteUnderRoot(root, candidate string) (string, error) {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		realRoot = filepath.Clean(root)
	}

	realCandidate, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		// File/dir does not exist yet; resolve the parent instead.
		parent := filepath.Dir(candidate)
		realParent, err2 := filepath.EvalSymlinks(parent)
		if err2 != nil {
			realParent = filepath.Clean(parent)
		}
		realCandidate = filepath.Join(realParent, filepath.Base(candidate))
	}

	relPath, err := filepath.Rel(realRoot, realCandidate)
	if err != nil {
		return "", fmt.Errorf("path escapes workspace root: %s", candidate)
	}
	sep := string(filepath.Separator)
	if relPath == ".." || strings.HasPrefix(relPath, ".."+sep) {
		return "", fmt.Errorf("path escapes workspace root: %s", candidate)
	}

	return realCandidate, nil
}

// checkWindowsReservedNames returns an error if any path component is a
// Windows reserved device name. Checked cross-platform for portability.
func checkWindowsReservedNames(path string) error {
	normalized := strings.ReplaceAll(path, `\`, "/")
	for _, part := range strings.Split(normalized, "/") {
		if part == "" || part == "." {
			continue
		}
		// Strip extension: "CON.txt" is still a reserved name.
		stem := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if windowsReservedNames[stem] {
			return fmt.Errorf("windows reserved name not allowed: %s", part)
		}
	}
	return nil
}

// ValidateSafeIdentifier checks that a user-supplied name (request name,
// environment name, folder name) is safe for filesystem operations:
//   - Not empty
//   - No path separators ('/' or '\')
//   - No traversal ("..")
//   - No null bytes or control characters
//   - Not a Windows reserved device name
func ValidateSafeIdentifier(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("name cannot be empty")
	}
	if strings.ContainsAny(name, "/\\\x00\r\n\t") {
		return fmt.Errorf("name contains invalid characters: %q", name)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("invalid name: %q", name)
	}
	return checkWindowsReservedNames(name)
}

// ValidateFileExtension verifies that path has one of the allowed extensions.
func ValidateFileExtension(path string, allowedExts ...string) error {
	lower := strings.ToLower(path)
	for _, ext := range allowedExts {
		if strings.HasSuffix(lower, strings.ToLower(ext)) {
			return nil
		}
	}
	return fmt.Errorf("file extension not allowed for %q (permitted: %s)", path, strings.Join(allowedExts, ", "))
}

