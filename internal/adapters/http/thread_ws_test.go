package api

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/yoke233/ai-workflow/internal/core"
)

type stubThreadAgentRuntime struct {
	activeProfileIDs []string
	sendCalls        []stubThreadSendCall
	sendErr          error
	cleanupCalls     []int64
	cleanupErr       error
}

type stubThreadSendCall struct {
	threadID  int64
	profileID string
	message   string
}

func (s *stubThreadAgentRuntime) InviteAgent(context.Context, int64, string) (*core.ThreadMember, error) {
	return nil, nil
}

func (s *stubThreadAgentRuntime) WaitAgentReady(context.Context, int64, string) error {
	return nil
}

func (s *stubThreadAgentRuntime) SendMessage(_ context.Context, threadID int64, profileID string, message string) error {
	if s.sendErr != nil {
		return s.sendErr
	}
	s.sendCalls = append(s.sendCalls, stubThreadSendCall{
		threadID:  threadID,
		profileID: profileID,
		message:   message,
	})
	return nil
}

func (s *stubThreadAgentRuntime) RemoveAgent(context.Context, int64, int64) error {
	return nil
}

func (s *stubThreadAgentRuntime) CleanupThread(_ context.Context, threadID int64) error {
	if s.cleanupErr != nil {
		return s.cleanupErr
	}
	s.cleanupCalls = append(s.cleanupCalls, threadID)
	return nil
}

func (s *stubThreadAgentRuntime) ActiveAgentProfileIDs(threadID int64) []string {
	out := make([]string, 0, len(s.activeProfileIDs))
	for _, profileID := range s.activeProfileIDs {
		out = append(out, profileID)
	}
	_ = threadID
	return out
}

type stubThreadAgentRegistry struct {
	profiles map[string]*core.AgentProfile
}

func (s *stubThreadAgentRegistry) GetProfile(_ context.Context, id string) (*core.AgentProfile, error) {
	return s.ResolveByID(context.Background(), id)
}

func (s *stubThreadAgentRegistry) ListProfiles(_ context.Context) ([]*core.AgentProfile, error) {
	out := make([]*core.AgentProfile, 0, len(s.profiles))
	for _, profile := range s.profiles {
		out = append(out, profile)
	}
	return out, nil
}

func (s *stubThreadAgentRegistry) CreateProfile(context.Context, *core.AgentProfile) error {
	return nil
}

func (s *stubThreadAgentRegistry) UpdateProfile(context.Context, *core.AgentProfile) error {
	return nil
}

func (s *stubThreadAgentRegistry) DeleteProfile(context.Context, string) error {
	return nil
}

func (s *stubThreadAgentRegistry) ResolveForAction(context.Context, *core.Action) (*core.AgentProfile, error) {
	return nil, core.ErrProfileNotFound
}

func (s *stubThreadAgentRegistry) ResolveByID(_ context.Context, id string) (*core.AgentProfile, error) {
	profile, ok := s.profiles[id]
	if !ok {
		return nil, core.ErrProfileNotFound
	}
	return profile, nil
}

