package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/zhanggui/internal/adapters/agent/acpclient"
	eventbridge "github.com/yoke233/zhanggui/internal/adapters/events/bridge"
	httpx "github.com/yoke233/zhanggui/internal/adapters/http/server"
	flowapp "github.com/yoke233/zhanggui/internal/application/flow"
	runtimeapp "github.com/yoke233/zhanggui/internal/application/runtime"
	"github.com/yoke233/zhanggui/internal/audit"
	"github.com/yoke233/zhanggui/internal/core"
	"github.com/yoke233/zhanggui/internal/skills"
)

// ACPExecutorConfig configures the ACP action executor.
type ACPExecutorConfig struct {
	Registry                 core.AgentRegistry
	Store                    Store
	Bus                      core.EventBus
	DefaultWorkDir           string
	MCPResolver              func(profileID string, agentSupportsSSE bool) []acpproto.McpServer
	SessionManager           runtimeapp.SessionManager
	ReworkFollowupTemplate   string
	ContinueFollowupTemplate string
	TokenRegistry            *httpx.TokenRegistry
	ServerAddr               string // e.g. "http://127.0.0.1:8080"
	AuditLogger              *audit.Logger

	// ActionContextBuilder generates per-run reference materials.
	// When nil, action-context is not injected (graceful degradation).
	ActionContextBuilder *skills.ActionContextBuilder
}

