package executor

import (
	"context"
	"strings"
	"testing"
	"time"

	acpproto "github.com/coder/acp-go-sdk"

	"github.com/yoke233/zhanggui/internal/adapters/agent/acpclient"
	eventbridge "github.com/yoke233/zhanggui/internal/adapters/events/bridge"
	flowapp "github.com/yoke233/zhanggui/internal/application/flow"
	probeapp "github.com/yoke233/zhanggui/internal/application/probe"
	runtimeapp "github.com/yoke233/zhanggui/internal/application/runtime"
	"github.com/yoke233/zhanggui/internal/core"
)

type stubSessionManager struct {
	acquireInput runtimeapp.SessionAcquireInput
}

func (s *stubSessionManager) Acquire(_ context.Context, in runtimeapp.SessionAcquireInput) (*runtimeapp.SessionHandle, error) {
	s.acquireInput = in
	return &runtimeapp.SessionHandle{ID: "stub-handle"}, nil
}

func (s *stubSessionManager) StartRun(_ context.Context, _ *runtimeapp.SessionHandle, _ string) (string, error) {
	return "stub-invocation", nil
}

func (s *stubSessionManager) WatchRun(_ context.Context, _ string, _ int64, _ runtimeapp.EventSink) (*runtimeapp.RunResult, error) {
	return &runtimeapp.RunResult{
		Text:       "done",
		StopReason: "completed",
	}, nil
}

func (s *stubSessionManager) RecoverRuns(_ context.Context, _ time.Time) ([]runtimeapp.RunRuntimeStatus, error) {
	return nil, nil
}

func (s *stubSessionManager) ProbeRun(_ context.Context, _ probeapp.RunProbeRuntimeRequest) (*probeapp.RunProbeRuntimeResult, error) {
	return nil, nil
}

func (s *stubSessionManager) Release(_ context.Context, _ *runtimeapp.SessionHandle) error {
	return nil
}

func (s *stubSessionManager) CleanupWorkItem(_ int64) {}

func (s *stubSessionManager) DrainActive(_ context.Context) error {
	return nil
}

func (s *stubSessionManager) ActiveCount() int {
	return 0
}

func (s *stubSessionManager) Close() {}

type stubRegistry struct {
	profile *core.AgentProfile
}

func (s stubRegistry) GetProfile(_ context.Context, id string) (*core.AgentProfile, error) {
	if s.profile != nil && s.profile.ID == id {
		return s.profile, nil
	}
	return nil, core.ErrProfileNotFound
}

func (s stubRegistry) ListProfiles(context.Context) ([]*core.AgentProfile, error) {
	return nil, nil
}

func (s stubRegistry) CreateProfile(context.Context, *core.AgentProfile) error {
	return nil
}

func (s stubRegistry) UpdateProfile(context.Context, *core.AgentProfile) error {
	return nil
}

func (s stubRegistry) DeleteProfile(context.Context, string) error {
	return nil
}

func (s stubRegistry) ResolveForAction(context.Context, *core.Action) (*core.AgentProfile, error) {
	return s.profile, nil
}

func (s stubRegistry) ResolveByID(_ context.Context, profileID string) (*core.AgentProfile, error) {
	if s.profile != nil && s.profile.ID == profileID {
		return s.profile, nil
	}
	return nil, core.ErrProfileNotFound
}

