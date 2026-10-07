package runner

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"pebblepost/internal/types"
)

// ReporterConfig describes one reporter output in a run.
// Format is one of "cli", "json", "junit", "html".
// OutPath is the file path for file-based reporters ("" = stdout).
type ReporterConfig struct {
	Format  string
	OutPath string
}

// FilterOptions restricts which requests are executed.
type FilterOptions struct {
	Folder  string // glob matched against the folder relative path
	Request string // glob matched against request relPath or name
	Tag     string // exact match against any entry in request.Tags
}

// RunOptions configures the CLI test runner execution.
type RunOptions struct {
	TargetPath      string
	EnvironmentName string
	Bail            bool
	DryRun          bool

	// TimeoutMs overrides per-request timeout globally (0 = use per-request setting).
	TimeoutMs int
	// RetryCount is the number of retries on network errors (not assertion failures).
	RetryCount int
	// DelayMs is the sleep duration between requests.
	DelayMs int

	// DataFile is the path to an imported CSV or JSON data file for iterations.
	DataFile string
	// DataRows provides pre-parsed data rows (useful for API/UI runner).
	DataRows []map[string]any
	// Iterations specifies how many iterations to run.
	Iterations int
	// MaxExecutionsPerIteration guards against infinite loops with pb.runner.setNextRequest (default: 100).
	MaxExecutionsPerIteration int
	// RequestPaths explicitly specifies the list and order of request files to execute.
	RequestPaths []string

	// Reporters specifies one or more output reporters.
	// If empty, a "cli" reporter writing to Writer (or stdout) is used.
	Reporters []ReporterConfig

	// CustomReporter allows injecting a programmatic reporter (e.g. SSE event emitter).
	CustomReporter Reporter

	// ExtraVars are highest-precedence variables from --var KEY=VALUE flags.
	ExtraVars map[string]string
	// EnvFile is a path to an additional env file (lower precedence than ExtraVars).
	EnvFile string
	// Filter restricts which requests run.
	Filter FilterOptions

	// TrustScripts determines if pre-request and post-response scripts can execute.
	// In the CLI runner, scripts are disabled unless explicitly trusted via --trust or a .pebbletrust file.
	// If nil, defaults to true (for programmatic API callers).
	TrustScripts *bool

	// Writer is the fallback writer for legacy callers (used when Reporters is empty).
	Writer io.Writer
	// ReportFormat is the legacy format field (maps to a single ReporterConfig).
	ReportFormat string

	// SecretBackend overrides the secret storage backend ("file", "keychain", "env").
	// If empty, it is auto-detected from workspace.json or PEBBLEPOST_SECRET_BACKEND.
	SecretBackend string
}

// RequestRunResult represents the outcome of executing a single request file.
type RequestRunResult struct {
	Iteration  int                      `json:"iteration,omitempty"`
	FilePath   string                   `json:"filePath"`
	RelPath    string                   `json:"relPath"`
	Request    *types.RequestDefinition `json:"request,omitempty"`
	Result     *types.ExecutionResult   `json:"result,omitempty"`
	Passed     bool                     `json:"passed"`
	Error      string                   `json:"error,omitempty"`
	Duration   time.Duration            `json:"duration"`
	RetryCount int                      `json:"retryCount,omitempty"`
	Skipped    bool                     `json:"skipped,omitempty"`
}

// IterationSummary records aggregate outcomes for a single iteration loop.
type IterationSummary struct {
	Iteration   int                `json:"iteration"`
	DataRow     map[string]any     `json:"dataRow,omitempty"`
	Results     []RequestRunResult `json:"results"`
	TotalTests  int                `json:"totalTests"`
	PassedTests int                `json:"passedTests"`
	FailedTests int                `json:"failedTests"`
	Duration    time.Duration      `json:"duration"`
	Passed      bool               `json:"passed"`
	Error       string             `json:"error,omitempty"`
}

// RunSummary aggregates execution statistics for a full collection test run.
type RunSummary struct {
	Target           string             `json:"target"`
	Environment      string             `json:"environment,omitempty"`
	TotalIterations  int                `json:"totalIterations"`
	PassedIterations int                `json:"passedIterations"`
	FailedIterations int                `json:"failedIterations"`
	TotalRequests    int                `json:"totalRequests"`
	PassedRequests   int                `json:"passedRequests"`
	FailedRequests   int                `json:"failedRequests"`
	SkippedRequests  int                `json:"skippedRequests,omitempty"`
	TotalTests       int                `json:"totalTests"`
	PassedTests      int                `json:"passedTests"`
	FailedTests      int                `json:"failedTests"`
	TotalDuration    time.Duration      `json:"totalDurationMs"`
	AvgDurationMs    float64            `json:"avgDurationMs"`
	P95DurationMs    float64            `json:"p95DurationMs"`
	PassRate         float64            `json:"passRate"` // 0.0 - 100.0%
	Bailed           bool               `json:"bailed"`
	DryRun           bool               `json:"dryRun,omitempty"`
	Success          bool               `json:"success"`
	Results          []RequestRunResult `json:"results"`
	Iterations       []IterationSummary `json:"iterations,omitempty"`
	// ExitCode is computed after Run completes:
	//   0 = all passed, 1 = assertion failure, 2 = config/parse error, 3 = network error
	ExitCode int `json:"-"`
}

// CalculateP95 computes the 95th percentile latency in milliseconds.
func CalculateP95(durations []time.Duration) float64 {
	if len(durations) == 0 {
		return 0
	}
	sorted := make([]float64, len(durations))
	for i, d := range durations {
		sorted[i] = float64(d.Milliseconds())
	}
	sort.Float64s(sorted)
	idx := int(float64(len(sorted))*0.95+0.5) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// ReporterConfigsFromFlags parses raw --reporter flag values.
// Each entry is "format" or "format:path". E.g. "junit:results.xml".
func ReporterConfigsFromFlags(values []string) ([]ReporterConfig, error) {
	var out []ReporterConfig
	valid := map[string]bool{"cli": true, "json": true, "junit": true, "html": true}
	for _, v := range values {
		parts := strings.SplitN(v, ":", 2)
		format := strings.ToLower(strings.TrimSpace(parts[0]))
		if !valid[format] {
			return nil, fmt.Errorf("unknown reporter %q: valid values are cli, json, junit, html", format)
		}
		rc := ReporterConfig{Format: format}
		if len(parts) == 2 {
			rc.OutPath = strings.TrimSpace(parts[1])
		}
		out = append(out, rc)
	}
	if len(out) == 0 {
		out = []ReporterConfig{{Format: "cli"}}
	}
	return out, nil
}

// resolveReporters converts legacy ReportFormat/Writer fields into a reporter list.
func resolveReporters(opts RunOptions) []ReporterConfig {
	if len(opts.Reporters) > 0 {
		return opts.Reporters
	}
	format := opts.ReportFormat
	if format == "" || format == "terminal" {
		format = "cli"
	}
	return []ReporterConfig{{Format: format}}
}

// resolveOut returns a writer for the given ReporterConfig.
// If OutPath is non-empty, opens/creates that file (caller must close if owned=true).
// Otherwise returns the provided fallback (typically os.Stdout).
func resolveOut(rc ReporterConfig, fallback io.Writer) (io.Writer, bool, error) {
	if rc.OutPath != "" {
		f, err := os.Create(rc.OutPath)
		if err != nil {
			return nil, false, fmt.Errorf("cannot open reporter output %q: %w", rc.OutPath, err)
		}
		return f, true, nil
	}
	if fallback != nil {
		return fallback, false, nil
	}
	return os.Stdout, false, nil
}
