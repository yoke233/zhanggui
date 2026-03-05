package web

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/yoke233/ai-workflow/internal/core"
)

func TestWSRequiresAuthWhenEnabled(t *testing.T) {
	hub := NewHub()
	srv := NewServer(Config{
		AuthEnabled: true,
		BearerToken: "ws-secret",
		Hub:         hub,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v3/ws"

	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Fatal("expected handshake error without token")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 handshake response, got %#v", resp)
	}

	queryTokenURL := wsURL + "?token=ws-secret"
	conn, resp, err := websocket.DefaultDialer.Dial(queryTokenURL, nil)
	if err != nil {
		t.Fatalf("dial with query token: %v", err)
	}
	defer conn.Close()
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101, got %d", resp.StatusCode)
	}

	header := http.Header{}
	header.Set("Authorization", "Bearer ws-secret")
	connByHeader, resp, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("dial with header token: %v", err)
	}
	defer connByHeader.Close()
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected 101, got %d", resp.StatusCode)
	}
}

func TestWSBroadcastAndRunsubscriptionFlow(t *testing.T) {
	hub := NewHub()
	srv := NewServer(Config{Hub: hub})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v3/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer conn.Close()

	if !waitForConnections(hub, 1, time.Second) {
		t.Fatal("ws connection did not register in hub")
	}

	hub.Broadcast(WSMessage{
		Type:      "stage_start",
		RunID:     "pipe-1",
		ProjectID: "proj-1",
	})
	stageStart := readWSMessage(t, conn, 2*time.Second)
	if stageStart.Type != "stage_start" {
		t.Fatalf("expected stage_start, got %s", stageStart.Type)
	}

	hub.Broadcast(WSMessage{
		Type:      "agent_output",
		RunID:     "pipe-1",
		ProjectID: "proj-1",
		Data: map[string]any{
			"content": "before-subscribe",
		},
	})

	subReq := map[string]string{
		"type":   "subscribe_run",
		"run_id": "pipe-1",
	}
	if err := conn.WriteJSON(subReq); err != nil {
		t.Fatalf("write subscribe message: %v", err)
	}

	subAck := readWSMessage(t, conn, 2*time.Second)
	if subAck.Type != "subscribed" || subAck.RunID != "pipe-1" {
		t.Fatalf("unexpected subscribe ack: %+v", subAck)
	}
	if content, ok := subAck.Data["content"].(string); ok && content == "before-subscribe" {
		t.Fatalf("received unsubscribed agent_output unexpectedly: %+v", subAck)
	}

	hub.Broadcast(WSMessage{
		Type:      "agent_output",
		RunID:     "pipe-1",
		ProjectID: "proj-1",
		Data: map[string]any{
			"content": "after-subscribe",
		},
	})
	out := readWSMessage(t, conn, 2*time.Second)
	if out.Type != "agent_output" || out.RunID != "pipe-1" {
		t.Fatalf("unexpected broadcast payload: %+v", out)
	}
}

func TestWSIssueSubscriptionReceivesCoreEventsByIssueID(t *testing.T) {
	hub := NewHub()
	srv := NewServer(Config{Hub: hub})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v3/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer conn.Close()

	if !waitForConnections(hub, 1, time.Second) {
		t.Fatal("ws connection did not register in hub")
	}

	if err := conn.WriteJSON(map[string]string{
		"type":     "subscribe_issue",
		"issue_id": "issue-1",
	}); err != nil {
		t.Fatalf("write subscribe_issue message: %v", err)
	}
	ack := readWSMessage(t, conn, 2*time.Second)
	if ack.Type != "subscribed" || ack.IssueID != "issue-1" {
		t.Fatalf("unexpected subscribe ack: %+v", ack)
	}

	hub.BroadcastCoreEvent(core.Event{
		Type:      core.EventIssueReady,
		RunID:     "pipe-2",
		ProjectID: "proj-1",
		IssueID:   "issue-2",
		Timestamp: time.Now(),
	})

	hub.BroadcastCoreEvent(core.Event{
		Type:      core.EventIssueReady,
		RunID:     "pipe-1",
		ProjectID: "proj-1",
		IssueID:   "issue-1",
		Timestamp: time.Now(),
		Data: map[string]string{
			"task_id": "task-1",
		},
	})

	got := readWSMessage(t, conn, 2*time.Second)
	if got.Type != string(core.EventIssueReady) {
		t.Fatalf("expected %q, got %q", core.EventIssueReady, got.Type)
	}
	if got.IssueID != "issue-1" {
		t.Fatalf("expected issue_id=issue-1, got %+v", got)
	}
	if got.RunID != "pipe-1" {
		t.Fatalf("expected run_id=pipe-1, got %+v", got)
	}
	if got.Data["task_id"] != "task-1" {
		t.Fatalf("expected task_id=task-1 in data, got %+v", got.Data)
	}
}

