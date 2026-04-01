package agentruntime

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	acphandler "github.com/yoke233/zhanggui/internal/adapters/agent/acp"
	"github.com/yoke233/zhanggui/internal/adapters/agent/acpclient"
	probeacp "github.com/yoke233/zhanggui/internal/adapters/probe/acp"
	v2sandbox "github.com/yoke233/zhanggui/internal/adapters/sandbox"
	runtimeapp "github.com/yoke233/zhanggui/internal/application/runtime"
	"github.com/yoke233/zhanggui/internal/core"
)

// LocalSessionManager manages ACP sessions in the same process.
// This is the default mode — no external dependencies, same behavior as before.
//
// StartRun executes synchronously (blocks until the run completes).
// WatchRun returns the cached result immediately.
type LocalSessionManager struct {
	pool    *ACPSessionPool
	sandbox v2sandbox.Sandbox

	mu          sync.Mutex
	handles     map[string]*localHandle
	invocations map[string]*localInvocation
	nextID      int64

	activeCount atomic.Int32
	drainWg     sync.WaitGroup
}

type localHandle struct {
	pooled     *pooledACPSession
	standalone *acpclient.Client
	events     *switchingEventHandler
	sessionID  acpproto.SessionId
	agentCtx   *core.AgentContext
	profile    *core.AgentProfile
	launch     acpclient.LaunchConfig
	caps       acpclient.ClientCapabilities
	workDir    string
	mcpServers []acpproto.McpServer
	reuse      bool
	workItemID int64
	actionID   int64
	runID      int64
}

type localInvocationEvent struct {
	seq    int64
	update acpclient.SessionUpdate
}

type localInvocation struct {
	id         string
	handleID   string
	runID      int64
	workItemID int64
	actionID   int64
	status     runtimeapp.RunRuntimeState
	result     *runtimeapp.RunResult
	err        error
	done       chan struct{} // closed when the run completes
	events     []localInvocationEvent
	createdAt  time.Time
}

// NewLocalSessionManager creates a session manager that runs agents in-process.
func NewLocalSessionManager(pool *ACPSessionPool, sandbox v2sandbox.Sandbox) *LocalSessionManager {
	return &LocalSessionManager{
		pool:        pool,
		sandbox:     sandbox,
		handles:     make(map[string]*localHandle),
		invocations: make(map[string]*localInvocation),
	}
}

func (m *LocalSessionManager) nextHandleID() string {
	m.nextID++
	return fmt.Sprintf("local-%d", m.nextID)
}

