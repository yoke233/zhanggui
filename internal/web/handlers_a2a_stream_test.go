package web

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/teamleader"
)

func TestA2AStream_ReturnsIncrementalEvents(t *testing.T) {
	bridge := &fakeA2ABridge{
		sendFn: func(input teamleader.A2ASendMessageInput) (*teamleader.A2ATaskSnapshot, error) {
			if input.ProjectID != "proj-stream" {
				t.Fatalf("project id = %q, want %q", input.ProjectID, "proj-stream")
			}
			return &teamleader.A2ATaskSnapshot{
				TaskID:    "task-stream",
				ProjectID: "proj-stream",
				SessionID: "ctx-stream",
				State:     teamleader.A2ATaskStateWorking,
			}, nil
		},
	}
	srv := NewServer(Config{
		A2AEnabled: true,
		Token:      "a2a-token",
		A2AVersion: "0.3",
		A2ABridge:  bridge,
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	reqBody := `{
		"jsonrpc":"2.0",
		"id":"stream-1",
		"method":"message/stream",
		"params":{
			"message":{
				"messageId":"m-stream-1",
				"role":"user",
				"parts":[{"kind":"text","text":"hello stream world"}]
			},
			"metadata":{"project_id":"proj-stream"}
		}
	}`

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/a2a", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer a2a-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d, body=%s", resp.StatusCode, string(raw))
	}
	if got := resp.Header.Get("Content-Type"); !strings.Contains(got, "text/event-stream") {
		t.Fatalf("expected text/event-stream, got %q", got)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read stream body failed: %v", err)
	}
	body := string(raw)
	if strings.Count(body, "event: delta\n") < 2 {
		t.Fatalf("expected at least two delta events, body=%q", body)
	}
	if !strings.Contains(body, `event: task`) {
		t.Fatalf("expected task event, body=%q", body)
	}
	if !strings.Contains(body, `"id":"task-stream"`) {
		t.Fatalf("expected streamed task id, body=%q", body)
	}
	if !strings.Contains(body, `event: done`) {
		t.Fatalf("expected done event, body=%q", body)
	}
}

func TestA2AUnauthorized_StreamReturns401(t *testing.T) {
	srv := NewServer(Config{
		A2AEnabled: true,
		Token:      "a2a-token",
		A2AVersion: "0.3",
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	reqBody := `{
		"jsonrpc":"2.0",
		"id":"stream-auth-1",
		"method":"message/stream",
		"params":{
			"message":{
				"messageId":"m-stream-auth-1",
				"role":"user",
				"parts":[{"kind":"text","text":"hello stream world"}]
			},
			"metadata":{"project_id":"proj-stream"}
		}
	}`

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/a2a", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 401, got %d, body=%s", resp.StatusCode, string(raw))
	}
}

func TestA2AStream_RequestContextCanceledDoesNotHang(t *testing.T) {
	bridge := &fakeA2ABridge{
		sendFn: func(input teamleader.A2ASendMessageInput) (*teamleader.A2ATaskSnapshot, error) {
			return &teamleader.A2ATaskSnapshot{
				TaskID:    "task-stream-canceled",
				ProjectID: "proj-stream",
				SessionID: "ctx-stream-canceled",
				State:     teamleader.A2ATaskStateWorking,
			}, nil
		},
	}
	srv := NewServer(Config{
		A2AEnabled: true,
		Token:      "a2a-token",
		A2AVersion: "0.3",
		A2ABridge:  bridge,
	})

	reqBody := `{
		"jsonrpc":"2.0",
		"id":"stream-cancel-1",
		"method":"message/stream",
		"params":{
			"message":{
				"messageId":"m-stream-cancel-1",
				"role":"user",
				"parts":[{"kind":"text","text":"hello stream world"}]
			},
			"metadata":{"project_id":"proj-stream"}
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/a2a", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer a2a-token")
	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		srv.Handler().ServeHTTP(rec, req)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("stream handler blocked after request context canceled")
	}
}
