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

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
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
		os.Exit(1)
	}
}

func handleRun(args []string) {
	runCmd := flag.NewFlagSet("run", flag.ExitOnError)
	envFlag := runCmd.String("e", "", "Environment name (e.g. dev, staging, prod)")
	bailFlag := runCmd.Bool("bail", false, "Stop execution immediately on first failed request or assertion")
	ciFlag := runCmd.Bool("ci", false, "CI/CD mode (synonym for strict non-zero exit codes)")
	reportFlag := runCmd.String("report", "terminal", "Output format: terminal or json")

	reordered := reorderArgs(args)
	_ = runCmd.Parse(reordered)

	targetPath := "."
	if runCmd.NArg() >= 1 {
		targetPath = runCmd.Arg(0)
	} else {
		// If ./collections exists, default to it
		if fi, err := os.Stat("collections"); err == nil && fi.IsDir() {
			targetPath = "collections"
		}
	}

	bail := *bailFlag
	_ = ciFlag // ciFlag enforces exit codes, which runner already does

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown on Ctrl+C / SIGTERM
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	r := runner.NewRunner()
	summary, err := r.Run(ctx, runner.RunOptions{
		TargetPath:      targetPath,
		EnvironmentName: *envFlag,
		Bail:            bail,
		ReportFormat:    *reportFlag,
		Writer:          os.Stdout,
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "Execution error: %v\n", err)
		os.Exit(1)
	}

	if !summary.Success {
		os.Exit(1)
	}

	os.Exit(0)
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
  -e <name>            Environment configuration name (e.g. dev, staging, prod)
  --bail               Stop immediately on the first failed assertion or request error
  --ci                 Run in continuous integration mode (exits 1 on any failure)
  --report <format>    Report format: 'terminal' (default) or 'json'

Examples:
  pebblepost run ./collections -e dev
  pebblepost run ./collections/auth/login.pebble.json -e staging
  pebblepost run ./collections -e prod --bail
  pebblepost run ./collections -e ci --report json > test-results.json
`, version)
}

// reorderArgs ensures flags placed after positional arguments are parsed correctly by flag.FlagSet.
func reorderArgs(args []string) []string {
	var flags []string
	var pos []string
	i := 0
	for i < len(args) {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			// If flag takes a value and value is in the next arg (e.g. -e dev, --report json)
			if (arg == "-e" || arg == "--report" || arg == "-r") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
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
