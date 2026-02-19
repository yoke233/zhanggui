# Codex Dev-Review-Test-Merge Pipeline Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 构建一条自动化链路：输入需求文件后，驱动 Codex 在目标项目编码并提交；随后执行 Codex Review，若有问题回流修复；再执行 Codex Test，失败继续回流修复；全部通过后开放合并闸门。  

**Architecture:** 在现有 Outbox/Lead/Worker 机制上增量扩展一个 `pipeline` 用例和 CLI 命令。链路核心由 `RunCodexPipeline` 编排：`coding -> review -> test -> merge-check`，其中 review/test 结果通过既有 `IngestQualityEvent` 写入审计轨迹，失败统一回流到编码角色。为了可测试性与可替换性，引入 `codexRunner` 接口封装 Codex CLI 调用，单测用 fake runner 覆盖回路场景。  

**Tech Stack:** Go (`cobra`, `testing`, `gorm/sqlite`), PowerShell wrapper scripts, existing Outbox domain/usecase, Codex CLI.  

---

## Preflight (一次性准备)

- 建议在独立 worktree 执行本计划。
- 参考技能：`@test-driven-development`、`@systematic-debugging`、`@verification-before-completion`、`@requesting-code-review`。
- 全程采用小步提交；每个 Task 完成后立即提交。

### Task 1: 定义 Pipeline 领域契约

**Files:**
- Create: `internal/domain/outbox/pipeline_contracts.go`
- Test: `internal/domain/outbox/pipeline_contracts_test.go`

**Step 1: Write the failing test**

```go
package outbox

import "testing"

func TestValidatePipelineRequest_RequireFields(t *testing.T) {
	req := PipelineRequest{}
	if err := ValidatePipelineRequest(req); err == nil {
		t.Fatalf("expected error for empty request")
	}
}

func TestValidatePipelineRequest_DefaultsAndBounds(t *testing.T) {
	req := PipelineRequest{
		IssueRef:       "local#1",
		ProjectDir:     "D:/project/zhanggui",
		PromptFile:     "mailbox/issue.md",
		CodingRole:     "backend",
		MaxReviewRound: 0,
		MaxTestRound:   0,
	}
	normalized, err := NormalizePipelineRequest(req)
	if err != nil {
		t.Fatalf("NormalizePipelineRequest() error = %v", err)
	}
	if normalized.MaxReviewRound != 3 || normalized.MaxTestRound != 3 {
		t.Fatalf("unexpected defaults: %#v", normalized)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/domain/outbox -run TestValidatePipelineRequest -v`  
Expected: FAIL，报 `undefined: PipelineRequest` 或 `undefined: ValidatePipelineRequest`。

**Step 3: Write minimal implementation**

```go
package outbox

import (
	"errors"
	"strings"
)

type PipelineRequest struct {
	IssueRef       string
	ProjectDir     string
	PromptFile     string
	CodingRole     string
	MaxReviewRound int
	MaxTestRound   int
}

func ValidatePipelineRequest(in PipelineRequest) error {
	if strings.TrimSpace(in.IssueRef) == "" {
		return errors.New("issue_ref is required")
	}
	if strings.TrimSpace(in.ProjectDir) == "" {
		return errors.New("project_dir is required")
	}
	if strings.TrimSpace(in.PromptFile) == "" {
		return errors.New("prompt_file is required")
	}
	return nil
}

func NormalizePipelineRequest(in PipelineRequest) (PipelineRequest, error) {
	if err := ValidatePipelineRequest(in); err != nil {
		return PipelineRequest{}, err
	}
	out := in
	if strings.TrimSpace(out.CodingRole) == "" {
		out.CodingRole = "backend"
	}
	if out.MaxReviewRound <= 0 {
		out.MaxReviewRound = 3
	}
	if out.MaxTestRound <= 0 {
		out.MaxTestRound = 3
	}
	return out, nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/domain/outbox -run TestValidatePipelineRequest -v`  
Expected: PASS。

**Step 5: Commit**

```bash
git add internal/domain/outbox/pipeline_contracts.go internal/domain/outbox/pipeline_contracts_test.go
git commit -m "feat: add pipeline request domain contracts"
```

