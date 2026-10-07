package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"pebblepost"
	"pebblepost/internal/app"
	"pebblepost/internal/docs"
	"pebblepost/internal/impexp"
	"pebblepost/internal/mockserver"
	"pebblepost/internal/openapisync"
	"pebblepost/internal/runner"
	"pebblepost/internal/secrets"
	"pebblepost/internal/types"
	"pebblepost/internal/workspace"
)

var (
	version = "0.2.0"
	commit  = "none"
	date    = "unknown"
	builtBy = "source"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(runner.ExitConfigError)
	}

	switch os.Args[1] {
	case "run":
		handleRun(os.Args[2:])
	case "sync":
		handleSync(os.Args[2:])
	case "import":
		handleImport(os.Args[2:])
	case "docs":
		handleDocs(os.Args[2:])
	case "mock":
		handleMock(os.Args[2:])
	case "serve":
		handleServe(os.Args[2:])
	case "secrets":
		handleSecrets(os.Args[2:])
	case "version", "-v", "--version":
		fmt.Printf("PebblePost CLI Runner v%s (commit: %s, date: %s, built by: %s)\n", version, commit, date, builtBy)
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(runner.ExitConfigError)
	}
}

// multiFlag is a flag.Value implementation for repeatable flags (e.g. --reporter, --var).
type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ", ") }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func handleRun(args []string) {
	fs := newFlagSet()

	var (
		envFlag           = fs.String("e", "", "Environment name (e.g. dev, staging, prod)")
		bailFlag          = fs.Bool("bail", false, "Stop execution immediately on first failure")
		dryRunFlag        = fs.Bool("dry-run", false, "Print request order without sending any HTTP requests")
		timeoutFlag       = fs.Int("timeout", 0, "Global request timeout in milliseconds (0 = per-request setting)")
		retryFlag         = fs.Int("retry", 0, "Number of retries on network errors (not assertion failures)")
		delayFlag         = fs.Int("delay", 0, "Delay between requests in milliseconds")
		dataFlag          = fs.String("data", "", "Path to data file (CSV or JSON) for data-driven iterations")
		iterationsFlag    = fs.Int("iterations", 0, "Number of iterations to run (0 = auto-detect from data file or 1)")
		envFileFlag       = fs.String("env-file", "", "Path to additional environment variable file (KEY=VALUE format)")
		folderFlag        = fs.String("folder", "", "Glob pattern to filter requests by folder path")
		requestFlag       = fs.String("request", "", "Glob pattern to filter requests by name or path")
		tagFlag           = fs.String("tag", "", "Filter requests by tag (exact match)")
		outFlag           = fs.String("out", "", "Output file path for the last --reporter (shorthand for reporter:path)")
		ciFlag            = fs.Bool("ci", false, "CI/CD mode (alias for --bail, preserved for backward compatibility)")
		trustFlag         = fs.Bool("trust", false, "Trust and execute pre-request and test scripts in collections")
		secretBackendFlag = fs.String("secret-backend", "", "Secret storage backend to use: file, keychain, env (default: from workspace.json or file)")
	)

	fs.StringVar(dataFlag, "d", "", "Path to data file (shorthand)")
	fs.IntVar(iterationsFlag, "n", 0, "Number of iterations to run (shorthand)")

	var reporterFlags multiFlag
	var varFlags multiFlag
	fs.Var(&reporterFlags, "reporter", "Reporter format: cli, json, junit, html. Repeatable. Append :path to write to file (e.g. junit:results.xml).")
	fs.Var(&varFlags, "var", "Extra variable KEY=VALUE (repeatable, highest precedence).")

	reordered := reorderArgs(args)
	if err := fs.Parse(reordered); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		os.Exit(runner.ExitConfigError)
	}

	// ── Target path ───────────────────────────────────────────────────────────
	targetPath := "."
	if fs.NArg() >= 1 {
		targetPath = fs.Arg(0)
	} else {
		if fi, err := os.Stat("collections"); err == nil && fi.IsDir() {
			targetPath = "collections"
		}
	}

	// ── Script Trust ──────────────────────────────────────────────────────────
	trusted := *trustFlag || isCollectionTrusted(targetPath) || os.Getenv("PEBBLEPOST_TRUST") == "1" || os.Getenv("PEBBLEPOST_TRUST_SCRIPTS") == "1"
	if !trusted {
		fmt.Fprintln(os.Stderr, "Notice: Scripts are disabled for untrusted collection. Pass --trust or create a .pebbletrust file to enable script execution.")
	}

	// ── Parse --var flags ─────────────────────────────────────────────────────
	extraVars, err := runner.ParseVarFlags(varFlags)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(runner.ExitConfigError)
	}

	// ── Parse --reporter flags ────────────────────────────────────────────────
	if len(reporterFlags) == 0 {
		reporterFlags = []string{"cli"}
	}
	reporters, err := runner.ReporterConfigsFromFlags(reporterFlags)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(runner.ExitConfigError)
	}

	// Apply --out to the last reporter if it has no path yet
	if *outFlag != "" && len(reporters) > 0 {
		last := &reporters[len(reporters)-1]
		if last.OutPath == "" {
			last.OutPath = *outFlag
		}
	}

	// --ci is a legacy alias for --bail
	bail := *bailFlag || *ciFlag

	// ── Signal handling ───────────────────────────────────────────────────────
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	// ── Run ───────────────────────────────────────────────────────────────────
	r := runner.NewRunner()
	summary, runErr := r.Run(ctx, runner.RunOptions{
		TargetPath:      targetPath,
		EnvironmentName: *envFlag,
		Bail:            bail,
		DryRun:          *dryRunFlag,
		TimeoutMs:       *timeoutFlag,
		RetryCount:      *retryFlag,
		DelayMs:         *delayFlag,
		DataFile:        *dataFlag,
		Iterations:      *iterationsFlag,
		EnvFile:         *envFileFlag,
		ExtraVars:       extraVars,
		Reporters:       reporters,
		TrustScripts:    &trusted,
		SecretBackend:   *secretBackendFlag,
		Filter: runner.FilterOptions{
			Folder:  *folderFlag,
			Request: *requestFlag,
			Tag:     *tagFlag,
		},
	})

	if runErr != nil {
		fmt.Fprintf(os.Stderr, "Execution error: %v\n", runErr)
		os.Exit(runner.ExitConfigError)
	}

	os.Exit(summary.ExitCode)
}

