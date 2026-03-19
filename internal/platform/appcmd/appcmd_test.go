package appcmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yoke233/ai-workflow/internal/platform/config"
)

func TestResolveGlobalConfigFilePathPrefersExistingYAML(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	yamlPath := filepath.Join(dataDir, "config.yaml")
	if err := os.WriteFile(yamlPath, []byte("server:\n  port: 9090\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(config.yaml) error = %v", err)
	}

	if got := resolveGlobalConfigFilePath(dataDir); got != yamlPath {
		t.Fatalf("resolveGlobalConfigFilePath() = %q, want %q", got, yamlPath)
	}
}

func TestLoadConfigReadsGlobalYAML(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("AI_WORKFLOW_DATA_DIR", dataDir)
	yamlPath := filepath.Join(dataDir, "config.yaml")
	if err := os.WriteFile(yamlPath, []byte("server:\n  port: 9091\nscheduler:\n  max_global_agents: 6\n  max_project_runs: 5\nruntime:\n  session_manager:\n    nats:\n      url: nats://config\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(config.yaml) error = %v", err)
	}

	cfg, _, _, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Server.Port != 9091 {
		t.Fatalf("cfg.Server.Port = %d, want 9091", cfg.Server.Port)
	}
	if cfg.Runtime.SessionManager.NATS.URL != "nats://config" {
		t.Fatalf("cfg.Runtime.SessionManager.NATS.URL = %q, want %q", cfg.Runtime.SessionManager.NATS.URL, "nats://config")
	}
	if cfg.Scheduler.MaxGlobalAgents != 6 {
		t.Fatalf("cfg.Scheduler.MaxGlobalAgents = %d, want 6", cfg.Scheduler.MaxGlobalAgents)
	}
	if cfg.Scheduler.MaxProjectRuns != 5 {
		t.Fatalf("cfg.Scheduler.MaxProjectRuns = %d, want 5", cfg.Scheduler.MaxProjectRuns)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("expected config.toml not to be created when config.yaml exists, err=%v", err)
	}
}

func TestLoadConfigGeneratesAdminTokenInSecretsToml(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("AI_WORKFLOW_DATA_DIR", dataDir)

	cfg, _, secrets, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg == nil {
		t.Fatal("LoadConfig() returned nil config")
	}
	adminToken := secrets.AdminToken()
	if adminToken == "" {
		t.Fatal("expected generated admin token, got empty")
	}

	secretsPath := filepath.Join(dataDir, "secrets.toml")
	saved, err := config.LoadSecrets(secretsPath)
	if err != nil {
		t.Fatalf("LoadSecrets(secrets.toml) error = %v", err)
	}
	if saved.AdminToken() != adminToken {
		t.Fatalf("saved admin token = %q, want %q", saved.AdminToken(), adminToken)
	}
	entry := saved.Tokens["admin"]
	if len(entry.Scopes) != 1 || entry.Scopes[0] != "*" {
		t.Fatalf("admin scopes = %#v, want [\"*\"]", entry.Scopes)
	}
	if entry.Submitter != "system.bootstrap" {
		t.Fatalf("admin submitter = %q, want %q", entry.Submitter, "system.bootstrap")
	}
}

func TestLoadConfigDoesNotGenerateAdminTokenWhenAuthDisabled(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("AI_WORKFLOW_DATA_DIR", dataDir)

	configToml := []byte("[server]\nauth_required = false\n")
	if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), configToml, 0o644); err != nil {
		t.Fatalf("WriteFile(config.toml) error = %v", err)
	}

	cfg, _, secrets, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if cfg.Server.IsAuthRequired() {
		t.Fatal("expected auth to be disabled")
	}
	if secrets.AdminToken() != "" {
		t.Fatalf("expected no generated admin token, got %q", secrets.AdminToken())
	}
	if _, err := os.Stat(filepath.Join(dataDir, "secrets.toml")); !os.IsNotExist(err) {
		t.Fatalf("expected secrets.toml not to be created when auth disabled, err=%v", err)
	}
}

func TestExpandStorePathUsesDataDirForRelativePaths(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	got := ExpandStorePath("db/data.db", dataDir)
	want := filepath.Join(dataDir, "db", "data.db")
	if got != want {
		t.Fatalf("ExpandStorePath() = %q, want %q", got, want)
	}
}

func TestExpandStorePathNormalizesLegacyDefaultStorePath(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	got := ExpandStorePath(".ai-workflow/data.db", dataDir)
	want := filepath.Join(dataDir, "data.db")
	if got != want {
		t.Fatalf("ExpandStorePath() = %q, want %q", got, want)
	}
}

func TestExpandStorePathNormalizesLegacyRootDirectory(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	got := ExpandStorePath(".ai-workflow", dataDir)
	want := dataDir
	if got != want {
		t.Fatalf("ExpandStorePath() = %q, want %q", got, want)
	}
}

func TestExpandStorePathKeepsAbsolutePaths(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	want := filepath.Join(dataDir, "store", "data.db")
	got := ExpandStorePath(want, t.TempDir())
	if got != want {
		t.Fatalf("ExpandStorePath() = %q, want %q", got, want)
	}
}

func TestResolveExecutorNATSURLPriority(t *testing.T) {
	t.Setenv("AI_WORKFLOW_NATS_URL", "nats://env")
	cfg := config.Defaults()
	cfg.Runtime.SessionManager.NATS.URL = "nats://config"

	if got := resolveExecutorNATSURL("nats://cli", &cfg); got != "nats://cli" {
		t.Fatalf("resolveExecutorNATSURL(cli) = %q, want cli", got)
	}
	if got := resolveExecutorNATSURL("", &cfg); got != "nats://env" {
		t.Fatalf("resolveExecutorNATSURL(env) = %q, want env", got)
	}

	t.Setenv("AI_WORKFLOW_NATS_URL", "")
	if got := resolveExecutorNATSURL("", &cfg); got != "nats://config" {
		t.Fatalf("resolveExecutorNATSURL(config) = %q, want config", got)
	}
}

func TestBuildServerBaseURLFallsBackToLoopback(t *testing.T) {
	t.Parallel()

	if got := buildServerBaseURL("", 8080); got != "http://127.0.0.1:8080" {
		t.Fatalf("buildServerBaseURL() = %q, want %q", got, "http://127.0.0.1:8080")
	}
}
