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
		Type:      core.EventIssueStarted,
		IssueID:   42,
		Timestamp: time.Now().UTC(),
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
	if ev.Type != core.EventIssueStarted {
		t.Fatalf("expected issue.started, got %s", ev.Type)
	}
}
