package web

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/acpclient"
	"github.com/yoke233/ai-workflow/internal/core"
)

func TestClaudeChatAssistantReplyUsesLoadSessionThenPrompt(t *testing.T) {
	resolver := &stubChatRoleResolver{
		agent: acpclient.AgentProfile{
			ID:            "claude",
			LaunchCommand: "claude-agent-acp",
			LaunchArgs:    []string{"--stdio"},
			Env: map[string]string{
				"CLAUDE_DEBUG": "1",
			},
		},
		roles: map[string]acpclient.RoleProfile{
			"reviewer": {
				ID:      "reviewer",
				AgentID: "claude",
				Capabilities: acpclient.ClientCapabilities{
					FSRead:   true,
					FSWrite:  false,
					Terminal: false,
				},
				SessionPolicy: acpclient.SessionPolicy{
					Reuse:             true,
					PreferLoadSession: true,
				},
			},
		},
	}
	client := &stubACPClient{
		loadResp: acpproto.SessionId("sid-loaded"),
		promptResp: &acpclient.PromptResult{
			Text: "hello from acp",
		},
	}
	factory := &recordingACPClientFactory{client: client}
	assistant := newClaudeChatAssistantForTest("claude", ACPChatAssistantDeps{
		DefaultRoleID: "team_leader",
		RoleResolver:  resolver,
		ClientFactory: factory,
	})

	got, err := assistant.Reply(context.Background(), ChatAssistantRequest{
		Message:        "hello",
		Role:           "reviewer",
		WorkDir:        "D:/repo/demo",
		AgentSessionID: "sid-old",
	})
	if err != nil {
		t.Fatalf("Reply returned error: %v", err)
	}

	if got.Reply != "hello from acp" {
		t.Fatalf("expected reply %q, got %q", "hello from acp", got.Reply)
	}
	if got.AgentSessionID != "sid-loaded" {
		t.Fatalf("expected session id %q, got %q", "sid-loaded", got.AgentSessionID)
	}
	if len(factory.launches) != 1 {
		t.Fatalf("expected one launch config, got %d", len(factory.launches))
	}
	launch := factory.launches[0]
	if launch.Command != "claude-agent-acp" {
		t.Fatalf("launch command = %q, want %q", launch.Command, "claude-agent-acp")
	}
	if len(launch.Args) != 1 || launch.Args[0] != "--stdio" {
		t.Fatalf("launch args = %#v, want [--stdio]", launch.Args)
	}
	if launch.WorkDir != "D:/repo/demo" {
		t.Fatalf("launch workdir = %q, want %q", launch.WorkDir, "D:/repo/demo")
	}
	if gotEnv := launch.Env["CLAUDE_DEBUG"]; gotEnv != "1" {
		t.Fatalf("launch env CLAUDE_DEBUG = %q, want %q", gotEnv, "1")
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
	if gotRole, _ := client.loadReqs[0].Meta["role_id"].(string); gotRole != "reviewer" {
		t.Fatalf("load metadata role_id = %q, want %q", gotRole, "reviewer")
	}
	if gotRole, _ := client.promptReqs[0].Meta["role_id"].(string); gotRole != "reviewer" {
		t.Fatalf("prompt metadata role_id = %q, want %q", gotRole, "reviewer")
	}
	if len(resolver.calls) != 1 || resolver.calls[0] != "reviewer" {
		t.Fatalf("resolver calls = %#v, want [reviewer]", resolver.calls)
	}
}

func TestClaudeChatAssistantReplyFallsBackToNewSessionWhenLoadFails(t *testing.T) {
	resolver := &stubChatRoleResolver{
		agent: acpclient.AgentProfile{
			ID:            "claude",
			LaunchCommand: "claude-agent-acp",
		},
		roles: map[string]acpclient.RoleProfile{
			"team_leader": {
				ID:      "team_leader",
				AgentID: "claude",
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
		loadErr: errors.New("session not found"),
		newResp: acpproto.SessionId("sid-new"),
		promptResp: &acpclient.PromptResult{
			Text: "new-session-reply",
		},
	}
	factory := &recordingACPClientFactory{client: client}
	assistant := newClaudeChatAssistantForTest("claude", ACPChatAssistantDeps{
		DefaultRoleID: "team_leader",
		RoleResolver:  resolver,
		ClientFactory: factory,
	})

	got, err := assistant.Reply(context.Background(), ChatAssistantRequest{
		Message:        "continue",
		WorkDir:        "D:/repo/demo",
		AgentSessionID: "sid-old",
	})
	if err != nil {
		t.Fatalf("Reply returned error: %v", err)
	}
	if got.AgentSessionID != "sid-new" {
		t.Fatalf("expected fallback new session id %q, got %q", "sid-new", got.AgentSessionID)
	}
	if len(client.loadReqs) != 1 {
		t.Fatalf("expected one LoadSession call, got %d", len(client.loadReqs))
	}
	if len(client.newReqs) != 1 {
		t.Fatalf("expected one NewSession call, got %d", len(client.newReqs))
	}
	if len(client.promptReqs) != 1 {
		t.Fatalf("expected one Prompt call, got %d", len(client.promptReqs))
	}
	if client.newReqs[0].Cwd != "D:/repo/demo" {
		t.Fatalf("new session cwd = %q, want %q", client.newReqs[0].Cwd, "D:/repo/demo")
	}
	if gotRole, _ := client.newReqs[0].Meta["role_id"].(string); gotRole != "team_leader" {
		t.Fatalf("new metadata role_id = %q, want %q", gotRole, "team_leader")
	}
}

func TestClaudeChatAssistantReplyPublishesWriteFileEventViaACPHandler(t *testing.T) {
	cwd := t.TempDir()
	pub := &recordingEventPublisher{}
	resolver := &stubChatRoleResolver{
		agent: acpclient.AgentProfile{
			ID:            "claude",
			LaunchCommand: "claude-agent-acp",
		},
		roles: map[string]acpclient.RoleProfile{
			"team_leader": {
				ID:      "team_leader",
				AgentID: "claude",
				Capabilities: acpclient.ClientCapabilities{
					FSRead:   true,
					FSWrite:  true,
					Terminal: true,
				},
			},
		},
	}
	client := &stubACPClient{
		newResp: acpproto.SessionId("sid-new"),
		promptResp: &acpclient.PromptResult{
			Text: "done",
		},
		writeReqOnPrompt: &acpproto.WriteTextFileRequest{
			Path:    filepath.Join("plans", "plan-a.md"),
			Content: "hello",
		},
	}
	factory := &recordingACPClientFactory{client: client}
	assistant := newClaudeChatAssistantForTest("claude", ACPChatAssistantDeps{
		DefaultRoleID:  "team_leader",
		RoleResolver:   resolver,
		ClientFactory:  factory,
		EventPublisher: pub,
	})

	_, err := assistant.Reply(context.Background(), ChatAssistantRequest{
		Message: "write plan",
		WorkDir: cwd,
	})
	if err != nil {
		t.Fatalf("Reply returned error: %v", err)
	}
	events := pub.Events()
	if len(events) == 0 {
		t.Fatal("expected files changed event")
	}
	if events[0].Type != core.EventTeamLeaderFilesChanged {
		t.Fatalf("event type = %q, want %q", events[0].Type, core.EventTeamLeaderFilesChanged)
	}
	if events[0].Data["session_id"] != "sid-new" {
		t.Fatalf("event session_id = %q, want %q", events[0].Data["session_id"], "sid-new")
	}
	if !strings.Contains(events[0].Data["file_paths"], "plans/plan-a.md") {
		t.Fatalf("event file_paths = %q, want contains %q", events[0].Data["file_paths"], "plans/plan-a.md")
	}
	writtenPath := filepath.Join(cwd, "plans", "plan-a.md")
	content, readErr := os.ReadFile(writtenPath)
	if readErr != nil {
		t.Fatalf("read written file failed: %v", readErr)
	}
	if string(content) != "hello" {
		t.Fatalf("written file content = %q, want %q", string(content), "hello")
	}
}

func TestClaudeChatAssistantReplyReturnsPromptError(t *testing.T) {
	resolver := &stubChatRoleResolver{
		agent: acpclient.AgentProfile{
			ID:            "claude",
			LaunchCommand: "claude-agent-acp",
		},
		roles: map[string]acpclient.RoleProfile{
			"team_leader": {
				ID:      "team_leader",
				AgentID: "claude",
			},
		},
	}
	client := &stubACPClient{
		newResp:   acpproto.SessionId("sid-new"),
		promptErr: errors.New("prompt failed"),
	}
	factory := &recordingACPClientFactory{client: client}
	assistant := newClaudeChatAssistantForTest("claude", ACPChatAssistantDeps{
		DefaultRoleID: "team_leader",
		RoleResolver:  resolver,
		ClientFactory: factory,
	})

	_, err := assistant.Reply(context.Background(), ChatAssistantRequest{
		Message: "hello",
	})
	if err == nil {
		t.Fatal("expected error when prompt fails")
	}
	if !strings.Contains(err.Error(), "prompt") {
		t.Fatalf("expected prompt error detail, got %v", err)
	}
}

type stubChatRoleResolver struct {
	mu    sync.Mutex
	agent acpclient.AgentProfile
	roles map[string]acpclient.RoleProfile
	calls []string
}

func (r *stubChatRoleResolver) Resolve(roleID string) (acpclient.AgentProfile, acpclient.RoleProfile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, roleID)
	role, ok := r.roles[roleID]
	if !ok {
		return acpclient.AgentProfile{}, acpclient.RoleProfile{}, errors.New("role not found")
	}
	return r.agent, role, nil
}

type recordingACPClientFactory struct {
	mu       sync.Mutex
	client   *stubACPClient
	err      error
	launches []acpclient.LaunchConfig
	caps     []acpclient.ClientCapabilities
	handlers []acpproto.Client
}

func (f *recordingACPClientFactory) New(
	_ context.Context,
	cfg acpclient.LaunchConfig,
	handler acpproto.Client,
	capabilities acpclient.ClientCapabilities,
) (ChatACPClient, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	f.launches = append(f.launches, cfg)
	f.caps = append(f.caps, capabilities)
	f.handlers = append(f.handlers, handler)
	if f.client != nil {
		f.client.handler = handler
		return f.client, nil
	}
	return &stubACPClient{handler: handler}, nil
}

type stubACPClient struct {
	mu sync.Mutex

	handler acpproto.Client

	loadReqs   []acpproto.LoadSessionRequest
	newReqs    []acpproto.NewSessionRequest
	promptReqs []acpproto.PromptRequest

	loadResp   acpproto.SessionId
	newResp    acpproto.SessionId
	promptResp *acpclient.PromptResult

	loadErr   error
	newErr    error
	promptErr error
	closeErr  error

	writeReqOnPrompt *acpproto.WriteTextFileRequest
}

func (c *stubACPClient) LoadSession(_ context.Context, req acpproto.LoadSessionRequest) (acpproto.SessionId, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.loadReqs = append(c.loadReqs, req)
	if c.loadErr != nil {
		return "", c.loadErr
	}
	if strings.TrimSpace(string(c.loadResp)) == "" {
		return acpproto.SessionId("sid-load-default"), nil
	}
	return c.loadResp, nil
}

func (c *stubACPClient) NewSession(_ context.Context, req acpproto.NewSessionRequest) (acpproto.SessionId, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.newReqs = append(c.newReqs, req)
	if c.newErr != nil {
		return "", c.newErr
	}
	if strings.TrimSpace(string(c.newResp)) == "" {
		return acpproto.SessionId("sid-new-default"), nil
	}
	return c.newResp, nil
}

func (c *stubACPClient) Prompt(ctx context.Context, req acpproto.PromptRequest) (*acpclient.PromptResult, error) {
	c.mu.Lock()
	c.promptReqs = append(c.promptReqs, req)
	writeReq := c.writeReqOnPrompt
	handler := c.handler
	promptErr := c.promptErr
	promptResp := c.promptResp
	c.mu.Unlock()

	if writeReq != nil && handler != nil {
		if _, err := handler.WriteTextFile(ctx, *writeReq); err != nil {
			return nil, err
		}
	}
	if promptErr != nil {
		return nil, promptErr
	}
	if promptResp != nil {
		return promptResp, nil
	}
	return &acpclient.PromptResult{Text: "ok"}, nil
}

func (c *stubACPClient) Close(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeErr
}

func (c *stubACPClient) Cancel(_ context.Context, _ acpproto.CancelNotification) error {
	return nil
}

type recordingEventPublisher struct {
	mu     sync.Mutex
	events []core.Event
}

func (p *recordingEventPublisher) Publish(_ context.Context, evt core.Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, evt)
	return nil
}

func (p *recordingEventPublisher) Events() []core.Event {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]core.Event, len(p.events))
	copy(out, p.events)
	return out
}
