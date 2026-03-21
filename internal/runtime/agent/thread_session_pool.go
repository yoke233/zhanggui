package agentruntime

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	acphandler "github.com/yoke233/zhanggui/internal/adapters/agent/acp"
	"github.com/yoke233/zhanggui/internal/adapters/agent/acpclient"
	eventbridge "github.com/yoke233/zhanggui/internal/adapters/events/bridge"
	"github.com/yoke233/zhanggui/internal/core"
	"github.com/yoke233/zhanggui/internal/threadctx"
)

type threadSessionKey struct {
	threadID int64
	agentID  string // AgentProfile.ID
}

type threadPooledSession struct {
	client    *acpclient.Client
	sessionID acpproto.SessionId
	events    *switchingEventHandler
	bridge    *eventbridge.EventBridge

	// mu serializes ACP Prompt calls for this session. Only one goroutine may
	// hold it at a time. RemoveAgent removes the session from the pool map
	// before acquiring mu, so SendMessage and RemoveAgent cannot deadlock.
	mu           sync.Mutex
	closing      bool
	lastUsed     time.Time
	turns        int
	inputTokens  int64
	outputTokens int64
}

// TokenGenerator creates scoped tokens for agent signal APIs.
type TokenGenerator interface {
	GenerateScopedToken(role string, scopes []string, submitter string) (string, error)
}

// ThreadSessionPool manages ACP sessions for agents participating in Threads.
// Unlike ACPSessionPool (which is tied to Issue lifecycle), ThreadSessionPool
// is driven by user invite/remove actions and has no Step/Execution concept.
type ThreadSessionPool struct {
	store                    core.Store
	bus                      core.EventBus
	registry                 core.AgentRegistry
	dataDir                  string
	threadSharedBootTemplate string

	// Signal config: injected into agent launch env so skills like task-signal can call back.
	serverAddr    string
	tokenRegistry TokenGenerator

	mu       sync.Mutex
	sessions map[threadSessionKey]*threadPooledSession

	// bootstrapFn overrides acpclient.Bootstrap for testing.
	// When nil, acpclient.Bootstrap is used.
	bootstrapFn func(context.Context, acpclient.BootstrapConfig) (*acpclient.BootstrapResult, error)
}

const (
	threadBootTimeout       = 2 * time.Minute
	threadBootPromptTimeout = 120 * time.Second
)

// NewThreadSessionPool creates a pool for managing Thread agent ACP sessions.
func NewThreadSessionPool(store core.Store, bus core.EventBus, registry core.AgentRegistry, dataDir string) *ThreadSessionPool {
	return &ThreadSessionPool{
		store:    store,
		bus:      bus,
		registry: registry,
		dataDir:  strings.TrimSpace(dataDir),
		sessions: make(map[threadSessionKey]*threadPooledSession),
	}
}

// SetSignalConfig configures the server address and token generator for agent signal injection.
func (p *ThreadSessionPool) SetSignalConfig(serverAddr string, tokenRegistry TokenGenerator) {
	if p == nil {
		return
	}
	p.serverAddr = strings.TrimSpace(serverAddr)
	p.tokenRegistry = tokenRegistry
}

// SetThreadSharedBootTemplate configures a global boot prompt prepended for every
// agent joining a thread, regardless of profile.
func (p *ThreadSessionPool) SetThreadSharedBootTemplate(template string) {
	if p == nil {
		return
	}
	p.threadSharedBootTemplate = strings.TrimSpace(template)
}

func updateThreadAgentStatus(m *core.ThreadMember, next core.ThreadAgentStatus) error {
	if m == nil {
		return fmt.Errorf("thread member is nil")
	}
	if !m.Status.Valid() {
		return fmt.Errorf("invalid current thread agent status %q", m.Status)
	}
	if !core.CanTransitionThreadAgentStatus(m.Status, next) {
		return fmt.Errorf("invalid thread agent status transition %q -> %q", m.Status, next)
	}
	m.Status = next
	return nil
}

// Helper functions to read/write AgentData fields on ThreadMember.

func memberGetString(m *core.ThreadMember, key string) string {
	if m.AgentData == nil {
		return ""
	}
	v, _ := m.AgentData[key].(string)
	return v
}

