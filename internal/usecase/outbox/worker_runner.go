package outbox

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	domainoutbox "zhanggui/internal/domain/outbox"
)

type WorkerRunInput struct {
	ContextPackDir string
	WorkflowFile   string
}

type WorkResultChanges struct {
	PR     string `json:"PR"`
	Commit string `json:"Commit"`
}

type WorkResultTests struct {
	Command  string `json:"Command"`
	Result   string `json:"Result"`
	Evidence string `json:"Evidence"`
}

type WorkResultEnvelope struct {
	IssueRef   string            `json:"IssueRef"`
	RunID      string            `json:"RunID"`
	Status     string            `json:"Status,omitempty"`
	Summary    string            `json:"Summary,omitempty"`
	ResultCode string            `json:"ResultCode,omitempty"`
	Changes    WorkResultChanges `json:"Changes"`
	Tests      WorkResultTests   `json:"Tests"`
	Source     string            `json:"-"`
}

func (r WorkResultEnvelope) toDomain() domainoutbox.WorkResult {
	return domainoutbox.WorkResult{
		IssueRef:   r.IssueRef,
		RunID:      r.RunID,
		ResultCode: r.ResultCode,
		Changes: &domainoutbox.WorkChanges{
			PR:     r.Changes.PR,
			Commit: r.Changes.Commit,
		},
		Tests: &domainoutbox.WorkTests{
			Command:  r.Tests.Command,
			Result:   r.Tests.Result,
			Evidence: r.Tests.Evidence,
		},
	}
}

func (s *Service) WorkerRun(ctx context.Context, input WorkerRunInput) error {
	if ctx == nil {
		return errors.New("context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	contextPackDir := strings.TrimSpace(input.ContextPackDir)
	if contextPackDir == "" {
		return errors.New("context pack dir is required")
	}

	workOrder, err := loadWorkOrder(filepath.Join(contextPackDir, "work_order.json"))
	if err != nil {
		return err
	}
	if err := domainoutbox.ValidateWorkOrder(workOrder); err != nil {
		return err
	}

	profile, err := loadWorkflowProfile(input.WorkflowFile)
	if err != nil {
		return err
	}
	executor := resolveExecutor(profile, workOrder.Role)

	startedAt := time.Now().UTC()
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(executor.TimeoutSeconds)*time.Second)
	defer cancel()

	commandText := strings.Join(append([]string{executor.Program}, executor.Args...), " ")
	result := WorkResultEnvelope{
		IssueRef: workOrder.IssueRef,
		RunID:    workOrder.RunID,
		Changes: WorkResultChanges{
			PR:     "none",
			Commit: resolveGitCommit(workOrder.RepoDir),
		},
		Tests: WorkResultTests{
			Command:  commandText,
			Result:   "pass",
			Evidence: "none",
		},
	}
	if strings.TrimSpace(result.Changes.Commit) == "" {
		result.Changes.Commit = "none"
	}

	cmd := exec.CommandContext(runCtx, executor.Program, executor.Args...)
	cmd.Dir = workOrder.RepoDir

	stdoutPath := filepath.Join(contextPackDir, "stdout.log")
	stderrPath := filepath.Join(contextPackDir, "stderr.log")
	workResultJSONPath := filepath.Join(contextPackDir, "work_result.json")
	workResultTextPath := filepath.Join(contextPackDir, "work_result.txt")
	workAuditPath := filepath.Join(contextPackDir, "work_audit.json")

	stdoutFile, err := os.Create(stdoutPath)
	if err != nil {
		return err
	}
	defer stdoutFile.Close()

	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		return err
	}
	defer stderrFile.Close()

	cmd.Stdout = stdoutFile
	cmd.Stderr = stderrFile

	runErr := cmd.Run()
	if runErr != nil {
		result.Tests.Result = "fail"
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			result.ResultCode = "env_unavailable"
			result.Summary = "executor timed out"
		} else {
			result.ResultCode = "test_failed"
			result.Summary = "executor failed"
		}
	} else {
		result.Summary = "executor ok"
	}

	result.Status = statusFromWorkResult(result)
	if err := writeWorkResultAuditJSON(workAuditPath, WorkAuditEnvelope{
		IssueRef:         workOrder.IssueRef,
		RunID:            workOrder.RunID,
		Role:             workOrder.Role,
		ExecutorProgram:  executor.Program,
		ExecutorArgs:     executor.Args,
		TimeoutSeconds:   executor.TimeoutSeconds,
		StartedAt:        startedAt.Format(time.RFC3339Nano),
		FinishedAt:       time.Now().UTC().Format(time.RFC3339Nano),
		ExitCode:         resolveExitCode(runErr),
		TimedOut:         errors.Is(runCtx.Err(), context.DeadlineExceeded),
		StdoutPath:       "stdout.log",
		StderrPath:       "stderr.log",
		WorkResultSource: "json+text",
		WorkResultJSON:   "work_result.json",
		WorkResultText:   "work_result.txt",
	}); err != nil {
		return err
	}

	if err := writeWorkResultJSON(workResultJSONPath, result); err != nil {
		return err
	}
	if err := writeWorkResultText(workResultTextPath, result); err != nil {
		return err
	}
	return nil
}

