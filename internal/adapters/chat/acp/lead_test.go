package acp

import (
	"context"
	"testing"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/zhanggui/internal/adapters/agent/acpclient"
	membus "github.com/yoke233/zhanggui/internal/adapters/events/memory"
	v2sandbox "github.com/yoke233/zhanggui/internal/adapters/sandbox"
	chatapp "github.com/yoke233/zhanggui/internal/application/chat"
	"github.com/yoke233/zhanggui/internal/core"
	"github.com/yoke233/zhanggui/internal/platform/config"
)

type fakeLeadRegistry struct {
	profile       *core.AgentProfile
	lastResolveID string
}

type fakeLeadDriverResolver struct {
	drivers      map[string]*core.DriverConfig
	lastDriverID string
}

type fakeLeadLLMResolver struct {
	configs      map[string]*config.RuntimeLLMEntryConfig
	lastConfigID string
}

func (f *fakeLeadDriverResolver) Resolve(_ context.Context, driverID string) (*core.DriverConfig, error) {
	f.lastDriverID = driverID
	driver, ok := f.drivers[driverID]
	if !ok {
		return nil, core.ErrProfileNotFound
	}
	cloned := *driver
	if driver.LaunchArgs != nil {
		cloned.LaunchArgs = append([]string(nil), driver.LaunchArgs...)
	}
	if driver.Env != nil {
		cloned.Env = make(map[string]string, len(driver.Env))
		for k, v := range driver.Env {
			cloned.Env[k] = v
		}
	}
	return &cloned, nil
}

func (f *fakeLeadLLMResolver) Resolve(_ context.Context, llmConfigID string) (*config.RuntimeLLMEntryConfig, error) {
	f.lastConfigID = llmConfigID
	item, ok := f.configs[llmConfigID]
	if !ok {
		return nil, core.ErrProfileNotFound
	}
	cloned := *item
	return &cloned, nil
}

func (f *fakeLeadRegistry) GetProfile(context.Context, string) (*core.AgentProfile, error) {
	return nil, core.ErrProfileNotFound
}
func (f *fakeLeadRegistry) ListProfiles(context.Context) ([]*core.AgentProfile, error) {
	return nil, nil
}
func (f *fakeLeadRegistry) CreateProfile(context.Context, *core.AgentProfile) error { return nil }
func (f *fakeLeadRegistry) UpdateProfile(context.Context, *core.AgentProfile) error { return nil }
func (f *fakeLeadRegistry) DeleteProfile(context.Context, string) error             { return nil }
func (f *fakeLeadRegistry) ResolveForAction(context.Context, *core.Action) (*core.AgentProfile, error) {
	return f.profile, nil
}
func (f *fakeLeadRegistry) ResolveByID(_ context.Context, id string) (*core.AgentProfile, error) {
	f.lastResolveID = id
	return f.profile, nil
}

type fakeChatACPClient struct {
	newSessionID  acpproto.SessionId
	loadSessionID acpproto.SessionId
	promptReply   string
	promptFn      func(context.Context, acpproto.PromptRequest) (*acpclient.PromptResult, error)

	initializeCalls int
	newCalls        int
	loadCalls       int
	promptCalls     int

	lastLoad acpproto.LoadSessionRequest
}

func (f *fakeChatACPClient) Initialize(context.Context, acpclient.ClientCapabilities) error {
	f.initializeCalls++
	return nil
}
func (f *fakeChatACPClient) NewSession(context.Context, acpproto.NewSessionRequest) (acpproto.SessionId, error) {
	f.newCalls++
	return f.newSessionID, nil
}
func (f *fakeChatACPClient) NewSessionResult(_ context.Context, req acpproto.NewSessionRequest) (acpclient.SessionResult, error) {
	f.newCalls++
	return acpclient.SessionResult{SessionID: f.newSessionID}, nil
}
func (f *fakeChatACPClient) LoadSession(_ context.Context, req acpproto.LoadSessionRequest) (acpproto.SessionId, error) {
	f.loadCalls++
	f.lastLoad = req
	return f.loadSessionID, nil
}
func (f *fakeChatACPClient) LoadSessionResult(_ context.Context, req acpproto.LoadSessionRequest) (acpclient.SessionResult, error) {
	f.loadCalls++
	f.lastLoad = req
	return acpclient.SessionResult{SessionID: f.loadSessionID}, nil
}
func (f *fakeChatACPClient) Prompt(ctx context.Context, req acpproto.PromptRequest) (*acpclient.PromptResult, error) {
	f.promptCalls++
	if f.promptFn != nil {
		return f.promptFn(ctx, req)
	}
	return &acpclient.PromptResult{Text: f.promptReply}, nil
}
func (f *fakeChatACPClient) SetConfigOption(context.Context, acpproto.SetSessionConfigOptionRequest) ([]acpproto.SessionConfigOptionSelect, error) {
	return nil, nil
}
func (f *fakeChatACPClient) SetSessionMode(context.Context, acpproto.SetSessionModeRequest) error {
	return nil
}
func (f *fakeChatACPClient) Cancel(context.Context, acpproto.CancelNotification) error { return nil }
func (f *fakeChatACPClient) Close(context.Context) error                               { return nil }

