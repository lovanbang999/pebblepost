package history

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pebblepost/internal/security"

	_ "modernc.org/sqlite" // pure-Go SQLite driver
)

const (
	defaultMaxBodyBytes = 100 * 1024 // 100 KB
	defaultMaxEntries   = 500
)

// Store manages the SQLite history database for one workspace.
type Store struct {
	db         *sql.DB
	maxEntries int
}

// Open opens (or creates) a SQLite history database under dir.
// dir is typically <dataDir>/history/ — it is created if missing.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("history: mkdir %s: %w", dir, err)
	}
	dbPath := filepath.Join(dir, "history.db")
	db, err := sql.Open("sqlite", dbPath+"?_journal=WAL&_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("history: open db: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite WAL allows concurrent readers, but writers must serialize

	s := &Store{db: db, maxEntries: defaultMaxEntries}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// migrate creates the schema on first run.
func (s *Store) migrate() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS history (
		id               INTEGER PRIMARY KEY AUTOINCREMENT,
		workspace_path   TEXT    NOT NULL,
		request_name     TEXT    NOT NULL DEFAULT '',
		method           TEXT    NOT NULL DEFAULT '',
		url              TEXT    NOT NULL DEFAULT '',
		status_code      INTEGER NOT NULL DEFAULT 0,
		duration_ms      REAL    NOT NULL DEFAULT 0,
		size_bytes       INTEGER NOT NULL DEFAULT 0,
		response_body    TEXT    NOT NULL DEFAULT '',
		response_headers TEXT    NOT NULL DEFAULT '{}',
		resolved_request TEXT    NOT NULL DEFAULT '{}',
		executed_at      TEXT    NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("history: migrate: %w", err)
	}
	// Indexes for common filter patterns
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_history_workspace ON history(workspace_path, executed_at DESC)`)
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_history_status ON history(workspace_path, status_code)`)
	return nil
}