func loadWorkOrder(path string) (domainoutbox.WorkOrder, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return domainoutbox.WorkOrder{}, err
	}

	var order domainoutbox.WorkOrder
	if err := json.Unmarshal(raw, &order); err != nil {
		return domainoutbox.WorkOrder{}, err
	}
	return order, nil
}

func writeWorkResultJSON(path string, result WorkResultEnvelope) error {
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func writeWorkResultText(path string, result WorkResultEnvelope) error {
	summary := strings.TrimSpace(result.Summary)
	if summary == "" {
		summary = "none"
	} else if strings.Contains(summary, "\n") {
		summary = strings.TrimSpace(strings.SplitN(summary, "\n", 2)[0])
	}

	content := fmt.Sprintf(
		"IssueRef: %s\nRunId: %s\nStatus: %s\nSummary: %s\nPR: %s\nCommit: %s\nTests: %s => %s\nEvidence: %s\nResultCode: %s\n\nNotes:\n- generated by worker runner\n",
		result.IssueRef,
		result.RunID,
		statusFromWorkResult(result),
		summary,
		noneIfEmpty(result.Changes.PR),
		noneIfEmpty(result.Changes.Commit),
		noneIfEmpty(result.Tests.Command),
		noneIfEmpty(result.Tests.Result),
		noneIfEmpty(result.Tests.Evidence),
		noneIfEmpty(result.ResultCode),
	)
	return os.WriteFile(path, []byte(content), 0o644)
}

func statusFromWorkResult(result WorkResultEnvelope) string {
	if !domainoutbox.IsNoneLike(result.ResultCode) || strings.TrimSpace(result.Tests.Result) == "fail" {
		return "fail"
	}
	return "ok"
}

func resolveGitCommit(repoDir string) string {
	cmd := exec.Command("git", "-C", repoDir, "rev-parse", "HEAD")
	raw, err := cmd.Output()
	if err != nil {
		return ""
	}
	sha := strings.TrimSpace(string(raw))
	if sha == "" {
		return ""
	}
	return "git:" + sha
}

func loadWorkResultFromContextPack(contextPackDir string) (WorkResultEnvelope, error) {
	jsonPath := filepath.Join(contextPackDir, "work_result.json")
	if _, err := os.Stat(jsonPath); err == nil {
		result, err := loadWorkResultJSON(jsonPath)
		if err != nil {
			return WorkResultEnvelope{}, err
		}
		result.Source = "json"
		return result, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return WorkResultEnvelope{}, err
	}

	textPath := filepath.Join(contextPackDir, "work_result.txt")
	result, err := loadWorkResultText(textPath)
	if err != nil {
		return WorkResultEnvelope{}, err
	}
	result.Source = "text"
	return result, nil
}

func loadWorkResultJSON(path string) (WorkResultEnvelope, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return WorkResultEnvelope{}, err
	}

	var result WorkResultEnvelope
	if err := json.Unmarshal(raw, &result); err != nil {
		return WorkResultEnvelope{}, err
	}

	if strings.TrimSpace(result.IssueRef) == "" || strings.TrimSpace(result.RunID) == "" {
		var snake workResultEnvelopeSnake
		if err := json.Unmarshal(raw, &snake); err != nil {
			return WorkResultEnvelope{}, err
		}
		candidate := snake.toEnvelope()
		if strings.TrimSpace(candidate.IssueRef) != "" && strings.TrimSpace(candidate.RunID) != "" {
			result = candidate
		}
	}
	if strings.TrimSpace(result.IssueRef) == "" || strings.TrimSpace(result.RunID) == "" {
		return WorkResultEnvelope{}, errors.New("work result json missing issue_ref/run_id")
	}
	if normalizedCode, err := normalizeWorkResultCode(result.ResultCode); err != nil {
		return WorkResultEnvelope{}, err
	} else {
		result.ResultCode = normalizedCode
	}
	if strings.TrimSpace(result.Changes.PR) == "" {
		result.Changes.PR = "none"
	}
	if strings.TrimSpace(result.Changes.Commit) == "" {
		result.Changes.Commit = "none"
	}
	if strings.TrimSpace(result.Tests.Result) == "" {
		result.Tests.Result = "n/a"
	}
	if strings.TrimSpace(result.Tests.Command) == "" {
		result.Tests.Command = "none"
	}
	if strings.TrimSpace(result.Tests.Evidence) == "" {
		result.Tests.Evidence = "none"
	}
	if strings.TrimSpace(result.Status) == "" {
		result.Status = statusFromWorkResult(result)
	}
	if strings.TrimSpace(result.Summary) == "" {
		result.Summary = "none"
	}
	return result, nil
}

