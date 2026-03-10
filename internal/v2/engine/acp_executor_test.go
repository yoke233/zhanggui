package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/acpclient"
	"github.com/yoke233/ai-workflow/internal/v2/core"
)

func TestBuildPromptFromBriefing(t *testing.T) {
	t.Run("basic prompt", func(t *testing.T) {
		step := &core.Step{Name: "implement auth"}
		prompt := buildPromptFromBriefing("Implement JWT authentication", step)
		if !strings.Contains(prompt, "# Task") {
			t.Error("prompt should start with # Task header")
		}
		if !strings.Contains(prompt, "Implement JWT authentication") {
			t.Error("prompt should contain briefing snapshot")
		}
	})

	t.Run("with acceptance criteria", func(t *testing.T) {
		step := &core.Step{
			Name: "implement auth",
			AcceptanceCriteria: []string{
				"All tests pass",
				"No security vulnerabilities",
			},
		}
		prompt := buildPromptFromBriefing("Implement JWT authentication", step)
		if !strings.Contains(prompt, "# Acceptance Criteria") {
			t.Error("prompt should contain acceptance criteria header")
		}
		if !strings.Contains(prompt, "- All tests pass") {
			t.Error("prompt should contain first criterion")
		}
		if !strings.Contains(prompt, "- No security vulnerabilities") {
			t.Error("prompt should contain second criterion")
		}
	})

	t.Run("empty acceptance criteria", func(t *testing.T) {
		step := &core.Step{Name: "simple task"}
		prompt := buildPromptFromBriefing("Do something", step)
		if strings.Contains(prompt, "Acceptance Criteria") {
			t.Error("prompt should not contain acceptance criteria when empty")
		}
	})
}

func TestEventBridge_ChunkAggregation(t *testing.T) {
	bus := NewMemBus()
	bridge := NewEventBridge(bus, core.EventExecAgentOutput, EventBridgeScope{
		FlowID: 1, StepID: 2, ExecID: 3,
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
	bridge := NewEventBridge(bus, core.EventExecAgentOutput, EventBridgeScope{
		FlowID: 1, StepID: 2, ExecID: 3,
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
	bridge := NewEventBridge(bus, core.EventExecAgentOutput, EventBridgeScope{
		FlowID: 1, StepID: 2, ExecID: 3,
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
	bridge := NewEventBridge(bus, core.EventChatOutput, EventBridgeScope{
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
	bridge := NewEventBridge(bus, core.EventExecAgentOutput, EventBridgeScope{})

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
		{core.EventExecAgentOutput, "agent_message_chunk", true},
		{core.EventExecAgentOutput, "agent_thought_chunk", true},
		{core.EventExecAgentOutput, "user_message_chunk", true},
		{core.EventExecAgentOutput, "agent_message", false},
		{core.EventExecAgentOutput, "agent_thought", false},
		{core.EventExecAgentOutput, "tool_call", false},
		{core.EventExecAgentOutput, "done", false},
		{core.EventChatOutput, "agent_message_chunk", true},
		{core.EventChatOutput, "agent_message", false},
		{core.EventFlowStarted, "agent_message_chunk", false}, // wrong event type
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