// NewACPActionExecutor creates an ActionExecutor that uses a SessionManager for ACP action runs.
// It resolves an action to an AgentProfile via the AgentRegistry, acquires a session,
// starts the run, watches for completion, then stores the result.
func NewACPActionExecutor(cfg ACPExecutorConfig) flowapp.ActionExecutor {
	return func(ctx context.Context, action *core.Action, run *core.Run) error {
		execCtx, cancel := context.WithTimeout(ctx, resolveACPActionTimeout(action))
		defer cancel()

		if cfg.SessionManager == nil {
			return fmt.Errorf("session manager is not configured")
		}

		profile, err := resolveActionAgent(execCtx, cfg.Registry, action)
		if err != nil {
			return fmt.Errorf("resolve agent for action %d: %w", action.ID, err)
		}
		run.AgentID = profile.ID

		workDir := resolveActionWorkDir(cfg.DefaultWorkDir)
		if ws := flowapp.WorkspaceFromContext(ctx); ws != nil {
			workDir = ws.Path
		}
		if v, ok := action.Config["work_dir"].(string); ok && v != "" {
			workDir = v
		}

		// Generate scoped token and build extra env vars for the agent process.
		var hasSignalSkill bool
		var scopedToken string
		extraEnv := map[string]string{}
		if cfg.TokenRegistry != nil && cfg.ServerAddr != "" &&
			(action.Type == core.ActionExec || action.Type == core.ActionGate) {
			scope := fmt.Sprintf("action:%d", action.ID)
			tok, err := cfg.TokenRegistry.GenerateScopedToken(
				fmt.Sprintf("agent-action-%d", action.ID),
				[]string{scope},
				fmt.Sprintf("agent/run-%d", run.ID),
			)
			if err != nil {
				slog.Warn("action-signal: failed to generate token", "action_id", action.ID, "error", err)
			} else {
				scopedToken = tok
				hasSignalSkill = true
				// Inject env vars — agent reads $AI_WORKFLOW_* in SKILL.md
				extraEnv["AI_WORKFLOW_API_TOKEN"] = tok
				extraEnv["AI_WORKFLOW_SERVER_ADDR"] = cfg.ServerAddr
				extraEnv["AI_WORKFLOW_ACTION_ID"] = fmt.Sprintf("%d", action.ID)
				extraEnv["AI_WORKFLOW_WORK_ITEM_ID"] = fmt.Sprintf("%d", action.WorkItemID)
				extraEnv["AI_WORKFLOW_ACTION_TYPE"] = string(action.Type)
				extraEnv["AI_WORKFLOW_RUN_ID"] = fmt.Sprintf("%d", run.ID)
				slog.Info("action-signal: env vars injected",
					"action_id", action.ID, "action_type", action.Type)
			}
		}

		launchCfg, err := acpclient.PrepareLaunch(execCtx, acpclient.BootstrapConfig{
			Profile:  profile,
			WorkDir:  workDir,
			ExtraEnv: extraEnv,
		})
		if err != nil {
			return fmt.Errorf("prepare launch config: %w", err)
		}

		var extraSkills []string
		if hasSignalSkill {
			extraSkills = []string{"action-signal"}
		}

		// --- action-context: progressive loading ---
		var actionContextDir string
		var ephemeralSkills map[string]string
		if cfg.ActionContextBuilder != nil &&
			(action.Type == core.ActionExec || action.Type == core.ActionGate) {

			ctxParentDir := filepath.Join(workDir, ".ai-workflow", "action-contexts")
			dir, buildErr := cfg.ActionContextBuilder.Build(execCtx, ctxParentDir, action, run)
			if buildErr != nil {
				slog.Warn("action-context: build failed, proceeding without",
					"action_id", action.ID, "error", buildErr)
			} else if dir != "" {
				actionContextDir = dir
				extraSkills = append(extraSkills, "action-context")
				ephemeralSkills = map[string]string{"action-context": actionContextDir}
				slog.Info("action-context: materials prepared",
					"action_id", action.ID, "dir", dir)
			}
		}

		defer func() {
			if actionContextDir != "" {
				skills.Cleanup(actionContextDir)
			}
		}()

		bridge := eventbridge.New(cfg.Bus, core.EventRunAgentOutput, eventbridge.Scope{
			WorkItemID: action.WorkItemID,
			ActionID:   action.ID,
			RunID:      run.ID,
		})
		var auditSink runtimeapp.EventSink
		if cfg.AuditLogger != nil {
			auditSink = cfg.AuditLogger.NewRunSink(audit.Scope{
				WorkItemID: action.WorkItemID,
				ActionID:   action.ID,
				RunID:      run.ID,
			})
		}
		sink := newMultiSink(bridge, auditSink)

		acpCaps := acpclient.InitCapabilities(profile)

		reuse := profile.Session.Reuse
		mcpFactory := buildActionMCPFactory(action, profile, run.ID, cfg.MCPResolver)
		publishRunAudit(execCtx, cfg.Bus, cfg.AuditLogger, action, run, "session.acquire", "started", map[string]any{
			"agent_id":      profile.ID,
			"session_reuse": reuse,
			"work_dir":      workDir,
		})

		handle, err := cfg.SessionManager.Acquire(execCtx, runtimeapp.SessionAcquireInput{
			Profile:         profile,
			Launch:          launchCfg,
			Caps:            acpCaps,
			WorkDir:         workDir,
			MCPFactory:      mcpFactory,
			WorkItemID:      action.WorkItemID,
			ActionID:        action.ID,
			RunID:           run.ID,
			Reuse:           reuse,
			IdleTTL:         profile.Session.IdleTTL,
			MaxTurns:        profile.Session.MaxTurns,
			ExtraSkills:     extraSkills,
			EphemeralSkills: ephemeralSkills,
		})
		if err != nil {
			publishRunAudit(execCtx, cfg.Bus, cfg.AuditLogger, action, run, "session.acquire", "failed", map[string]any{
				"agent_id": profile.ID,
				"error":    err.Error(),
			})
			return fmt.Errorf("acquire session: %w", err)
		}
		publishRunAudit(execCtx, cfg.Bus, cfg.AuditLogger, action, run, "session.acquire", "succeeded", map[string]any{
			"agent_id":         profile.ID,
			"agent_context_id": derefInt64(handle.AgentContextID),
			"has_prior_turns":  handle.HasPriorTurns,
		})
		defer func() {
			if scopedToken != "" && cfg.TokenRegistry != nil {
				cfg.TokenRegistry.RemoveToken(scopedToken)
			}
			if !reuse {
				_ = cfg.SessionManager.Release(context.Background(), handle)
			}
		}()

		if handle.AgentContextID != nil {
			run.AgentContextID = handle.AgentContextID
		}

		feedback := flowapp.ResolveLatestFeedback(execCtx, cfg.Store, action)
		hasActionContext := actionContextDir != ""
		runInput := flowapp.BuildRunInputForAction(profile, run.BriefingSnapshot, action, handle.HasPriorTurns, feedback, cfg.ReworkFollowupTemplate, cfg.ContinueFollowupTemplate, hasActionContext)

		// Persist the full run input for auditability.
		run.Input = buildRunInputRecord(runInput, profile, workDir, hasSignalSkill, hasActionContext, action)

		publishRunAudit(execCtx, cfg.Bus, cfg.AuditLogger, action, run, "run.dispatch", "started", map[string]any{
			"agent_id":    profile.ID,
			"input_chars": len(runInput),
		})
		invocationID, err := cfg.SessionManager.StartRun(execCtx, handle, runInput)
		if err != nil {
			publishRunAudit(execCtx, cfg.Bus, cfg.AuditLogger, action, run, "run.dispatch", "failed", map[string]any{
				"error": err.Error(),
			})
			return fmt.Errorf("start run: %w", err)
		}
		publishRunAudit(execCtx, cfg.Bus, cfg.AuditLogger, action, run, "run.dispatch", "succeeded", map[string]any{
			"invocation_id": invocationID,
		})

		publishRunAudit(execCtx, cfg.Bus, cfg.AuditLogger, action, run, "run.watch", "started", map[string]any{
			"invocation_id": invocationID,
		})
		result, err := cfg.SessionManager.WatchRun(execCtx, invocationID, 0, sink)
		if err != nil {
			publishRunAudit(execCtx, cfg.Bus, cfg.AuditLogger, action, run, "run.watch", "failed", map[string]any{
				"invocation_id": invocationID,
				"error":         err.Error(),
			})
			return fmt.Errorf("watch run: %w", err)
		}
		watchAuditData := map[string]any{
			"invocation_id": invocationID,
		}
		if result != nil {
			watchAuditData["stop_reason"] = result.StopReason
			watchAuditData["output_chars"] = len(strings.TrimSpace(result.Text))
		}
		publishRunAudit(execCtx, cfg.Bus, cfg.AuditLogger, action, run, "run.watch", "completed", watchAuditData)
		if result != nil && result.AgentContextID != nil {
			run.AgentContextID = result.AgentContextID
		}

		// Flush any remaining accumulated chunks.
		bridge.FlushPending(context.Background())

		// Publish done event with full reply.
		replyText := strings.TrimSpace(result.Text)
		bridge.PublishData(context.Background(), map[string]any{
			"type":    "done",
			"content": replyText,
		})

		// Store agent output inline on the Run.
		run.ResultMarkdown = replyText
		if action.Type == core.ActionGate {
			run.ResultMetadata = extractGateMetadata(replyText)
		}
		publishRunAudit(execCtx, cfg.Bus, cfg.AuditLogger, action, run, "deliverable.persist", "succeeded", map[string]any{
			"result_chars": len(replyText),
		})

		// Fallback: if agent couldn't curl (network-isolated sandbox),
		// extract signal from output text and create ActionSignal internally.
		if hasSignalSkill {
			tryFallbackSignal(context.Background(), cfg.Store, cfg.Bus, cfg.AuditLogger, action, run, replyText, profile.ID)
		}

		run.Output = map[string]any{
			"text":        replyText,
			"stop_reason": result.StopReason,
		}
		if result.InputTokens > 0 || result.OutputTokens > 0 {
			run.Output["input_tokens"] = result.InputTokens
			run.Output["output_tokens"] = result.OutputTokens
		}

		// Persist structured usage record for analytics.
		// Cache and reasoning tokens are breakdowns within input/output, not additive.
		totalTokens := result.InputTokens + result.OutputTokens
		if totalTokens > 0 {
			var durationMs int64
			if run.StartedAt != nil {
				// Approximate duration from run start to now.
				durationMs = time.Since(*run.StartedAt).Milliseconds()
			}
			var projectID *int64
			if workItem, fErr := cfg.Store.GetWorkItem(execCtx, action.WorkItemID); fErr == nil && workItem.ProjectID != nil {
				projectID = workItem.ProjectID
			}
			usageRec := &core.UsageRecord{
				RunID:            run.ID,
				WorkItemID:       action.WorkItemID,
				ActionID:         action.ID,
				ProjectID:        projectID,
				AgentID:          profile.ID,
				ProfileID:        profile.ID,
				ModelID:          result.ModelID,
				InputTokens:      result.InputTokens,
				OutputTokens:     result.OutputTokens,
				CacheReadTokens:  result.CacheReadTokens,
				CacheWriteTokens: result.CacheWriteTokens,
				ReasoningTokens:  result.ReasoningTokens,
				TotalTokens:      totalTokens,
				DurationMs:       durationMs,
			}
			if _, uErr := cfg.Store.CreateUsageRecord(execCtx, usageRec); uErr != nil {
				slog.Warn("failed to persist usage record",
					"run_id", run.ID, "error", uErr)
			}
		}

		slog.Info("runtime ACP action executed",
			"action_id", action.ID, "agent", profile.ID,
			"output_len", len(replyText),
			"stop_reason", result.StopReason,
			"input_tokens", result.InputTokens,
			"output_tokens", result.OutputTokens)

		return nil
	}
}

