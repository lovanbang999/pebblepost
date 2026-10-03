package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"pebblepost/internal/httpclient"
	"pebblepost/internal/scripting"
	"pebblepost/internal/security"
	"pebblepost/internal/workspace"
)

// Runner manages CLI collection test executions.
type Runner struct {
	workspaceSvc        *workspace.WorkspaceService
	environmentSvc      *workspace.EnvironmentService
	interpolator        *workspace.Interpolator
	inheritanceResolver *workspace.InheritanceResolver
	client              httpclient.Client
	scriptEngine        *scripting.Engine
	formatter           *Formatter
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
		formatter:           NewFormatter(),
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
		formatter:           NewFormatter(),
	}
}

// Run executes the requests discovered at targetPath using the specified options.
func (r *Runner) Run(ctx context.Context, opts RunOptions) (*RunSummary, error) {
	out := opts.Writer
	if out == nil {
		out = os.Stdout
	}

	targetPath := opts.TargetPath
	if targetPath == "" {
		targetPath = "."
	}

	// 1. Discover *.pebble.json request files
	requestFiles, err := r.discoverRequests(targetPath)
	if err != nil {
		return nil, fmt.Errorf("request discovery error: %w", err)
	}
	if len(requestFiles) == 0 {
		return nil, fmt.Errorf("no *.pebble.json request files found in %s", targetPath)
	}

	// 2. Locate workspace root & initialize variable map
	wsRoot := r.findWorkspaceRoot(targetPath)
	if wsRoot != "" && r.client != nil {
		r.client.SetWorkspace(wsRoot)
	}
	varMap := make(map[string]string)
	var secretValues []string

	if opts.EnvironmentName != "" && r.environmentSvc != nil {
		envDef, err := r.environmentSvc.GetEnvironment(wsRoot, opts.EnvironmentName)
		if err == nil && envDef != nil {
			varMap = r.interpolator.BuildVariableMap(envDef, nil)
			for _, v := range envDef.Variables {
				if v.Secret && v.Value != "" {
					secretValues = append(secretValues, v.Value)
				}
			}
		} else {
			r.formatter.PrintWarning(out, fmt.Sprintf("Environment '%s' not found in workspace %s, proceeding with empty environment.", opts.EnvironmentName, wsRoot))
		}
	}

	if opts.ReportFormat != "json" {
		r.formatter.PrintHeader(out, targetPath, opts.EnvironmentName, len(requestFiles), opts.Bail)
	}

	summary := &RunSummary{
		Target:      targetPath,
		Environment: opts.EnvironmentName,
		Results:     make([]RequestRunResult, 0, len(requestFiles)),
	}

	startTime := time.Now()

	// 3. Execute each request in sequence
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

		reqStart := time.Now()
		req, err := r.workspaceSvc.ReadRequest(filePath)
		if err != nil {
			reqResult := RequestRunResult{
				FilePath: filePath,
				RelPath:  relPath,
				Passed:   false,
				Error:    fmt.Sprintf("Failed to read request file: %v", err),
				Duration: time.Since(reqStart),
			}
			summary.TotalRequests++
			summary.FailedRequests++
			summary.Results = append(summary.Results, reqResult)

			if opts.ReportFormat != "json" {
				r.formatter.PrintRequestFailure(out, relPath, "READ", reqResult.Error, reqResult.Duration)
			}

			if opts.Bail {
				summary.Bailed = true
				break
			}
			continue
		}

		// 1. Resolve folder chain for inheritance
		var folderChain []workspace.FolderChainItem
		if r.inheritanceResolver != nil {
			folderChain, _ = r.inheritanceResolver.DiscoverFolderChain(wsRoot, filePath)
		}

		// 2. Merge folder variables with current chained environment variables
		currentVarMap := make(map[string]string)
		for k, v := range varMap {
			currentVarMap[k] = v
		}
		if r.inheritanceResolver != nil && len(folderChain) > 0 {
			currentVarMap, _ = r.inheritanceResolver.MergeVariables(currentVarMap, folderChain, nil)
		}

		// 3. Merge headers and resolve auth
		reqToExecute := *req
		if r.inheritanceResolver != nil && len(folderChain) > 0 {
			mergedHeaders, _ := r.inheritanceResolver.MergeHeaders(folderChain, req.Headers)
			reqToExecute.Headers = mergedHeaders
			resolvedAuth, _ := r.inheritanceResolver.ResolveAuth(folderChain, req.Auth)
			reqToExecute.Auth = resolvedAuth
		}

		// 4. Interpolate request with current chained environment variables + folder variables
		interpolatedReq := r.interpolator.InterpolateRequest(&reqToExecute, currentVarMap)

		// 5. Build script chains
		var preScripts []workspace.ScriptChainItem
		var postScripts []workspace.ScriptChainItem
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

		// 6. Execute Pre-Request scripts (Root-to-Leaf)
		var preErrOccurred error
		if r.scriptEngine != nil {
			scriptTimeout := scriptTimeoutFor(interpolatedReq.Settings.ScriptTimeoutMs)
			for _, s := range preScripts {
				preResult, preErr := r.scriptEngine.ExecutePreRequestNamed(s.Source, s.Script, interpolatedReq, currentVarMap, scriptTimeout)
				if preResult != nil {
					for k, v := range preResult.ExtractedEnvVars {
						currentVarMap[k] = v
						varMap[k] = v // propagate to subsequent requests
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
			reqResult := RequestRunResult{
				FilePath: filePath,
				RelPath:  relPath,
				Request:  req,
				Passed:   false,
				Error:    preErrOccurred.Error(),
				Duration: time.Since(reqStart),
			}
			summary.TotalRequests++
			summary.FailedRequests++
			summary.Results = append(summary.Results, reqResult)

			if opts.ReportFormat != "json" {
				r.formatter.PrintRequestFailure(out, relPath, req.Method, reqResult.Error, reqResult.Duration)
			}

			if opts.Bail {
				summary.Bailed = true
				break
			}
			continue
		}

		// 7. Execute HTTP request
		execResult, execErr := r.client.Execute(ctx, interpolatedReq)
		reqDuration := time.Since(reqStart)

		if execErr != nil && execResult == nil {
			reqResult := RequestRunResult{
				FilePath: filePath,
				RelPath:  relPath,
				Request:  req,
				Passed:   false,
				Error:    fmt.Sprintf("Network execution error: %v", execErr),
				Duration: reqDuration,
			}
			summary.TotalRequests++
			summary.FailedRequests++
			summary.Results = append(summary.Results, reqResult)

			if opts.ReportFormat != "json" {
				r.formatter.PrintRequestFailure(out, relPath, req.Method, reqResult.Error, reqResult.Duration)
			}

			if opts.Bail {
				summary.Bailed = true
				break
			}
			continue
		}

		// 8. Execute Post-Response test scripts (Leaf-to-Root)
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

		// Evaluate pass/fail status
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
			FilePath: filePath,
			RelPath:  relPath,
			Request:  req,
			Result:   execResult,
			Passed:   passed,
			Error:    failureReason,
			Duration: reqDuration,
		}
		summary.Results = append(summary.Results, reqResult)

		if opts.ReportFormat != "json" {
			r.formatter.PrintRequestResult(out, reqResult)
		}

		if !passed && opts.Bail {
			summary.Bailed = true
			break
		}
	}

	summary.TotalDuration = time.Since(startTime)
	summary.Success = (summary.FailedRequests == 0 && summary.FailedTests == 0 && !summary.Bailed)

	if opts.ReportFormat == "json" {
		r.formatter.PrintJSON(out, summary)
	} else {
		r.formatter.PrintSummary(out, summary)
	}

	return summary, nil
}

// discoverRequests recursively collects all *.pebble.json files within targetPath.
func (r *Runner) discoverRequests(targetPath string) ([]string, error) {
	fi, err := os.Stat(targetPath)
	if err != nil {
		return nil, err
	}

	// Single request file execution
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

	// Sort files alphabetically to ensure deterministic test order
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

// scriptTimeoutFor converts the per-request ScriptTimeoutMs setting to a
// Duration. Zero or negative values fall back to the engine default (5 s).
func scriptTimeoutFor(ms int) time.Duration {
	if ms <= 0 {
		return scripting.DefaultScriptTimeout
	}
	return time.Duration(ms) * time.Millisecond
}