### Task 2: 增加 Codex CLI 适配器（可 fake）

**Files:**
- Create: `internal/usecase/outbox/codex_runner.go`
- Test: `internal/usecase/outbox/codex_runner_test.go`
- Modify: `internal/usecase/outbox/service.go`

**Step 1: Write the failing test**

```go
func TestCodexRunner_ParseJSONResult(t *testing.T) {
	raw := `{"status":"pass","summary":"ok","result_code":"none","commit":"git:abc"}`
	got, err := parseCodexResult(raw)
	if err != nil {
		t.Fatalf("parseCodexResult() error = %v", err)
	}
	if got.Status != "pass" || got.Commit != "git:abc" {
		t.Fatalf("unexpected result: %#v", got)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/usecase/outbox -run TestCodexRunner_ParseJSONResult -v`  
Expected: FAIL，报 `undefined: parseCodexResult`。

**Step 3: Write minimal implementation**

```go
type CodexRunMode string

const (
	CodexRunCoding CodexRunMode = "coding"
	CodexRunReview CodexRunMode = "review"
	CodexRunTest   CodexRunMode = "test"
)

type CodexRunInput struct {
	Mode       CodexRunMode
	ProjectDir string
	PromptFile string
	IssueRef   string
	RunID      string
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

func parseCodexResult(raw string) (CodexRunOutput, error) {
	var out CodexRunOutput
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return CodexRunOutput{}, err
	}
	return out, nil
}
```

并在 `service.go` 增加字段与默认注入点：

```go
type Service struct {
	// ...
	codexRunner codexRunner
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/usecase/outbox -run TestCodexRunner_ParseJSONResult -v`  
Expected: PASS。

**Step 5: Commit**

```bash
git add internal/usecase/outbox/codex_runner.go internal/usecase/outbox/codex_runner_test.go internal/usecase/outbox/service.go
git commit -m "feat: add codex runner adapter for pipeline orchestration"
```

### Task 3: 实现 Pipeline 编排用例（编码 -> Review -> Test 回路）

**Files:**
- Create: `internal/usecase/outbox/pipeline_run.go`
- Test: `internal/usecase/outbox/pipeline_run_test.go`
- Modify: `internal/usecase/outbox/service.go`

**Step 1: Write the failing test**

```go
func TestRunCodexPipeline_ReviewFailThenFixThenTestPass(t *testing.T) {
	svc, repo := setupService(t)
	issueRef := createLeadClaimedIssue(t, svc, context.Background(), "pipeline", "body", []string{"to:backend", "state:doing"})

	fake := &fakeCodexRunner{
		results: []CodexRunOutput{
			{Status: "pass", Commit: "git:c1"}, // coding #1
			{Status: "fail", ResultCode: "review_changes_requested", Evidence: "review://r1"}, // review #1
			{Status: "pass", Commit: "git:c2"}, // coding #2
			{Status: "pass", Evidence: "review://r2"}, // review #2
			{Status: "pass", Evidence: "test://t2"}, // test #2
		},
	}
	svc.codexRunner = fake

	out, err := svc.RunCodexPipeline(context.Background(), RunCodexPipelineInput{
		IssueRef:       issueRef,
		ProjectDir:     ".",
		PromptFile:     "mailbox/issue.md",
		CodingRole:     "backend",
		MaxReviewRound: 3,
		MaxTestRound:   3,
	})
	if err != nil {
		t.Fatalf("RunCodexPipeline() error = %v", err)
	}
	if !out.ReadyToMerge {
		t.Fatalf("expected ReadyToMerge=true")
	}
	_ = repo
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/usecase/outbox -run TestRunCodexPipeline_ReviewFailThenFixThenTestPass -v`  
Expected: FAIL，报 `undefined: RunCodexPipeline` 或 `undefined: RunCodexPipelineInput`。

**Step 3: Write minimal implementation**

