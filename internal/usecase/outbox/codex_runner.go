package outbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type CodexRunMode string

const (
	CodexRunCoding CodexRunMode = "coding"
	CodexRunReview CodexRunMode = "review"
	CodexRunTest   CodexRunMode = "test"
)

type CodexRunInput struct {
	Mode       CodexRunMode `json:"mode"`
	Role       string       `json:"role,omitempty"`
	ProjectDir string       `json:"project_dir"`
	PromptFile string       `json:"prompt_file"`
	IssueRef   string       `json:"issue_ref"`
	RunID      string       `json:"run_id"`
}

type CodexRunOutput struct {
	Status     string `json:"status"`
	Summary    string `json:"summary"`
	ResultCode string `json:"result_code"`
	Commit     string `json:"commit"`
	Evidence   string `json:"evidence"`
}

type codexRunner interface {
	Run(context.Context, CodexRunInput) (CodexRunOutput, error)
}

type noopCodexRunner struct{}

func newDefaultCodexRunner() codexRunner {
	return noopCodexRunner{}
}

func (noopCodexRunner) Run(_ context.Context, _ CodexRunInput) (CodexRunOutput, error) {
	return CodexRunOutput{}, errors.New("codex runner is not configured")
}

// ConfigureCodexRunnerWithWorkflow binds a workflow-backed codex runner.
func (s *Service) ConfigureCodexRunnerWithWorkflow(workflowFile string) {
	if s == nil {
		return
	}
	s.codexRunner = newWorkflowCodexRunner(workflowFile)
}

type workflowCodexRunner struct {
	workflowFile string
}

const defaultCodexExecutorTimeout = 1800

func newWorkflowCodexRunner(workflowFile string) codexRunner {
	resolved := strings.TrimSpace(workflowFile)
	if resolved == "" {
		resolved = "workflow.toml"
	}
	return workflowCodexRunner{workflowFile: resolved}
}

func (r workflowCodexRunner) Run(ctx context.Context, input CodexRunInput) (CodexRunOutput, error) {
	if err := ctx.Err(); err != nil {
		return CodexRunOutput{}, err
	}

	profile, err := loadWorkflowProfile(r.workflowFile)
	if err != nil {
		return CodexRunOutput{}, err
	}

	role := resolveCodexRole(input)
	executor := resolveExecutor(profile, role)
	runCtx, cancel := withExecutorTimeout(ctx, executor.TimeoutSeconds)
	defer cancel()

	cmd := exec.CommandContext(runCtx, executor.Program, executor.Args...)
	projectDir := strings.TrimSpace(input.ProjectDir)
	if projectDir == "" {
		projectDir = "."
	}
	cmd.Dir = projectDir
	cmd.Env = append(os.Environ(),
		"ZG_CODEX_MODE="+strings.TrimSpace(string(input.Mode)),
		"ZG_CODEX_ROLE="+role,
		"ZG_CODEX_ISSUE_REF="+strings.TrimSpace(input.IssueRef),
		"ZG_CODEX_PROMPT_FILE="+strings.TrimSpace(input.PromptFile),
		"ZG_CODEX_RUN_ID="+strings.TrimSpace(input.RunID),
		"ZG_RUN_ID="+strings.TrimSpace(input.RunID),
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if runErr != nil && errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		return CodexRunOutput{
			Status:     "fail",
			Summary:    "codex executor timed out",
			ResultCode: "env_unavailable",
			Evidence:   defaultCodexEvidence(input.Mode, input.RunID),
		}, nil
	}
	raw := strings.TrimSpace(stdout.String())
	if raw != "" {
		parsed, parseErr := parseCodexResult(raw)
		if parseErr == nil {
			return normalizeCodexRunOutput(parsed, input, runErr, stderr.String()), nil
		}
		if runErr == nil {
			return CodexRunOutput{}, fmt.Errorf("parse codex result: %w", parseErr)
		}
	}

	if runErr != nil {
		summary := firstLine(strings.TrimSpace(stderr.String()))
		if summary == "" {
			summary = runErr.Error()
		}
		return CodexRunOutput{
			Status:     "fail",
			Summary:    summary,
			ResultCode: defaultCodexFailureCode(input.Mode),
			Evidence:   defaultCodexEvidence(input.Mode, input.RunID),
		}, nil
	}

	return CodexRunOutput{
		Status:     "pass",
		Summary:    defaultCodexSuccessSummary(input.Mode),
		ResultCode: "none",
		Evidence:   defaultCodexEvidence(input.Mode, input.RunID),
	}, nil
}

func withExecutorTimeout(ctx context.Context, timeoutSeconds int) (context.Context, context.CancelFunc) {
	effective := timeoutSeconds
	if effective <= 0 {
		effective = defaultCodexExecutorTimeout
	}
	return context.WithTimeout(ctx, time.Duration(effective)*time.Second)
}

func parseCodexResult(raw string) (CodexRunOutput, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return CodexRunOutput{}, errors.New("codex result is empty")
	}

	var out CodexRunOutput
	if err := json.Unmarshal([]byte(trimmed), &out); err != nil {
		return CodexRunOutput{}, err
	}
	return out, nil
}

func resolveCodexRole(input CodexRunInput) string {
	role := strings.TrimSpace(input.Role)
	if role != "" {
		return role
	}
	switch input.Mode {
	case CodexRunReview:
		return "reviewer"
	case CodexRunTest:
		return "qa"
	default:
		return "backend"
	}
}

func normalizeCodexRunOutput(out CodexRunOutput, input CodexRunInput, runErr error, stderr string) CodexRunOutput {
	status := strings.ToLower(strings.TrimSpace(out.Status))
	if status == "" {
		if runErr == nil {
			status = "pass"
		} else {
			status = "fail"
		}
	}
	out.Status = status

	if strings.TrimSpace(out.Summary) == "" {
		if status == "pass" {
			out.Summary = defaultCodexSuccessSummary(input.Mode)
		} else {
			summary := firstLine(strings.TrimSpace(stderr))
			if summary == "" && runErr != nil {
				summary = runErr.Error()
			}
			if summary == "" {
				summary = "codex step failed"
			}
			out.Summary = summary
		}
	}

	if strings.TrimSpace(out.ResultCode) == "" {
		if status == "pass" {
			out.ResultCode = "none"
		} else {
			out.ResultCode = defaultCodexFailureCode(input.Mode)
		}
	}

	if strings.TrimSpace(out.Evidence) == "" {
		out.Evidence = defaultCodexEvidence(input.Mode, input.RunID)
	}

	return out
}

func defaultCodexSuccessSummary(mode CodexRunMode) string {
	switch mode {
	case CodexRunReview:
		return "review approved"
	case CodexRunTest:
		return "tests passed"
	default:
		return "coding completed"
	}
}

func defaultCodexFailureCode(mode CodexRunMode) string {
	switch mode {
	case CodexRunReview:
		return "review_changes_requested"
	case CodexRunTest:
		return "ci_failed"
	default:
		return "manual_intervention"
	}
}

func defaultCodexEvidence(mode CodexRunMode, runID string) string {
	normalizedMode := strings.TrimSpace(string(mode))
	if normalizedMode == "" {
		normalizedMode = "unknown"
	}
	normalizedRunID := strings.TrimSpace(runID)
	if normalizedRunID == "" {
		return "codex://" + normalizedMode
	}
	return "codex://" + normalizedMode + "/" + normalizedRunID
}

func firstLine(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	parts := strings.SplitN(trimmed, "\n", 2)
	return strings.TrimSpace(parts[0])
}