func TestWSIssueCreatedAlwaysBroadcastEvenWhenIssueIDPresent(t *testing.T) {
	hub := NewHub()
	srv := NewServer(Config{Hub: hub})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v3/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer conn.Close()

	if !waitForConnections(hub, 1, time.Second) {
		t.Fatal("ws connection did not register in hub")
	}

	hub.BroadcastCoreEvent(core.Event{
		Type:      core.EventIssueCreated,
		ProjectID: "proj-1",
		IssueID:   "issue-created-1",
		Timestamp: time.Now(),
	})

	got := readWSMessage(t, conn, 2*time.Second)
	if got.Type != string(core.EventIssueCreated) {
		t.Fatalf("expected %q, got %q", core.EventIssueCreated, got.Type)
	}
	if got.IssueID != "issue-created-1" {
		t.Fatalf("expected issue_id=issue-created-1, got %+v", got)
	}
}

func TestWSBroadcastCoreEventFallsBackIssueIDFromData(t *testing.T) {
	hub := NewHub()
	srv := NewServer(Config{Hub: hub})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v3/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer conn.Close()

	if !waitForConnections(hub, 1, time.Second) {
		t.Fatal("ws connection did not register in hub")
	}

	if err := conn.WriteJSON(map[string]string{
		"type":     "subscribe_issue",
		"issue_id": "issue-fallback-1",
	}); err != nil {
		t.Fatalf("write subscribe_issue message: %v", err)
	}
	ack := readWSMessage(t, conn, 2*time.Second)
	if ack.Type != "subscribed" || ack.IssueID != "issue-fallback-1" {
		t.Fatalf("unexpected subscribe ack: %+v", ack)
	}

	hub.BroadcastCoreEvent(core.Event{
		Type:      core.EventIssueReady,
		ProjectID: "proj-1",
		Timestamp: time.Now(),
		Data: map[string]string{
			"issue_id": "issue-fallback-1",
			"task_id":  "task-fallback-1",
		},
	})

	got := readWSMessage(t, conn, 2*time.Second)
	if got.Type != string(core.EventIssueReady) {
		t.Fatalf("expected %q, got %q", core.EventIssueReady, got.Type)
	}
	if got.IssueID != "issue-fallback-1" {
		t.Fatalf("expected fallback issue_id=issue-fallback-1, got %+v", got)
	}
	if got.Data["task_id"] != "task-fallback-1" {
		t.Fatalf("expected task_id=task-fallback-1, got %+v", got.Data)
	}
}

func TestWSBroadcastCoreEventParsesACPUpdateJSON(t *testing.T) {
	hub := NewHub()
	srv := NewServer(Config{Hub: hub})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v3/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer conn.Close()

	if !waitForConnections(hub, 1, time.Second) {
		t.Fatal("ws connection did not register in hub")
	}

	rawUpdate := `{"type":"agent_message","text":"hello","nested":{"x":1}}`
	hub.BroadcastCoreEvent(core.Event{
		Type:      core.EventStageStart,
		ProjectID: "proj-1",
		Timestamp: time.Now(),
		Data: map[string]string{
			"session_id":       "chat-session-1",
			"agent_session_id": "agent-session-1",
			"acp_update_json":  rawUpdate,
			"acp_content_json": `{"should":"drop"}`,
			"keep":             "value",
		},
	})

	got := readWSMessage(t, conn, 2*time.Second)
	if got.Type != string(core.EventStageStart) {
		t.Fatalf("expected %q, got %q", core.EventStageStart, got.Type)
	}

	if _, ok := got.Data["acp_update_json"]; ok {
		t.Fatalf("expected acp_update_json removed, got data=%+v", got.Data)
	}
	if _, ok := got.Data["acp_content_json"]; ok {
		t.Fatalf("expected acp_content_json removed, got data=%+v", got.Data)
	}
	if got.Data["keep"] != "value" {
		t.Fatalf("expected keep=value, got data=%+v", got.Data)
	}

	acpPayload, ok := got.Data["acp"].(map[string]any)
	if !ok {
		t.Fatalf("expected data.acp object, got %#v", got.Data["acp"])
	}
	if acpPayload["type"] != "agent_message" {
		t.Fatalf("expected data.acp.type=agent_message, got %#v", acpPayload["type"])
	}
	if acpPayload["text"] != "hello" {
		t.Fatalf("expected data.acp.text=hello, got %#v", acpPayload["text"])
	}
	nested, ok := acpPayload["nested"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested object, got %#v", acpPayload["nested"])
	}
	if nested["x"] != float64(1) {
		t.Fatalf("expected nested.x=1, got %#v", nested["x"])
	}
}