func resolveActionWorkDir(defaultWorkDir string) string {
	if trimmed := strings.TrimSpace(defaultWorkDir); trimmed != "" {
		return trimmed
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	if trimmed := strings.TrimSpace(cwd); trimmed != "" {
		return trimmed
	}
	return "."
}

func resolveACPActionTimeout(action *core.Action) time.Duration {
	if action != nil && action.Timeout > 0 {
		return action.Timeout
	}
	return 120 * time.Second
}

// resolveActionAgent resolves the agent profile for an action.
// It first checks action.Config["profile_id"] for an explicit profile assignment,
// then falls back to ResolveForAction (role + capabilities matching).
func resolveActionAgent(ctx context.Context, registry core.AgentRegistry, action *core.Action) (*core.AgentProfile, error) {
	if pid, ok := action.Config["profile_id"].(string); ok && pid != "" {
		p, err := registry.ResolveByID(ctx, pid)
		if err == nil {
			return p, nil
		}
		slog.Warn("resolve agent: explicit profile_id not found, falling back",
			"profile_id", pid, "action_id", action.ID, "error", err)
	}
	return registry.ResolveForAction(ctx, action)
}

func buildActionMCPFactory(action *core.Action, profile *core.AgentProfile, runID int64, resolver func(profileID string, agentSupportsSSE bool) []acpproto.McpServer) func(agentSupportsSSE bool) []acpproto.McpServer {
	if resolver == nil || action == nil || profile == nil || !profile.MCP.Enabled {
		return nil
	}
	// MCP tools should only be exposed while running concrete actions.
	if action.Type != core.ActionExec && action.Type != core.ActionGate {
		return nil
	}
	return func(agentSupportsSSE bool) []acpproto.McpServer {
		servers := resolver(profile.ID, agentSupportsSSE)
		slog.Debug("mcp: resolved servers",
			"profile", profile.ID, "action_id", action.ID,
			"action_type", action.Type, "run_id", runID,
			"server_count", len(servers))
		// Inject action context env vars into internal stdio MCP servers (mcp-serve).
		actionEnv := []acpproto.EnvVariable{
			{Name: "AI_WORKFLOW_ACTION_ID", Value: fmt.Sprintf("%d", action.ID)},
			{Name: "AI_WORKFLOW_WORK_ITEM_ID", Value: fmt.Sprintf("%d", action.WorkItemID)},
			{Name: "AI_WORKFLOW_ACTION_TYPE", Value: string(action.Type)},
			{Name: "AI_WORKFLOW_RUN_ID", Value: fmt.Sprintf("%d", runID)},
		}
		for i := range servers {
			if servers[i].Stdio != nil && containsArg(servers[i].Stdio.Args, "mcp-serve") {
				servers[i].Stdio.Env = append(servers[i].Stdio.Env, actionEnv...)
				slog.Debug("mcp: injected action env into mcp-serve",
					"action_id", action.ID, "env_count", len(servers[i].Stdio.Env))
			}
		}
		return servers
	}
}

func containsArg(args []string, target string) bool {
	for _, a := range args {
		if a == target {
			return true
		}
	}
	return false
}

var reGateJSONLine = regexp.MustCompile(`(?m)^AI_WORKFLOW_GATE_JSON:\s*(\{.*\})\s*$`)

// reSignalLine matches the unified fallback signal line:
//
//	AI_WORKFLOW_SIGNAL: {"decision":"complete|need_help|approve|reject","reason":"..."}
var reSignalLine = regexp.MustCompile(`(?m)^AI_WORKFLOW_SIGNAL:\s*(\{.*\})\s*$`)

// extractGateMetadata parses a deterministic JSON line emitted by the reviewer agent.
// Expected format (single line):
//
//	AI_WORKFLOW_GATE_JSON: {"verdict":"pass"|"reject","reason":"...","reject_targets":[1,2,3]}
func extractGateMetadata(markdown string) map[string]any {
	m := reGateJSONLine.FindAllStringSubmatch(markdown, -1)
	if len(m) == 0 {
		return map[string]any{"verdict": "pass"}
	}
	raw := strings.TrimSpace(m[len(m)-1][1])
	if raw == "" {
		return map[string]any{"verdict": "pass"}
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return map[string]any{"verdict": "reject", "reason": "invalid gate json"}
	}
	verdict, _ := parsed["verdict"].(string)
	verdict = strings.ToLower(strings.TrimSpace(verdict))
	if verdict != "reject" {
		verdict = "pass"
	}
	parsed["verdict"] = verdict
	return parsed
}

func publishRunAudit(ctx context.Context, bus core.EventBus, auditLogger *audit.Logger, action *core.Action, run *core.Run, kind string, status string, data map[string]any) {
	if action == nil || run == nil {
		return
	}
	payload := map[string]any{
		"kind":   strings.TrimSpace(kind),
		"status": strings.TrimSpace(status),
	}
	for k, v := range data {
		payload[k] = v
	}
	if auditLogger != nil {
		if logRef := auditLogger.LogRunAudit(ctx, audit.Scope{
			WorkItemID: action.WorkItemID,
			ActionID:   action.ID,
			RunID:      run.ID,
		}, kind, status, data); strings.TrimSpace(logRef) != "" {
			payload["log_ref"] = logRef
		}
	}
	if bus == nil {
		return
	}
	bus.Publish(ctx, core.Event{
		Type:       core.EventRunAudit,
		WorkItemID: action.WorkItemID,
		ActionID:   action.ID,
		RunID:      run.ID,
		Timestamp:  time.Now().UTC(),
		Data:       payload,
	})
}

func derefInt64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

// outputSignal is the parsed result of an AI_WORKFLOW_SIGNAL line from agent output.
type outputSignal struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
	Payload  map[string]any
}