```go
type RunCodexPipelineInput struct {
	IssueRef       string
	ProjectDir     string
	PromptFile     string
	CodingRole     string
	MaxReviewRound int
	MaxTestRound   int
}

type RunCodexPipelineResult struct {
	IssueRef      string
	Rounds        int
	ReadyToMerge  bool
	LastResult    string
	LastResultCode string
}

func (s *Service) RunCodexPipeline(ctx context.Context, in RunCodexPipelineInput) (RunCodexPipelineResult, error) {
	// 1) normalize input
	// 2) coding
	// 3) review fail -> ingest(review changes_requested) -> continue coding
	// 4) test fail -> ingest(ci fail) -> continue coding
	// 5) pass -> ingest(review approved + ci pass)
	// 6) return ReadyToMerge
	return RunCodexPipelineResult{IssueRef: in.IssueRef, ReadyToMerge: true}, nil
}
```

补充：失败路径必须调用 `IngestQualityEvent`，保证审计可回放。

**Step 4: Run test to verify it passes**

Run: `go test ./internal/usecase/outbox -run TestRunCodexPipeline_ReviewFailThenFixThenTestPass -v`  
Expected: PASS。

**Step 5: Commit**

```bash
git add internal/usecase/outbox/pipeline_run.go internal/usecase/outbox/pipeline_run_test.go internal/usecase/outbox/service.go
git commit -m "feat: add codex pipeline orchestration with review/test feedback loops"
```

### Task 4: 固化“失败回流编码器”路由策略

**Files:**
- Modify: `internal/usecase/outbox/lead_runner.go`
- Modify: `internal/usecase/outbox/quality_event_ingest.go`
- Test: `internal/usecase/outbox/quality_event_ingest_test.go`

**Step 1: Write the failing test**

```go
func TestIngestQualityEvent_CIFailRoutesToBackend(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()
	issueRef := createLeadClaimedIssue(t, svc, ctx, "ci fail route", "body", []string{"to:backend", "to:qa", "state:review"})

	out, err := svc.IngestQualityEvent(ctx, IngestQualityEventInput{
		IssueRef: issueRef,
		Source:   "manual",
		Category: "ci",
		Result:   "fail",
		Actor:    "quality-bot",
		Evidence: []string{"https://ci.local/build/1"},
	})
	if err != nil {
		t.Fatalf("IngestQualityEvent() error = %v", err)
	}
	if out.RoutedRole != "backend" {
		t.Fatalf("RoutedRole = %q, want backend", out.RoutedRole)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/usecase/outbox -run TestIngestQualityEvent_CIFailRoutesToBackend -v`  
Expected: FAIL（当前实现在部分标签组合下可能路由到 qa）。

**Step 3: Write minimal implementation**

```go
func nextRoleForFixCycle(labels []string) string {
	if containsString(labels, "to:backend") {
		return "backend"
	}
	if containsString(labels, "to:frontend") {
		return "frontend"
	}
	return "backend"
}
```

并将 `quality_event_ingest.go` 中 failure 路由替换为 `nextRoleForFixCycle(labels)`，避免失败事件回流到 reviewer/qa。

**Step 4: Run test to verify it passes**

Run: `go test ./internal/usecase/outbox -run TestIngestQualityEvent_CIFailRoutesToBackend -v`  
Expected: PASS。

**Step 5: Commit**

```bash
git add internal/usecase/outbox/lead_runner.go internal/usecase/outbox/quality_event_ingest.go internal/usecase/outbox/quality_event_ingest_test.go
git commit -m "fix: route review/ci failures back to coding role"
```

### Task 5: 增加 Merge Gate 用例（可检查、可执行）

**Files:**
- Create: `internal/usecase/outbox/merge_gate.go`
- Test: `internal/usecase/outbox/merge_gate_test.go`

**Step 1: Write the failing test**

```go
func TestMergeGate_RequiresReviewApprovedAndQAPass(t *testing.T) {
	svc, _ := setupService(t)
	ctx := context.Background()
	issueRef := createLeadClaimedIssue(t, svc, ctx, "merge gate", "body", []string{"to:backend", "state:review"})

	ok, reason, err := svc.CanMergeIssue(ctx, issueRef)
	if err != nil {
		t.Fatalf("CanMergeIssue() error = %v", err)
	}
	if ok || reason == "" {
		t.Fatalf("expected merge to be blocked")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/usecase/outbox -run TestMergeGate_RequiresReviewApprovedAndQAPass -v`  
