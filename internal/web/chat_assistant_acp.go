package web

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/acpclient"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/teamleader"
)

const (
	defaultWebChatRoleID   = "team_leader"
	defaultWebChatTimeout  = 90 * time.Second
	chatSessionIdleTimeout = 30 * time.Minute
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
	SupportsSSEMCP() bool
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

// ACPChatAssistant runs multi-turn chat on ACP protocol with long-lived session pooling.
type ACPChatAssistant struct {
	deps ACPChatAssistantDeps

	activeMu   sync.Mutex
	activeRuns map[string]*activeChatRun

	poolMu      sync.Mutex
	sessionPool map[string]*pooledChatSession
}

type activeChatRun struct {
	client         ChatACPClient
	agentSessionID string
	cancel         context.CancelFunc
}

// pooledChatSession keeps an ACP client alive between chat turns.
type pooledChatSession struct {
	client    ChatACPClient
	sessionID acpproto.SessionId
	roleID    string
	workDir   string
	handler   *teamleader.ACPHandler

	mu        sync.Mutex
	idleTimer *time.Timer
	closed    bool
}

func (p *pooledChatSession) stopIdleTimer() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.idleTimer != nil {
		p.idleTimer.Stop()
		p.idleTimer = nil
	}
}

func (p *pooledChatSession) resetIdleTimer(timeout time.Duration, onExpire func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	if p.idleTimer != nil {
		p.idleTimer.Stop()
	}
	p.idleTimer = time.AfterFunc(timeout, onExpire)
}

func (p *pooledChatSession) isClosed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.closed
}

