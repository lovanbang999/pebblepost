package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"pebblepost/internal/httpclient"
	"pebblepost/internal/scripting"
	"pebblepost/internal/security"
	"pebblepost/internal/types"
	"pebblepost/internal/workspace"
)

// ExitCode constants for os.Exit.
const (
	ExitSuccess       = 0 // all assertions passed
	ExitAssertionFail = 1 // one or more test assertions failed
	ExitConfigError   = 2 // configuration/parse error
	ExitNetworkError  = 3 // network error or timeout
)

// Runner manages CLI collection test executions.
type Runner struct {
	workspaceSvc        *workspace.WorkspaceService
	environmentSvc      *workspace.EnvironmentService
	interpolator        *workspace.Interpolator
	inheritanceResolver *workspace.InheritanceResolver
	client              httpclient.Client
	scriptEngine        *scripting.Engine
}

// NewRunner creates a fully initialized CLI Runner.
func NewRunner() *Runner {
	wsSvc := workspace.NewWorkspaceService()
	return &Runner{
		workspaceSvc:        wsSvc,
		environmentSvc:      workspace.NewEnvironmentService(),
		interpolator:        workspace.NewInterpolator(),
		inheritanceResolver: workspace.NewInheritanceResolver(wsSvc),
		client:              httpclient.NewClient(),
		scriptEngine:        scripting.NewEngine(),
	}
}

// NewCustomRunner allows injecting custom clients or services for unit testing.
func NewCustomRunner(
	wsSvc *workspace.WorkspaceService,
	envSvc *workspace.EnvironmentService,
	interpolator *workspace.Interpolator,
	client httpclient.Client,
	scriptEngine *scripting.Engine,
) *Runner {
	return &Runner{
		workspaceSvc:        wsSvc,
		environmentSvc:      envSvc,
		interpolator:        interpolator,
		inheritanceResolver: workspace.NewInheritanceResolver(wsSvc),
		client:              client,
		scriptEngine:        scriptEngine,
	}
}

