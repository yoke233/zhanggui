package acp

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
)

func fixtureProbeLaunchConfig(t *testing.T, scenario string) acpclient.LaunchConfig {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	engineDir := filepath.Dir(thisFile)
	repoRoot := filepath.Clean(filepath.Join(engineDir, "..", "..", "..", ".."))
	fixtureAgent := filepath.Join(repoRoot, "internal", "adapters", "agent", "acpclient", "testdata", "fixture_agent.go")
	fixtureJSON := filepath.Join(repoRoot, "internal", "adapters", "agent", "acpclient", "testdata", "codex_fixtures.json")
	return acpclient.LaunchConfig{
		Command: "go",
		Args:    []string{"run", fixtureAgent, fixtureJSON, scenario},
		WorkDir: repoRoot,
	}
}

func TestRunACPExecutionProbe_LoadsExistingSession(t *testing.T) {
	launch := fixtureProbeLaunchConfig(t, "new_session_simple_prompt")
	client, err := acpclient.New(launch, &acpclient.NopHandler{})
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	defer client.Close(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	workDir := t.TempDir()

	caps := acpclient.ClientCapabilities{FSRead: true}
	if err := client.Initialize(ctx, caps); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	sessionID, err := client.NewSession(ctx, acpproto.NewSessionRequest{
		Cwd:        workDir,
		McpServers: []acpproto.McpServer{},
	})
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	if _, err := client.Prompt(ctx, acpproto.PromptRequest{
		SessionId: sessionID,
		Prompt:    []acpproto.ContentBlock{{Text: &acpproto.ContentBlockText{Text: "initial"}}},
	}); err != nil {
		t.Fatalf("initial prompt: %v", err)
	}

	result, err := Run(ctx, Target{
		Launch:     launch,
		Caps:       caps,
		WorkDir:    workDir,
		MCPServers: []acpproto.McpServer{},
		SessionID:  sessionID,
		Question:   "probe status",
		Timeout:    10 * time.Second,
	})
	if err != nil {
		t.Fatalf("runACPExecutionProbe: %v", err)
	}
	if !result.Reachable || !result.Answered {
		t.Fatalf("expected reachable+answered probe result, got %+v", result)
	}
	if result.ReplyText == "" {
		t.Fatal("expected non-empty probe reply")
	}
}
