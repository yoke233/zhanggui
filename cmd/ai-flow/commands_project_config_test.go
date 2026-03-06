package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yoke233/ai-workflow/internal/config"
)

func TestMergeBootstrapProjectConfig_UsesProjectOverrides(t *testing.T) {
	repoPath := t.TempDir()
	projectConfigPath := filepath.Join(repoPath, ".ai-workflow", "config.toml")
	if err := os.MkdirAll(filepath.Dir(projectConfigPath), 0o755); err != nil {
		t.Fatalf("mkdir project config dir: %v", err)
	}
	if err := os.WriteFile(projectConfigPath, []byte(`
[run]
default_template = "quick"
max_total_retries = 9

[server]
port = 19191
`), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	base := config.Defaults()
	base.A2A.Token = "test-token"
	base.Run.DefaultTemplate = "standard"
	base.Run.MaxTotalRetries = 5
	base.Server.Port = 8080

	merged, err := mergeBootstrapProjectConfig(&base, repoPath)
	if err != nil {
		t.Fatalf("mergeBootstrapProjectConfig returned error: %v", err)
	}
	if merged.Run.DefaultTemplate != "quick" {
		t.Fatalf("expected project default template quick, got %q", merged.Run.DefaultTemplate)
	}
	if merged.Run.MaxTotalRetries != 9 {
		t.Fatalf("expected project max_total_retries=9, got %d", merged.Run.MaxTotalRetries)
	}
	if merged.Server.Port != 19191 {
		t.Fatalf("expected project server.port=19191, got %d", merged.Server.Port)
	}
}

func TestMergeBootstrapProjectConfig_EnvOverridesProject(t *testing.T) {
	t.Setenv("AI_WORKFLOW_SERVER_PORT", "28080")

	repoPath := t.TempDir()
	projectConfigPath := filepath.Join(repoPath, ".ai-workflow", "config.toml")
	if err := os.MkdirAll(filepath.Dir(projectConfigPath), 0o755); err != nil {
		t.Fatalf("mkdir project config dir: %v", err)
	}
	if err := os.WriteFile(projectConfigPath, []byte(`
[server]
port = 19090
`), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	base := config.Defaults()
	base.A2A.Token = "test-token"
	base.Server.Port = 8080

	merged, err := mergeBootstrapProjectConfig(&base, repoPath)
	if err != nil {
		t.Fatalf("mergeBootstrapProjectConfig returned error: %v", err)
	}
	if merged.Server.Port != 28080 {
		t.Fatalf("expected env override server.port=28080, got %d", merged.Server.Port)
	}
}

func TestMergeBootstrapProjectConfig_WithoutProjectPathUsesBase(t *testing.T) {
	base := config.Defaults()
	base.A2A.Token = "test-token"
	base.Server.Port = 38080

	merged, err := mergeBootstrapProjectConfig(&base, "")
	if err != nil {
		t.Fatalf("mergeBootstrapProjectConfig returned error: %v", err)
	}
	if merged.Server.Port != 38080 {
		t.Fatalf("expected base server.port=38080, got %d", merged.Server.Port)
	}
}