// Run executes the requests discovered at targetPath using the specified options.
func (r *Runner) Run(ctx context.Context, opts RunOptions) (*RunSummary, error) {
	// ── Resolve reporters ────────────────────────────────────────────────────
	reporterCfgs := resolveReporters(opts)

	// Determine fallback writer from legacy Writer field
	legacyWriter := opts.Writer
	if legacyWriter == nil {
		legacyWriter = os.Stdout
	}

	type reporterEntry struct {
		r     Reporter
		w     io.Writer
		owned bool
		f     *os.File
	}
	var reporters []reporterEntry
	for _, rc := range reporterCfgs {
		w, owned, err := resolveOut(rc, legacyWriter)
		if err != nil {
			return nil, fmt.Errorf("reporter setup: %w", err)
		}
		reporters = append(reporters, reporterEntry{
			r:     newReporter(rc, w),
			w:     w,
			owned: owned,
		})
	}
	defer func() {
		for _, e := range reporters {
			if e.owned {
				if f, ok := e.w.(*os.File); ok {
					_ = f.Close()
				}
			}
		}
	}()

	broadcast := func(fn func(Reporter)) {
		for _, e := range reporters {
			fn(e.r)
		}
	}

	// ── Discover request files ────────────────────────────────────────────────
	targetPath := opts.TargetPath
	if targetPath == "" {
		targetPath = "."
	}

	requestFiles, err := r.discoverRequests(targetPath)
	if err != nil {
		return nil, fmt.Errorf("request discovery error: %w", err)
	}
	if len(requestFiles) == 0 {
		return nil, fmt.Errorf("no *.pebble.json request files found in %s", targetPath)
	}

	// ── Locate workspace root ────────────────────────────────────────────────
	wsRoot := r.findWorkspaceRoot(targetPath)
	if wsRoot != "" && r.client != nil {
		r.client.SetWorkspace(wsRoot)
	}

	// ── Build variable map ───────────────────────────────────────────────────
	baseVarMap := make(map[string]string)
	var secretValues []string

	if opts.EnvironmentName != "" && r.environmentSvc != nil {
		envDef, envErr := r.environmentSvc.GetEnvironment(wsRoot, opts.EnvironmentName)
		if envErr == nil && envDef != nil {
			baseVarMap = r.interpolator.BuildVariableMap(envDef, nil)
			for _, v := range envDef.Variables {
				if v.Secret && v.Value != "" {
					secretValues = append(secretValues, v.Value)
				}
			}
		} else {
			broadcast(func(rep Reporter) {
				rep.PrintWarning(fmt.Sprintf("Environment '%s' not found in workspace %s, proceeding with empty environment.", opts.EnvironmentName, wsRoot))
			})
		}
	}

	// Apply full variable precedence (env < env-file < PEBBLE_VAR_* < --var)
	varMap := BuildVarMap(baseVarMap, opts.ExtraVars, opts.EnvFile, &secretValues, r.interpolator)

	// ── Print run header (CLI only; JSON/JUnit/HTML skip this) ───────────────
	broadcast(func(rep Reporter) {
		rep.PrintHeader(targetPath, opts.EnvironmentName, len(requestFiles), opts.Bail, opts.DryRun)
	})

	summary := &RunSummary{
		Target:      targetPath,
		Environment: opts.EnvironmentName,
		Results:     make([]RequestRunResult, 0, len(requestFiles)),
		DryRun:      opts.DryRun,
	}

	startTime := time.Now()

	// ── Execute requests ─────────────────────────────────────────────────────
	hasNetworkError := false
	hasAssertionFailure := false
	hasConfigError := false

	for _, filePath := range requestFiles {
		select {
		case <-ctx.Done():
			return summary, ctx.Err()
		default:
		}

		relPath, _ := filepath.Rel(wsRoot, filePath)
		if relPath == "" {
			relPath = filePath
		}

		// ── Delay between requests ──────────────────────────────────────────
		if len(summary.Results) > 0 && opts.DelayMs > 0 {
			select {
			case <-ctx.Done():
				return summary, ctx.Err()
			case <-time.After(time.Duration(opts.DelayMs) * time.Millisecond):
			}
		}

		reqStart := time.Now()

		// ── Read request file ───────────────────────────────────────────────
		req, readErr := r.workspaceSvc.ReadRequest(filePath)
		if readErr != nil {
			hasConfigError = true
			reqResult := RequestRunResult{
				FilePath: filePath,
				RelPath:  relPath,
				Passed:   false,
				Error:    fmt.Sprintf("Failed to read request file: %v", readErr),
				Duration: time.Since(reqStart),
			}
			summary.TotalRequests++
			summary.FailedRequests++
			summary.Results = append(summary.Results, reqResult)
			broadcast(func(rep Reporter) { rep.PrintRequestResult(reqResult) })
			if opts.Bail {
				summary.Bailed = true
				break
			}
			continue
		}

		// ── Apply filter ────────────────────────────────────────────────────
		if !FilterRequest(relPath, req, opts.Filter) {
			skipResult := RequestRunResult{
				FilePath: filePath,
				RelPath:  relPath,
				Request:  req,
				Passed:   true,
				Skipped:  true,
				Duration: time.Since(reqStart),
			}
			summary.SkippedRequests++
			summary.Results = append(summary.Results, skipResult)
			broadcast(func(rep Reporter) { rep.PrintRequestResult(skipResult) })
			continue
		}

		// ── Dry run: print but do not send ──────────────────────────────────
		if opts.DryRun {
			method := req.Method
			dryResult := RequestRunResult{
				FilePath: filePath,
				RelPath:  relPath,
				Request:  req,
				Passed:   true,
				Skipped:  true,
				Duration: 0,
				Error:    fmt.Sprintf("dry-run: would send %s %s", method, req.URL),
			}
			summary.TotalRequests++
			summary.PassedRequests++
			summary.Results = append(summary.Results, dryResult)
			broadcast(func(rep Reporter) { rep.PrintRequestResult(dryResult) })
			continue
		}

		// ── Folder inheritance ──────────────────────────────────────────────
		var folderChain []workspace.FolderChainItem
		if r.inheritanceResolver != nil {
			folderChain, _ = r.inheritanceResolver.DiscoverFolderChain(wsRoot, filePath)
		}

		currentVarMap := make(map[string]string, len(varMap))
		for k, v := range varMap {
			currentVarMap[k] = v
		}
		if r.inheritanceResolver != nil && len(folderChain) > 0 {
			currentVarMap, _ = r.inheritanceResolver.MergeVariables(currentVarMap, folderChain, nil)
		}

		reqToExecute := *req
		if r.inheritanceResolver != nil && len(folderChain) > 0 {
			mergedHeaders, _ := r.inheritanceResolver.MergeHeaders(folderChain, req.Headers)
			reqToExecute.Headers = mergedHeaders
			resolvedAuth, _ := r.inheritanceResolver.ResolveAuth(folderChain, req.Auth)
			reqToExecute.Auth = resolvedAuth
		}

		// ── Apply global timeout override ───────────────────────────────────
		if opts.TimeoutMs > 0 {
			reqToExecute.Settings.TimeoutMs = opts.TimeoutMs
		}

		interpolatedReq := r.interpolator.InterpolateRequest(&reqToExecute, currentVarMap)

		// ── Pre-request scripts ─────────────────────────────────────────────
		var preScripts, postScripts []workspace.ScriptChainItem
		if r.inheritanceResolver != nil && len(folderChain) > 0 {
			preScripts, postScripts = r.inheritanceResolver.ResolveScripts(folderChain, interpolatedReq.Scripts)
		} else {
			if interpolatedReq.Scripts.PreRequest != "" {
				preScripts = append(preScripts, workspace.ScriptChainItem{Source: "request", Script: interpolatedReq.Scripts.PreRequest})
			}
			if interpolatedReq.Scripts.PostResponse != "" {
				postScripts = append(postScripts, workspace.ScriptChainItem{Source: "request", Script: interpolatedReq.Scripts.PostResponse})
			}
		}

		var preErrOccurred error
		if r.scriptEngine != nil {
			scriptTimeout := scriptTimeoutFor(interpolatedReq.Settings.ScriptTimeoutMs)
			for _, s := range preScripts {
				preResult, preErr := r.scriptEngine.ExecutePreRequestNamed(s.Source, s.Script, interpolatedReq, currentVarMap, scriptTimeout)
				if preResult != nil {
					for k, v := range preResult.ExtractedEnvVars {
						currentVarMap[k] = v
						varMap[k] = v
					}
					interpolatedReq = preResult.Request
				}
				if preErr != nil {
					preErrOccurred = fmt.Errorf("[%s] Pre-request script error: %w", s.Source, preErr)
					break
				}
			}
		}

		if preErrOccurred != nil {
			hasConfigError = true
			reqResult := RequestRunResult{
				FilePath: filePath, RelPath: relPath, Request: req,
				Passed: false, Error: preErrOccurred.Error(), Duration: time.Since(reqStart),
			}
			summary.TotalRequests++
			summary.FailedRequests++
			summary.Results = append(summary.Results, reqResult)
			broadcast(func(rep Reporter) { rep.PrintRequestResult(reqResult) })
			if opts.Bail {
				summary.Bailed = true
				break
			}
			continue
		}

		// ── Execute HTTP request (with retry on network error) ───────────────
		maxAttempts := 1 + opts.RetryCount
		var execResult *types.ExecutionResult
		var execErr error
		var retryCount int

		for attempt := 0; attempt < maxAttempts; attempt++ {
			if attempt > 0 {
				select {
				case <-ctx.Done():
					execErr = ctx.Err()
					goto afterRetry
				case <-time.After(time.Duration(attempt*500) * time.Millisecond):
				}
			}
			execResult, execErr = r.client.Execute(ctx, interpolatedReq)
			retryCount = attempt
			if execErr == nil || execResult != nil {
				break // success or partial result
			}
			// only retry on network errors
			if !isNetworkError(execErr) {
				break
			}
		}
	afterRetry:

		reqDuration := time.Since(reqStart)

		if execErr != nil && execResult == nil {
			hasNetworkError = true
			reqResult := RequestRunResult{
				FilePath: filePath, RelPath: relPath, Request: req,
				Passed:     false,
				Error:      fmt.Sprintf("Network execution error: %v", execErr),
				Duration:   reqDuration,
				RetryCount: retryCount,
			}
			summary.TotalRequests++
			summary.FailedRequests++
			summary.Results = append(summary.Results, reqResult)
			broadcast(func(rep Reporter) { rep.PrintRequestResult(reqResult) })
			if opts.Bail {
				summary.Bailed = true
				break
			}
			continue
		}

		// ── Post-response scripts ───────────────────────────────────────────
		if r.scriptEngine != nil {
			scriptTimeout := scriptTimeoutFor(interpolatedReq.Settings.ScriptTimeoutMs)
			for _, s := range postScripts {
				postResult, _ := r.scriptEngine.ExecutePostResponseNamed(s.Source, s.Script, interpolatedReq, execResult, currentVarMap, scriptTimeout)
				if postResult != nil {
					execResult.Tests = append(execResult.Tests, postResult.Tests...)
					execResult.Logs = append(execResult.Logs, postResult.Logs...)
					execResult.ConsoleLogs = append(execResult.ConsoleLogs, postResult.ConsoleLogs...)
					for k, v := range postResult.ExtractedEnvVars {
						currentVarMap[k] = v
						varMap[k] = v
					}
				}
			}
		}

		// ── Evaluate pass/fail ──────────────────────────────────────────────
		passed := true
		var failureReason string

		if execResult.Error != "" {
			passed = false
			failureReason = execResult.Error
		}

		for _, test := range execResult.Tests {
			summary.TotalTests++
			if test.Passed {
				summary.PassedTests++
			} else {
				summary.FailedTests++
				passed = false
				hasAssertionFailure = true
				if failureReason == "" {
					failureReason = fmt.Sprintf("Assertion failed: %s (%s)", test.Name, test.Message)
				}
			}
		}

		summary.TotalRequests++
		if passed {
			summary.PassedRequests++
		} else {
			summary.FailedRequests++
		}

		// ── Mask secrets ────────────────────────────────────────────────────
		if len(secretValues) > 0 {
			failureReason = security.MaskSecrets(failureReason, secretValues)
			if execResult != nil {
				execResult.Body = security.MaskSecrets(execResult.Body, secretValues)
				for i, log := range execResult.Logs {
					execResult.Logs[i] = security.MaskSecrets(log, secretValues)
				}
				for i, t := range execResult.Tests {
					execResult.Tests[i].Message = security.MaskSecrets(t.Message, secretValues)
				}
			}
		}

		reqResult := RequestRunResult{
			FilePath: filePath, RelPath: relPath,
			Request: req, Result: execResult,
			Passed: passed, Error: failureReason,
			Duration: reqDuration, RetryCount: retryCount,
		}
		summary.Results = append(summary.Results, reqResult)
		broadcast(func(rep Reporter) { rep.PrintRequestResult(reqResult) })

		if !passed && opts.Bail {
			summary.Bailed = true
			break
		}
	}

	summary.TotalDuration = time.Since(startTime)
	summary.Success = (summary.FailedRequests == 0 && summary.FailedTests == 0 && !summary.Bailed)

	// Compute exit code
	switch {
	case hasConfigError && !hasNetworkError && !hasAssertionFailure:
		summary.ExitCode = ExitConfigError
	case hasNetworkError && !hasAssertionFailure:
		summary.ExitCode = ExitNetworkError
	case !summary.Success:
		summary.ExitCode = ExitAssertionFail
	default:
		summary.ExitCode = ExitSuccess
	}

	broadcast(func(rep Reporter) { rep.PrintSummary(summary) })

	return summary, nil
}