// Record saves one execution to the database, masking secrets and truncating the body.
// It also enforces the maxEntries cap for the workspace.
func (s *Store) Record(input RecordInput) error {
	if input.Result == nil {
		return nil
	}

	maxBody := input.MaxBodyBytes
	if maxBody <= 0 {
		maxBody = defaultMaxBodyBytes
	}

	// Truncate response body
	body := input.Result.Body
	if len(body) > maxBody {
		body = body[:maxBody] + "\n…[truncated]"
	}

	// Mask secrets in the stored body and error
	if len(input.SecretsToMask) > 0 {
		body = security.MaskSecrets(body, input.SecretsToMask)
	}

	// Serialize and mask the resolved request
	resolvedReqJSON, err := json.Marshal(map[string]any{
		"method":  input.Method,
		"url":     input.URL,
		"headers": input.Result.Headers,
	})
	if err != nil {
		resolvedReqJSON = []byte("{}")
	}
	if len(input.SecretsToMask) > 0 {
		masked := security.MaskSecrets(string(resolvedReqJSON), input.SecretsToMask)
		resolvedReqJSON = []byte(masked)
	}

	// Serialize response headers
	headersJSON, _ := json.Marshal(input.Result.Headers)

	executedAt := input.Result.ExecutedAt
	if executedAt.IsZero() {
		executedAt = time.Now()
	}

	_, err = s.db.Exec(`INSERT INTO history
		(workspace_path, request_name, method, url, status_code, duration_ms,
		 size_bytes, response_body, response_headers, resolved_request, executed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		input.WorkspacePath,
		input.RequestName,
		input.Method,
		input.URL,
		input.Result.StatusCode,
		input.Result.Timing.TotalDurationMs,
		input.Result.Size,
		body,
		string(headersJSON),
		string(resolvedReqJSON),
		executedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("history: insert: %w", err)
	}

	// Enforce max entries: delete oldest rows beyond the cap
	_, err = s.db.Exec(`DELETE FROM history WHERE workspace_path = ? AND id NOT IN (
		SELECT id FROM history WHERE workspace_path = ?
		ORDER BY executed_at DESC LIMIT ?
	)`, input.WorkspacePath, input.WorkspacePath, s.maxEntries)
	return err
}

// List returns paginated history entries matching the filter.
func (s *Store) List(f ListFilter) ([]HistoryEntry, int, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	offset := (f.Page - 1) * f.Limit

	var args []any
	where := "workspace_path = ?"
	args = append(args, f.WorkspacePath)

	if f.StatusCode > 0 {
		where += " AND status_code = ?"
		args = append(args, f.StatusCode)
	}
	if f.Method != "" {
		where += " AND UPPER(method) = ?"
		args = append(args, strings.ToUpper(f.Method))
	}
	if !f.Since.IsZero() {
		where += " AND executed_at >= ?"
		args = append(args, f.Since.UTC().Format(time.RFC3339Nano))
	}
	if f.Search != "" {
		where += " AND (request_name LIKE ? OR url LIKE ?)"
		pat := "%" + strings.ReplaceAll(f.Search, "%", "\\%") + "%"
		args = append(args, pat, pat)
	}

	// Total count
	var total int
	row := s.db.QueryRow("SELECT COUNT(*) FROM history WHERE "+where, args...)
	if err := row.Scan(&total); err != nil {
		return nil, 0, err
	}

	// Paginated rows (no response body in list view — keep payloads small)
	rows, err := s.db.Query(
		`SELECT id, workspace_path, request_name, method, url, status_code,
		        duration_ms, size_bytes, response_headers, executed_at
		 FROM history WHERE `+where+`
		 ORDER BY executed_at DESC LIMIT ? OFFSET ?`,
		append(args, f.Limit, offset)...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var entries []HistoryEntry
	for rows.Next() {
		var e HistoryEntry
		var headersJSON, executedAtStr string
		if err := rows.Scan(
			&e.ID, &e.WorkspacePath, &e.RequestName, &e.Method, &e.URL,
			&e.StatusCode, &e.DurationMs, &e.SizeBytes, &headersJSON, &executedAtStr,
		); err != nil {
			return nil, 0, err
		}
		_ = json.Unmarshal([]byte(headersJSON), &e.ResponseHeaders)
		e.ExecutedAt, _ = time.Parse(time.RFC3339Nano, executedAtStr)
		entries = append(entries, e)
	}
	return entries, total, rows.Err()
}

// Get returns a single full history entry by ID, including body and resolved request.
func (s *Store) Get(id int64) (*HistoryEntry, error) {
	row := s.db.QueryRow(`SELECT id, workspace_path, request_name, method, url,
		status_code, duration_ms, size_bytes, response_body, response_headers,
		resolved_request, executed_at
		FROM history WHERE id = ?`, id)

	var e HistoryEntry
	var headersJSON, resolvedJSON, executedAtStr string
	if err := row.Scan(
		&e.ID, &e.WorkspacePath, &e.RequestName, &e.Method, &e.URL,
		&e.StatusCode, &e.DurationMs, &e.SizeBytes, &e.ResponseBody,
		&headersJSON, &resolvedJSON, &executedAtStr,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("history: entry %d not found", id)
		}
		return nil, err
	}
	_ = json.Unmarshal([]byte(headersJSON), &e.ResponseHeaders)
	e.ResolvedRequest = json.RawMessage(resolvedJSON)
	e.ExecutedAt, _ = time.Parse(time.RFC3339Nano, executedAtStr)
	return &e, nil
}

// Delete removes a single history entry.
func (s *Store) Delete(id int64) error {
	_, err := s.db.Exec("DELETE FROM history WHERE id = ?", id)
	return err
}

// Clear removes all history for a given workspace.
func (s *Store) Clear(workspacePath string) error {
	_, err := s.db.Exec("DELETE FROM history WHERE workspace_path = ?", workspacePath)
	return err
}