// Acquire gets or creates an ACP session.
func (m *LocalSessionManager) Acquire(ctx context.Context, in runtimeapp.SessionAcquireInput) (*runtimeapp.SessionHandle, error) {
	sb := m.sandbox
	if sb == nil {
		sb = v2sandbox.NoopSandbox{}
	}
	sandboxedLaunch := in.Launch
	var err error
	if !acpclient.UsesInProcAdapterProfile(in.Profile) {
		scope := fmt.Sprintf("workitem-%d", in.WorkItemID)
		if !in.Reuse {
			scope = fmt.Sprintf("workitem-%d-run-%d", in.WorkItemID, in.RunID)
		}
		sandboxedLaunch, err = sb.Prepare(ctx, v2sandbox.PrepareInput{
			Profile:         in.Profile,
			Launch:          in.Launch,
			Scope:           scope,
			ExtraSkills:     in.ExtraSkills,
			EphemeralSkills: in.EphemeralSkills,
		})
		if err != nil {
			return nil, fmt.Errorf("prepare sandbox: %w", err)
		}
	}

	m.mu.Lock()
	handleID := m.nextHandleID()
	m.mu.Unlock()

	lh := &localHandle{
		reuse:      in.Reuse,
		profile:    in.Profile,
		workItemID: in.WorkItemID,
		actionID:   in.ActionID,
		runID:      in.RunID,
		launch:     sandboxedLaunch,
		caps:       in.Caps,
		workDir:    in.WorkDir,
	}

	if in.Reuse && m.pool != nil {
		sess, ac, err := m.pool.Acquire(ctx, acpSessionAcquireInput{
			Profile:    in.Profile,
			Launch:     sandboxedLaunch,
			Caps:       in.Caps,
			WorkDir:    in.WorkDir,
			MCPFactory: in.MCPFactory,
			WorkItemID: in.WorkItemID,
			ActionID:   in.ActionID,
			RunID:      in.RunID,
			IdleTTL:    in.IdleTTL,
			MaxTurns:   in.MaxTurns,
		})
		if err != nil {
			return nil, err
		}
		lh.pooled = sess
		lh.agentCtx = ac
		lh.sessionID = sess.sessionID
		lh.events = sess.events
		if in.MCPFactory != nil && sess.client != nil {
			lh.mcpServers = in.MCPFactory(sess.client.SupportsSSEMCP())
		}
	} else {
		switcher := &switchingEventHandler{}
		handler := acphandler.NewACPHandler(in.WorkDir, "", nil)
		handler.SetSuppressEvents(true)

		bootResult, err := acpclient.Bootstrap(ctx, acpclient.BootstrapConfig{
			Profile:        in.Profile,
			WorkDir:        in.WorkDir,
			LaunchOverride: &sandboxedLaunch,
			Handler:        handler,
			EventHandler:   switcher,
			Session: &acpclient.BootstrapSessionConfig{
				MCPFactory: in.MCPFactory,
			},
		})
		if err != nil {
			return nil, err
		}
		handler.SetSessionID(string(bootResult.Session.ID))

		lh.standalone = bootResult.Client
		lh.sessionID = bootResult.Session.ID
		lh.events = switcher
		if in.MCPFactory != nil {
			lh.mcpServers = in.MCPFactory(bootResult.SupportsSSEMCP)
		}
	}

	handle := &runtimeapp.SessionHandle{ID: handleID}
	if lh.agentCtx != nil && lh.agentCtx.ID > 0 {
		id := lh.agentCtx.ID
		handle.AgentContextID = &id
	}
	if lh.reuse && lh.pooled != nil {
		_, turns, _, _ := lh.pooled.statsSnapshot()
		if turns > 0 {
			handle.HasPriorTurns = true
		}
	}

	m.mu.Lock()
	m.handles[handleID] = lh
	m.mu.Unlock()

	return handle, nil
}

// StartRun executes synchronously in local mode.
// Returns an invocation ID; the result is available immediately via WatchRun.
func (m *LocalSessionManager) StartRun(ctx context.Context, handle *runtimeapp.SessionHandle, text string) (string, error) {
	m.mu.Lock()
	lh, ok := m.handles[handle.ID]
	if !ok {
		m.mu.Unlock()
		return "", fmt.Errorf("session handle %q not found", handle.ID)
	}
	invocationID := fmt.Sprintf("li-%d-%d", time.Now().UnixNano(), m.nextID)
	m.nextID++
	inv := &localInvocation{
		id:         invocationID,
		handleID:   handle.ID,
		runID:      lh.runID,
		workItemID: lh.workItemID,
		actionID:   lh.actionID,
		status:     runtimeapp.RunRunning,
		done:       make(chan struct{}),
		createdAt:  time.Now().UTC(),
	}
	m.invocations[invocationID] = inv
	m.mu.Unlock()

	m.activeCount.Add(1)
	m.drainWg.Add(1)

	// Execute synchronously.
	result, err := m.executeRun(ctx, lh, text, inv)

	m.mu.Lock()
	if err != nil {
		inv.status = runtimeapp.RunFailed
		inv.err = err
	} else {
		inv.status = runtimeapp.RunDone
		inv.result = result
	}
	close(inv.done)
	m.mu.Unlock()

	m.activeCount.Add(-1)
	m.drainWg.Done()

	if err != nil {
		return invocationID, err
	}
	return invocationID, nil
}

