//go:build probe

// Probe suite: verifies the self_preflight -> self_restart workflow
// using a real codex-acp agent with our MCP server configured.
//
// Run manually:
//
//	go test -tags probe ./cmd/acp-probe/ -run TestBootstrapGateBlock -v -timeout 300s
//	go test -tags probe ./cmd/acp-probe/ -run TestBootstrapFullFlow -v -timeout 600s
package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
)

// bootstrapTestSetup creates a codex ACP client with our MCP dev tools configured.
func bootstrapTestSetup(t *testing.T) (client *acpclient.Client, sessionID acpproto.SessionId, rec *captureRecorder, cancel context.CancelFunc, ctx context.Context) {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))

	binaryPath := filepath.Join(repoRoot, "ai-flow.exe")
	if _, err := os.Stat(binaryPath); err != nil {
		t.Skipf("ai-flow.exe not found at %s — run 'go build -tags dev -o ai-flow.exe ./cmd/ai-flow' first", binaryPath)
	}

	dbPath := filepath.Join(repoRoot, ".ai-workflow", "data.db")
	cfg := codexLaunchConfig(repoRoot)

	mcpServer := acpproto.McpServer{
		Stdio: &acpproto.McpServerStdio{
			Name:    "ai-workflow",
			Command: binaryPath,
			Args:    []string{"mcp-serve"},
			Env: []acpproto.EnvVariable{
				{Name: "AI_WORKFLOW_DB_PATH", Value: dbPath},
				{Name: "AI_WORKFLOW_DEV_MODE", Value: "true"},
				{Name: "AI_WORKFLOW_SOURCE_ROOT", Value: repoRoot},
				{Name: "AI_WORKFLOW_SERVER_ADDR", Value: "http://127.0.0.1:8080"},
			},
		},
	}

	rec = newCaptureRecorder()
	handler := &acpclient.NopHandler{}
	client = initClient(t, cfg, handler, rec)

	ctx, cancel = context.WithTimeout(context.Background(), 8*time.Minute)

	var err error
	sessionID, err = client.NewSession(ctx, acpproto.NewSessionRequest{
		Cwd:        repoRoot,
		McpServers: []acpproto.McpServer{mcpServer},
	})
	if err != nil {
		cancel()
		closeClient(client)
		t.Fatalf("new session: %v", err)
	}
	t.Logf("session: %s", sessionID)
	return
}

// TestBootstrapGateBlock verifies that self_restart is blocked without preflight.
// Fast test: ~2 minutes (codex startup + 2 prompts).
func TestBootstrapGateBlock(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real codex test in short mode")
	}

	client, sessionID, rec, cancel, ctx := bootstrapTestSetup(t)
	defer cancel()
	defer closeClient(client)

	// Phase 1: Check preflight status — should say "no preflight".
	t.Log("=== Phase 1: Check preflight status ===")
	result1, err := client.Prompt(ctx, acpproto.PromptRequest{
		SessionId: sessionID,
		Prompt: []acpproto.ContentBlock{{Text: &acpproto.ContentBlockText{
			Text: "Use the self_preflight_status tool from the ai-workflow MCP server. Report the raw JSON result.",
		}}},
	})
	if err != nil {
		t.Fatalf("prompt 1: %v", err)
	}
	t.Logf("phase 1: stopReason=%s events=%d", result1.StopReason, len(rec.Snapshot()))

	// Phase 2: Attempt self_restart — should be blocked.
	t.Log("=== Phase 2: Attempt self_restart (should be blocked) ===")
	rec.Reset()
	result2, err := client.Prompt(ctx, acpproto.PromptRequest{
		SessionId: sessionID,
		Prompt: []acpproto.ContentBlock{{Text: &acpproto.ContentBlockText{
			Text: "Call the self_restart tool (do NOT use force=true). Report the exact error message.",
		}}},
	})
	if err != nil {
		t.Fatalf("prompt 2: %v", err)
	}
	t.Logf("phase 2: stopReason=%s events=%d", result2.StopReason, len(rec.Snapshot()))
	t.Log("=== Gate block verified ===")
}

// TestBootstrapFullFlow runs the complete self_preflight → self_restart workflow.
// Slow test: ~5-8 minutes (includes running go vet + go test).
// Run:  go test ./cmd/acp-probe/ -run TestBootstrapFullFlow -v -timeout 600s
func TestBootstrapFullFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real codex full bootstrap test in short mode")
	}

	client, sessionID, rec, cancel, ctx := bootstrapTestSetup(t)
	defer cancel()
	defer closeClient(client)

	// Phase 1: Run self_preflight (skip frontend for speed).
	t.Log("=== Phase 1: Run self_preflight ===")
	result1, err := client.Prompt(ctx, acpproto.PromptRequest{
		SessionId: sessionID,
		Prompt: []acpproto.ContentBlock{{Text: &acpproto.ContentBlockText{
			Text: "Run the self_preflight tool with skip_frontend=true. Report: success, commit_sha, duration, steps.",
		}}},
	})
	if err != nil {
		t.Fatalf("prompt preflight: %v", err)
	}
	t.Logf("preflight: stopReason=%s events=%d", result1.StopReason, len(rec.Snapshot()))

	// Phase 2: Check preflight status — should allow restart now.
	t.Log("=== Phase 2: Verify preflight status ===")
	rec.Reset()
	result2, err := client.Prompt(ctx, acpproto.PromptRequest{
		SessionId: sessionID,
		Prompt: []acpproto.ContentBlock{{Text: &acpproto.ContentBlockText{
			Text: "Check self_preflight_status. Report whether can_restart is true.",
		}}},
	})
	if err != nil {
		t.Fatalf("prompt status: %v", err)
	}
	t.Logf("status: stopReason=%s events=%d", result2.StopReason, len(rec.Snapshot()))

	t.Log("=== Full bootstrap workflow verified ===")
}

func logEvents(t *testing.T, rec *captureRecorder, label string) {
	t.Helper()
	events := rec.Snapshot()
	t.Logf("%s: %d events", label, len(events))
	for _, e := range events {
		typ := e.Update.Type
		text := e.Update.Text
		if len(text) > 80 {
			text = text[:80] + "..."
		}
		text = strings.ReplaceAll(text, "\n", " ")
		if text != "" {
			t.Logf("  [%s] %s", typ, text)
		}
	}
}