Expected: FAIL，报 `undefined: CanMergeIssue`。

**Step 3: Write minimal implementation**

```go
func (s *Service) CanMergeIssue(ctx context.Context, issueRef string) (bool, string, error) {
	issue, err := s.GetIssue(ctx, issueRef)
	if err != nil {
		return false, "", err
	}
	if containsString(issue.Labels, "needs-human") {
		return false, "needs-human present", nil
	}
	if !containsString(issue.Labels, "review:approved") {
		return false, "missing review:approved", nil
	}
	if !containsString(issue.Labels, "qa:pass") {
		return false, "missing qa:pass", nil
	}
	return true, "ready", nil
}
```

可选：新增 `MergeIssue`，在通过 gate 后写入结构化事件并 `CloseIssue`。

**Step 4: Run test to verify it passes**

Run: `go test ./internal/usecase/outbox -run TestMergeGate_RequiresReviewApprovedAndQAPass -v`  
Expected: PASS。

**Step 5: Commit**

```bash
git add internal/usecase/outbox/merge_gate.go internal/usecase/outbox/merge_gate_test.go
git commit -m "feat: add merge gate checks for review and qa verdicts"
```

### Task 6: 增加 CLI 命令（一键跑链路 + 合并闸门）

**Files:**
- Create: `cmd/outbox_pipeline.go`
- Create: `cmd/outbox_merge.go`
- Modify: `cmd/outbox.go`
- Test: `cmd/outbox_quality_webhook_test.go` (保持回归)
- Create: `cmd/outbox_pipeline_test.go`

**Step 1: Write the failing test**

```go
func TestOutboxPipelineRunFlags(t *testing.T) {
	cmd := newOutboxPipelineRunCmd(nil)
	if err := cmd.ParseFlags([]string{
		"--issue", "local#1",
		"--project-dir", ".",
		"--prompt-file", "mailbox/issue.md",
	}); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./cmd -run TestOutboxPipelineRunFlags -v`  
Expected: FAIL，报 `undefined: newOutboxPipelineRunCmd`。

**Step 3: Write minimal implementation**

```go
var outboxPipelineCmd = &cobra.Command{
	Use:   "pipeline",
	Short: "Run codex coding/review/test pipeline",
}

var outboxPipelineRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Execute coding -> review -> test loop and open merge gate on success",
	RunE: withApp(func(cmd *cobra.Command, _ *bootstrap.App, svc *outbox.Service) error {
		// read flags and call svc.RunCodexPipeline(...)
		return nil
	}),
}
```

同时增加：
- `outbox merge check --issue local#1`
- `outbox merge apply --issue local#1 --actor lead-integrator`

**Step 4: Run test to verify it passes**

Run: `go test ./cmd -run TestOutboxPipelineRunFlags -v`  
Expected: PASS。

**Step 5: Commit**

```bash
git add cmd/outbox_pipeline.go cmd/outbox_merge.go cmd/outbox.go cmd/outbox_pipeline_test.go
git commit -m "feat: add outbox pipeline and merge gate commands"
```

### Task 7: Codex Wrapper 脚本 + Workflow 配置 + 文档

**Files:**
- Create: `scripts/codex-coder.ps1`
- Create: `scripts/codex-review.ps1`
- Create: `scripts/codex-test.ps1`
- Modify: `workflow.toml`
- Modify: `README.md`
- Modify: `docs/workflow/README.md`

**Step 1: Write the failing test**

```go
func TestResolveExecutor_UsesWorkflowCodexScripts(t *testing.T) {
	// 在 workflow_profile_test.go 增加用例：
	// executors.backend/reviewer/qa 指向 pwsh + scripts/codex-*.ps1
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/usecase/outbox -run TestResolveExecutor_UsesWorkflowCodexScripts -v`  
Expected: FAIL（workflow fixture 尚未配置 codex executor）。

**Step 3: Write minimal implementation**

`workflow.toml` 示例（关键片段）：

