package secretary

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/yoke233/ai-workflow/internal/acpclient"
	"github.com/yoke233/ai-workflow/internal/core"
)

type recordingACPEventPublisher struct {
	mu     sync.Mutex
	events []core.Event
}

func (r *recordingACPEventPublisher) Publish(evt core.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, evt)
}

func (r *recordingACPEventPublisher) Events() []core.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]core.Event, len(r.events))
	copy(out, r.events)
	return out
}

type recordingChatRunEventRecorder struct {
	mu     sync.Mutex
	events []core.ChatRunEvent
}

func (r *recordingChatRunEventRecorder) AppendChatRunEvent(event core.ChatRunEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
	return nil
}

func (r *recordingChatRunEventRecorder) Events() []core.ChatRunEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]core.ChatRunEvent, len(r.events))
	copy(out, r.events)
	return out
}

func TestHandleWriteFilePublishesChangedEvent(t *testing.T) {
	cwd := t.TempDir()
	pub := &recordingACPEventPublisher{}
	handler := NewACPHandler(cwd, "chat-1", pub)

	req := acpclient.WriteFileRequest{
		Path:    "./plans/plan-a.md",
		Content: "hello secretary",
	}
	result, err := handler.HandleWriteFile(context.Background(), req)
	if err != nil {
		t.Fatalf("HandleWriteFile() error = %v", err)
	}
	if result.BytesWritten != len([]byte(req.Content)) {
		t.Fatalf("bytes written = %d, want %d", result.BytesWritten, len([]byte(req.Content)))
	}

	raw, err := os.ReadFile(filepath.Join(cwd, "plans", "plan-a.md"))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(raw) != req.Content {
		t.Fatalf("written content = %q, want %q", string(raw), req.Content)
	}

	events := pub.Events()
	if len(events) != 1 {
		t.Fatalf("published events = %d, want 1", len(events))
	}
	if events[0].Type != core.EventSecretaryFilesChanged {
		t.Fatalf("event type = %q, want %q", events[0].Type, core.EventSecretaryFilesChanged)
	}
	if events[0].Data["session_id"] != "chat-1" {
		t.Fatalf("event session_id = %q, want %q", events[0].Data["session_id"], "chat-1")
	}
	if !strings.Contains(events[0].Data["file_paths"], "plans/plan-a.md") {
		t.Fatalf("event file_paths = %q, should contain %q", events[0].Data["file_paths"], "plans/plan-a.md")
	}

	sessionCtx := handler.SessionContext()
	if sessionCtx.SessionID != "chat-1" {
		t.Fatalf("session context id = %q, want %q", sessionCtx.SessionID, "chat-1")
	}
	if len(sessionCtx.ChangedFiles) != 1 || sessionCtx.ChangedFiles[0] != "plans/plan-a.md" {
		t.Fatalf("changed files = %#v, want [%q]", sessionCtx.ChangedFiles, "plans/plan-a.md")
	}
}

func TestHandleWriteFileRejectsPathOutsideScope(t *testing.T) {
	cwd := t.TempDir()
	pub := &recordingACPEventPublisher{}
	handler := NewACPHandler(cwd, "chat-1", pub)

	outsidePath := filepath.Join("..", "escape.md")
	if _, err := handler.HandleWriteFile(context.Background(), acpclient.WriteFileRequest{
		Path:    outsidePath,
		Content: "x",
	}); err == nil {
		t.Fatalf("expected out-of-scope error for path %q", outsidePath)
	}

	if len(pub.Events()) != 0 {
		t.Fatalf("no event should be published when write fails")
	}
}

