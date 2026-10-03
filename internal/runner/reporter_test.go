package runner

import (
	"bytes"
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"pebblepost/internal/types"
)

func makeTestSummary(passed, failed int) *RunSummary {
	results := make([]RequestRunResult, 0, passed+failed)

	for i := 0; i < passed; i++ {
		results = append(results, RequestRunResult{
			RelPath:  "req/pass.pebble.json",
			Passed:   true,
			Duration: 50 * time.Millisecond,
		})
	}
	for i := 0; i < failed; i++ {
		results = append(results, RequestRunResult{
			RelPath:  "req/fail.pebble.json",
			Passed:   false,
			Error:    "assertion failed: expected 200 got 404",
			Duration: 30 * time.Millisecond,
		})
	}

	return &RunSummary{
		Target:         "collections",
		Environment:    "test",
		TotalRequests:  passed + failed,
		PassedRequests: passed,
		FailedRequests: failed,
		TotalTests:     passed + failed,
		PassedTests:    passed,
		FailedTests:    failed,
		TotalDuration:  200 * time.Millisecond,
		Success:        failed == 0,
		Results:        results,
	}
}

// ─── CLI Reporter ──────────────────────────────────────────────────────────────

func TestCLIReporter_PrintHeader(t *testing.T) {
	var buf bytes.Buffer
	rep := newCLIReporter(&buf)
	rep.PrintHeader("collections", "dev", 3, true, false)

	out := buf.String()
	if !strings.Contains(out, "PebblePost CLI Test Runner") {
		t.Error("expected header title")
	}
	if !strings.Contains(out, "collections") {
		t.Error("expected target path in header")
	}
	if !strings.Contains(out, "enabled (--bail)") {
		t.Error("expected bail status in header")
	}
}

func TestCLIReporter_DryRunHeader(t *testing.T) {
	var buf bytes.Buffer
	rep := newCLIReporter(&buf)
	rep.PrintHeader("collections", "", 2, false, true)

	if !strings.Contains(buf.String(), "dry-run") && !strings.Contains(buf.String(), "Dry Run") {
		t.Error("expected dry-run notice in header")
	}
}

func TestCLIReporter_PrintSummary_Passed(t *testing.T) {
	var buf bytes.Buffer
	rep := newCLIReporter(&buf)
	s := makeTestSummary(5, 0)
	rep.PrintSummary(s)

	out := buf.String()
	if !strings.Contains(out, "PASSED") {
		t.Error("expected PASSED status")
	}
	if !strings.Contains(out, "5") {
		t.Error("expected total request count")
	}
}

func TestCLIReporter_PrintSummary_Failed(t *testing.T) {
	var buf bytes.Buffer
	rep := newCLIReporter(&buf)
	s := makeTestSummary(2, 1)
	rep.PrintSummary(s)

	if !strings.Contains(buf.String(), "FAILED") {
		t.Error("expected FAILED status")
	}
}

func TestCLIReporter_PrintRequestResult_Skipped(t *testing.T) {
	var buf bytes.Buffer
	rep := newCLIReporter(&buf)
	rep.PrintRequestResult(RequestRunResult{
		RelPath: "auth/login.pebble.json",
		Skipped: true,
		Passed:  true,
	})

	if !strings.Contains(buf.String(), "SKIP") {
		t.Error("expected SKIP badge for skipped request")
	}
}

// ─── JSON Reporter ─────────────────────────────────────────────────────────────

func TestJSONReporter_PrintSummary_ValidJSON(t *testing.T) {
	var buf bytes.Buffer
	rep := &jsonReporter{out: &buf}
	s := makeTestSummary(2, 1)
	rep.PrintSummary(s)

	if !strings.Contains(buf.String(), `"totalRequests"`) {
		t.Error("expected totalRequests field in JSON output")
	}
	if !strings.Contains(buf.String(), `"success"`) {
		t.Error("expected success field in JSON output")
	}
}

// ─── JUnit Reporter ────────────────────────────────────────────────────────────