```toml
[executors.backend]
program = "pwsh"
args = ["-NoProfile", "-File", "scripts/codex-coder.ps1"]
timeout_seconds = 3600

[executors.reviewer]
program = "pwsh"
args = ["-NoProfile", "-File", "scripts/codex-review.ps1"]
timeout_seconds = 1800

[executors.qa]
program = "pwsh"
args = ["-NoProfile", "-File", "scripts/codex-test.ps1"]
timeout_seconds = 1800
```

`codex-review.ps1` 输出规范（示例）：

```powershell
$result = @{
  status = "fail" # or "pass"
  summary = "review found 2 issues"
  result_code = "review_changes_requested"
  evidence = "codex-review://run-001"
}
$result | ConvertTo-Json -Compress
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/usecase/outbox -run TestResolveExecutor_UsesWorkflowCodexScripts -v`  
Expected: PASS。

**Step 5: Commit**

```bash
git add scripts/codex-coder.ps1 scripts/codex-review.ps1 scripts/codex-test.ps1 workflow.toml README.md docs/workflow/README.md
git commit -m "chore: wire codex wrappers and workflow executors for pipeline roles"
```

### Task 8: 端到端回归与交付验收

**Files:**
- Create: `internal/usecase/outbox/pipeline_e2e_test.go`
- Modify: `docs/plans/2026-02-19-codex-dev-review-test-merge-pipeline.md` (记录验证结果)

**Step 1: Write the failing test**

```go
func TestCodexPipeline_EndToEnd_ReviewAndTestLoopThenMergeReady(t *testing.T) {
	// fake codex:
	// coding(pass) -> review(fail) -> coding(pass) -> review(pass) -> test(fail) -> coding(pass) -> review(pass) -> test(pass)
	// assert: ready_to_merge=true, issue has review:approved + qa:pass
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/usecase/outbox -run TestCodexPipeline_EndToEnd_ReviewAndTestLoopThenMergeReady -v`  
Expected: FAIL（链路细节未完全补齐前会失败）。

**Step 3: Write minimal implementation**

补齐 `RunCodexPipeline` 的 round 计数、错误信息、最大轮次上限触发 `manual_intervention`，并确保每轮写入结构化 comment。

**Step 4: Run test to verify it passes**

Run:
- `go test ./internal/usecase/outbox -run TestCodexPipeline_EndToEnd_ReviewAndTestLoopThenMergeReady -v`
- `go test ./...`

Expected:
- 指定 e2e 用例 PASS
- 全量测试 PASS

**Step 5: Commit**

```bash
git add internal/usecase/outbox/pipeline_e2e_test.go internal/usecase/outbox/pipeline_run.go internal/usecase/outbox/pipeline_run_test.go docs/plans/2026-02-19-codex-dev-review-test-merge-pipeline.md
git commit -m "test: add e2e coverage for codex pipeline feedback loops"
```

## Verification Checklist (must pass before merge)

- `go test ./...`
- `go run . outbox pipeline run --issue local#1 --project-dir . --prompt-file mailbox/issue.md --workflow workflow.toml`
- `go run . outbox merge check --issue local#1`
- 若 `check` 返回 ready，再执行 `go run . outbox merge apply --issue local#1 --actor lead-integrator`

预期结果：
- review/test 失败时会生成质量事件并自动回流编码角色；
- review/test 均通过时，`merge check` 显示 ready；
- `merge apply` 成功后 issue 进入 `state:done` 并关闭。

## Validation Log (2026-02-19)

已执行并通过：
- `go test ./...`
- `go run . init-db`
- `go run . outbox create --title "pipeline smoke" --body "smoke body" --label "to:backend" --label "state:todo"`
- `go run . outbox claim --issue local#7 --assignee lead-backend --actor lead-backend --body "claim for pipeline smoke"`
- `go run . outbox pipeline run --issue local#7 --project-dir . --prompt-file mailbox/issue.md --workflow workflow.toml`
- `go run . outbox merge check --issue local#7`
- `go run . outbox merge apply --issue local#7 --actor lead-integrator`

关键结果：
- pipeline 命令输出：`ready_to_merge=true`，`rounds=1`；
- merge check 输出：`ready=true reason=ready`；
- merge apply 输出：`closed=true reason=ready`，issue 完成闭环。