func TestBuildRunInputFromSnapshot(t *testing.T) {
	t.Run("basic execution input", func(t *testing.T) {
		step := &core.Action{Name: "implement auth"}
		executionInput := flowapp.BuildRunInputFromSnapshot("Implement JWT authentication", step, false)
		if !strings.Contains(executionInput, "# Task") {
			t.Error("execution input should start with # Task header")
		}
		if !strings.Contains(executionInput, "Implement JWT authentication") {
			t.Error("execution input should contain briefing snapshot")
		}
	})

	t.Run("with acceptance criteria", func(t *testing.T) {
		step := &core.Action{
			Name: "implement auth",
			AcceptanceCriteria: []string{
				"All tests pass",
				"No security vulnerabilities",
			},
		}
		executionInput := flowapp.BuildRunInputFromSnapshot("Implement JWT authentication", step, false)
		if !strings.Contains(executionInput, "# Acceptance Criteria") {
			t.Error("execution input should contain acceptance criteria header")
		}
		if !strings.Contains(executionInput, "- All tests pass") {
			t.Error("execution input should contain first criterion")
		}
		if !strings.Contains(executionInput, "- No security vulnerabilities") {
			t.Error("execution input should contain second criterion")
		}
	})

	t.Run("empty acceptance criteria", func(t *testing.T) {
		step := &core.Action{Name: "simple task"}
		executionInput := flowapp.BuildRunInputFromSnapshot("Do something", step, false)
		if strings.Contains(executionInput, "Acceptance Criteria") {
			t.Error("execution input should not contain acceptance criteria when empty")
		}
	})

	t.Run("with action context", func(t *testing.T) {
		step := &core.Action{Name: "implement"}
		executionInput := flowapp.BuildRunInputFromSnapshot("Do something", step, true)
		if !strings.Contains(executionInput, "# Reference Materials") {
			t.Error("execution input should contain Reference Materials header when hasActionContext=true")
		}
		if !strings.Contains(executionInput, "skills/action-context/") {
			t.Error("execution input should reference skills/action-context/ path")
		}
	})

	t.Run("without action context", func(t *testing.T) {
		step := &core.Action{Name: "implement"}
		executionInput := flowapp.BuildRunInputFromSnapshot("Do something", step, false)
		if strings.Contains(executionInput, "Reference Materials") {
			t.Error("execution input should not contain Reference Materials when hasActionContext=false")
		}
	})
}

func TestACPActionExecutor_UsesFallbackWorkDirWhenDefaultMissing(t *testing.T) {
	t.Parallel()

	sessionMgr := &stubSessionManager{}
	profile := &core.AgentProfile{
		ID:   "worker",
		Role: core.RoleWorker,
	}
	executor := NewACPActionExecutor(ACPExecutorConfig{
		Registry:       stubRegistry{profile: profile},
		SessionManager: sessionMgr,
		Bus:            NewMemBus(),
	})
	action := &core.Action{
		ID:         11,
		WorkItemID: 22,
		Name:       "execute-work-item",
		Type:       core.ActionExec,
		AgentRole:  string(core.RoleWorker),
	}
	run := &core.Run{
		ID:               33,
		ActionID:         action.ID,
		BriefingSnapshot: "do the work",
	}

	if err := executor(t.Context(), action, run); err != nil {
		t.Fatalf("executor() error = %v", err)
	}
	if strings.TrimSpace(sessionMgr.acquireInput.WorkDir) == "" {
		t.Fatal("expected non-empty work_dir passed to session manager")
	}
}

func TestResolveACPActionTimeout(t *testing.T) {
	t.Parallel()

	if got := resolveACPActionTimeout(nil); got != 120*time.Second {
		t.Fatalf("resolveACPActionTimeout(nil) = %s, want 120s", got)
	}

	action := &core.Action{Timeout: 45 * time.Second}
	if got := resolveACPActionTimeout(action); got != 45*time.Second {
		t.Fatalf("resolveACPActionTimeout(action) = %s, want 45s", got)
	}
}

func TestEventBridge_ChunkAggregation(t *testing.T) {
	bus := NewMemBus()
	bridge := eventbridge.New(bus, core.EventRunAgentOutput, eventbridge.Scope{
		WorkItemID: 1, ActionID: 2, RunID: 3,
	})

	sub := bus.Subscribe(core.SubscribeOpts{BufferSize: 64})
	defer sub.Cancel()

	ctx := t.Context()

	// Send multiple message chunks.
	bridge.HandleSessionUpdate(ctx, acpclient.SessionUpdate{
		Type: "agent_message_chunk", Text: "Hello ",
	})
	bridge.HandleSessionUpdate(ctx, acpclient.SessionUpdate{
		Type: "agent_message_chunk", Text: "world",
	})

	// Flush should aggregate.
	bridge.FlushPending(ctx)

	// Drain events: should have 2 chunks + 1 aggregated message.
	var events []core.Event
	timeout := time.After(200 * time.Millisecond)
	for {
		select {
		case ev := <-sub.C:
			events = append(events, ev)
		case <-timeout:
			goto check
		}
	}
check:
	// Find the aggregated event.
	var foundAggregated bool
	for _, ev := range events {
		subType, _ := ev.Data["type"].(string)
		if subType == "agent_message" {
			content, _ := ev.Data["content"].(string)
			if content != "Hello world" {
				t.Errorf("aggregated content = %q, want 'Hello world'", content)
			}
			foundAggregated = true
		}
	}
	if !foundAggregated {
		t.Error("expected aggregated agent_message event")
	}
}

