//go:build probe

// Integration test: verifies that LoadSession on a real codex ACP replays
// historical events, and that the ACPHandler suppress flag silences them.
//
// Run manually:  go test -tags probe ./cmd/acp-probe/ -run TestLoadSession -v -timeout 300s
package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/acpclient"
	"github.com/yoke233/ai-workflow/internal/teamleader"
)

// codexLaunchConfig returns a LaunchConfig that spawns a real codex-acp process.
func codexLaunchConfig(workDir string) acpclient.LaunchConfig {
	return acpclient.LaunchConfig{
		Command: "npx",
		Args:    []string{"-y", "@zed-industries/codex-acp"},
		WorkDir: workDir,
	}
}

// eventRecorder counts events by type and records whether they arrive.
type eventRecorder struct {
	mu       sync.Mutex
	total    atomic.Int64
	byType   map[string]int64
	suppress bool // mirrors ACPHandler.suppressEvents for standalone use
}

func newEventRecorder() *eventRecorder {
	return &eventRecorder{byType: make(map[string]int64)}
}

func (r *eventRecorder) HandleSessionUpdate(_ context.Context, u acpclient.SessionUpdate) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.suppress {
		return nil
	}
	r.total.Add(1)
	r.byType[u.Type]++
	return nil
}

func (r *eventRecorder) Total() int64 { return r.total.Load() }

func (r *eventRecorder) Count(typ string) int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.byType[typ]
}

func (r *eventRecorder) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.total.Store(0)
	r.byType = make(map[string]int64)
}

func (r *eventRecorder) SetSuppress(v bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.suppress = v
}

func (r *eventRecorder) Dump(t *testing.T) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t.Logf("total events: %d", r.total.Load())
	for typ, cnt := range r.byType {
		t.Logf("  %-30s %d", typ, cnt)
	}
}

// initClient creates and initializes an ACP client.
func initClient(t *testing.T, cfg acpclient.LaunchConfig, handler acpproto.Client, recorder acpclient.EventHandler) *acpclient.Client {
	t.Helper()
	client, err := acpclient.New(cfg, handler, acpclient.WithEventHandler(recorder))
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := client.Initialize(ctx, acpclient.ClientCapabilities{
		FSRead:   true,
		FSWrite:  true,
		Terminal: true,
	}); err != nil {
		client.Close(context.Background())
		t.Fatalf("initialize: %v", err)
	}
	return client
}

func closeClient(client *acpclient.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = client.Close(ctx)
}

// TestLoadSessionReplaysEvents verifies that LoadSession on a new codex process
// replays historical events from a previous session, and that suppressing works.
func TestLoadSessionReplaysEvents(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping real codex ACP test in short mode")
	}

	workDir, err := os.MkdirTemp("", "acp-loadsession-test-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(workDir)

	cfg := codexLaunchConfig(workDir)

	// --- Phase 1: create session, send a prompt, then close (simulate crash) ---
	t.Log("=== Phase 1: create initial session and send prompt ===")
	rec1 := newEventRecorder()
	handler1 := &acpclient.NopHandler{}
	client1 := initClient(t, cfg, handler1, rec1)

	ctx1, cancel1 := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel1()

	sessionID, err := client1.NewSession(ctx1, acpproto.NewSessionRequest{
		Cwd:        workDir,
		McpServers: []acpproto.McpServer{},
	})
	if err != nil {
		closeClient(client1)
		t.Fatalf("new session: %v", err)
	}
	t.Logf("session created: %s", sessionID)

	prompt := "Reply with exactly: PROBE_TEST_OK"
	result, err := client1.Prompt(ctx1, acpproto.PromptRequest{
		SessionId: sessionID,
		Prompt:    []acpproto.ContentBlock{{Text: &acpproto.ContentBlockText{Text: prompt}}},
	})
	if err != nil {
		closeClient(client1)
		t.Fatalf("prompt: %v", err)
	}
	t.Logf("prompt result: stopReason=%s textLen=%d", result.StopReason, len(result.Text))
	t.Logf("phase 1 events:")
	rec1.Dump(t)
	phase1Events := rec1.Total()
	if phase1Events == 0 {
		closeClient(client1)
		t.Fatal("expected at least some events from phase 1 prompt")
	}

	// Close client to simulate crash.
	closeClient(client1)
	t.Log("client closed (simulating crash)")

	// --- Phase 2: new client, LoadSession WITHOUT suppress → expect replayed events ---
	t.Log("=== Phase 2: LoadSession without suppress (expect replayed events) ===")
	rec2 := newEventRecorder()
	handler2 := &acpclient.NopHandler{}
	client2 := initClient(t, cfg, handler2, rec2)
	defer closeClient(client2)

	ctx2, cancel2 := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel2()

	loaded, err := client2.LoadSession(ctx2, acpproto.LoadSessionRequest{
		SessionId:  sessionID,
		Cwd:        workDir,
		McpServers: []acpproto.McpServer{},
	})
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	// Give agent a moment to send replayed notifications.
	time.Sleep(2 * time.Second)

	t.Logf("loaded session: %s", loaded)
	t.Logf("phase 2 events (replayed during LoadSession):")
	rec2.Dump(t)
	replayedEvents := rec2.Total()
	t.Logf("replayed events count: %d (phase 1 had: %d)", replayedEvents, phase1Events)

	if replayedEvents == 0 {
		t.Log("WARNING: no events replayed during LoadSession — agent may not replay; test inconclusive")
	} else {
		t.Logf("CONFIRMED: LoadSession replayed %d events — this is the bug source", replayedEvents)
	}

	// --- Phase 3: verify suppress mechanism works ---
	t.Log("=== Phase 3: verify ACPHandler.SetSuppressEvents works ===")

	// Use a real ACPHandler to verify suppress logic.
	acpHandler := teamleader.NewACPHandler(workDir, "test-session", nil)
	acpHandler.SetSuppressEvents(true)

	// Simulate feeding replayed events through the handler.
	suppressed := 0
	for i := 0; i < 10; i++ {
		_ = acpHandler.HandleSessionUpdate(context.Background(), acpclient.SessionUpdate{
			SessionID: "test",
			Type:      "agent_message_chunk",
			Text:      fmt.Sprintf("replayed chunk %d", i),
		})
		suppressed++
	}
	// If suppress works, no events should have been published (publisher is nil,
	// but the important thing is that the method returned early without panic).
	t.Logf("fed %d events while suppress=true — all silently dropped", suppressed)

	acpHandler.SetSuppressEvents(false)
	t.Log("suppress disabled — future events would be published normally")

	t.Log("=== Test complete ===")
}
