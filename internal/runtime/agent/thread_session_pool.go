package agentruntime

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	acphandler "github.com/yoke233/ai-workflow/internal/adapters/agent/acp"
	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
	eventbridge "github.com/yoke233/ai-workflow/internal/adapters/events/bridge"
	"github.com/yoke233/ai-workflow/internal/core"
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

// ThreadSessionPool manages ACP sessions for agents participating in Threads.
// Unlike ACPSessionPool (which is tied to Issue lifecycle), ThreadSessionPool
// is driven by user invite/remove actions and has no Step/Execution concept.
type ThreadSessionPool struct {
	store    core.Store
	bus      core.EventBus
	registry core.AgentRegistry

	mu       sync.Mutex
	sessions map[threadSessionKey]*threadPooledSession
}

// NewThreadSessionPool creates a pool for managing Thread agent ACP sessions.
func NewThreadSessionPool(store core.Store, bus core.EventBus, registry core.AgentRegistry) *ThreadSessionPool {
	return &ThreadSessionPool{
		store:    store,
		bus:      bus,
		registry: registry,
		sessions: make(map[threadSessionKey]*threadPooledSession),
	}
}

// InviteAgent starts an ACP session for the given profile in the given thread.
// It creates a DB record, launches the ACP process, runs the boot sequence,
// and returns the updated ThreadAgentSession.
func (p *ThreadSessionPool) InviteAgent(ctx context.Context, threadID int64, profileID string) (*core.ThreadAgentSession, error) {
	if p == nil {
		return nil, fmt.Errorf("thread session pool is nil")
	}

	// Resolve profile + driver.
	profile, driver, err := p.registry.ResolveByID(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("resolve profile %q: %w", profileID, err)
	}

	// Check for existing paused session (for resume with prior summary).
	var priorSummary string
	existingSessions, _ := p.store.ListThreadAgentSessions(ctx, threadID)
	for _, s := range existingSessions {
		if s.AgentProfileID == profileID && s.Status == core.ThreadAgentPaused {
			priorSummary = s.ProgressSummary
			// Update existing session to booting.
			s.Status = core.ThreadAgentBooting
			_ = p.store.UpdateThreadAgentSession(ctx, s)
			return p.bootSession(ctx, s, profile, driver, priorSummary)
		}
		if s.AgentProfileID == profileID && (s.Status == core.ThreadAgentActive || s.Status == core.ThreadAgentBooting) {
			return s, nil // already active
		}
	}

	// Create new DB record.
	sess := &core.ThreadAgentSession{
		ThreadID:       threadID,
		AgentProfileID: profileID,
		Status:         core.ThreadAgentBooting,
	}
	id, err := p.store.CreateThreadAgentSession(ctx, sess)
	if err != nil {
		return nil, fmt.Errorf("create thread agent session: %w", err)
	}
	sess.ID = id

	return p.bootSession(ctx, sess, profile, driver, priorSummary)
}

