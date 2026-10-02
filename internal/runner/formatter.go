package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// ANSI color escape codes.
const (
	colorReset   = "\033[0m"
	colorRed     = "\033[31m"
	colorGreen   = "\033[32m"
	colorYellow  = "\033[33m"
	colorBlue    = "\033[34m"
	colorMagenta = "\033[35m"
	colorCyan    = "\033[36m"
	colorBold    = "\033[1m"
)

// Formatter manages output rendering for CLI test executions.
type Formatter struct {
	noColor bool
}

// NewFormatter creates a new Formatter, detecting NO_COLOR configuration.
func NewFormatter() *Formatter {
	noColor := os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb"
	return &Formatter{noColor: noColor}
}

func (f *Formatter) colorize(code, text string) string {
	if f.noColor {
		return text
	}
	return code + text + colorReset
}

// PrintHeader outputs run configuration details before execution starts.
func (f *Formatter) PrintHeader(out io.Writer, target, env string, requestCount int, bail bool) {
	if env == "" {
		env = "(none)"
	}
	bailStr := "disabled"
	if bail {
		bailStr = "enabled (--bail)"
	}

	header := fmt.Sprintf(`
%s
Target:      %s
Environment: %s
Requests:    %d found
Bail Mode:   %s
%s
`,
		f.colorize(colorBold, "PebblePost CLI Test Runner"),
		target,
		env,
		requestCount,
		bailStr,
		strings.Repeat("-", 60),
	)
	_, _ = fmt.Fprint(out, header)
}

// PrintWarning prints an execution warning message.
func (f *Formatter) PrintWarning(out io.Writer, msg string) {
	_, _ = fmt.Fprintf(out, "%s %s\n", f.colorize(colorYellow, "[WARN]"), msg)
}

// PrintRequestFailure prints an immediate failure message when a request cannot be read or sent.
func (f *Formatter) PrintRequestFailure(out io.Writer, relPath, method, errMessage string, duration time.Duration) {
	badge := f.colorize(colorRed+colorBold, "[FAIL]")
	methodStr := f.colorize(colorBold, method)
	_, _ = fmt.Fprintf(out, "%s %s %s (%s)\n", badge, methodStr, relPath, duration.Round(time.Millisecond))
	_, _ = fmt.Fprintf(out, "       %s %s\n\n", f.colorize(colorRed, "x Error:"), errMessage)
}

// PrintRequestResult prints the outcome of executing a single request.
func (f *Formatter) PrintRequestResult(out io.Writer, res RequestRunResult) {
	badge := f.colorize(colorGreen+colorBold, "[PASS]")
	if !res.Passed {
		badge = f.colorize(colorRed+colorBold, "[FAIL]")
	}

	method := "REQ"
	if res.Request != nil && res.Request.Method != "" {
		method = res.Request.Method
	}

	statusText := ""
	if res.Result != nil && res.Result.StatusCode > 0 {
		statusText = fmt.Sprintf("%d %s - ", res.Result.StatusCode, res.Result.StatusText)
	}

	_, _ = fmt.Fprintf(out, "%s %s %s (%s%s)\n",
		badge,
		f.methodColor(method),
		res.RelPath,
		statusText,
		res.Duration.Round(time.Millisecond),
	)

	// Print individual assertions if present
	if res.Result != nil {
		for _, test := range res.Result.Tests {
			if test.Passed {
				_, _ = fmt.Fprintf(out, "       %s %s\n", f.colorize(colorGreen, "+"), test.Name)
			} else {
				msg := ""
				if test.Message != "" {
					msg = fmt.Sprintf(" (%s)", test.Message)
				}
				_, _ = fmt.Fprintf(out, "       %s %s%s\n", f.colorize(colorRed, "-"), test.Name, msg)
			}
		}
	}

	if res.Error != "" && (res.Result == nil || len(res.Result.Tests) == 0) {
		_, _ = fmt.Fprintf(out, "       %s %s\n", f.colorize(colorRed, "x Error:"), res.Error)
	}

	_, _ = fmt.Fprintln(out)
}

// PrintSummary prints the final statistics table and overall status.
func (f *Formatter) PrintSummary(out io.Writer, s *RunSummary) {
	statusStr := f.colorize(colorGreen+colorBold, "PASSED")
	if !s.Success {
		statusStr = f.colorize(colorRed+colorBold, "FAILED")
	}

	divider := strings.Repeat("=", 60)
	summaryText := fmt.Sprintf(`%s
                        TEST SUMMARY                          
%s
Requests:    %d total | %s passed | %s failed
Assertions:  %d total | %s passed | %s failed
Duration:    %s
Status:      %s
%s
`,
		divider,
		divider,
		s.TotalRequests,
		f.colorize(colorGreen, fmt.Sprintf("%d", s.PassedRequests)),
		f.colorize(colorRed, fmt.Sprintf("%d", s.FailedRequests)),
		s.TotalTests,
		f.colorize(colorGreen, fmt.Sprintf("%d", s.PassedTests)),
		f.colorize(colorRed, fmt.Sprintf("%d", s.FailedTests)),
		s.TotalDuration.Round(time.Millisecond),
		statusStr,
		divider,
	)

	if s.Bailed {
		summaryText += fmt.Sprintf("%s Execution halted early due to --bail flag.\n\n", f.colorize(colorYellow, "[INFO]"))
	}

	_, _ = fmt.Fprint(out, summaryText)
}

// PrintJSON formats the summary output as indented JSON.
func (f *Formatter) PrintJSON(out io.Writer, s *RunSummary) {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	_ = enc.Encode(s)
}

func (f *Formatter) methodColor(method string) string {
	switch strings.ToUpper(method) {
	case "GET":
		return f.colorize(colorCyan+colorBold, "GET")
	case "POST":
		return f.colorize(colorGreen+colorBold, "POST")
	case "PUT":
		return f.colorize(colorYellow+colorBold, "PUT")
	case "DELETE":
		return f.colorize(colorRed+colorBold, "DELETE")
	case "PATCH":
		return f.colorize(colorMagenta+colorBold, "PATCH")
	default:
		return f.colorize(colorBold, method)
	}
}