func memberGetInt(m *core.ThreadMember, key string) int {
	if m.AgentData == nil {
		return 0
	}
	switch v := m.AgentData[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	}
	return 0
}

func memberGetInt64(m *core.ThreadMember, key string) int64 {
	if m.AgentData == nil {
		return 0
	}
	switch v := m.AgentData[key].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	}
	return 0
}

func memberSetAgentData(m *core.ThreadMember, key string, val any) {
	if m.AgentData == nil {
		m.AgentData = map[string]any{}
	}
	m.AgentData[key] = val
}

// InviteAgent registers an agent for the given thread and kicks off the boot
// sequence in the background. It returns the ThreadMember immediately (status
// = booting). Callers learn about completion / failure via EventBus events
// (EventThreadAgentBooted, EventThreadAgentJoined, EventThreadAgentFailed).
func (p *ThreadSessionPool) InviteAgent(ctx context.Context, threadID int64, profileID string) (*core.ThreadMember, error) {
	if p == nil {
		return nil, fmt.Errorf("thread session pool is nil")
	}
	key := threadSessionKey{threadID: threadID, agentID: profileID}

	// Resolve profile.
	profile, err := p.registry.ResolveByID(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("resolve profile %q: %w", profileID, err)
	}

	// Check for existing paused agent member (for resume with prior summary).
	var priorSummary string
	var priorSessionID string
	existingMembers, _ := p.store.ListThreadMembers(ctx, threadID)
	for _, m := range existingMembers {
		if m.Kind != "agent" || m.AgentProfileID != profileID {
			continue
		}
		if m.Status == core.ThreadAgentPaused {
			priorSummary = memberGetString(m, "progress_summary")
			priorSessionID = memberGetString(m, "acp_session_id")
			if err := updateThreadAgentStatus(m, core.ThreadAgentBooting); err != nil {
				return nil, err
			}
			_ = p.store.UpdateThreadMember(ctx, m)
			go p.bootSessionBackground(m, profile, priorSummary, priorSessionID)
			return m, nil
		}
		if m.Status == core.ThreadAgentActive || m.Status == core.ThreadAgentBooting {
			if p.sessionReady(key) {
				return m, nil
			}
			priorSummary = memberGetString(m, "progress_summary")
			priorSessionID = memberGetString(m, "acp_session_id")
			go p.bootSessionBackground(m, profile, priorSummary, priorSessionID)
			return m, nil
		}
	}

	// Create new DB record.
	member := &core.ThreadMember{
		ThreadID:       threadID,
		Kind:           core.ThreadMemberKindAgent,
		UserID:         profileID,
		AgentProfileID: profileID,
		Role:           core.ThreadMemberKindAgent,
		Status:         core.ThreadAgentBooting,
	}
	id, err := p.store.AddThreadMember(ctx, member)
	if err != nil {
		return nil, fmt.Errorf("create thread agent member: %w", err)
	}
	member.ID = id

	go p.bootSessionBackground(member, profile, priorSummary, priorSessionID)
	return member, nil
}

// bootSessionBackground runs the full ACP boot sequence in a background
// goroutine and publishes success/failure events via the EventBus.
func (p *ThreadSessionPool) bootSessionBackground(member *core.ThreadMember, profile *core.AgentProfile, priorSummary string, priorSessionID string) {
	bootCtx, cancel := context.WithTimeout(context.Background(), threadBootTimeout)
	defer cancel()
	if _, err := p.bootSession(bootCtx, member, profile, priorSummary, priorSessionID); err != nil {
		slog.Warn("thread pool: background boot failed",
			"thread_id", member.ThreadID, "profile", profile.ID, "error", err)
	}
}

