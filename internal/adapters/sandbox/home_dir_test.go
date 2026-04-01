package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/yoke233/zhanggui/internal/adapters/agent/acpclient"
	"github.com/yoke233/zhanggui/internal/core"
	skillset "github.com/yoke233/zhanggui/internal/skills"
)

func TestDetectHome(t *testing.T) {
	t.Parallel()

	homeKey, _, kind, err := detectHome("codex-acp", nil, nil)
	if err != nil {
		t.Fatalf("detectHome(codex-acp) error = %v", err)
	}
	if homeKey != "CODEX_HOME" || kind != "codex" {
		t.Fatalf("detectHome(codex-acp) = (%q,%q), want (CODEX_HOME,codex)", homeKey, kind)
	}

	homeKey, _, kind, err = detectHome("custom", map[string]string{"CLAUDE_CONFIG_DIR": "C:\\tmp\\.claude"}, nil)
	if err != nil {
		t.Fatalf("detectHome(custom via env) error = %v", err)
	}
	if homeKey != "CLAUDE_CONFIG_DIR" || kind != "claude" {
		t.Fatalf("detectHome(custom) = (%q,%q), want (CLAUDE_CONFIG_DIR,claude)", homeKey, kind)
	}
}

func TestHomeDirSandboxPrepareSetsEnvAndLinksSkills(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("AI_WORKFLOW_DATA_DIR", dataDir)
	baseHome := filepath.Join(t.TempDir(), ".codex")
	if err := os.MkdirAll(filepath.Join(baseHome, "skills", ".system"), 0o755); err != nil {
		t.Fatalf("mkdir base skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join(baseHome, "auth.json"), []byte(`{"token":"x"}`), 0o600); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(baseHome, "config.toml"), []byte("model = \"gpt-5.4\"\n"), 0o600); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}

	skillsRoot := filepath.Join(dataDir, "skills")
	skillDir := filepath.Join(skillsRoot, "demo-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillset.DefaultSkillMD("demo-skill")), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})
	if err := os.Chdir(dataDir); err != nil {
		t.Fatalf("chdir data dir: %v", err)
	}

	sb := HomeDirSandbox{
		DataDir:          dataDir,
		SkillsRoot:       skillsRoot,
		RequireCodexAuth: true,
	}
	got, err := sb.Prepare(context.Background(), PrepareInput{
		Profile: &core.AgentProfile{
			ID:     "worker",
			Skills: []string{"demo-skill"},
			Driver: core.DriverConfig{
				Env: map[string]string{"CODEX_HOME": baseHome},
			},
		},
		Launch: acpclient.LaunchConfig{
			Command: "agent",
			Env:     map[string]string{},
		},
		Scope: "flow-1",
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	home := got.Env["CODEX_HOME"]
	if home == "" {
		t.Fatal("CODEX_HOME should be set")
	}
	if got.Env["TMPDIR"] == "" || got.Env["TMP"] == "" || got.Env["TEMP"] == "" {
		t.Fatalf("tmp envs should be set, got=%v", got.Env)
	}
	if _, err := os.Stat(filepath.Join(home, "auth.json")); err != nil {
		t.Fatalf("expected auth.json in isolated home: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "config.toml")); err != nil {
		t.Fatalf("expected config.toml in isolated home: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "skills", "demo-skill")); err != nil {
		t.Fatalf("expected skill link/dir in isolated home: %v", err)
	}
}

func TestHomeDirSandboxPrepareRejectsInvalidSkill(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("AI_WORKFLOW_DATA_DIR", dataDir)
	baseHome := filepath.Join(t.TempDir(), ".codex")
	if err := os.MkdirAll(filepath.Join(baseHome, "skills", ".system"), 0o755); err != nil {
		t.Fatalf("mkdir base skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join(baseHome, "auth.json"), []byte(`{"token":"x"}`), 0o600); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(baseHome, "config.toml"), []byte("model = \"gpt-5.4\"\n"), 0o600); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}

	skillsRoot := filepath.Join(dataDir, "skills")
	skillDir := filepath.Join(skillsRoot, "broken-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# broken"), 0o644); err != nil {
		t.Fatalf("write invalid SKILL.md: %v", err)
	}

	sb := HomeDirSandbox{
		DataDir:          dataDir,
		SkillsRoot:       skillsRoot,
		RequireCodexAuth: true,
	}
	_, err := sb.Prepare(context.Background(), PrepareInput{
		Profile: &core.AgentProfile{
			ID:     "worker",
			Skills: []string{"broken-skill"},
			Driver: core.DriverConfig{
				Env: map[string]string{"CODEX_HOME": baseHome},
			},
		},
		Launch: acpclient.LaunchConfig{
			Command: "agent",
			Env:     map[string]string{},
		},
		Scope: "flow-1",
	})
	if err == nil {
		t.Fatal("expected invalid skill to fail sandbox preparation")
	}
}

func TestHomeDirSandboxPrepareRejectsUnsafeEphemeralSkillName(t *testing.T) {
	dataDir := t.TempDir()
	baseHome := filepath.Join(t.TempDir(), ".codex")
	if err := os.MkdirAll(filepath.Join(baseHome, "skills", ".system"), 0o755); err != nil {
		t.Fatalf("mkdir base skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join(baseHome, "auth.json"), []byte(`{"token":"x"}`), 0o600); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(baseHome, "config.toml"), []byte("model = \"gpt-5.4\"\n"), 0o600); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}

	outsideFile := filepath.Join(dataDir, "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("keep"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	sb := HomeDirSandbox{
		DataDir:          dataDir,
		SkillsRoot:       filepath.Join(dataDir, "skills"),
		RequireCodexAuth: true,
	}
	srcSkill := t.TempDir()
	_, err := sb.Prepare(context.Background(), PrepareInput{
		Profile: &core.AgentProfile{
			ID: "worker",
			Driver: core.DriverConfig{
				Env: map[string]string{"CODEX_HOME": baseHome},
			},
		},
		Launch: acpclient.LaunchConfig{
			Command: "agent",
			Env:     map[string]string{},
		},
		Scope:           "flow-1",
		EphemeralSkills: map[string]string{"../../outside.txt": srcSkill},
	})
	if err == nil {
		t.Fatal("expected unsafe ephemeral skill name to fail sandbox preparation")
	}

	b, readErr := os.ReadFile(outsideFile)
	if readErr != nil {
		t.Fatalf("outside file should still exist: %v", readErr)
	}
	if string(b) != "keep" {
		t.Fatalf("outside file content = %q, want keep", string(b))
	}
}

func TestHomeDirSandboxPrepareRejectsEscapingEphemeralSkillName(t *testing.T) {
	dataDir := t.TempDir()
	baseHome := filepath.Join(t.TempDir(), ".codex")
	if err := os.MkdirAll(filepath.Join(baseHome, "skills", ".system"), 0o755); err != nil {
		t.Fatalf("mkdir base skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join(baseHome, "auth.json"), []byte(`{"token":"x"}`), 0o600); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(baseHome, "config.toml"), []byte("model = \"gpt-5.4\"\n"), 0o600); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}

	skillsRoot := filepath.Join(dataDir, "skills")
	if err := os.MkdirAll(skillsRoot, 0o755); err != nil {
		t.Fatalf("mkdir skills root: %v", err)
	}

	outsideRoot := t.TempDir()
	protectedPath := filepath.Join(outsideRoot, "keep.txt")
	if err := os.WriteFile(protectedPath, []byte("keep"), 0o644); err != nil {
		t.Fatalf("write protected path: %v", err)
	}

	sb := HomeDirSandbox{
		DataDir:          dataDir,
		SkillsRoot:       skillsRoot,
		RequireCodexAuth: true,
	}
	_, err := sb.Prepare(context.Background(), PrepareInput{
		Profile: &core.AgentProfile{
			ID: "worker",
			Driver: core.DriverConfig{
				Env: map[string]string{"CODEX_HOME": baseHome},
			},
		},
		Launch: acpclient.LaunchConfig{
			Command: "agent",
			Env:     map[string]string{},
		},
		Scope: "flow-1",
		EphemeralSkills: map[string]string{
			"..\\..\\keep.txt": outsideRoot,
		},
	})
	if err == nil {
		t.Fatal("expected escaping ephemeral skill name to fail")
	}
	if _, statErr := os.Stat(protectedPath); statErr != nil {
		t.Fatalf("expected protected path to remain untouched: %v", statErr)
	}
}

