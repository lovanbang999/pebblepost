package gitclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"pebblepost/internal/types"
)

const defaultGitTimeout = 30 * time.Second

// Service handles git operations inside a workspace.
type Service struct{}

// NewService creates a new git service.
func NewService() *Service {
	return &Service{}
}

// runGit executes a git command safely with explicit argument lists.
// It never invokes a shell string and only runs within workspaceRoot.
func (s *Service) runGit(ctx context.Context, workspaceRoot string, args ...string) (string, error) {
	if workspaceRoot == "" {
		return "", errors.New("workspace root cannot be empty")
	}

	cCtx, cancel := context.WithTimeout(ctx, defaultGitTimeout)
	defer cancel()

	cmdArgs := append([]string{"-C", workspaceRoot}, args...)
	cmd := exec.CommandContext(cCtx, "git", cmdArgs...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		errOutput := strings.TrimSpace(stderr.String())
		if errOutput == "" {
			errOutput = strings.TrimSpace(stdout.String())
		}
		if errOutput == "" {
			errOutput = err.Error()
		}
		return "", errors.New(errOutput)
	}

	return stdout.String(), nil
}

// IsRepo checks if the workspaceRoot is inside a git repository work tree.
func (s *Service) IsRepo(ctx context.Context, workspaceRoot string) bool {
	out, err := s.runGit(ctx, workspaceRoot, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

// Init initializes a new git repository in workspaceRoot.
func (s *Service) Init(ctx context.Context, workspaceRoot string) error {
	_, err := s.runGit(ctx, workspaceRoot, "init")
	return err
}

// Status returns the comprehensive git status of the workspace repository.
func (s *Service) Status(ctx context.Context, workspaceRoot string) (*GitStatus, error) {
	if !s.IsRepo(ctx, workspaceRoot) {
		return &GitStatus{IsRepo: false}, nil
	}

	res := &GitStatus{
		IsRepo: true,
		Files:  []FileStatus{},
	}

	// 1. Current branch or detached commit
	branchOut, err := s.runGit(ctx, workspaceRoot, "branch", "--show-current")
	if err == nil && strings.TrimSpace(branchOut) != "" {
		res.Branch = strings.TrimSpace(branchOut)
	} else {
		// Check for detached HEAD
		headOut, err := s.runGit(ctx, workspaceRoot, "rev-parse", "--short", "HEAD")
		if err == nil && strings.TrimSpace(headOut) != "" {
			res.Branch = fmt.Sprintf("detached@%s", strings.TrimSpace(headOut))
		} else {
			res.Branch = "main"
		}
	}

	// 2. Ahead / Behind upstream
	revListOut, err := s.runGit(ctx, workspaceRoot, "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	if err == nil {
		parts := strings.Fields(strings.TrimSpace(revListOut))
		if len(parts) >= 2 {
			res.Ahead, _ = strconv.Atoi(parts[0])
			res.Behind, _ = strconv.Atoi(parts[1])
		}
	}

	// 3. Porcelain status
	statusOut, err := s.runGit(ctx, workspaceRoot, "status", "--porcelain=v1", "-uall")
	if err != nil {
		return res, nil
	}

	lines := strings.Split(statusOut, "\n")
	for _, line := range lines {
		if len(line) < 3 {
			continue
		}
		x := line[0]
		y := line[1]
		rest := strings.TrimSpace(line[2:])
		if rest == "" {
			continue
		}

		oldPath := ""
		filePath := rest
		if strings.Contains(rest, " -> ") {
			pts := strings.SplitN(rest, " -> ", 2)
			oldPath = strings.Trim(pts[0], "\"")
			filePath = strings.Trim(pts[1], "\"")
		} else {
			filePath = strings.Trim(filePath, "\"")
		}

		code := strings.TrimSpace(string([]byte{x, y}))
		staged := x != ' ' && x != '?'

		res.Files = append(res.Files, FileStatus{
			Path:   filePath,
			Code:   code,
			Staged: staged,
			Old:    oldPath,
		})
	}

	res.Dirty = len(res.Files) > 0
	return res, nil
}

// Branches returns all local git branches.
func (s *Service) Branches(ctx context.Context, workspaceRoot string) ([]Branch, error) {
	if !s.IsRepo(ctx, workspaceRoot) {
		return nil, errors.New("not a git repository")
	}

	out, err := s.runGit(ctx, workspaceRoot, "branch", "--list", "--no-color", "-vv")
	if err != nil {
		return nil, err
	}

	branches := []Branch{}
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		isCurrent := strings.HasPrefix(line, "*")
		content := strings.TrimPrefix(trimmed, "*")
		content = strings.TrimSpace(content)

		fields := strings.Fields(content)
		if len(fields) == 0 {
			continue
		}
		name := fields[0]

		remote := ""
		if strings.Contains(content, "[") && strings.Contains(content, "]") {
			start := strings.Index(content, "[")
			end := strings.Index(content, "]")
			if start < end {
				remote = content[start+1 : end]
			}
		}

		branches = append(branches, Branch{
			Name:    name,
			Current: isCurrent,
			Remote:  remote,
		})
	}

	return branches, nil
}

// validatePath guards against escaping the workspace root.
func (s *Service) validatePath(workspaceRoot, relPath string) error {
	cleanRel := filepath.Clean(relPath)
	if strings.HasPrefix(cleanRel, "..") || filepath.IsAbs(cleanRel) {
		return fmt.Errorf("invalid path %q: must be relative and inside workspace", relPath)
	}
	if workspaceRoot != "" {
		fullPath := filepath.Join(workspaceRoot, cleanRel)
		rel, err := filepath.Rel(workspaceRoot, fullPath)
		if err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("path %q escapes workspace root %q", relPath, workspaceRoot)
		}
	}
	return nil
}

