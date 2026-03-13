package sandbox

import (
	"context"
	"runtime"
	"testing"

	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
)

func TestLiteBoxSandboxPrepareWrapsLaunch(t *testing.T) {
	t.Parallel()

	sb := LiteBoxSandbox{
		Base:          NoopSandbox{},
		BridgeCommand: "litebox-acp",
		BridgeArgs:    []string{"--verbose"},
		RunnerPath:    "D:\\litebox\\runner.exe",
		RunnerArgs:    []string{"--rootfs", "D:\\rootfs"},
	}

	got, err := sb.Prepare(context.Background(), PrepareInput{
		Launch: acpclient.LaunchConfig{
			Command: "agent-linux",
			Args:    []string{"serve", "--stdio"},
			WorkDir: "D:\\repo",
			Env:     map[string]string{"CODEX_HOME": "D:\\tmp\\home"},
		},
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	if got.Command != "litebox-acp" {
		t.Fatalf("wrapped command = %q, want %q", got.Command, "litebox-acp")
	}
	if got.WorkDir != "D:\\repo" {
		t.Fatalf("wrapped workdir = %q, want original", got.WorkDir)
	}
	if got.Env["CODEX_HOME"] != "D:\\tmp\\home" {
		t.Fatalf("wrapped env CODEX_HOME = %q, want preserved", got.Env["CODEX_HOME"])
	}

	wantPrefix := []string{"--verbose", "-runner", "D:\\litebox\\runner.exe"}
	for i, want := range wantPrefix {
		if got.Args[i] != want {
			t.Fatalf("wrapped args[%d] = %q, want %q; all=%v", i, got.Args[i], want, got.Args)
		}
	}
	if !containsPair(got.Args, "-program", "agent-linux") {
		t.Fatalf("wrapped args missing program pair: %v", got.Args)
	}
	if !containsPair(got.Args, "-program-arg", "serve") || !containsPair(got.Args, "-program-arg", "--stdio") {
		t.Fatalf("wrapped args missing program args: %v", got.Args)
	}
}

func TestNormalizeLiteBoxRunnerArgsWindowsDefaults(t *testing.T) {
	t.Parallel()

	got := normalizeLiteBoxRunnerArgs([]string{"--rootfs", "D:\\rootfs"})
	if runtime.GOOS == "windows" {
		if !contains(got, "--unstable") || !contains(got, "--rewrite-syscalls") {
			t.Fatalf("windows runner args = %v, want default rewrite flags", got)
		}
		return
	}
	if contains(got, "--unstable") || contains(got, "--rewrite-syscalls") {
		t.Fatalf("non-windows runner args = %v, should not inject windows-only flags", got)
	}
}

func TestLiteBoxSandboxPrepareRequiresBridgeAndRunner(t *testing.T) {
	t.Parallel()

	_, err := (LiteBoxSandbox{}).Prepare(context.Background(), PrepareInput{
		Launch: acpclient.LaunchConfig{Command: "agent-linux"},
	})
	if err == nil {
		t.Fatal("Prepare() error = nil, want validation error")
	}
}

func containsPair(args []string, flag string, value string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag && args[i+1] == value {
			return true
		}
	}
	return false
}

func contains(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

