package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
	"github.com/yoke233/ai-workflow/internal/core"
)

func TestBoxLiteSandboxMaterializesExternalHomeLinks(t *testing.T) {
	dataDir := t.TempDir()
	baseHome := filepath.Join(t.TempDir(), ".codex")
	if err := os.MkdirAll(filepath.Join(baseHome, "skills", ".system"), 0o755); err != nil {
		t.Fatalf("mkdir base skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join(baseHome, "auth.json"), []byte(`{"token":"x"}`), 0o600); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	workDir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("mkdir workdir: %v", err)
	}

	sb := BoxLiteSandbox{
		Base:    HomeDirSandbox{DataDir: dataDir, SkillsRoot: filepath.Join(dataDir, "skills")},
		Command: "boxlite",
		Image:   "ghcr.io/example/acp:latest",
	}
	_, err := sb.Prepare(context.Background(), PrepareInput{
		Profile: &core.AgentProfile{
			ID: "worker",
			Driver: core.DriverConfig{
				Env: map[string]string{"CODEX_HOME": baseHome},
			},
		},
		Launch: acpclient.LaunchConfig{
			Command: "npx",
			Args:    []string{"-y", "@zed-industries/codex-acp"},
			WorkDir: workDir,
			Env:     map[string]string{},
		},
		Scope: "flow-1",
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	isolatedHome := filepath.Join(dataDir, "acp-homes", "codex", "worker", "flow-1")
	authPath := filepath.Join(isolatedHome, "auth.json")
	info, err := os.Lstat(authPath)
	if err != nil {
		t.Fatalf("lstat auth.json: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("auth.json should be materialized for container mounts, got symlink")
	}
	if _, err := os.Stat(filepath.Join(isolatedHome, "skills", ".system")); err != nil {
		t.Fatalf("expected .system to exist after materialization: %v", err)
	}
}
