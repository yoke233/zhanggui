//go:build dev

package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestPreflightGateCanRestart_NoResult(t *testing.T) {
	g := NewPreflightGate()
	ok, reason := g.CanRestart("abc123")
	if ok {
		t.Fatal("expected restart to be blocked with no preflight result")
	}
	if reason == "" {
		t.Fatal("expected non-empty reason")
	}
	t.Logf("blocked reason: %s", reason)
}

func TestPreflightGateCanRestart_FailedResult(t *testing.T) {
	g := NewPreflightGate()
	g.mu.Lock()
	g.last = &PreflightResult{
		Success:   false,
		CommitSHA: "abc123",
		Timestamp: time.Now(),
	}
	g.mu.Unlock()

	ok, reason := g.CanRestart("abc123")
	if ok {
		t.Fatal("expected restart to be blocked with failed preflight")
	}
	t.Logf("blocked reason: %s", reason)
}

func TestPreflightGateCanRestart_WrongCommit_Enforced(t *testing.T) {
	g := NewPreflightGate()
	g.SetEnforceCommitSHA(true)
	g.mu.Lock()
	g.last = &PreflightResult{
		Success:   true,
		CommitSHA: "abc12345",
		Timestamp: time.Now(),
	}
	g.mu.Unlock()

	ok, reason := g.CanRestart("def67890")
	if ok {
		t.Fatal("expected restart to be blocked with wrong commit when enforced")
	}
	t.Logf("blocked reason: %s", reason)
}

func TestPreflightGateCanRestart_WrongCommit_NotEnforced(t *testing.T) {
	g := NewPreflightGate()
	// enforceCommitSHA defaults to false
	g.mu.Lock()
	g.last = &PreflightResult{
		Success:   true,
		CommitSHA: "abc12345",
		Timestamp: time.Now(),
	}
	g.mu.Unlock()

	ok, reason := g.CanRestart("def67890")
	if !ok {
		t.Fatalf("expected restart allowed when commit SHA not enforced, got: %s", reason)
	}
}

func TestPreflightGateCanRestart_Success(t *testing.T) {
	g := NewPreflightGate()
	g.mu.Lock()
	g.last = &PreflightResult{
		Success:   true,
		CommitSHA: "abc123",
		Timestamp: time.Now(),
	}
	g.mu.Unlock()

	ok, reason := g.CanRestart("abc123")
	if !ok {
		t.Fatalf("expected restart allowed, got blocked: %s", reason)
	}
}

func TestPreflightGateRun_VetStep(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))

	// Verify repo root has go.mod.
	if _, err := os.Stat(filepath.Join(repoRoot, "go.mod")); err != nil {
		t.Skipf("repo root not found at %s: %v", repoRoot, err)
	}

	g := NewPreflightGate()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Run with skip_frontend=true to keep test fast, only vet+build+test.
	// We only test the first step (vet) completes to verify the machinery works.
	result, err := g.Run(ctx, repoRoot, "web", true)
	if err != nil {
		t.Fatalf("preflight run: %v", err)
	}

	t.Logf("preflight result: success=%v, commit=%s, duration=%s", result.Success, result.CommitSHA[:8], result.Duration)
	for _, step := range result.Steps {
		t.Logf("  step %s: success=%v duration=%s", step.Name, step.Success, step.Duration)
	}

	if result.CommitSHA == "" {
		t.Error("expected non-empty commit SHA")
	}

	// After run, gate should reflect the result.
	if result.Success {
		ok, _ := g.CanRestart(result.CommitSHA)
		if !ok {
			t.Error("expected restart allowed after successful preflight")
		}
	}
}

func TestPreflightGateRunConcurrentBlocked(t *testing.T) {
	g := NewPreflightGate()
	g.mu.Lock()
	g.running = true
	g.mu.Unlock()

	_, err := g.Run(context.Background(), ".", "web", false)
	if err == nil {
		t.Fatal("expected error when preflight already running")
	}
	t.Logf("concurrent blocked: %v", err)
}