func TestWSChatSessionSubscriptionRoutesChatEventsBySessionID(t *testing.T) {
	hub := NewHub()
	srv := NewServer(Config{Hub: hub})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v3/ws"
	connA, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws client A: %v", err)
	}
	defer connA.Close()

	connB, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws client B: %v", err)
	}
	defer connB.Close()

	if !waitForConnections(hub, 2, time.Second) {
		t.Fatal("ws connections did not register in hub")
	}

	if err := connA.WriteJSON(map[string]string{
		"type":       "subscribe_chat_session",
		"session_id": "chat-1",
	}); err != nil {
		t.Fatalf("write subscribe_chat_session message: %v", err)
	}

	ack := readWSMessage(t, connA, 2*time.Second)
	if ack.Type != "subscribed" {
		t.Fatalf("expected subscribed ack, got %+v", ack)
	}

	hub.BroadcastCoreEvent(core.Event{
		Type:      core.EventRunStarted,
		ProjectID: "proj-1",
		Timestamp: time.Now(),
		Data: map[string]string{
			"session_id": "chat-1",
		},
	})

	gotA := readWSMessage(t, connA, 2*time.Second)
	if gotA.Type != string(core.EventRunStarted) {
		t.Fatalf("expected run_started for subscribed client, got %+v", gotA)
	}
	if gotA.Data["session_id"] != "chat-1" {
		t.Fatalf("expected session_id=chat-1, got %+v", gotA.Data)
	}

	assertNoWSMessage(t, connB, 200*time.Millisecond)

	if err := connA.WriteJSON(map[string]string{
		"type":       "unsubscribe_chat_session",
		"session_id": "chat-1",
	}); err != nil {
		t.Fatalf("write unsubscribe_chat_session message: %v", err)
	}
	unsubAck := readWSMessage(t, connA, 2*time.Second)
	if unsubAck.Type != "unsubscribed" {
		t.Fatalf("expected unsubscribed ack, got %+v", unsubAck)
	}

	hub.BroadcastCoreEvent(core.Event{
		Type:      core.EventRunUpdate,
		ProjectID: "proj-1",
		Timestamp: time.Now(),
		Data: map[string]string{
			"session_id": "chat-1",
		},
	})

	assertNoWSMessage(t, connA, 200*time.Millisecond)
	assertNoWSMessage(t, connB, 200*time.Millisecond)
}

func TestWSChatSessionSubscriptionCleansUpOnDisconnect(t *testing.T) {
	hub := NewHub()
	srv := NewServer(Config{Hub: hub})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v3/ws"
	connA, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws client A: %v", err)
	}

	connB, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws client B: %v", err)
	}
	defer connB.Close()

	if !waitForConnections(hub, 2, time.Second) {
		t.Fatal("ws connections did not register in hub")
	}

	if err := connA.WriteJSON(map[string]string{
		"type":       "subscribe_chat_session",
		"session_id": "chat-1",
	}); err != nil {
		t.Fatalf("write subscribe_chat_session message: %v", err)
	}
	ack := readWSMessage(t, connA, 2*time.Second)
	if ack.Type != "subscribed" {
		t.Fatalf("expected subscribed ack, got %+v", ack)
	}

	hub.mu.RLock()
	_, exists := hub.chatSessionSubs["chat-1"]
	hub.mu.RUnlock()
	if !exists {
		t.Fatal("expected chat-1 subscribers to exist before disconnect")
	}

	if err := connA.Close(); err != nil {
		t.Fatalf("close ws client A: %v", err)
	}

	if !waitForConnections(hub, 1, time.Second) {
		t.Fatal("ws connection A did not unregister from hub")
	}
	if !waitForNoChatSessionSubscribers(hub, "chat-1", time.Second) {
		t.Fatal("chat-1 subscribers were not cleaned after disconnect")
	}

	hub.BroadcastCoreEvent(core.Event{
		Type:      core.EventRunStarted,
		ProjectID: "proj-1",
		Timestamp: time.Now(),
		Data: map[string]string{
			"session_id": "chat-1",
		},
	})
	assertNoWSMessage(t, connB, 200*time.Millisecond)
}

