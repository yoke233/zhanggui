package web

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/acpclient"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/teamleader"
)

const (
	defaultWebChatRoleID  = "team_leader"
	defaultWebChatTimeout = 90 * time.Second
)

// ChatRoleResolver resolves a role id into ACP launch metadata.
type ChatRoleResolver interface {
	Resolve(roleID string) (acpclient.AgentProfile, acpclient.RoleProfile, error)
}

// ChatACPClient is the minimal ACP session API used by chat assistant.
type ChatACPClient interface {
	LoadSession(ctx context.Context, req acpproto.LoadSessionRequest) (acpproto.SessionId, error)
	NewSession(ctx context.Context, req acpproto.NewSessionRequest) (acpproto.SessionId, error)
	Prompt(ctx context.Context, req acpproto.PromptRequest) (*acpclient.PromptResult, error)
	Cancel(ctx context.Context, req acpproto.CancelNotification) error
	Close(ctx context.Context) error
}

// ChatACPClientFactory creates initialized ACP clients for one chat request.
type ChatACPClientFactory interface {
	New(ctx context.Context, cfg acpclient.LaunchConfig, handler acpproto.Client, capabilities acpclient.ClientCapabilities) (ChatACPClient, error)
}

// ChatEventPublisher receives assistant callback events (e.g. file writes).
type ChatEventPublisher interface {
	Publish(ctx context.Context, evt core.Event) error
}

// ACPChatAssistantDeps contains injectable dependencies for ACP chat assistant.
type ACPChatAssistantDeps struct {
	DefaultRoleID    string
	Timeout          time.Duration
	RoleResolver     ChatRoleResolver
	ClientFactory    ChatACPClientFactory
	EventPublisher   ChatEventPublisher
	RunEventRecorder teamleader.ChatRunEventRecorder
	MCPEnv           teamleader.MCPEnvConfig
}

// ACPChatAssistant runs one-turn chat on ACP protocol.
type ACPChatAssistant struct {
	deps ACPChatAssistantDeps

	activeMu   sync.Mutex
	activeRuns map[string]*activeChatRun
}

type activeChatRun struct {
	client         ChatACPClient
	agentSessionID string
	cancel         context.CancelFunc
}

// NewACPChatAssistantWithDeps builds a ChatAssistant backed by ACP protocol with injectable dependencies.
func NewACPChatAssistantWithDeps(deps ACPChatAssistantDeps) ChatAssistant {
	return newACPChatAssistant(deps)
}

func newACPChatAssistant(deps ACPChatAssistantDeps) *ACPChatAssistant {
	if strings.TrimSpace(deps.DefaultRoleID) == "" {
		deps.DefaultRoleID = defaultWebChatRoleID
	}
	if deps.Timeout <= 0 {
		deps.Timeout = defaultWebChatTimeout
	}
	if deps.ClientFactory == nil {
		deps.ClientFactory = defaultACPClientFactory{}
	}
	return &ACPChatAssistant{
		deps:       deps,
		activeRuns: make(map[string]*activeChatRun),
	}
}

