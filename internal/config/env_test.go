package config

import (
	"os"
	"testing"
)

func TestApplyEnvOverrides_ServerHost(t *testing.T) {
	t.Setenv("AI_WORKFLOW_SERVER_HOST", "0.0.0.0")

	cfg := Defaults()
	if cfg.Server.Host == "0.0.0.0" {
		t.Fatal("expected default host to differ from override value")
	}

	if err := ApplyEnvOverrides(&cfg); err != nil {
		t.Fatalf("ApplyEnvOverrides returned error: %v", err)
	}

	if cfg.Server.Host != "0.0.0.0" {
		t.Fatalf("expected server host to be overridden to 0.0.0.0, got %q", cfg.Server.Host)
	}
}

func TestApplyEnvOverrides_EmptyServerHostIsAppliedVerbatim(t *testing.T) {
	t.Setenv("AI_WORKFLOW_SERVER_HOST", "")

	cfg := Defaults()
	cfg.Server.Host = "127.0.0.1"

	if err := ApplyEnvOverrides(&cfg); err != nil {
		t.Fatalf("ApplyEnvOverrides returned error: %v", err)
	}

	if cfg.Server.Host != "" {
		t.Fatalf("expected empty host override to be applied, got %q", cfg.Server.Host)
	}
}

func TestApplyEnvOverrides_DoesNotMutateWhenUnset(t *testing.T) {
	_ = os.Unsetenv("AI_WORKFLOW_SERVER_HOST")

	cfg := Defaults()
	original := cfg.Server.Host

	if err := ApplyEnvOverrides(&cfg); err != nil {
		t.Fatalf("ApplyEnvOverrides returned error: %v", err)
	}

	if cfg.Server.Host != original {
		t.Fatalf("expected server host %q to stay unchanged, got %q", original, cfg.Server.Host)
	}
}