func handleImport(args []string) {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		outDirFlag    = fs.String("out-dir", "", "Target directory relative to workspace (default: collections/<collection-name>)")
		formatFlag    = fs.String("format", "", "Import format: bruno, insomnia, har, postman, openapi, curl (default: auto-detect)")
		workspaceFlag = fs.String("workspace", ".", "Workspace root directory (default: current directory)")
		jsonFlag      = fs.Bool("json", false, "Output import report as JSON to stdout")
	)

	// Short aliases
	fs.StringVar(outDirFlag, "o", "", "Alias for --out-dir")
	fs.StringVar(formatFlag, "f", "", "Alias for --format")

	reordered := reorderArgs(args)
	if err := fs.Parse(reordered); err != nil {
		os.Exit(runner.ExitConfigError)
	}

	if fs.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Error: missing source file or directory to import\n\n")
		fmt.Fprintf(os.Stderr, "Usage: pebblepost import <file-or-dir> [flags]\n")
		os.Exit(runner.ExitConfigError)
	}

	sourcePath := fs.Arg(0)
	svc := impexp.NewService()

	result, err := svc.ParseFile(sourcePath, impexp.Format(*formatFlag))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Import error: %v\n", err)
		os.Exit(runner.ExitConfigError)
	}

	report, err := svc.SaveToWorkspace(*workspaceFlag, *outDirFlag, result)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to save imported collection to workspace: %v\n", err)
		os.Exit(runner.ExitConfigError)
	}

	if *jsonFlag {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report)
		return
	}

	// Human-readable CLI summary report
	fmt.Println("────────────────────────────────────────────────────────────────")
	fmt.Printf("✓ Import Successful: %s\n", report.CollectionName)
	fmt.Printf("  • Format:           %s\n", report.SourceFormat)
	fmt.Printf("  • Requests:         %d imported\n", report.TotalRequests)
	if report.TotalFolders > 0 {
		fmt.Printf("  • Folders:          %d created\n", report.TotalFolders)
	}
	if report.TotalVariables > 0 {
		fmt.Printf("  • Variables:        %d mapped\n", report.TotalVariables)
	}

	if len(report.Warnings) > 0 {
		fmt.Println("\nWarnings:")
		for _, w := range report.Warnings {
			fmt.Printf("  ⚠ %s\n", w)
		}
	}

	if len(report.Skipped) > 0 {
		fmt.Printf("\nSkipped Items (%d):\n", len(report.Skipped))
		for _, sk := range report.Skipped {
			fmt.Printf("  ✗ %s: %s\n", sk.Name, sk.Reason)
		}
	}
	fmt.Println("────────────────────────────────────────────────────────────────")
}

