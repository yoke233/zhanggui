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

// InviteAgent starts an ACP session for the given profile in the given thread.
// It creates a DB record, launches the ACP process, runs the boot sequence,
// and returns the updated ThreadMember.
func (p *ThreadSessionPool) InviteAgent(ctx context.Context, threadID int64, profileID string) (*core.ThreadMember, error) {
	if p == nil {
		return nil, fmt.Errorf("thread session pool is nil")
	}

	// Resolve profile.
	profile, err := p.registry.ResolveByID(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("resolve profile %q: %w", profileID, err)
	}

	// Check for existing paused agent member (for resume with prior summary).
	var priorSummary string
	existingMembers, _ := p.store.ListThreadMembers(ctx, threadID)
	for _, m := range existingMembers {
		if m.Kind != "agent" || m.AgentProfileID != profileID {
			continue
		}
		if m.Status == core.ThreadAgentPaused {
			priorSummary = memberGetString(m, "progress_summary")
			if err := updateThreadAgentStatus(m, core.ThreadAgentBooting); err != nil {
				return nil, err
			}
			_ = p.store.UpdateThreadMember(ctx, m)
			return p.bootSession(ctx, m, profile, priorSummary)
		}
		if m.Status == core.ThreadAgentActive || m.Status == core.ThreadAgentBooting {
			return m, nil // already active
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

	return p.bootSession(ctx, member, profile, priorSummary)
}

func (p *ThreadSessionPool) bootSession(ctx context.Context, member *core.ThreadMember, profile *core.AgentProfile, priorSummary string) (*core.ThreadMember, error) {
	key := threadSessionKey{threadID: member.ThreadID, agentID: profile.ID}

	// Launch ACP process.
	launchCfg := acpclient.LaunchConfig{
		Command: profile.Driver.LaunchCommand,
		Args:    profile.Driver.LaunchArgs,
		Env:     cloneStringMap(profile.Driver.Env),
	}

	bridge := eventbridge.New(p.bus, core.EventThreadAgentOutput, eventbridge.Scope{
		SessionID: fmt.Sprintf("thread-%d-%s", member.ThreadID, profile.ID),
	})
	switcher := &switchingEventHandler{}
	switcher.Set(bridge)

	handler := acphandler.NewACPHandler("", "", nil)
	handler.SetSuppressEvents(true)
	client, err := acpclient.New(launchCfg, handler, acpclient.WithEventHandler(switcher))
	if err != nil {
		_ = updateThreadAgentStatus(member, core.ThreadAgentFailed)
		_ = p.store.UpdateThreadMember(ctx, member)
		p.publishThreadEvent(ctx, core.EventThreadAgentFailed, member.ThreadID, profile.ID, map[string]any{"error": err.Error()})
		return member, fmt.Errorf("launch ACP agent %q: %w", profile.ID, err)
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
		_ = updateThreadAgentStatus(member, core.ThreadAgentFailed)
		_ = p.store.UpdateThreadMember(ctx, member)
		p.publishThreadEvent(ctx, core.EventThreadAgentFailed, member.ThreadID, profile.ID, map[string]any{"error": err.Error()})
		return member, fmt.Errorf("initialize ACP agent %q: %w", profile.ID, err)
	}

	acpSessionID, err := client.NewSession(initCtx, acpproto.NewSessionRequest{})
	if err != nil {
		_ = client.Close(context.Background())
		_ = updateThreadAgentStatus(member, core.ThreadAgentFailed)
		_ = p.store.UpdateThreadMember(ctx, member)
		p.publishThreadEvent(ctx, core.EventThreadAgentFailed, member.ThreadID, profile.ID, map[string]any{"error": err.Error()})
		return member, fmt.Errorf("create ACP session: %w", err)
	}

	memberSetAgentData(member, "acp_session_id", string(acpSessionID))

	// Build boot prompt.
	bootPrompt, err := p.buildBootPrompt(ctx, member.ThreadID, profile, priorSummary)
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

	member, err := p.store.GetThreadMember(ctx, agentSessionID)
	if err != nil {
		return err
	}

	key := threadSessionKey{threadID: member.ThreadID, agentID: member.AgentProfileID}

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