func (a *ACPChatAssistant) Reply(ctx context.Context, req ChatAssistantRequest) (ChatAssistantResponse, error) {
	if a == nil {
		return ChatAssistantResponse{}, errors.New("chat assistant is nil")
	}
	message := strings.TrimSpace(req.Message)
	if message == "" {
		return ChatAssistantResponse{}, errors.New("message is required")
	}

	roleResolver := a.deps.RoleResolver
	if roleResolver == nil {
		return ChatAssistantResponse{}, errors.New("chat assistant role resolver is not configured")
	}
	roleID := strings.TrimSpace(req.Role)
	if roleID == "" {
		roleID = strings.TrimSpace(a.deps.DefaultRoleID)
	}
	if roleID == "" {
		return ChatAssistantResponse{}, errors.New("chat role is required")
	}

	agent, role, err := roleResolver.Resolve(roleID)
	if err != nil {
		return ChatAssistantResponse{}, fmt.Errorf("resolve chat role %q: %w", roleID, err)
	}

	launchCfg := acpclient.LaunchConfig{
		Command: strings.TrimSpace(agent.LaunchCommand),
		Args:    cloneStrings(agent.LaunchArgs),
		WorkDir: strings.TrimSpace(req.WorkDir),
		Env:     cloneStringMap(agent.Env),
	}
	if launchCfg.Command == "" {
		return ChatAssistantResponse{}, fmt.Errorf("chat role %q resolved empty launch command", roleID)
	}
	if isAgentSDKInprocLaunch(launchCfg.Command) && launchCfg.WorkDir == "" {
		return ChatAssistantResponse{}, errors.New("workdir is required for agentsdk-inproc")
	}

	runCtx, cancel := withDefaultTimeout(ctx, a.deps.Timeout)
	defer cancel()

	handler := teamleader.NewACPHandler(launchCfg.WorkDir, strings.TrimSpace(req.AgentSessionID), a.deps.EventPublisher)
	handler.SetProjectID(strings.TrimSpace(req.ProjectID))
	handler.SetChatSessionID(strings.TrimSpace(req.ChatSessionID))
	handler.SetPermissionPolicy(role.PermissionPolicy)
	handler.SetRunEventRecorder(a.deps.RunEventRecorder)
	client, err := a.deps.ClientFactory.New(runCtx, launchCfg, handler, role.Capabilities)
	if err != nil {
		return ChatAssistantResponse{}, fmt.Errorf("create acp client: %w", err)
	}
	defer closeACPClient(client)

	session, err := startWebChatSession(
		runCtx,
		client,
		roleID,
		role,
		strings.TrimSpace(req.AgentSessionID),
		launchCfg.WorkDir,
		a.deps.MCPEnv,
	)
	if err != nil {
		return ChatAssistantResponse{}, err
	}
	handler.SetSessionID(string(session))
	if chatSessionID := strings.TrimSpace(req.ChatSessionID); chatSessionID != "" {
		agentSessionID := strings.TrimSpace(string(session))
		if agentSessionID == "" {
			agentSessionID = strings.TrimSpace(req.AgentSessionID)
		}
		a.setActiveRun(chatSessionID, &activeChatRun{
			client:         client,
			agentSessionID: agentSessionID,
			cancel:         cancel,
		})
		defer a.clearActiveRun(chatSessionID)
	}

	result, err := client.Prompt(runCtx, acpproto.PromptRequest{
		SessionId: session,
		Prompt: []acpproto.ContentBlock{
			{Text: &acpproto.ContentBlockText{Text: message}},
		},
		Meta: map[string]any{
			"role_id": roleID,
		},
	})
	if err != nil {
		return ChatAssistantResponse{}, fmt.Errorf("acp prompt failed: %w", err)
	}
	if result == nil {
		return ChatAssistantResponse{}, errors.New("acp prompt returned empty result")
	}

	reply := strings.TrimSpace(result.Text)
	if reply == "" {
		return ChatAssistantResponse{}, errors.New("chat assistant returned empty reply")
	}

	sessionID := strings.TrimSpace(string(session))
	if sessionID == "" {
		sessionID = strings.TrimSpace(req.AgentSessionID)
	}

	return ChatAssistantResponse{
		Reply:          reply,
		AgentSessionID: sessionID,
	}, nil
}

func (a *ACPChatAssistant) CancelChat(chatSessionID string) error {
	if a == nil {
		return errors.New("chat assistant is nil")
	}
	trimmedSessionID := strings.TrimSpace(chatSessionID)
	if trimmedSessionID == "" {
		return errors.New("chat session id is required")
	}

	run := a.getActiveRun(trimmedSessionID)
	if run == nil {
		return errors.New("chat session is not running")
	}

	if run.cancel != nil {
		run.cancel()
	}
	agentSessionID := strings.TrimSpace(run.agentSessionID)
	if run.client == nil || agentSessionID == "" {
		return nil
	}

	cancelCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return run.client.Cancel(cancelCtx, acpproto.CancelNotification{
		SessionId: acpproto.SessionId(agentSessionID),
	})
}

func startWebChatSession(
	ctx context.Context,
	client ChatACPClient,
	roleID string,
	role acpclient.RoleProfile,
	persistedSessionID string,
	cwd string,
	mcpEnv teamleader.MCPEnvConfig,
) (acpproto.SessionId, error) {
	if client == nil {
		return "", errors.New("chat acp client is required")
	}

	metadata := map[string]any{
		"role_id": roleID,
	}
	trimmedCWD := strings.TrimSpace(cwd)
	effectiveMCPServers := teamleader.MCPToolsFromRoleConfig(role, mcpEnv)
	if sessionID := strings.TrimSpace(persistedSessionID); shouldLoadPersistedChatSession(role.SessionPolicy, sessionID) {
		loaded, err := client.LoadSession(ctx, acpproto.LoadSessionRequest{
			SessionId:  acpproto.SessionId(sessionID),
			Cwd:        trimmedCWD,
			McpServers: effectiveMCPServers,
			Meta:       metadata,
		})
		if err == nil && strings.TrimSpace(string(loaded)) != "" {
			return loaded, nil
		}
	}

	session, err := client.NewSession(ctx, acpproto.NewSessionRequest{
		Cwd:        trimmedCWD,
		McpServers: effectiveMCPServers,
		Meta:       metadata,
	})
	if err != nil {
		return "", fmt.Errorf("create chat session: %w", err)
	}
	return session, nil
}

