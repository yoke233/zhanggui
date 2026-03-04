package main

import (
	"fmt"
	"os"

	agentclaude "github.com/yoke233/ai-workflow/internal/plugins/agent-claude"
	runtimeprocess "github.com/yoke233/ai-workflow/internal/plugins/runtime-process"
	"github.com/yoke233/ai-workflow/internal/tui"
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
	case "project":
		if len(args) < 2 {
			return fmt.Errorf("usage: ai-flow project <add|list|scan>")
		}
		switch args[1] {
		case "add":
			return cmdProjectAdd(args[2:])
		case "list", "ls":
			return cmdProjectList()
		case "scan":
			return cmdProjectScan(args[2:])
		default:
			return fmt.Errorf("unknown project command: %s", args[1])
		}
	case "Run":
		if len(args) < 2 {
			return fmt.Errorf("usage: ai-flow Run <create|start|status|list|action>")
		}
		switch args[1] {
		case "create":
			return cmdRunCreate(args[2:])
		case "start":
			return cmdRunstart(args[2:])
		case "status":
			return cmdRunStatus(args[2:])
		case "list":
			return cmdRunList(args[2:])
		case "action":
			return cmdRunAction(args[2:])
		default:
			return fmt.Errorf("unknown Run command: %s", args[1])
		}
	case "scheduler":
		if len(args) < 2 {
			return fmt.Errorf("usage: ai-flow scheduler <run|once>")
		}
		switch args[1] {
		case "run":
			return cmdSchedulerRun()
		case "once":
			return cmdSchedulerOnce()
		default:
			return fmt.Errorf("unknown scheduler command: %s", args[1])
		}
	case "github":
		if len(args) < 2 {
			return fmt.Errorf("usage: ai-flow github <replay|validate>")
		}
		switch args[1] {
		case "replay":
			return cmdGitHubReplay(args[2:])
		case "validate":
			return cmdGitHubValidate(args[2:])
		default:
			return fmt.Errorf("unknown github command: %s", args[1])
		}
	case "server":
		return cmdServer(args[1:])
	case "tui":
		exec, store, err := bootstrap()
		if err != nil {
			return err
		}
		defer store.Close()
		claude := agentclaude.New("claude")
		runtime := runtimeprocess.New()
		return tui.Run(exec, store, claude, runtime)
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
	return nil
}

func printUsage() {
	fmt.Println(`ai-flow - AI Workflow Orchestrator

Usage:
  ai-flow version
  ai-flow project add <id> <repo-path>
  ai-flow project list
  ai-flow project scan <root>
  ai-flow Run create <project-id> <name> <description> [template]
  ai-flow Run start <Run-id>
  ai-flow Run status <Run-id>
  ai-flow Run list [project-id]
  ai-flow Run action <Run-id> <approve|reject|modify|skip|rerun|change_role|abort|pause|resume> [--stage <stage>] [--role <role>] [--message <text>]
  ai-flow scheduler run
  ai-flow scheduler once
  ai-flow github replay --delivery-id <id>
  ai-flow github validate
  ai-flow server [--port <port>]
  ai-flow tui`)
}