// isProtectedSecret returns true if path matches secret environment files.
func isProtectedSecret(relPath string) bool {
	base := filepath.Base(relPath)
	return strings.HasSuffix(base, ".secret.env.json") || base == ".secret.env.json"
}

// Stage stages one or more files, strictly enforcing secret and .gitignore guards.
func (s *Service) Stage(ctx context.Context, workspaceRoot string, paths []string) error {
	if !s.IsRepo(ctx, workspaceRoot) {
		return errors.New("not a git repository")
	}
	if len(paths) == 0 {
		return nil
	}

	for _, p := range paths {
		if err := s.validatePath(workspaceRoot, p); err != nil {
			return err
		}

		// Guard 1: Secret environment file protection
		if isProtectedSecret(p) {
			return fmt.Errorf("refusing to stage protected secret file %q: secret files must never be committed", p)
		}

		// Guard 2: .gitignore protection
		ignored, _ := s.isIgnored(ctx, workspaceRoot, p)
		if ignored {
			return fmt.Errorf("refusing to stage ignored file %q: protected by .gitignore", p)
		}
	}

	args := append([]string{"add", "--"}, paths...)
	_, err := s.runGit(ctx, workspaceRoot, args...)
	return err
}

// isIgnored checks if git check-ignore matches the file.
func (s *Service) isIgnored(ctx context.Context, workspaceRoot, relPath string) (bool, error) {
	out, err := s.runGit(ctx, workspaceRoot, "check-ignore", "--", relPath)
	if err == nil && strings.TrimSpace(out) != "" {
		return true, nil
	}
	return false, nil
}

// Unstage unstages one or more files from index.
func (s *Service) Unstage(ctx context.Context, workspaceRoot string, paths []string) error {
	if !s.IsRepo(ctx, workspaceRoot) {
		return errors.New("not a git repository")
	}
	if len(paths) == 0 {
		return nil
	}

	for _, p := range paths {
		if err := s.validatePath(workspaceRoot, p); err != nil {
			return err
		}
	}

	args := append([]string{"restore", "--staged", "--"}, paths...)
	_, err := s.runGit(ctx, workspaceRoot, args...)
	return err
}

