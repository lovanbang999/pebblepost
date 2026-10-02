package runner

import (
	"io"
	"time"

	"pebblepost/internal/types"
)

// RunOptions configures the CLI test runner execution.
type RunOptions struct {
	TargetPath      string
	EnvironmentName string
	Bail            bool
	ReportFormat    string // "terminal" or "json"
	Writer          io.Writer
}

// RequestRunResult represents the outcome of executing a single request file.
type RequestRunResult struct {
	FilePath string                   `json:"filePath"`
	RelPath  string                   `json:"relPath"`
	Request  *types.RequestDefinition `json:"request"`
	Result   *types.ExecutionResult   `json:"result,omitempty"`
	Passed   bool                     `json:"passed"`
	Error    string                   `json:"error,omitempty"`
	Duration time.Duration            `json:"duration"`
}

// RunSummary aggregates execution statistics for a full collection test run.
type RunSummary struct {
	Target          string             `json:"target"`
	Environment     string             `json:"environment,omitempty"`
	TotalRequests   int                `json:"totalRequests"`
	PassedRequests  int                `json:"passedRequests"`
	FailedRequests  int                `json:"failedRequests"`
	TotalTests      int                `json:"totalTests"`
	PassedTests     int                `json:"passedTests"`
	FailedTests     int                `json:"failedTests"`
	TotalDuration   time.Duration      `json:"totalDurationMs"`
	Bailed          bool               `json:"bailed"`
	Success         bool               `json:"success"`
	Results         []RequestRunResult `json:"results"`
}