func TestWSChatSessionSubscriptionBroadcastsToMultipleSubscribers(t *testing.T) {
	hub := NewHub()
	srv := NewServer(Config{Hub: hub})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v3/ws"
	connA, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws client A: %v", err)
	}
	defer connA.Close()

	connB, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws client B: %v", err)
	}
	defer connB.Close()

	connC, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws client C: %v", err)
	}
	defer connC.Close()

	if !waitForConnections(hub, 3, time.Second) {
		t.Fatal("ws connections did not register in hub")
	}

	if err := connA.WriteJSON(map[string]string{
		"type":       "subscribe_chat_session",
		"session_id": "chat-1",
	}); err != nil {
		t.Fatalf("client A subscribe chat-1: %v", err)
	}
	if err := connB.WriteJSON(map[string]string{
		"type":       "subscribe_chat_session",
		"session_id": "chat-1",
	}); err != nil {
		t.Fatalf("client B subscribe chat-1: %v", err)
	}
	if err := connC.WriteJSON(map[string]string{
		"type":       "subscribe_chat_session",
		"session_id": "chat-2",
	}); err != nil {
		t.Fatalf("client C subscribe chat-2: %v", err)
	}

	if ack := readWSMessage(t, connA, 2*time.Second); ack.Type != "subscribed" || ack.SessionID != "chat-1" {
		t.Fatalf("unexpected client A subscribe ack: %+v", ack)
	}
	if ack := readWSMessage(t, connB, 2*time.Second); ack.Type != "subscribed" || ack.SessionID != "chat-1" {
		t.Fatalf("unexpected client B subscribe ack: %+v", ack)
	}
	if ack := readWSMessage(t, connC, 2*time.Second); ack.Type != "subscribed" || ack.SessionID != "chat-2" {
		t.Fatalf("unexpected client C subscribe ack: %+v", ack)
	}

	hub.BroadcastCoreEvent(core.Event{
		Type:      core.EventRunStarted,
		ProjectID: "proj-1",
		Timestamp: time.Now(),
		Data: map[string]string{
			"session_id": "chat-1",
		},
	})

	startA := readWSMessage(t, connA, 2*time.Second)
	if startA.Type != string(core.EventRunStarted) || startA.Data["session_id"] != "chat-1" {
		t.Fatalf("unexpected client A started event: %+v", startA)
	}
	startB := readWSMessage(t, connB, 2*time.Second)
	if startB.Type != string(core.EventRunStarted) || startB.Data["session_id"] != "chat-1" {
		t.Fatalf("unexpected client B started event: %+v", startB)
	}
	assertNoWSMessage(t, connC, 200*time.Millisecond)

	if err := connB.WriteJSON(map[string]string{
		"type":       "unsubscribe_chat_session",
		"session_id": "chat-1",
	}); err != nil {
		t.Fatalf("client B unsubscribe chat-1: %v", err)
	}
	if ack := readWSMessage(t, connB, 2*time.Second); ack.Type != "unsubscribed" || ack.SessionID != "chat-1" {
		t.Fatalf("unexpected client B unsubscribe ack: %+v", ack)
	}

	hub.BroadcastCoreEvent(core.Event{
		Type:      core.EventRunCompleted,
		ProjectID: "proj-1",
		Timestamp: time.Now(),
		Data: map[string]string{
			"session_id": "chat-1",
		},
	})

	completedA := readWSMessage(t, connA, 2*time.Second)
	if completedA.Type != string(core.EventRunCompleted) || completedA.Data["session_id"] != "chat-1" {
		t.Fatalf("unexpected client A completed event: %+v", completedA)
	}
	assertNoWSMessage(t, connB, 200*time.Millisecond)
	assertNoWSMessage(t, connC, 200*time.Millisecond)
}