// Commit records changes in the repository.
func (s *Service) Commit(ctx context.Context, workspaceRoot, message string) error {
	if !s.IsRepo(ctx, workspaceRoot) {
		return errors.New("not a git repository")
	}
	msg := strings.TrimSpace(message)
	if msg == "" {
		return errors.New("commit message cannot be empty")
	}

	_, err := s.runGit(ctx, workspaceRoot, "commit", "-m", msg)
	return err
}

// Log returns recent commit history.
func (s *Service) Log(ctx context.Context, workspaceRoot string, limit int) ([]CommitInfo, error) {
	if !s.IsRepo(ctx, workspaceRoot) {
		return nil, errors.New("not a git repository")
	}
	if limit <= 0 {
		limit = 20
	}

	out, err := s.runGit(ctx, workspaceRoot, "log", fmt.Sprintf("-n%d", limit), "--pretty=format:%H%x09%h%x09%an%x09%ad%x09%s", "--date=short")
	if err != nil {
		// New repo with no commits returns error, handle gracefully
		return []CommitInfo{}, nil
	}

	commits := []CommitInfo{}
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		parts := strings.Split(line, "\t")
		if len(parts) >= 5 {
			commits = append(commits, CommitInfo{
				Hash:    parts[0],
				Short:   parts[1],
				Author:  parts[2],
				Date:    parts[3],
				Subject: parts[4],
			})
		}
	}

	return commits, nil
}

// validateBranchName checks if a branch name is valid according to git rules.
func (s *Service) validateBranchName(ctx context.Context, workspaceRoot, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("branch name cannot be empty")
	}
	if strings.ContainsAny(name, ";|&$`\"'<>") {
		return fmt.Errorf("invalid branch name %q: contains illegal shell characters", name)
	}

	_, err := s.runGit(ctx, workspaceRoot, "check-ref-format", "--branch", name)
	if err != nil {
		return fmt.Errorf("invalid git branch name %q: %w", name, err)
	}
	return nil
}

// Checkout switches to the specified branch, auto-stashing dirty changes if needed.
func (s *Service) Checkout(ctx context.Context, workspaceRoot, branch string) (*CheckoutResponse, error) {
	if !s.IsRepo(ctx, workspaceRoot) {
		return nil, errors.New("not a git repository")
	}
	if err := s.validateBranchName(ctx, workspaceRoot, branch); err != nil {
		return nil, err
	}

	// Check if dirty
	status, err := s.Status(ctx, workspaceRoot)
	if err != nil {
		return nil, err
	}

	stashed := false
	if status.Dirty {
		// Auto-stash
		stashMsg := fmt.Sprintf("pebblepost-auto-stash-%d", time.Now().Unix())
		_, err := s.runGit(ctx, workspaceRoot, "stash", "push", "-u", "-m", stashMsg)
		if err != nil {
			return nil, fmt.Errorf("failed to auto-stash changes before checkout: %w", err)
		}
		stashed = true
	}

	// Switch branch
	_, checkoutErr := s.runGit(ctx, workspaceRoot, "checkout", branch)
	if checkoutErr != nil {
		// If checkout failed and we stashed, attempt to pop stash back
		if stashed {
			_, _ = s.runGit(ctx, workspaceRoot, "stash", "pop")
		}
		return nil, fmt.Errorf("failed to checkout branch %q: %w", branch, checkoutErr)
	}

	// Restore stash if stashed
	if stashed {
		popOut, popErr := s.runGit(ctx, workspaceRoot, "stash", "pop")
		if popErr != nil {
			return &CheckoutResponse{
				Success: true,
				Stashed: true,
				Message: fmt.Sprintf("Switched to %s (stash popped with conflicts: %s)", branch, popOut),
			}, nil
		}
		return &CheckoutResponse{
			Success: true,
			Stashed: true,
			Message: fmt.Sprintf("Switched to %s (uncommitted changes auto-stashed and restored)", branch),
		}, nil
	}

	return &CheckoutResponse{
		Success: true,
		Stashed: false,
		Message: fmt.Sprintf("Switched to %s", branch),
	}, nil
}

