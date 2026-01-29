package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/yoke233/zhanggui/internal/taskdir"
)

type DockerRunner struct {
	cfg taskdir.SandboxSpec
}

func NewDockerRunner(cfg taskdir.SandboxSpec) *DockerRunner {
	return &DockerRunner{cfg: cfg}
}

func (r *DockerRunner) Run(ctx context.Context, spec RunSpec) (Result, error) {
	if len(spec.Entrypoint) == 0 {
		if err := ensureDefaultArtifacts(spec.OutputDir, spec.TaskID, spec.RunID, spec.Rev); err != nil {
			return Result{Mode: "docker", ExitCode: 1}, err
		}
		return Result{Mode: "docker", ExitCode: 0}, nil
	}

	image := strings.TrimSpace(r.cfg.Image)
	if image == "" {
		return Result{Mode: "docker", ExitCode: 1}, fmt.Errorf("docker runner 需要 sandbox.image")
	}

	network := strings.TrimSpace(r.cfg.Network)
	if network == "" {
		network = "none"
	}

	hostOut, err := filepath.Abs(spec.OutputDir)
	if err != nil {
		return Result{Mode: "docker", ExitCode: 1}, err
	}

	containerName := fmt.Sprintf("taskctl-%s-%s-%s", spec.TaskID, spec.RunID, spec.Rev)

	args := []string{
		"run",
		"--rm",
		"--name", containerName,
		"--workdir", "/out",
		"--network", network,
		"-v", hostOut + ":/out",
		image,
	}
	args = append(args, spec.Entrypoint...)

	cmd := exec.CommandContext(ctx, "docker", args...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			_ = exec.Command("docker", "rm", "-f", containerName).Run()
			return Result{Mode: "docker", ExitCode: 1, Stdout: stdout.String(), Stderr: stderr.String()}, err
		}
	}

	res := Result{Mode: "docker", ExitCode: exitCode, Stdout: stdout.String(), Stderr: stderr.String()}
	if exitCode != 0 {
		_ = exec.Command("docker", "rm", "-f", containerName).Run()
		return res, fmt.Errorf("docker runner exit code=%d", exitCode)
	}

	return res, nil
}
