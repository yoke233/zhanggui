package acpclient

import (
	"context"
	"strings"
	"testing"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
)

func TestIntegration_ClientLifecycle(t *testing.T) {
	requireACPClientIntegration(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

	sess, err := c.NewSession(ctx, acpproto.NewSessionRequest{
		Cwd:        t.TempDir(),
		McpServers: []acpproto.McpServer{},
	})
	if err != nil {
		t.Fatalf("new session failed: %v", err)
	}
	if sess == "" {
		t.Fatal("expected non-empty session id")
	}

	got, err := c.Prompt(ctx, acpproto.PromptRequest{
		SessionId: sess,
		Prompt: []acpproto.ContentBlock{
			{Text: &acpproto.ContentBlockText{Text: "hello"}},
		},
		Meta: map[string]any{
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
