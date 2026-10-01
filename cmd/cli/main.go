package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	runCmd := flag.NewFlagSet("run", flag.ExitOnError)
	envFlag := runCmd.String("e", "", "Environment name (e.g. dev, staging, prod)")
	ciFlag := runCmd.Bool("ci", false, "Continuous Integration mode with strict exit codes")

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		_ = runCmd.Parse(os.Args[2:])
		if runCmd.NArg() < 1 {
			fmt.Println("Error: Collection path required. Example: pebblepost run ./collections -e dev")
			os.Exit(1)
		}
		targetPath := runCmd.Arg(0)
		fmt.Printf("PebblePost CLI Runner executing: %s (env: %s, ci: %v)\n", targetPath, *envFlag, *ciFlag)
		// CLI execution runner logic will be hooked into internal/workspace and internal/httpclient
	case "version":
		fmt.Println("PebblePost v0.1.0")
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`PebblePost - Modern Git-Friendly API Client & Test Runner

Usage:
  pebblepost run <path-to-collection> [-e <environment>] [--ci]
  pebblepost version

Options:
  -e <name>   Select active environment configuration
  --ci        CI/CD mode: Exit with code 1 if any test assertions fail`)
}
