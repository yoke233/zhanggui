package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
	eventbridge "github.com/yoke233/ai-workflow/internal/adapters/events/bridge"
	httpx "github.com/yoke233/ai-workflow/internal/adapters/http/server"
	flowapp "github.com/yoke233/ai-workflow/internal/application/flow"
	runtimeapp "github.com/yoke233/ai-workflow/internal/application/runtime"
	"github.com/yoke233/ai-workflow/internal/audit"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/skills"
)

// ACPExecutorConfig configures the ACP step executor.
type ACPExecutorConfig struct {
	Registry                 core.AgentRegistry
	Store                    core.Store
	Bus                      core.EventBus
	DefaultWorkDir           string
	MCPResolver              func(profileID string, agentSupportsSSE bool) []acpproto.McpServer
	SessionManager           runtimeapp.SessionManager
	ReworkFollowupTemplate   string
	ContinueFollowupTemplate string
	TokenRegistry            *httpx.TokenRegistry
	ServerAddr               string // e.g. "http://127.0.0.1:8080"
	AuditLogger              *audit.Logger

	// StepContextBuilder generates per-execution reference materials.
	// When nil, step-context is not injected (graceful degradation).
	StepContextBuilder *skills.ActionContextBuilder
}

// NewACPActionExecutor creates a ActionExecutor that uses a SessionManager for ACP agent execution.
// It resolves step → AgentProfile via the AgentRegistry, acquires a session,
// starts the execution, watches for completion, then stores the result.
func NewACPActionExecutor(cfg ACPExecutorConfig) flowapp.ActionExecutor {
	return func(ctx context.Context, step *core.Action, exec *core.Run) error {
		if cfg.SessionManager == nil {
			return fmt.Errorf("session manager is not configured")
		}

		profile, err := resolveStepAgent(ctx, cfg.Registry, step)
		if err != nil {
			return fmt.Errorf("resolve agent for step %d: %w", step.ID, err)
		}
		exec.AgentID = profile.ID

		workDir := cfg.DefaultWorkDir
		if ws := flowapp.WorkspaceFromContext(ctx); ws != nil {
			workDir = ws.Path
		}
		if v, ok := step.Config["work_dir"].(string); ok && v != "" {
			workDir = v
		}

		launchCfg := acpclient.LaunchConfig{
			Command: profile.Driver.LaunchCommand,
			Args:    profile.Driver.LaunchArgs,
			WorkDir: workDir,
			Env:     cloneEnv(profile.Driver.Env),
		}

		// Generate scoped token and inject step-signal env vars for the agent process.
		var hasSignalSkill bool
		var scopedToken string
		if cfg.TokenRegistry != nil && cfg.ServerAddr != "" &&
			(step.Type == core.ActionExec || step.Type == core.ActionGate) {
			scope := fmt.Sprintf("step:%d", step.ID)
			tok, err := cfg.TokenRegistry.GenerateScopedToken(
				fmt.Sprintf("agent-step-%d", step.ID),
				[]string{scope},
				fmt.Sprintf("agent/exec-%d", exec.ID),
			)
			if err != nil {
				slog.Warn("step-signal: failed to generate token", "step_id", step.ID, "error", err)
			} else {
				scopedToken = tok
				hasSignalSkill = true
				// Inject env vars — agent reads $AI_WORKFLOW_* in SKILL.md
				launchCfg.Env["AI_WORKFLOW_API_TOKEN"] = tok
				launchCfg.Env["AI_WORKFLOW_SERVER_ADDR"] = cfg.ServerAddr
				launchCfg.Env["AI_WORKFLOW_STEP_ID"] = fmt.Sprintf("%d", step.ID)
				launchCfg.Env["AI_WORKFLOW_ISSUE_ID"] = fmt.Sprintf("%d", step.WorkItemID)
				launchCfg.Env["AI_WORKFLOW_STEP_TYPE"] = string(step.Type)
				launchCfg.Env["AI_WORKFLOW_EXEC_ID"] = fmt.Sprintf("%d", exec.ID)
				slog.Info("step-signal: env vars injected",
					"step_id", step.ID, "step_type", step.Type)
			}
		}

		var extraSkills []string
		if hasSignalSkill {
			extraSkills = []string{"step-signal"}
		}

		// --- step-context: progressive loading ---
		var stepContextDir string
		var ephemeralSkills map[string]string
		if cfg.StepContextBuilder != nil &&
			(step.Type == core.ActionExec || step.Type == core.ActionGate) {

			ctxParentDir := filepath.Join(workDir, ".ai-workflow", "step-contexts")
			dir, buildErr := cfg.StepContextBuilder.Build(ctx, ctxParentDir, step, exec)
			if buildErr != nil {
				slog.Warn("step-context: build failed, proceeding without",
					"step_id", step.ID, "error", buildErr)
			} else if dir != "" {
				stepContextDir = dir
				extraSkills = append(extraSkills, "step-context")
				ephemeralSkills = map[string]string{"step-context": stepContextDir}
				slog.Info("step-context: materials prepared",
					"step_id", step.ID, "dir", dir)
			}
		}

		defer func() {
			if stepContextDir != "" {
				skills.Cleanup(stepContextDir)
			}
		}()

		bridge := eventbridge.New(cfg.Bus, core.EventRunAgentOutput, eventbridge.Scope{
			WorkItemID: step.WorkItemID,
			ActionID:   step.ID,
			RunID:      exec.ID,
		})
		var auditSink runtimeapp.EventSink
		if cfg.AuditLogger != nil {
			auditSink = cfg.AuditLogger.NewRunSink(audit.Scope{
				WorkItemID: step.WorkItemID,
				ActionID:   step.ID,
				RunID:      exec.ID,
			})
		}
		sink := newMultiSink(bridge, auditSink)

		caps := profile.EffectiveCapabilities()
		acpCaps := acpclient.ClientCapabilities{
			FSRead:   caps.FSRead,
			FSWrite:  caps.FSWrite,
			Terminal: caps.Terminal,
		}

		reuse := profile.Session.Reuse
		mcpFactory := buildStepMCPFactory(step, profile, exec.ID, cfg.MCPResolver)
		publishExecutionAudit(ctx, cfg.Bus, cfg.AuditLogger, step, exec, "session.acquire", "started", map[string]any{
			"agent_id":      profile.ID,
			"session_reuse": reuse,
			"work_dir":      workDir,
		})

		handle, err := cfg.SessionManager.Acquire(ctx, runtimeapp.SessionAcquireInput{
			Profile:         profile,
			Launch:          launchCfg,
			Caps:            acpCaps,
			WorkDir:         workDir,
			MCPFactory:      mcpFactory,
			IssueID:         step.WorkItemID,
			StepID:          step.ID,
			ExecID:          exec.ID,
			Reuse:           reuse,
			IdleTTL:         profile.Session.IdleTTL,
			MaxTurns:        profile.Session.MaxTurns,
			ExtraSkills:     extraSkills,
			EphemeralSkills: ephemeralSkills,
		})
		if err != nil {
			publishExecutionAudit(ctx, cfg.Bus, cfg.AuditLogger, step, exec, "session.acquire", "failed", map[string]any{
				"agent_id": profile.ID,
				"error":    err.Error(),
			})
			return fmt.Errorf("acquire session: %w", err)
		}
		publishExecutionAudit(ctx, cfg.Bus, cfg.AuditLogger, step, exec, "session.acquire", "succeeded", map[string]any{
			"agent_id":         profile.ID,
			"agent_context_id": derefInt64(handle.AgentContextID),
			"has_prior_turns":  handle.HasPriorTurns,
		})
		defer func() {
			if scopedToken != "" && cfg.TokenRegistry != nil {
				cfg.TokenRegistry.RemoveToken(scopedToken)
			}
			if !reuse {
				_ = cfg.SessionManager.Release(ctx, handle)
			}
		}()

		if handle.AgentContextID != nil {
			exec.AgentContextID = handle.AgentContextID
		}

		feedback := flowapp.ResolveLatestFeedback(ctx, cfg.Store, step)
		hasStepContext := stepContextDir != ""
		executionInput := flowapp.BuildRunInputForAction(profile, exec.BriefingSnapshot, step, handle.HasPriorTurns, feedback, cfg.ReworkFollowupTemplate, cfg.ContinueFollowupTemplate, hasStepContext)

		// Persist the full execution input for auditability.
		exec.Input = buildExecutionInputRecord(executionInput, profile, workDir, hasSignalSkill, hasStepContext, step)

		publishExecutionAudit(ctx, cfg.Bus, cfg.AuditLogger, step, exec, "execution.dispatch", "started", map[string]any{
			"agent_id":    profile.ID,
			"input_chars": len(executionInput),
		})
		invocationID, err := cfg.SessionManager.StartExecution(ctx, handle, executionInput)
		if err != nil {
			publishExecutionAudit(ctx, cfg.Bus, cfg.AuditLogger, step, exec, "execution.dispatch", "failed", map[string]any{
				"error": err.Error(),
			})
			return fmt.Errorf("start execution: %w", err)
		}
		publishExecutionAudit(ctx, cfg.Bus, cfg.AuditLogger, step, exec, "execution.dispatch", "succeeded", map[string]any{
			"invocation_id": invocationID,
		})

		publishExecutionAudit(ctx, cfg.Bus, cfg.AuditLogger, step, exec, "execution.watch", "started", map[string]any{
			"invocation_id": invocationID,
		})
		result, err := cfg.SessionManager.WatchExecution(ctx, invocationID, 0, sink)
		if err != nil {
			publishExecutionAudit(ctx, cfg.Bus, cfg.AuditLogger, step, exec, "execution.watch", "failed", map[string]any{
				"invocation_id": invocationID,
				"error":         err.Error(),
			})
			return fmt.Errorf("watch execution: %w", err)
		}
		watchAuditData := map[string]any{
			"invocation_id": invocationID,
		}
		if result != nil {
			watchAuditData["stop_reason"] = result.StopReason
			watchAuditData["output_chars"] = len(strings.TrimSpace(result.Text))
		}
		publishExecutionAudit(ctx, cfg.Bus, cfg.AuditLogger, step, exec, "execution.watch", "completed", watchAuditData)
		if result != nil && result.AgentContextID != nil {
			exec.AgentContextID = result.AgentContextID
		}

		// Flush any remaining accumulated chunks.
		bridge.FlushPending(ctx)

		// Publish done event with full reply.
		replyText := strings.TrimSpace(result.Text)
		bridge.PublishData(ctx, map[string]any{
			"type":    "done",
			"content": replyText,
		})

		// Store agent output inline on the Run.
		exec.ResultMarkdown = replyText
		if step.Type == core.ActionGate {
			exec.ResultMetadata = extractGateMetadata(replyText)
		}
		publishExecutionAudit(ctx, cfg.Bus, cfg.AuditLogger, step, exec, "deliverable.persist", "succeeded", map[string]any{
			"result_chars": len(replyText),
		})

		// Fallback: if agent couldn't curl (network-isolated sandbox),
		// extract signal from output text and create ActionSignal internally.
		if hasSignalSkill {
			tryFallbackSignal(ctx, cfg.Store, cfg.Bus, cfg.AuditLogger, step, exec, replyText, profile.ID)
		}

		exec.Output = map[string]any{
			"text":        replyText,
			"stop_reason": result.StopReason,
		}
		if result.InputTokens > 0 || result.OutputTokens > 0 {
			exec.Output["input_tokens"] = result.InputTokens
			exec.Output["output_tokens"] = result.OutputTokens
		}

		// Persist structured usage record for analytics.
		// Cache and reasoning tokens are breakdowns within input/output, not additive.
		totalTokens := result.InputTokens + result.OutputTokens
		if totalTokens > 0 {
			var durationMs int64
			if exec.StartedAt != nil {
				// Approximate duration from exec start to now.
				durationMs = time.Since(*exec.StartedAt).Milliseconds()
			}
			var projectID *int64
			if issue, fErr := cfg.Store.GetWorkItem(ctx, step.WorkItemID); fErr == nil && issue.ProjectID != nil {
				projectID = issue.ProjectID
			}
			usageRec := &core.UsageRecord{
				RunID:            exec.ID,
				WorkItemID:       step.WorkItemID,
				ActionID:         step.ID,
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
			if _, uErr := cfg.Store.CreateUsageRecord(ctx, usageRec); uErr != nil {
				slog.Warn("failed to persist usage record",
					"exec_id", exec.ID, "error", uErr)
			}
		}

		slog.Info("runtime ACP step executed",
			"step_id", step.ID, "agent", profile.ID,
			"output_len", len(replyText),
			"stop_reason", result.StopReason,
			"input_tokens", result.InputTokens,
			"output_tokens", result.OutputTokens)

		return nil
	}
}

// resolveStepAgent resolves the agent profile for a step.
// It first checks step.Config["profile_id"] for an explicit profile assignment,
// then falls back to ResolveForAction (role + capabilities matching).
func resolveStepAgent(ctx context.Context, registry core.AgentRegistry, step *core.Action) (*core.AgentProfile, error) {
	if pid, ok := step.Config["profile_id"].(string); ok && pid != "" {
		p, err := registry.ResolveByID(ctx, pid)
		if err == nil {
			return p, nil
		}
		slog.Warn("resolve agent: explicit profile_id not found, falling back",
			"profile_id", pid, "step_id", step.ID, "error", err)
	}
	return registry.ResolveForAction(ctx, step)
}

func buildStepMCPFactory(step *core.Action, profile *core.AgentProfile, execID int64, resolver func(profileID string, agentSupportsSSE bool) []acpproto.McpServer) func(agentSupportsSSE bool) []acpproto.McpServer {
	if resolver == nil || step == nil || profile == nil || !profile.MCP.Enabled {
		return nil
	}
	// MCP tools should only be exposed while executing concrete steps.
	if step.Type != core.ActionExec && step.Type != core.ActionGate {
		return nil
	}
	return func(agentSupportsSSE bool) []acpproto.McpServer {
		servers := resolver(profile.ID, agentSupportsSSE)
		slog.Debug("mcp: resolved servers",
			"profile", profile.ID, "step_id", step.ID,
			"step_type", step.Type, "exec_id", execID,
			"server_count", len(servers))
		// Inject step context env vars into internal stdio MCP servers (mcp-serve).
		stepEnv := []acpproto.EnvVariable{
			{Name: "AI_WORKFLOW_STEP_ID", Value: fmt.Sprintf("%d", step.ID)},
			{Name: "AI_WORKFLOW_ISSUE_ID", Value: fmt.Sprintf("%d", step.WorkItemID)},
			{Name: "AI_WORKFLOW_STEP_TYPE", Value: string(step.Type)},
			{Name: "AI_WORKFLOW_EXEC_ID", Value: fmt.Sprintf("%d", execID)},
		}
		for i := range servers {
			if servers[i].Stdio != nil && containsArg(servers[i].Stdio.Args, "mcp-serve") {
				servers[i].Stdio.Env = append(servers[i].Stdio.Env, stepEnv...)
				slog.Debug("mcp: injected step env into mcp-serve",
					"step_id", step.ID, "env_count", len(servers[i].Stdio.Env))
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

func cloneEnv(in map[string]string) map[string]string {
	if in == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func publishExecutionAudit(ctx context.Context, bus core.EventBus, auditLogger *audit.Logger, step *core.Action, exec *core.Run, kind string, status string, data map[string]any) {
	if step == nil || exec == nil {
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
		if logRef := auditLogger.LogExecutionAudit(ctx, audit.Scope{
			WorkItemID: step.WorkItemID,
			ActionID:   step.ID,
			RunID:      exec.ID,
		}, kind, status, data); strings.TrimSpace(logRef) != "" {
			payload["log_ref"] = logRef
		}
	}
	if bus == nil {
		return
	}
	bus.Publish(ctx, core.Event{
		Type:       core.EventExecutionAudit,
		WorkItemID: step.WorkItemID,
		ActionID:   step.ID,
		RunID:      exec.ID,
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
	var sig outputSignal
	if err := json.Unmarshal([]byte(raw), &sig); err != nil {
		return nil
	}
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
func tryFallbackSignal(ctx context.Context, store core.Store, bus core.EventBus, auditLogger *audit.Logger, step *core.Action, exec *core.Run, replyText, profileID string) {
	// Check if a terminal signal was already received via HTTP curl.
	existing, _ := store.GetLatestActionSignal(ctx, step.ID,
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
		ActionID:   step.ID,
		WorkItemID: step.WorkItemID,
		RunID:      exec.ID,
		Type:       sigType,
		Source:     core.SignalSourceAgent,
		Summary:    parsed.Reason,
		Payload:    map[string]any{"reason": parsed.Reason, "source": "output_fallback"},
		Actor:      fmt.Sprintf("agent/%s", profileID),
	}
	sigID, err := store.CreateActionSignal(ctx, sig)
	if err != nil {
		slog.Warn("step-signal: failed to create fallback signal",
			"step_id", step.ID, "decision", parsed.Decision, "error", err)
		return
	}

	bus.Publish(ctx, core.Event{
		Type:       core.EventActionSignal,
		WorkItemID: step.WorkItemID,
		ActionID:   step.ID,
		Timestamp:  time.Now().UTC(),
		Data: map[string]any{
			"signal_id": sigID,
			"type":      string(sigType),
			"source":    "agent",
			"method":    "output_fallback",
		},
	})
	publishExecutionAudit(ctx, bus, auditLogger, step, exec, "signal.fallback", "created", map[string]any{
		"signal_id":   sigID,
		"signal_type": string(sigType),
		"actor":       fmt.Sprintf("agent/%s", profileID),
	})
	slog.Info("step-signal: created from output fallback",
		"step_id", step.ID, "decision", parsed.Decision, "signal_id", sigID)
}

// buildExecutionInputRecord captures the full context sent to the agent for auditability.
func buildExecutionInputRecord(prompt string, profile *core.AgentProfile, workDir string, hasSignalSkill bool, hasStepContext bool, step *core.Action) map[string]any {
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
		injectedSkills = append(injectedSkills, "step-signal")
	}
	if hasStepContext {
		injectedSkills = append(injectedSkills, "step-context")
	}
	if len(injectedSkills) > 0 {
		rec["skills_injected"] = injectedSkills
	}

	// Step config (objective, profile_id, etc.)
	if step != nil && len(step.Config) > 0 {
		rec["step_config"] = step.Config
	}

	return rec
}
