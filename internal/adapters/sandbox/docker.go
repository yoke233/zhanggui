package sandbox

import (
	"context"
	"fmt"

	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
)

type DockerSandbox struct {
	Base Sandbox

	Command        string
	Image          string
	RunArgs        []string
	CPUs           string
	Memory         string
	MemorySwap     string
	Network        string
	PidsLimit      string
	ReadOnlyRootFS bool
	Tmpfs          []string
}

func (s DockerSandbox) Prepare(ctx context.Context, in PrepareInput) (acpclient.LaunchConfig, error) {
	return prepareContainerLaunch(ctx, s.Base, in, containerLaunchSpec{
		command:        s.Command,
		image:          s.Image,
		runArgs:        append([]string(nil), s.RunArgs...),
		cpus:           s.CPUs,
		memory:         s.Memory,
		memorySwap:     s.MemorySwap,
		network:        s.Network,
		pidsLimit:      s.PidsLimit,
		readOnlyRootFS: s.ReadOnlyRootFS,
		tmpfs:          append([]string(nil), s.Tmpfs...),
	}, buildDockerArgs)
}

func buildDockerArgs(spec containerLaunchSpec, launch acpclient.LaunchConfig, mounts []containerVolume) []string {
	args := make([]string, 0, 20+len(spec.runArgs)+len(spec.tmpfs)*2+len(mounts)*2+len(launch.Env)*2+len(launch.Args))
	args = append(args, "run", "--rm", "-i")
	if spec.cpus != "" {
		args = append(args, "--cpus", spec.cpus)
	}
	if spec.memory != "" {
		args = append(args, "--memory", spec.memory)
	}
	if spec.memorySwap != "" {
		args = append(args, "--memory-swap", spec.memorySwap)
	}
	if spec.network != "" {
		args = append(args, "--network", spec.network)
	}
	if spec.pidsLimit != "" {
		args = append(args, "--pids-limit", spec.pidsLimit)
	}
	if spec.readOnlyRootFS {
		args = append(args, "--read-only")
	}
	for _, tmpfs := range spec.tmpfs {
		args = append(args, "--tmpfs", tmpfs)
	}
	for _, mount := range mounts {
		args = append(args, "-v", mount.hostPath+":"+mount.containerPath)
	}
	if launch.WorkDir != "" {
		args = append(args, "-w", launch.WorkDir)
	}
	worktreeName := launch.Env["__CONTAINER_WORKTREE_NAME"]
	filteredEnv := make(map[string]string, len(launch.Env))
	for k, v := range launch.Env {
		if k != "__CONTAINER_WORKTREE_NAME" {
			filteredEnv[k] = v
		}
	}
	args = appendSortedEnvArgs(args, filteredEnv, "-e")
	args = append(args, spec.runArgs...)

	if worktreeName != "" {
		// Wrap command: rewrite .git file to container path, then exec agent.
		gitFixup := fmt.Sprintf(
			`printf 'gitdir: %s/worktrees/%s\n' > /workspace/.git && exec "$@"`,
			containerGitDir, worktreeName,
		)
		args = append(args, spec.image, "sh", "-c", gitFixup, "--", launch.Command)
		args = append(args, launch.Args...)
	} else {
		args = append(args, spec.image, launch.Command)
		args = append(args, launch.Args...)
	}
	return args
}
