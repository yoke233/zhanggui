package sandbox

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/yoke233/ai-workflow/internal/acpclient"
)

// LiteBoxSandbox rewrites ACP launch config to run through the litebox-acp bridge.
//
// It composes with another Sandbox (typically HomeDirSandbox) so process-local HOME/TMP/skills
// isolation and execution-engine isolation stay decoupled.
type LiteBoxSandbox struct {
	Base Sandbox

	BridgeCommand string
	BridgeArgs    []string
	RunnerPath    string
	RunnerArgs    []string
}

func (s LiteBoxSandbox) Prepare(ctx context.Context, in PrepareInput) (acpclient.LaunchConfig, error) {
	base := s.Base
	if base == nil {
		base = NoopSandbox{}
	}

	launch, err := base.Prepare(ctx, in)
	if err != nil {
		return launch, err
	}

	bridgeCommand := strings.TrimSpace(s.BridgeCommand)
	if bridgeCommand == "" {
		return launch, fmt.Errorf("litebox sandbox: bridge command is required")
	}
	runnerPath := strings.TrimSpace(s.RunnerPath)
	if runnerPath == "" {
		return launch, fmt.Errorf("litebox sandbox: runner path is required")
	}
	program := strings.TrimSpace(launch.Command)
	if program == "" {
		return launch, fmt.Errorf("litebox sandbox: target program is required")
	}

	runnerArgs := normalizeLiteBoxRunnerArgs(s.RunnerArgs)
	args := make([]string, 0, len(s.BridgeArgs)+2+(len(runnerArgs)*2)+2+(len(launch.Args)*2))
	args = append(args, s.BridgeArgs...)
	args = append(args, "-runner", runnerPath)
	for _, arg := range runnerArgs {
		args = append(args, "-runner-arg", arg)
	}
	args = append(args, "-program", program)
	for _, arg := range launch.Args {
		args = append(args, "-program-arg", arg)
	}

	launch.Command = bridgeCommand
	launch.Args = args
	return launch, nil
}

func normalizeLiteBoxRunnerArgs(args []string) []string {
	out := append([]string(nil), args...)
	if runtime.GOOS != "windows" {
		return out
	}

	hasUnstable := false
	hasRewrite := false
	for _, arg := range out {
		switch strings.TrimSpace(arg) {
		case "--unstable":
			hasUnstable = true
		case "--rewrite-syscalls":
			hasRewrite = true
		}
	}
	if !hasUnstable {
		out = append(out, "--unstable")
	}
	if !hasRewrite {
		out = append(out, "--rewrite-syscalls")
	}
	return out
}
