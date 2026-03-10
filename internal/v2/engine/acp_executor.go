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
	"github.com/yoke233/ai-workflow/internal/teamleader"
	"github.com/yoke233/ai-workflow/internal/v2/core"
)

// ACPExecutorConfig configures the ACP step executor.
type ACPExecutorConfig struct {
	Registry       core.AgentRegistry
	Store          core.Store
	Bus            core.EventBus
	DefaultWorkDir string
	MCPEnv         teamleader.MCPEnvConfig
}

// NewACPStepExecutor creates a StepExecutor that spawns ACP agent processes.
// It resolves step → AgentProfile + AgentDriver via the AgentRegistry, then runs:
//
//	spawn process → initialize → new session → prompt (briefing) → collect output → close.
func NewACPStepExecutor(cfg ACPExecutorConfig) StepExecutor {
	return func(ctx context.Context, step *core.Step, exec *core.Execution) error {
		profile, driver, err := cfg.Registry.ResolveForStep(ctx, step)
		if err != nil {
			return fmt.Errorf("resolve agent for step %d: %w", step.ID, err)
		}
		exec.AgentID = profile.ID

		workDir := cfg.DefaultWorkDir
		if ws := WorkspaceFromContext(ctx); ws != nil {
			workDir = ws.Path
		}
		if v, ok := step.Config["work_dir"].(string); ok && v != "" {
			workDir = v
		}

		launchCfg := acpclient.LaunchConfig{
			Command: driver.LaunchCommand,
			Args:    driver.LaunchArgs,
			WorkDir: workDir,
			Env:     driver.Env,
		}

		bridge := NewEventBridge(cfg.Bus, core.EventExecAgentOutput, EventBridgeScope{
			FlowID: step.FlowID,
			StepID: step.ID,
			ExecID: exec.ID,
		})

		caps := profile.EffectiveCapabilities()
		acpCaps := acpclient.ClientCapabilities{
			FSRead:   caps.FSRead,
			FSWrite:  caps.FSWrite,
			Terminal: caps.Terminal,
		}

		client, err := acpclient.New(launchCfg, &acpclient.NopHandler{},
			acpclient.WithEventHandler(bridge))
		if err != nil {
			return fmt.Errorf("launch ACP agent %q: %w", driver.ID, err)
		}
		defer client.Close(context.Background())

		if err := client.Initialize(ctx, acpCaps); err != nil {
			return fmt.Errorf("initialize ACP agent %q: %w", driver.ID, err)
		}

		var mcpServers []acpproto.McpServer
		if profile.MCP.Enabled {
			roleProfile := acpclient.RoleProfile{
				ID:         profile.ID,
				MCPEnabled: true,
				MCPTools:   append([]string(nil), profile.MCP.Tools...),
			}
			mcpServers = teamleader.MCPToolsFromRoleConfig(roleProfile, cfg.MCPEnv, client.SupportsSSEMCP())
		}

		sessionID, err := client.NewSession(ctx, acpproto.NewSessionRequest{
			Cwd:        workDir,
			McpServers: mcpServers,
		})
		if err != nil {
			return fmt.Errorf("create ACP session: %w", err)
		}

		prompt := buildPromptFromBriefing(exec.BriefingSnapshot, step)

		result, err := client.Prompt(ctx, acpproto.PromptRequest{
			SessionId: sessionID,
			Prompt: []acpproto.ContentBlock{
				{Text: &acpproto.ContentBlockText{Text: prompt}},
			},
		})
		if err != nil {
			return fmt.Errorf("ACP prompt failed: %w", err)
		}

		// Flush any remaining accumulated chunks.
		bridge.FlushPending(ctx)

		// Publish done event with full reply.
		replyText := ""
		if result != nil {
			replyText = strings.TrimSpace(result.Text)
		}
		bridge.PublishData(ctx, map[string]any{
			"type":    "done",
			"content": replyText,
		})

		// Store agent output as an Artifact.
		art := &core.Artifact{
			ExecutionID:    exec.ID,
			StepID:         step.ID,
			FlowID:         step.FlowID,
			ResultMarkdown: replyText,
		}
		artID, err := cfg.Store.CreateArtifact(ctx, art)
		if err != nil {
			return fmt.Errorf("store artifact: %w", err)
		}
		exec.ArtifactID = &artID
		exec.Output = map[string]any{
			"text":        replyText,
			"stop_reason": string(result.StopReason),
		}
		if result.Usage != nil {
			exec.Output["input_tokens"] = result.Usage.InputTokens
			exec.Output["output_tokens"] = result.Usage.OutputTokens
		}

		slog.Info("v2 ACP step executed",
			"step_id", step.ID, "agent", profile.ID,
			"output_len", len(replyText),
			"stop_reason", result.StopReason)

		return nil
	}
}

