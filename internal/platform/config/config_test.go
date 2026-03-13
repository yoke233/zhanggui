package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	cfg := Defaults()

	if cfg.Run.DefaultTemplate != "standard" {
		t.Fatalf("expected default template standard, got %s", cfg.Run.DefaultTemplate)
	}
	if cfg.Scheduler.MaxGlobalAgents != 3 {
		t.Fatalf("expected max_global_agents 3, got %d", cfg.Scheduler.MaxGlobalAgents)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Fatalf("expected server host 127.0.0.1, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 8080 {
		t.Fatalf("expected server port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Store.Path != ".ai-workflow/data.db" {
		t.Fatalf("expected default store path .ai-workflow/data.db, got %s", cfg.Store.Path)
	}
	if !cfg.Scheduler.Watchdog.Enabled {
		t.Fatal("expected scheduler.watchdog.enabled default true, got false")
	}
	if got := cfg.Scheduler.Watchdog.Interval.Duration; got != 5*time.Minute {
		t.Fatalf("expected scheduler.watchdog.interval 5m, got %s", got)
	}
	if strings.TrimSpace(cfg.Runtime.Prompts.PRImplementObjective) == "" {
		t.Fatal("expected runtime.prompts.pr_implement_objective default to be set")
	}
	if strings.TrimSpace(cfg.Runtime.Prompts.PRGateObjective) == "" {
		t.Fatal("expected runtime.prompts.pr_gate_objective default to be set")
	}
	if strings.TrimSpace(cfg.Runtime.Prompts.PRMergeReworkFeedback) == "" {
		t.Fatal("expected runtime.prompts.pr_merge_rework_feedback default to be set")
	}
}

func TestLoadDefaults_RuntimeAgents(t *testing.T) {
	cfg := Defaults()

	if len(cfg.Runtime.Agents.Drivers) != 2 {
		t.Fatalf("expected 2 runtime drivers, got %d", len(cfg.Runtime.Agents.Drivers))
	}
	if len(cfg.Runtime.Agents.Profiles) != 4 {
		t.Fatalf("expected 4 runtime profiles, got %d", len(cfg.Runtime.Agents.Profiles))
	}

	byID := make(map[string]RuntimeProfileConfig, len(cfg.Runtime.Agents.Profiles))
	for _, profile := range cfg.Runtime.Agents.Profiles {
		byID[profile.ID] = profile
	}

	lead, ok := byID["lead"]
	if !ok {
		t.Fatal("expected lead profile")
	}
	if lead.Driver != "claude-acp" {
		t.Fatalf("expected lead.driver=claude-acp, got %q", lead.Driver)
	}
	if lead.PromptTemplate != "team_leader" {
		t.Fatalf("expected lead.prompt_template=team_leader, got %q", lead.PromptTemplate)
	}
}

func TestLoadLayerRejectsLegacySections(t *testing.T) {
	_, err := LoadLayerBytes([]byte(`
[a2a]
enabled = true
`))
	if err == nil {
		t.Fatal("expected legacy a2a section to be rejected")
	}

	_, err = LoadLayerBytes([]byte(`
[role_bindings.team_leader]
role = "team_leader"
`))
	if err == nil {
		t.Fatal("expected legacy role_bindings section to be rejected")
	}
}

func TestConfigDefaults_GitHub(t *testing.T) {
	cfg := Defaults()
	if cfg.GitHub.Enabled {
		t.Fatalf("expected github.enabled default false, got true")
	}
	if cfg.GitHub.Token != "" {
		t.Fatalf("expected github.token default empty, got %q", cfg.GitHub.Token)
	}
}

func TestDefaultsTOML_LoadGlobalStrict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, DefaultsTOML(), 0o644); err != nil {
		t.Fatalf("write defaults.toml: %v", err)
	}

	cfg, err := LoadGlobal(path)
	if err != nil {
		t.Fatalf("LoadGlobal(defaults.toml) returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected config to be loaded")
	}
}

func TestLoadGlobalYAMLReadsSchedulerMaxProjectRuns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte("scheduler:\n  max_project_runs: 7\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	cfg, err := LoadGlobal(path)
	if err != nil {
		t.Fatalf("LoadGlobal(config.yaml) returned error: %v", err)
	}
	if cfg.Scheduler.MaxProjectRuns != 7 {
		t.Fatalf("expected scheduler.max_project_runs 7, got %d", cfg.Scheduler.MaxProjectRuns)
	}
}
