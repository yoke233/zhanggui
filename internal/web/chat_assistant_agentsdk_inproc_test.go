//go:build agentsdk

package web

import (
	"context"
	"strings"
	"testing"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/acpclient"
)

func TestBuildAgentSDKModelFactoryRejectsUnknownProvider(t *testing.T) {
	_, err := buildAgentSDKModelFactory(map[string]string{
		"AGENTSDK_MODEL_PROVIDER": "unknown-provider",
	})
	if err == nil {
		t.Fatal("expected error for unsupported model provider")
	}
	if !strings.Contains(err.Error(), "AGENTSDK_MODEL_PROVIDER") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDefaultACPClientFactorySupportsAgentSDKInproc(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	factory := defaultACPClientFactory{}
	client, err := factory.New(ctx, acpclient.LaunchConfig{
		Command: agentsdkInprocLaunchCommand,
		WorkDir: root,
		Env: map[string]string{
			"AGENTSDK_MODEL_PROVIDER": "stub",
			"AGENTSDK_STUB_RESPONSE":  "inproc-ok",
		},
	}, &acpclient.NopHandler{}, acpclient.ClientCapabilities{})
	if err != nil {
		t.Fatalf("factory.New returned error: %v", err)
	}
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer closeCancel()
		if closeErr := client.Close(closeCtx); closeErr != nil {
			t.Fatalf("client.Close returned error: %v", closeErr)
		}
	}()

	session, err := client.NewSession(ctx, acpproto.NewSessionRequest{
		Cwd:        root,
		McpServers: []acpproto.McpServer{},
	})
	if err != nil {
		t.Fatalf("client.NewSession returned error: %v", err)
	}
	if strings.TrimSpace(string(session)) == "" {
		t.Fatal("expected non-empty session id")
	}

	promptResult, err := client.Prompt(ctx, acpproto.PromptRequest{
		SessionId: session,
		Prompt: []acpproto.ContentBlock{
			{Text: &acpproto.ContentBlockText{Text: "hello"}},
		},
	})
	if err != nil {
		t.Fatalf("client.Prompt returned error: %v", err)
	}
	if promptResult == nil {
		t.Fatal("expected non-nil prompt result")
	}
	if !strings.Contains(promptResult.Text, "inproc-ok") {
		t.Fatalf("prompt text = %q, want contains %q", promptResult.Text, "inproc-ok")
	}
}

func TestACPChatAssistantRejectsMissingWorkDirForAgentSDKInproc(t *testing.T) {
	resolver := &stubChatRoleResolver{
		agent: acpclient.AgentProfile{
			ID:            "agentsdk",
			LaunchCommand: agentsdkInprocLaunchCommand,
		},
		roles: map[string]acpclient.RoleProfile{
			"team_leader": {
				ID:      "team_leader",
				AgentID: "agentsdk",
			},
		},
	}
	factory := &recordingACPClientFactory{client: &stubACPClient{}}
	assistant := newACPChatAssistant(ACPChatAssistantDeps{
		DefaultRoleID: "team_leader",
		RoleResolver:  resolver,
		ClientFactory: factory,
	})

	_, err := assistant.Reply(context.Background(), ChatAssistantRequest{
		Message: "hello",
	})
	if err == nil {
		t.Fatal("expected error when workdir is missing in agentsdk-inproc mode")
	}
	if !strings.Contains(err.Error(), "workdir is required") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(factory.launches) != 0 {
		t.Fatalf("client factory should not be invoked, got launches=%d", len(factory.launches))
	}
}