func (p *ThreadSessionPool) bootSession(ctx context.Context, sess *core.ThreadAgentSession, profile *core.AgentProfile, driver *core.AgentDriver, priorSummary string) (*core.ThreadAgentSession, error) {
	key := threadSessionKey{threadID: sess.ThreadID, agentID: profile.ID}

	// Launch ACP process.
	launchCfg := acpclient.LaunchConfig{
		Command: driver.LaunchCommand,
		Args:    driver.LaunchArgs,
		Env:     cloneStringMap(driver.Env),
	}

	bridge := eventbridge.New(p.bus, core.EventThreadAgentOutput, eventbridge.Scope{
		SessionID: fmt.Sprintf("thread-%d-%s", sess.ThreadID, profile.ID),
	})
	switcher := &switchingEventHandler{}
	switcher.Set(bridge)

	handler := acphandler.NewACPHandler("", "", nil)
	handler.SetSuppressEvents(true)
	client, err := acpclient.New(launchCfg, handler, acpclient.WithEventHandler(switcher))
	if err != nil {
		sess.Status = core.ThreadAgentFailed
		_ = p.store.UpdateThreadAgentSession(ctx, sess)
		p.publishThreadEvent(ctx, core.EventThreadAgentFailed, sess.ThreadID, profile.ID, map[string]any{"error": err.Error()})
		return sess, fmt.Errorf("launch ACP agent %q: %w", driver.ID, err)
	}

	caps := profile.EffectiveCapabilities()
	initCtx, initCancel := context.WithTimeout(ctx, 30*time.Second)
	defer initCancel()
	if err := client.Initialize(initCtx, acpclient.ClientCapabilities{
		FSRead:   caps.FSRead,
		FSWrite:  caps.FSWrite,
		Terminal: caps.Terminal,
	}); err != nil {
		_ = client.Close(context.Background())
		sess.Status = core.ThreadAgentFailed
		_ = p.store.UpdateThreadAgentSession(ctx, sess)
		p.publishThreadEvent(ctx, core.EventThreadAgentFailed, sess.ThreadID, profile.ID, map[string]any{"error": err.Error()})
		return sess, fmt.Errorf("initialize ACP agent %q: %w", driver.ID, err)
	}

	acpSessionID, err := client.NewSession(initCtx, acpproto.NewSessionRequest{})
	if err != nil {
		_ = client.Close(context.Background())
		sess.Status = core.ThreadAgentFailed
		_ = p.store.UpdateThreadAgentSession(ctx, sess)
		p.publishThreadEvent(ctx, core.EventThreadAgentFailed, sess.ThreadID, profile.ID, map[string]any{"error": err.Error()})
		return sess, fmt.Errorf("create ACP session: %w", err)
	}

	sess.ACPSessionID = string(acpSessionID)

	// Build boot prompt.
	bootPrompt, err := p.buildBootPrompt(ctx, sess.ThreadID, profile, priorSummary)
	if err != nil {
		slog.Warn("thread pool: build boot prompt failed, proceeding without", "error", err)
	}

	// Send boot prompt.
	if strings.TrimSpace(bootPrompt) != "" {
		bootCtx, bootCancel := context.WithTimeout(ctx, 60*time.Second)
		defer bootCancel()
		_, err = client.Prompt(bootCtx, acpproto.PromptRequest{
			SessionId: acpSessionID,
			Prompt:    []acpproto.ContentBlock{{Text: &acpproto.ContentBlockText{Text: bootPrompt}}},
		})
		bridge.FlushPending(ctx)
		if err != nil {
			slog.Warn("thread pool: boot prompt failed", "thread_id", sess.ThreadID, "profile", profile.ID, "error", err)
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
	sess.Status = core.ThreadAgentActive
	_ = p.store.UpdateThreadAgentSession(ctx, sess)

	p.publishThreadEvent(ctx, core.EventThreadAgentBooted, sess.ThreadID, profile.ID, nil)
	p.publishThreadEvent(ctx, core.EventThreadAgentJoined, sess.ThreadID, profile.ID, nil)

	slog.Info("thread pool: agent joined", "thread_id", sess.ThreadID, "profile", profile.ID, "session_id", string(acpSessionID))
	return sess, nil
}

func (p *ThreadSessionPool) buildBootPrompt(ctx context.Context, threadID int64, profile *core.AgentProfile, priorSummary string) (string, error) {
	thread, err := p.store.GetThread(ctx, threadID)
	if err != nil {
		return "", err
	}

	msgs, _ := p.store.ListThreadMessages(ctx, threadID, 20, 0)
	parts, _ := p.store.ListThreadParticipants(ctx, threadID)

	// Resolve linked work items.
	var workItems []*core.Issue
	links, _ := p.store.ListWorkItemsByThread(ctx, threadID)
	for _, link := range links {
		issue, err := p.store.GetIssue(ctx, link.WorkItemID)
		if err == nil {
			workItems = append(workItems, issue)
		}
	}

	return BuildBootPrompt(ThreadBootInput{
		Thread:         thread,
		RecentMessages: msgs,
		Participants:   parts,
		WorkItems:      workItems,
		AgentProfile:   profile,
		PriorSummary:   priorSummary,
	}), nil
}

// SendMessage routes a human message to all active agents in a thread and
// saves the agent responses as ThreadMessages.
func (p *ThreadSessionPool) SendMessage(ctx context.Context, threadID int64, profileID string, message string) error {
	if p == nil {
		return fmt.Errorf("thread session pool is nil")
	}

	key := threadSessionKey{threadID: threadID, agentID: profileID}

	p.mu.Lock()
	pooled := p.sessions[key]
	p.mu.Unlock()

	if pooled == nil {
		return fmt.Errorf("no active session for profile %q in thread %d", profileID, threadID)
	}

	pooled.mu.Lock()
	defer pooled.mu.Unlock()
	if pooled.closing {
		return fmt.Errorf("session for profile %q in thread %d is closing", profileID, threadID)
	}

	promptCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	result, err := pooled.client.Prompt(promptCtx, acpproto.PromptRequest{
		SessionId: pooled.sessionID,
		Prompt:    []acpproto.ContentBlock{{Text: &acpproto.ContentBlockText{Text: message}}},
	})
	pooled.bridge.FlushPending(ctx)

	if err != nil {
		return fmt.Errorf("prompt agent %q: %w", profileID, err)
	}

	pooled.lastUsed = time.Now().UTC()
	pooled.turns++

	// Extract token usage from result.
	if result != nil && result.Usage != nil {
		pooled.inputTokens += int64(result.Usage.InputTokens)
		pooled.outputTokens += int64(result.Usage.OutputTokens)
	}

	// Save agent response as a thread message.
	reply := ""
	if result != nil {
		reply = strings.TrimSpace(result.Text)
	}
	if reply != "" {
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
	}

	// Persist token usage periodically (every 5 turns or on demand).
	if pooled.turns%5 == 0 {
		p.persistTokenUsage(ctx, threadID, profileID, pooled)
	}

	// Check context budget warning.
	p.checkContextBudget(ctx, threadID, profileID, pooled)

	return nil
}

// RemoveAgent gracefully removes an agent from a thread. If graceful=true,
// it requests a progress summary from the agent before disconnecting.
func (p *ThreadSessionPool) RemoveAgent(ctx context.Context, threadID int64, agentSessionID int64) error {
	if p == nil {
		return fmt.Errorf("thread session pool is nil")
	}

	sess, err := p.store.GetThreadAgentSession(ctx, agentSessionID)
	if err != nil {
		return err
	}

	key := threadSessionKey{threadID: sess.ThreadID, agentID: sess.AgentProfileID}

	p.mu.Lock()
	pooled := p.sessions[key]
	delete(p.sessions, key)
	p.mu.Unlock()

	if pooled != nil {
		pooled.mu.Lock()
		pooled.closing = true

		// Request progress summary before closing (graceful leave).
		summary := p.requestProgressSummaryLocked(ctx, pooled)
		if strings.TrimSpace(summary) != "" {
			sess.ProgressSummary = summary
			sess.Status = core.ThreadAgentPaused
		} else {
			sess.Status = core.ThreadAgentLeft
		}

		// Persist final token usage.
		sess.TurnCount = pooled.turns
		sess.TotalInputTokens = pooled.inputTokens
		sess.TotalOutputTokens = pooled.outputTokens

		// Close ACP.
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = pooled.client.Close(closeCtx)
		cancel()
		pooled.mu.Unlock()
	} else {
		sess.Status = core.ThreadAgentLeft
	}

	_ = p.store.UpdateThreadAgentSession(ctx, sess)

	p.publishThreadEvent(ctx, core.EventThreadAgentLeft, sess.ThreadID, sess.AgentProfileID, nil)

	slog.Info("thread pool: agent removed", "thread_id", sess.ThreadID, "profile", sess.AgentProfileID, "status", sess.Status)
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

	result, err := pooled.client.Prompt(ctx, acpproto.PromptRequest{
		SessionId: pooled.sessionID,
		Prompt: []acpproto.ContentBlock{{Text: &acpproto.ContentBlockText{
			Text: "You are about to leave this thread. Please provide a brief summary of your progress, key decisions made, and any pending items. Keep it concise (under 500 words).",
		}}},
	})
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

// CleanupThread closes all agent sessions for a thread.
func (p *ThreadSessionPool) CleanupThread(threadID int64) {
	if p == nil {
		return
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
			_ = s.client.Close(closeCtx)
			cancel()
		}
	}
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
	sessions, err := p.store.ListThreadAgentSessions(ctx, threadID)
	if err != nil {
		return
	}
	for _, s := range sessions {
		if s.AgentProfileID == profileID && (s.Status == core.ThreadAgentActive || s.Status == core.ThreadAgentBooting) {
			s.TurnCount = pooled.turns
			s.TotalInputTokens = pooled.inputTokens
			s.TotalOutputTokens = pooled.outputTokens
			_ = p.store.UpdateThreadAgentSession(ctx, s)
			return
		}
	}
}

func (p *ThreadSessionPool) checkContextBudget(ctx context.Context, threadID int64, profileID string, pooled *threadPooledSession) {
	// Look up the profile to check MaxContextTokens.
	profile, _, err := p.registry.ResolveByID(ctx, profileID)
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

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