func TestHandleRequestPermissionSelectsAllowAlwaysByPolicy(t *testing.T) {
	cwd := t.TempDir()
	handler := NewACPHandler(cwd, "chat-1", nil)
	handler.SetPermissionPolicy([]acpclient.PermissionRule{
		{
			Pattern: "fs/write_text_file",
			Scope:   "cwd",
			Action:  "allow_always",
		},
	})

	decision, err := handler.HandleRequestPermission(context.Background(), acpclient.PermissionRequest{
		Action:   "fs/write_text_file",
		Resource: filepath.Join(cwd, "plans", "plan-a.md"),
		Options: []acpclient.PermissionOption{
			{OptionID: "opt-allow-once", Kind: "allow_once", Name: "Allow once"},
			{OptionID: "opt-allow-always", Kind: "allow_always", Name: "Allow always"},
			{OptionID: "opt-reject-once", Kind: "reject_once", Name: "Reject once"},
		},
	})
	if err != nil {
		t.Fatalf("HandleRequestPermission() error = %v", err)
	}
	if decision.Outcome != "selected" {
		t.Fatalf("permission outcome = %q, want %q", decision.Outcome, "selected")
	}
	if decision.OptionID != "opt-allow-always" {
		t.Fatalf("permission option id = %q, want %q", decision.OptionID, "opt-allow-always")
	}
}

func TestHandleRequestPermissionUnknownScopeFallsBackToDefault(t *testing.T) {
	cwd := t.TempDir()
	handler := NewACPHandler(cwd, "chat-1", nil)
	handler.SetPermissionPolicy([]acpclient.PermissionRule{
		{
			Pattern: "fs/write_text_file",
			Scope:   "project",
			Action:  "allow_always",
		},
	})

	decision, err := handler.HandleRequestPermission(context.Background(), acpclient.PermissionRequest{
		Action:   "fs/write_text_file",
		Resource: filepath.Join(cwd, "plans", "plan-a.md"),
		Options: []acpclient.PermissionOption{
			{OptionID: "opt-allow-once", Kind: "allow_once", Name: "Allow once"},
			{OptionID: "opt-allow-always", Kind: "allow_always", Name: "Allow always"},
		},
	})
	if err != nil {
		t.Fatalf("HandleRequestPermission() error = %v", err)
	}
	if decision.Outcome != "selected" {
		t.Fatalf("permission outcome = %q, want %q", decision.Outcome, "selected")
	}
	if decision.OptionID != "opt-allow-once" {
		t.Fatalf("permission option id = %q, want %q", decision.OptionID, "opt-allow-once")
	}
}

func TestHandleRequestPermissionInvalidRuleActionCancels(t *testing.T) {
	cwd := t.TempDir()
	handler := NewACPHandler(cwd, "chat-1", nil)
	handler.SetPermissionPolicy([]acpclient.PermissionRule{
		{
			Pattern: "fs/write_text_file",
			Scope:   "cwd",
			Action:  "allow_forever",
		},
	})

	decision, err := handler.HandleRequestPermission(context.Background(), acpclient.PermissionRequest{
		Action:   "fs/write_text_file",
		Resource: filepath.Join(cwd, "plans", "plan-a.md"),
		Options: []acpclient.PermissionOption{
			{OptionID: "opt-allow-once", Kind: "allow_once", Name: "Allow once"},
			{OptionID: "opt-allow-always", Kind: "allow_always", Name: "Allow always"},
			{OptionID: "opt-reject-once", Kind: "reject_once", Name: "Reject once"},
		},
	})
	if err != nil {
		t.Fatalf("HandleRequestPermission() error = %v", err)
	}
	if decision.Outcome != "cancelled" {
		t.Fatalf("permission outcome = %q, want %q", decision.Outcome, "cancelled")
	}
	if decision.OptionID != "" {
		t.Fatalf("permission option id = %q, want empty", decision.OptionID)
	}
}

