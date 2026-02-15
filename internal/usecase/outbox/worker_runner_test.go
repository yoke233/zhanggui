package outbox

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadWorkResultText(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "work_result.txt")
	content := "IssueRef: local#12\nRunId: 2026-02-14-backend-0003\nStatus: ok\nPR: none\nCommit: git:abc123\nTests: go test ./... => pass\nEvidence: none\nResultCode: none\n\nNotes:\n- done\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write result txt: %v", err)
	}

	result, err := loadWorkResultText(path)
	if err != nil {
		t.Fatalf("loadWorkResultText() error = %v", err)
	}
	if result.IssueRef != "local#12" {
		t.Fatalf("IssueRef = %q", result.IssueRef)
	}
	if result.RunID != "2026-02-14-backend-0003" {
		t.Fatalf("RunID = %q", result.RunID)
	}
	if result.Changes.Commit != "git:abc123" {
		t.Fatalf("Commit = %q", result.Changes.Commit)
	}
	if result.Tests.Result != "pass" {
		t.Fatalf("Tests.Result = %q", result.Tests.Result)
	}
}

func TestLoadWorkflowProfile(t *testing.T) {
	tempDir := t.TempDir()
	workflowPath := filepath.Join(tempDir, "workflow.toml")
	content := `
version = 2

[outbox]
backend = "sqlite"
path = "state/outbox.sqlite"

[roles]
enabled = ["backend"]

[repos]
main = "."

[role_repo]
backend = "main"

[groups.backend]
role = "backend"
max_concurrent = 4
mode = "owner"
writeback = "full"
listen_labels = ["to:backend"]

[executors.backend]
program = "go"
args = ["test", "./..."]
timeout_seconds = 1800
`
	if err := os.WriteFile(workflowPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	profile, err := loadWorkflowProfile(workflowPath)
	if err != nil {
		t.Fatalf("loadWorkflowProfile() error = %v", err)
	}
	if profile.Outbox.Backend != "sqlite" {
		t.Fatalf("outbox.backend = %q", profile.Outbox.Backend)
	}
	if !isRoleEnabled(profile, "backend") {
		t.Fatalf("backend role should be enabled")
	}
	group, ok := findGroupByRole(profile, "backend")
	if !ok {
		t.Fatalf("backend group should exist")
	}
	if len(group.ListenLabels) != 1 || group.ListenLabels[0] != "to:backend" {
		t.Fatalf("listen labels = %#v", group.ListenLabels)
	}
}

func TestLoadWorkResultTextStatusFallback(t *testing.T) {
	testCases := []struct {
		name           string
		status         string
		resultCodeLine string
		wantResultCode string
	}{
		{
			name:           "fail status fallback",
			status:         "fail",
			resultCodeLine: "",
			wantResultCode: "test_failed",
		},
		{
			name:           "blocked status fallback",
			status:         "blocked",
			resultCodeLine: "",
			wantResultCode: "dep_unresolved",
		},
		{
			name:           "explicit result code",
			status:         "fail",
			resultCodeLine: "ResultCode: env_unavailable\n",
			wantResultCode: "env_unavailable",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			tempDir := t.TempDir()
			path := filepath.Join(tempDir, "work_result.txt")
			content := "IssueRef: local#99\nRunId: 2026-02-14-backend-0009\nStatus: " + testCase.status + "\nPR: none\nCommit: git:abc123\nTests: cmd /c exit 1 => fail\nEvidence: none\n" + testCase.resultCodeLine + "\nNotes:\n- generated\n"
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatalf("write result txt: %v", err)
			}

			result, err := loadWorkResultText(path)
			if err != nil {
				t.Fatalf("loadWorkResultText() error = %v", err)
			}
			if result.ResultCode != testCase.wantResultCode {
				t.Fatalf("ResultCode = %q, want %q", result.ResultCode, testCase.wantResultCode)
			}
		})
	}
}

func TestLoadWorkResultTextMissingRequiredFields(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "work_result.txt")
	content := "IssueRef: local#12\nStatus: ok\nPR: none\nCommit: none\nTests: none => n/a\nEvidence: none\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write result txt: %v", err)
	}

	if _, err := loadWorkResultText(path); err == nil {
		t.Fatalf("loadWorkResultText() expected error for missing RunId")
	}
}