func (p *ThreadSessionPool) bootSession(ctx context.Context, member *core.ThreadMember, profile *core.AgentProfile, priorSummary string, priorSessionID string) (*core.ThreadMember, error) {
	key := threadSessionKey{threadID: member.ThreadID, agentID: profile.ID}
	workspaceDir, scopeCfg, err := p.prepareThreadWorkspace(ctx, member.ThreadID)
	if err != nil {
		_ = updateThreadAgentStatus(member, core.ThreadAgentFailed)
		_ = p.store.UpdateThreadMember(ctx, member)
		return member, fmt.Errorf("prepare thread workspace: %w", err)
	}

	// Install thread-scoped skills into workspace agent home directories.
	p.ensureThreadSkills(workspaceDir, profile)

	// Build extra env vars for signal callback.
	extraEnv := map[string]string{}
	if p.tokenRegistry != nil && p.serverAddr != "" {
		scope := fmt.Sprintf("thread:%d:agent:%s", member.ThreadID, profile.ID)
		tok, tokErr := p.tokenRegistry.GenerateScopedToken(
			fmt.Sprintf("thread-agent-%d-%s", member.ThreadID, profile.ID),
			[]string{scope},
			fmt.Sprintf("thread/%d", member.ThreadID),
		)
		if tokErr != nil {
			slog.Warn("thread pool: failed to generate signal token", "thread_id", member.ThreadID, "error", tokErr)
		} else {
			extraEnv["AI_WORKFLOW_SERVER_ADDR"] = p.serverAddr
			extraEnv["AI_WORKFLOW_API_TOKEN"] = tok
		}
	}

	bridge := eventbridge.New(p.bus, core.EventThreadAgentOutput, eventbridge.Scope{
		SessionID: fmt.Sprintf("thread-%d-%s", member.ThreadID, profile.ID),
	})
	switcher := &switchingEventHandler{}
	switcher.Set(bridge)

	handler := acphandler.NewACPHandler(workspaceDir, "", nil)
	handler.SetThreadWorkspace(scopeCfg)
	handler.SetSuppressEvents(true)

	bootstrapFn := acpclient.Bootstrap
	if p.bootstrapFn != nil {
		bootstrapFn = p.bootstrapFn
	}
	bootResult, err := bootstrapFn(ctx, acpclient.BootstrapConfig{
		Profile:      profile,
		WorkDir:      workspaceDir,
		ExtraEnv:     extraEnv,
		Handler:      handler,
		EventHandler: switcher,
		Session: &acpclient.BootstrapSessionConfig{
			PriorSessionID: strings.TrimSpace(priorSessionID),
		},
	})
	if err != nil {
		_ = updateThreadAgentStatus(member, core.ThreadAgentFailed)
		_ = p.store.UpdateThreadMember(ctx, member)
		p.publishThreadEvent(ctx, core.EventThreadAgentFailed, member.ThreadID, profile.ID, map[string]any{"error": err.Error()})
		return member, err
	}
	client := bootResult.Client
	acpSessionID := bootResult.Session.ID

	memberSetAgentData(member, "acp_session_id", string(acpSessionID))

	// Build boot prompt.
	bootPrompt, err := p.buildBootPrompt(ctx, member.ThreadID, profile, priorSummary)
	if err != nil {
		slog.Warn("thread pool: build boot prompt failed, proceeding without", "error", err)
	}

	// Send boot prompt.
	if strings.TrimSpace(bootPrompt) != "" && !(bootResult.Session.Loaded && strings.TrimSpace(priorSummary) == "") {
		bootCtx, bootCancel := context.WithTimeout(ctx, threadBootPromptTimeout)
		defer bootCancel()
		_, err = client.PromptText(bootCtx, acpSessionID, bootPrompt)
		bridge.FlushPending(ctx)
		if err != nil {
			slog.Warn("thread pool: boot prompt failed", "thread_id", member.ThreadID, "profile", profile.ID, "error", err)
		}
	}

	pooled := &threadPooledSession{
		client:    client,
		sessionID: acpSessionID,
		events:    switcher,
		bridge:    bridge,
		lastUsed:  time.Now().UTC(),
	}

	p.mu.Lock()
	p.sessions[key] = pooled
	p.mu.Unlock()

	// Update DB to active.
	if err := updateThreadAgentStatus(member, core.ThreadAgentActive); err != nil {
		return nil, err
	}
	_ = p.store.UpdateThreadMember(ctx, member)

	p.publishThreadEvent(ctx, core.EventThreadAgentBooted, member.ThreadID, profile.ID, nil)
	p.publishThreadEvent(ctx, core.EventThreadAgentJoined, member.ThreadID, profile.ID, nil)

	slog.Info("thread pool: agent joined", "thread_id", member.ThreadID, "profile", profile.ID, "session_id", string(acpSessionID))
	return member, nil
}