func TestEventBridge_TypeSwitchFlushes(t *testing.T) {
	bus := NewMemBus()
	bridge := eventbridge.New(bus, core.EventRunAgentOutput, eventbridge.Scope{
		WorkItemID: 1, ActionID: 2, RunID: 3,
	})

	sub := bus.Subscribe(core.SubscribeOpts{BufferSize: 64})
	defer sub.Cancel()

	ctx := t.Context()

	// Send thought chunks then switch to message.
	bridge.HandleSessionUpdate(ctx, acpclient.SessionUpdate{
		Type: "agent_thought_chunk", Text: "thinking...",
	})
	// Switching to message should flush thought.
	bridge.HandleSessionUpdate(ctx, acpclient.SessionUpdate{
		Type: "agent_message_chunk", Text: "reply",
	})
	bridge.FlushPending(ctx)

	var events []core.Event
	timeout := time.After(200 * time.Millisecond)
	for {
		select {
		case ev := <-sub.C:
			events = append(events, ev)
		case <-timeout:
			goto check
		}
	}
check:
	var foundThought, foundMessage bool
	for _, ev := range events {
		subType, _ := ev.Data["type"].(string)
		switch subType {
		case "agent_thought":
			if ev.Data["content"] == "thinking..." {
				foundThought = true
			}
		case "agent_message":
			if ev.Data["content"] == "reply" {
				foundMessage = true
			}
		}
	}
	if !foundThought {
		t.Error("expected aggregated agent_thought event after type switch")
	}
	if !foundMessage {
		t.Error("expected aggregated agent_message event")
	}
}

func TestEventBridge_ToolCall(t *testing.T) {
	bus := NewMemBus()
	bridge := eventbridge.New(bus, core.EventRunAgentOutput, eventbridge.Scope{
		WorkItemID: 1, ActionID: 2, RunID: 3,
	})

	sub := bus.Subscribe(core.SubscribeOpts{BufferSize: 64})
	defer sub.Cancel()

	ctx := t.Context()

	bridge.HandleSessionUpdate(ctx, acpclient.SessionUpdate{
		Type:    "tool_call",
		RawJSON: []byte(`{"title":"read file","toolCallId":"tc-1"}`),
	})

	ev := <-sub.C
	if ev.Data["type"] != "tool_call" {
		t.Errorf("type = %v, want tool_call", ev.Data["type"])
	}
	if ev.Data["content"] != "read file" {
		t.Errorf("content = %v, want 'read file'", ev.Data["content"])
	}
	if ev.Data["tool_call_id"] != "tc-1" {
		t.Errorf("tool_call_id = %v, want 'tc-1'", ev.Data["tool_call_id"])
	}
}

func TestEventBridge_SessionID(t *testing.T) {
	bus := NewMemBus()
	bridge := eventbridge.New(bus, core.EventChatOutput, eventbridge.Scope{
		SessionID: "chat-123",
	})

	sub := bus.Subscribe(core.SubscribeOpts{BufferSize: 10})
	defer sub.Cancel()

	ctx := t.Context()
	bridge.PublishData(ctx, map[string]any{"type": "done", "content": "hi"})

	ev := <-sub.C
	if ev.Type != core.EventChatOutput {
		t.Errorf("type = %s, want chat.output", ev.Type)
	}
	if ev.Data["session_id"] != "chat-123" {
		t.Errorf("session_id = %v, want chat-123", ev.Data["session_id"])
	}
}

func TestEventBridge_LastActivity(t *testing.T) {
	bus := NewMemBus()
	bridge := eventbridge.New(bus, core.EventRunAgentOutput, eventbridge.Scope{})

	before := bridge.LastActivity()
	time.Sleep(5 * time.Millisecond)

	bridge.HandleSessionUpdate(t.Context(), acpclient.SessionUpdate{
		Type: "agent_message_chunk", Text: "x",
	})

	after := bridge.LastActivity()
	if !after.After(before) {
		t.Error("lastActivity should be updated after HandleSessionUpdate")
	}
}

