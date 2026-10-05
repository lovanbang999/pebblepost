package gitclient

// GitStatus represents the repository status of a workspace.
type GitStatus struct {
	IsRepo bool         `json:"isRepo"`
	Branch string       `json:"branch"`
	Ahead  int          `json:"ahead"`
	Behind int          `json:"behind"`
	Dirty  bool         `json:"dirty"`
	Files  []FileStatus `json:"files"`
}

// FileStatus represents an altered file in git status.
type FileStatus struct {
	Path   string `json:"path"`
	Code   string `json:"code"`   // "M", "A", "D", "?", "R", "MM", etc.
	Staged bool   `json:"staged"` // true if staged in the index
	Old    string `json:"old,omitempty"`
}

// Branch represents a git branch.
type Branch struct {
	Name    string `json:"name"`
	Current bool   `json:"current"`
	Remote  string `json:"remote,omitempty"`
}

// CommitInfo represents a git commit.
type CommitInfo struct {
	Hash    string `json:"hash"`
	Short   string `json:"short"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Subject string `json:"subject"`
}

// DiffEntry represents a field-level diff of a request or file.
type DiffEntry struct {
	Field  string `json:"field"`  // "name", "method", "url", "headers", "body", "scripts", "raw"
	Before string `json:"before"` // content in HEAD
	After  string `json:"after"`  // content in working tree
	Type   string `json:"type"`   // "text", "json", "kv", "js"
}

// StageRequest is the payload to stage or unstage files.
type StageRequest struct {
	Paths []string `json:"paths"`
}

// CommitRequest is the payload to create a commit.
type CommitRequest struct {
	Message string `json:"message"`
}

// CheckoutRequest is the payload to switch branch.
type CheckoutRequest struct {
	Branch string `json:"branch"`
}

// CreateBranchRequest is the payload to create and checkout a new branch.
type CreateBranchRequest struct {
	Name string `json:"name"`
}

// PushPullResponse carries terminal/log output from git pull/push.
type PushPullResponse struct {
	Success bool   `json:"success"`
	Output  string `json:"output"`
	Error   string `json:"error,omitempty"`
}

// CheckoutResponse carries outcome of checkout (e.g. whether stashed).
type CheckoutResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Stashed bool   `json:"stashed"`
}