func (p *ThreadSessionPool) buildBootPrompt(ctx context.Context, threadID int64, profile *core.AgentProfile, priorSummary string) (string, error) {
	thread, err := p.store.GetThread(ctx, threadID)
	if err != nil {
		return "", err
	}

	msgs, _ := p.store.ListThreadMessages(ctx, threadID, 20, 0)
	parts, _ := p.store.ListThreadMembers(ctx, threadID)

	// Resolve linked work items.
	var workItems []*core.WorkItem
	links, _ := p.store.ListWorkItemsByThread(ctx, threadID)
	for _, link := range links {
		wi, err := p.store.GetWorkItem(ctx, link.WorkItemID)
		if err == nil {
			workItems = append(workItems, wi)
		}
	}
	workspaceCtx, _ := threadctx.LoadContextFile(p.dataDir, threadID)

	return BuildBootPrompt(ThreadBootInput{
		Thread:         thread,
		RecentMessages: msgs,
		Participants:   parts,
		WorkItems:      workItems,
		AgentProfile:   profile,
		PriorSummary:   priorSummary,
		Workspace:      workspaceCtx,
		SharedTemplate: p.threadSharedBootTemplate,
	}), nil
}

// WaitAgentReady waits for boot/failed lifecycle events and falls back to a
// low-frequency status check so external state changes are still observed.
func (p *ThreadSessionPool) WaitAgentReady(ctx context.Context, threadID int64, profileID string) error {
	key := threadSessionKey{threadID: threadID, agentID: profileID}
	if err := p.agentReadyState(ctx, key, profileID); err != nil || p.sessionReady(key) {
		return err
	}

	var events <-chan core.Event
	if p.bus != nil {
		sub := p.bus.Subscribe(core.SubscribeOpts{
			Types:      []core.EventType{core.EventThreadAgentBooted, core.EventThreadAgentFailed, core.EventThreadAgentLeft},
			BufferSize: 8,
		})
		defer sub.Cancel()
		events = sub.C
	}

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for agent %q in thread %d", profileID, threadID)
		case event, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			if eventTypeMatchesThreadAgent(event, threadID, profileID) {
				if err := p.agentReadyState(ctx, key, profileID); err != nil || p.sessionReady(key) {
					return err
				}
			}
		case <-ticker.C:
			if err := p.agentReadyState(ctx, key, profileID); err != nil || p.sessionReady(key) {
				return err
			}
		}
	}
}

func (p *ThreadSessionPool) sessionReady(key threadSessionKey) bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.sessions[key] != nil
}

func (p *ThreadSessionPool) sessionForKey(key threadSessionKey) *threadPooledSession {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.sessions[key]
}

func (p *ThreadSessionPool) agentReadyState(ctx context.Context, key threadSessionKey, profileID string) error {
	if p.sessionReady(key) {
		return nil
	}
	if p == nil || p.store == nil {
		return fmt.Errorf("agent %q is not available in thread %d", profileID, key.threadID)
	}
	members, err := p.store.ListThreadMembers(ctx, key.threadID)
	if err != nil {
		return err
	}
	for _, member := range members {
		if member == nil || member.AgentProfileID != profileID {
			continue
		}
		switch member.Status {
		case core.ThreadAgentActive:
			return nil
		case core.ThreadAgentFailed:
			return fmt.Errorf("agent %q failed to boot in thread %d", profileID, key.threadID)
		case core.ThreadAgentLeft:
			return fmt.Errorf("agent %q left thread %d before becoming ready", profileID, key.threadID)
		}
	}
	return nil
}

func (p *ThreadSessionPool) recoverSession(ctx context.Context, key threadSessionKey, profileID string) error {
	if p == nil || p.store == nil {
		return fmt.Errorf("agent %q is not available in thread %d", profileID, key.threadID)
	}
	if p.sessionReady(key) {
		return nil
	}

	members, err := p.store.ListThreadMembers(ctx, key.threadID)
	if err != nil {
		return err
	}

	var member *core.ThreadMember
	for _, candidate := range members {
		if candidate == nil || candidate.Kind != core.ThreadMemberKindAgent || candidate.AgentProfileID != profileID {
			continue
		}
		switch candidate.Status {
		case core.ThreadAgentActive, core.ThreadAgentBooting, core.ThreadAgentPaused:
			member = candidate
		}
	}
	if member == nil {
		return fmt.Errorf("no active session for profile %q in thread %d", profileID, key.threadID)
	}

	profile, err := p.registry.ResolveByID(ctx, profileID)
	if err != nil {
		return fmt.Errorf("resolve profile %q: %w", profileID, err)
	}

	priorSummary := memberGetString(member, "progress_summary")
	priorSessionID := memberGetString(member, "acp_session_id")
	if member.Status == core.ThreadAgentPaused {
		if err := updateThreadAgentStatus(member, core.ThreadAgentBooting); err != nil {
			return err
		}
		_ = p.store.UpdateThreadMember(ctx, member)
	}

	_, err = p.bootSession(ctx, member, profile, priorSummary, priorSessionID)
	return err
}