func TestAPI_WebSocket_ThreadSend(t *testing.T) {
	_, ts := setupAPI(t)

	// Create a thread first.
	resp, err := post(ts, "/threads", map[string]any{
		"title":    "ws-test-thread",
		"owner_id": "user-1",
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var thread core.Thread
	if err := decodeJSON(resp, &thread); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Connect WebSocket.
	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Send thread.send message.
	if err := conn.WriteJSON(map[string]any{
		"type": "thread.send",
		"data": map[string]any{
			"request_id": "req-t1",
			"thread_id":  thread.ID,
			"message":    "hello thread",
			"sender_id":  "user-1",
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Read thread.ack response.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var ack struct {
		Type string `json:"type"`
		Data struct {
			RequestID string `json:"request_id"`
			ThreadID  int64  `json:"thread_id"`
			Status    string `json:"status"`
		} `json:"data"`
	}
	if err := conn.ReadJSON(&ack); err != nil {
		t.Fatalf("read: %v", err)
	}
	if ack.Type != "thread.ack" {
		t.Fatalf("ack type = %q, want thread.ack", ack.Type)
	}
	if ack.Data.RequestID != "req-t1" {
		t.Fatalf("request_id = %q, want req-t1", ack.Data.RequestID)
	}
	if ack.Data.ThreadID != thread.ID {
		t.Fatalf("thread_id = %d, want %d", ack.Data.ThreadID, thread.ID)
	}
	if ack.Data.Status != "accepted" {
		t.Fatalf("status = %q, want accepted", ack.Data.Status)
	}
}

func TestAPI_WebSocket_ThreadSend_TargetAgent(t *testing.T) {
	h, ts := setupAPI(t)
	threadPool := &stubThreadAgentRuntime{activeProfileIDs: []string{"worker-a", "worker-b"}}
	h.threadPool = threadPool

	resp, err := post(ts, "/threads", map[string]any{
		"title":    "ws-target-thread",
		"owner_id": "user-1",
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var thread core.Thread
	if err := decodeJSON(resp, &thread); err != nil {
		t.Fatalf("decode: %v", err)
	}

	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{
		"type": "thread.send",
		"data": map[string]any{
			"request_id":      "req-target",
			"thread_id":       thread.ID,
			"message":         "@worker-a 请处理这个问题",
			"sender_id":       "user-1",
			"target_agent_id": "worker-a",
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var ack struct {
		Type string `json:"type"`
		Data struct {
			RequestID string `json:"request_id"`
			ThreadID  int64  `json:"thread_id"`
			Status    string `json:"status"`
		} `json:"data"`
	}
	if err := conn.ReadJSON(&ack); err != nil {
		t.Fatalf("read: %v", err)
	}
	if ack.Type != "thread.ack" {
		t.Fatalf("ack type = %q, want thread.ack", ack.Type)
	}

	time.Sleep(100 * time.Millisecond)
	if len(threadPool.sendCalls) != 1 {
		t.Fatalf("send calls = %d, want 1", len(threadPool.sendCalls))
	}
	if threadPool.sendCalls[0].profileID != "worker-a" {
		t.Fatalf("profile_id = %q, want worker-a", threadPool.sendCalls[0].profileID)
	}
	if threadPool.sendCalls[0].message != "请处理这个问题" {
		t.Fatalf("message = %q, want mention-stripped content", threadPool.sendCalls[0].message)
	}

	msgs, err := h.store.ListThreadMessages(context.Background(), thread.ID, 10, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("messages = %d, want 1", len(msgs))
	}
	if got := msgs[0].Metadata["target_agent_id"]; got != "worker-a" {
		t.Fatalf("message metadata target_agent_id = %v, want worker-a", got)
	}
}

func TestAPI_WebSocket_ThreadSend_DefaultModeDoesNotBroadcastToAgents(t *testing.T) {
	h, ts := setupAPI(t)
	threadPool := &stubThreadAgentRuntime{activeProfileIDs: []string{"worker-a", "worker-b"}}
	h.threadPool = threadPool

	resp, err := post(ts, "/threads", map[string]any{
		"title": "ws-mention-only-thread",
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	var thread core.Thread
	if err := decodeJSON(resp, &thread); err != nil {
		t.Fatalf("decode: %v", err)
	}

	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{
		"type": "thread.send",
		"data": map[string]any{
			"request_id": "req-default",
			"thread_id":  thread.ID,
			"message":    "普通讨论消息",
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var ack struct {
		Type string `json:"type"`
	}
	if err := conn.ReadJSON(&ack); err != nil {
		t.Fatalf("read: %v", err)
	}
	if ack.Type != "thread.ack" {
		t.Fatalf("ack type = %q, want thread.ack", ack.Type)
	}

	time.Sleep(100 * time.Millisecond)
	if len(threadPool.sendCalls) != 0 {
		t.Fatalf("send calls = %d, want 0 when thread is in mention_only mode", len(threadPool.sendCalls))
	}
}

func TestAPI_WebSocket_ThreadSend_BroadcastModeRoutesToAllActiveAgents(t *testing.T) {
	h, ts := setupAPI(t)
	threadPool := &stubThreadAgentRuntime{activeProfileIDs: []string{"worker-a", "worker-b"}}
	h.threadPool = threadPool

	resp, err := post(ts, "/threads", map[string]any{
		"title": "ws-broadcast-thread",
		"metadata": map[string]any{
			"agent_routing_mode": "broadcast",
		},
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	var thread core.Thread
	if err := decodeJSON(resp, &thread); err != nil {
		t.Fatalf("decode: %v", err)
	}

	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{
		"type": "thread.send",
		"data": map[string]any{
			"request_id": "req-broadcast",
			"thread_id":  thread.ID,
			"message":    "请大家同步一下",
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var ack struct {
		Type string `json:"type"`
	}
	if err := conn.ReadJSON(&ack); err != nil {
		t.Fatalf("read: %v", err)
	}
	if ack.Type != "thread.ack" {
		t.Fatalf("ack type = %q, want thread.ack", ack.Type)
	}

	time.Sleep(100 * time.Millisecond)
	if len(threadPool.sendCalls) != 2 {
		t.Fatalf("send calls = %d, want 2 in broadcast mode", len(threadPool.sendCalls))
	}
}

func TestAPI_WebSocket_ThreadSend_AutoModeRoutesToBestMatchingAgent(t *testing.T) {
	h, ts := setupAPI(t)
	threadPool := &stubThreadAgentRuntime{activeProfileIDs: []string{"worker-a", "worker-b"}}
	h.threadPool = threadPool
	h.registry = &stubThreadAgentRegistry{
		profiles: map[string]*core.AgentProfile{
			"worker-a": {
				ID:           "worker-a",
				Name:         "frontend specialist",
				Capabilities: []string{"react", "ui"},
				Skills:       []string{"tailwind"},
				Role:         "worker",
			},
			"worker-b": {
				ID:           "worker-b",
				Name:         "backend specialist",
				Capabilities: []string{"go", "api"},
				Skills:       []string{"sql"},
				Role:         "worker",
			},
		},
	}

	resp, err := post(ts, "/threads", map[string]any{
		"title": "ws-auto-thread",
		"metadata": map[string]any{
			"agent_routing_mode": "auto",
		},
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	var thread core.Thread
	if err := decodeJSON(resp, &thread); err != nil {
		t.Fatalf("decode: %v", err)
	}

	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{
		"type": "thread.send",
		"data": map[string]any{
			"request_id": "req-auto",
			"thread_id":  thread.ID,
			"message":    "请负责 react 和 ui 的同学看一下这个页面",
			"sender_id":  "user-1",
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var ack struct {
		Type string `json:"type"`
	}
	if err := conn.ReadJSON(&ack); err != nil {
		t.Fatalf("read: %v", err)
	}
	if ack.Type != "thread.ack" {
		t.Fatalf("ack type = %q, want thread.ack", ack.Type)
	}

	time.Sleep(100 * time.Millisecond)
	if len(threadPool.sendCalls) != 1 {
		t.Fatalf("send calls = %d, want 1", len(threadPool.sendCalls))
	}
	if threadPool.sendCalls[0].profileID != "worker-a" {
		t.Fatalf("profile_id = %q, want worker-a", threadPool.sendCalls[0].profileID)
	}

	msgs, err := h.store.ListThreadMessages(context.Background(), thread.ID, 10, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("messages = %d, want 1", len(msgs))
	}
	autoRoutedTo, ok := msgs[0].Metadata["auto_routed_to"].([]any)
	if !ok || len(autoRoutedTo) != 1 || autoRoutedTo[0] != "worker-a" {
		t.Fatalf("message metadata auto_routed_to = %#v, want [worker-a]", msgs[0].Metadata["auto_routed_to"])
	}
}

func TestAPI_WebSocket_ThreadSend_TargetAgentUnavailable(t *testing.T) {
	h, ts := setupAPI(t)
	h.threadPool = &stubThreadAgentRuntime{activeProfileIDs: []string{"worker-a"}}

	resp, err := post(ts, "/threads", map[string]any{
		"title": "ws-target-thread",
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	var thread core.Thread
	if err := decodeJSON(resp, &thread); err != nil {
		t.Fatalf("decode: %v", err)
	}

	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{
		"type": "thread.send",
		"data": map[string]any{
			"request_id":      "req-miss",
			"thread_id":       thread.ID,
			"message":         "@worker-z 处理一下",
			"target_agent_id": "worker-z",
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var errResp struct {
		Type string `json:"type"`
		Data struct {
			Code      string `json:"code"`
			RequestID string `json:"request_id"`
			Error     string `json:"error"`
		} `json:"data"`
	}
	if err := conn.ReadJSON(&errResp); err != nil {
		t.Fatalf("read: %v", err)
	}
	if errResp.Type != "thread.error" {
		t.Fatalf("type = %q, want thread.error", errResp.Type)
	}
	if errResp.Data.Code != "TARGET_AGENT_UNAVAILABLE" {
		t.Fatalf("code = %q, want TARGET_AGENT_UNAVAILABLE", errResp.Data.Code)
	}

	msgs, err := h.store.ListThreadMessages(context.Background(), thread.ID, 10, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("messages = %d, want 0", len(msgs))
	}
}

func TestAPI_WebSocket_ThreadSend_InvalidThread(t *testing.T) {
	_, ts := setupAPI(t)

	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Send thread.send with non-existent thread_id.
	if err := conn.WriteJSON(map[string]any{
		"type": "thread.send",
		"data": map[string]any{
			"request_id": "req-bad",
			"thread_id":  9999,
			"message":    "nobody home",
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var errResp struct {
		Type string `json:"type"`
		Data struct {
			Code      string `json:"code"`
			RequestID string `json:"request_id"`
			Error     string `json:"error"`
		} `json:"data"`
	}
	if err := conn.ReadJSON(&errResp); err != nil {
		t.Fatalf("read: %v", err)
	}
	if errResp.Type != "thread.error" {
		t.Fatalf("type = %q, want thread.error", errResp.Type)
	}
	if errResp.Data.Code != "THREAD_NOT_FOUND" {
		t.Fatalf("code = %q, want THREAD_NOT_FOUND", errResp.Data.Code)
	}
}

func TestAPI_WebSocket_ThreadSend_PersistFailureReturnsError(t *testing.T) {
	h, ts := setupAPI(t)
	h.store = &failingCreateThreadMessageStore{
		Store: h.store,
		err:   errors.New("insert failed"),
	}

	resp, err := post(ts, "/threads", map[string]any{
		"title":    "ws-test-thread",
		"owner_id": "user-1",
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	var thread core.Thread
	if err := decodeJSON(resp, &thread); err != nil {
		t.Fatalf("decode: %v", err)
	}

	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{
		"type": "thread.send",
		"data": map[string]any{
			"request_id": "req-t1",
			"thread_id":  thread.ID,
			"message":    "hello thread",
			"sender_id":  "user-1",
		},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var errResp struct {
		Type string `json:"type"`
		Data struct {
			Code      string `json:"code"`
			RequestID string `json:"request_id"`
			Error     string `json:"error"`
		} `json:"data"`
	}
	if err := conn.ReadJSON(&errResp); err != nil {
		t.Fatalf("read: %v", err)
	}
	if errResp.Type != "thread.error" {
		t.Fatalf("type = %q, want thread.error", errResp.Type)
	}
	if errResp.Data.Code != "THREAD_SEND_FAILED" {
		t.Fatalf("code = %q, want THREAD_SEND_FAILED", errResp.Data.Code)
	}
	if errResp.Data.RequestID != "req-t1" {
		t.Fatalf("request_id = %q, want req-t1", errResp.Data.RequestID)
	}
}

func TestAPI_WebSocket_SubscribeThread(t *testing.T) {
	h, ts := setupAPI(t)

	// Create a thread.
	resp, _ := post(ts, "/threads", map[string]any{"title": "sub-test-thread"})
	var thread core.Thread
	decodeJSON(resp, &thread)

	// Connect WebSocket.
	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	// Subscribe to thread.
	if err := conn.WriteJSON(map[string]any{
		"type": "subscribe_thread",
		"data": map[string]any{"thread_id": thread.ID},
	}); err != nil {
		t.Fatalf("write subscribe: %v", err)
	}

	// Read subscribe ack.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var subAck struct {
		Type string `json:"type"`
		Data struct {
			ThreadID int64  `json:"thread_id"`
			Status   string `json:"status"`
		} `json:"data"`
	}
	if err := conn.ReadJSON(&subAck); err != nil {
		t.Fatalf("read subscribe ack: %v", err)
	}
	if subAck.Type != "thread.subscribed" {
		t.Fatalf("type = %q, want thread.subscribed", subAck.Type)
	}
	if subAck.Data.ThreadID != thread.ID {
		t.Fatalf("thread_id = %d, want %d", subAck.Data.ThreadID, thread.ID)
	}

	// Publish a thread.message event.
	h.bus.Publish(context.Background(), core.Event{
		Type:      core.EventThreadMessage,
		Data:      map[string]any{"thread_id": thread.ID, "message": "hello from bus"},
		Timestamp: time.Now().UTC(),
	})

	// Should receive the thread event.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var ev core.Event
	if err := conn.ReadJSON(&ev); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if ev.Type != core.EventThreadMessage {
		t.Fatalf("event type = %q, want thread.message", ev.Type)
	}
}

func TestAPI_WebSocket_UnsubscribeThread(t *testing.T) {
	h, ts := setupAPI(t)

	// Create a thread.
	resp, _ := post(ts, "/threads", map[string]any{"title": "unsub-test-thread"})
	var thread core.Thread
	decodeJSON(resp, &thread)

	// Connect WebSocket.
	wsURL := "ws" + ts.URL[4:] + "/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	// Subscribe to thread.
	conn.WriteJSON(map[string]any{
		"type": "subscribe_thread",
		"data": map[string]any{"thread_id": thread.ID},
	})
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var ack map[string]any
	conn.ReadJSON(&ack) // consume subscribe ack

	// Unsubscribe from thread.
	if err := conn.WriteJSON(map[string]any{
		"type": "unsubscribe_thread",
		"data": map[string]any{"thread_id": thread.ID},
	}); err != nil {
		t.Fatalf("write unsubscribe: %v", err)
	}

	// Read unsubscribe ack.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var unsubAck struct {
		Type string `json:"type"`
		Data struct {
			ThreadID int64  `json:"thread_id"`
			Status   string `json:"status"`
		} `json:"data"`
	}
	if err := conn.ReadJSON(&unsubAck); err != nil {
		t.Fatalf("read unsubscribe ack: %v", err)
	}
	if unsubAck.Type != "thread.unsubscribed" {
		t.Fatalf("type = %q, want thread.unsubscribed", unsubAck.Type)
	}

	// Publish thread event — should NOT be received (unsubscribed).
	h.bus.Publish(context.Background(), core.Event{
		Type:      core.EventThreadMessage,
		Data:      map[string]any{"thread_id": thread.ID, "message": "should not arrive"},
		Timestamp: time.Now().UTC(),
	})

	// Publish a non-thread event to confirm connection is still alive.
	h.bus.Publish(context.Background(), core.Event{
		Type:       core.EventWorkItemStarted,
		WorkItemID: 42,
		Timestamp:  time.Now().UTC(),
	})

	// Should receive the issue event but NOT the thread event.
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var ev core.Event
	if err := conn.ReadJSON(&ev); err != nil {
		t.Fatalf("read event: %v", err)
	}
	if ev.Type == core.EventThreadMessage {
		t.Fatal("received thread.message after unsubscribing")
	}
	if ev.Type != core.EventWorkItemStarted {
		t.Fatalf("expected issue.started, got %s", ev.Type)
	}
}
