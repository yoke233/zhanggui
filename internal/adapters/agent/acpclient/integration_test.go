package acpclient

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
)

func TestFullLifecycleWithRoleMetadata(t *testing.T) {
	requireACPClientIntegration(t)

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

	sess, err := c.NewSession(ctx, acpproto.NewSessionRequest{
		Cwd:        t.TempDir(),
		McpServers: []acpproto.McpServer{},
	})
	if err != nil {
		t.Fatalf("new session failed: %v", err)
	}

	res, err := c.Prompt(ctx, acpproto.PromptRequest{
		SessionId: sess,
		Prompt: []acpproto.ContentBlock{
			{Text: &acpproto.ContentBlockText{Text: "请回复测试完成"}},
		},
		Meta: map[string]any{
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

func (h *integrationHandler) ReadTextFile(context.Context, acpproto.ReadTextFileRequest) (acpproto.ReadTextFileResponse, error) {
	return acpproto.ReadTextFileResponse{}, nil
}

func (h *integrationHandler) WriteTextFile(context.Context, acpproto.WriteTextFileRequest) (acpproto.WriteTextFileResponse, error) {
	h.mu.Lock()
	h.writeHits++
	h.mu.Unlock()
	return acpproto.WriteTextFileResponse{}, nil
}

func (h *integrationHandler) RequestPermission(context.Context, acpproto.RequestPermissionRequest) (acpproto.RequestPermissionResponse, error) {
	h.mu.Lock()
	h.permissionHits++
	h.mu.Unlock()
	return acpproto.RequestPermissionResponse{
		Outcome: acpproto.RequestPermissionOutcome{
			Cancelled: &acpproto.RequestPermissionOutcomeCancelled{Outcome: "cancelled"},
		},
	}, nil
}

func (h *integrationHandler) CreateTerminal(context.Context, acpproto.CreateTerminalRequest) (acpproto.CreateTerminalResponse, error) {
	return acpproto.CreateTerminalResponse{TerminalId: "it1"}, nil
}

func (h *integrationHandler) KillTerminalCommand(context.Context, acpproto.KillTerminalCommandRequest) (acpproto.KillTerminalCommandResponse, error) {
	return acpproto.KillTerminalCommandResponse{}, nil
}

func (h *integrationHandler) TerminalOutput(context.Context, acpproto.TerminalOutputRequest) (acpproto.TerminalOutputResponse, error) {
	return acpproto.TerminalOutputResponse{}, nil
}

func (h *integrationHandler) ReleaseTerminal(context.Context, acpproto.ReleaseTerminalRequest) (acpproto.ReleaseTerminalResponse, error) {
	return acpproto.ReleaseTerminalResponse{}, nil
}

func (h *integrationHandler) WaitForTerminalExit(context.Context, acpproto.WaitForTerminalExitRequest) (acpproto.WaitForTerminalExitResponse, error) {
	return acpproto.WaitForTerminalExitResponse{}, nil
}

func (h *integrationHandler) SessionUpdate(context.Context, acpproto.SessionNotification) error {
	return nil
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