func (m *LocalSessionManager) executeRun(ctx context.Context, lh *localHandle, text string, inv *localInvocation) (*runtimeapp.RunResult, error) {
	// Capture events for the invocation record.
	collector := &eventCollector{inv: inv, mu: &m.mu}

	if lh.events != nil {
		lh.events.Set(collector)
		defer lh.events.Set(nil)
	}

	var client *acpclient.Client
	var unlock func()
	if lh.reuse && lh.pooled != nil {
		lh.pooled.mu.Lock()
		unlock = lh.pooled.mu.Unlock
		client = lh.pooled.client
	} else {
		client = lh.standalone
	}

	// Pre-run token budget check (reuse sessions only).
	if lh.reuse && lh.pooled != nil && m.pool != nil && lh.profile != nil {
		status := m.pool.CheckTokenBudget(lh.pooled, lh.profile)
		if status == TokenBudgetExceeded {
			input, output := m.pool.SessionTokenUsage(lh.pooled)
			return nil, fmt.Errorf("token budget exceeded for agent %s (used %d input + %d output, limit %d): %w",
				lh.profile.ID, input, output, lh.profile.Session.MaxContextTokens, core.ErrTokenBudgetExceeded)
		}
		if status == TokenBudgetWarning {
			input, output := m.pool.SessionTokenUsage(lh.pooled)
			slog.Warn("token budget warning: approaching limit",
				"agent", lh.profile.ID,
				"workitem_id", lh.workItemID,
				"input_tokens", input,
				"output_tokens", output,
				"limit", lh.profile.Session.MaxContextTokens)
		}
	}

	result, err := client.PromptText(ctx, lh.sessionID, text)
	if unlock != nil {
		unlock()
		unlock = nil
	}
	if err != nil {
		if lh.reuse && lh.pooled != nil && m.pool != nil {
			m.pool.Invalidate(context.Background(), lh.pooled, lh.agentCtx)
		}
		return nil, fmt.Errorf("ACP run failed: %w", err)
	}

	if lh.reuse && lh.pooled != nil && m.pool != nil {
		m.pool.NoteTurn(ctx, lh.agentCtx, lh.pooled)
	}

	out := &runtimeapp.RunResult{
		Text:       strings.TrimSpace(result.Text),
		StopReason: string(result.StopReason),
	}
	if result.Usage != nil {
		out.InputTokens = int64(result.Usage.InputTokens)
		out.OutputTokens = int64(result.Usage.OutputTokens)
		if result.Usage.CachedReadTokens != nil {
			out.CacheReadTokens = int64(*result.Usage.CachedReadTokens)
		}
		if result.Usage.CachedWriteTokens != nil {
			out.CacheWriteTokens = int64(*result.Usage.CachedWriteTokens)
		}
		if result.Usage.ThoughtTokens != nil {
			out.ReasoningTokens = int64(*result.Usage.ThoughtTokens)
		}
	}

	// Post-run: record token usage for budget tracking.
	if lh.reuse && lh.pooled != nil && m.pool != nil {
		m.pool.NoteTokens(lh.pooled, out.InputTokens, out.OutputTokens)
	}

	if lh.agentCtx != nil && lh.agentCtx.ID > 0 {
		id := lh.agentCtx.ID
		out.AgentContextID = &id
	}
	return out, nil
}

func localProfileID(profile *core.AgentProfile) string {
	if profile == nil || strings.TrimSpace(profile.ID) == "" {
		return "unknown"
	}
	return profile.ID
}

// WatchRun returns the result of a completed run (local mode completes synchronously).
// If the run is still active (shouldn't happen in local mode), it waits.
func (m *LocalSessionManager) WatchRun(ctx context.Context, invocationID string, lastEventSeq int64, sink runtimeapp.EventSink) (*runtimeapp.RunResult, error) {
	m.mu.Lock()
	inv, ok := m.invocations[invocationID]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("invocation %q not found", invocationID)
	}

	// Wait for completion.
	select {
	case <-inv.done:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Replay events to sink if requested.
	if sink != nil {
		m.mu.Lock()
		events := append([]localInvocationEvent{}, inv.events...)
		m.mu.Unlock()
		for _, ev := range events {
			if ev.seq <= lastEventSeq {
				continue
			}
			_ = sink.HandleSessionUpdate(ctx, ev.update)
		}
	}

	if inv.err != nil {
		return nil, inv.err
	}
	return inv.result, nil
}

