package main

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestRootCommandShowsHelpWhenNoArgs(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	cmd := newRootCmd(commandDeps{
		out:            &stdout,
		err:            &stdout,
		version:        versionString,
		runServer:      func([]string) error { return nil },
		runExecutor:    func([]string) error { return nil },
		runQualityGate: func([]string) error { return nil },
		runMCPServe:    func([]string) error { return nil },
		runOrchestrate: func([]string) error { return nil },
	})
	cmd.SetArgs(nil)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "AI Workflow Orchestrator") {
		t.Fatalf("help output missing title: %q", output)
	}
	if !strings.Contains(output, "quality-gate") || !strings.Contains(output, "mcp-serve") {
		t.Fatalf("help output missing commands: %q", output)
	}
}

func TestVersionCommandPrintsVersion(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	cmd := newRootCmd(commandDeps{
		out:            &stdout,
		err:            &stdout,
		version:        versionString,
		runServer:      func([]string) error { return nil },
		runExecutor:    func([]string) error { return nil },
		runQualityGate: func([]string) error { return nil },
		runMCPServe:    func([]string) error { return nil },
		runOrchestrate: func([]string) error { return nil },
	})
	cmd.SetArgs([]string{"version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != versionString {
		t.Fatalf("version output = %q, want %q", got, versionString)
	}
}

func TestServerCommandForwardsFlags(t *testing.T) {
	t.Parallel()

	var gotArgs []string
	cmd := newRootCmd(commandDeps{
		out:     &bytes.Buffer{},
		err:     &bytes.Buffer{},
		version: versionString,
		runServer: func(args []string) error {
			gotArgs = append([]string(nil), args...)
			return nil
		},
		runExecutor:    func([]string) error { return nil },
		runQualityGate: func([]string) error { return nil },
		runMCPServe:    func([]string) error { return nil },
		runOrchestrate: func([]string) error { return nil },
	})
	cmd.SetArgs([]string{"server", "--port", "9090"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := []string{"--port", "9090"}
	if !reflect.DeepEqual(gotArgs, want) {
		t.Fatalf("server args = %#v, want %#v", gotArgs, want)
	}
}

func TestExecutorCommandForwardsFlags(t *testing.T) {
	t.Parallel()

	var gotArgs []string
	cmd := newRootCmd(commandDeps{
		out:       &bytes.Buffer{},
		err:       &bytes.Buffer{},
		version:   versionString,
		runServer: func([]string) error { return nil },
		runExecutor: func(args []string) error {
			gotArgs = append([]string(nil), args...)
			return nil
		},
		runQualityGate: func([]string) error { return nil },
		runMCPServe:    func([]string) error { return nil },
		runOrchestrate: func([]string) error { return nil },
	})
	cmd.SetArgs([]string{"executor", "--nats-url", "nats://local", "--agents", "claude,codex", "--max-concurrent", "4"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := []string{"--nats-url", "nats://local", "--agents", "claude,codex", "--max-concurrent", "4"}
	if !reflect.DeepEqual(gotArgs, want) {
		t.Fatalf("executor args = %#v, want %#v", gotArgs, want)
	}
}

func TestQualityGateCommandForwardsFlags(t *testing.T) {
	t.Parallel()

	var gotArgs []string
	cmd := newRootCmd(commandDeps{
		out:         &bytes.Buffer{},
		err:         &bytes.Buffer{},
		version:     versionString,
		runServer:   func([]string) error { return nil },
		runExecutor: func([]string) error { return nil },
		runQualityGate: func(args []string) error {
			gotArgs = append([]string(nil), args...)
			return nil
		},
		runMCPServe:    func([]string) error { return nil },
		runOrchestrate: func([]string) error { return nil },
	})
	cmd.SetArgs([]string{"quality-gate", "--skip-frontend"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := []string{"--skip-frontend"}
	if !reflect.DeepEqual(gotArgs, want) {
		t.Fatalf("quality-gate args = %#v, want %#v", gotArgs, want)
	}
}

func TestMCPServeCommandInvokesRunner(t *testing.T) {
	t.Parallel()

	called := false
	cmd := newRootCmd(commandDeps{
		out:            &bytes.Buffer{},
		err:            &bytes.Buffer{},
		version:        versionString,
		runServer:      func([]string) error { return nil },
		runExecutor:    func([]string) error { return nil },
		runQualityGate: func([]string) error { return nil },
		runMCPServe: func(args []string) error {
			called = true
			if len(args) != 0 {
				t.Fatalf("mcp-serve args = %#v, want nil/empty", args)
			}
			return nil
		},
		runOrchestrate: func([]string) error { return nil },
	})
	cmd.SetArgs([]string{"mcp-serve"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !called {
		t.Fatal("expected mcp-serve runner to be called")
	}
}

func TestRuntimeEnsureExecutionProfilesForwardsFlags(t *testing.T) {
	t.Parallel()

	var gotArgs []string
	cmd := newRootCmd(commandDeps{
		out:            &bytes.Buffer{},
		err:            &bytes.Buffer{},
		version:        versionString,
		runServer:      func([]string) error { return nil },
		runExecutor:    func([]string) error { return nil },
		runQualityGate: func([]string) error { return nil },
		runMCPServe:    func([]string) error { return nil },
		runOrchestrate: func([]string) error { return nil },
		runRuntime: func(args []string) error {
			gotArgs = append([]string(nil), args...)
			return nil
		},
	})
	cmd.SetArgs([]string{"runtime", "ensure-execution-profiles", "--driver-id", "codex-acp", "--manager-profile", "ceo"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := []string{"ensure-execution-profiles", "--driver-id", "codex-acp", "--manager-profile", "ceo"}
	if !reflect.DeepEqual(gotArgs, want) {
		t.Fatalf("runtime args = %#v, want %#v", gotArgs, want)
	}
}

func TestUnknownCommandReturnsCobraError(t *testing.T) {
	t.Parallel()

	cmd := newRootCmd(commandDeps{
		out:            &bytes.Buffer{},
		err:            &bytes.Buffer{},
		version:        versionString,
		runServer:      func([]string) error { return nil },
		runExecutor:    func([]string) error { return nil },
		runQualityGate: func([]string) error { return nil },
		runMCPServe:    func([]string) error { return nil },
		runOrchestrate: func([]string) error { return nil },
	})
	cmd.SetArgs([]string{"nope"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected unknown command error")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("unexpected error = %v", err)
	}
}

func TestRunWithArgsPropagatesRunnerError(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	cmd := newRootCmd(commandDeps{
		out:       &bytes.Buffer{},
		err:       &bytes.Buffer{},
		version:   versionString,
		runServer: func([]string) error { return boom },
		runExecutor: func([]string) error {
			return nil
		},
		runQualityGate: func([]string) error { return nil },
		runMCPServe:    func([]string) error { return nil },
		runOrchestrate: func([]string) error { return nil },
	})
	cmd.SetArgs([]string{"server"})

	err := cmd.Execute()
	if !errors.Is(err, boom) {
		t.Fatalf("Execute() error = %v, want %v", err, boom)
	}
}

func TestOrchestrateCommandForwardsTaskCreateFlags(t *testing.T) {
	t.Parallel()

	var gotArgs []string
	cmd := newRootCmd(commandDeps{
		out:            &bytes.Buffer{},
		err:            &bytes.Buffer{},
		version:        versionString,
		runServer:      func([]string) error { return nil },
		runExecutor:    func([]string) error { return nil },
		runQualityGate: func([]string) error { return nil },
		runMCPServe:    func([]string) error { return nil },
		runOrchestrate: func(args []string) error {
			gotArgs = append([]string(nil), args...)
			return nil
		},
	})
	cmd.SetArgs([]string{
		"orchestrate", "task", "create",
		"--title", "CEO bootstrap",
		"--project-id", "12",
		"--json",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := []string{"task", "create", "--title", "CEO bootstrap", "--project-id", "12", "--json"}
	if !reflect.DeepEqual(gotArgs, want) {
		t.Fatalf("orchestrate args = %#v, want %#v", gotArgs, want)
	}
}