func loadWorkResultText(path string) (WorkResultEnvelope, error) {
	file, err := os.Open(path)
	if err != nil {
		return WorkResultEnvelope{}, err
	}
	defer file.Close()

	headers := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			break
		}
		idx := strings.Index(line, ":")
		if idx <= 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:idx]))
		value := strings.TrimSpace(line[idx+1:])
		headers[key] = value
	}
	if err := scanner.Err(); err != nil {
		return WorkResultEnvelope{}, err
	}

	issueRef := firstNonEmpty(headers["issueref"], headers["issue_ref"])
	runID := firstNonEmpty(headers["runid"], headers["run_id"])
	if issueRef == "" || runID == "" {
		return WorkResultEnvelope{}, errors.New("work result text missing issue_ref/run_id")
	}

	testsValue := firstNonEmpty(headers["tests"], "none")
	testCommand := testsValue
	testResult := "n/a"
	if strings.Contains(testsValue, "=>") {
		parts := strings.SplitN(testsValue, "=>", 2)
		testCommand = strings.TrimSpace(parts[0])
		testResult = strings.TrimSpace(parts[1])
	}
	if testCommand == "" {
		testCommand = "none"
	}
	if testResult == "" {
		testResult = "n/a"
	}

	result := WorkResultEnvelope{
		IssueRef: issueRef,
		RunID:    runID,
		Status:   strings.ToLower(firstNonEmpty(headers["status"], "")),
		Summary:  firstNonEmpty(headers["summary"], "none"),
		Changes: WorkResultChanges{
			PR:     firstNonEmpty(headers["pr"], "none"),
			Commit: firstNonEmpty(headers["commit"], "none"),
		},
		Tests: WorkResultTests{
			Command:  testCommand,
			Result:   testResult,
			Evidence: firstNonEmpty(headers["evidence"], "none"),
		},
		ResultCode: firstNonEmpty(headers["resultcode"], headers["result_code"]),
	}

	if normalizedCode, err := normalizeWorkResultCode(result.ResultCode); err != nil {
		return WorkResultEnvelope{}, err
	} else {
		result.ResultCode = normalizedCode
	}

	if result.Status == "fail" && strings.TrimSpace(result.ResultCode) == "" {
		result.ResultCode = "test_failed"
	}
	if result.Status == "blocked" && strings.TrimSpace(result.ResultCode) == "" {
		result.ResultCode = "dep_unresolved"
	}
	if strings.TrimSpace(result.Status) == "" {
		result.Status = statusFromWorkResult(result)
	}
	if strings.TrimSpace(result.Summary) == "" {
		result.Summary = "none"
	}
	return result, nil
}

type workResultEnvelopeSnake struct {
	IssueRef   string                 `json:"issue_ref"`
	RunID      string                 `json:"run_id"`
	Status     string                 `json:"status,omitempty"`
	Summary    string                 `json:"summary,omitempty"`
	ResultCode string                 `json:"result_code,omitempty"`
	Changes    workResultChangesSnake `json:"changes"`
	Tests      workResultTestsSnake   `json:"tests"`
}

type workResultChangesSnake struct {
	PR     string `json:"pr"`
	Commit string `json:"commit"`
}

type workResultTestsSnake struct {
	Command  string `json:"command"`
	Result   string `json:"result"`
	Evidence string `json:"evidence"`
}

