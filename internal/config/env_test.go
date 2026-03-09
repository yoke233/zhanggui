package config

import (
	"os"
	"testing"
	"time"
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

func TestApplyEnvOverrides_Watchdog(t *testing.T) {
	t.Setenv("AI_WORKFLOW_SCHEDULER_WATCHDOG_ENABLED", "false")
	t.Setenv("AI_WORKFLOW_SCHEDULER_WATCHDOG_INTERVAL", "2m")
	t.Setenv("AI_WORKFLOW_SCHEDULER_WATCHDOG_STUCK_RUN_TTL", "45m")
	t.Setenv("AI_WORKFLOW_SCHEDULER_WATCHDOG_STUCK_MERGE_TTL", "20m")
	t.Setenv("AI_WORKFLOW_SCHEDULER_WATCHDOG_QUEUE_STALE_TTL", "90m")

	cfg := Defaults()
	if err := ApplyEnvOverrides(&cfg); err != nil {
		t.Fatalf("ApplyEnvOverrides returned error: %v", err)
	}

	if cfg.Scheduler.Watchdog.Enabled {
		t.Fatal("expected watchdog enabled override to disable watchdog")
	}
	if got := cfg.Scheduler.Watchdog.Interval.Duration; got != 2*time.Minute {
		t.Fatalf("watchdog interval = %s, want 2m", got)
	}
	if got := cfg.Scheduler.Watchdog.StuckRunTTL.Duration; got != 45*time.Minute {
		t.Fatalf("watchdog stuck run ttl = %s, want 45m", got)
	}
	if got := cfg.Scheduler.Watchdog.StuckMergeTTL.Duration; got != 20*time.Minute {
		t.Fatalf("watchdog stuck merge ttl = %s, want 20m", got)
	}
	if got := cfg.Scheduler.Watchdog.QueueStaleTTL.Duration; got != 90*time.Minute {
		t.Fatalf("watchdog queue stale ttl = %s, want 90m", got)
	}
}
