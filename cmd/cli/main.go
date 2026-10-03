package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"pebblepost/internal/runner"
)

const version = "0.2.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(runner.ExitConfigError)
	}

	switch os.Args[1] {
	case "run":
		handleRun(os.Args[2:])
	case "version", "-v", "--version":
		fmt.Printf("PebblePost CLI Runner v%s\n", version)
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
		envFlag     = fs.String("e", "", "Environment name (e.g. dev, staging, prod)")
		bailFlag    = fs.Bool("bail", false, "Stop execution immediately on first failure")
		dryRunFlag  = fs.Bool("dry-run", false, "Print request order without sending any HTTP requests")
		timeoutFlag = fs.Int("timeout", 0, "Global request timeout in milliseconds (0 = per-request setting)")
		retryFlag   = fs.Int("retry", 0, "Number of retries on network errors (not assertion failures)")
		delayFlag   = fs.Int("delay", 0, "Delay between requests in milliseconds")
		envFileFlag = fs.String("env-file", "", "Path to additional environment variable file (KEY=VALUE format)")
		folderFlag  = fs.String("folder", "", "Glob pattern to filter requests by folder path")
		requestFlag = fs.String("request", "", "Glob pattern to filter requests by name or path")
		tagFlag     = fs.String("tag", "", "Filter requests by tag (exact match)")
		outFlag     = fs.String("out", "", "Output file path for the last --reporter (shorthand for reporter:path)")
		ciFlag      = fs.Bool("ci", false, "CI/CD mode (alias for --bail, preserved for backward compatibility)")
	)

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
		EnvFile:         *envFileFlag,
		ExtraVars:       extraVars,
		Reporters:       reporters,
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

func printUsage() {
	fmt.Printf(`PebblePost CLI Runner v%s
A local-first, Git-friendly API test runner for CI/CD pipelines.

Usage:
  pebblepost run [<path-to-collection>] [flags]
  pebblepost version
  pebblepost help

Commands:
  run          Execute API requests and test assertions in a collection
  version      Print version information
  help         Print this help message

Flags for 'run':
  -e <name>                 Environment configuration name (e.g. dev, staging, prod)
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

Exit Codes:
  0   All requests passed
  1   One or more assertion failures
  2   Configuration or parse error
  3   Network error or timeout

Examples:
  pebblepost run ./collections -e dev
  pebblepost run ./collections -e prod --bail --reporter junit:results.xml
  pebblepost run ./collections --reporter cli --reporter html:report.html
  pebblepost run ./collections --var BASE_URL=https://api.example.com --env-file .env
  pebblepost run ./collections --folder auth --tag smoke --retry 3 --timeout 5000
  pebblepost run ./collections --dry-run
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
		"-e": true, "--reporter": true, "--out": true, "--var": true,
		"--env-file": true, "--folder": true, "--request": true, "--tag": true,
		"--timeout": true, "--retry": true, "--delay": true,
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
