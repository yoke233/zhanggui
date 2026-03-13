package executor

import (
	"strings"
	"testing"
	"time"

	acpproto "github.com/coder/acp-go-sdk"

	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
	eventbridge "github.com/yoke233/ai-workflow/internal/adapters/events/bridge"
	flowapp "github.com/yoke233/ai-workflow/internal/application/flow"
	"github.com/yoke233/ai-workflow/internal/core"
)

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

	t.Run("with step context", func(t *testing.T) {
		step := &core.Action{Name: "implement"}
		executionInput := flowapp.BuildRunInputFromSnapshot("Do something", step, true)
		if !strings.Contains(executionInput, "# Reference Materials") {
			t.Error("execution input should contain Reference Materials header when hasStepContext=true")
		}
		if !strings.Contains(executionInput, "skills/step-context/") {
			t.Error("execution input should reference skills/step-context/ path")
		}
	})

	t.Run("without step context", func(t *testing.T) {
		step := &core.Action{Name: "implement"}
		executionInput := flowapp.BuildRunInputFromSnapshot("Do something", step, false)
		if strings.Contains(executionInput, "Reference Materials") {
			t.Error("execution input should not contain Reference Materials when hasStepContext=false")
		}
	})
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
		factory := buildStepMCPFactory(&core.Action{Type: core.ActionExec}, "worker", 0, nil)
		if factory != nil {
			t.Fatal("expected nil factory")
		}
	})

	t.Run("composite step does not inject", func(t *testing.T) {
		factory := buildStepMCPFactory(&core.Action{Type: core.ActionPlan}, "worker", 1, resolver)
		if factory != nil {
			t.Fatal("expected nil factory for composite step")
		}
	})

	t.Run("exec step injects", func(t *testing.T) {
		factory := buildStepMCPFactory(&core.Action{Type: core.ActionExec}, "worker", 1, resolver)
		if factory == nil {
			t.Fatal("expected non-nil factory for exec step")
		}
		servers := factory(true)
		if len(servers) != 1 || servers[0].Sse == nil {
			t.Fatalf("unexpected servers: %+v", servers)
		}
	})

	t.Run("gate step injects", func(t *testing.T) {
		factory := buildStepMCPFactory(&core.Action{Type: core.ActionGate}, "worker", 1, resolver)
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
