package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"

	"github.com/yoke233/zhanggui/internal/taskdir"
)

type LocalRunner struct {
	cfg taskdir.SandboxSpec
}

func NewLocalRunner(cfg taskdir.SandboxSpec) *LocalRunner {
	return &LocalRunner{cfg: cfg}
}

func (r *LocalRunner) Run(ctx context.Context, spec RunSpec) (Result, error) {
	if len(spec.Entrypoint) == 0 {
		if err := ensureDefaultArtifacts(spec.OutputDir, spec.TaskID, spec.RunID, spec.Rev); err != nil {
			return Result{Mode: "local", ExitCode: 1}, err
		}
		return Result{Mode: "local", ExitCode: 0}, nil
	}

	cmd := exec.CommandContext(ctx, spec.Entrypoint[0], spec.Entrypoint[1:]...)
	cmd.Dir = spec.OutputDir

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			return Result{Mode: "local", ExitCode: 1, Stdout: stdout.String(), Stderr: stderr.String()}, err
		}
	}

	res := Result{Mode: "local", ExitCode: exitCode, Stdout: stdout.String(), Stderr: stderr.String()}
	if exitCode != 0 {
		return res, fmt.Errorf("local runner exit code=%d", exitCode)
	}

	return res, nil
}
