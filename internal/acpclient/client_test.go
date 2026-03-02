package acpclient

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestClientLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	h := &recordingHandler{}
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
	if sess.SessionID == "" {
		t.Fatal("expected non-empty session id")
	}

	got, err := c.Prompt(ctx, PromptRequest{
		SessionID: sess.SessionID,
		Prompt:    "hello",
		Metadata: map[string]string{
			"role_id": "worker",
		},
	})
	if err != nil {
		t.Fatalf("prompt failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil prompt result")
	}
	if !strings.Contains(got.Text, "worker") {
		t.Fatalf("expected role metadata in response text, got %q", got.Text)
	}
	if h.writeCount() == 0 {
		t.Fatal("expected write-file tool call to be routed to handler")
	}
	if h.updateCount() == 0 {
		t.Fatal("expected session/update callback to be invoked")
	}
}

func TestClientCloseIsIdempotent(t *testing.T) {
	c, err := New(testLaunchConfig(t), &NopHandler{})
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	if err := c.Close(context.Background()); err != nil {
		t.Fatalf("first close failed: %v", err)
	}
	if err := c.Close(context.Background()); err != nil {
		t.Fatalf("second close failed: %v", err)
	}
}

func testLaunchConfig(t *testing.T) LaunchConfig {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	acpDir := filepath.Dir(thisFile)
	repoRoot := filepath.Clean(filepath.Join(acpDir, "..", ".."))
	fakeAgentPath := filepath.Join(repoRoot, "internal", "acpclient", "testdata", "fake_agent.go")
	return LaunchConfig{
		Command: "go",
		Args:    []string{"run", fakeAgentPath},
		WorkDir: repoRoot,
	}
}

type recordingHandler struct {
	mu            sync.Mutex
	writeFileHits int
	updateHits    int
}

func (h *recordingHandler) HandleReadFile(context.Context, ReadFileRequest) (ReadFileResult, error) {
	return ReadFileResult{Content: ""}, nil
}

func (h *recordingHandler) HandleWriteFile(context.Context, WriteFileRequest) (WriteFileResult, error) {
	h.mu.Lock()
	h.writeFileHits++
	h.mu.Unlock()
	return WriteFileResult{BytesWritten: 1}, nil
}

func (h *recordingHandler) HandleRequestPermission(context.Context, PermissionRequest) (PermissionDecision, error) {
	return PermissionDecision{Outcome: "allow"}, nil
}

func (h *recordingHandler) HandleTerminalCreate(context.Context, TerminalCreateRequest) (TerminalCreateResult, error) {
	return TerminalCreateResult{TerminalID: "t1"}, nil
}

func (h *recordingHandler) HandleTerminalWrite(context.Context, TerminalWriteRequest) (TerminalWriteResult, error) {
	return TerminalWriteResult{Written: 0}, nil
}

func (h *recordingHandler) HandleTerminalRead(context.Context, TerminalReadRequest) (TerminalReadResult, error) {
	return TerminalReadResult{}, nil
}

func (h *recordingHandler) HandleTerminalResize(context.Context, TerminalResizeRequest) (TerminalResizeResult, error) {
	return TerminalResizeResult{}, nil
}

func (h *recordingHandler) HandleTerminalClose(context.Context, TerminalCloseRequest) (TerminalCloseResult, error) {
	return TerminalCloseResult{}, nil
}

func (h *recordingHandler) HandleSessionUpdate(context.Context, SessionUpdate) error {
	h.mu.Lock()
	h.updateHits++
	h.mu.Unlock()
	return nil
}

func (h *recordingHandler) writeCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.writeFileHits
}

func (h *recordingHandler) updateCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.updateHits
}
