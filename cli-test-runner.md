# Plan: CLI Test Runner (Task 7.1)

> Implementation of CLI Test Runner for CI/CD integration with exit codes, chained environment variables, and `--bail` support.

---

## 1. Goal & Requirements
- **Command Syntax**: `pebblepost run <path-to-collection> [-e <env>] [--bail] [--report json|terminal]`
- **Discovery**: Recursively discovers and executes `*.pebble.json` requests in deterministic order.
- **Environment & Chaining**: Loads active environment (`dev.env.json` + `dev.secret.env.json`) and updates in-memory variable map dynamically when scripts call `pb.environment.set()`.
- **Assertion Evaluation**: Runs pre-request scripts and post-response assertion tests (`pb.test`, `pb.expect`).
- **Failure Handling**:
  - Without `--bail`: Runs all requests, reports complete results table, exits with code 1 if any assertion or network error fails.
  - With `--bail`: Halts execution immediately on the first failed assertion or request error, returns exit code 1.
- **Terminal Output**: Human-readable table with colored status badges, response durations, and assertion details.

---

## 2. Implementation Steps

- [x] **Task 1: Core Test Runner (`internal/runner/runner.go`)**
  - Implement `Runner` struct with `WorkspaceService`, `EnvironmentService`, `Interpolator`, `Client`, and `ScriptEngine`.
  - Implement request collection traversal and sorting.
  - Implement single request execution with in-memory chained variable updates.
  - Implement summary aggregation (`TotalRequests`, `PassedRequests`, `FailedRequests`, `TotalTests`, `PassedTests`, `FailedTests`, `Duration`).

- [x] **Task 2: Terminal Formatter (`internal/runner/formatter.go`)**
  - Implement real-time request progress output (Method badge, status, latency).
  - Implement assertion breakdown reporting with clear pass/fail indicators.
  - Implement clean final summary table.

- [x] **Task 3: CLI Entrypoint Update (`cmd/cli/main.go`)**
  - Update flag parsing for `run` subcommand (`-e`, `--bail`, `--ci`, `--report`).
  - Wire up `runner.NewRunner()` and execute `runCmd`.
  - Ensure correct OS exit codes (`os.Exit(0)` on all-pass, `os.Exit(1)` on any failure).

- [x] **Task 4: Unit & Integration Testing (`internal/runner/runner_test.go`)**
  - Test running a valid collection with passing assertions.
  - Test chained environment variable propagation (`pb.environment.set`).
  - Test `--bail` behavior on failing assertion.
  - Verify exit codes and report counts.

- [x] **Task 5: Verification & Plan Checkoff**
  - Run `go test -v ./internal/runner`
  - Build single binary: `go build -o /tmp/pebblepost-cli ./cmd/cli`
  - Execute live test run on collections in workspace.
  - Check off Task 7.1 in `pebblepost-plan.md`.