func TestLoadWorkResultJSONDefaults(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "work_result.json")
	content := `{
  "IssueRef": "local#21",
  "RunID": "2026-02-14-backend-0011",
  "Changes": {"PR": "", "Commit": ""},
  "Tests": {"Command": "", "Result": "", "Evidence": ""}
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write result json: %v", err)
	}

	result, err := loadWorkResultJSON(path)
	if err != nil {
		t.Fatalf("loadWorkResultJSON() error = %v", err)
	}
	if result.Changes.PR != "none" || result.Changes.Commit != "none" {
		t.Fatalf("changes defaults = %#v", result.Changes)
	}
	if result.Tests.Command != "none" || result.Tests.Result != "n/a" || result.Tests.Evidence != "none" {
		t.Fatalf("tests defaults = %#v", result.Tests)
	}
}

func TestLoadWorkResultJSONMissingRequiredFields(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "work_result.json")
	content := `{"IssueRef": "", "RunID": ""}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write result json: %v", err)
	}

	if _, err := loadWorkResultJSON(path); err == nil {
		t.Fatalf("loadWorkResultJSON() expected error for missing issue_ref/run_id")
	}
}

func TestLoadWorkResultFromContextPackPrefersJSON(t *testing.T) {
	tempDir := t.TempDir()
	jsonPath := filepath.Join(tempDir, "work_result.json")
	textPath := filepath.Join(tempDir, "work_result.txt")

	jsonResult := WorkResultEnvelope{
		IssueRef: "local#31",
		RunID:    "2026-02-14-backend-0031",
		Changes: WorkResultChanges{
			PR:     "none",
			Commit: "git:json",
		},
		Tests: WorkResultTests{
			Command:  "cmd /c echo json",
			Result:   "pass",
			Evidence: "none",
		},
	}
	raw, err := json.MarshalIndent(jsonResult, "", "  ")
	if err != nil {
		t.Fatalf("marshal json result: %v", err)
	}
	if err := os.WriteFile(jsonPath, raw, 0o644); err != nil {
		t.Fatalf("write result json: %v", err)
	}

	textContent := "IssueRef: local#31\nRunId: 2026-02-14-backend-0031\nStatus: fail\nPR: none\nCommit: git:text\nTests: cmd /c exit 1 => fail\nEvidence: none\nResultCode: test_failed\n\nNotes:\n- fallback\n"
	if err := os.WriteFile(textPath, []byte(textContent), 0o644); err != nil {
		t.Fatalf("write result txt: %v", err)
	}

	result, err := loadWorkResultFromContextPack(tempDir)
	if err != nil {
		t.Fatalf("loadWorkResultFromContextPack() error = %v", err)
	}
	if result.Changes.Commit != "git:json" {
		t.Fatalf("commit = %q, want git:json", result.Changes.Commit)
	}
	if result.ResultCode != "" {
		t.Fatalf("ResultCode = %q, want empty", result.ResultCode)
	}
}

func TestBuildStructuredCommentDefaults(t *testing.T) {
	body := buildStructuredComment(StructuredCommentInput{
		Role:     "backend",
		IssueRef: "local#7",
		RunID:    "2026-02-14-backend-0007",
		Action:   "update",
		Status:   "review",
		ReadUpTo: "e19",
		Trigger:  "workrun:2026-02-14-backend-0007",
		Summary:  "worker completed",
	})

	requiredSnippets := []string{
		"IssueRef: local#7",
		"RunId: 2026-02-14-backend-0007",
		"ResultCode: none",
		"BlockedBy:\n- none",
		"OpenQuestions:\n- none",
		"- PR: none",
		"- Commit: none",
		"- Command: none",
		"- Result: none",
		"- Evidence: none",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(body, snippet) {
			t.Fatalf("structured body missing %q, body=%s", snippet, body)
		}
	}
}

func TestNormalizeBlockedBy(t *testing.T) {
	got := normalizeBlockedBy([]string{"", " dep-A ", "dep-B", " "})
	if got != "dep-A, dep-B" {
		t.Fatalf("normalizeBlockedBy() = %q", got)
	}
	if normalizeBlockedBy(nil) != "none" {
		t.Fatalf("normalizeBlockedBy(nil) should be none")
	}
}