// buildPromptFromBriefing constructs the prompt text sent to the ACP agent.
func buildPromptFromBriefing(snapshot string, step *core.Step) string {
	var sb strings.Builder
	sb.WriteString("# Task\n\n")
	sb.WriteString(snapshot)

	if len(step.AcceptanceCriteria) > 0 {
		sb.WriteString("\n\n# Acceptance Criteria\n\n")
		for _, c := range step.AcceptanceCriteria {
			sb.WriteString("- ")
			sb.WriteString(c)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// ---------------------------------------------------------------------------
// EventBridge — chunk aggregation, tool call extraction, usage tracking
// ---------------------------------------------------------------------------

// EventBridgeScope identifies the context of events published by the bridge.
type EventBridgeScope struct {
	FlowID    int64
	StepID    int64
	ExecID    int64
	SessionID string // used for chat events (no flow/step/exec)
}

// EventBridge converts ACP SessionUpdate events into v2 core.Event published on the bus.
// Chunk types are accumulated and flushed as complete content on type boundaries.
// Matches v1 stageEventBridge semantics.
type EventBridge struct {
	bus       core.EventBus
	eventType core.EventType
	scope     EventBridgeScope

	lastActivity atomic.Int64 // unix nano

	mu             sync.Mutex
	pendingThought strings.Builder
	pendingMessage strings.Builder
}

// NewEventBridge creates an EventBridge for publishing ACP events.
func NewEventBridge(bus core.EventBus, eventType core.EventType, scope EventBridgeScope) *EventBridge {
	b := &EventBridge{
		bus:       bus,
		eventType: eventType,
		scope:     scope,
	}
	b.lastActivity.Store(time.Now().UnixNano())
	return b
}

// LastActivity returns the time of the last ACP event received.
func (b *EventBridge) LastActivity() time.Time {
	return time.Unix(0, b.lastActivity.Load())
}

// HandleSessionUpdate implements acpclient.EventHandler.
func (b *EventBridge) HandleSessionUpdate(ctx context.Context, update acpclient.SessionUpdate) error {
	b.lastActivity.Store(time.Now().UnixNano())

	// Flush accumulated chunks when the incoming type differs (preserves boundaries).
	switch update.Type {
	case "agent_thought_chunk":
		b.flushMessage(ctx)
	case "agent_message_chunk":
		b.flushThought(ctx)
	default:
		b.FlushPending(ctx)
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

// FlushPending flushes all accumulated thought and message chunks.
func (b *EventBridge) FlushPending(ctx context.Context) {
	b.flushThought(ctx)
	b.flushMessage(ctx)
}

// PublishData publishes an event with arbitrary data (used for done, prompt events).
func (b *EventBridge) PublishData(ctx context.Context, data map[string]any) {
	b.publish(ctx, data)
}

func (b *EventBridge) flushThought(ctx context.Context) {
	b.mu.Lock()
	thought := b.pendingThought.String()
	b.pendingThought.Reset()
	b.mu.Unlock()
	if thought != "" {
		b.publish(ctx, map[string]any{
			"type":    "agent_thought",
			"content": thought,
		})
	}
}

func (b *EventBridge) flushMessage(ctx context.Context) {
	b.mu.Lock()
	message := b.pendingMessage.String()
	b.pendingMessage.Reset()
	b.mu.Unlock()
	if message != "" {
		b.publish(ctx, map[string]any{
			"type":    "agent_message",
			"content": message,
		})
	}
}

// publishChunk sends a streaming event for WS broadcast (persister will skip it).
func (b *EventBridge) publishChunk(ctx context.Context, update acpclient.SessionUpdate) {
	if update.Text == "" {
		return
	}
	b.publish(ctx, map[string]any{
		"type":    update.Type,
		"content": update.Text,
	})
}

func (b *EventBridge) publishToolCall(ctx context.Context, update acpclient.SessionUpdate) {
	data := map[string]any{"type": "tool_call"}
	var parsed struct {
		Title      string `json:"title"`
		ToolCallID string `json:"toolCallId"`
	}
	if json.Unmarshal(update.RawJSON, &parsed) == nil {
		if parsed.Title != "" {
			data["content"] = parsed.Title
		}
		if parsed.ToolCallID != "" {
			data["tool_call_id"] = parsed.ToolCallID
		}
	}
	b.publish(ctx, data)
}

func (b *EventBridge) publishToolCallCompleted(ctx context.Context, update acpclient.SessionUpdate) {
	data := map[string]any{"type": "tool_call_completed"}
	var parsed struct {
		ToolCallID string `json:"toolCallId"`
		RawOutput  struct {
			ExitCode int    `json:"exit_code"`
			Stdout   string `json:"stdout"`
			Stderr   string `json:"stderr"`
		} `json:"rawOutput"`
	}
	if json.Unmarshal(update.RawJSON, &parsed) == nil {
		data["tool_call_id"] = parsed.ToolCallID
		data["exit_code"] = parsed.RawOutput.ExitCode
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
	b.publish(ctx, data)
}

func (b *EventBridge) publishUsageUpdate(ctx context.Context, update acpclient.SessionUpdate) {
	data := map[string]any{"type": "usage_update"}
	var usage struct {
		Size int64 `json:"size"`
		Used int64 `json:"used"`
	}
	if json.Unmarshal(update.RawJSON, &usage) == nil {
		data["usage_size"] = usage.Size
		data["usage_used"] = usage.Used
	}
	b.publish(ctx, data)
}

func (b *EventBridge) publish(ctx context.Context, data map[string]any) {
	ev := core.Event{
		Type:      b.eventType,
		FlowID:    b.scope.FlowID,
		StepID:    b.scope.StepID,
		ExecID:    b.scope.ExecID,
		Data:      data,
		Timestamp: time.Now().UTC(),
	}
	if b.scope.SessionID != "" {
		if ev.Data == nil {
			ev.Data = map[string]any{}
		}
		ev.Data["session_id"] = b.scope.SessionID
	}
	b.bus.Publish(ctx, ev)
}