// CreateBranch creates and checks out a new branch.
func (s *Service) CreateBranch(ctx context.Context, workspaceRoot, name string) error {
	if !s.IsRepo(ctx, workspaceRoot) {
		return errors.New("not a git repository")
	}
	if err := s.validateBranchName(ctx, workspaceRoot, name); err != nil {
		return err
	}

	_, err := s.runGit(ctx, workspaceRoot, "checkout", "-b", name)
	return err
}

// Pull performs git pull and translates auth and conflict errors into readable messages.
func (s *Service) Pull(ctx context.Context, workspaceRoot string) (*PushPullResponse, error) {
	if !s.IsRepo(ctx, workspaceRoot) {
		return nil, errors.New("not a git repository")
	}

	out, err := s.runGit(ctx, workspaceRoot, "pull")
	if err != nil {
		return &PushPullResponse{
			Success: false,
			Output:  out,
			Error:   translateGitError(err.Error()),
		}, err
	}

	return &PushPullResponse{
		Success: true,
		Output:  out,
	}, nil
}

// Push performs git push and translates auth and conflict errors into readable messages.
func (s *Service) Push(ctx context.Context, workspaceRoot string) (*PushPullResponse, error) {
	if !s.IsRepo(ctx, workspaceRoot) {
		return nil, errors.New("not a git repository")
	}

	// Push current branch
	status, err := s.Status(ctx, workspaceRoot)
	if err != nil {
		return nil, err
	}

	args := []string{"push"}
	if status.Ahead > 0 && status.Behind == 0 {
		// Set upstream if needed
		args = []string{"push", "-u", "origin", status.Branch}
	}

	out, err := s.runGit(ctx, workspaceRoot, args...)
	if err != nil {
		return &PushPullResponse{
			Success: false,
			Output:  out,
			Error:   translateGitError(err.Error()),
		}, err
	}

	return &PushPullResponse{
		Success: true,
		Output:  out,
	}, nil
}

func translateGitError(errText string) string {
	lower := strings.ToLower(errText)
	switch {
	case strings.Contains(lower, "authentication failed") || strings.Contains(lower, "permission denied (publickey)"):
		return "Git authentication failed. Please configure your SSH keys or git credential helper."
	case strings.Contains(lower, "conflict"):
		return "Merge conflict detected. Please resolve conflicts before continuing."
	case strings.Contains(lower, "non-fast-forward") || strings.Contains(lower, "fetch first"):
		return "Push rejected: remote has newer commits. Please pull before pushing."
	default:
		return errText
	}
}

// DiffRequest calculates per-field diff for a request file comparing HEAD with working tree.
func (s *Service) DiffRequest(ctx context.Context, workspaceRoot, relPath string) ([]DiffEntry, error) {
	if !s.IsRepo(ctx, workspaceRoot) {
		return nil, errors.New("not a git repository")
	}
	if err := s.validatePath(workspaceRoot, relPath); err != nil {
		return nil, err
	}

	// 1. Get HEAD content
	headBytes := []byte{}
	headOut, err := s.runGit(ctx, workspaceRoot, "show", fmt.Sprintf("HEAD:%s", relPath))
	if err == nil {
		headBytes = []byte(headOut)
	}

	// 2. Get Working tree content
	fullPath := filepath.Join(workspaceRoot, relPath)
	workBytes, _ := os.ReadFile(fullPath)

	// If neither exists
	if len(headBytes) == 0 && len(workBytes) == 0 {
		return []DiffEntry{}, nil
	}

	// Check if this is a Pebble request definition
	isPebbleRequest := strings.HasSuffix(relPath, ".pebble.json") || strings.HasSuffix(relPath, ".request.json")
	if isPebbleRequest {
		entries := diffRequestDefinitions(headBytes, workBytes)
		if len(entries) > 0 {
			return entries, nil
		}
	}

	// If not parsed as pebble request or no differences found via field diff, fallback to raw diff if content differs
	if !bytes.Equal(headBytes, workBytes) {
		return []DiffEntry{
			{
				Field:  "content",
				Before: string(headBytes),
				After:  string(workBytes),
				Type:   "text",
			},
		}, nil
	}

	return []DiffEntry{}, nil
}

