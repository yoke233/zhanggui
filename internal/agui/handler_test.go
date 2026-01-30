package agui

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func readNextEvent(t *testing.T, r *bufio.Reader) map[string]any {
	t.Helper()
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("read line: %v", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(line, "data: ") {
			raw := strings.TrimPrefix(line, "data: ")
			var ev map[string]any
			if err := json.Unmarshal([]byte(raw), &ev); err != nil {
				t.Fatalf("bad event json: %v (%s)", err, raw)
			}
			return ev
		}
	}
}

func TestHandler_DemoToolAndInterruptResume(t *testing.T) {
	runsDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	h, err := NewHandler(Options{
		RunsDir:  runsDir,
		BasePath: "/agui",
		Protocol: "agui.v0",
		Logger:   logger,
	})
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	mux := http.NewServeMux()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := &http.Client{Timeout: 10 * time.Second}

	// run#1: waits for a2ui action then interrupts
	{
		body := `{"threadId":"thread-test-1","runId":"run-test-1","workflow":"demo"}`
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/agui/run", strings.NewReader(body))
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("status=%d body=%s", resp.StatusCode, string(b))
		}

		reader := bufio.NewReader(resp.Body)
		var toolCallID string
		var interruptID string
		for i := 0; i < 200; i++ {
			ev := readNextEvent(t, reader)
			if ev["type"] == "activity_message" {
				content, _ := ev["content"].(map[string]any)
				if content == nil {
					t.Fatalf("missing activity content")
				}
				msgs, _ := content["messages"].([]any)
				for _, msg := range msgs {
					m, _ := msg.(map[string]any)
					uc, _ := m["updateComponents"].(map[string]any)
					if uc == nil {
						continue
					}
					components, _ := uc["components"].([]any)
					for _, comp := range components {
						cm, _ := comp.(map[string]any)
						if cm == nil {
							continue
						}
						if cm["id"] != "choice_a" {
							continue
						}
						action, _ := cm["action"].(map[string]any)
						if action == nil {
							continue
						}
						ctx, _ := action["context"].(map[string]any)
						if ctx == nil {
							continue
						}
						toolCallID, _ = ctx["toolCallId"].(string)
					}
				}
				if toolCallID == "" {
					t.Fatalf("missing toolCallId in a2ui action context")
				}

				payload := map[string]any{
					"action": map[string]any{
						"name":              "choose_a",
						"surfaceId":         "main",
						"sourceComponentId": "choice_a",
						"timestamp":         time.Now().Format(time.RFC3339),
						"context": map[string]any{
							"runId":      "run-test-1",
							"threadId":   "thread-test-1",
							"toolCallId": toolCallID,
						},
					},
				}
				b, _ := json.Marshal(payload)
				res, err := client.Post(srv.URL+"/agui/action", "application/json", bytes.NewReader(b))
				if err != nil {
					t.Fatalf("action post: %v", err)
				}
				_ = res.Body.Close()
			}

			if ev["type"] == "RUN_FINISHED" {
				outcome, _ := ev["outcome"].(string)
				if outcome != "interrupt" {
					continue
				}
				interrupt, _ := ev["interrupt"].(map[string]any)
				if interrupt == nil {
					t.Fatalf("missing interrupt payload")
				}
				interruptID, _ = interrupt["id"].(string)
				if interruptID == "" {
					t.Fatalf("missing interrupt.id")
				}
				break
			}
		}

		if toolCallID == "" {
			t.Fatalf("did not see activity_message")
		}
		if interruptID == "" {
			t.Fatalf("did not see RUN_FINISHED interrupt")
		}

		if _, err := os.Stat(filepath.Join(runsDir, "run-test-1", "events", "events.jsonl")); err != nil {
			t.Fatalf("events.jsonl missing: %v", err)
		}
	}

	// run#2: resume and finish success
	{
		// Read interrupt id from run#1 state file
		st, err := readRunState(filepath.Join(runsDir, "run-test-1"))
		if err != nil {
			t.Fatalf("readRunState: %v", err)
		}
		if st.PendingInt == nil || st.PendingInt.ID == "" {
			t.Fatalf("run-test-1 missing pending interrupt")
		}

		body := map[string]any{
			"threadId": "thread-test-1",
			"runId":    "run-test-2",
			"workflow": "demo",
			"resume": map[string]any{
				"interruptId": st.PendingInt.ID,
				"payload":     map[string]any{"verdict": "approve"},
			},
		}
		b, _ := json.Marshal(body)
		req, err := http.NewRequest(http.MethodPost, srv.URL+"/agui/run", bytes.NewReader(b))
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			t.Fatalf("status=%d body=%s", resp.StatusCode, string(b))
		}

		reader := bufio.NewReader(resp.Body)
		var ok bool
		for i := 0; i < 100; i++ {
			ev := readNextEvent(t, reader)
			if ev["type"] == "RUN_FINISHED" {
				outcome, _ := ev["outcome"].(string)
				if outcome == "success" {
					ok = true
					break
				}
			}
		}
		if !ok {
			t.Fatalf("did not see RUN_FINISHED success")
		}

		if _, err := os.Stat(filepath.Join(runsDir, "run-test-2", "events", "result.json")); err != nil {
			t.Fatalf("result.json missing: %v", err)
		}
	}
}
