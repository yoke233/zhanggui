package acpclient

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFullLifecycleWithRoleMetadata(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	h := &integrationHandler{}
	c, err := New(testLaunchConfig(t), h, WithEventHandler(h))
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}
	defer func() {
		if err := c.Close(context.Background()); err != nil {
			t.Fatalf("close client failed: %v", err)
		}
	}()

	if err := c.Initialize(ctx, ClientCapabilities{FSRead: true, FSWrite: true, Terminal: true}); err != nil {
		t.Fatalf("initialize failed: %v", err)
	}

	sess, err := c.NewSession(ctx, NewSessionRequest{CWD: t.TempDir()})
	if err != nil {
		t.Fatalf("new session failed: %v", err)
	}

	res, err := c.Prompt(ctx, PromptRequest{
		SessionID: sess.SessionID,
		Prompt:    "请回复测试完成",
		Metadata: map[string]string{
			"role_id": "worker",
		},
	})
	if err != nil {
		t.Fatalf("prompt failed: %v", err)
	}

	if res == nil || res.StopReason == "" {
		t.Fatalf("expected non-empty prompt result, got %#v", res)
	}
	if !strings.Contains(res.Text, "worker") {
		t.Fatalf("expected role metadata in response text, got %q", res.Text)
	}
	if h.writeCount() == 0 {
		t.Fatal("expected write-file callback to be invoked")
	}
	if h.permissionCount() == 0 {
		t.Fatal("expected permission callback to be invoked")
	}
	if h.updateCount() == 0 {
		t.Fatal("expected session/update callback to be invoked")
	}
}

type integrationHandler struct {
	mu             sync.Mutex
	writeHits      int
	permissionHits int
	updateHits     int
}

func (h *integrationHandler) HandleReadFile(context.Context, ReadFileRequest) (ReadFileResult, error) {
	return ReadFileResult{}, nil
}

func (h *integrationHandler) HandleWriteFile(context.Context, WriteFileRequest) (WriteFileResult, error) {
	h.mu.Lock()
	h.writeHits++
	h.mu.Unlock()
	return WriteFileResult{BytesWritten: 1}, nil
}

func (h *integrationHandler) HandleRequestPermission(context.Context, PermissionRequest) (PermissionDecision, error) {
	h.mu.Lock()
	h.permissionHits++
	h.mu.Unlock()
	return PermissionDecision{Outcome: "allow"}, nil
}

func (h *integrationHandler) HandleTerminalCreate(context.Context, TerminalCreateRequest) (TerminalCreateResult, error) {
	return TerminalCreateResult{TerminalID: "it1"}, nil
}

func (h *integrationHandler) HandleTerminalWrite(context.Context, TerminalWriteRequest) (TerminalWriteResult, error) {
	return TerminalWriteResult{}, nil
}

func (h *integrationHandler) HandleTerminalRead(context.Context, TerminalReadRequest) (TerminalReadResult, error) {
	return TerminalReadResult{}, nil
}

func (h *integrationHandler) HandleTerminalResize(context.Context, TerminalResizeRequest) (TerminalResizeResult, error) {
	return TerminalResizeResult{}, nil
}

func (h *integrationHandler) HandleTerminalClose(context.Context, TerminalCloseRequest) (TerminalCloseResult, error) {
	return TerminalCloseResult{}, nil
}

func (h *integrationHandler) HandleSessionUpdate(context.Context, SessionUpdate) error {
	h.mu.Lock()
	h.updateHits++
	h.mu.Unlock()
	return nil
}

func (h *integrationHandler) writeCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.writeHits
}

func (h *integrationHandler) permissionCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.permissionHits
}

func (h *integrationHandler) updateCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.updateHits
}
