package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/acpclient"
	"github.com/yoke233/ai-workflow/internal/core"
)

// acpSessionEntry holds a live ACP client and session for cross-stage reuse.
type acpSessionEntry struct {
	client    *acpclient.Client
	sessionID acpproto.SessionId
}

// acpPoolKey builds the session pool key for a given run and stage.
func acpPoolKey(runID string, stage core.StageID) string {
	return runID + ":" + string(stage)
}

// acpPoolGet retrieves a cached ACP session for the given run+stage.
func (e *Executor) acpPoolGet(runID string, stage core.StageID) *acpSessionEntry {
	e.acpPoolMu.Lock()
	defer e.acpPoolMu.Unlock()
	return e.acpPool[acpPoolKey(runID, stage)]
}

// acpPoolPut stores an ACP session in the pool for later reuse.
func (e *Executor) acpPoolPut(runID string, stage core.StageID, entry *acpSessionEntry) {
	e.acpPoolMu.Lock()
	defer e.acpPoolMu.Unlock()
	e.acpPool[acpPoolKey(runID, stage)] = entry
}

// acpPoolGetSessionID returns the ACP session ID for a given run+stage, or "".
func (e *Executor) acpPoolGetSessionID(runID string, stage core.StageID) string {
	entry := e.acpPoolGet(runID, stage)
	if entry != nil {
		return string(entry.sessionID)
	}
	return ""
}

// GetStageSessionStatus checks whether an ACP session for the given run+stage is alive.
func (e *Executor) GetStageSessionStatus(runID string, stage core.StageID) core.StageSessionStatus {
	entry := e.acpPoolGet(runID, stage)
	if entry != nil {
		return core.StageSessionStatus{Alive: true, SessionID: string(entry.sessionID)}
	}
	return core.StageSessionStatus{Alive: false}
}

// WakeStageSession re-spawns an ACP agent for the given run+stage and creates a new session.
// It returns the new session ID. The run must still have a valid worktree.
func (e *Executor) WakeStageSession(ctx context.Context, runID string, stage core.StageID) (string, error) {
	// Check if already alive.
	if entry := e.acpPoolGet(runID, stage); entry != nil {
		return string(entry.sessionID), nil
	}

	// Load run to get worktree path and stage config.
	p, err := e.store.GetRun(runID)
	if err != nil {
		return "", fmt.Errorf("get run: %w", err)
	}
	if p == nil {
		return "", fmt.Errorf("run %s not found", runID)
	}
	if p.WorktreePath == "" {
		return "", fmt.Errorf("run %s has no worktree (may have been cleaned up)", runID)
	}

	// Find stage config.
	var stageCfg *core.StageConfig
	for i := range p.Stages {
		if p.Stages[i].Name == stage {
			stageCfg = &p.Stages[i]
			break
		}
	}
	if stageCfg == nil {
		return "", fmt.Errorf("stage %s not found in run %s", stage, runID)
	}

	roleName := strings.TrimSpace(stageCfg.Role)
	if roleName == "" {
		return "", fmt.Errorf("stage %s has no role configured", stage)
	}
	if e.roleResolver == nil {
		return "", fmt.Errorf("role resolver is not configured")
	}
	agentProfile, roleProfile, err := e.roleResolver.Resolve(roleName)
	if err != nil {
		return "", fmt.Errorf("resolve role %q: %w", roleName, err)
	}
	if e.acpHandlerFactory == nil {
		return "", fmt.Errorf("acp handler factory is not configured")
	}

	launchCfg := acpclient.LaunchConfig{
		Command: strings.TrimSpace(agentProfile.LaunchCommand),
		Args:    append([]string(nil), agentProfile.LaunchArgs...),
		WorkDir: p.WorktreePath,
		Env:     cloneStringMapForEngine(agentProfile.Env),
	}

	bridge := &stageEventBridge{
		executor:  e,
		runID:     p.ID,
		stage:     stage,
		agentName: agentProfile.ID,
	}
	bridge.lastActivity.Store(time.Now().UnixNano())

	handler := e.acpHandlerFactory.NewHandler(p.WorktreePath, e.bus)
	e.acpHandlerFactory.SetPermissionPolicy(handler, roleProfile.PermissionPolicy)

	acpOpts := []acpclient.Option{
		acpclient.WithEventHandler(bridge),
	}
	client, err := acpclient.New(launchCfg, handler, acpOpts...)
	if err != nil {
		return "", fmt.Errorf("create acp client: %w", err)
	}

	if err := client.Initialize(ctx, roleProfile.Capabilities); err != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = client.Close(closeCtx)
		cancel()
		return "", fmt.Errorf("acp initialize: %w", err)
	}

	var effectiveMCPServers []acpproto.McpServer
	if e.mcpServerResolver != nil {
		effectiveMCPServers = e.mcpServerResolver(roleProfile, client.SupportsSSEMCP())
	}
	session, err := client.NewSession(ctx, acpproto.NewSessionRequest{
		Cwd:        p.WorktreePath,
		McpServers: effectiveMCPServers,
	})
	if err != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = client.Close(closeCtx)
		cancel()
		return "", fmt.Errorf("acp new session: %w", err)
	}

	entry := &acpSessionEntry{client: client, sessionID: session}
	e.acpPoolPut(p.ID, stage, entry)
	if e.logger != nil {
		e.logger.Info("acp stage session woken",
			"run_id", p.ID, "stage", stage, "session_id", string(session))
	}
	return string(session), nil
}