func TestLeadAgentRestoresPersistedSession(t *testing.T) {
	registry := &fakeLeadRegistry{
		profile: &core.AgentProfile{
			ID:   "lead",
			Name: "Codex Lead",
			Role: core.RoleLead,
			Driver: core.DriverConfig{
				LaunchCommand: "fake",
			},
		},
	}

	firstClient := &fakeChatACPClient{
		newSessionID: "acp-session-1",
		promptReply:  "first reply",
	}
	secondClient := &fakeChatACPClient{
		loadSessionID: "acp-session-1",
		promptReply:   "second reply",
	}
	clients := []ChatACPClient{firstClient, secondClient}
	newClient := func(_ acpclient.LaunchConfig, _ acpproto.Client, _opts ...acpclient.Option) (ChatACPClient, error) {
		client := clients[0]
		clients = clients[1:]
		return client, nil
	}

	cfg := LeadAgentConfig{
		Registry:  registry,
		Bus:       membus.NewBus(),
		Sandbox:   v2sandbox.NoopSandbox{},
		DataDir:   t.TempDir(),
		NewClient: newClient,
	}

	agent := NewLeadAgent(cfg)
	firstResp, err := agent.Chat(context.Background(), chatapp.Request{Message: "第一次"})
	if err != nil {
		t.Fatalf("first chat: %v", err)
	}
	if firstResp.SessionID != "acp-session-1" {
		t.Fatalf("first session_id = %q, want acp-session-1", firstResp.SessionID)
	}
	if firstResp.WSPath != "/api/ws?session_id=acp-session-1&types=chat.output" {
		t.Fatalf("first ws_path = %q", firstResp.WSPath)
	}
	agent.Shutdown()

	agent = NewLeadAgent(cfg)
	secondResp, err := agent.Chat(context.Background(), chatapp.Request{
		SessionID: "acp-session-1",
		Message:   "继续聊",
	})
	if err != nil {
		t.Fatalf("second chat: %v", err)
	}
	if secondResp.SessionID != "acp-session-1" {
		t.Fatalf("second session_id = %q, want acp-session-1", secondResp.SessionID)
	}
	if secondClient.loadCalls != 1 {
		t.Fatalf("load calls = %d, want 1", secondClient.loadCalls)
	}
	if string(secondClient.lastLoad.SessionId) != "acp-session-1" {
		t.Fatalf("loaded session = %q, want acp-session-1", secondClient.lastLoad.SessionId)
	}

	detail, err := agent.GetSession(context.Background(), "acp-session-1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if len(detail.Messages) != 4 {
		t.Fatalf("message count = %d, want 4", len(detail.Messages))
	}
	if detail.WSPath != "/api/ws?session_id=acp-session-1&types=chat.output" {
		t.Fatalf("detail ws_path = %q", detail.WSPath)
	}
	if detail.ProfileID != "lead" || detail.ProfileName != "Codex Lead" {
		t.Fatalf("unexpected profile info: %+v", detail.SessionSummary)
	}
	if detail.Messages[0].Role != "user" || detail.Messages[1].Role != "assistant" {
		t.Fatalf("unexpected history: %+v", detail.Messages)
	}
}