func handleDocs(args []string) {
	fs := flag.NewFlagSet("docs", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		formatFlag    = fs.String("format", "md", "Output format: md, html, openapi, json (default: md)")
		formatShort   = fs.String("f", "", "Alias for --format")
		outFlag       = fs.String("out", "", "Output directory or file path (default: docs/)")
		outShort      = fs.String("o", "", "Alias for --out")
		stdoutFlag    = fs.Bool("stdout", false, "Print documentation directly to stdout")
		workspaceFlag = fs.String("workspace", ".", "Workspace root directory")
	)

	reordered := reorderArgs(args)
	if err := fs.Parse(reordered); err != nil {
		os.Exit(runner.ExitConfigError)
	}

	targetPath := "."
	if fs.NArg() > 0 {
		targetPath = fs.Arg(0)
	}

	format := *formatFlag
	if *formatShort != "" {
		format = *formatShort
	}
	outPath := *outFlag
	if *outShort != "" {
		outPath = *outShort
	}

	gen := docs.NewGenerator()

	if *stdoutFlag {
		data, _, err := gen.Generate(*workspaceFlag, targetPath, format)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error generating documentation: %v\n", err)
			os.Exit(runner.ExitConfigError)
		}
		os.Stdout.Write(data)
		return
	}

	writtenPath, err := gen.WriteDocs(*workspaceFlag, targetPath, format, outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write documentation: %v\n", err)
		os.Exit(runner.ExitConfigError)
	}

	fmt.Println("────────────────────────────────────────────────────────────────")
	fmt.Printf("✓ Documentation Generated Successfully\n")
	fmt.Printf("  • Target:   %s\n", targetPath)
	fmt.Printf("  • Format:   %s\n", strings.ToLower(format))
	fmt.Printf("  • Output:   %s\n", writtenPath)
	fmt.Println("────────────────────────────────────────────────────────────────")
}

func printUsage() {
	fmt.Printf(`PebblePost CLI Runner v%s
A local-first, Git-friendly API test runner for CI/CD pipelines.

Usage:
  pebblepost run [<path-to-collection>] [flags]
  pebblepost sync [--check|--apply] [<path-to-folder>] [flags]
  pebblepost secrets <list|get|set|delete|migrate|rollback> [flags]
  pebblepost import <file-or-dir> [flags]
  pebblepost docs [<path-to-collection>] [flags]
  pebblepost mock [<path-to-collection>] [flags]
  pebblepost version
  pebblepost help

Commands:
  run          Execute API requests and test assertions in a collection
  sync         Synchronize a collection folder with an OpenAPI specification
  secrets      Manage and migrate secrets across storage backends (file, keychain, env)
  import       Import collections from Postman, Bruno, Insomnia, HAR, OpenAPI, or cURL
  docs         Generate API documentation from collections in Markdown, HTML, or OpenAPI
  mock         Start a mock server serving saved Examples from a collection
  version      Print version information
  help         Print this help message

Flags for 'secrets':
  list         pebblepost secrets list [--env <name>] [--backend <type>]
  get          pebblepost secrets get <key> [--env <name>] [--backend <type>]
  set          pebblepost secrets set <key> <value> [--env <name>] [--backend <type>]
  delete       pebblepost secrets delete <key> [--env <name>] [--backend <type>]
  migrate      pebblepost secrets migrate --env <name> --from <type> --to <type> [--yes]
  rollback     pebblepost secrets rollback --env <name> --backup <path>

Flags for 'sync':
      --check               Check for drift against OpenAPI spec (exits 1 on drift for CI gates)
      --apply               Apply non-conflicting spec changes to collection folder
      --spec <path|url>     OpenAPI spec location (defaults to link stored in _folder.pebble.json)
      --force               Force apply conflicting changes (e.g. delete endpoints with user scripts)
      --json                Output drift report as JSON

Flags for 'mock':
  -p, --port <port>         Port to listen on (default: 8080)
      --host <host>         Host interface to bind to (default: 127.0.0.1)
      --delay <ms>          Global simulated latency in milliseconds
      --status <code>       Global HTTP status code override
      --error-rate <rate>   Global simulated error-injection rate (0.0 to 1.0)

Flags for 'docs':
  -f, --format <format>     Documentation format: md (default), html, openapi, json
  -o, --out <path>          Output directory or file path (default: docs/)
      --stdout              Print documentation directly to stdout
      --workspace <path>    Workspace root directory (default: .)

Flags for 'import':
  -o, --out-dir <path>      Output directory relative to workspace (default: collections/<name>)
  -f, --format <format>     Explicit format: bruno, insomnia, har, postman, openapi, curl
      --workspace <path>    Workspace root directory (default: .)
      --json                Output import report as JSON to stdout

Flags for 'run':
  -e <name>                 Environment configuration name (e.g. dev, staging, prod)
  --secret-backend <type>   Secret storage backend: file, keychain, env (default: from workspace.json or file)
  -d, --data <file>         Data file (CSV or JSON) for data-driven iterations
  -n, --iterations <n>      Number of iterations to run (auto-detected if data file provided)
  --reporter <format>       Reporter: cli (default), json, junit, html. Repeatable.
                            Append :<path> to write to file: --reporter junit:results.xml
  --out <path>              Output file path for the last --reporter
  --var KEY=VALUE           Extra variable (repeatable, overrides all other variable sources)
  --env-file <path>         Path to KEY=VALUE env file (lower precedence than --var)
  --folder <glob>           Filter: run only requests in matching folder path
  --request <glob>          Filter: run only requests matching name or path
  --tag <tag>               Filter: run only requests with the given tag
  --timeout <ms>            Global request timeout in milliseconds
  --retry <n>               Retry count for network errors (not assertion failures)
  --delay <ms>              Delay between requests in milliseconds
  --bail                    Stop immediately on the first failure
  --dry-run                 Print request order without sending HTTP requests
  --ci                      CI/CD mode (alias for --bail)
  --trust                   Trust and execute pre-request and test scripts in collections

Exit Codes:
  0   Success (all requests passed or import successful)
  1   One or more assertion failures
  2   Configuration or parse error
  3   Network error or timeout

Examples:
  pebblepost import ./postman_collection.json
  pebblepost import ./my-bruno-collection --out-dir collections/ecommerce
  pebblepost import ./traffic.har --json
  pebblepost run ./collections -e dev
  pebblepost run ./collections -d users.csv -n 5
  pebblepost run ./collections -e prod --bail --reporter junit:results.xml
`, version)
}