func eventTypeMatchesThreadAgent(event core.Event, threadID int64, profileID string) bool {
	if event.Type != core.EventThreadAgentBooted && event.Type != core.EventThreadAgentFailed && event.Type != core.EventThreadAgentLeft {
		return false
	}
	var eventThreadID int64
	switch value := event.Data["thread_id"].(type) {
	case int64:
		eventThreadID = value
	case int:
		eventThreadID = int64(value)
	case float64:
		eventThreadID = int64(value)
	}
	eventProfileID, _ := event.Data["profile_id"].(string)
	return eventThreadID == threadID && strings.TrimSpace(eventProfileID) == profileID
}

// PromptAgent sends a raw prompt to an active thread agent and returns the
// reply without persisting it as a ThreadMessage. Higher-level orchestrators
// can use it to implement meeting flows such as concurrent or group chat.
func (p *ThreadSessionPool) PromptAgent(ctx context.Context, threadID int64, profileID string, message string) (*core.ThreadAgentPromptResult, error) {
	if p == nil {
		return nil, fmt.Errorf("thread session pool is nil")
	}

	key := threadSessionKey{threadID: threadID, agentID: profileID}
	pooled := p.sessionForKey(key)

	if pooled == nil {
		if err := p.recoverSession(ctx, key, profileID); err != nil {
			return nil, err
		}
		pooled = p.sessionForKey(key)
	}

	if pooled == nil {
		return nil, fmt.Errorf("no active session for profile %q in thread %d", profileID, threadID)
	}

	pooled.mu.Lock()
	defer pooled.mu.Unlock()
	if pooled.closing {
		return nil, fmt.Errorf("session for profile %q in thread %d is closing", profileID, threadID)
	}

	promptCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	result, err := pooled.client.PromptText(promptCtx, pooled.sessionID, message)
	pooled.bridge.FlushPending(ctx)

	if err != nil {
		return nil, fmt.Errorf("prompt agent %q: %w", profileID, err)
	}

	pooled.lastUsed = time.Now().UTC()
	pooled.turns++

	// Extract token usage from result.
	if result != nil && result.Usage != nil {
		pooled.inputTokens += int64(result.Usage.InputTokens)
		pooled.outputTokens += int64(result.Usage.OutputTokens)
	}

	reply := ""
	var inputTokens int64
	var outputTokens int64
	if result != nil {
		reply = strings.TrimSpace(result.Text)
		if result.Usage != nil {
			inputTokens = int64(result.Usage.InputTokens)
			outputTokens = int64(result.Usage.OutputTokens)
		}
	}

	// Persist token usage periodically (every 5 turns or on demand).
	if pooled.turns%5 == 0 {
		p.persistTokenUsage(ctx, threadID, profileID, pooled)
	}

	// Check context budget warning.
	p.checkContextBudget(ctx, threadID, profileID, pooled)

	return &core.ThreadAgentPromptResult{
		Content:      reply,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	}, nil
}

// SendMessage routes a human message to a specific active agent in a thread
// and saves the reply as a ThreadMessage.
func (p *ThreadSessionPool) SendMessage(ctx context.Context, threadID int64, profileID string, message string) error {
	promptResult, err := p.PromptAgent(ctx, threadID, profileID, message)
	if err != nil {
		return err
	}

	reply := ""
	if promptResult != nil {
		reply = strings.TrimSpace(promptResult.Content)
	}
	if reply == "" {
		return nil
	}

	agentMsg := &core.ThreadMessage{
		ThreadID: threadID,
		SenderID: profileID,
		Role:     "agent",
		Content:  reply,
	}
	if _, err := p.store.CreateThreadMessage(ctx, agentMsg); err != nil {
		slog.Warn("thread pool: save agent message failed", "thread_id", threadID, "profile", profileID, "error", err)
	}

	// Publish agent output event for WS broadcast.
	p.publishThreadEvent(ctx, core.EventThreadAgentOutput, threadID, profileID, map[string]any{
		"content": reply,
	})

	return nil
}

