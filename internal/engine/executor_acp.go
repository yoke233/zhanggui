package engine

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
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
		effectiveMCPServers = e.mcpServerResolver(roleProfile)
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
			"content": replyText,
			"type":    "done",
		},
		Timestamp: time.Now(),
	})
	return nil
}

// stageEventBridge converts ACP session updates to EventAgentOutput events.
type stageEventBridge struct {
	executor     *Executor
	runID        string
	stage        core.StageID
	agentName    string
	lastActivity atomic.Int64 // unix nano, updated on every HandleSessionUpdate
}

func (b *stageEventBridge) HandleSessionUpdate(ctx context.Context, update acpclient.SessionUpdate) error {
	b.lastActivity.Store(time.Now().UnixNano())
	if update.Text == "" {
		return nil
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
	return nil
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