// newFlagSet creates a FlagSet in ContinueOnError mode so we can handle errors ourselves.
func newFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

// reorderArgs moves flag arguments before positional arguments for flag.FlagSet compatibility.
func reorderArgs(args []string) []string {
	// Known flags that consume the next argument as their value
	valueFlags := map[string]bool{
		"-e": true, "--env": true, "--reporter": true, "--out": true, "--var": true,
		"--env-file": true, "--folder": true, "--request": true, "--tag": true,
		"--timeout": true, "--retry": true, "--delay": true,
		"-d": true, "--data": true, "-n": true, "--iterations": true,
		"--out-dir": true, "-o": true, "--format": true, "-f": true, "--workspace": true, "-w": true,
		"--port": true, "-p": true, "--host": true, "--status": true, "--error-rate": true,
		"--spec": true, "--secret-backend": true,
		"-b": true, "--backend": true, "--from": true, "--to": true, "--backup": true,
	}

	var flags []string
	var pos []string
	i := 0
	for i < len(args) {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			if valueFlags[arg] && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				flags = append(flags, args[i+1])
				i++
			}
		} else {
			pos = append(pos, arg)
		}
		i++
	}
	return append(flags, pos...)
}

func handleMock(args []string) {
	fs := flag.NewFlagSet("mock", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	portFlag := fs.Int("port", 8080, "Port to listen on (default: 8080)")
	fs.IntVar(portFlag, "p", 8080, "Port to listen on (shorthand)")
	hostFlag := fs.String("host", "127.0.0.1", "Host interface to bind to (default: 127.0.0.1)")
	delayFlag := fs.Int64("delay", 0, "Global simulated latency in milliseconds")
	statusFlag := fs.Int("status", 0, "Global HTTP status code override")
	errorRateFlag := fs.Float64("error-rate", 0.0, "Global simulated error-injection rate (0.0 to 1.0)")

	reordered := reorderArgs(args)
	if err := fs.Parse(reordered); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		os.Exit(runner.ExitConfigError)
	}

	targetPath := "."
	if fs.NArg() > 0 {
		targetPath = fs.Arg(0)
	}

	host := *hostFlag
	isLoopback := host == "127.0.0.1" || host == "localhost" || host == "::1" || host == ""
	if !isLoopback {
		fmt.Fprintf(os.Stderr, "⚠️  WARNING: Mock server is bound to non-loopback host %s.\n", host)
		fmt.Fprintln(os.Stderr, "    This exposes your mock endpoints to external network access.")
	}

	cfg := mockserver.ServerConfig{
		Host:             host,
		Port:             *portFlag,
		WorkspacePath:    targetPath,
		TargetPath:       targetPath,
		GlobalDelayMs:    *delayFlag,
		GlobalStatusCode: *statusFlag,
		GlobalErrorRate:  *errorRateFlag,
	}

	srv := mockserver.NewServer(cfg, nil)
	if err := srv.LoadRoutes(targetPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error loading mock routes: %v\n", err)
		os.Exit(runner.ExitConfigError)
	}

	status := srv.Status()
	if status.RoutesCount == 0 {
		fmt.Printf("⚠️  No requests with saved examples found in %s.\n", targetPath)
		fmt.Println("   Save examples on your requests in PebblePost to serve mock responses.")
		os.Exit(0)
	}

	if err := srv.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start mock server: %v\n", err)
		os.Exit(runner.ExitNetworkError)
	}
	defer srv.Stop()

	fmt.Printf("\n🚀 PebblePost Mock Server running at %s\n", srv.URL())
	fmt.Printf("📂 Target: %s (%d routes loaded)\n\n", targetPath, status.RoutesCount)
	fmt.Printf("   %-8s %-32s %s\n", "METHOD", "PATH", "EXAMPLES")
	fmt.Println("   " + strings.Repeat("─", 65))
	for _, r := range status.Routes {
		exStr := strings.Join(r.ExampleNames, ", ")
		if len(exStr) > 30 {
			exStr = exStr[:27] + "..."
		}
		fmt.Printf("   %-8s %-32s %s\n", r.Method, r.PathPattern, exStr)
	}
	fmt.Printf("\nPress Ctrl+C to stop. Streaming incoming requests:\n\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logChan := srv.SubscribeLogs(ctx)
	go func() {
		for logEntry := range logChan {
			matchedStr := "MATCHED"
			if !logEntry.Matched {
				matchedStr = "UNMATCHED"
			}
			ts := logEntry.Timestamp.Format("15:04:05")
			fmt.Printf("[%s] %-7s %-25s -> %d %-9s [%dms] (%s)\n",
				ts, logEntry.Method, logEntry.Path, logEntry.StatusCode,
				matchedStr, logEntry.DurationMs, logEntry.MatchedExample)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\nShutting down mock server...")
}

func handleSync(args []string) {
	if len(args) == 0 {
		printSyncUsage()
		os.Exit(runner.ExitConfigError)
	}

	if args[0] == "link" {
		handleSyncLink(args[1:])
		return
	}

	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	checkFlag := fs.Bool("check", false, "Check for drift against OpenAPI spec (exits 1 if drift detected)")
	applyFlag := fs.Bool("apply", false, "Apply non-conflicting spec changes to collection")
	specFlag := fs.String("spec", "", "OpenAPI spec file path or remote URL")
	forceFlag := fs.Bool("force", false, "Force apply conflicting changes (e.g. deleting endpoints with user scripts)")
	jsonFlag := fs.Bool("json", false, "Output drift report as JSON")

	reordered := reorderArgs(args)
	if err := fs.Parse(reordered); err != nil {
		fmt.Fprintf(os.Stderr, "Flag error: %v\n", err)
		os.Exit(runner.ExitConfigError)
	}

	targetPath := "."
	if fs.NArg() > 0 {
		targetPath = fs.Arg(0)
	}

	isCheck := *checkFlag
	isApply := *applyFlag
	if !isCheck && !isApply {
		isCheck = true
	}

	wsSvc := workspace.NewWorkspaceService()
	specLocation := *specFlag

	if specLocation == "" {
		folderDef, err := wsSvc.ReadFolder(targetPath)
		if err == nil && folderDef != nil && folderDef.OpenAPISync != nil {
			specLocation = folderDef.OpenAPISync.SpecLocation
		}
	}

	if specLocation == "" {
		fmt.Fprintf(os.Stderr, "Error: No OpenAPI specification linked to folder %q.\n", targetPath)
		fmt.Fprintf(os.Stderr, "Provide --spec <path|url> or link folder first: pebblepost sync link %s <spec>\n", targetPath)
		os.Exit(runner.ExitConfigError)
	}

	specData, specHash, err := openapisync.LoadSpec(specLocation, targetPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading OpenAPI spec %q: %v\n", specLocation, err)
		os.Exit(runner.ExitConfigError)
	}

	spec, err := openapisync.ParseSpec(specData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing OpenAPI spec: %v\n", err)
		os.Exit(runner.ExitConfigError)
	}

	report, err := openapisync.CompareSpecAndFolder(targetPath, spec, specHash, specLocation)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error calculating OpenAPI diff: %v\n", err)
		os.Exit(runner.ExitConfigError)
	}

	if *jsonFlag {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report)
		if isCheck && report.HasDrift {
			os.Exit(1)
		}
		return
	}

	if isCheck {
		printSyncCheckReport(report)
		if report.HasDrift {
			os.Exit(1) // CI exit code 1 for drift
		}
		return
	}

	if isApply {
		res, err := openapisync.ApplyDiff(wsSvc, targetPath, report, nil, *forceFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error applying OpenAPI diff: %v\n", err)
			os.Exit(runner.ExitConfigError)
		}
		printSyncApplyResult(report, res)
	}
}

