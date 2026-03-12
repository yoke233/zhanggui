package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

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
			IssueID: step.IssueID,
			StepID:  step.ID,
			ExecID:  exec.ID,
		})

		caps := profile.EffectiveCapabilities()
		acpCaps := acpclient.ClientCapabilities{
			FSRead:   caps.FSRead,
			FSWrite:  caps.FSWrite,
			Terminal: caps.Terminal,
		}

		reuse := profile.Session.Reuse
		mcpFactory := buildStepMCPFactory(step, profile.ID, cfg.MCPResolver)

		handle, err := cfg.SessionManager.Acquire(ctx, runtimeapp.SessionAcquireInput{
			Profile:    profile,
			Driver:     driver,
			Launch:     launchCfg,
			Caps:       acpCaps,
			WorkDir:    workDir,
			MCPFactory: mcpFactory,
			IssueID:    step.IssueID,
			StepID:     step.ID,
			ExecID:     exec.ID,
			Reuse:      reuse,
			IdleTTL:    profile.Session.IdleTTL,
			MaxTurns:   profile.Session.MaxTurns,
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

		executionInput := flowapp.BuildExecutionInputForStep(profile, exec.BriefingSnapshot, step, handle.HasPriorTurns, cfg.ReworkFollowupTemplate, cfg.ContinueFollowupTemplate)

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
			IssueID:        step.IssueID,
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
			if issue, fErr := cfg.Store.GetIssue(ctx, step.IssueID); fErr == nil && issue.ProjectID != nil {
				projectID = issue.ProjectID
			}
			usageRec := &core.UsageRecord{
				ExecutionID:      exec.ID,
				IssueID:          step.IssueID,
				StepID:           step.ID,
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

func buildStepMCPFactory(step *core.Step, profileID string, resolver func(profileID string, agentSupportsSSE bool) []acpproto.McpServer) func(agentSupportsSSE bool) []acpproto.McpServer {
	if resolver == nil || step == nil {
		return nil
	}
	// complete_step should only be exposed while executing concrete steps.
	if step.Type != core.StepExec && step.Type != core.StepGate {
		return nil
	}
	return func(agentSupportsSSE bool) []acpproto.McpServer {
		return resolver(profileID, agentSupportsSSE)
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
