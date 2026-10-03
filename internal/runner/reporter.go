package runner

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"os"
	"strings"
	"time"
)

// Reporter writes run results to an output destination.
type Reporter interface {
	// PrintHeader is called once before any requests run.
	PrintHeader(target, env string, requestCount int, bail bool, dryRun bool)
	// PrintRequestResult is called after each request completes.
	PrintRequestResult(res RequestRunResult)
	// PrintSummary is called once after all requests complete.
	PrintSummary(s *RunSummary)
	// PrintWarning emits an advisory message.
	PrintWarning(msg string)
}

// newReporter constructs a Reporter for a ReporterConfig.
func newReporter(rc ReporterConfig, out io.Writer) Reporter {
	switch rc.Format {
	case "json":
		return &jsonReporter{out: out}
	case "junit":
		return &junitReporter{out: out}
	case "html":
		return &htmlReporter{out: out}
	default:
		return newCLIReporter(out)
	}
}

// ─── CLI Reporter ─────────────────────────────────────────────────────────────

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

type cliReporter struct {
	out     io.Writer
	noColor bool
}

func newCLIReporter(out io.Writer) *cliReporter {
	noColor := os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb"
	return &cliReporter{out: out, noColor: noColor}
}

func (f *cliReporter) colorize(code, text string) string {
	if f.noColor {
		return text
	}
	return code + text + colorReset
}

func (f *cliReporter) PrintHeader(target, env string, requestCount int, bail bool, dryRun bool) {
	if env == "" {
		env = "(none)"
	}
	bailStr := "disabled"
	if bail {
		bailStr = "enabled (--bail)"
	}
	dryRunStr := ""
	if dryRun {
		dryRunStr = fmt.Sprintf("\nDry Run:     %s", f.colorize(colorYellow, "YES — requests will NOT be sent"))
	}
	header := fmt.Sprintf(`
%s
Target:      %s
Environment: %s
Requests:    %d found
Bail Mode:   %s%s
%s
`,
		f.colorize(colorBold, "PebblePost CLI Test Runner"),
		target,
		env,
		requestCount,
		bailStr,
		dryRunStr,
		strings.Repeat("-", 60),
	)
	_, _ = fmt.Fprint(f.out, header)
}

func (f *cliReporter) PrintWarning(msg string) {
	_, _ = fmt.Fprintf(f.out, "%s %s\n", f.colorize(colorYellow, "[WARN]"), msg)
}

func (f *cliReporter) PrintRequestResult(res RequestRunResult) {
	if res.Skipped {
		_, _ = fmt.Fprintf(f.out, "%s %s\n\n",
			f.colorize(colorBlue+colorBold, "[SKIP]"), res.RelPath)
		return
	}

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

	retrySuffix := ""
	if res.RetryCount > 0 {
		retrySuffix = fmt.Sprintf(" [retried %dx]", res.RetryCount)
	}

	_, _ = fmt.Fprintf(f.out, "%s %s %s (%s%s)%s\n",
		badge,
		f.methodColor(method),
		res.RelPath,
		statusText,
		res.Duration.Round(time.Millisecond),
		retrySuffix,
	)

	if res.Result != nil {
		for _, test := range res.Result.Tests {
			if test.Passed {
				_, _ = fmt.Fprintf(f.out, "       %s %s\n", f.colorize(colorGreen, "+"), test.Name)
			} else {
				msg := ""
				if test.Message != "" {
					msg = fmt.Sprintf(" (%s)", test.Message)
				}
				_, _ = fmt.Fprintf(f.out, "       %s %s%s\n", f.colorize(colorRed, "-"), test.Name, msg)
			}
		}
	}

	if res.Error != "" && (res.Result == nil || len(res.Result.Tests) == 0) {
		_, _ = fmt.Fprintf(f.out, "       %s %s\n", f.colorize(colorRed, "x Error:"), res.Error)
	}

	_, _ = fmt.Fprintln(f.out)
}