func handleSyncLink(args []string) {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: pebblepost sync link <folder> <spec-path-or-url>")
		os.Exit(runner.ExitConfigError)
	}
	folderPath := args[0]
	specLocation := args[1]

	specData, specHash, err := openapisync.LoadSpec(specLocation, folderPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading OpenAPI spec: %v\n", err)
		os.Exit(runner.ExitConfigError)
	}
	_, err = openapisync.ParseSpec(specData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid OpenAPI 3.x spec: %v\n", err)
		os.Exit(runner.ExitConfigError)
	}

	wsSvc := workspace.NewWorkspaceService()
	folderDef, err := wsSvc.ReadFolder(folderPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading folder %s: %v\n", folderPath, err)
		os.Exit(runner.ExitConfigError)
	}

	if folderDef.OpenAPISync == nil {
		folderDef.OpenAPISync = &types.OpenAPISyncConfig{}
	}
	folderDef.OpenAPISync.SpecLocation = specLocation
	folderDef.OpenAPISync.SpecHash = specHash
	folderDef.OpenAPISync.LastSyncedAt = time.Now().UTC().Format(time.RFC3339)

	if err := wsSvc.SaveFolder(folderPath, folderDef); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving folder metadata: %v\n", err)
		os.Exit(runner.ExitConfigError)
	}

	shortHash := specHash
	if len(shortHash) > 12 {
		shortHash = shortHash[:12]
	}
	fmt.Printf("✔ Linked folder %q to OpenAPI spec %q (hash: %s)\n", folderPath, specLocation, shortHash)
}