// PromptStageSession sends a message to an existing ACP stage session.
// The session must be alive in the pool (call WakeStageSession first if needed).
func (e *Executor) PromptStageSession(ctx context.Context, runID string, stage core.StageID, message string) error {
	entry := e.acpPoolGet(runID, stage)
	if entry == nil {
		return fmt.Errorf("no active session for run %s stage %s", runID, stage)
	}

	p, err := e.store.GetRun(runID)
	if err != nil {
		return fmt.Errorf("get run: %w", err)
	}
	if p == nil {
		return fmt.Errorf("run %s not found", runID)
	}

	var stageCfg *core.StageConfig
	agentName := ""
	for i := range p.Stages {
		if p.Stages[i].Name == stage {
			stageCfg = &p.Stages[i]
			if resolved, _, resolveErr := e.resolveStageAgentName(stageCfg); resolveErr == nil {
				agentName = resolved
			}
			break
		}
	}
	if stageCfg == nil {
		return fmt.Errorf("stage %s not found in run %s", stage, runID)
	}

	bridge := &stageEventBridge{
		executor:  e,
		runID:     runID,
		stage:     stage,
		agentName: agentName,
	}
	bridge.lastActivity.Store(time.Now().UnixNano())

	return e.promptACPSession(ctx, entry, p, stageCfg, agentName, message, bridge)
}

// acpPoolCleanup closes and removes all pooled sessions for a given run.
func (e *Executor) acpPoolCleanup(runID string) {
	e.acpPoolMu.Lock()
	var toClose []string
	var entries []*acpSessionEntry
	for key, entry := range e.acpPool {
		if strings.HasPrefix(key, runID+":") {
			toClose = append(toClose, key)
			entries = append(entries, entry)
			delete(e.acpPool, key)
		}
	}
	e.acpPoolMu.Unlock()

	if len(entries) > 0 && e.logger != nil {
		e.logger.Info("acp pool cleanup", "run_id", runID, "sessions", len(entries))
	}
	for i, entry := range entries {
		if entry.client == nil {
			continue
		}
		if e.logger != nil {
			e.logger.Info("acp pool closing session", "key", toClose[i])
		}
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = entry.client.Close(closeCtx)
		cancel()
	}
}