func TestWSChatSessionSubscriptionReplaysCachedEventsForLateSubscriber(t *testing.T) {
	hub := NewHub()
	srv := NewServer(Config{Hub: hub})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v3/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws client: %v", err)
	}
	defer conn.Close()

	if !waitForConnections(hub, 1, time.Second) {
		t.Fatal("ws connection did not register in hub")
	}

	hub.BroadcastCoreEvent(core.Event{
		Type:      core.EventRunStarted,
		ProjectID: "proj-1",
		Timestamp: time.Now(),
		Data: map[string]string{
			"session_id": "chat-late",
		},
	})
	hub.BroadcastCoreEvent(core.Event{
		Type:      core.EventRunUpdate,
		ProjectID: "proj-1",
		Timestamp: time.Now(),
		Data: map[string]string{
			"session_id": "chat-late",
		},
	})
	hub.BroadcastCoreEvent(core.Event{
		Type:      core.EventRunCompleted,
		ProjectID: "proj-1",
		Timestamp: time.Now(),
		Data: map[string]string{
			"session_id": "chat-late",
		},
	})

	hub.mu.RLock()
	cachedCount := len(hub.chatSessionEventCache["chat-late"])
	hub.mu.RUnlock()
	if cachedCount != 3 {
		t.Fatalf("expected 3 cached events before subscribe, got %d", cachedCount)
	}

	if err := conn.WriteJSON(map[string]string{
		"type":       "subscribe_chat_session",
		"session_id": "chat-late",
	}); err != nil {
		t.Fatalf("subscribe chat-late: %v", err)
	}

	ack := readWSMessage(t, conn, 2*time.Second)
	if ack.Type != "subscribed" || ack.SessionID != "chat-late" {
		t.Fatalf("unexpected subscribe ack: %+v", ack)
	}

	started := readWSMessage(t, conn, 2*time.Second)
	if started.Type != string(core.EventRunStarted) {
		t.Fatalf("expected started replay event, got %+v", started)
	}
	if started.Data["session_id"] != "chat-late" {
		t.Fatalf("expected started replay session_id=chat-late, got %+v", started.Data)
	}

	updated := readWSMessage(t, conn, 2*time.Second)
	if updated.Type != string(core.EventRunUpdate) {
		t.Fatalf("expected update replay event, got %+v", updated)
	}
	if updated.Data["session_id"] != "chat-late" {
		t.Fatalf("expected update replay session_id=chat-late, got %+v", updated.Data)
	}

	completed := readWSMessage(t, conn, 2*time.Second)
	if completed.Type != string(core.EventRunCompleted) {
		t.Fatalf("expected completed replay event, got %+v", completed)
	}
	if completed.Data["session_id"] != "chat-late" {
		t.Fatalf("expected completed replay session_id=chat-late, got %+v", completed.Data)
	}
}

func waitForConnections(hub *Hub, want int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if hub.ConnectionCount() == want {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return hub.ConnectionCount() == want
}

func waitForNoChatSessionSubscribers(hub *Hub, sessionID string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	id := strings.TrimSpace(sessionID)
	for time.Now().Before(deadline) {
		hub.mu.RLock()
		subscribers := hub.chatSessionSubs[id]
		subscriberCount := len(subscribers)
		hub.mu.RUnlock()
		if subscriberCount == 0 {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	return len(hub.chatSessionSubs[id]) == 0
}

func readWSMessage(t *testing.T, conn *websocket.Conn, timeout time.Duration) WSMessage {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(timeout))

	_, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read ws message: %v", err)
	}

	var msg WSMessage
	if err := json.Unmarshal(payload, &msg); err != nil {
		t.Fatalf("unmarshal ws message: %v, payload=%s", err, string(payload))
	}
	return msg
}

func assertNoWSMessage(t *testing.T, conn *websocket.Conn, timeout time.Duration) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	_, _, err := conn.ReadMessage()
	if err == nil {
		t.Fatalf("expected no ws message within %s, but received one", timeout)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return
	}
	t.Fatalf("expected read timeout, got err=%v", err)
}