func printSyncUsage() {
	fmt.Println(`Usage:
  pebblepost sync --check <folder> [--spec <path|url>]
  pebblepost sync --apply <folder> [--force]
  pebblepost sync link <folder> <spec-path-or-url>

Options:
  --check      Check for drift between folder and OpenAPI spec (exits 1 on drift)
  --apply      Apply non-conflicting changes to folder
  --spec       Path or URL to OpenAPI spec (overrides linked spec in folder)
  --force      Apply conflicting removals (e.g. deleting requests with scripts)
  --json       Output diff report as JSON`)
}

func printSyncCheckReport(report *openapisync.SyncDiffReport) {
	fmt.Println("════════════════════════════════════════════════════════════════")
	fmt.Println("  OpenAPI Synchronization Drift Report")
	fmt.Printf("  Folder: %s\n", report.FolderPath)
	fmt.Printf("  Spec:   %s\n", report.SpecLocation)
	fmt.Println("════════════════════════════════════════════════════════════════")

	if !report.HasDrift {
		fmt.Println("\n✔ Collection is in sync with OpenAPI specification. No drift detected.")
		return
	}

	if report.AddedCount > 0 {
		fmt.Printf("\n  Added Endpoints (+%d):\n", report.AddedCount)
		for _, ep := range report.Endpoints {
			if ep.DiffType == openapisync.DiffAdded {
				fmt.Printf("    + %-7s %s (%s)\n", ep.Method, ep.Path, ep.Summary)
			}
		}
	}

	if report.ChangedCount > 0 {
		fmt.Printf("\n  Changed Endpoints (~%d):\n", report.ChangedCount)
		for _, ep := range report.Endpoints {
			if ep.DiffType == openapisync.DiffChanged {
				fmt.Printf("    ~ %-7s %s (%s)\n", ep.Method, ep.Path, ep.Summary)
				for _, fd := range ep.FieldDiffs {
					fmt.Printf("        • %s: %s\n", fd.Field, fd.Description)
				}
			}
		}
	}

	if report.RemovedCount > 0 {
		fmt.Printf("\n  Removed Endpoints (-%d):\n", report.RemovedCount)
		for _, ep := range report.Endpoints {
			if ep.DiffType == openapisync.DiffRemoved {
				fmt.Printf("    - %-7s %s (%s)\n", ep.Method, ep.Path, ep.Summary)
				if ep.Conflict != nil {
					fmt.Printf("        ⚠️  CONFLICT: %s\n", ep.Conflict.Details)
				}
			}
		}
	}

	fmt.Println("\n────────────────────────────────────────────────────────────────")
	fmt.Printf("  Drift detected: +%d added, ~%d changed, -%d removed (%d conflict(s))\n",
		report.AddedCount, report.ChangedCount, report.RemovedCount, report.ConflictCount)
}

func printSyncApplyResult(report *openapisync.SyncDiffReport, res *openapisync.ApplyResult) {
	fmt.Println("════════════════════════════════════════════════════════════════")
	fmt.Println("  OpenAPI Synchronization Apply Result")
	fmt.Printf("  Folder: %s\n", report.FolderPath)
	fmt.Println("════════════════════════════════════════════════════════════════")
	fmt.Printf("  Applied: %d change(s)\n", res.AppliedCount)
	fmt.Printf("  Skipped: %d change(s)\n", res.SkippedCount)

	if len(res.CreatedFiles) > 0 {
		fmt.Println("\n  Created Files:")
		for _, f := range res.CreatedFiles {
			fmt.Printf("    + %s\n", f)
		}
	}

	if len(res.UpdatedFiles) > 0 {
		fmt.Println("\n  Updated Files:")
		for _, f := range res.UpdatedFiles {
			fmt.Printf("    ~ %s\n", f)
		}
	}

	if len(res.DeletedFiles) > 0 {
		fmt.Println("\n  Deleted Files:")
		for _, f := range res.DeletedFiles {
			fmt.Printf("    - %s\n", f)
		}
	}

	if res.SkippedCount > 0 {
		fmt.Println("\n  Skipped Items (Conflicts):")
		for _, ep := range report.Endpoints {
			if ep.DiffType == openapisync.DiffRemoved && ep.Conflict != nil {
				fmt.Printf("    ! %s %s (contains user assets; run with --force to remove)\n", ep.Method, ep.Path)
			}
		}
	}

	fmt.Println("\n✔ Successfully synchronized collection folder with OpenAPI spec.")
}

func handleServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	host := fs.String("host", envOr("PEBBLEPOST_HOST", envOr("PEBBLE_HOST", "127.0.0.1")), "Interface to listen on.")
	port := fs.String("port", envOr("PORT", "8080"), "Port to listen on.")
	tokenEnv := envOr("PEBBLEPOST_TOKEN", os.Getenv("PEBBLE_TOKEN"))
	token := fs.String("token", tokenEnv, "Bearer token for API authentication.")
	dataDir := fs.String("data-dir", envOr("PEBBLEPOST_DATA_DIR", envOr("DATA_DIR", "./data")), "Data directory.")
	_ = fs.Parse(args)

	authToken := *token
	isLoopback := *host == "127.0.0.1" || *host == "::1" || *host == "localhost"
	if !isLoopback && authToken == "" {
		fmt.Fprintln(os.Stderr, "ERROR: --token or PEBBLEPOST_TOKEN is required when --host is not 127.0.0.1.")
		os.Exit(1)
	}
	if authToken == "" {
		authToken = generateToken()
	}

	inst, err := app.BootstrapWithToken(*dataDir, authToken)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap error: %v\n", err)
		os.Exit(1)
	}
	defer inst.Close()

	frontendFS, err := pebblepost.FrontendFS()
	if err == nil {
		inst.Mux.Handle("/", http.FileServer(http.FS(frontendFS)))
	}

	addr := fmt.Sprintf("%s:%s", *host, *port)
	fmt.Printf("PebblePost server listening on %s (token: %s)\n", addr, authToken)
	if err := http.ListenAndServe(addr, inst); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func generateToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "pebblepost-token-default"
	}
	return hex.EncodeToString(b)
}

func envOr(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

// isCollectionTrusted checks for a .pebbletrust file in targetPath or any parent directory.
func isCollectionTrusted(targetPath string) bool {
	candidates := []string{
		filepath.Join(targetPath, ".pebbletrust"),
		".pebbletrust",
	}
	if abs, err := filepath.Abs(targetPath); err == nil {
		dir := abs
		for {
			candidates = append(candidates, filepath.Join(dir, ".pebbletrust"))
			parent := filepath.Dir(dir)
			if parent == dir || parent == "" {
				break
			}
			dir = parent
		}
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return true
		}
	}
	return false
}

