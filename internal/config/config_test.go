package config

import "testing"

func TestMergeAgentConfig(t *testing.T) {
	global := &AgentConfig{Binary: ptr("claude"), MaxTurns: ptr(30)}
	project := &AgentConfig{MaxTurns: ptr(50)}

	merged := MergeAgentConfig(global, project)

	if *merged.Binary != "claude" {
		t.Errorf("expected binary claude, got %s", *merged.Binary)
	}
	if *merged.MaxTurns != 50 {
		t.Errorf("expected max_turns 50, got %d", *merged.MaxTurns)
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg := Defaults()
	if cfg.Pipeline.DefaultTemplate != "standard" {
		t.Errorf("expected default template standard, got %s", cfg.Pipeline.DefaultTemplate)
	}
	if cfg.Scheduler.MaxGlobalAgents != 3 {
		t.Errorf("expected max_global_agents 3, got %d", cfg.Scheduler.MaxGlobalAgents)
	}
	if cfg.Secretary.ReviewGatePlugin != "review-ai-panel" {
		t.Errorf("expected secretary.review_gate_plugin review-ai-panel, got %s", cfg.Secretary.ReviewGatePlugin)
	}
	if cfg.Secretary.ReviewPanel.MaxRounds != 2 {
		t.Errorf("expected secretary.review_panel.max_rounds 2, got %d", cfg.Secretary.ReviewPanel.MaxRounds)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected server host 127.0.0.1, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected server port 8080, got %d", cfg.Server.Port)
	}
}

func TestLoadDefaults_IncludesSpecConfig(t *testing.T) {
	cfg := Defaults()
	if cfg.Spec.Enabled {
		t.Fatalf("expected spec.enabled default false, got true")
	}
	if cfg.Spec.Provider != "noop" {
		t.Fatalf("expected spec.provider default noop, got %q", cfg.Spec.Provider)
	}
	if cfg.Spec.OnFailure != "warn" {
		t.Fatalf("expected spec.on_failure default warn, got %q", cfg.Spec.OnFailure)
	}
	if cfg.Spec.OpenSpec.Binary != "openspec" {
		t.Fatalf("expected spec.openspec.binary default openspec, got %q", cfg.Spec.OpenSpec.Binary)
	}
}

func TestConfig_Defaults_GitHub(t *testing.T) {
	cfg := Defaults()
	if cfg.GitHub.Enabled {
		t.Fatalf("expected github.enabled default false, got true")
	}
	if cfg.GitHub.Token != "" {
		t.Fatalf("expected github.token default empty, got %q", cfg.GitHub.Token)
	}
	if cfg.GitHub.AppID != 0 {
		t.Fatalf("expected github.app_id default 0, got %d", cfg.GitHub.AppID)
	}
	if cfg.GitHub.PrivateKeyPath != "" {
		t.Fatalf("expected github.private_key_path default empty, got %q", cfg.GitHub.PrivateKeyPath)
	}
	if cfg.GitHub.InstallationID != 0 {
		t.Fatalf("expected github.installation_id default 0, got %d", cfg.GitHub.InstallationID)
	}
	if cfg.GitHub.WebhookSecret != "" {
		t.Fatalf("expected github.webhook_secret default empty, got %q", cfg.GitHub.WebhookSecret)
	}
}

func TestConfigZeroValue_SpecSafeWhenMissing(t *testing.T) {
	cfg := Config{}
	ApplyConfigLayer(&cfg, &ConfigLayer{})
	if cfg.Spec.Provider != "" {
		t.Fatalf("expected zero-value provider to remain empty, got %q", cfg.Spec.Provider)
	}
	if cfg.Spec.OnFailure != "" {
		t.Fatalf("expected zero-value on_failure to remain empty, got %q", cfg.Spec.OnFailure)
	}
	if cfg.Spec.Enabled {
		t.Fatalf("expected zero-value spec.enabled false, got true")
	}
}

func ptr[T any](v T) *T { return &v }