func shouldLoadPersistedChatSession(policy acpclient.SessionPolicy, persistedSessionID string) bool {
	if strings.TrimSpace(persistedSessionID) == "" {
		return false
	}
	if !policy.Reuse {
		return false
	}
	if !policy.PreferLoadSession {
		return false
	}
	return true
}

type defaultACPClientFactory struct{}

func (f defaultACPClientFactory) New(
	ctx context.Context,
	cfg acpclient.LaunchConfig,
	handler acpproto.Client,
	capabilities acpclient.ClientCapabilities,
) (ChatACPClient, error) {
	opts := make([]acpclient.Option, 0, 1)
	if eventHandler, ok := handler.(acpclient.EventHandler); ok {
		opts = append(opts, acpclient.WithEventHandler(eventHandler))
	}
	if isAgentSDKInprocLaunch(cfg.Command) {
		return newAgentSDKInprocClient(ctx, cfg, handler, capabilities, opts...)
	}
	client, err := acpclient.New(cfg, handler, opts...)
	if err != nil {
		return nil, err
	}
	if err := client.Initialize(ctx, capabilities); err != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = client.Close(closeCtx)
		return nil, err
	}
	return client, nil
}

func closeACPClient(client ChatACPClient) {
	if client == nil {
		return
	}
	closeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = client.Close(closeCtx)
}

func withDefaultTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return context.WithTimeout(context.Background(), timeout)
	}
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

func (a *ACPChatAssistant) setActiveRun(chatSessionID string, run *activeChatRun) {
	if a == nil || strings.TrimSpace(chatSessionID) == "" {
		return
	}
	a.activeMu.Lock()
	defer a.activeMu.Unlock()
	a.activeRuns[strings.TrimSpace(chatSessionID)] = run
}

func (a *ACPChatAssistant) getActiveRun(chatSessionID string) *activeChatRun {
	if a == nil || strings.TrimSpace(chatSessionID) == "" {
		return nil
	}
	a.activeMu.Lock()
	defer a.activeMu.Unlock()
	return a.activeRuns[strings.TrimSpace(chatSessionID)]
}

func (a *ACPChatAssistant) clearActiveRun(chatSessionID string) {
	if a == nil || strings.TrimSpace(chatSessionID) == "" {
		return
	}
	a.activeMu.Lock()
	defer a.activeMu.Unlock()
	delete(a.activeRuns, strings.TrimSpace(chatSessionID))
}

func newLegacyProviderRoleResolver(
	agentID string,
	launchCommand string,
	launchArgs []string,
	launchEnv map[string]string,
) *acpclient.RoleResolver {
	agentID = strings.TrimSpace(agentID)
	agent := acpclient.AgentProfile{
		ID:            agentID,
		LaunchCommand: strings.TrimSpace(launchCommand),
		LaunchArgs:    cloneStrings(launchArgs),
		Env:           cloneStringMap(launchEnv),
		CapabilitiesMax: acpclient.ClientCapabilities{
			FSRead:   true,
			FSWrite:  true,
			Terminal: true,
		},
	}
	roles := []acpclient.RoleProfile{
		{
			ID:      "team_leader",
			AgentID: agentID,
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
		{
			ID:      "worker",
			AgentID: agentID,
			Capabilities: acpclient.ClientCapabilities{
				FSRead:   true,
				FSWrite:  true,
				Terminal: true,
			},
			SessionPolicy: acpclient.SessionPolicy{
				Reuse: true,
			},
		},
		{
			ID:      "reviewer",
			AgentID: agentID,
			Capabilities: acpclient.ClientCapabilities{
				FSRead:   true,
				FSWrite:  false,
				Terminal: false,
			},
			SessionPolicy: acpclient.SessionPolicy{
				Reuse:       true,
				ResetPrompt: true,
			},
		},
		{
			ID:      "aggregator",
			AgentID: agentID,
			Capabilities: acpclient.ClientCapabilities{
				FSRead:   true,
				FSWrite:  false,
				Terminal: false,
			},
			SessionPolicy: acpclient.SessionPolicy{
				Reuse:       true,
				ResetPrompt: true,
			},
		},
		{
			ID:      "plan_parser",
			AgentID: agentID,
			Capabilities: acpclient.ClientCapabilities{
				FSRead:   true,
				FSWrite:  false,
				Terminal: false,
			},
			SessionPolicy: acpclient.SessionPolicy{
				Reuse: true,
			},
		},
	}
	return acpclient.NewRoleResolver([]acpclient.AgentProfile{agent}, roles)
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	out := make(map[string]string, len(values))
	for k, v := range values {
		out[k] = v
	}
	return out
}
