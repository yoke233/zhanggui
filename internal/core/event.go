package core

import (
	"context"
	"time"
)

// EventType identifies the kind of domain event.
type EventType string

const (
	EventWorkItemQueued    EventType = "work_item.queued"
	EventWorkItemStarted   EventType = "work_item.started"
	EventWorkItemCompleted EventType = "work_item.completed"
	EventWorkItemFailed    EventType = "work_item.failed"
	EventWorkItemCancelled EventType = "work_item.cancelled"

	EventActionReady     EventType = "action.ready"
	EventActionStarted   EventType = "action.started"
	EventActionCompleted EventType = "action.completed"
	EventActionFailed    EventType = "action.failed"
	EventActionBlocked   EventType = "action.blocked"

	EventRunCreated          EventType = "run.created"
	EventRunStarted          EventType = "run.started"
	EventRunSucceeded        EventType = "run.succeeded"
	EventRunFailed           EventType = "run.failed"
	EventExecutionAudit      EventType = "execution.audit"
	EventRunProbeRequested   EventType = "run.probe_requested"
	EventRunProbeSent        EventType = "run.probe_sent"
	EventRunProbeAnswered    EventType = "run.probe_answered"
	EventRunProbeTimeout     EventType = "run.probe_timeout"
	EventRunProbeUnreachable EventType = "run.probe_unreachable"

	EventGatePassed             EventType = "gate.passed"
	EventGateRejected           EventType = "gate.rejected"
	EventGateAwaitingHuman      EventType = "gate.awaiting_human"
	EventGateReworkLimitReached EventType = "gate.rework_limit_reached"

	// Action signal events -- agent/human explicit declarations.
	EventActionNeedHelp  EventType = "action.need_help"
	EventActionUnblocked EventType = "action.unblocked"
	EventActionSignal    EventType = "action.signal"

	// Agent output events -- discriminated by Data["type"].
	EventRunAgentOutput EventType = "run.agent_output"

	// Chat events for LeadAgent direct conversations.
	EventChatOutput            EventType = "chat.output"
	EventChatPermissionRequest EventType = "chat.permission_request"

	// Thread events for multi-participant discussion.
	EventThreadMessage       EventType = "thread.message"
	EventThreadAgentJoined   EventType = "thread.agent_joined"
	EventThreadAgentLeft     EventType = "thread.agent_left"
	EventThreadAgentOutput   EventType = "thread.agent_output"
	EventThreadAgentBooted   EventType = "thread.agent_booted"
	EventThreadAgentFailed   EventType = "thread.agent_failed"
	EventThreadAgentThinking EventType = "thread.agent_thinking"

	// Feature manifest events.
	EventManifestEntryUpdated EventType = "manifest.entry_updated"
	EventManifestGateChecked  EventType = "manifest.gate_checked"

	// Notification events.
	EventNotificationCreated EventType = "notification.created"
	EventNotificationRead    EventType = "notification.read"
	EventNotificationAllRead EventType = "notification.all_read"

	// Workspace events.
	EventWorkspaceWarning EventType = "workspace.warning"
)

// IsTransientAgentEvent returns true for streaming chunk events that should
// NOT be persisted (they are only useful for real-time WebSocket broadcast).
// Aggregated events (agent_message, agent_thought, tool_call, done) ARE persisted.
func IsTransientAgentEvent(ev Event) bool {
	if ev.Type != EventRunAgentOutput && ev.Type != EventChatOutput && ev.Type != EventThreadAgentOutput {
		return false
	}
	subType, _ := ev.Data["type"].(string)
	switch subType {
	case "agent_message_chunk", "agent_thought_chunk", "user_message_chunk":
		return true
	}
	return false
}

// Event category constants.
const (
	EventCategoryDomain    = "domain"
	EventCategoryToolAudit = "tool_audit"
)

// Event is a domain event emitted during WorkItem execution.
type Event struct {
	ID         int64          `json:"id"`
	Type       EventType      `json:"type"`
	Category   string         `json:"category,omitempty"`
	WorkItemID int64          `json:"work_item_id,omitempty"`
	ActionID   int64          `json:"action_id,omitempty"`
	RunID      int64          `json:"run_id,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
	Timestamp  time.Time      `json:"timestamp"`
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