// RemoveAgent gracefully removes an agent from a thread. If graceful=true,
// it requests a progress summary from the agent before disconnecting.
func (p *ThreadSessionPool) RemoveAgent(ctx context.Context, threadID int64, agentSessionID int64) error {
	if p == nil {
		return fmt.Errorf("thread session pool is nil")
	}

	member, err := p.store.GetThreadMember(ctx, agentSessionID)
	if err != nil {
		return err
	}

	key := threadSessionKey{threadID: member.ThreadID, agentID: member.AgentProfileID}

	p.mu.Lock()
	pooled := p.sessions[key]
	delete(p.sessions, key)
	p.mu.Unlock()

	// Terminal states (failed/left) — just ensure DB is marked and clean up.
	if member.Status == core.ThreadAgentFailed || member.Status == core.ThreadAgentLeft {
		member.Status = core.ThreadAgentLeft
		_ = p.store.UpdateThreadMember(ctx, member)
		p.publishThreadEvent(ctx, core.EventThreadAgentLeft, member.ThreadID, member.AgentProfileID, nil)
		return nil
	}

	if pooled != nil {
		pooled.mu.Lock()
		pooled.closing = true

		// Request progress summary before closing (graceful leave).
		summary := p.requestProgressSummaryLocked(ctx, pooled)
		if strings.TrimSpace(summary) != "" {
			memberSetAgentData(member, "progress_summary", summary)
			if err := updateThreadAgentStatus(member, core.ThreadAgentPaused); err != nil {
				pooled.mu.Unlock()
				return err
			}
		} else {
			if err := updateThreadAgentStatus(member, core.ThreadAgentLeft); err != nil {
				pooled.mu.Unlock()
				return err
			}
		}

		// Persist final token usage.
		memberSetAgentData(member, "turn_count", pooled.turns)
		memberSetAgentData(member, "total_input_tokens", pooled.inputTokens)
		memberSetAgentData(member, "total_output_tokens", pooled.outputTokens)

		// Close ACP.
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = pooled.client.Close(closeCtx)
		cancel()
		pooled.mu.Unlock()
	} else {
		if err := updateThreadAgentStatus(member, core.ThreadAgentLeft); err != nil {
			return err
		}
	}

	_ = p.store.UpdateThreadMember(ctx, member)

	p.publishThreadEvent(ctx, core.EventThreadAgentLeft, member.ThreadID, member.AgentProfileID, nil)

	slog.Info("thread pool: agent removed", "thread_id", member.ThreadID, "profile", member.AgentProfileID, "status", member.Status)
	return nil
}

// requestProgressSummary asks the agent to summarize its progress before
// leaving. Returns the summary text or empty string on failure.
func (p *ThreadSessionPool) requestProgressSummary(ctx context.Context, pooled *threadPooledSession) string {
	if pooled == nil || pooled.client == nil {
		return ""
	}

	summaryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	pooled.mu.Lock()
	defer pooled.mu.Unlock()

	return p.requestProgressSummaryLocked(summaryCtx, pooled)
}

func (p *ThreadSessionPool) requestProgressSummaryLocked(ctx context.Context, pooled *threadPooledSession) string {
	if pooled == nil || pooled.client == nil {
		return ""
	}

	result, err := pooled.client.PromptText(ctx, pooled.sessionID,
		"You are about to leave this thread. Please provide a brief summary of your progress, key decisions made, and any pending items. Keep it concise (under 500 words).")
	pooled.bridge.FlushPending(ctx)

	if err != nil {
		slog.Warn("thread pool: progress summary request failed", "error", err)
		return ""
	}
	if result == nil {
		return ""
	}
	return strings.TrimSpace(result.Text)
}