// parseOutputSignal extracts a structured signal from agent output text.
// Returns nil if no valid signal line is found.
func parseOutputSignal(text string) *outputSignal {
	matches := reSignalLine.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}
	// Use the last match (agent may have retried).
	raw := strings.TrimSpace(matches[len(matches)-1][1])
	if raw == "" {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil
	}
	var sig outputSignal
	sig.Decision, _ = payload["decision"].(string)
	sig.Reason, _ = payload["reason"].(string)
	delete(payload, "decision")
	delete(payload, "reason")
	sig.Payload = payload
	sig.Decision = strings.ToLower(strings.TrimSpace(sig.Decision))
	switch sig.Decision {
	case "complete", "need_help", "approve", "reject":
		return &sig
	default:
		return nil
	}
}

// decisionToSignalType maps a decision string to a core.SignalType.
func decisionToSignalType(decision string) (core.SignalType, bool) {
	switch decision {
	case "complete":
		return core.SignalComplete, true
	case "need_help":
		return core.SignalNeedHelp, true
	case "approve":
		return core.SignalApprove, true
	case "reject":
		return core.SignalReject, true
	default:
		return "", false
	}
}

// tryFallbackSignal checks whether a terminal signal was already received via HTTP.
// If not, it parses the agent output for an AI_WORKFLOW_SIGNAL line and creates the
// ActionSignal internally. This covers network-isolated environments where the agent
// cannot curl the decision endpoint.
func tryFallbackSignal(ctx context.Context, store core.ActionSignalStore, bus core.EventBus, auditLogger *audit.Logger, action *core.Action, run *core.Run, replyText, profileID string) {
	// Check if a terminal signal was already received via HTTP curl.
	existing, _ := store.GetLatestActionSignal(ctx, action.ID,
		core.SignalComplete, core.SignalNeedHelp, core.SignalApprove, core.SignalReject)
	if existing != nil {
		return // signal already received via HTTP
	}

	parsed := parseOutputSignal(replyText)
	if parsed == nil {
		return // no fallback signal in output
	}

	sigType, ok := decisionToSignalType(parsed.Decision)
	if !ok {
		return
	}

	sig := &core.ActionSignal{
		ActionID:   action.ID,
		WorkItemID: action.WorkItemID,
		RunID:      run.ID,
		Type:       sigType,
		Source:     core.SignalSourceAgent,
		Summary:    parsed.Reason,
		Payload:    buildFallbackSignalPayload(parsed),
		Actor:      fmt.Sprintf("agent/%s", profileID),
	}
	sigID, err := store.CreateActionSignal(ctx, sig)
	if err != nil {
		slog.Warn("action-signal: failed to create fallback signal",
			"action_id", action.ID, "decision", parsed.Decision, "error", err)
		return
	}

	bus.Publish(ctx, core.Event{
		Type:       core.EventActionSignal,
		WorkItemID: action.WorkItemID,
		ActionID:   action.ID,
		Timestamp:  time.Now().UTC(),
		Data: map[string]any{
			"signal_id": sigID,
			"type":      string(sigType),
			"source":    "agent",
			"method":    "output_fallback",
		},
	})
	publishRunAudit(ctx, bus, auditLogger, action, run, "signal.fallback", "created", map[string]any{
		"signal_id":   sigID,
		"signal_type": string(sigType),
		"actor":       fmt.Sprintf("agent/%s", profileID),
	})
	slog.Info("action-signal: created from output fallback",
		"action_id", action.ID, "decision", parsed.Decision, "signal_id", sigID)
}