func (s workResultEnvelopeSnake) toEnvelope() WorkResultEnvelope {
	return WorkResultEnvelope{
		IssueRef:   s.IssueRef,
		RunID:      s.RunID,
		Status:     s.Status,
		Summary:    s.Summary,
		ResultCode: s.ResultCode,
		Changes: WorkResultChanges{
			PR:     s.Changes.PR,
			Commit: s.Changes.Commit,
		},
		Tests: WorkResultTests{
			Command:  s.Tests.Command,
			Result:   s.Tests.Result,
			Evidence: s.Tests.Evidence,
		},
	}
}

func normalizeWorkResultCode(code string) (string, error) {
	trimmed := strings.TrimSpace(code)
	if domainoutbox.IsNoneLike(trimmed) {
		return "", nil
	}
	if err := domainoutbox.ValidateResultCode(trimmed); err != nil {
		return "", err
	}
	return trimmed, nil
}

type WorkAuditEnvelope struct {
	IssueRef         string   `json:"IssueRef"`
	RunID            string   `json:"RunID"`
	Role             string   `json:"Role"`
	ExecutorProgram  string   `json:"ExecutorProgram"`
	ExecutorArgs     []string `json:"ExecutorArgs"`
	TimeoutSeconds   int      `json:"TimeoutSeconds"`
	StartedAt        string   `json:"StartedAt"`
	FinishedAt       string   `json:"FinishedAt"`
	ExitCode         int      `json:"ExitCode"`
	TimedOut         bool     `json:"TimedOut"`
	StdoutPath       string   `json:"StdoutPath"`
	StderrPath       string   `json:"StderrPath"`
	WorkResultSource string   `json:"WorkResultSource"`
	WorkResultJSON   string   `json:"WorkResultJSON"`
	WorkResultText   string   `json:"WorkResultText"`
}

func writeWorkResultAuditJSON(path string, audit WorkAuditEnvelope) error {
	raw, err := json.MarshalIndent(audit, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func resolveExitCode(runErr error) int {
	if runErr == nil {
		return 0
	}

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

type StructuredCommentInput struct {
	Role         string
	IssueRef     string
	RunID        string
	Action       string
	Status       string
	ResultCode   string
	ReadUpTo     string
	Trigger      string
	Summary      string
	Changes      WorkResultChanges
	Tests        WorkResultTests
	BlockedBy    []string
	OpenQuestion string
	Next         string
}

func buildStructuredComment(input StructuredCommentInput) string {
	blockedBy := normalizeBlockedBy(input.BlockedBy)
	openQuestion := strings.TrimSpace(input.OpenQuestion)
	if openQuestion == "" {
		openQuestion = "none"
	}
	resultCode := strings.TrimSpace(input.ResultCode)
	if resultCode == "" {
		resultCode = "none"
	}

	return fmt.Sprintf(
		"Role: %s\nRepo: main\nIssueRef: %s\nRunId: %s\nSpecRef: none\nContractsRef: none\nAction: %s\nStatus: %s\nResultCode: %s\nReadUpTo: %s\nTrigger: %s\n\nSummary:\n- %s\n\nChanges:\n- PR: %s\n- Commit: %s\n\nTests:\n- Command: %s\n- Result: %s\n- Evidence: %s\n\nBlockedBy:\n- %s\n\nOpenQuestions:\n- %s\n\nNext:\n- %s\n",
		noneIfEmpty(input.Role),
		noneIfEmpty(input.IssueRef),
		noneIfEmpty(input.RunID),
		noneIfEmpty(input.Action),
		noneIfEmpty(input.Status),
		resultCode,
		noneIfEmpty(input.ReadUpTo),
		noneIfEmpty(input.Trigger),
		noneIfEmpty(input.Summary),
		noneIfEmpty(input.Changes.PR),
		noneIfEmpty(input.Changes.Commit),
		noneIfEmpty(input.Tests.Command),
		noneIfEmpty(input.Tests.Result),
		noneIfEmpty(input.Tests.Evidence),
		blockedBy,
		openQuestion,
		noneIfEmpty(input.Next),
	)
}

func normalizeBlockedBy(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	clean := make([]string, 0, len(values))
	for _, value := range values {
		item := strings.TrimSpace(value)
		if item == "" {
			continue
		}
		clean = append(clean, item)
	}
	if len(clean) == 0 {
		return "none"
	}
	return strings.Join(clean, ", ")
}

func noneIfEmpty(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "none"
	}
	return trimmed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