func TestIsTransientAgentEvent(t *testing.T) {
	tests := []struct {
		eventType core.EventType
		subType   string
		want      bool
	}{
		{core.EventRunAgentOutput, "agent_message_chunk", true},
		{core.EventRunAgentOutput, "agent_thought_chunk", true},
		{core.EventRunAgentOutput, "user_message_chunk", true},
		{core.EventRunAgentOutput, "agent_message", false},
		{core.EventRunAgentOutput, "agent_thought", false},
		{core.EventRunAgentOutput, "tool_call", false},
		{core.EventRunAgentOutput, "done", false},
		{core.EventChatOutput, "agent_message_chunk", true},
		{core.EventChatOutput, "agent_message", false},
		{core.EventWorkItemStarted, "agent_message_chunk", false}, // wrong event type
	}
	for _, tt := range tests {
		ev := core.Event{
			Type: tt.eventType,
			Data: map[string]any{"type": tt.subType},
		}
		got := core.IsTransientAgentEvent(ev)
		if got != tt.want {
			t.Errorf("IsTransientAgentEvent(%s/%s) = %v, want %v",
				tt.eventType, tt.subType, got, tt.want)
		}
	}
}

func TestBuildStepMCPFactory(t *testing.T) {
	resolverCalled := 0
	resolver := func(profileID string, agentSupportsSSE bool) []acpproto.McpServer {
		resolverCalled++
		return []acpproto.McpServer{{Sse: &acpproto.McpServerSseInline{Name: "complete-step", Type: "sse", Url: "http://127.0.0.1:8080/api/v1/mcp"}}}
	}

	t.Run("nil resolver returns nil", func(t *testing.T) {
		factory := buildActionMCPFactory(&core.Action{Type: core.ActionExec}, &core.AgentProfile{ID: "worker", MCP: core.ProfileMCP{Enabled: true}}, 0, nil)
		if factory != nil {
			t.Fatal("expected nil factory")
		}
	})

	t.Run("composite step does not inject", func(t *testing.T) {
		factory := buildActionMCPFactory(&core.Action{Type: core.ActionPlan}, &core.AgentProfile{ID: "worker", MCP: core.ProfileMCP{Enabled: true}}, 1, resolver)
		if factory != nil {
			t.Fatal("expected nil factory for composite step")
		}
	})

	t.Run("profile without mcp does not inject", func(t *testing.T) {
		factory := buildActionMCPFactory(&core.Action{Type: core.ActionExec}, &core.AgentProfile{ID: "worker"}, 1, resolver)
		if factory != nil {
			t.Fatal("expected nil factory for profile without MCP")
		}
	})

	t.Run("exec step injects", func(t *testing.T) {
		factory := buildActionMCPFactory(&core.Action{Type: core.ActionExec}, &core.AgentProfile{ID: "worker", MCP: core.ProfileMCP{Enabled: true}}, 1, resolver)
		if factory == nil {
			t.Fatal("expected non-nil factory for exec step")
		}
		servers := factory(true)
		if len(servers) != 1 || servers[0].Sse == nil {
			t.Fatalf("unexpected servers: %+v", servers)
		}
	})

	t.Run("gate step injects", func(t *testing.T) {
		factory := buildActionMCPFactory(&core.Action{Type: core.ActionGate}, &core.AgentProfile{ID: "worker", MCP: core.ProfileMCP{Enabled: true}}, 1, resolver)
		if factory == nil {
			t.Fatal("expected non-nil factory for gate step")
		}
		_ = factory(false)
	})

	if resolverCalled != 2 {
		t.Fatalf("resolver called %d times, want 2", resolverCalled)
	}
}

