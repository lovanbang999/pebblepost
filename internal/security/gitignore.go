package security

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const secretEnvPattern = "*.secret.env.json"

// GitignoreStatus describes whether the workspace root's .gitignore covers
// secret environment files.
type GitignoreStatus int

const (
	// GitignoreOK means the *.secret.env.json rule is already present.
	GitignoreOK GitignoreStatus = iota
	// GitignoreMissing means the .gitignore file does not exist.
	GitignoreMissing
	// GitignoreNoRule means the .gitignore exists but lacks the required rule.
	GitignoreNoRule
)

// CheckGitignore inspects <rootPath>/.gitignore and reports whether it
// contains a rule for *.secret.env.json. It does NOT write anything.
func CheckGitignore(rootPath string) (GitignoreStatus, error) {
	gitignorePath := filepath.Join(rootPath, ".gitignore")
	f, err := os.Open(gitignorePath)
	if err != nil {
		if os.IsNotExist(err) {
			return GitignoreMissing, nil
		}
		return GitignoreMissing, fmt.Errorf("failed to read .gitignore: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == secretEnvPattern ||
			line == "/"+secretEnvPattern ||
			strings.HasSuffix(line, "/"+secretEnvPattern) {
			return GitignoreOK, nil
		}
	}
	return GitignoreNoRule, scanner.Err()
}

// EnsureGitignoreEntry appends the *.secret.env.json rule to
// <rootPath>/.gitignore (creating the file if necessary).
// Call this ONLY after obtaining explicit user confirmation.
func EnsureGitignoreEntry(rootPath string) error {
	gitignorePath := filepath.Join(rootPath, ".gitignore")
	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open .gitignore for writing: %w", err)
	}
	defer f.Close()

	_, err = fmt.Fprintf(f, "\n# PebblePost: never commit secret environment files\n%s\n", secretEnvPattern)
	return err
}
