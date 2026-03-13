package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"os"
	"path/filepath"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
	eventbridge "github.com/yoke233/ai-workflow/internal/adapters/events/bridge"
	httpx "github.com/yoke233/ai-workflow/internal/adapters/http/server"
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
	TokenRegistry            *httpx.TokenRegistry
	ServerAddr               string // e.g. "http://127.0.0.1:8080"
}

// NewACPStepExecutor creates a StepExecutor that uses a SessionManager for ACP agent execution.
// It resolves step → AgentProfile + AgentDriver via the AgentRegistry, acquires a session,
// starts the execution, watches for completion, then stores the result.
func NewACPStepExecutor(cfg ACPExecutorConfig) flowapp.StepExecutor {
	return func(ctx context.Context, step *core.Step, exec *core.Execution) error {
		if cfg.SessionManager == nil {
			return fmt.Errorf("session manager is not configured")
		}

		profile, driver, err := resolveStepAgent(ctx, cfg.Registry, step)
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

		// Generate scoped token and write step-signal skill into workspace.
		var scopedToken string
		if cfg.TokenRegistry != nil && cfg.ServerAddr != "" && workDir != "" &&
			(step.Type == core.StepExec || step.Type == core.StepGate) {
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
				if err := writeStepSignalSkill(workDir, driver.ID, cfg.ServerAddr, tok, step, exec.ID); err != nil {
					slog.Warn("step-signal: failed to write skill", "step_id", step.ID, "error", err)
				} else {
					slog.Info("step-signal: skill written to workspace",
						"step_id", step.ID, "step_type", step.Type, "work_dir", workDir)
				}
			}
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
		mcpFactory := buildStepMCPFactory(step, profile.ID, exec.ID, cfg.MCPResolver)

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
		executionInput := flowapp.BuildExecutionInputForStep(profile, exec.BriefingSnapshot, step, handle.HasPriorTurns, feedback, cfg.ReworkFollowupTemplate, cfg.ContinueFollowupTemplate)

		// Persist the full execution input for auditability.
		exec.Input = buildExecutionInputRecord(executionInput, profile, driver, workDir, scopedToken != "", step)

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

// resolveStepAgent resolves the agent profile and driver for a step.
// It first checks step.Config["profile_id"] for an explicit profile assignment,
// then falls back to ResolveForStep (role + capabilities matching).
func resolveStepAgent(ctx context.Context, registry core.AgentRegistry, step *core.Step) (*core.AgentProfile, *core.AgentDriver, error) {
	if pid, ok := step.Config["profile_id"].(string); ok && pid != "" {
		p, d, err := registry.ResolveByID(ctx, pid)
		if err == nil {
			return p, d, nil
		}
		slog.Warn("resolve agent: explicit profile_id not found, falling back",
			"profile_id", pid, "step_id", step.ID, "error", err)
	}
	return registry.ResolveForStep(ctx, step)
}

func buildStepMCPFactory(step *core.Step, profileID string, execID int64, resolver func(profileID string, agentSupportsSSE bool) []acpproto.McpServer) func(agentSupportsSSE bool) []acpproto.McpServer {
	if resolver == nil || step == nil {
		return nil
	}
	// MCP tools should only be exposed while executing concrete steps.
	if step.Type != core.StepExec && step.Type != core.StepGate {
		return nil
	}
	return func(agentSupportsSSE bool) []acpproto.McpServer {
		servers := resolver(profileID, agentSupportsSSE)
		slog.Debug("mcp: resolved servers",
			"profile", profileID, "step_id", step.ID,
			"step_type", step.Type, "exec_id", execID,
			"server_count", len(servers))
		// Inject step context env vars into internal stdio MCP servers (mcp-serve).
		stepEnv := []acpproto.EnvVariable{
			{Name: "AI_WORKFLOW_STEP_ID", Value: fmt.Sprintf("%d", step.ID)},
			{Name: "AI_WORKFLOW_ISSUE_ID", Value: fmt.Sprintf("%d", step.IssueID)},
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

// writeStepSignalSkill writes the step-signal skill into the workspace so ACP agents auto-load it.
// For codex-acp: .agents/skills/step-signal/SKILL.md
// For claude-acp: .claude/skills/step-signal/SKILL.md
func writeStepSignalSkill(workDir, driverID, serverAddr, token string, step *core.Step, execID int64) error {
	driverLower := strings.ToLower(driverID)
	var skillDir string
	switch {
	case strings.Contains(driverLower, "codex"):
		skillDir = filepath.Join(workDir, ".agents", "skills", "step-signal")
	case strings.Contains(driverLower, "claude"):
		skillDir = filepath.Join(workDir, ".claude", "skills", "step-signal")
	default:
		skillDir = filepath.Join(workDir, ".agents", "skills", "step-signal")
	}
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return fmt.Errorf("mkdir skill dir: %w", err)
	}

	content := buildStepSignalSkillContent(serverAddr, token, step, execID)
	return os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644)
}

func buildStepSignalSkillContent(serverAddr, token string, step *core.Step, execID int64) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("name: step-signal\n")
	b.WriteString("description: Signal task completion or gate decisions to the AI Workflow engine\n")
	b.WriteString("---\n\n")
	b.WriteString("# Step Signal\n\n")
	b.WriteString("You are running inside an **ai-workflow** managed step. ")
	b.WriteString("Use the HTTP API below to signal your result when you finish.\n\n")

	b.WriteString("## Your Context\n\n")
	b.WriteString(fmt.Sprintf("- **Server**: `%s`\n", serverAddr))
	b.WriteString(fmt.Sprintf("- **Step ID**: `%d`\n", step.ID))
	b.WriteString(fmt.Sprintf("- **Issue ID**: `%d`\n", step.IssueID))
	b.WriteString(fmt.Sprintf("- **Step Type**: `%s`\n", step.Type))
	b.WriteString(fmt.Sprintf("- **Execution ID**: `%d`\n\n", execID))

	baseURL := fmt.Sprintf("%s/api/steps/%d/decision", serverAddr, step.ID)
	authHeader := fmt.Sprintf(`-H "Authorization: Bearer %s"`, token)

	switch step.Type {
	case core.StepExec:
		b.WriteString("## After Completing Your Task\n\n")
		b.WriteString("**Signal completion** (do this BEFORE ending your response):\n\n")
		b.WriteString("```bash\n")
		b.WriteString(fmt.Sprintf("curl -s -X POST \"%s\" \\\n", baseURL))
		b.WriteString(fmt.Sprintf("  %s \\\n", authHeader))
		b.WriteString("  -H \"Content-Type: application/json\" \\\n")
		b.WriteString("  -d '{\"decision\":\"complete\",\"reason\":\"<one sentence summary of what you did>\"}'\n")
		b.WriteString("```\n\n")
		b.WriteString("**If you are stuck and need human help:**\n\n")
		b.WriteString("```bash\n")
		b.WriteString(fmt.Sprintf("curl -s -X POST \"%s\" \\\n", baseURL))
		b.WriteString(fmt.Sprintf("  %s \\\n", authHeader))
		b.WriteString("  -H \"Content-Type: application/json\" \\\n")
		b.WriteString("  -d '{\"decision\":\"need_help\",\"reason\":\"<why you are stuck>\"}'\n")
		b.WriteString("```\n")

	case core.StepGate:
		b.WriteString("## After Reviewing the Code\n\n")
		b.WriteString("**Approve** (review passes):\n\n")
		b.WriteString("```bash\n")
		b.WriteString(fmt.Sprintf("curl -s -X POST \"%s\" \\\n", baseURL))
		b.WriteString(fmt.Sprintf("  %s \\\n", authHeader))
		b.WriteString("  -H \"Content-Type: application/json\" \\\n")
		b.WriteString("  -d '{\"decision\":\"approve\",\"reason\":\"<why it passes review>\"}'\n")
		b.WriteString("```\n\n")
		b.WriteString("**Reject** (needs fixes):\n\n")
		b.WriteString("```bash\n")
		b.WriteString(fmt.Sprintf("curl -s -X POST \"%s\" \\\n", baseURL))
		b.WriteString(fmt.Sprintf("  %s \\\n", authHeader))
		b.WriteString("  -H \"Content-Type: application/json\" \\\n")
		b.WriteString("  -d '{\"decision\":\"reject\",\"reason\":\"<what needs fixing>\"}'\n")
		b.WriteString("```\n")
	}

	b.WriteString("\n## Rules\n\n")
	b.WriteString("1. **Always signal before ending your response.** The engine needs your signal to proceed.\n")
	b.WriteString("2. Only call the decision endpoint **once** per execution.\n")
	if step.Type == core.StepGate {
		b.WriteString("3. Also output `AI_WORKFLOW_GATE_JSON: {\"verdict\":\"pass|reject\",\"reason\":\"...\"}` as a fallback.\n")
	}

	return b.String()
}

// buildExecutionInputRecord captures the full context sent to the agent for auditability.
func buildExecutionInputRecord(prompt string, profile *core.AgentProfile, driver *core.AgentDriver, workDir string, hasSignalSkill bool, step *core.Step) map[string]any {
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
	}

	if driver != nil {
		rec["driver_id"] = driver.ID
		rec["launch_command"] = driver.LaunchCommand
		rec["launch_args"] = driver.LaunchArgs
	}

	// Skills injected
	var skills []string
	if hasSignalSkill {
		skills = append(skills, "step-signal")
	}
	if len(skills) > 0 {
		rec["skills_injected"] = skills
	}

	// Step config (objective, profile_id, etc.)
	if step != nil && len(step.Config) > 0 {
		rec["step_config"] = step.Config
	}

	return rec
}