func handleSecrets(args []string) {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printSecretsUsage()
		return
	}

	subcmd := args[0]
	subargs := args[1:]

	fs := flag.NewFlagSet("secrets "+subcmd, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	envFlag := fs.String("env", "dev", "Environment name")
	fs.StringVar(envFlag, "e", "dev", "Alias for --env")
	backendFlag := fs.String("backend", "", "Secret backend (file, keychain, env)")
	fs.StringVar(backendFlag, "b", "", "Alias for --backend")
	workspaceFlag := fs.String("workspace", ".", "Workspace root path")
	fs.StringVar(workspaceFlag, "w", ".", "Alias for --workspace")
	fromFlag := fs.String("from", "file", "Source backend for migration (file, keychain, env)")
	toFlag := fs.String("to", "keychain", "Destination backend for migration (file, keychain, env)")
	yesFlag := fs.Bool("yes", false, "Confirm migration without interactive prompt")
	fs.BoolVar(yesFlag, "y", false, "Alias for --yes")
	backupFlag := fs.String("backup", "", "Backup file path for rollback")

	reordered := reorderArgs(subargs)
	if err := fs.Parse(reordered); err != nil {
		os.Exit(runner.ExitConfigError)
	}

	wsRoot := *workspaceFlag
	wsSvc := workspace.NewWorkspaceService()
	wsDef, _ := wsSvc.GetWorkspaceInfo(wsRoot)
	wsName := "pebblepost"
	if wsDef != nil && wsDef.Name != "" {
		wsName = wsDef.Name
	}

	getStore := func(bType string) (secrets.SecretStore, error) {
		var cfg secrets.SecretBackendConfig
		if wsDef != nil {
			cfg = secrets.FromTypesConfig(wsDef.SecretBackend.Default, wsDef.SecretBackend.PerEnv)
		}
		if bType != "" {
			cfg.Default = secrets.SecretBackendType(bType)
		}
		envDir := secrets.EnvDirFromRoot(wsRoot)
		return secrets.OpenStore(cfg, *envFlag, envDir, wsName)
	}

	switch subcmd {
	case "list":
		store, err := getStore(*backendFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening secret store: %v\n", err)
			os.Exit(runner.ExitConfigError)
		}
		keys, err := store.List(*envFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing secrets for %q: %v\n", *envFlag, err)
			os.Exit(runner.ExitConfigError)
		}
		fmt.Printf("Secrets for environment %q (backend: %s):\n", *envFlag, store.Backend())
		if len(keys) == 0 {
			fmt.Println("  (no secrets found)")
			return
		}
		for _, k := range keys {
			fmt.Printf("  • %s\n", k)
		}

	case "get":
		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "Usage: pebblepost secrets get <key> [--env <name>] [--backend <type>]")
			os.Exit(runner.ExitConfigError)
		}
		key := fs.Arg(0)
		store, err := getStore(*backendFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening secret store: %v\n", err)
			os.Exit(runner.ExitConfigError)
		}
		val, err := store.Get(*envFlag, key)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting secret %q: %v\n", key, err)
			os.Exit(runner.ExitConfigError)
		}
		fmt.Println(val)

	case "set":
		if fs.NArg() < 2 {
			fmt.Fprintln(os.Stderr, "Usage: pebblepost secrets set <key> <value> [--env <name>] [--backend <type>]")
			os.Exit(runner.ExitConfigError)
		}
		key := fs.Arg(0)
		val := fs.Arg(1)
		store, err := getStore(*backendFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening secret store: %v\n", err)
			os.Exit(runner.ExitConfigError)
		}
		if err := store.Set(*envFlag, key, val); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving secret %q: %v\n", key, err)
			os.Exit(runner.ExitConfigError)
		}
		fmt.Printf("✔ Saved secret %q to environment %q (backend: %s)\n", key, *envFlag, store.Backend())

	case "delete", "rm":
		if fs.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "Usage: pebblepost secrets delete <key> [--env <name>] [--backend <type>]")
			os.Exit(runner.ExitConfigError)
		}
		key := fs.Arg(0)
		store, err := getStore(*backendFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening secret store: %v\n", err)
			os.Exit(runner.ExitConfigError)
		}
		if err := store.Delete(*envFlag, key); err != nil {
			fmt.Fprintf(os.Stderr, "Error deleting secret %q: %v\n", key, err)
			os.Exit(runner.ExitConfigError)
		}
		fmt.Printf("✔ Deleted secret %q from environment %q\n", key, *envFlag)

	case "migrate":
		srcStore, err := getStore(*fromFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening source store: %v\n", err)
			os.Exit(runner.ExitConfigError)
		}
		dstStore, err := getStore(*toFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening destination store: %v\n", err)
			os.Exit(runner.ExitConfigError)
		}

		keys, err := srcStore.List(*envFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing source secrets: %v\n", err)
			os.Exit(runner.ExitConfigError)
		}
		if len(keys) == 0 {
			fmt.Printf("No secrets found in %s backend for environment %q to migrate.\n", srcStore.Backend(), *envFlag)
			return
		}

		fmt.Printf("Migration Plan:\n")
		fmt.Printf("  Environment:  %s\n", *envFlag)
		fmt.Printf("  Source:       %s\n", srcStore.Backend())
		fmt.Printf("  Destination:  %s\n", dstStore.Backend())
		fmt.Printf("  Secrets (%d):  %s\n\n", len(keys), strings.Join(keys, ", "))

		opts := secrets.MigrateOptions{
			Confirm: func(_ secrets.MigrationPlan) bool {
				if *yesFlag {
					return true
				}
				fmt.Print("Proceed with migration? [y/N]: ")
				reader := bufio.NewReader(os.Stdin)
				resp, _ := reader.ReadString('\n')
				resp = strings.TrimSpace(strings.ToLower(resp))
				return resp == "y" || resp == "yes"
			},
			Warn: func(msg string) {
				fmt.Fprintf(os.Stderr, "Warning: %s\n", msg)
			},
		}

		backupPath, err := secrets.Migrate(*envFlag, srcStore, dstStore, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Migration failed: %v\n", err)
			os.Exit(runner.ExitConfigError)
		}
		if backupPath == "" && !*yesFlag {
			fmt.Println("Migration aborted.")
			return
		}

		fmt.Println("✔ Migration complete!")
		if backupPath != "" {
			fmt.Printf("✔ Backup saved at: %s\n", backupPath)
			fmt.Printf("  Rollback command: pebblepost secrets rollback --env %s --backup %s\n", *envFlag, backupPath)
		}

	case "rollback":
		if *backupFlag == "" {
			fmt.Fprintln(os.Stderr, "Error: --backup <path> is required for rollback.")
			os.Exit(runner.ExitConfigError)
		}
		dstStore, _ := getStore("keychain")
		if err := secrets.Rollback(*envFlag, *backupFlag, dstStore); err != nil {
			fmt.Fprintf(os.Stderr, "Rollback failed: %v\n", err)
			os.Exit(runner.ExitConfigError)
		}
		fmt.Printf("✔ Rollback successful. Restored secrets from backup: %s\n", *backupFlag)

	default:
		fmt.Fprintf(os.Stderr, "Unknown secrets command: %s\n\n", subcmd)
		printSecretsUsage()
		os.Exit(runner.ExitConfigError)
	}
}

func printSecretsUsage() {
	fmt.Println(`Usage:
  pebblepost secrets <command> [flags]

Commands:
  list              List secret keys for an environment
  get <key>         Print decrypted secret value
  set <key> <val>   Store a secret in the configured backend
  delete <key>      Remove a secret
  migrate           Move secrets from one backend to another (e.g. file -> keychain)
  rollback          Restore a secret file from a backup

Flags:
  -e, --env <name>      Environment name (default: dev)
  -b, --backend <type>  Backend override: file, keychain, env (default: from workspace.json or file)
  -w, --workspace <dir> Workspace directory (default: .)
      --from <type>     Source backend for migration (default: file)
      --to <type>       Destination backend for migration (default: keychain)
  -y, --yes             Skip confirmation prompt for migration
      --backup <path>   Backup file path for rollback`)
}