func (p *pooledChatSession) close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	if p.idleTimer != nil {
		p.idleTimer.Stop()
		p.idleTimer = nil
	}
	client := p.client
	p.mu.Unlock()
	closeACPClient(client)
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
		deps:        deps,
		activeRuns:  make(map[string]*activeChatRun),
		sessionPool: make(map[string]*pooledChatSession),
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

	chatSessionID := strings.TrimSpace(req.ChatSessionID)

	// Try reusing a pooled ACP session.
	pooled := a.getPooledSession(chatSessionID)
	if pooled != nil {
		pooled.stopIdleTimer()
	}

	if pooled == nil {
		// Create a new ACP client and session.
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

		handler := teamleader.NewACPHandler(launchCfg.WorkDir, strings.TrimSpace(req.AgentSessionID), a.deps.EventPublisher)
		handler.SetProjectID(strings.TrimSpace(req.ProjectID))
		handler.SetChatSessionID(chatSessionID)
		handler.SetPermissionPolicy(role.PermissionPolicy)
		handler.SetRunEventRecorder(a.deps.RunEventRecorder)

		createCtx, createCancel := context.WithTimeout(context.Background(), a.deps.Timeout)
		defer createCancel()

		client, err := a.deps.ClientFactory.New(createCtx, launchCfg, handler, role.Capabilities)
		if err != nil {
			return ChatAssistantResponse{}, fmt.Errorf("create acp client: %w", err)
		}

		session, err := startWebChatSession(
			createCtx, client, roleID, role,
			strings.TrimSpace(req.AgentSessionID),
			launchCfg.WorkDir, a.deps.MCPEnv, handler,
		)
		if err != nil {
			closeACPClient(client)
			return ChatAssistantResponse{}, err
		}
		handler.SetSessionID(string(session))

		pooled = &pooledChatSession{
			client:    client,
			sessionID: session,
			roleID:    roleID,
			workDir:   launchCfg.WorkDir,
			handler:   handler,
		}
	}

	// Register active run for cancel support.
	promptCtx, promptCancel := withDefaultTimeout(ctx, a.deps.Timeout)
	defer promptCancel()

	agentSessionID := strings.TrimSpace(string(pooled.sessionID))
	if agentSessionID == "" {
		agentSessionID = strings.TrimSpace(req.AgentSessionID)
	}
	if chatSessionID != "" {
		a.setActiveRun(chatSessionID, &activeChatRun{
			client:         pooled.client,
			agentSessionID: agentSessionID,
			cancel:         promptCancel,
		})
		defer a.clearActiveRun(chatSessionID)
	}

	result, err := pooled.client.Prompt(promptCtx, acpproto.PromptRequest{
		SessionId: pooled.sessionID,
		Prompt: []acpproto.ContentBlock{
			{Text: &acpproto.ContentBlockText{Text: message}},
		},
		Meta: map[string]any{
			"role_id": pooled.roleID,
		},
	})
	if pooled.handler != nil {
		if flushErr := pooled.handler.FlushPendingChatRunEvents(); flushErr != nil {
			log.Printf("[chat] flush pending acp chunk events failed session_id=%s err=%v", chatSessionID, flushErr)
		}
	}
	if err != nil {
		if chatSessionID != "" {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				// Prompt was cancelled/timed out but ACP process may be fine — keep pooled.
				a.poolSession(chatSessionID, pooled)
			} else {
				a.removePooledSession(chatSessionID)
			}
		} else {
			pooled.close()
		}
		return ChatAssistantResponse{}, fmt.Errorf("acp prompt failed: %w", err)
	}
	if result == nil {
		if chatSessionID != "" {
			a.removePooledSession(chatSessionID)
		} else {
			pooled.close()
		}
		return ChatAssistantResponse{}, errors.New("acp prompt returned empty result")
	}

	reply := strings.TrimSpace(result.Text)
	if reply == "" {
		if chatSessionID != "" {
			a.removePooledSession(chatSessionID)
		} else {
			pooled.close()
		}
		return ChatAssistantResponse{}, errors.New("chat assistant returned empty reply")
	}

	// Success — pool the session for reuse.
	if chatSessionID != "" {
		a.poolSession(chatSessionID, pooled)
	} else {
		pooled.close()
	}

	sessionID := agentSessionID
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
	handler *teamleader.ACPHandler,
) (acpproto.SessionId, error) {
	if client == nil {
		return "", errors.New("chat acp client is required")
	}

	metadata := map[string]any{
		"role_id": roleID,
	}
	trimmedCWD := strings.TrimSpace(cwd)
	effectiveMCPServers := teamleader.MCPToolsFromRoleConfig(role, mcpEnv, client.SupportsSSEMCP())
	if sessionID := strings.TrimSpace(persistedSessionID); shouldLoadPersistedChatSession(role.SessionPolicy, sessionID) {
		// Suppress event publishing during LoadSession to avoid replaying
		// historical events (thoughts, messages, tool calls) to the frontend.
		if handler != nil {
			handler.SetSuppressEvents(true)
		}
		loaded, err := client.LoadSession(ctx, acpproto.LoadSessionRequest{
			SessionId:  acpproto.SessionId(sessionID),
			Cwd:        trimmedCWD,
			McpServers: effectiveMCPServers,
			Meta:       metadata,
		})
		if handler != nil {
			handler.SetSuppressEvents(false)
		}
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

// --- session pool management ---

func (a *ACPChatAssistant) getPooledSession(chatSessionID string) *pooledChatSession {
	key := strings.TrimSpace(chatSessionID)
	if key == "" {
		return nil
	}
	a.poolMu.Lock()
	defer a.poolMu.Unlock()
	ps := a.sessionPool[key]
	if ps != nil && ps.isClosed() {
		delete(a.sessionPool, key)
		return nil
	}
	return ps
}

func (a *ACPChatAssistant) setPooledSession(chatSessionID string, ps *pooledChatSession) {
	key := strings.TrimSpace(chatSessionID)
	if key == "" || ps == nil {
		return
	}
	a.poolMu.Lock()
	defer a.poolMu.Unlock()
	if old, exists := a.sessionPool[key]; exists && old != ps {
		old.close()
	}
	a.sessionPool[key] = ps
}

func (a *ACPChatAssistant) removePooledSession(chatSessionID string) {
	key := strings.TrimSpace(chatSessionID)
	if key == "" {
		return
	}
	a.poolMu.Lock()
	ps, exists := a.sessionPool[key]
	if exists {
		delete(a.sessionPool, key)
	}
	a.poolMu.Unlock()
	if ps != nil {
		ps.close()
	}
}

func (a *ACPChatAssistant) poolSession(chatSessionID string, ps *pooledChatSession) {
	a.setPooledSession(chatSessionID, ps)
	ps.resetIdleTimer(chatSessionIdleTimeout, func() {
		a.removePooledSession(chatSessionID)
	})
}

// IsChatSessionAlive reports whether a pooled ACP session exists for the given chat session.
func (a *ACPChatAssistant) IsChatSessionAlive(chatSessionID string) bool {
	ps := a.getPooledSession(chatSessionID)
	return ps != nil
}

// IsChatSessionRunning reports whether the chat session is currently processing a prompt.
func (a *ACPChatAssistant) IsChatSessionRunning(chatSessionID string) bool {
	return a.getActiveRun(chatSessionID) != nil
}

// ShutdownSessions closes all pooled ACP sessions.
func (a *ACPChatAssistant) ShutdownSessions() {
	if a == nil {
		return
	}
	a.poolMu.Lock()
	sessions := make([]*pooledChatSession, 0, len(a.sessionPool))
	for key, ps := range a.sessionPool {
		sessions = append(sessions, ps)
		delete(a.sessionPool, key)
	}
	a.poolMu.Unlock()
	for _, ps := range sessions {
		ps.close()
	}
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