// diffRequestDefinitions extracts field-level differences between HEAD and working copy RequestDefinitions.
func diffRequestDefinitions(headBytes, workBytes []byte) []DiffEntry {
	var headReq, workReq types.RequestDefinition
	headOk := len(headBytes) > 0 && json.Unmarshal(headBytes, &headReq) == nil
	workOk := len(workBytes) > 0 && json.Unmarshal(workBytes, &workReq) == nil

	if !headOk && !workOk {
		return nil
	}

	var entries []DiffEntry

	// Name
	if headReq.Name != workReq.Name {
		entries = append(entries, DiffEntry{
			Field:  "name",
			Before: headReq.Name,
			After:  workReq.Name,
			Type:   "text",
		})
	}

	// Method
	if headReq.Method != workReq.Method {
		entries = append(entries, DiffEntry{
			Field:  "method",
			Before: headReq.Method,
			After:  workReq.Method,
			Type:   "text",
		})
	}

	// URL
	if headReq.URL != workReq.URL {
		entries = append(entries, DiffEntry{
			Field:  "url",
			Before: headReq.URL,
			After:  workReq.URL,
			Type:   "text",
		})
	}

	// Headers
	headHdr := formatKeyValues(headReq.Headers)
	workHdr := formatKeyValues(workReq.Headers)
	if headHdr != workHdr {
		entries = append(entries, DiffEntry{
			Field:  "headers",
			Before: headHdr,
			After:  workHdr,
			Type:   "kv",
		})
	}

	// Body
	headBody := formatBody(headReq.Body)
	workBody := formatBody(workReq.Body)
	if headBody != workBody {
		entries = append(entries, DiffEntry{
			Field:  "body",
			Before: headBody,
			After:  workBody,
			Type:   "json",
		})
	}

	// Scripts
	headScript := formatScripts(headReq.Scripts)
	workScript := formatScripts(workReq.Scripts)
	if headScript != workScript {
		entries = append(entries, DiffEntry{
			Field:  "scripts",
			Before: headScript,
			After:  workScript,
			Type:   "js",
		})
	}

	return entries
}

func formatKeyValues(kvs []types.KeyValue) string {
	if len(kvs) == 0 {
		return ""
	}
	b, _ := json.MarshalIndent(kvs, "", "  ")
	return string(b)
}

func formatBody(b types.BodyDefinition) string {
	if b.Type == "" || b.Type == "none" {
		return ""
	}
	if b.Raw != "" {
		return b.Raw
	}
	if len(b.FormData) > 0 {
		return formatKeyValues(b.FormData)
	}
	if len(b.UrlEncoded) > 0 {
		return formatKeyValues(b.UrlEncoded)
	}
	if b.GraphQL != nil {
		return b.GraphQL.Query
	}
	return ""
}

func formatScripts(s types.ScriptDefinition) string {
	parts := []string{}
	if s.PreRequest != "" {
		parts = append(parts, "// Pre-Request Script\n"+s.PreRequest)
	}
	if s.PostResponse != "" {
		parts = append(parts, "// Post-Response Script\n"+s.PostResponse)
	}
	return strings.Join(parts, "\n\n")
}