func TestHomeDirSandboxPrepare_AllowsEphemeralSkillWithoutGlobalMirror(t *testing.T) {
	dataDir := t.TempDir()
	baseHome := filepath.Join(t.TempDir(), ".codex")
	if err := os.MkdirAll(filepath.Join(baseHome, "skills", ".system"), 0o755); err != nil {
		t.Fatalf("mkdir base skills: %v", err)
	}
	if err := os.WriteFile(filepath.Join(baseHome, "auth.json"), []byte(`{"token":"x"}`), 0o600); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(baseHome, "config.toml"), []byte("model = \"gpt-5.4\"\n"), 0o600); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}

	skillsRoot := filepath.Join(dataDir, "skills")
	if err := os.MkdirAll(filepath.Join(skillsRoot, "action-signal"), 0o755); err != nil {
		t.Fatalf("mkdir action-signal dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsRoot, "action-signal", "SKILL.md"), []byte(skillset.DefaultSkillMD("action-signal")), 0o644); err != nil {
		t.Fatalf("write action-signal SKILL.md: %v", err)
	}

	ephemeralDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(ephemeralDir, "SKILL.md"), []byte(skillset.DefaultSkillMD("action-context")), 0o644); err != nil {
		t.Fatalf("write ephemeral SKILL.md: %v", err)
	}

	sb := HomeDirSandbox{
		DataDir:          dataDir,
		SkillsRoot:       skillsRoot,
		RequireCodexAuth: true,
	}
	got, err := sb.Prepare(context.Background(), PrepareInput{
		Profile: &core.AgentProfile{
			ID: "worker",
			Driver: core.DriverConfig{
				Env: map[string]string{"CODEX_HOME": baseHome},
			},
		},
		Launch: acpclient.LaunchConfig{
			Command: "agent",
			Env:     map[string]string{},
		},
		Scope:           "flow-1",
		ExtraSkills:     []string{"action-signal", "action-context"},
		EphemeralSkills: map[string]string{"action-context": ephemeralDir},
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	home := got.Env["CODEX_HOME"]
	if _, err := os.Stat(filepath.Join(home, "skills", "action-signal")); err != nil {
		t.Fatalf("expected global action-signal linked: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "skills", "action-context")); err != nil {
		t.Fatalf("expected ephemeral action-context linked: %v", err)
	}
}

func TestLinkPathIfMissingCopiesFileOnWindowsFallback(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	dstDir := t.TempDir()
	src := filepath.Join(srcDir, "a.txt")
	dst := filepath.Join(dstDir, "a.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	if err := linkPathIfMissing(dst, src, false); err != nil {
		t.Fatalf("linkPathIfMissing() error = %v", err)
	}

	b, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(b) != "hello" {
		t.Fatalf("dst content = %q, want hello", string(b))
	}

	if runtime.GOOS == "windows" {
		return
	}
	fi, err := os.Lstat(dst)
	if err != nil {
		t.Fatalf("lstat dst: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("non-windows should prefer symlink for file linking")
	}
}
