package main

import (
	"fmt"
	"os"

	"github.com/yoke233/ai-workflow/internal/platform/appcmd"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	return runWithArgs(os.Args[1:])
}

func runWithArgs(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}

	switch args[0] {
	case "version":
		fmt.Println("ai-flow v0.1.0-dev")
	case "server":
		return appcmd.RunServer(args[1:])
	case "executor":
		return appcmd.RunExecutor(args[1:])
	case "quality-gate":
		return appcmd.RunQualityGate(args[1:])
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
	return nil
}

func printUsage() {
	fmt.Println(`ai-flow - AI Workflow Orchestrator

Usage:
  ai-flow version
  ai-flow server [--port <port>]
  ai-flow executor --nats-url <url> [--agents claude,codex] [--max-concurrent 2]
  ai-flow quality-gate [--backend-only|--frontend-only|--skip-backend|--skip-frontend]`)
}
