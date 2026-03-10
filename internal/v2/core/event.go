package core

import (
	"context"
	"time"
)

// EventType identifies the kind of domain event.
type EventType string

const (
	EventFlowQueued    EventType = "flow.queued"
	EventFlowStarted   EventType = "flow.started"
	EventFlowCompleted EventType = "flow.completed"
	EventFlowFailed    EventType = "flow.failed"
	EventFlowCancelled EventType = "flow.cancelled"

	EventStepReady     EventType = "step.ready"
	EventStepStarted   EventType = "step.started"
	EventStepCompleted EventType = "step.completed"
	EventStepFailed    EventType = "step.failed"
	EventStepBlocked   EventType = "step.blocked"

	EventExecCreated   EventType = "exec.created"
	EventExecStarted   EventType = "exec.started"
	EventExecSucceeded EventType = "exec.succeeded"
	EventExecFailed    EventType = "exec.failed"

	EventGatePassed   EventType = "gate.passed"
	EventGateRejected EventType = "gate.rejected"

	// Agent output events — discriminated by Data["type"].
	EventExecAgentOutput EventType = "exec.agent_output"

	// Chat events for LeadAgent direct conversations.
	EventChatOutput EventType = "chat.output"
)

// IsTransientAgentEvent returns true for streaming chunk events that should
// NOT be persisted (they are only useful for real-time WebSocket broadcast).
// Aggregated events (agent_message, agent_thought, tool_call, done) ARE persisted.
func IsTransientAgentEvent(ev Event) bool {
	if ev.Type != EventExecAgentOutput && ev.Type != EventChatOutput {
		return false
	}
	subType, _ := ev.Data["type"].(string)
	switch subType {
	case "agent_message_chunk", "agent_thought_chunk", "user_message_chunk":
		return true
	}
	return false
}

// Event is a domain event emitted during Flow execution.
type Event struct {
	ID        int64          `json:"id"`
	Type      EventType      `json:"type"`
	FlowID    int64          `json:"flow_id,omitempty"`
	StepID    int64          `json:"step_id,omitempty"`
	ExecID    int64          `json:"exec_id,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

// EventBus is the publish/subscribe interface for domain events.
// Defined in core, implemented in engine or a dedicated package.
type EventBus interface {
	Publish(ctx context.Context, event Event)
	Subscribe(opts SubscribeOpts) *Subscription
}

// SubscribeOpts configures an event subscription.
type SubscribeOpts struct {
	Types      []EventType
	BufferSize int
}

// Subscription represents an active event subscription.
type Subscription struct {
	C      <-chan Event
	Cancel func()
}
