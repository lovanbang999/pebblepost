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
	"strconv"
	"strings"
	"time"

	"pebblepost/internal/extractor"
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
	if opts.CustomReporter != nil {
		reporters = append(reporters, reporterEntry{
			r:     opts.CustomReporter,
			w:     io.Discard,
			owned: false,
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

	// ── Resolve data rows & iterations ───────────────────────────────────────
	var dataRows []map[string]any
	if len(opts.DataRows) > 0 {
		dataRows = opts.DataRows
	} else if opts.DataFile != "" {
		parsedRows, parseErr := ParseDataFile(opts.DataFile)
		if parseErr != nil {
			return &RunSummary{
				Target:   opts.TargetPath,
				ExitCode: ExitConfigError,
				Success:  false,
			}, fmt.Errorf("failed to parse data file %q: %w", opts.DataFile, parseErr)
		}
		dataRows = parsedRows
	}

	totalIterations := ComputeIterations(opts.Iterations, len(dataRows))

	// ── Discover request files ────────────────────────────────────────────────
	targetPath := opts.TargetPath
	if targetPath == "" {
		targetPath = "."
	}

	var requestFiles []string
	var err error
	if len(opts.RequestPaths) > 0 {
		requestFiles = opts.RequestPaths
	} else {
		requestFiles, err = r.discoverRequests(targetPath)
		if err != nil {
			return nil, fmt.Errorf("request discovery error: %w", err)
		}
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

	// ── Preload request items for indexing ───────────────────────────────────
	type runnerItem struct {
		filePath string
		relPath  string
		rawReq   *types.RequestDefinition
		readErr  error
	}

	items := make([]runnerItem, len(requestFiles))
	for i, fp := range requestFiles {
		rel, _ := filepath.Rel(wsRoot, fp)
		if rel == "" {
			rel = fp
		}
		req, rErr := r.workspaceSvc.ReadRequest(fp)
		items[i] = runnerItem{
			filePath: fp,
			relPath:  rel,
			rawReq:   req,
			readErr:  rErr,
		}
	}

	findRequestIndex := func(target string) int {
		targetTrimmed := strings.TrimSpace(target)
		if targetTrimmed == "" {
			return -1
		}
		// 1. Match by ID
		for idx, it := range items {
			if it.rawReq != nil && it.rawReq.ID != "" && strings.EqualFold(it.rawReq.ID, targetTrimmed) {
				return idx
			}
		}
		// 2. Match by exact Name
		for idx, it := range items {
			if it.rawReq != nil && strings.EqualFold(strings.TrimSpace(it.rawReq.Name), targetTrimmed) {
				return idx
			}
		}
		// 3. Match by RelPath (with or without extension)
		for idx, it := range items {
			relClean := strings.TrimSuffix(it.relPath, workspace.PebbleExt)
			if strings.EqualFold(it.relPath, targetTrimmed) || strings.EqualFold(relClean, targetTrimmed) {
				return idx
			}
		}
		// 4. Match by base filename (with or without extension)
		for idx, it := range items {
			base := filepath.Base(it.filePath)
			baseClean := strings.TrimSuffix(base, workspace.PebbleExt)
			if strings.EqualFold(base, targetTrimmed) || strings.EqualFold(baseClean, targetTrimmed) {
				return idx
			}
		}
		return -1
	}

	// ── Print run header (CLI only; JSON/JUnit/HTML skip this) ───────────────
	broadcast(func(rep Reporter) {
		rep.PrintHeader(targetPath, opts.EnvironmentName, len(requestFiles), opts.Bail, opts.DryRun)
	})

	maxExecs := opts.MaxExecutionsPerIteration
	if maxExecs <= 0 {
		maxExecs = 100
	}

	summary := &RunSummary{
		Target:          targetPath,
		Environment:     opts.EnvironmentName,
		TotalIterations: totalIterations,
		Results:         make([]RequestRunResult, 0, len(items)*totalIterations),
		Iterations:      make([]IterationSummary, 0, totalIterations),
		DryRun:          opts.DryRun,
	}

	startTime := time.Now()

	hasNetworkError := false
	hasAssertionFailure := false
	hasConfigError := false
	var stopAllExecution bool

	// ── Iteration Loop ────────────────────────────────────────────────────────
	for iterIdx := 0; iterIdx < totalIterations; iterIdx++ {
		select {
		case <-ctx.Done():
			return summary, ctx.Err()
		default:
		}

		iterNumber := iterIdx + 1
		var dataRow map[string]any
		if iterIdx < len(dataRows) {
			dataRow = dataRows[iterIdx]
		}

		iterSummary := IterationSummary{
			Iteration: iterNumber,
			DataRow:   dataRow,
			Results:   make([]RequestRunResult, 0, len(items)),
			Passed:    true,
		}
		iterStartTime := time.Now()

		// Prepare per-iteration variables
		currentIterVarMap := make(map[string]string, len(varMap)+len(dataRow)+2)
		for k, v := range varMap {
			currentIterVarMap[k] = v
		}
		currentIterVarMap["$iteration"] = strconv.Itoa(iterNumber)
		currentIterVarMap["iteration"] = strconv.Itoa(iterNumber)

		// Inject data row variables: both "data.<key>" and "<key>" (data row takes precedence)
		for k, v := range dataRow {
			vStr := fmt.Sprintf("%v", v)
			currentIterVarMap["data."+k] = vStr
			currentIterVarMap[k] = vStr
		}

		iterCtx := &scripting.IterationContext{
			Iteration:      iterIdx,
			IterationCount: totalIterations,
			DataRow:        dataRow,
		}

		currIdx := 0
		execCount := 0
		loopDetected := false

		for currIdx >= 0 && currIdx < len(items) {
			select {
			case <-ctx.Done():
				iterSummary.Duration = time.Since(iterStartTime)
				summary.Iterations = append(summary.Iterations, iterSummary)
				return summary, ctx.Err()
			default:
			}

			execCount++
			if execCount > maxExecs {
				loopDetected = true
				hasConfigError = true
				loopErrMsg := fmt.Sprintf("Infinite loop detected: exceeded maximum executions (%d) in iteration %d", maxExecs, iterNumber)
				broadcast(func(rep Reporter) {
					rep.PrintWarning(loopErrMsg)
				})
				iterSummary.Passed = false
				iterSummary.Error = loopErrMsg
				break
			}

			item := items[currIdx]
			filePath := item.filePath
			relPath := item.relPath

			// ── Delay between requests ────────────────────────────────────────
			if len(summary.Results) > 0 && opts.DelayMs > 0 {
				select {
				case <-ctx.Done():
					iterSummary.Duration = time.Since(iterStartTime)
					summary.Iterations = append(summary.Iterations, iterSummary)
					return summary, ctx.Err()
				case <-time.After(time.Duration(opts.DelayMs) * time.Millisecond):
				}
			}

			reqStart := time.Now()

			// ── Read request error check ──────────────────────────────────────
			if item.readErr != nil {
				hasConfigError = true
				reqResult := RequestRunResult{
					Iteration: iterNumber,
					FilePath:  filePath,
					RelPath:   relPath,
					Passed:    false,
					Error:     fmt.Sprintf("Failed to read request file: %v", item.readErr),
					Duration:  time.Since(reqStart),
				}
				summary.TotalRequests++
				summary.FailedRequests++
				summary.Results = append(summary.Results, reqResult)
				iterSummary.Results = append(iterSummary.Results, reqResult)
				iterSummary.Passed = false
				broadcast(func(rep Reporter) { rep.PrintRequestResult(reqResult) })
				if opts.Bail {
					summary.Bailed = true
					stopAllExecution = true
					break
				}
				currIdx++
				continue
			}

			req := item.rawReq

			// ── Apply filter ──────────────────────────────────────────────────
			if !FilterRequest(relPath, req, opts.Filter) {
				skipResult := RequestRunResult{
					Iteration: iterNumber,
					FilePath:  filePath,
					RelPath:   relPath,
					Request:   req,
					Passed:    true,
					Skipped:   true,
					Duration:  time.Since(reqStart),
				}
				summary.SkippedRequests++
				summary.Results = append(summary.Results, skipResult)
				iterSummary.Results = append(iterSummary.Results, skipResult)
				broadcast(func(rep Reporter) { rep.PrintRequestResult(skipResult) })
				currIdx++
				continue
			}

			// ── Dry run: print but do not send ────────────────────────────────
			if opts.DryRun {
				method := req.Method
				dryResult := RequestRunResult{
					Iteration: iterNumber,
					FilePath:  filePath,
					RelPath:   relPath,
					Request:   req,
					Passed:    true,
					Skipped:   true,
					Duration:  0,
					Error:     fmt.Sprintf("dry-run: would send %s %s", method, req.URL),
				}
				summary.TotalRequests++
				summary.PassedRequests++
				summary.Results = append(summary.Results, dryResult)
				iterSummary.Results = append(iterSummary.Results, dryResult)
				broadcast(func(rep Reporter) { rep.PrintRequestResult(dryResult) })
				currIdx++
				continue
			}

			// ── Folder inheritance ────────────────────────────────────────────
			var folderChain []workspace.FolderChainItem
			if r.inheritanceResolver != nil {
				folderChain, _ = r.inheritanceResolver.DiscoverFolderChain(wsRoot, filePath)
			}

			currentVarMap := make(map[string]string, len(currentIterVarMap))
			for k, v := range currentIterVarMap {
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

			// ── Apply global timeout override ─────────────────────────────────
			if opts.TimeoutMs > 0 {
				reqToExecute.Settings.TimeoutMs = opts.TimeoutMs
			}

			interpolatedReq := r.interpolator.InterpolateRequest(&reqToExecute, currentVarMap)

			// ── Pre-request scripts ───────────────────────────────────────────
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

			var nextRequestTarget *string
			var nextRequestSet bool
			var preErrOccurred error

			canRunScripts := opts.TrustScripts == nil || *opts.TrustScripts

			if r.scriptEngine != nil && canRunScripts {
				scriptTimeout := scriptTimeoutFor(interpolatedReq.Settings.ScriptTimeoutMs)
				for _, s := range preScripts {
					preResult, preErr := r.scriptEngine.ExecutePreRequestWithContext(s.Source, s.Script, interpolatedReq, currentVarMap, iterCtx, scriptTimeout)
					if preResult != nil {
						for k, v := range preResult.ExtractedEnvVars {
							currentVarMap[k] = v
							currentIterVarMap[k] = v
							varMap[k] = v
						}
						interpolatedReq = preResult.Request
						if preResult.StopAll {
							stopAllExecution = true
						}
						if preResult.NextRequest != nil {
							nextRequestTarget = preResult.NextRequest
							nextRequestSet = true
						}
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
					Iteration: iterNumber,
					FilePath:  filePath, RelPath: relPath, Request: req,
					Passed: false, Error: preErrOccurred.Error(), Duration: time.Since(reqStart),
				}
				summary.TotalRequests++
				summary.FailedRequests++
				summary.Results = append(summary.Results, reqResult)
				iterSummary.Results = append(iterSummary.Results, reqResult)
				iterSummary.Passed = false
				broadcast(func(rep Reporter) { rep.PrintRequestResult(reqResult) })
				if opts.Bail {
					summary.Bailed = true
					stopAllExecution = true
					break
				}
				currIdx++
				continue
			}

			// ── Execute HTTP / gRPC / stream request (with retry on network error) ─
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
					break
				}
				if !isNetworkError(execErr) {
					break
				}
			}
		afterRetry:
			reqDuration := time.Since(reqStart)

			if execErr != nil && execResult == nil {
				hasNetworkError = true
				reqResult := RequestRunResult{
					Iteration: iterNumber,
					FilePath:  filePath, RelPath: relPath, Request: req,
					Passed:     false,
					Error:      fmt.Sprintf("Network execution error: %v", execErr),
					Duration:   reqDuration,
					RetryCount: retryCount,
				}
				summary.TotalRequests++
				summary.FailedRequests++
				summary.Results = append(summary.Results, reqResult)
				iterSummary.Results = append(iterSummary.Results, reqResult)
				iterSummary.Passed = false
				broadcast(func(rep Reporter) { rep.PrintRequestResult(reqResult) })
				if opts.Bail {
					summary.Bailed = true
					stopAllExecution = true
					break
				}
				currIdx++
				continue
			}

			// ── Response Extractors (run before assertion scripts) ────────────
			if execResult != nil && len(interpolatedReq.Extractors) > 0 {
				extractionResults, _, extractorWarnings := extractor.ExtractAll(interpolatedReq.Extractors, execResult.Body)
				if execResult.ExtractedEnvVars == nil {
					execResult.ExtractedEnvVars = make(map[string]string)
				}
				for _, res := range extractionResults {
					if !res.Success {
						continue
					}
					k, v := res.Name, res.Value
					execResult.ExtractedEnvVars[k] = v
					currentVarMap[k] = v
					currentIterVarMap[k] = v
					if strings.EqualFold(res.Scope, "environment") {
						varMap[k] = v
					}
				}
				for _, w := range extractorWarnings {
					execResult.Logs = append(execResult.Logs, "[WARN] "+w)
					execResult.ConsoleLogs = append(execResult.ConsoleLogs, types.ConsoleLogEntry{
						Timestamp: time.Now(),
						Level:     "warn",
						Source:    "extractor",
						Message:   w,
					})
					broadcast(func(rep Reporter) {
						rep.PrintWarning(w)
					})
				}
			}

			// ── Post-response scripts ─────────────────────────────────────────
			if !canRunScripts && (len(preScripts) > 0 || len(postScripts) > 0) {
				execResult.ConsoleLogs = append(execResult.ConsoleLogs, types.ConsoleLogEntry{
					Timestamp: time.Now(),
					Level:     "warn",
					Source:    "security",
					Message:   "Scripts skipped: collection is not trusted. Use --trust or a .pebbletrust file.",
				})
			}
			if r.scriptEngine != nil && canRunScripts {
				scriptTimeout := scriptTimeoutFor(interpolatedReq.Settings.ScriptTimeoutMs)
				for _, s := range postScripts {
					postResult, _ := r.scriptEngine.ExecutePostResponseWithContext(s.Source, s.Script, interpolatedReq, execResult, currentVarMap, iterCtx, scriptTimeout)
					if postResult != nil {
						execResult.Tests = append(execResult.Tests, postResult.Tests...)
						execResult.Logs = append(execResult.Logs, postResult.Logs...)
						execResult.ConsoleLogs = append(execResult.ConsoleLogs, postResult.ConsoleLogs...)
						for k, v := range postResult.ExtractedEnvVars {
							currentVarMap[k] = v
							currentIterVarMap[k] = v
							varMap[k] = v
						}
						if postResult.StopAll {
							stopAllExecution = true
						}
						if postResult.NextRequest != nil {
							nextRequestTarget = postResult.NextRequest
							nextRequestSet = true
						}
					}
				}
			}

			// ── Evaluate pass/fail ────────────────────────────────────────────
			passed := true
			var failureReason string

			if execResult.Error != "" {
				passed = false
				failureReason = execResult.Error
			}

			for _, test := range execResult.Tests {
				summary.TotalTests++
				iterSummary.TotalTests++
				if test.Passed {
					summary.PassedTests++
					iterSummary.PassedTests++
				} else {
					summary.FailedTests++
					iterSummary.FailedTests++
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
				iterSummary.Passed = false
			}

			// ── Mask secrets ──────────────────────────────────────────────────
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
				Iteration: iterNumber,
				FilePath:  filePath, RelPath: relPath,
				Request: req, Result: execResult,
				Passed: passed, Error: failureReason,
				Duration: reqDuration, RetryCount: retryCount,
			}
			summary.Results = append(summary.Results, reqResult)
			iterSummary.Results = append(iterSummary.Results, reqResult)
			broadcast(func(rep Reporter) { rep.PrintRequestResult(reqResult) })

			if !passed && opts.Bail {
				summary.Bailed = true
				stopAllExecution = true
				break
			}

			if stopAllExecution {
				break
			}

			// ── Determine next request index ──────────────────────────────────
			if nextRequestSet {
				if nextRequestTarget == nil || *nextRequestTarget == "" {
					// pb.runner.setNextRequest(null) -> finish current iteration early!
					break
				}
				targetName := *nextRequestTarget
				targetIdx := findRequestIndex(targetName)
				if targetIdx >= 0 {
					currIdx = targetIdx
				} else {
					broadcast(func(rep Reporter) {
						rep.PrintWarning(fmt.Sprintf("setNextRequest: request %q not found, continuing sequentially", targetName))
					})
					currIdx++
				}
			} else {
				currIdx++
			}
		}

		iterSummary.Duration = time.Since(iterStartTime)
		if loopDetected {
			iterSummary.Passed = false
		}
		if iterSummary.Passed {
			summary.PassedIterations++
		} else {
			summary.FailedIterations++
		}
		summary.Iterations = append(summary.Iterations, iterSummary)

		if stopAllExecution || summary.Bailed {
			break
		}
	}

	summary.TotalIterations = len(summary.Iterations)
	summary.TotalDuration = time.Since(startTime)
	summary.Success = (summary.FailedRequests == 0 && summary.FailedTests == 0 && !summary.Bailed && !hasConfigError && !hasNetworkError)

	// ── Aggregate Latency & Pass Rate ─────────────────────────────────────────
	durations := make([]time.Duration, 0, len(summary.Results))
	var sumDur time.Duration
	for _, res := range summary.Results {
		if !res.Skipped {
			durations = append(durations, res.Duration)
			sumDur += res.Duration
		}
	}
	summary.P95DurationMs = CalculateP95(durations)
	if len(durations) > 0 {
		summary.AvgDurationMs = float64(sumDur.Milliseconds()) / float64(len(durations))
	}
	executedCount := summary.TotalRequests - summary.SkippedRequests
	if executedCount > 0 {
		summary.PassRate = (float64(summary.PassedRequests) / float64(executedCount)) * 100.0
	} else if summary.TotalRequests > 0 {
		summary.PassRate = 100.0
	}

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