func TestLeadAgentPersistsProjectAndProfileSelection(t *testing.T) {
	registry := &fakeLeadRegistry{
		profile: &core.AgentProfile{
			ID:   "lead-alt",
			Name: "Claude Lead",
			Role: core.RoleLead,
			Driver: core.DriverConfig{
				LaunchCommand: "fake",
			},
		},
	}
	client := &fakeChatACPClient{
		newSessionID: "acp-session-2",
		promptReply:  "reply",
	}

	agent := NewLeadAgent(LeadAgentConfig{
		Registry: registry,
		Bus:      membus.NewBus(),
		Sandbox:  v2sandbox.NoopSandbox{},
		DataDir:  t.TempDir(),
		NewClient: func(_ acpclient.LaunchConfig, _ acpproto.Client, _opts ...acpclient.Option) (ChatACPClient, error) {
			return client, nil
		},
		ProfileID: "lead",
	})

	resp, err := agent.Chat(context.Background(), chatapp.Request{
		Message:     "hello",
		ProjectID:   42,
		ProjectName: "Alpha",
		ProfileID:   "lead-alt",
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if registry.lastResolveID != "lead-alt" {
		t.Fatalf("resolved profile = %q, want lead-alt", registry.lastResolveID)
	}

	detail, err := agent.GetSession(context.Background(), resp.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if detail.ProjectID != 42 || detail.ProjectName != "Alpha" {
		t.Fatalf("unexpected project info: %+v", detail.SessionSummary)
	}
	if detail.ProfileID != "lead-alt" || detail.ProfileName != "Claude Lead" {
		t.Fatalf("unexpected profile info: %+v", detail.SessionSummary)
	}
}

func TestLeadAgentStartChatUsesBackgroundContext(t *testing.T) {
	registry := &fakeLeadRegistry{
		profile: &core.AgentProfile{
			ID:   "lead",
			Name: "Codex Lead",
			Role: core.RoleLead,
			Driver: core.DriverConfig{
				LaunchCommand: "fake",
			},
		},
	}

	releasePrompt := make(chan struct{})
	client := &fakeChatACPClient{
		newSessionID: "acp-session-bg",
		promptFn: func(ctx context.Context, _ acpproto.PromptRequest) (*acpclient.PromptResult, error) {
			select {
			case <-releasePrompt:
				return &acpclient.PromptResult{Text: "async reply"}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	}

	bgCtx, bgCancel := context.WithCancel(context.Background())
	defer bgCancel()

	agent := NewLeadAgent(LeadAgentConfig{
		Registry: registry,
		Bus:      membus.NewBus(),
		Sandbox:  v2sandbox.NoopSandbox{},
		DataDir:  t.TempDir(),
		NewClient: func(_ acpclient.LaunchConfig, _ acpproto.Client, _opts ...acpclient.Option) (ChatACPClient, error) {
			return client, nil
		},
		BackgroundContext: bgCtx,
	})
	defer agent.Shutdown()

	reqCtx, reqCancel := context.WithCancel(context.Background())
	resp, err := agent.StartChat(reqCtx, chatapp.Request{Message: "后台发送"})
	if err != nil {
		t.Fatalf("StartChat: %v", err)
	}
	reqCancel()
	close(releasePrompt)

	deadline := time.Now().Add(2 * time.Second)
	for {
		detail, detailErr := agent.GetSession(context.Background(), resp.SessionID)
		if detailErr == nil && len(detail.Messages) >= 2 {
			if got := detail.Messages[1].Content; got != "async reply" {
				t.Fatalf("assistant message = %q, want async reply", got)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for async reply, last err=%v", detailErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestLeadAgentStartChatEmptyReplyKeepsSessionAlive(t *testing.T) {
	registry := &fakeLeadRegistry{
		profile: &core.AgentProfile{
			ID:   "lead",
			Name: "Codex Lead",
			Role: core.RoleLead,
			Driver: core.DriverConfig{
				LaunchCommand: "fake",
			},
		},
	}

	promptDone := make(chan struct{}, 1)
	client := &fakeChatACPClient{
		newSessionID: "acp-session-empty",
		promptFn: func(context.Context, acpproto.PromptRequest) (*acpclient.PromptResult, error) {
			promptDone <- struct{}{}
			return &acpclient.PromptResult{Text: ""}, nil
		},
	}

	agent := NewLeadAgent(LeadAgentConfig{
		Registry: registry,
		Bus:      membus.NewBus(),
		Sandbox:  v2sandbox.NoopSandbox{},
		DataDir:  t.TempDir(),
		NewClient: func(_ acpclient.LaunchConfig, _ acpproto.Client, _opts ...acpclient.Option) (ChatACPClient, error) {
			return client, nil
		},
	})
	defer agent.Shutdown()

	resp, err := agent.StartChat(context.Background(), chatapp.Request{Message: "继续执行"})
	if err != nil {
		t.Fatalf("StartChat: %v", err)
	}

	select {
	case <-promptDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for prompt completion")
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		detail, detailErr := agent.GetSession(context.Background(), resp.SessionID)
		if detailErr == nil {
			if detail.Status != "alive" {
				t.Fatalf("session status = %q, want alive", detail.Status)
			}
			if len(detail.Messages) != 1 || detail.Messages[0].Role != "user" {
				t.Fatalf("unexpected message history: %+v", detail.Messages)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for session detail, last err=%v", detailErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestLeadAgentUsesSelectedDriverForCreateAndReload(t *testing.T) {
	registry := &fakeLeadRegistry{
		profile: &core.AgentProfile{
			ID:          "lead-alt",
			Name:        "Claude Lead",
			DriverID:    "claude-acp",
			LLMConfigID: "openai-response-default",
			Role:        core.RoleLead,
			Driver: core.DriverConfig{
				LaunchCommand: "default-driver",
			},
		},
	}
	driverResolver := &fakeLeadDriverResolver{
		drivers: map[string]*core.DriverConfig{
			"codex-cli": {
				LaunchCommand: "codex",
				LaunchArgs:    []string{"chat"},
				CapabilitiesMax: core.DriverCapabilities{
					FSRead:   true,
					FSWrite:  true,
					Terminal: true,
				},
			},
		},
	}
	llmResolver := &fakeLeadLLMResolver{
		configs: map[string]*config.RuntimeLLMEntryConfig{
			"openai-response-default": {
				ID:      "openai-response-default",
				Type:    "openai_response",
				BaseURL: "https://api.openai.com/v1",
				APIKey:  "test-key",
				Model:   "gpt-4.1-mini",
			},
		},
	}

	firstClient := &fakeChatACPClient{
		newSessionID: "acp-driver-1",
		promptReply:  "first reply",
	}
	secondClient := &fakeChatACPClient{
		loadSessionID: "acp-driver-1",
		promptReply:   "second reply",
	}
	clients := []ChatACPClient{firstClient, secondClient}
	launches := make([]acpclient.LaunchConfig, 0, len(clients))
	newClient := func(cfg acpclient.LaunchConfig, _ acpproto.Client, _opts ...acpclient.Option) (ChatACPClient, error) {
		launches = append(launches, cfg)
		client := clients[0]
		clients = clients[1:]
		return client, nil
	}

	cfg := LeadAgentConfig{
		Registry:          registry,
		DriverResolver:    driverResolver.Resolve,
		LLMConfigResolver: llmResolver.Resolve,
		Bus:               membus.NewBus(),
		Sandbox:           v2sandbox.NoopSandbox{},
		DataDir:           t.TempDir(),
		NewClient:         newClient,
	}

	agent := NewLeadAgent(cfg)
	resp, err := agent.Chat(context.Background(), chatapp.Request{
		Message:   "hello",
		ProfileID: "lead-alt",
		DriverID:  "codex-cli",
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if registry.lastResolveID != "lead-alt" {
		t.Fatalf("resolved profile = %q, want lead-alt", registry.lastResolveID)
	}
	if driverResolver.lastDriverID != "codex-cli" {
		t.Fatalf("resolved driver = %q, want codex-cli", driverResolver.lastDriverID)
	}
	if llmResolver.lastConfigID != "openai-response-default" {
		t.Fatalf("resolved llm config = %q, want openai-response-default", llmResolver.lastConfigID)
	}
	if len(launches) != 1 || launches[0].Command != "codex" {
		t.Fatalf("launch command = %+v, want codex", launches)
	}
	if launches[0].Env["AGENTSDK_PROVIDER"] != "openai_response" || launches[0].Env["OPENAI_API_KEY"] != "test-key" {
		t.Fatalf("launch env = %#v, want llm env injected", launches[0].Env)
	}

	detail, err := agent.GetSession(context.Background(), resp.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if detail.DriverID != "codex-cli" {
		t.Fatalf("driver_id = %q, want codex-cli", detail.DriverID)
	}
	agent.Shutdown()

	reloaded := NewLeadAgent(cfg)
	_, err = reloaded.Chat(context.Background(), chatapp.Request{
		SessionID: resp.SessionID,
		Message:   "continue",
	})
	if err != nil {
		t.Fatalf("reload chat: %v", err)
	}
	if len(launches) != 2 || launches[1].Command != "codex" {
		t.Fatalf("reload launch command = %+v, want codex", launches)
	}
	if secondClient.loadCalls != 1 {
		t.Fatalf("load calls = %d, want 1", secondClient.loadCalls)
	}
}

func TestLeadAgentPersistsSessionStateMetadata(t *testing.T) {
	registry := &fakeLeadRegistry{
		profile: &core.AgentProfile{
			ID:   "lead",
			Name: "Claude Lead",
			Role: core.RoleLead,
			Driver: core.DriverConfig{
				LaunchCommand: "fake",
			},
		},
	}
	client := &fakeChatACPClient{
		newSessionID: "acp-session-3",
		promptReply:  "reply",
	}

	agent := NewLeadAgent(LeadAgentConfig{
		Registry: registry,
		Bus:      membus.NewBus(),
		Sandbox:  v2sandbox.NoopSandbox{},
		DataDir:  t.TempDir(),
		NewClient: func(_ acpclient.LaunchConfig, _ acpproto.Client, _opts ...acpclient.Option) (ChatACPClient, error) {
			return client, nil
		},
	})

	resp, err := agent.Chat(context.Background(), chatapp.Request{Message: "hello"})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}

	agent.captureSessionState(resp.SessionID, acpclient.SessionUpdate{
		Type: "available_commands_update",
		Commands: []acpproto.AvailableCommand{
			{
				Name:        "review",
				Description: "Review current changes",
				Input: &acpproto.AvailableCommandInput{
					Unstructured: &acpproto.UnstructuredCommandInput{Hint: "optional instructions"},
				},
			},
		},
	})
	agent.captureSessionState(resp.SessionID, acpclient.SessionUpdate{
		Type: "config_option_update",
		ConfigOptions: []acpproto.SessionConfigOptionSelect{
			{
				Type:         "select",
				Id:           acpproto.SessionConfigId("model"),
				Name:         "Model",
				CurrentValue: acpproto.SessionConfigValueId("model-1"),
				Options: acpproto.SessionConfigSelectOptions{
					Ungrouped: &acpproto.SessionConfigSelectOptionsUngrouped{
						{
							Value: acpproto.SessionConfigValueId("model-1"),
							Name:  "Model 1",
						},
						{
							Value: acpproto.SessionConfigValueId("model-2"),
							Name:  "Model 2",
						},
					},
				},
			},
		},
	})

	detail, err := agent.GetSession(context.Background(), resp.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if len(detail.AvailableCommands) != 1 || detail.AvailableCommands[0].Name != "review" {
		t.Fatalf("unexpected available commands: %+v", detail.AvailableCommands)
	}
	if detail.AvailableCommands[0].Input == nil || detail.AvailableCommands[0].Input.Hint != "optional instructions" {
		t.Fatalf("unexpected command input: %+v", detail.AvailableCommands[0].Input)
	}
	if len(detail.ConfigOptions) != 1 {
		t.Fatalf("unexpected config options: %+v", detail.ConfigOptions)
	}
	if detail.ConfigOptions[0].ID != "model" || detail.ConfigOptions[0].CurrentValue != "model-1" {
		t.Fatalf("unexpected config option payload: %+v", detail.ConfigOptions[0])
	}
	if len(detail.ConfigOptions[0].Options) != 2 {
		t.Fatalf("unexpected config option values: %+v", detail.ConfigOptions[0].Options)
	}

	agent.Shutdown()
	reloaded := NewLeadAgent(LeadAgentConfig{
		Registry: registry,
		Bus:      membus.NewBus(),
		Sandbox:  v2sandbox.NoopSandbox{},
		DataDir:  agent.cfg.DataDir,
		NewClient: func(_ acpclient.LaunchConfig, _ acpproto.Client, _opts ...acpclient.Option) (ChatACPClient, error) {
			return client, nil
		},
	})
	reloadedDetail, err := reloaded.GetSession(context.Background(), resp.SessionID)
	if err != nil {
		t.Fatalf("get reloaded session: %v", err)
	}
	if len(reloadedDetail.AvailableCommands) != 1 || len(reloadedDetail.ConfigOptions) != 1 {
		t.Fatalf("expected persisted session state, got commands=%+v config=%+v", reloadedDetail.AvailableCommands, reloadedDetail.ConfigOptions)
	}
}
