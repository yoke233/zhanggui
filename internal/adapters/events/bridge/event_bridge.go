package bridge

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
	"github.com/yoke233/ai-workflow/internal/core"
)

type Scope struct {
	WorkItemID int64
	ActionID   int64
	RunID      int64
	SessionID  string
}

type EventBridge struct {
	bus       core.EventBus
	eventType core.EventType
	scope     Scope

	lastActivity atomic.Int64

	mu             sync.Mutex
	pendingThought strings.Builder
	pendingMessage strings.Builder
}

func New(bus core.EventBus, eventType core.EventType, scope Scope) *EventBridge {
	b := &EventBridge{
		bus:       bus,
		eventType: eventType,
		scope:     scope,
	}
	b.lastActivity.Store(time.Now().UnixNano())
	return b
}

func (b *EventBridge) LastActivity() time.Time {
	return time.Unix(0, b.lastActivity.Load())
}

func (b *EventBridge) SetSessionID(sessionID string) {
	b.scope.SessionID = strings.TrimSpace(sessionID)
}

func (b *EventBridge) HandleSessionUpdate(ctx context.Context, update acpclient.SessionUpdate) error {
	b.lastActivity.Store(time.Now().UnixNano())

	switch update.Type {
	case "agent_thought_chunk":
		b.flushMessage(ctx)
	case "agent_message_chunk":
		b.flushThought(ctx)
	default:
		b.FlushPending(ctx)
	}

	switch update.Type {
	case "agent_thought_chunk":
		b.mu.Lock()
		b.pendingThought.WriteString(update.Text)
		b.mu.Unlock()
		b.publishChunk(ctx, update)
	case "agent_message_chunk":
		b.mu.Lock()
		b.pendingMessage.WriteString(update.Text)
		b.mu.Unlock()
		b.publishChunk(ctx, update)
	case "tool_call":
		b.publishToolCall(ctx, update)
	case "tool_call_update":
		if update.Status == "completed" {
			b.publishToolCallCompleted(ctx, update)
		}
	case "usage_update":
		b.publishUsageUpdate(ctx, update)
	default:
		b.publishChunk(ctx, update)
	}

	return nil
}

func (b *EventBridge) FlushPending(ctx context.Context) {
	b.flushThought(ctx)
	b.flushMessage(ctx)
}

func (b *EventBridge) PublishData(ctx context.Context, data map[string]any) {
	b.publish(ctx, data)
}

func (b *EventBridge) flushThought(ctx context.Context) {
	b.mu.Lock()
	thought := b.pendingThought.String()
	b.pendingThought.Reset()
	b.mu.Unlock()
	if thought != "" {
		b.publish(ctx, map[string]any{
			"type":    "agent_thought",
			"content": thought,
		})
	}
}

func (b *EventBridge) flushMessage(ctx context.Context) {
	b.mu.Lock()
	message := b.pendingMessage.String()
	b.pendingMessage.Reset()
	b.mu.Unlock()
	if message != "" {
		b.publish(ctx, map[string]any{
			"type":    "agent_message",
			"content": message,
		})
	}
}

func (b *EventBridge) publishChunk(ctx context.Context, update acpclient.SessionUpdate) {
	if update.Text == "" {
		return
	}
	b.publish(ctx, map[string]any{
		"type":    update.Type,
		"content": update.Text,
	})
}

func (b *EventBridge) publishToolCall(ctx context.Context, update acpclient.SessionUpdate) {
	data := map[string]any{"type": "tool_call"}
	var parsed struct {
		Title      string `json:"title"`
		ToolCallID string `json:"toolCallId"`
	}
	if json.Unmarshal(update.RawJSON, &parsed) == nil {
		if parsed.Title != "" {
			data["content"] = parsed.Title
		}
		if parsed.ToolCallID != "" {
			data["tool_call_id"] = parsed.ToolCallID
		}
	}
	b.publish(ctx, data)
}

func (b *EventBridge) publishToolCallCompleted(ctx context.Context, update acpclient.SessionUpdate) {
	data := map[string]any{"type": "tool_call_completed"}
	var parsed struct {
		ToolCallID string `json:"toolCallId"`
		RawOutput  struct {
			ExitCode int    `json:"exit_code"`
			Stdout   string `json:"stdout"`
			Stderr   string `json:"stderr"`
		} `json:"rawOutput"`
	}
	if json.Unmarshal(update.RawJSON, &parsed) == nil {
		data["tool_call_id"] = parsed.ToolCallID
		data["exit_code"] = parsed.RawOutput.ExitCode
		stdout := parsed.RawOutput.Stdout
		if len(stdout) > 2000 {
			stdout = stdout[:2000] + "...(truncated)"
		}
		data["content"] = stdout
		if parsed.RawOutput.Stderr != "" {
			stderr := parsed.RawOutput.Stderr
			if len(stderr) > 2000 {
				stderr = stderr[:2000] + "...(truncated)"
			}
			data["stderr"] = stderr
		}
	}
	b.publish(ctx, data)
}

func (b *EventBridge) publishUsageUpdate(ctx context.Context, update acpclient.SessionUpdate) {
	data := map[string]any{"type": "usage_update"}
	var usage struct {
		Size int64 `json:"size"`
		Used int64 `json:"used"`
	}
	if json.Unmarshal(update.RawJSON, &usage) == nil {
		data["usage_size"] = usage.Size
		data["usage_used"] = usage.Used
	}
	b.publish(ctx, data)
}

func (b *EventBridge) publish(ctx context.Context, data map[string]any) {
	ev := core.Event{
		Type:       b.eventType,
		WorkItemID: b.scope.WorkItemID,
		ActionID:   b.scope.ActionID,
		RunID:      b.scope.RunID,
		Data:      data,
		Timestamp: time.Now().UTC(),
	}
	if b.scope.SessionID != "" {
		if ev.Data == nil {
			ev.Data = map[string]any{}
		}
		ev.Data["session_id"] = b.scope.SessionID
	}
	b.bus.Publish(ctx, ev)
}
