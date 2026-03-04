package web

import (
	"encoding/json"
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

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws"

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

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws"
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

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws"
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

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws"
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

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws"
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

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/ws"
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