func buildFallbackSignalPayload(parsed *outputSignal) map[string]any {
	payload := map[string]any{
		"reason": parsed.Reason,
		"source": "output_fallback",
	}
	if parsed == nil {
		return payload
	}
	for k, v := range parsed.Payload {
		payload[k] = v
	}
	return payload
}

// buildRunInputRecord captures the full context sent to the agent for auditability.
func buildRunInputRecord(prompt string, profile *core.AgentProfile, workDir string, hasSignalSkill bool, hasActionContext bool, action *core.Action) map[string]any {
	rec := map[string]any{
		"prompt":   prompt,
		"work_dir": workDir,
	}

	// Agent identity
	if profile != nil {
		rec["profile_id"] = profile.ID
		rec["profile_name"] = profile.Name
		rec["role"] = profile.Role

		caps := profile.EffectiveCapabilities()
		rec["capabilities"] = map[string]any{
			"fs_read":  caps.FSRead,
			"fs_write": caps.FSWrite,
			"terminal": caps.Terminal,
		}

		rec["actions_allowed"] = profile.ActionsAllowed
		rec["session_reuse"] = profile.Session.Reuse
		rec["session_max_turns"] = profile.Session.MaxTurns

		if profile.MCP.Enabled {
			rec["mcp_enabled"] = true
			rec["mcp_tools"] = profile.MCP.Tools
		}

		rec["launch_command"] = profile.Driver.LaunchCommand
		rec["launch_args"] = profile.Driver.LaunchArgs
	}

	// Skills injected
	var injectedSkills []string
	if hasSignalSkill {
		injectedSkills = append(injectedSkills, "action-signal")
	}
	if hasActionContext {
		injectedSkills = append(injectedSkills, "action-context")
	}
	if len(injectedSkills) > 0 {
		rec["skills_injected"] = injectedSkills
	}

	// Action config (objective, profile_id, etc.)
	if action != nil && len(action.Config) > 0 {
		rec["action_config"] = action.Config
	}

	return rec
}