func (f *cliReporter) PrintSummary(s *RunSummary) {
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
	if s.DryRun {
		summaryText += fmt.Sprintf("%s Dry run complete — no HTTP requests were sent.\n\n", f.colorize(colorBlue, "[INFO]"))
	}

	_, _ = fmt.Fprint(f.out, summaryText)
}

func (f *cliReporter) methodColor(method string) string {
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

// ─── JSON Reporter ─────────────────────────────────────────────────────────────

type jsonReporter struct {
	out io.Writer
}

func (j *jsonReporter) PrintHeader(_, _ string, _ int, _, _ bool) {}
func (j *jsonReporter) PrintWarning(_ string)                     {}
func (j *jsonReporter) PrintRequestResult(_ RequestRunResult)     {}
func (j *jsonReporter) PrintSummary(s *RunSummary) {
	enc := json.NewEncoder(j.out)
	enc.SetIndent("", "  ")
	_ = enc.Encode(s)
}

// ─── JUnit XML Reporter ────────────────────────────────────────────────────────

type junitReporter struct {
	out io.Writer
}

func (j *junitReporter) PrintHeader(_, _ string, _ int, _, _ bool) {}
func (j *junitReporter) PrintWarning(_ string)                     {}
func (j *junitReporter) PrintRequestResult(_ RequestRunResult)     {}

type junitTestSuites struct {
	XMLName    xml.Name         `xml:"testsuites"`
	Name       string           `xml:"name,attr"`
	Tests      int              `xml:"tests,attr"`
	Failures   int              `xml:"failures,attr"`
	Errors     int              `xml:"errors,attr"`
	Time       string           `xml:"time,attr"`
	TestSuites []junitTestSuite `xml:"testsuite"`
}

type junitTestSuite struct {
	XMLName   xml.Name        `xml:"testsuite"`
	Name      string          `xml:"name,attr"`
	Tests     int             `xml:"tests,attr"`
	Failures  int             `xml:"failures,attr"`
	Errors    int             `xml:"errors,attr"`
	Time      string          `xml:"time,attr"`
	TestCases []junitTestCase `xml:"testcase"`
}

type junitTestCase struct {
	XMLName   xml.Name      `xml:"testcase"`
	Name      string        `xml:"name,attr"`
	ClassName string        `xml:"classname,attr"`
	Time      string        `xml:"time,attr"`
	Failure   *junitFailure `xml:"failure,omitempty"`
	Error     *junitError   `xml:"error,omitempty"`
	Skipped   *junitSkipped `xml:"skipped,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Text    string `xml:",chardata"`
}

type junitError struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Text    string `xml:",chardata"`
}

type junitSkipped struct{}

func (j *junitReporter) PrintSummary(s *RunSummary) {
	suites := junitTestSuites{
		Name:     s.Target,
		Tests:    s.TotalTests,
		Failures: s.FailedTests,
	}
	suites.Time = fmt.Sprintf("%.3f", s.TotalDuration.Seconds())

	for _, res := range s.Results {
		suiteName := res.RelPath
		if res.Request != nil && res.Request.Name != "" {
			suiteName = res.Request.Name
		}

		suite := junitTestSuite{
			Name: suiteName,
			Time: fmt.Sprintf("%.3f", res.Duration.Seconds()),
		}

		if res.Skipped {
			tc := junitTestCase{
				Name:      suiteName,
				ClassName: res.RelPath,
				Skipped:   &junitSkipped{},
			}
			suite.Tests = 1
			suite.TestCases = []junitTestCase{tc}
			suites.TestSuites = append(suites.TestSuites, suite)
			continue
		}

		if res.Error != "" && (res.Result == nil || len(res.Result.Tests) == 0) {
			// Network or read error — emit as a single error testcase
			tc := junitTestCase{
				Name:      suiteName,
				ClassName: res.RelPath,
				Time:      fmt.Sprintf("%.3f", res.Duration.Seconds()),
				Error: &junitError{
					Message: res.Error,
					Type:    "ExecutionError",
					Text:    res.Error,
				},
			}
			suite.Tests = 1
			suite.Errors = 1
			suite.TestCases = []junitTestCase{tc}
			suites.TestSuites = append(suites.TestSuites, suite)
			suites.Errors++
			continue
		}

		if res.Result != nil {
			for _, t := range res.Result.Tests {
				tc := junitTestCase{
					Name:      t.Name,
					ClassName: res.RelPath,
					Time:      fmt.Sprintf("%.3f", res.Duration.Seconds()),
				}
				if !t.Passed {
					tc.Failure = &junitFailure{
						Message: t.Message,
						Type:    "AssertionError",
						Text:    t.Message,
					}
				}
				suite.TestCases = append(suite.TestCases, tc)
				suite.Tests++
				if !t.Passed {
					suite.Failures++
				}
			}
		} else if !res.Passed {
			tc := junitTestCase{
				Name:      suiteName,
				ClassName: res.RelPath,
				Time:      fmt.Sprintf("%.3f", res.Duration.Seconds()),
				Failure: &junitFailure{
					Message: res.Error,
					Type:    "RequestFailure",
					Text:    res.Error,
				},
			}
			suite.Tests = 1
			suite.Failures = 1
			suite.TestCases = []junitTestCase{tc}
		}

		suites.TestSuites = append(suites.TestSuites, suite)
	}

	_, _ = fmt.Fprintf(j.out, `<?xml version="1.0" encoding="UTF-8"?>`+"\n")
	enc := xml.NewEncoder(j.out)
	enc.Indent("", "  ")
	_ = enc.Encode(suites)
	_, _ = fmt.Fprintln(j.out)
}

// ─── HTML Reporter ─────────────────────────────────────────────────────────────

type htmlReporter struct {
	out io.Writer
}

func (h *htmlReporter) PrintHeader(_, _ string, _ int, _, _ bool) {}
func (h *htmlReporter) PrintWarning(_ string)                     {}
func (h *htmlReporter) PrintRequestResult(_ RequestRunResult)     {}

func (h *htmlReporter) PrintSummary(s *RunSummary) {
	statusClass := "passed"
	statusLabel := "PASSED"
	if !s.Success {
		statusClass = "failed"
		statusLabel = "FAILED"
	}

	var rows strings.Builder
	for _, res := range s.Results {
		rowClass := "pass"
		rowLabel := "PASS"
		if res.Skipped {
			rowClass = "skip"
			rowLabel = "SKIP"
		} else if !res.Passed {
			rowClass = "fail"
			rowLabel = "FAIL"
		}

		method := ""
		if res.Request != nil {
			method = res.Request.Method
		}

		status := ""
		if res.Result != nil && res.Result.StatusCode > 0 {
			status = fmt.Sprintf("%d", res.Result.StatusCode)
		}

		errMsg := html.EscapeString(res.Error)

		var assertions strings.Builder
		if res.Result != nil {
			for _, t := range res.Result.Tests {
				icon := "✓"
				cls := "assert-pass"
				if !t.Passed {
					icon = "✗"
					cls = "assert-fail"
				}
				assertions.WriteString(fmt.Sprintf(
					`<div class="assertion %s">%s %s%s</div>`,
					cls, icon, html.EscapeString(t.Name),
					func() string {
						if t.Message != "" {
							return " — " + html.EscapeString(t.Message)
						}
						return ""
					}(),
				))
			}
		}

		rows.WriteString(fmt.Sprintf(`
<tr class="%s">
  <td><span class="badge %s">%s</span></td>
  <td><code class="method">%s</code></td>
  <td>%s</td>
  <td>%s</td>
  <td>%s</td>
  <td>%s</td>
</tr>
<tr class="detail-row %s"><td colspan="6"><div class="assertions">%s%s</div></td></tr>`,
			rowClass,
			rowClass, rowLabel,
			html.EscapeString(method),
			html.EscapeString(res.RelPath),
			status,
			html.EscapeString(res.Duration.Round(time.Millisecond).String()),
			errMsg,
			rowClass, assertions.String(),
			func() string {
				if errMsg != "" && (res.Result == nil || len(res.Result.Tests) == 0) {
					return fmt.Sprintf(`<div class="assertion assert-fail">✗ %s</div>`, errMsg)
				}
				return ""
			}(),
		))
	}

	report := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>PebblePost Test Report</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;background:#0f0f0f;color:#e4e4e7;padding:2rem}
h1{font-size:1.5rem;font-weight:700;margin-bottom:.25rem}
.subtitle{color:#71717a;font-size:.875rem;margin-bottom:1.5rem}
.summary-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(140px,1fr));gap:1rem;margin-bottom:2rem}
.card{background:#18181b;border:1px solid #27272a;border-radius:.75rem;padding:1rem}
.card-label{font-size:.7rem;text-transform:uppercase;letter-spacing:.05em;color:#71717a;margin-bottom:.25rem}
.card-value{font-size:1.5rem;font-weight:700}
.card-value.green{color:#4ade80}.card-value.red{color:#f87171}.card-value.blue{color:#60a5fa}.card-value.yellow{color:#fbbf24}
.status-badge{display:inline-block;padding:.25rem .75rem;border-radius:9999px;font-weight:700;font-size:.875rem}
.status-badge.passed{background:#14532d;color:#4ade80}.status-badge.failed{background:#450a0a;color:#f87171}
table{width:100%%;border-collapse:collapse;background:#18181b;border-radius:.75rem;overflow:hidden;border:1px solid #27272a}
th{text-align:left;padding:.75rem 1rem;font-size:.75rem;text-transform:uppercase;letter-spacing:.05em;color:#71717a;border-bottom:1px solid #27272a}
td{padding:.625rem 1rem;font-size:.875rem;border-bottom:1px solid #1f1f23;vertical-align:middle}
tr.pass td{}.tr.fail td{}.tr.skip td{color:#71717a}
.badge{display:inline-block;padding:.125rem .5rem;border-radius:.25rem;font-size:.7rem;font-weight:700}
.badge.pass{background:#14532d;color:#4ade80}.badge.fail{background:#450a0a;color:#f87171}.badge.skip{background:#1c1917;color:#78716c}
.method{font-family:monospace;font-size:.8rem;color:#93c5fd}
.detail-row td{padding:.25rem 1rem .75rem;background:#0f0f0f}
.assertions{display:flex;flex-direction:column;gap:.25rem;padding:.5rem 0}
.assertion{font-size:.8rem;padding:.25rem .5rem;border-radius:.25rem}
.assert-pass{color:#4ade80}.assert-fail{color:#f87171;background:#450a0a22}
code{font-family:monospace}
</style>
</head>
<body>
<h1>PebblePost Test Report</h1>
<p class="subtitle">Target: %s | Environment: %s | Duration: %s</p>
<div class="summary-grid">
  <div class="card"><div class="card-label">Status</div><div><span class="status-badge %s">%s</span></div></div>
  <div class="card"><div class="card-label">Requests</div><div class="card-value blue">%d</div></div>
  <div class="card"><div class="card-label">Passed</div><div class="card-value green">%d</div></div>
  <div class="card"><div class="card-label">Failed</div><div class="card-value red">%d</div></div>
  <div class="card"><div class="card-label">Assertions</div><div class="card-value blue">%d</div></div>
</div>
<table>
<thead><tr><th>Status</th><th>Method</th><th>Path</th><th>HTTP</th><th>Duration</th><th>Error</th></tr></thead>
<tbody>%s</tbody>
</table>
</body>
</html>`,
		html.EscapeString(s.Target),
		func() string {
			if s.Environment != "" {
				return html.EscapeString(s.Environment)
			}
			return "(none)"
		}(),
		s.TotalDuration.Round(time.Millisecond),
		statusClass, statusLabel,
		s.TotalRequests,
		s.PassedRequests,
		s.FailedRequests,
		s.TotalTests,
		rows.String(),
	)

	_, _ = fmt.Fprint(h.out, report)
}