// runACPStage executes a pipeline stage via ACP protocol.
// If stage.ReuseSessionFrom is set, it reuses the ACP client+session from that source stage.
func (e *Executor) runACPStage(
	ctx context.Context,
	agentName string,
	agentProfile acpclient.AgentProfile,
	roleProfile acpclient.RoleProfile,
	p *core.Run,
	stage *core.StageConfig,
	prompt string,
) error {
	bridge := &stageEventBridge{
		executor:  e,
		runID:     p.ID,
		stage:     stage.Name,
		agentName: agentName,
	}
	bridge.lastActivity.Store(time.Now().UnixNano())

	stageCtx := ctx
	var cancel context.CancelFunc

	if stage.IdleTimeout > 0 {
		stageCtx, cancel = startIdleChecker(ctx, &bridge.lastActivity, stage.IdleTimeout, e.logger, map[string]string{
			"run_id": p.ID,
			"stage":  string(stage.Name),
		})
		defer cancel()
	} else if stage.Timeout > 0 {
		stageCtx, cancel = context.WithTimeout(ctx, stage.Timeout)
		defer cancel()
	}

	// Try to reuse a pooled session from a previous stage.
	if source := stage.ReuseSessionFrom; source != "" {
		entry := e.acpPoolGet(p.ID, source)
		if entry != nil {
			if e.logger != nil {
				e.logger.Info("acp session reuse",
					"run_id", p.ID, "stage", stage.Name, "source_stage", source,
					"session_id", string(entry.sessionID))
			}
			return e.promptACPSession(stageCtx, entry, p, stage, agentName, prompt, bridge)
		}
		// Source session not found — fall through to create a new one.
		if e.logger != nil {
			e.logger.Warn("acp session pool miss, creating new session",
				"run_id", p.ID, "stage", stage.Name, "source", source)
		}
	}

	// Create a new ACP client + session.
	if e.acpHandlerFactory == nil {
		return fmt.Errorf("acp handler factory is not configured for stage %s", stage.Name)
	}

	launchCfg := acpclient.LaunchConfig{
		Command: strings.TrimSpace(agentProfile.LaunchCommand),
		Args:    append([]string(nil), agentProfile.LaunchArgs...),
		WorkDir: p.WorktreePath,
		Env:     cloneStringMapForEngine(agentProfile.Env),
	}
	handler := e.acpHandlerFactory.NewHandler(p.WorktreePath, e.bus)
	e.acpHandlerFactory.SetPermissionPolicy(handler, roleProfile.PermissionPolicy)

	acpOpts := []acpclient.Option{
		acpclient.WithEventHandler(bridge),
	}
	client, err := acpclient.New(launchCfg, handler, acpOpts...)
	if err != nil {
		return fmt.Errorf("create acp client for stage %s: %w", stage.Name, err)
	}

	if err := client.Initialize(stageCtx, roleProfile.Capabilities); err != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = client.Close(closeCtx)
		cancel()
		return fmt.Errorf("acp initialize for stage %s: %w", stage.Name, err)
	}

	var effectiveMCPServers []acpproto.McpServer
	if e.mcpServerResolver != nil {
		effectiveMCPServers = e.mcpServerResolver(roleProfile, client.SupportsSSEMCP())
	}
	session, err := client.NewSession(stageCtx, acpproto.NewSessionRequest{
		Cwd:        p.WorktreePath,
		McpServers: effectiveMCPServers,
	})
	if err != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = client.Close(closeCtx)
		cancel()
		return fmt.Errorf("acp new session for stage %s: %w", stage.Name, err)
	}

	entry := &acpSessionEntry{client: client, sessionID: session}

	// Always pool the session — it will be cleaned up at run end or reused by a later stage.
	e.acpPoolPut(p.ID, stage.Name, entry)
	if e.logger != nil {
		e.logger.Info("acp session created and pooled",
			"run_id", p.ID, "stage", stage.Name, "session_id", string(session))
	}

	return e.promptACPSession(stageCtx, entry, p, stage, agentName, prompt, bridge)
}

