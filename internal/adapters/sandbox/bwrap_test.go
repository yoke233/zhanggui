package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
	"github.com/yoke233/ai-workflow/internal/core"
)

func TestBwrapSandboxPrepareWrapsLaunch(t *testing.T) {
	t.Parallel()
	patchLookPath(t, func(name string) (string, error) {
		if name != "bwrap" {
			t.Fatalf("unexpected LookPath(%q)", name)
		}
		return "/usr/bin/bwrap", nil
	})

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

	sb := BwrapSandbox{
		Base:    HomeDirSandbox{DataDir: dataDir, SkillsRoot: filepath.Join(dataDir, "skills")},
		Command: "bwrap",
	}
	got, err := sb.Prepare(context.Background(), PrepareInput{
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
	isolatedTmp := filepath.Join(isolatedHome, "tmp")
	if got.Command != "bwrap" {
		t.Fatalf("wrapped command = %q, want bwrap", got.Command)
	}
	if got.WorkDir != "" {
		t.Fatalf("wrapped workdir = %q, want empty", got.WorkDir)
	}
	if got.Env != nil {
		t.Fatalf("wrapped env should be nil, got=%v", got.Env)
	}
	if !contains(got.Args, "--clearenv") || !containsPair(got.Args, "--chdir", workDir) {
		t.Fatalf("wrapped args missing bwrap process flags: %v", got.Args)
	}
	if !containsPairWithPrefix(got.Args, "--bind", workDir) {
		t.Fatalf("wrapped args missing workdir bind: %v", got.Args)
	}
	if !containsPairWithPrefix(got.Args, "--bind", isolatedHome) {
		t.Fatalf("wrapped args missing CODEX_HOME bind: %v", got.Args)
	}
	if !containsTriple(got.Args, "--bind", isolatedTmp, "/tmp") {
		t.Fatalf("wrapped args missing /tmp bind: %v", got.Args)
	}
	if !containsPairWithPrefix(got.Args, "--setenv", "CODEX_HOME") {
		t.Fatalf("wrapped args missing CODEX_HOME env: %v", got.Args)
	}
	if !containsPairWithPrefix(got.Args, "--setenv", "HOME") {
		t.Fatalf("wrapped args missing HOME env: %v", got.Args)
	}
	if !containsPairWithPrefix(got.Args, "--setenv", "TMPDIR") {
		t.Fatalf("wrapped args missing TMPDIR env: %v", got.Args)
	}
	if !containsPairWithPrefix(got.Args, "--setenv", "PIP_CACHE_DIR") {
		t.Fatalf("wrapped args missing PIP_CACHE_DIR env: %v", got.Args)
	}
	if !containsPairWithPrefix(got.Args, "--setenv", "UV_CACHE_DIR") {
		t.Fatalf("wrapped args missing UV_CACHE_DIR env: %v", got.Args)
	}
	if !containsPairWithPrefix(got.Args, "--setenv", "PYTHONPYCACHEPREFIX") {
		t.Fatalf("wrapped args missing PYTHONPYCACHEPREFIX env: %v", got.Args)
	}
	if !containsPairWithPrefix(got.Args, "--setenv", "GOCACHE") {
		t.Fatalf("wrapped args missing GOCACHE env: %v", got.Args)
	}
	if !containsPairWithPrefix(got.Args, "--setenv", "NPM_CONFIG_CACHE") {
		t.Fatalf("wrapped args missing NPM_CONFIG_CACHE env: %v", got.Args)
	}
	if !contains(got.Args, "npx") || !contains(got.Args, "@zed-industries/codex-acp") {
		t.Fatalf("wrapped args missing target command tail: %v", got.Args)
	}
	if stat, err := os.Stat(filepath.Join(isolatedTmp, "go-build")); err != nil || !stat.IsDir() {
		t.Fatalf("expected go-build cache dir, err=%v", err)
	}
	if stat, err := os.Stat(filepath.Join(isolatedTmp, "npm-cache")); err != nil || !stat.IsDir() {
		t.Fatalf("expected npm-cache dir, err=%v", err)
	}
	if stat, err := os.Stat(filepath.Join(isolatedTmp, "pip-cache")); err != nil || !stat.IsDir() {
		t.Fatalf("expected pip-cache dir, err=%v", err)
	}
	if stat, err := os.Stat(filepath.Join(isolatedTmp, "uv-cache")); err != nil || !stat.IsDir() {
		t.Fatalf("expected uv-cache dir, err=%v", err)
	}
	if stat, err := os.Stat(filepath.Join(isolatedTmp, "pycache")); err != nil || !stat.IsDir() {
		t.Fatalf("expected pycache dir, err=%v", err)
	}
}

func containsPairWithPrefix(args []string, flag string, valuePrefix string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag && strings.HasPrefix(args[i+1], valuePrefix) {
			return true
		}
	}
	return false
}

func containsTriple(args []string, flag string, value1 string, value2 string) bool {
	for i := 0; i+2 < len(args); i++ {
		if args[i] == flag && args[i+1] == value1 && args[i+2] == value2 {
			return true
		}
	}
	return false
}
