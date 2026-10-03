package history

import (
	"encoding/json"
	"time"

	"pebblepost/internal/types"
)

// HistoryEntry is one recorded request execution stored in the history database.
type HistoryEntry struct {
	ID              int64               `json:"id"`
	WorkspacePath   string              `json:"workspacePath"`
	RequestName     string              `json:"requestName"`
	Method          string              `json:"method"`
	URL             string              `json:"url"`
	StatusCode      int                 `json:"statusCode"`
	DurationMs      float64             `json:"durationMs"`
	SizeBytes       int64               `json:"sizeBytes"`
	ResponseBody    string              `json:"responseBody"` // truncated to MaxBodyKB
	ResponseHeaders map[string][]string `json:"responseHeaders"`
	// ResolvedRequest holds the interpolated request with secrets already masked.
	ResolvedRequest json.RawMessage `json:"resolvedRequest"`
	ExecutedAt      time.Time       `json:"executedAt"`
}

// ListFilter carries optional constraints for listing history entries.
type ListFilter struct {
	WorkspacePath string
	StatusCode    int       // 0 = any
	Method        string    // empty = any (e.g. GET, POST)
	Search        string    // substring match on name/url
	Since         time.Time // zero = any (filter by execution time)
	Page          int       // 1-based
	Limit         int       // default 50
}

// RecordInput is the data provided to Store.Record from the execute handler.
type RecordInput struct {
	WorkspacePath string
	RequestName   string
	Method        string
	URL           string
	Result        *types.ExecutionResult
	// SecretsToMask are literal secret values to replace with "****" before saving.
	SecretsToMask []string
	// MaxBodyBytes is the byte cap for the stored response body (0 = use default).
	MaxBodyBytes int
}