// promptACPSession sends a prompt to an existing ACP session and publishes the result.
func (e *Executor) promptACPSession(
	ctx context.Context,
	entry *acpSessionEntry,
	p *core.Run,
	stage *core.StageConfig,
	agentName string,
	prompt string,
	bridge *stageEventBridge,
) error {
	// Update the event bridge for the current stage.
	bridge.stage = stage.Name

	result, err := entry.client.Prompt(ctx, acpproto.PromptRequest{
		SessionId: entry.sessionID,
		Prompt: []acpproto.ContentBlock{
			{Text: &acpproto.ContentBlockText{Text: prompt}},
		},
	})
	if err != nil {
		return fmt.Errorf("acp prompt for stage %s: %w", stage.Name, err)
	}

	// Flush any remaining accumulated chunks before the done event.
	bridge.flushPending(ctx)

	replyText := ""
	if result != nil {
		replyText = strings.TrimSpace(result.Text)
	}
	e.bus.Publish(ctx, core.Event{
		Type:  core.EventAgentOutput,
		RunID: p.ID,
		Stage: stage.Name,
		Agent: agentName,
		Data: map[string]string{
			"type":    "done",
			"content": replyText,
		},
		Timestamp: time.Now(),
	})
	return nil
}

// stageEventBridge converts ACP session updates to EventAgentOutput events.
// Chunk types are accumulated and flushed as complete content; tool_call events
// are stored individually; usage is tracked and included in the final done event.
type stageEventBridge struct {
	executor      *Executor
	runID         string
	stage         core.StageID
	agentName     string
	lastActivity atomic.Int64 // unix nano, updated on every HandleSessionUpdate

	mu             sync.Mutex
	pendingThought strings.Builder
	pendingMessage strings.Builder
}

func (b *stageEventBridge) HandleSessionUpdate(ctx context.Context, update acpclient.SessionUpdate) error {
	b.lastActivity.Store(time.Now().UnixNano())

	// Flush accumulated chunks whenever the incoming type differs.
	// This ensures thought/message boundaries are preserved regardless
	// of what event type follows — no need to enumerate every case.
	switch update.Type {
	case "agent_thought_chunk":
		b.flushMessage(ctx)
	case "agent_message_chunk":
		b.flushThought(ctx)
	default:
		b.flushPending(ctx)
	}

	switch update.Type {
	case "agent_thought_chunk":
		b.mu.Lock()
		b.pendingThought.WriteString(update.Text)
		b.mu.Unlock()
		b.publishChunk(ctx, update)

	case "agent_message_chunk":
		b.mu.Lock()
		b.pendingMessage.WriteString(update.Text)
		b.mu.Unlock()
		b.publishChunk(ctx, update)

	case "tool_call":
		b.publishToolCall(ctx, update)

	case "tool_call_update":
		if update.Status == "completed" {
			b.publishToolCallCompleted(ctx, update)
		}

	case "usage_update":
		b.publishUsageUpdate(ctx, update)

	default:
		b.publishChunk(ctx, update)
	}

	return nil
}

// publishChunk sends a streaming event for WS broadcast (persister will skip it).
func (b *stageEventBridge) publishChunk(ctx context.Context, update acpclient.SessionUpdate) {
	if update.Text == "" {
		return
	}
	b.executor.bus.Publish(ctx, core.Event{
		Type:  core.EventAgentOutput,
		RunID: b.runID,
		Stage: b.stage,
		Agent: b.agentName,
		Data: map[string]string{
			"content": update.Text,
			"type":    update.Type,
		},
		Timestamp: time.Now(),
	})
}

func (b *stageEventBridge) flushPending(ctx context.Context) {
	b.flushThought(ctx)
	b.flushMessage(ctx)
}

func (b *stageEventBridge) flushThought(ctx context.Context) {
	b.mu.Lock()
	thought := b.pendingThought.String()
	b.pendingThought.Reset()
	b.mu.Unlock()
	if thought != "" {
		b.executor.bus.Publish(ctx, core.Event{
			Type:  core.EventAgentOutput,
			RunID: b.runID,
			Stage: b.stage,
			Agent: b.agentName,
			Data: map[string]string{
				"type":    "agent_thought",
				"content": thought,
			},
			Timestamp: time.Now(),
		})
	}
}

func (b *stageEventBridge) flushMessage(ctx context.Context) {
	b.mu.Lock()
	message := b.pendingMessage.String()
	b.pendingMessage.Reset()
	b.mu.Unlock()
	if message != "" {
		b.executor.bus.Publish(ctx, core.Event{
			Type:  core.EventAgentOutput,
			RunID: b.runID,
			Stage: b.stage,
			Agent: b.agentName,
			Data: map[string]string{
				"type":    "agent_message",
				"content": message,
			},
			Timestamp: time.Now(),
		})
	}
}

