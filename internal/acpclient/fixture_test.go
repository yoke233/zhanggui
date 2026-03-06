package acpclient

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
)

// fixtureAgentConfig returns a LaunchConfig that runs fixture_agent.go with the
// given fixture file and default scenario.
func fixtureAgentConfig(t *testing.T, scenario string) LaunchConfig {
	t.Helper()
	fixtureAgent, fixtureJSON, repoRoot := fixturePaths(t)
	return LaunchConfig{
		Command: "go",
		Args:    []string{"run", fixtureAgent, fixtureJSON, scenario},
		WorkDir: repoRoot,
	}
}

func fixturePaths(t *testing.T) (fixtureAgent string, fixtureJSON string, repoRoot string) {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	pkgDir := filepath.Dir(thisFile)
	repoRoot = filepath.Clean(filepath.Join(pkgDir, "..", ".."))
	fixtureAgent = filepath.Join(pkgDir, "testdata", "fixture_agent.go")
	fixtureJSON = filepath.Join(pkgDir, "testdata", "codex_fixtures.json")
	return fixtureAgent, fixtureJSON, repoRoot
}

// eventCounter counts session updates by type.
type eventCounter struct {
	mu     sync.Mutex
	total  int
	byType map[string]int
	events []SessionUpdate
}

func newEventCounter() *eventCounter {
	return &eventCounter{byType: make(map[string]int)}
}

func (c *eventCounter) HandleSessionUpdate(_ context.Context, u SessionUpdate) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.total++
	c.byType[u.Type]++
	c.events = append(c.events, u)
	return nil
}

func (c *eventCounter) Total() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.total
}

func (c *eventCounter) Count(typ string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.byType[typ]
}

func (c *eventCounter) Types() map[string]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]int, len(c.byType))
	for k, v := range c.byType {
		out[k] = v
	}
	return out
}

func (c *eventCounter) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.total = 0
	c.byType = make(map[string]int)
	c.events = nil
}

// suppressingCounter wraps eventCounter with a suppress flag.
type suppressingCounter struct {
	mu       sync.Mutex
	suppress bool
	inner    *eventCounter
}

func newSuppressingCounter() *suppressingCounter {
	return &suppressingCounter{inner: newEventCounter()}
}

func (s *suppressingCounter) HandleSessionUpdate(ctx context.Context, u SessionUpdate) error {
	s.mu.Lock()
	sup := s.suppress
	s.mu.Unlock()
	if sup {
		return nil
	}
	return s.inner.HandleSessionUpdate(ctx, u)
}

func (s *suppressingCounter) SetSuppress(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.suppress = v
}

// --- Tests ---

func TestFixtureNewSessionPromptEmitsExpectedEvents(t *testing.T) {
	cfg := fixtureAgentConfig(t, "new_session_simple_prompt")
	counter := newEventCounter()
	handler := &NopHandler{}

	client, err := New(cfg, handler, WithEventHandler(counter))
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	defer client.Close(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := client.Initialize(ctx, ClientCapabilities{FSRead: true}); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	session, err := client.NewSession(ctx, acpproto.NewSessionRequest{
		Cwd:        t.TempDir(),
		McpServers: []acpproto.McpServer{},
	})
	if err != nil {
		t.Fatalf("new session: %v", err)
	}

	result, err := client.Prompt(ctx, acpproto.PromptRequest{
		SessionId: session,
		Prompt:    []acpproto.ContentBlock{{Text: &acpproto.ContentBlockText{Text: "test"}}},
	})
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}

	if result == nil || result.StopReason == "" {
		t.Fatalf("expected non-empty result, got %v", result)
	}

	total := counter.Total()
	if total == 0 {
		t.Fatal("expected events from prompt, got 0")
	}

	types := counter.Types()
	t.Logf("events: total=%d types=%v", total, types)

	// Fixture has agent_message_chunk, usage_update, available_commands_update.
	if types["agent_message_chunk"] == 0 {
		t.Error("expected agent_message_chunk events")
	}
}