// CleanupThread closes all in-memory ACP sessions for a thread before the
// thread aggregate is deleted from persistent storage.
func (p *ThreadSessionPool) CleanupThread(ctx context.Context, threadID int64) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	var toClose []*threadPooledSession
	for k, s := range p.sessions {
		if k.threadID == threadID {
			toClose = append(toClose, s)
			delete(p.sessions, k)
		}
	}
	p.mu.Unlock()

	for _, s := range toClose {
		if s.client != nil {
			closeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			if ctx != nil {
				closeCtx, cancel = context.WithTimeout(ctx, 3*time.Second)
			}
			_ = s.client.Close(closeCtx)
			cancel()
		}
	}
	return nil
}

// Close shuts down all sessions in the pool.
func (p *ThreadSessionPool) Close() {
	if p == nil {
		return
	}
	p.mu.Lock()
	all := make([]*threadPooledSession, 0, len(p.sessions))
	for k, s := range p.sessions {
		all = append(all, s)
		delete(p.sessions, k)
	}
	p.mu.Unlock()

	for _, s := range all {
		if s.client != nil {
			closeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			_ = s.client.Close(closeCtx)
			cancel()
		}
	}
}

// ActiveAgentProfileIDs returns the profile IDs of all active agents for a thread.
func (p *ThreadSessionPool) ActiveAgentProfileIDs(threadID int64) []string {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	var ids []string
	for k := range p.sessions {
		if k.threadID == threadID {
			ids = append(ids, k.agentID)
		}
	}
	return ids
}

func (p *ThreadSessionPool) persistTokenUsage(ctx context.Context, threadID int64, profileID string, pooled *threadPooledSession) {
	members, err := p.store.ListThreadMembers(ctx, threadID)
	if err != nil {
		return
	}
	for _, m := range members {
		if m.Kind == core.ThreadMemberKindAgent && m.AgentProfileID == profileID && (m.Status == core.ThreadAgentActive || m.Status == core.ThreadAgentBooting) {
			memberSetAgentData(m, "turn_count", pooled.turns)
			memberSetAgentData(m, "total_input_tokens", pooled.inputTokens)
			memberSetAgentData(m, "total_output_tokens", pooled.outputTokens)
			_ = p.store.UpdateThreadMember(ctx, m)
			return
		}
	}
}

func (p *ThreadSessionPool) checkContextBudget(ctx context.Context, threadID int64, profileID string, pooled *threadPooledSession) {
	// Look up the profile to check MaxContextTokens.
	profile, err := p.registry.ResolveByID(ctx, profileID)
	if err != nil || profile == nil {
		return
	}

	maxTokens := profile.Session.MaxContextTokens
	if maxTokens <= 0 {
		return
	}

	warnRatio := profile.Session.ContextWarnRatio
	if warnRatio <= 0 {
		warnRatio = 0.8
	}

	totalUsed := pooled.inputTokens + pooled.outputTokens
	threshold := int64(float64(maxTokens) * warnRatio)

	if totalUsed >= threshold {
		// Publish system warning.
		p.bus.Publish(ctx, core.Event{
			Type: core.EventThreadMessage,
			Data: map[string]any{
				"thread_id": threadID,
				"type":      "system_warning",
				"sender_id": "system",
				"message":   fmt.Sprintf("Agent %s is approaching context budget limit (%d/%d tokens, %.0f%%)", profileID, totalUsed, maxTokens, float64(totalUsed)/float64(maxTokens)*100),
			},
			Timestamp: time.Now().UTC(),
		})
	}
}

func (p *ThreadSessionPool) publishThreadEvent(ctx context.Context, eventType core.EventType, threadID int64, profileID string, extra map[string]any) {
	if p == nil || p.bus == nil {
		return
	}
	data := map[string]any{
		"thread_id":  threadID,
		"profile_id": profileID,
	}
	for k, v := range extra {
		data[k] = v
	}
	p.bus.Publish(ctx, core.Event{
		Type:      eventType,
		Data:      data,
		Timestamp: time.Now().UTC(),
	})
}