func (b *stageEventBridge) publishToolCall(ctx context.Context, update acpclient.SessionUpdate) {
	data := map[string]string{"type": "tool_call"}
	var parsed struct {
		Title      string `json:"title"`
		ToolCallID string `json:"toolCallId"`
	}
	if json.Unmarshal([]byte(update.RawUpdateJSON), &parsed) == nil {
		if parsed.Title != "" {
			data["content"] = parsed.Title
		}
		if parsed.ToolCallID != "" {
			data["tool_call_id"] = parsed.ToolCallID
		}
	}
	b.executor.bus.Publish(ctx, core.Event{
		Type:  core.EventAgentOutput,
		RunID: b.runID,
		Stage: b.stage,
		Agent: b.agentName,
		Data:  data,
		Timestamp: time.Now(),
	})
}

func (b *stageEventBridge) publishToolCallCompleted(ctx context.Context, update acpclient.SessionUpdate) {
	data := map[string]string{"type": "tool_call_completed"}
	var parsed struct {
		ToolCallID string `json:"toolCallId"`
		RawOutput  struct {
			ExitCode int    `json:"exit_code"`
			Stdout   string `json:"stdout"`
			Stderr   string `json:"stderr"`
		} `json:"rawOutput"`
	}
	if json.Unmarshal([]byte(update.RawUpdateJSON), &parsed) == nil {
		data["tool_call_id"] = parsed.ToolCallID
		data["exit_code"] = fmt.Sprintf("%d", parsed.RawOutput.ExitCode)
		stdout := parsed.RawOutput.Stdout
		if len(stdout) > 2000 {
			stdout = stdout[:2000] + "...(truncated)"
		}
		data["content"] = stdout
		if parsed.RawOutput.Stderr != "" {
			stderr := parsed.RawOutput.Stderr
			if len(stderr) > 2000 {
				stderr = stderr[:2000] + "...(truncated)"
			}
			data["stderr"] = stderr
		}
	}
	b.executor.bus.Publish(ctx, core.Event{
		Type:  core.EventAgentOutput,
		RunID: b.runID,
		Stage: b.stage,
		Agent: b.agentName,
		Data:  data,
		Timestamp: time.Now(),
	})
}


func (b *stageEventBridge) publishUsageUpdate(ctx context.Context, update acpclient.SessionUpdate) {
	data := map[string]string{"type": "usage_update"}
	var usage struct {
		Size int64 `json:"size"`
		Used int64 `json:"used"`
	}
	if json.Unmarshal([]byte(update.RawUpdateJSON), &usage) == nil {
		data["usage_size"] = fmt.Sprintf("%d", usage.Size)
		data["usage_used"] = fmt.Sprintf("%d", usage.Used)
	}
	b.executor.bus.Publish(ctx, core.Event{
		Type:  core.EventAgentOutput,
		RunID: b.runID,
		Stage: b.stage,
		Agent: b.agentName,
		Data:  data,
		Timestamp: time.Now(),
	})
}

// startIdleChecker starts a background goroutine that cancels the returned context
// when lastActivity has not been updated for longer than idleTimeout.
func startIdleChecker(
	parent context.Context,
	lastActivity *atomic.Int64,
	idleTimeout time.Duration,
	logger *slog.Logger,
	logMeta map[string]string,
) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)

	checkInterval := idleTimeout / 5
	if checkInterval < 10*time.Millisecond {
		checkInterval = 10 * time.Millisecond
	}
	if checkInterval > 30*time.Second {
		checkInterval = 30 * time.Second
	}

	go func() {
		ticker := time.NewTicker(checkInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				last := time.Unix(0, lastActivity.Load())
				if time.Since(last) > idleTimeout {
					if logger != nil {
						logger.Warn("acp stage idle timeout",
							"idle_duration", time.Since(last),
							"run_id", logMeta["run_id"],
							"stage", logMeta["stage"])
					}
					cancel()
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return ctx, cancel
}