func TestParseOutputSignal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		text     string
		wantNil  bool
		decision string
		reason   string
		extraKey string
		extraVal any
	}{
		{
			name:     "complete signal",
			text:     "some output\nAI_WORKFLOW_SIGNAL: {\"decision\":\"complete\",\"reason\":\"done\"}",
			decision: "complete",
			reason:   "done",
		},
		{
			name:     "reject signal",
			text:     "review output\nAI_WORKFLOW_SIGNAL: {\"decision\":\"reject\",\"reason\":\"missing tests\"}",
			decision: "reject",
			reason:   "missing tests",
		},
		{
			name:     "approve signal",
			text:     "AI_WORKFLOW_SIGNAL: {\"decision\":\"approve\",\"reason\":\"looks good\"}",
			decision: "approve",
			reason:   "looks good",
		},
		{
			name:     "need_help signal",
			text:     "AI_WORKFLOW_SIGNAL: {\"decision\":\"need_help\",\"reason\":\"stuck on auth\"}",
			decision: "need_help",
			reason:   "stuck on auth",
		},
		{
			name:    "no signal line",
			text:    "just a normal response without any signal",
			wantNil: true,
		},
		{
			name:    "invalid decision",
			text:    "AI_WORKFLOW_SIGNAL: {\"decision\":\"unknown\",\"reason\":\"wat\"}",
			wantNil: true,
		},
		{
			name:    "invalid json",
			text:    "AI_WORKFLOW_SIGNAL: not-json",
			wantNil: true,
		},
		{
			name:    "empty text",
			text:    "",
			wantNil: true,
		},
		{
			name:     "multiple signals uses last",
			text:     "AI_WORKFLOW_SIGNAL: {\"decision\":\"need_help\",\"reason\":\"first\"}\nAI_WORKFLOW_SIGNAL: {\"decision\":\"complete\",\"reason\":\"second\"}",
			decision: "complete",
			reason:   "second",
		},
		{
			name:     "signal with surrounding text",
			text:     "I finished the task.\n\nAI_WORKFLOW_SIGNAL: {\"decision\":\"complete\",\"reason\":\"all done\"}\n\nThanks!",
			decision: "complete",
			reason:   "all done",
		},
		{
			name:     "signal preserves artifact metadata",
			text:     "AI_WORKFLOW_SIGNAL: {\"decision\":\"complete\",\"reason\":\"all done\",\"artifact_namespace\":\"gstack\",\"artifact_type\":\"design_doc\"}",
			decision: "complete",
			reason:   "all done",
			extraKey: "artifact_namespace",
			extraVal: "gstack",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			sig := parseOutputSignal(tt.text)
			if tt.wantNil {
				if sig != nil {
					t.Fatalf("expected nil, got %+v", sig)
				}
				return
			}
			if sig == nil {
				t.Fatal("expected signal, got nil")
			}
			if sig.Decision != tt.decision {
				t.Errorf("decision = %q, want %q", sig.Decision, tt.decision)
			}
			if sig.Reason != tt.reason {
				t.Errorf("reason = %q, want %q", sig.Reason, tt.reason)
			}
			if tt.extraKey != "" && sig.Payload[tt.extraKey] != tt.extraVal {
				t.Errorf("payload[%q] = %v, want %v", tt.extraKey, sig.Payload[tt.extraKey], tt.extraVal)
			}
		})
	}
}

func TestDecisionToSignalType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		decision string
		wantType core.SignalType
		wantOK   bool
	}{
		{"complete", core.SignalComplete, true},
		{"need_help", core.SignalNeedHelp, true},
		{"approve", core.SignalApprove, true},
		{"reject", core.SignalReject, true},
		{"unknown", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.decision, func(t *testing.T) {
			got, ok := decisionToSignalType(tt.decision)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.wantType {
				t.Errorf("type = %q, want %q", got, tt.wantType)
			}
		})
	}
}

func TestPublishRunAudit(t *testing.T) {
	bus := NewMemBus()
	sub := bus.Subscribe(core.SubscribeOpts{BufferSize: 4})
	defer sub.Cancel()

	step := &core.Action{
		ID:         22,
		WorkItemID: 11,
		Type:       core.ActionExec,
	}
	exec := &core.Run{
		ID:       33,
		ActionID: step.ID,
	}

	publishRunAudit(t.Context(), bus, nil, step, exec, "run.watch", "completed", map[string]any{
		"invocation_id": "inv-1",
		"output_chars":  128,
	})

	select {
	case ev := <-sub.C:
		if ev.Type != core.EventRunAudit {
			t.Fatalf("event type = %s, want %s", ev.Type, core.EventRunAudit)
		}
		if ev.WorkItemID != step.WorkItemID || ev.ActionID != step.ID || ev.RunID != exec.ID {
			t.Fatalf("unexpected event scope: %+v", ev)
		}
		if got, _ := ev.Data["kind"].(string); got != "run.watch" {
			t.Fatalf("kind = %q, want run.watch", got)
		}
		if got, _ := ev.Data["status"].(string); got != "completed" {
			t.Fatalf("status = %q, want completed", got)
		}
		if got, _ := ev.Data["invocation_id"].(string); got != "inv-1" {
			t.Fatalf("invocation_id = %q, want inv-1", got)
		}
		if got, _ := ev.Data["output_chars"].(int); got != 128 {
			t.Fatalf("output_chars = %v, want 128", ev.Data["output_chars"])
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for run.audit event")
	}
}