func writeWorkerWorkflow(t *testing.T, rootDir string, program string, args []string, timeoutSeconds int) string {
	t.Helper()

	argsRaw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal workflow args: %v", err)
	}
	content := "version = 2\n\n[outbox]\nbackend = \"sqlite\"\npath = \"state/outbox.sqlite\"\n\n[roles]\nenabled = [\"backend\"]\n\n[repos]\nmain = \".\"\n\n[role_repo]\nbackend = \"main\"\n\n[groups.backend]\nrole = \"backend\"\nmax_concurrent = 1\nmode = \"owner\"\nwriteback = \"full\"\nlisten_labels = [\"to:backend\"]\n\n[executors.backend]\nprogram = \"" + program + "\"\nargs = " + string(argsRaw) + "\ntimeout_seconds = " + strings.TrimSpace(jsonNumberString(timeoutSeconds)) + "\n"
	path := filepath.Join(rootDir, "workflow.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write workflow: %v", err)
	}
	return path
}

func jsonNumberString(value int) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

func writeWorkOrderFile(t *testing.T, contextPackDir string, issueRef string, runID string, role string, repoDir string) {
	t.Helper()

	order := WorkResultEnvelope{}
	_ = order
	raw := `{
  "IssueRef": "` + issueRef + `",
  "RunID": "` + runID + `",
  "Role": "` + role + `",
  "RepoDir": "` + strings.ReplaceAll(repoDir, `\`, `\\`) + `"
}`
	if err := os.WriteFile(filepath.Join(contextPackDir, "work_order.json"), []byte(raw), 0o644); err != nil {
		t.Fatalf("write work order: %v", err)
	}
}

func TestWorkerRunSuccessWritesPassResult(t *testing.T) {
	contextPackDir := t.TempDir()
	repoDir := t.TempDir()
	workflowPath := writeWorkerWorkflow(t, t.TempDir(), "cmd", []string{"/c", "echo", "worker-ok"}, 30)
	writeWorkOrderFile(t, contextPackDir, "local#41", "2026-02-14-backend-0041", "backend", repoDir)

	svc := &Service{}
	if err := svc.WorkerRun(context.Background(), WorkerRunInput{
		ContextPackDir: contextPackDir,
		WorkflowFile:   workflowPath,
	}); err != nil {
		t.Fatalf("WorkerRun() error = %v", err)
	}

	result, err := loadWorkResultJSON(filepath.Join(contextPackDir, "work_result.json"))
	if err != nil {
		t.Fatalf("loadWorkResultJSON() error = %v", err)
	}
	if result.IssueRef != "local#41" || result.RunID != "2026-02-14-backend-0041" {
		t.Fatalf("result identity = %#v", result)
	}
	if result.Tests.Result != "pass" {
		t.Fatalf("Tests.Result = %q, want pass", result.Tests.Result)
	}
	if strings.TrimSpace(result.ResultCode) != "" {
		t.Fatalf("ResultCode = %q, want empty", result.ResultCode)
	}
	if result.Changes.Commit != "none" && !strings.HasPrefix(result.Changes.Commit, "git:") {
		t.Fatalf("Commit = %q", result.Changes.Commit)
	}
	if _, err := os.Stat(filepath.Join(contextPackDir, "stdout.log")); err != nil {
		t.Fatalf("stdout.log should exist, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(contextPackDir, "stderr.log")); err != nil {
		t.Fatalf("stderr.log should exist, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(contextPackDir, "work_audit.json")); err != nil {
		t.Fatalf("work_audit.json should exist, err=%v", err)
	}
}

func TestWorkerRunCommandFailureWritesTestFailed(t *testing.T) {
	contextPackDir := t.TempDir()
	repoDir := t.TempDir()
	workflowPath := writeWorkerWorkflow(t, t.TempDir(), "cmd", []string{"/c", "exit", "1"}, 30)
	writeWorkOrderFile(t, contextPackDir, "local#42", "2026-02-14-backend-0042", "backend", repoDir)

	svc := &Service{}
	if err := svc.WorkerRun(context.Background(), WorkerRunInput{
		ContextPackDir: contextPackDir,
		WorkflowFile:   workflowPath,
	}); err != nil {
		t.Fatalf("WorkerRun() error = %v", err)
	}

	result, err := loadWorkResultJSON(filepath.Join(contextPackDir, "work_result.json"))
	if err != nil {
		t.Fatalf("loadWorkResultJSON() error = %v", err)
	}
	if result.Tests.Result != "fail" {
		t.Fatalf("Tests.Result = %q, want fail", result.Tests.Result)
	}
	if result.ResultCode != "test_failed" {
		t.Fatalf("ResultCode = %q, want test_failed", result.ResultCode)
	}
}

func TestWorkerRunTimeoutWritesEnvUnavailable(t *testing.T) {
	rootDir, err := os.MkdirTemp("", "zg-worker-timeout-root-*")
	if err != nil {
		t.Fatalf("MkdirTemp() error = %v", err)
	}
	t.Cleanup(func() {
		for i := 0; i < 30; i++ {
			if removeErr := os.RemoveAll(rootDir); removeErr == nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	})

	contextPackDir := filepath.Join(rootDir, "context-pack")
	repoDir := filepath.Join(rootDir, "repo")
	workflowRoot := filepath.Join(rootDir, "workflow")
	if err := os.MkdirAll(contextPackDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(context pack) error = %v", err)
	}
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(repo) error = %v", err)
	}
	if err := os.MkdirAll(workflowRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll(workflow) error = %v", err)
	}
	workflowPath := writeWorkerWorkflow(t, workflowRoot, "cmd", []string{"/c", "ping", "127.0.0.1", "-n", "6"}, 1)
	writeWorkOrderFile(t, contextPackDir, "local#43", "2026-02-14-backend-0043", "backend", repoDir)

	svc := &Service{}
	if err := svc.WorkerRun(context.Background(), WorkerRunInput{
		ContextPackDir: contextPackDir,
		WorkflowFile:   workflowPath,
	}); err != nil {
		t.Fatalf("WorkerRun() error = %v", err)
	}

	result, err := loadWorkResultJSON(filepath.Join(contextPackDir, "work_result.json"))
	if err != nil {
		t.Fatalf("loadWorkResultJSON() error = %v", err)
	}
	if result.Tests.Result != "fail" {
		t.Fatalf("Tests.Result = %q, want fail", result.Tests.Result)
	}
	if result.ResultCode != "env_unavailable" {
		t.Fatalf("ResultCode = %q, want env_unavailable", result.ResultCode)
	}
}

func TestWorkerRunInputValidation(t *testing.T) {
	contextPackDir := t.TempDir()
	repoDir := t.TempDir()
	workflowPath := writeWorkerWorkflow(t, t.TempDir(), "cmd", []string{"/c", "echo", "worker-ok"}, 30)
	writeWorkOrderFile(t, contextPackDir, "local#44", "2026-02-14-backend-0044", "backend", repoDir)

	svc := &Service{}
	if err := svc.WorkerRun(nil, WorkerRunInput{
		ContextPackDir: contextPackDir,
		WorkflowFile:   workflowPath,
	}); err == nil {
		t.Fatalf("WorkerRun(nil ctx) expected error")
	}

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := svc.WorkerRun(canceledCtx, WorkerRunInput{
		ContextPackDir: contextPackDir,
		WorkflowFile:   workflowPath,
	}); err == nil {
		t.Fatalf("WorkerRun(canceled ctx) expected error")
	}

	if err := svc.WorkerRun(context.Background(), WorkerRunInput{
		ContextPackDir: "",
		WorkflowFile:   workflowPath,
	}); err == nil {
		t.Fatalf("WorkerRun(empty context pack) expected error")
	}
}

func TestWorkerRunInvalidWorkOrder(t *testing.T) {
	contextPackDir := t.TempDir()
	workflowPath := writeWorkerWorkflow(t, t.TempDir(), "cmd", []string{"/c", "echo", "worker-ok"}, 30)
	raw := `{
  "IssueRef": "local#45",
  "RunID": "invalid-run-id",
  "Role": "backend",
  "RepoDir": "."
}`
	if err := os.WriteFile(filepath.Join(contextPackDir, "work_order.json"), []byte(raw), 0o644); err != nil {
		t.Fatalf("write invalid work order: %v", err)
	}

	svc := &Service{}
	if err := svc.WorkerRun(context.Background(), WorkerRunInput{
		ContextPackDir: contextPackDir,
		WorkflowFile:   workflowPath,
	}); err == nil {
		t.Fatalf("WorkerRun(invalid work order) expected error")
	}
}

func TestWorkerRunMissingWorkOrderFile(t *testing.T) {
	contextPackDir := t.TempDir()
	workflowPath := writeWorkerWorkflow(t, t.TempDir(), "cmd", []string{"/c", "echo", "worker-ok"}, 30)

	svc := &Service{}
	if err := svc.WorkerRun(context.Background(), WorkerRunInput{
		ContextPackDir: contextPackDir,
		WorkflowFile:   workflowPath,
	}); err == nil {
		t.Fatalf("WorkerRun(missing work_order.json) expected error")
	}
}

func TestWorkerRunMissingWorkflowFile(t *testing.T) {
	contextPackDir := t.TempDir()
	repoDir := t.TempDir()
	writeWorkOrderFile(t, contextPackDir, "local#46", "2026-02-14-backend-0046", "backend", repoDir)

	svc := &Service{}
	if err := svc.WorkerRun(context.Background(), WorkerRunInput{
		ContextPackDir: contextPackDir,
		WorkflowFile:   filepath.Join(t.TempDir(), "missing-workflow.toml"),
	}); err == nil {
		t.Fatalf("WorkerRun(missing workflow) expected error")
	}
}

func TestLoadWorkOrderErrors(t *testing.T) {
	if _, err := loadWorkOrder(filepath.Join(t.TempDir(), "missing-work-order.json")); err == nil {
		t.Fatalf("loadWorkOrder(missing) expected error")
	}

	workOrderPath := filepath.Join(t.TempDir(), "work_order.json")
	if err := os.WriteFile(workOrderPath, []byte("{bad json"), 0o644); err != nil {
		t.Fatalf("write invalid work order: %v", err)
	}
	if _, err := loadWorkOrder(workOrderPath); err == nil {
		t.Fatalf("loadWorkOrder(invalid json) expected error")
	}
}

func TestLoadWorkResultFromContextPackFallbackToText(t *testing.T) {
	contextPackDir := t.TempDir()
	jsonPath := filepath.Join(contextPackDir, "work_result.json")
	textPath := filepath.Join(contextPackDir, "work_result.txt")

	if err := os.WriteFile(jsonPath, []byte("{bad json"), 0o644); err != nil {
		t.Fatalf("write invalid json: %v", err)
	}
	textContent := "IssueRef: local#61\nRunId: 2026-02-14-backend-0061\nStatus: fail\nPR: none\nCommit: git:text-fallback\nTests: cmd /c exit 1 => fail\nEvidence: none\n\nNotes:\n- fallback\n"
	if err := os.WriteFile(textPath, []byte(textContent), 0o644); err != nil {
		t.Fatalf("write result txt: %v", err)
	}

	if _, err := loadWorkResultFromContextPack(contextPackDir); err == nil {
		t.Fatalf("loadWorkResultFromContextPack() expected error when JSON exists but is invalid")
	}
}

func TestLoadWorkResultFromContextPackMissingFiles(t *testing.T) {
	if _, err := loadWorkResultFromContextPack(t.TempDir()); err == nil {
		t.Fatalf("loadWorkResultFromContextPack(missing files) expected error")
	}
}

func TestLoadWorkResultFromContextPackFallsBackToLogsWhenResultMissing(t *testing.T) {
	contextPackDir := t.TempDir()
	repoDir := t.TempDir()
	writeWorkOrderFile(t, contextPackDir, "local#70", "2026-02-14-backend-0070", "backend", repoDir)

	stdoutPath := filepath.Join(contextPackDir, "stdout.log")
	if err := os.WriteFile(stdoutPath, []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write stdout.log: %v", err)
	}

	result, err := loadWorkResultFromContextPack(contextPackDir)
	if err != nil {
		t.Fatalf("loadWorkResultFromContextPack() error = %v", err)
	}
	if result.Source != "logs" {
		t.Fatalf("Source = %q, want logs", result.Source)
	}
	if result.IssueRef != "local#70" || result.RunID != "2026-02-14-backend-0070" {
		t.Fatalf("identity = %#v", result)
	}
	if result.Tests.Evidence != "stdout.log" {
		t.Fatalf("Evidence = %q, want stdout.log", result.Tests.Evidence)
	}
}

func TestLoadWorkResultFromContextPackFallsBackToTextWhenJSONMissing(t *testing.T) {
	contextPackDir := t.TempDir()
	textPath := filepath.Join(contextPackDir, "work_result.txt")

	textContent := "IssueRef: local#62\nRunId: 2026-02-14-backend-0062\nStatus: ok\nPR: none\nCommit: git:text-only\nTests: go test ./... => pass\nEvidence: none\n\nNotes:\n- fallback\n"
	if err := os.WriteFile(textPath, []byte(textContent), 0o644); err != nil {
		t.Fatalf("write result txt: %v", err)
	}

	result, err := loadWorkResultFromContextPack(contextPackDir)
	if err != nil {
		t.Fatalf("loadWorkResultFromContextPack() error = %v", err)
	}
	if result.Source != "text" {
		t.Fatalf("Source = %q, want text", result.Source)
	}
	if result.Changes.Commit != "git:text-only" {
		t.Fatalf("Commit = %q", result.Changes.Commit)
	}
}

func TestLoadWorkResultJSONAcceptsSnakeCase(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "work_result.json")
	content := `{
  "issue_ref": "local#81",
  "run_id": "2026-02-14-backend-0081",
  "result_code": "none",
  "changes": { "pr": "none", "commit": "git:snake" },
  "tests": { "command": "go test ./...", "result": "pass", "evidence": "none" }
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write result json: %v", err)
	}

	result, err := loadWorkResultJSON(path)
	if err != nil {
		t.Fatalf("loadWorkResultJSON() error = %v", err)
	}
	if result.IssueRef != "local#81" || result.RunID != "2026-02-14-backend-0081" {
		t.Fatalf("result identity = %#v", result)
	}
	if result.ResultCode != "" {
		t.Fatalf("ResultCode = %q, want empty", result.ResultCode)
	}
	if result.Changes.Commit != "git:snake" {
		t.Fatalf("Commit = %q", result.Changes.Commit)
	}
}