func TestHandleRequestPermissionEmptyPatternDoesNotActAsWildcard(t *testing.T) {
	cwd := t.TempDir()
	handler := NewACPHandler(cwd, "chat-1", nil)
	handler.SetPermissionPolicy([]acpclient.PermissionRule{
		{
			Pattern: "",
			Action:  "allow_always",
		},
	})

	decision, err := handler.HandleRequestPermission(context.Background(), acpclient.PermissionRequest{
		Action: "terminal/create",
		Options: []acpclient.PermissionOption{
			{OptionID: "opt-allow-once", Kind: "allow_once", Name: "Allow once"},
			{OptionID: "opt-allow-always", Kind: "allow_always", Name: "Allow always"},
		},
	})
	if err != nil {
		t.Fatalf("HandleRequestPermission() error = %v", err)
	}
	if decision.Outcome != "selected" {
		t.Fatalf("permission outcome = %q, want %q", decision.Outcome, "selected")
	}
	if decision.OptionID != "opt-allow-once" {
		t.Fatalf("permission option id = %q, want %q", decision.OptionID, "opt-allow-once")
	}
}

func TestHandleRequestPermissionUnknownRequestActionFallsBackToAllowOnce(t *testing.T) {
	handler := NewACPHandler(t.TempDir(), "chat-1", nil)

	decision, err := handler.HandleRequestPermission(context.Background(), acpclient.PermissionRequest{
		Action: "fs/delete_file",
		Options: []acpclient.PermissionOption{
			{OptionID: "opt-allow-once", Kind: "allow_once", Name: "Allow once"},
			{OptionID: "opt-allow-always", Kind: "allow_always", Name: "Allow always"},
			{OptionID: "opt-reject-once", Kind: "reject_once", Name: "Reject once"},
		},
	})
	if err != nil {
		t.Fatalf("HandleRequestPermission() error = %v", err)
	}
	if decision.Outcome != "selected" {
		t.Fatalf("permission outcome = %q, want %q", decision.Outcome, "selected")
	}
	if decision.OptionID != "opt-allow-once" {
		t.Fatalf("permission option id = %q, want %q", decision.OptionID, "opt-allow-once")
	}
}

func TestHandleRequestPermissionUnknownRequestActionWithoutOptionsAllows(t *testing.T) {
	handler := NewACPHandler(t.TempDir(), "chat-1", nil)

	decision, err := handler.HandleRequestPermission(context.Background(), acpclient.PermissionRequest{
		Action: "tool/execute",
	})
	if err != nil {
		t.Fatalf("HandleRequestPermission() error = %v", err)
	}
	if decision.Outcome != "allow" {
		t.Fatalf("permission outcome = %q, want %q", decision.Outcome, "allow")
	}
	if decision.OptionID != "" {
		t.Fatalf("permission option id = %q, want empty", decision.OptionID)
	}
}

func TestHandleRequestPermissionUnknownRequestActionWithoutAllowOptionCancels(t *testing.T) {
	handler := NewACPHandler(t.TempDir(), "chat-1", nil)

	decision, err := handler.HandleRequestPermission(context.Background(), acpclient.PermissionRequest{
		Action: "fs/delete_file",
		Options: []acpclient.PermissionOption{
			{OptionID: "opt-reject-once", Kind: "reject_once", Name: "Reject once"},
			{OptionID: "opt-reject-always", Kind: "reject_always", Name: "Reject always"},
		},
	})
	if err != nil {
		t.Fatalf("HandleRequestPermission() error = %v", err)
	}
	if decision.Outcome != "cancelled" {
		t.Fatalf("permission outcome = %q, want %q", decision.Outcome, "cancelled")
	}
	if decision.OptionID != "" {
		t.Fatalf("permission option id = %q, want empty", decision.OptionID)
	}
}