func TestJUnitReporter_PrintSummary_ValidXML(t *testing.T) {
	var buf bytes.Buffer
	rep := &junitReporter{out: &buf}
	s := makeTestSummary(1, 1)

	// Add real assertion results to the failed result
	s.Results[1].Result = &types.ExecutionResult{
		StatusCode: 404,
		Tests: []types.TestAssertionResult{
			{Name: "status == 200", Passed: false, Message: "expected 200 got 404"},
		},
	}

	rep.PrintSummary(s)

	output := buf.String()
	if !strings.Contains(output, `<?xml version="1.0" encoding="UTF-8"?>`) {
		t.Error("expected XML declaration")
	}
	if !strings.Contains(output, "<testsuites") {
		t.Error("expected <testsuites> root element")
	}
	if !strings.Contains(output, "<testsuite") {
		t.Error("expected <testsuite> elements")
	}

	// Verify it is valid XML
	var suites junitTestSuites
	if err := xml.Unmarshal(buf.Bytes()[len(`<?xml version="1.0" encoding="UTF-8"?>`)+1:], &suites); err != nil {
		t.Fatalf("JUnit output is not valid XML: %v\nOutput:\n%s", err, output)
	}
}

func TestJUnitReporter_AllPassed(t *testing.T) {
	var buf bytes.Buffer
	rep := &junitReporter{out: &buf}
	s := makeTestSummary(3, 0)
	rep.PrintSummary(s)

	output := buf.String()
	if strings.Contains(output, "<failure") {
		t.Error("should not contain <failure> elements when all passed")
	}
}

func TestJUnitReporter_NetworkError(t *testing.T) {
	var buf bytes.Buffer
	rep := &junitReporter{out: &buf}
	s := &RunSummary{
		Target:         "collections",
		TotalRequests:  1,
		FailedRequests: 1,
		TotalDuration:  100 * time.Millisecond,
		Results: []RequestRunResult{
			{
				RelPath:  "api/test.pebble.json",
				Passed:   false,
				Error:    "Network execution error: connection refused",
				Duration: 100 * time.Millisecond,
			},
		},
	}
	rep.PrintSummary(s)

	if !strings.Contains(buf.String(), "<error") {
		t.Error("expected <error> element for network error")
	}
}

// ─── HTML Reporter ─────────────────────────────────────────────────────────────

func TestHTMLReporter_PrintSummary_Structure(t *testing.T) {
	var buf bytes.Buffer
	rep := &htmlReporter{out: &buf}
	s := makeTestSummary(2, 1)
	rep.PrintSummary(s)

	out := buf.String()
	checks := []string{
		"<!DOCTYPE html>",
		"PebblePost Test Report",
		"FAILED", // overall status (2 passed, 1 failed → success=false)
		`class="badge pass"`,
		`class="badge fail"`,
		"<table>",
		"</html>",
	}
	for _, check := range checks {
		if !strings.Contains(out, check) {
			t.Errorf("HTML report missing expected content: %q", check)
		}
	}
}

// ─── ReporterConfigsFromFlags ──────────────────────────────────────────────────

func TestReporterConfigsFromFlags(t *testing.T) {
	tests := []struct {
		input   []string
		wantFmt []string
		wantOut []string
		wantErr bool
	}{
		{
			input:   []string{"cli"},
			wantFmt: []string{"cli"},
			wantOut: []string{""},
		},
		{
			input:   []string{"junit:results.xml"},
			wantFmt: []string{"junit"},
			wantOut: []string{"results.xml"},
		},
		{
			input:   []string{"cli", "html:report.html", "junit:results.xml"},
			wantFmt: []string{"cli", "html", "junit"},
			wantOut: []string{"", "report.html", "results.xml"},
		},
		{
			input:   []string{"invalid"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(strings.Join(tt.input, ","), func(t *testing.T) {
			got, err := ReporterConfigsFromFlags(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.wantFmt) {
				t.Fatalf("expected %d reporters, got %d", len(tt.wantFmt), len(got))
			}
			for i, rc := range got {
				if rc.Format != tt.wantFmt[i] {
					t.Errorf("reporter[%d]: expected format %q, got %q", i, tt.wantFmt[i], rc.Format)
				}
				if rc.OutPath != tt.wantOut[i] {
					t.Errorf("reporter[%d]: expected outPath %q, got %q", i, tt.wantOut[i], rc.OutPath)
				}
			}
		})
	}
}