func (p *ThreadSessionPool) prepareThreadWorkspace(ctx context.Context, threadID int64) (string, acphandler.ThreadWorkspaceConfig, error) {
	if strings.TrimSpace(p.dataDir) == "" {
		return "", acphandler.ThreadWorkspaceConfig{}, fmt.Errorf("thread data dir is not configured")
	}
	paths, err := threadctx.EnsureLayout(p.dataDir, threadID)
	if err != nil {
		return "", acphandler.ThreadWorkspaceConfig{}, err
	}
	workspaceCtx, err := threadctx.SyncContextFile(ctx, p.store, p.dataDir, threadID)
	if err != nil {
		return "", acphandler.ThreadWorkspaceConfig{}, err
	}

	cfg := acphandler.ThreadWorkspaceConfig{
		ThreadID:     threadID,
		WorkspaceDir: paths.ThreadDir,
	}
	if workspaceCtx != nil {
		refs, _ := p.store.ListThreadContextRefs(ctx, threadID)
		for _, ref := range refs {
			mount, err := threadctx.ResolveMount(ctx, p.store, ref)
			if err != nil || mount == nil {
				continue
			}
			cfg.Mounts = append(cfg.Mounts, acphandler.ThreadMount{
				Alias:         mount.Slug,
				TargetPath:    mount.TargetPath,
				Access:        string(mount.Access),
				CheckCommands: append([]string(nil), mount.CheckCommands...),
			})
		}
	}
	return paths.ThreadDir, cfg, nil
}

// threadScopedSkills lists skills that should be auto-installed into every thread agent workspace.
var threadScopedSkills = []string{"task-signal"}

// ensureThreadSkills creates .codex/skills/ and .claude/skills/ inside the thread workspace
// and symlinks each thread-scoped skill from the global skills root. This makes ACP agents
// auto-discover the skills when CODEX_HOME or CLAUDE_CONFIG_DIR points at the workspace subdir.
func (p *ThreadSessionPool) ensureThreadSkills(workspaceDir string, profile *core.AgentProfile) {
	if workspaceDir == "" {
		return
	}
	skillsRoot, err := resolveGlobalSkillsRoot(p.dataDir)
	if err != nil {
		slog.Warn("thread pool: cannot resolve skills root", "error", err)
		return
	}

	// Determine home directory names based on driver type.
	homeDirs := inferAgentHomeDirs(profile)

	for _, homeDir := range homeDirs {
		targetSkillsDir := filepath.Join(workspaceDir, homeDir, "skills")
		if err := os.MkdirAll(targetSkillsDir, 0o755); err != nil {
			slog.Warn("thread pool: create skills dir failed", "dir", targetSkillsDir, "error", err)
			continue
		}
		for _, skillName := range threadScopedSkills {
			src := filepath.Join(skillsRoot, skillName)
			if _, err := os.Stat(src); err != nil {
				continue // skill not extracted yet
			}
			dst := filepath.Join(targetSkillsDir, skillName)
			if _, err := os.Lstat(dst); err == nil {
				continue // already present
			}
			if linkErr := linkDirBestEffort(dst, src); linkErr != nil {
				slog.Warn("thread pool: link skill failed", "skill", skillName, "dst", dst, "error", linkErr)
			}
		}
	}
}

// inferAgentHomeDirs returns workspace-relative home dir names for the agent.
// Always produces both .codex and .claude so skills are available regardless of runtime.
func inferAgentHomeDirs(profile *core.AgentProfile) []string {
	if profile == nil {
		return []string{".codex", ".claude"}
	}
	cmd := strings.ToLower(strings.TrimSpace(profile.Driver.LaunchCommand))
	id := strings.ToLower(strings.TrimSpace(profile.ID))
	if strings.Contains(cmd, "codex") || strings.Contains(id, "codex") {
		return []string{".codex"}
	}
	if strings.Contains(cmd, "claude") || strings.Contains(id, "claude") {
		return []string{".claude"}
	}
	return []string{".codex", ".claude"}
}

func resolveGlobalSkillsRoot(dataDir string) (string, error) {
	if strings.TrimSpace(dataDir) != "" {
		return filepath.Join(strings.TrimSpace(dataDir), "skills"), nil
	}
	// Fallback: use appdata default.
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ai-workflow", "skills"), nil
}

func linkDirBestEffort(dst, src string) error {
	if err := os.Symlink(src, dst); err == nil {
		return nil
	}
	// Windows fallback: junction.
	if runtime.GOOS == "windows" {
		out, err := exec.Command("cmd", "/c", "mklink", "/J", dst, src).CombinedOutput()
		if err != nil {
			return fmt.Errorf("mklink /J: %s: %w", strings.TrimSpace(string(out)), err)
		}
		return nil
	}
	return fmt.Errorf("symlink %s -> %s failed", dst, src)
}
