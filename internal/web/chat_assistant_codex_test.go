package web

import (
	"context"
	"errors"
	"testing"

	"github.com/user/ai-workflow/internal/acpclient"
)

func TestCodexChatAssistantReplyUsesDefaultRoleWhenEmpty(t *testing.T) {
	resolver := &stubChatRoleResolver{
		agent: acpclient.AgentProfile{
			ID:            "codex",
			LaunchCommand: "codex-agent-acp",
		},
		roles: map[string]acpclient.RoleProfile{
			"secretary": {
				ID:      "secretary",
				AgentID: "codex",
				Capabilities: acpclient.ClientCapabilities{
					FSRead:   true,
					FSWrite:  true,
					Terminal: true,
				},
			},
		},
	}
	client := &stubACPClient{
		newResp: acpclient.SessionInfo{SessionID: "sid-new"},
		promptResp: &acpclient.PromptResult{
			Text: "hello from codex acp",
		},
	}
	factory := &recordingACPClientFactory{client: client}
	assistant := newCodexChatAssistantForTest("codex", "gpt-5.3-codex", "high", ACPChatAssistantDeps{
		DefaultRoleID: "secretary",
		RoleResolver:  resolver,
		ClientFactory: factory,
	})

	got, err := assistant.Reply(context.Background(), ChatAssistantRequest{
		Message: "hello",
		WorkDir: "D:/repo/demo",
	})
	if err != nil {
		t.Fatalf("Reply returned error: %v", err)
	}
	if got.Reply != "hello from codex acp" {
		t.Fatalf("expected reply %q, got %q", "hello from codex acp", got.Reply)
	}
	if got.AgentSessionID != "sid-new" {
		t.Fatalf("expected session id %q, got %q", "sid-new", got.AgentSessionID)
	}
	if len(client.loadReqs) != 0 {
		t.Fatalf("expected no LoadSession calls on first turn, got %d", len(client.loadReqs))
	}
	if len(client.newReqs) != 1 {
		t.Fatalf("expected one NewSession call, got %d", len(client.newReqs))
	}
	if len(client.promptReqs) != 1 {
		t.Fatalf("expected one Prompt call, got %d", len(client.promptReqs))
	}
	if gotRole := client.newReqs[0].Metadata["role_id"]; gotRole != "secretary" {
		t.Fatalf("new metadata role_id = %q, want %q", gotRole, "secretary")
	}
	if gotRole := client.promptReqs[0].Metadata["role_id"]; gotRole != "secretary" {
		t.Fatalf("prompt metadata role_id = %q, want %q", gotRole, "secretary")
	}
}

func TestCodexChatAssistantReplyUsesResolvedRoleLaunchAndLoadSession(t *testing.T) {
	resolver := &stubChatRoleResolver{
		agent: acpclient.AgentProfile{
			ID:            "worker-agent",
			LaunchCommand: "worker-agent-acp",
			LaunchArgs:    []string{"--transport", "stdio"},
		},
		roles: map[string]acpclient.RoleProfile{
			"worker": {
				ID:      "worker",
				AgentID: "worker-agent",
				Capabilities: acpclient.ClientCapabilities{
					FSRead:   true,
					FSWrite:  true,
					Terminal: true,
				},
				SessionPolicy: acpclient.SessionPolicy{
					Reuse:             true,
					PreferLoadSession: true,
				},
			},
		},
	}
	client := &stubACPClient{
		loadResp: acpclient.SessionInfo{SessionID: "sid-loaded"},
		promptResp: &acpclient.PromptResult{
			Text: "continued",
		},
	}
	factory := &recordingACPClientFactory{client: client}
	assistant := newCodexChatAssistantForTest("codex", "", "", ACPChatAssistantDeps{
		DefaultRoleID: "secretary",
		RoleResolver:  resolver,
		ClientFactory: factory,
	})

	got, err := assistant.Reply(context.Background(), ChatAssistantRequest{
		Message:        "next question",
		Role:           "worker",
		WorkDir:        "D:/repo/demo",
		AgentSessionID: "sid-old",
	})
	if err != nil {
		t.Fatalf("Reply returned error: %v", err)
	}
	if got.AgentSessionID != "sid-loaded" {
		t.Fatalf("expected loaded session id %q, got %q", "sid-loaded", got.AgentSessionID)
	}
	if len(factory.launches) != 1 {
		t.Fatalf("expected one launch config, got %d", len(factory.launches))
	}
	launch := factory.launches[0]
	if launch.Command != "worker-agent-acp" {
		t.Fatalf("launch command = %q, want %q", launch.Command, "worker-agent-acp")
	}
	if len(launch.Args) != 2 || launch.Args[0] != "--transport" || launch.Args[1] != "stdio" {
		t.Fatalf("launch args = %#v, want [--transport stdio]", launch.Args)
	}
	if len(client.loadReqs) != 1 {
		t.Fatalf("expected one LoadSession call, got %d", len(client.loadReqs))
	}
	if len(client.newReqs) != 0 {
		t.Fatalf("expected no NewSession call when load succeeds, got %d", len(client.newReqs))
	}
	if len(client.promptReqs) != 1 {
		t.Fatalf("expected one Prompt call, got %d", len(client.promptReqs))
	}
}

func TestCodexChatAssistantReplyReturnsFactoryError(t *testing.T) {
	resolver := &stubChatRoleResolver{
		agent: acpclient.AgentProfile{
			ID:            "codex",
			LaunchCommand: "codex-agent-acp",
		},
		roles: map[string]acpclient.RoleProfile{
			"secretary": {
				ID:      "secretary",
				AgentID: "codex",
			},
		},
	}
	factory := &recordingACPClientFactory{err: errors.New("create client failed")}
	assistant := newCodexChatAssistantForTest("codex", "", "", ACPChatAssistantDeps{
		DefaultRoleID: "secretary",
		RoleResolver:  resolver,
		ClientFactory: factory,
	})

	_, err := assistant.Reply(context.Background(), ChatAssistantRequest{
		Message: "hello",
	})
	if err == nil {
		t.Fatal("expected factory error")
	}
}