func TestStatusFromWorkResult(t *testing.T) {
	testCases := []struct {
		name   string
		input  WorkResultEnvelope
		status string
	}{
		{
			name: "pass",
			input: WorkResultEnvelope{
				ResultCode: "",
				Tests:      WorkResultTests{Result: "pass"},
			},
			status: "ok",
		},
		{
			name: "tests fail",
			input: WorkResultEnvelope{
				ResultCode: "",
				Tests:      WorkResultTests{Result: "fail"},
			},
			status: "fail",
		},
		{
			name: "has result code",
			input: WorkResultEnvelope{
				ResultCode: "test_failed",
				Tests:      WorkResultTests{Result: "pass"},
			},
			status: "fail",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got := statusFromWorkResult(testCase.input)
			if got != testCase.status {
				t.Fatalf("statusFromWorkResult() = %q, want %q", got, testCase.status)
			}
		})
	}
}

func TestWriteWorkResultJSONRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "work_result.json")
	input := WorkResultEnvelope{
		IssueRef:   "local#71",
		RunID:      "2026-02-14-backend-0071",
		ResultCode: "test_failed",
		Changes: WorkResultChanges{
			PR:     "none",
			Commit: "git:abc",
		},
		Tests: WorkResultTests{
			Command:  "go test ./...",
			Result:   "fail",
			Evidence: "none",
		},
	}

	if err := writeWorkResultJSON(path, input); err != nil {
		t.Fatalf("writeWorkResultJSON() error = %v", err)
	}

	got, err := loadWorkResultJSON(path)
	if err != nil {
		t.Fatalf("loadWorkResultJSON() error = %v", err)
	}
	if got.IssueRef != input.IssueRef || got.RunID != input.RunID || got.ResultCode != input.ResultCode {
		t.Fatalf("round trip mismatch, got=%#v input=%#v", got, input)
	}
}

func TestWriteWorkResultTextContainsStatus(t *testing.T) {
	path := filepath.Join(t.TempDir(), "work_result.txt")
	input := WorkResultEnvelope{
		IssueRef:   "local#72",
		RunID:      "2026-02-14-backend-0072",
		ResultCode: "test_failed",
		Changes: WorkResultChanges{
			PR:     "none",
			Commit: "git:abc",
		},
		Tests: WorkResultTests{
			Command:  "go test ./...",
			Result:   "fail",
			Evidence: "none",
		},
	}

	if err := writeWorkResultText(path, input); err != nil {
		t.Fatalf("writeWorkResultText() error = %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read work_result.txt error = %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "Status: fail") {
		t.Fatalf("work_result.txt missing fail status, text=%s", text)
	}
	if !strings.Contains(text, "ResultCode: test_failed") {
		t.Fatalf("work_result.txt missing result code, text=%s", text)
	}
}

func TestResolveGitCommitNonGitRepo(t *testing.T) {
	if got := resolveGitCommit(t.TempDir()); got != "" {
		t.Fatalf("resolveGitCommit(non-git) = %q, want empty", got)
	}
}