func TestFixtureLoadSessionReplaysHistoricalEvents(t *testing.T) {
	cfg := fixtureAgentConfig(t, "new_session_simple_prompt")
	counter := newEventCounter()
	handler := &NopHandler{}

	client, err := New(cfg, handler, WithEventHandler(counter))
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	defer client.Close(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := client.Initialize(ctx, ClientCapabilities{FSRead: true}); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	// LoadSession should replay events from load_session_replay scenario.
	loaded, err := client.LoadSession(ctx, acpproto.LoadSessionRequest{
		SessionId:  "old-session-id",
		Cwd:        t.TempDir(),
		McpServers: []acpproto.McpServer{},
	})
	if err != nil {
		t.Fatalf("load session: %v", err)
	}
	if strings.TrimSpace(string(loaded)) == "" {
		t.Fatal("expected non-empty loaded session id")
	}

	// Wait briefly for replay notifications to arrive.
	time.Sleep(500 * time.Millisecond)

	total := counter.Total()
	types := counter.Types()
	t.Logf("replay events: total=%d types=%v", total, types)

	// The load_session_replay fixture has user_message_chunk, agent_message_chunk, available_commands_update.
	if total == 0 {
		t.Fatal("expected replayed events during LoadSession, got 0")
	}
	if types["user_message_chunk"] == 0 {
		t.Error("expected user_message_chunk in replay")
	}
	if types["agent_message_chunk"] == 0 {
		t.Error("expected agent_message_chunk in replay")
	}
}

func TestFixtureLoadSessionThenPrompt(t *testing.T) {
	cfg := fixtureAgentConfig(t, "new_session_simple_prompt")
	counter := newEventCounter()
	handler := &NopHandler{}

	client, err := New(cfg, handler, WithEventHandler(counter))
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	defer client.Close(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := client.Initialize(ctx, ClientCapabilities{FSRead: true}); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	// Load session first.
	loaded, err := client.LoadSession(ctx, acpproto.LoadSessionRequest{
		SessionId:  "old-session-id",
		Cwd:        t.TempDir(),
		McpServers: []acpproto.McpServer{},
	})
	if err != nil {
		t.Fatalf("load session: %v", err)
	}

	// Wait for replay events.
	time.Sleep(300 * time.Millisecond)
	replayTotal := counter.Total()
	t.Logf("replay events: %d", replayTotal)

	// Reset counter, then send a prompt on the loaded session.
	counter.Reset()
	result, err := client.Prompt(ctx, acpproto.PromptRequest{
		SessionId: loaded,
		Prompt:    []acpproto.ContentBlock{{Text: &acpproto.ContentBlockText{Text: "follow-up"}}},
	})
	if err != nil {
		t.Fatalf("prompt after load: %v", err)
	}

	promptTotal := counter.Total()
	types := counter.Types()
	t.Logf("prompt events: total=%d types=%v", promptTotal, types)

	if result == nil || result.StopReason == "" {
		t.Fatalf("expected non-empty prompt result, got %v", result)
	}
	if promptTotal == 0 {
		t.Fatal("expected events from prompt, got 0")
	}
}

func TestFixtureSuppressEventsBlocksLoadSessionReplay(t *testing.T) {
	cfg := fixtureAgentConfig(t, "new_session_simple_prompt")
	counter := newSuppressingCounter()
	handler := &NopHandler{}

	client, err := New(cfg, handler, WithEventHandler(counter))
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	defer client.Close(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := client.Initialize(ctx, ClientCapabilities{FSRead: true}); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	// Suppress events before LoadSession (this is what the fix does).
	counter.SetSuppress(true)

	loaded, err := client.LoadSession(ctx, acpproto.LoadSessionRequest{
		SessionId:  "old-session-id",
		Cwd:        t.TempDir(),
		McpServers: []acpproto.McpServer{},
	})
	if err != nil {
		t.Fatalf("load session: %v", err)
	}

	// Wait for any replay notifications.
	time.Sleep(500 * time.Millisecond)

	// Re-enable events.
	counter.SetSuppress(false)

	suppressedTotal := counter.inner.Total()
	t.Logf("events during suppressed LoadSession: %d (should be 0)", suppressedTotal)
	if suppressedTotal != 0 {
		t.Errorf("expected 0 events while suppressed, got %d", suppressedTotal)
	}

	// Now send a prompt — events should flow normally.
	result, err := client.Prompt(ctx, acpproto.PromptRequest{
		SessionId: loaded,
		Prompt:    []acpproto.ContentBlock{{Text: &acpproto.ContentBlockText{Text: "follow-up"}}},
	})
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}

	promptTotal := counter.inner.Total()
	t.Logf("events after suppress disabled: %d", promptTotal)
	if promptTotal == 0 {
		t.Error("expected events from prompt after suppress disabled, got 0")
	}
	if result == nil || result.StopReason == "" {
		t.Fatalf("expected result, got %v", result)
	}
}

func TestFixtureToolUseScenario(t *testing.T) {
	cfg := fixtureAgentConfig(t, "new_session_tool_use")
	counter := newEventCounter()
	handler := &NopHandler{}

	client, err := New(cfg, handler, WithEventHandler(counter))
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	defer client.Close(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := client.Initialize(ctx, ClientCapabilities{FSRead: true, FSWrite: true, Terminal: true}); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	session, err := client.NewSession(ctx, acpproto.NewSessionRequest{
		Cwd:        t.TempDir(),
		McpServers: []acpproto.McpServer{},
	})
	if err != nil {
		t.Fatalf("new session: %v", err)
	}

	result, err := client.Prompt(ctx, acpproto.PromptRequest{
		SessionId: session,
		Prompt:    []acpproto.ContentBlock{{Text: &acpproto.ContentBlockText{Text: "create file"}}},
	})
	if err != nil {
		t.Fatalf("prompt: %v", err)
	}

	total := counter.Total()
	types := counter.Types()
	t.Logf("tool_use events: total=%d types=%v", total, types)

	if result == nil || result.StopReason == "" {
		t.Fatalf("expected result, got %v", result)
	}

	// Fixture has tool_call and tool_call_update events.
	if types["tool_call"] == 0 {
		t.Error("expected tool_call events in tool_use scenario")
	}
}

func TestFixtureFileUsesRawNotificationFormat(t *testing.T) {
	_, fixtureJSON, _ := fixturePaths(t)
	data, err := os.ReadFile(fixtureJSON)
	if err != nil {
		t.Fatalf("read fixture json: %v", err)
	}

	var fixture struct {
		Scenarios map[string]struct {
			Events []map[string]json.RawMessage `json:"events"`
		} `json:"scenarios"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("unmarshal fixture json: %v", err)
	}

	for scenarioName, scenario := range fixture.Scenarios {
		if len(scenario.Events) == 0 {
			continue
		}
		for index, event := range scenario.Events {
			if raw, ok := event["raw"]; !ok || len(raw) == 0 {
				t.Fatalf("scenario %q event %d missing raw notification payload", scenarioName, index)
			}
			if _, ok := event["update"]; ok {
				t.Fatalf("scenario %q event %d still uses legacy update field", scenarioName, index)
			}
		}
	}
}