func TestHandleSessionUpdatePublishesMinimalData(t *testing.T) {
	pub := &recordingACPEventPublisher{}
	handler := NewACPHandler(t.TempDir(), "agent-session-1", pub)
	handler.SetProjectID("proj-1")
	handler.SetChatSessionID("chat-session-1")

	rawUpdate := `{"type":"agent_message","content":[{"type":"text","text":"hello"}]}`
	err := handler.HandleSessionUpdate(context.Background(), acpclient.SessionUpdate{
		SessionID:      "acp-session-fallback",
		Type:           "agent_message",
		Text:           "hello",
		Status:         "running",
		RawUpdateJSON:  rawUpdate,
		RawContentJSON: `{"text":"ignore-me"}`,
	})
	if err != nil {
		t.Fatalf("HandleSessionUpdate() error = %v", err)
	}

	events := pub.Events()
	if len(events) != 1 {
		t.Fatalf("published events = %d, want 1", len(events))
	}
	if events[0].Type != core.EventChatRunUpdate {
		t.Fatalf("event type = %q, want %q", events[0].Type, core.EventChatRunUpdate)
	}

	wantData := map[string]string{
		"session_id":       "chat-session-1",
		"agent_session_id": "agent-session-1",
		"acp_update_json":  rawUpdate,
	}
	if len(events[0].Data) != len(wantData) {
		t.Fatalf("event data size = %d, want %d, data=%#v", len(events[0].Data), len(wantData), events[0].Data)
	}
	for key, wantValue := range wantData {
		if got := events[0].Data[key]; got != wantValue {
			t.Fatalf("event data[%q] = %q, want %q", key, got, wantValue)
		}
	}

	unexpectedKeys := []string{"update_type", "text", "status", "acp_content_json"}
	for _, key := range unexpectedKeys {
		if _, ok := events[0].Data[key]; ok {
			t.Fatalf("event data should not contain %q, data=%#v", key, events[0].Data)
		}
	}
}

func TestHandleSessionUpdatePersistsNonChunkEvent(t *testing.T) {
	pub := &recordingACPEventPublisher{}
	recorder := &recordingChatRunEventRecorder{}
	handler := NewACPHandler(t.TempDir(), "agent-session-1", pub)
	handler.SetProjectID("proj-1")
	handler.SetChatSessionID("chat-session-1")
	handler.SetRunEventRecorder(recorder)

	if err := handler.HandleSessionUpdate(context.Background(), acpclient.SessionUpdate{
		SessionID:     "acp-session-fallback",
		Type:          "tool_call",
		Status:        "pending",
		RawUpdateJSON: `{"sessionUpdate":"tool_call","title":"Terminal","status":"pending"}`,
	}); err != nil {
		t.Fatalf("HandleSessionUpdate() error = %v", err)
	}

	events := recorder.Events()
	if len(events) != 1 {
		t.Fatalf("persisted events = %d, want 1", len(events))
	}
	if events[0].SessionID != "chat-session-1" || events[0].ProjectID != "proj-1" {
		t.Fatalf("unexpected persisted event identity: %#v", events[0])
	}
	if events[0].EventType != "chat_run_update" || events[0].UpdateType != "tool_call" {
		t.Fatalf("unexpected persisted event type fields: %#v", events[0])
	}
	if events[0].Payload == nil {
		t.Fatalf("expected persisted payload")
	}
	if _, ok := events[0].Payload["acp"]; !ok {
		t.Fatalf("expected payload.acp to exist, got=%#v", events[0].Payload)
	}
}

func TestHandleSessionUpdateSkipsChunkPersistence(t *testing.T) {
	pub := &recordingACPEventPublisher{}
	recorder := &recordingChatRunEventRecorder{}
	handler := NewACPHandler(t.TempDir(), "agent-session-1", pub)
	handler.SetProjectID("proj-1")
	handler.SetChatSessionID("chat-session-1")
	handler.SetRunEventRecorder(recorder)

	if err := handler.HandleSessionUpdate(context.Background(), acpclient.SessionUpdate{
		SessionID:     "acp-session-fallback",
		Type:          "agent_message_chunk",
		Text:          "hello",
		RawUpdateJSON: `{"sessionUpdate":"agent_message_chunk","content":{"text":"hello"}}`,
	}); err != nil {
		t.Fatalf("HandleSessionUpdate() error = %v", err)
	}

	if got := len(recorder.Events()); got != 0 {
		t.Fatalf("persisted chunk events = %d, want 0", got)
	}
}
