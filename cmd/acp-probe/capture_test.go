//go:build probe

// Capture tool: runs real codex ACP in multiple scenarios and dumps all events
// to a JSON fixture file for offline mock testing.
//
// Run manually:  go test -tags probe ./cmd/acp-probe/ -run TestCaptureACPEvents -v -timeout 300s
// The output goes to internal/acpclient/testdata/codex_fixtures.json
package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
)

// --- fixture data model ---

type FixtureEvent struct {
	OffsetMs int64                   `json:"offset_ms"`
	Update   acpclient.SessionUpdate `json:"update"`
}

type FixturePromptResult struct {
	Text       string `json:"text"`
	StopReason string `json:"stop_reason"`
}

type FixtureScenario struct {
	Description string               `json:"description"`
	SessionID   string               `json:"session_id"`
	Events      []FixtureEvent       `json:"events"`
	Result      *FixturePromptResult `json:"result,omitempty"`
}

type FixtureFile struct {
	CapturedAt string                     `json:"captured_at"`
	Agent      string                     `json:"agent"`
	Scenarios  map[string]FixtureScenario `json:"scenarios"`
}

// --- capture recorder ---

type captureRecorder struct {
	mu     sync.Mutex
	start  time.Time
	events []FixtureEvent
}

func newCaptureRecorder() *captureRecorder {
	return &captureRecorder{start: time.Now()}
}

func (r *captureRecorder) HandleSessionUpdate(_ context.Context, u acpclient.SessionUpdate) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, FixtureEvent{
		OffsetMs: time.Since(r.start).Milliseconds(),
		Update:   u,
	})
	return nil
}

func (r *captureRecorder) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.start = time.Now()
	r.events = nil
}

func (r *captureRecorder) Snapshot() []FixtureEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]FixtureEvent, len(r.events))
	copy(out, r.events)
	return out
}

func TestCaptureACPEvents(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real codex capture in short mode")
	}

	workDir, err := os.MkdirTemp("", "acp-capture-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(workDir)

	cfg := codexLaunchConfig(workDir)
	fixtures := &FixtureFile{
		CapturedAt: time.Now().UTC().Format(time.RFC3339),
		Agent:      "codex-acp",
		Scenarios:  make(map[string]FixtureScenario),
	}

	// =================== Scenario 1: new_session_simple_prompt ===================
	t.Log("=== Scenario: new_session_simple_prompt ===")
	rec := newCaptureRecorder()
	client, sessionID := captureNewSessionAndPrompt(t, cfg, workDir, rec,
		"Reply with exactly: HELLO_ACP_TEST")
	events1 := rec.Snapshot()
	t.Logf("captured %d events", len(events1))

	fixtures.Scenarios["new_session_simple_prompt"] = FixtureScenario{
		Description: "NewSession then simple prompt asking for exact reply",
		SessionID:   string(sessionID),
		Events:      events1,
		Result:      &FixturePromptResult{Text: "HELLO_ACP_TEST", StopReason: "end_turn"},
	}

	// Keep client alive for scenario 2; close to simulate crash.
	closeClient(client)

	// =================== Scenario 2: load_session_replay ===================
	t.Log("=== Scenario: load_session_replay ===")
	rec.Reset()
	handler2 := &acpclient.NopHandler{}
	client2 := initClient(t, cfg, handler2, rec)

	ctx2, cancel2 := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel2()

	loaded, err := client2.LoadSession(ctx2, acpproto.LoadSessionRequest{
		SessionId:  sessionID,
		Cwd:        workDir,
		McpServers: []acpproto.McpServer{},
	})
	if err != nil {
		closeClient(client2)
		t.Fatalf("load session: %v", err)
	}
	// Wait for replay events to arrive.
	time.Sleep(3 * time.Second)
	events2 := rec.Snapshot()
	t.Logf("captured %d replay events", len(events2))

	fixtures.Scenarios["load_session_replay"] = FixtureScenario{
		Description: "LoadSession on new process — replays historical conversation events",
		SessionID:   string(loaded),
		Events:      events2,
	}

	// =================== Scenario 3: load_session_then_prompt ===================
	t.Log("=== Scenario: load_session_then_prompt ===")
	rec.Reset()

	result3, err := client2.Prompt(ctx2, acpproto.PromptRequest{
		SessionId: loaded,
		Prompt:    []acpproto.ContentBlock{{Text: &acpproto.ContentBlockText{Text: "Reply with exactly: SECOND_TURN_OK"}}},
	})
	if err != nil {
		closeClient(client2)
		t.Fatalf("prompt after load: %v", err)
	}
	events3 := rec.Snapshot()
	t.Logf("captured %d events for prompt after load", len(events3))

	var result3Fix *FixturePromptResult
	if result3 != nil {
		result3Fix = &FixturePromptResult{
			Text:       result3.Text,
			StopReason: string(result3.StopReason),
		}
	}
	fixtures.Scenarios["load_session_then_prompt"] = FixtureScenario{
		Description: "Prompt sent after LoadSession on resumed session",
		SessionID:   string(loaded),
		Events:      events3,
		Result:      result3Fix,
	}
	closeClient(client2)

	// =================== Scenario 4: new_session_tool_use ===================
	t.Log("=== Scenario: new_session_tool_use ===")
	rec.Reset()
	client4, sessionID4 := captureNewSessionAndPrompt(t, cfg, workDir, rec,
		"Create a file called test_capture.txt with content 'captured by acp-probe' in the current directory.")
	events4 := rec.Snapshot()
	t.Logf("captured %d events (with tool use)", len(events4))

	fixtures.Scenarios["new_session_tool_use"] = FixtureScenario{
		Description: "NewSession then prompt that triggers tool calls (file write)",
		SessionID:   string(sessionID4),
		Events:      events4,
	}
	closeClient(client4)

	// =================== Write fixture file ===================
	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	outPath := filepath.Join(repoRoot, "internal", "acpclient", "testdata", "codex_fixtures.json")

	data, err := json.MarshalIndent(fixtures, "", "  ")
	if err != nil {
		t.Fatalf("marshal fixtures: %v", err)
	}
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}
	t.Logf("fixtures written to %s (%d bytes)", outPath, len(data))
	for name, sc := range fixtures.Scenarios {
		t.Logf("  %-30s  %d events", name, len(sc.Events))
	}
}

func captureNewSessionAndPrompt(
	t *testing.T,
	cfg acpclient.LaunchConfig,
	workDir string,
	rec *captureRecorder,
	prompt string,
) (*acpclient.Client, acpproto.SessionId) {
	t.Helper()
	handler := &acpclient.NopHandler{}
	client := initClient(t, cfg, handler, rec)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	sessionID, err := client.NewSession(ctx, acpproto.NewSessionRequest{
		Cwd:        workDir,
		McpServers: []acpproto.McpServer{},
	})
	if err != nil {
		closeClient(client)
		t.Fatalf("new session: %v", err)
	}

	_, err = client.Prompt(ctx, acpproto.PromptRequest{
		SessionId: sessionID,
		Prompt:    []acpproto.ContentBlock{{Text: &acpproto.ContentBlockText{Text: prompt}}},
	})
	if err != nil {
		closeClient(client)
		t.Fatalf("prompt: %v", err)
	}
	return client, sessionID
}