// RecoverRuns returns recent run statuses (local mode: only in-memory).
func (m *LocalSessionManager) RecoverRuns(_ context.Context, since time.Time) ([]runtimeapp.RunRuntimeStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var out []runtimeapp.RunRuntimeStatus
	for _, inv := range m.invocations {
		if inv.createdAt.Before(since) {
			continue
		}
		status := runtimeapp.RunRuntimeStatus{
			InvocationID: inv.id,
			RunID:        inv.runID,
			WorkItemID:   inv.workItemID,
			ActionID:     inv.actionID,
			Status:       inv.status,
			CreatedAt:    inv.createdAt,
		}
		if inv.result != nil {
			status.Result = inv.result
		}
		if inv.err != nil {
			status.Error = inv.err.Error()
		}
		out = append(out, status)
	}
	return out, nil
}

// ProbeRun sends a side-channel probe to a currently running local run.
func (m *LocalSessionManager) ProbeRun(ctx context.Context, req runtimeapp.RunProbeRuntimeRequest) (*runtimeapp.RunProbeRuntimeResult, error) {
	m.mu.Lock()
	var handle *localHandle
	for _, candidate := range m.handles {
		if candidate.runID == req.RunID {
			handle = candidate
			break
		}
	}
	m.mu.Unlock()

	if handle == nil {
		return &runtimeapp.RunProbeRuntimeResult{
			Reachable:  false,
			Error:      "run route is not active",
			ObservedAt: time.Now().UTC(),
		}, nil
	}

	return probeacp.Run(ctx, probeacp.Target{
		Launch:     handle.launch,
		Caps:       handle.caps,
		WorkDir:    handle.workDir,
		MCPServers: handle.mcpServers,
		SessionID:  handle.sessionID,
		Question:   req.Question,
		Timeout:    req.Timeout,
	})
}

// Release marks a session handle as no longer active.
func (m *LocalSessionManager) Release(ctx context.Context, handle *runtimeapp.SessionHandle) error {
	m.mu.Lock()
	lh, ok := m.handles[handle.ID]
	if ok {
		delete(m.handles, handle.ID)
	}
	m.mu.Unlock()

	if !ok || lh == nil {
		return nil
	}
	if !lh.reuse && lh.standalone != nil {
		return lh.standalone.Close(ctx)
	}
	return nil
}

// CleanupWorkItem releases all sessions for a work item.
func (m *LocalSessionManager) CleanupWorkItem(workItemID int64) {
	if m.pool != nil {
		m.pool.CleanupWorkItem(workItemID)
	}
}

// DrainActive blocks until all in-flight runs complete.
func (m *LocalSessionManager) DrainActive(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		m.drainWg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ActiveCount returns the number of invocations with runs in flight.
func (m *LocalSessionManager) ActiveCount() int {
	return int(m.activeCount.Load())
}

// Close shuts down all sessions.
func (m *LocalSessionManager) Close() {
	if m.pool != nil {
		m.pool.Close()
	}
	m.mu.Lock()
	for id, lh := range m.handles {
		if lh.standalone != nil {
			_ = lh.standalone.Close(context.Background())
		}
		delete(m.handles, id)
	}
	m.mu.Unlock()
}

// eventCollector captures events for a local invocation record.
type eventCollector struct {
	inv *localInvocation
	mu  *sync.Mutex
}

func (c *eventCollector) HandleSessionUpdate(ctx context.Context, update acpclient.SessionUpdate) error {
	c.mu.Lock()
	c.inv.events = append(c.inv.events, localInvocationEvent{
		seq:    int64(len(c.inv.events) + 1),
		update: update,
	})
	c.mu.Unlock()
	return nil
}
