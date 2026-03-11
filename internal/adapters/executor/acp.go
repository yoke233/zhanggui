package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"text/template"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
	eventbridge "github.com/yoke233/ai-workflow/internal/adapters/events/bridge"
	flowapp "github.com/yoke233/ai-workflow/internal/application/flow"
	runtimeapp "github.com/yoke233/ai-workflow/internal/application/runtime"
	"github.com/yoke233/ai-workflow/internal/core"
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
}

// NewACPStepExecutor creates a StepExecutor that uses a SessionManager for ACP agent execution.
// It resolves step → AgentProfile + AgentDriver via the AgentRegistry, acquires a session,
// starts the execution, watches for completion, then stores the result.
func NewACPStepExecutor(cfg ACPExecutorConfig) flowapp.StepExecutor {
	return func(ctx context.Context, step *core.Step, exec *core.Execution) error {
		if cfg.SessionManager == nil {
			return fmt.Errorf("session manager is not configured")
		}

		profile, driver, err := cfg.Registry.ResolveForStep(ctx, step)
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
			Command: driver.LaunchCommand,
			Args:    driver.LaunchArgs,
			WorkDir: workDir,
			Env:     cloneEnv(driver.Env),
		}

		bridge := eventbridge.New(cfg.Bus, core.EventExecAgentOutput, eventbridge.Scope{
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

		reuse := profile.Session.Reuse

		handle, err := cfg.SessionManager.Acquire(ctx, runtimeapp.SessionAcquireInput{
			Profile: profile,
			Driver:  driver,
			Launch:  launchCfg,
			Caps:    acpCaps,
			WorkDir: workDir,
			MCPFactory: func(agentSupportsSSE bool) []acpproto.McpServer {
				if cfg.MCPResolver != nil {
					return cfg.MCPResolver(profile.ID, agentSupportsSSE)
				}
				return nil
			},
			FlowID:   step.FlowID,
			StepID:   step.ID,
			ExecID:   exec.ID,
			Reuse:    reuse,
			IdleTTL:  profile.Session.IdleTTL,
			MaxTurns: profile.Session.MaxTurns,
		})
		if err != nil {
			return fmt.Errorf("acquire session: %w", err)
		}
		defer func() {
			if !reuse {
				_ = cfg.SessionManager.Release(ctx, handle)
			}
		}()

		if handle.AgentContextID != nil {
			exec.AgentContextID = handle.AgentContextID
		}

		executionInput := buildExecutionInputForStep(profile, exec.BriefingSnapshot, step, handle.HasPriorTurns, cfg.ReworkFollowupTemplate, cfg.ContinueFollowupTemplate)

		invocationID, err := cfg.SessionManager.StartExecution(ctx, handle, executionInput)
		if err != nil {
			return fmt.Errorf("start execution: %w", err)
		}

		result, err := cfg.SessionManager.WatchExecution(ctx, invocationID, 0, bridge)
		if err != nil {
			return fmt.Errorf("watch execution: %w", err)
		}
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

		// Store agent output as an Artifact.
		art := &core.Artifact{
			ExecutionID:    exec.ID,
			StepID:         step.ID,
			FlowID:         step.FlowID,
			ResultMarkdown: replyText,
		}
		if step.Type == core.StepGate {
			art.Metadata = extractGateMetadata(replyText)
		}
		artID, err := cfg.Store.CreateArtifact(ctx, art)
		if err != nil {
			return fmt.Errorf("store artifact: %w", err)
		}
		exec.ArtifactID = &artID
		exec.Output = map[string]any{
			"text":        replyText,
			"stop_reason": result.StopReason,
		}
		if result.InputTokens > 0 || result.OutputTokens > 0 {
			exec.Output["input_tokens"] = result.InputTokens
			exec.Output["output_tokens"] = result.OutputTokens
		}

		slog.Info("runtime ACP step executed",
			"step_id", step.ID, "agent", profile.ID,
			"output_len", len(replyText),
			"stop_reason", result.StopReason)

		return nil
	}
}

var reGateJSONLine = regexp.MustCompile(`(?m)^AI_WORKFLOW_GATE_JSON:\s*(\{.*\})\s*$`)

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

// buildExecutionInputFromBriefing constructs the execution input sent to the ACP agent.
func buildExecutionInputFromBriefing(snapshot string, step *core.Step) string {
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

func buildExecutionInputForStep(profile *core.AgentProfile, snapshot string, step *core.Step, hasPriorTurns bool, reworkTmpl string, continueTmpl string) string {
	// For gate steps, always repeat the full instruction block to ensure deterministic output.
	if step != nil && step.Type == core.StepGate {
		return buildExecutionInputFromBriefing(snapshot, step)
	}

	feedback := latestGateFeedback(step)
	// If the agent is in a reused session and we already have prior turns, send only the incremental
	// feedback to preserve execution context caching and leverage the existing context window.
	if profile != nil && profile.Session.Reuse && hasPriorTurns {
		if feedback != "" {
			return renderFollowupExecutionMessage(reworkTmpl, followupVars{Feedback: feedback, StepName: stepName(step)})
		}
		// No explicit feedback — continue without re-sending the full base instruction block.
		return renderFollowupExecutionMessage(continueTmpl, followupVars{StepName: stepName(step)})
	}

	// Default: full base instruction block + optional feedback section.
	base := buildExecutionInputFromBriefing(snapshot, step)
	if feedback == "" {
		return base
	}
	return base + "\n\n# Gate Feedback (Rework)\n\n" + feedback + "\n"
}

func latestGateFeedback(step *core.Step) string {
	if step == nil || step.Config == nil {
		return ""
	}
	last, _ := step.Config["last_gate_feedback"].(map[string]any)
	if last == nil {
		// Fall back to the end of rework_history.
		if arr, ok := step.Config["rework_history"].([]any); ok && len(arr) > 0 {
			if m, ok := arr[len(arr)-1].(map[string]any); ok {
				last = m
			}
		}
	}
	if last == nil {
		return ""
	}
	reason, _ := last["reason"].(string)
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("Reason: ")
	sb.WriteString(reason)
	if prURL, ok := last["pr_url"].(string); ok && strings.TrimSpace(prURL) != "" {
		sb.WriteString("\nPR: ")
		sb.WriteString(strings.TrimSpace(prURL))
	}
	if n, ok := last["pr_number"]; ok {
		sb.WriteString("\nPR Number: ")
		sb.WriteString(fmt.Sprint(n))
	}
	return sb.String()
}

type followupVars struct {
	Feedback string
	StepName string
}

func stepName(step *core.Step) string {
	if step == nil {
		return ""
	}
	return strings.TrimSpace(step.Name)
}

func renderFollowupExecutionMessage(tmplText string, vars followupVars) string {
	// Safe fallback: no template provided.
	if strings.TrimSpace(tmplText) == "" {
		if strings.TrimSpace(vars.Feedback) == "" {
			if vars.StepName == "" {
				return "# Continue\n\n请继续完成当前任务（复用已有上下文）。\n"
			}
			return "# Continue\n\n请继续完成本 step（复用已有上下文）： " + vars.StepName + "\n"
		}
		if vars.StepName == "" {
			return "# Rework Requested\n\n反馈：\n" + vars.Feedback + "\n"
		}
		return "# Rework Requested\n\n(step: " + vars.StepName + ")\n\n反馈：\n" + vars.Feedback + "\n"
	}

	tmpl, err := template.New("runtime-followup").Parse(tmplText)
	if err != nil {
		// Never fail the execution due to follow-up template issues.
		slog.Warn("runtime followup execution message: invalid template", "error", err)
		return "# Rework Requested\n\n反馈：\n" + vars.Feedback + "\n"
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, vars); err != nil {
		slog.Warn("runtime followup execution message: render failed", "error", err)
		return "# Rework Requested\n\n反馈：\n" + vars.Feedback + "\n"
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "# Rework Requested\n\n反馈：\n" + vars.Feedback + "\n"
	}
	return out
}
