package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
)

func TestDockerSandboxPrepareWrapsLaunch(t *testing.T) {
	t.Parallel()

	homeDir := filepath.Join(t.TempDir(), "home")
	tmpDir := filepath.Join(t.TempDir(), "tmp")
	workDir := filepath.Join(t.TempDir(), "repo")
	for _, dir := range []string{homeDir, tmpDir, workDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	sb := DockerSandbox{
		Base:           NoopSandbox{},
		Command:        "docker",
		Image:          "ghcr.io/yoke233/ai-workflow-agent:latest",
		RunArgs:        []string{"--pull=never"},
		CPUs:           "1.5",
		Memory:         "2g",
		MemorySwap:     "2g",
		PidsLimit:      "128",
		Network:        "bridge",
		ReadOnlyRootFS: true,
		Tmpfs:          []string{"/run:size=64m"},
	}
	got, err := sb.Prepare(context.Background(), PrepareInput{
		Launch: acpclient.LaunchConfig{
			Command: "claude-acp",
			Args:    []string{"--stdio"},
			WorkDir: workDir,
			Env: map[string]string{
				"CLAUDE_CONFIG_DIR": homeDir,
				"TMPDIR":            tmpDir,
				"TMP":               tmpDir,
				"TEMP":              tmpDir,
			},
		},
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	if got.Command != "docker" {
		t.Fatalf("wrapped command = %q, want docker", got.Command)
	}
	if !containsPair(got.Args, "--cpus", "1.5") ||
		!containsPair(got.Args, "--memory", "2g") ||
		!containsPair(got.Args, "--memory-swap", "2g") ||
		!containsPair(got.Args, "--pids-limit", "128") ||
		!containsPair(got.Args, "--network", "bridge") {
		t.Fatalf("wrapped args missing docker resource flags: %v", got.Args)
	}
	if !contains(got.Args, "--read-only") || !containsPair(got.Args, "--tmpfs", "/run:size=64m") {
		t.Fatalf("wrapped args missing rootfs/tmpfs flags: %v", got.Args)
	}
	expectedClaudeDir := containerHomeBase + "/.claude"
	if !containsPair(got.Args, "-e", "CLAUDE_CONFIG_DIR="+expectedClaudeDir) {
		t.Fatalf("wrapped args missing container home env: %v", got.Args)
	}
	if !containsPair(got.Args, "-v", homeDir+":"+expectedClaudeDir) {
		t.Fatalf("wrapped args missing home mount: %v", got.Args)
	}
	if !containsPair(got.Args, "-v", tmpDir+":"+containerTempDir) {
		t.Fatalf("wrapped args missing tmp mount: %v", got.Args)
	}
	if !contains(got.Args, "ghcr.io/yoke233/ai-workflow-agent:latest") || !contains(got.Args, "claude-acp") {
		t.Fatalf("wrapped args missing image/program tail: %v", got.Args)
	}
}

func TestDockerSandboxPrepareRequiresImage(t *testing.T) {
	t.Parallel()

	_, err := (DockerSandbox{Command: "docker"}).Prepare(context.Background(), PrepareInput{
		Launch: acpclient.LaunchConfig{Command: "claude-acp"},
	})
	if err == nil {
		t.Fatal("Prepare() error = nil, want validation error")
	}
}