// discoverRequests recursively collects all *.pebble.json files within targetPath.
func (r *Runner) discoverRequests(targetPath string) ([]string, error) {
	fi, err := os.Stat(targetPath)
	if err != nil {
		return nil, err
	}

	if !fi.IsDir() {
		if strings.HasSuffix(targetPath, workspace.PebbleExt) {
			return []string{targetPath}, nil
		}
		return nil, fmt.Errorf("target file '%s' is not a %s file", targetPath, workspace.PebbleExt)
	}

	var files []string
	err = filepath.Walk(targetPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		name := info.Name()
		if info.IsDir() {
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(name, workspace.PebbleExt) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// findWorkspaceRoot searches upwards from path to locate a directory containing .pebble.
func (r *Runner) findWorkspaceRoot(path string) string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	fi, err := os.Stat(absPath)
	if err == nil && !fi.IsDir() {
		absPath = filepath.Dir(absPath)
	}
	curr := absPath
	for i := 0; i < 6; i++ {
		pebbleDir := filepath.Join(curr, workspace.PebbleDir)
		if fi, err := os.Stat(pebbleDir); err == nil && fi.IsDir() {
			return curr
		}
		parent := filepath.Dir(curr)
		if parent == curr || parent == "/" || parent == "." {
			break
		}
		curr = parent
	}
	return absPath
}

// scriptTimeoutFor converts the per-request ScriptTimeoutMs setting to a Duration.
func scriptTimeoutFor(ms int) time.Duration {
	if ms <= 0 {
		return scripting.DefaultScriptTimeout
	}
	return time.Duration(ms) * time.Millisecond
}

// isNetworkError returns true for errors that are worth retrying (network-level).
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "eof")
}
