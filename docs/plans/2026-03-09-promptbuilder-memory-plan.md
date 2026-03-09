# PromptBuilder + Memory 分层上下文实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将平铺的 prompt 模板渲染升级为分层上下文拼装（冷/温/热），让 AI agent 看到任务背景、兄弟任务状态和历史事件。

**Architecture:** Memory 接口定义在 core/，SQLite 实现在 store-sqlite/。PromptBuilder 放在 engine/ 包装现有 RenderPrompt()，通过 PromptVars 新增字段将上下文注入模板。现有 .tmpl 模板加占位符，不做破坏性修改。

**Tech Stack:** Go 1.22+, SQLite, text/template

---

### Task 1: Memory 接口定义

**Files:**
- Create: `internal/core/memory.go`

**Step 1: 创建 Memory 接口文件**

```go
package core

// Memory provides layered context for prompt building.
// Implementations return pre-formatted text ready for prompt injection.
type Memory interface {
	// RecallCold returns rarely-changing background context.
	// Current implementation: Issue title + body preview.
	RecallCold(issueID string) (string, error)

	// RecallWarm returns parent and sibling issue summaries.
	// Returns empty string if the issue has no parent.
	RecallWarm(issueID string) (string, error)

	// RecallHot returns recent activity: TaskSteps, filtered RunEvents, ReviewRecords.
	// runID may be empty, in which case run-specific data is skipped.
	RecallHot(issueID string, runID string) (string, error)
}
```

**Step 2: 验证编译**

Run: `go build ./internal/core/...`
Expected: BUILD SUCCESS

**Step 3: Commit**

```bash
git add internal/core/memory.go
git commit -m "feat(core): add Memory interface for layered prompt context"
```

---

### Task 2: SQLiteMemory 实现 — RecallCold

**Files:**
- Create: `internal/plugins/store-sqlite/memory.go`
- Test: `internal/plugins/store-sqlite/memory_test.go`

**Step 1: 写失败测试**

```go
package storesqlite

import (
	"strings"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

func setupMemoryTest(t *testing.T) (*SQLiteStore, *SQLiteMemory) {
	t.Helper()
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	project := &core.Project{ID: "proj-mem", Name: "memory-test", RepoPath: t.TempDir()}
	if err := s.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	return s, NewSQLiteMemory(s)
}

func TestRecallCold(t *testing.T) {
	s, mem := setupMemoryTest(t)
	defer s.Close()

	issue := &core.Issue{
		ID:        "issue-cold-1",
		ProjectID: "proj-mem",
		Title:     "Implement user auth",
		Body:      "We need JWT-based authentication with refresh tokens.",
		Status:    core.IssueStatusDraft,
	}
	if err := s.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	cold, err := mem.RecallCold("issue-cold-1")
	if err != nil {
		t.Fatalf("RecallCold: %v", err)
	}
	if !strings.Contains(cold, "Implement user auth") {
		t.Errorf("cold should contain title, got: %s", cold)
	}
	if !strings.Contains(cold, "JWT-based authentication") {
		t.Errorf("cold should contain body preview, got: %s", cold)
	}
}

func TestRecallCold_NotFound(t *testing.T) {
	s, mem := setupMemoryTest(t)
	defer s.Close()

	cold, err := mem.RecallCold("nonexistent")
	if err != nil {
		t.Fatalf("RecallCold should not error for missing issue: %v", err)
	}
	if cold != "" {
		t.Errorf("RecallCold should return empty for missing issue, got: %q", cold)
	}
}
```

**Step 2: 运行测试验证失败**

Run: `go test ./internal/plugins/store-sqlite/... -run TestRecallCold -v`
Expected: FAIL (SQLiteMemory not defined)

**Step 3: 写最小实现**

```go
package storesqlite

import (
	"fmt"
	"strings"

	"github.com/yoke233/ai-workflow/internal/core"
)

// SQLiteMemory implements core.Memory using the existing SQLiteStore.
type SQLiteMemory struct {
	store *SQLiteStore
}

// NewSQLiteMemory creates a Memory backed by the given SQLiteStore.
func NewSQLiteMemory(store *SQLiteStore) *SQLiteMemory {
	return &SQLiteMemory{store: store}
}

func (m *SQLiteMemory) RecallCold(issueID string) (string, error) {
	issue, err := m.store.GetIssue(strings.TrimSpace(issueID))
	if err != nil || issue == nil {
		return "", nil
	}
	body := truncateRunes(issue.Body, 500)
	return fmt.Sprintf("## 任务背景\n标题: %s\n描述: %s", issue.Title, body), nil
}

// truncateRunes truncates s to at most maxLen runes.
func truncateRunes(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen])
}
```

**Step 4: 运行测试验证通过**

Run: `go test ./internal/plugins/store-sqlite/... -run TestRecallCold -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/plugins/store-sqlite/memory.go internal/plugins/store-sqlite/memory_test.go
git commit -m "feat(store): add SQLiteMemory with RecallCold implementation"
```

---

### Task 3: SQLiteMemory 实现 — RecallWarm

**Files:**
- Modify: `internal/plugins/store-sqlite/memory.go`
- Modify: `internal/plugins/store-sqlite/memory_test.go`

**Step 1: 写失败测试**

在 `memory_test.go` 追加：

```go
func TestRecallWarm_WithParent(t *testing.T) {
	s, mem := setupMemoryTest(t)
	defer s.Close()

	parent := &core.Issue{
		ID:        "issue-parent",
		ProjectID: "proj-mem",
		Title:     "Build auth system",
		Body:      "Complete authentication and authorization system.",
		Status:    core.IssueStatusDecomposed,
	}
	if err := s.CreateIssue(parent); err != nil {
		t.Fatalf("CreateIssue parent: %v", err)
	}
	child1 := &core.Issue{
		ID:        "issue-child-1",
		ProjectID: "proj-mem",
		ParentID:  "issue-parent",
		Title:     "Implement JWT tokens",
		Status:    core.IssueStatusDone,
	}
	child2 := &core.Issue{
		ID:        "issue-child-2",
		ProjectID: "proj-mem",
		ParentID:  "issue-parent",
		Title:     "Implement user management",
		Status:    core.IssueStatusExecuting,
	}
	if err := s.CreateIssue(child1); err != nil {
		t.Fatalf("CreateIssue child1: %v", err)
	}
	if err := s.CreateIssue(child2); err != nil {
		t.Fatalf("CreateIssue child2: %v", err)
	}

	warm, err := mem.RecallWarm("issue-child-2")
	if err != nil {
		t.Fatalf("RecallWarm: %v", err)
	}
	if !strings.Contains(warm, "Build auth system") {
		t.Errorf("warm should contain parent title, got: %s", warm)
	}
	if !strings.Contains(warm, "Implement JWT tokens") {
		t.Errorf("warm should contain sibling title, got: %s", warm)
	}
	if !strings.Contains(warm, "done") {
		t.Errorf("warm should contain sibling status, got: %s", warm)
	}
	// Should not contain self
	if strings.Contains(warm, "Implement user management") {
		t.Errorf("warm should not contain self, got: %s", warm)
	}
}

func TestRecallWarm_NoParent(t *testing.T) {
	s, mem := setupMemoryTest(t)
	defer s.Close()

	issue := &core.Issue{
		ID:        "issue-no-parent",
		ProjectID: "proj-mem",
		Title:     "Standalone issue",
		Status:    core.IssueStatusDraft,
	}
	if err := s.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	warm, err := mem.RecallWarm("issue-no-parent")
	if err != nil {
		t.Fatalf("RecallWarm: %v", err)
	}
	if warm != "" {
		t.Errorf("RecallWarm should return empty for no-parent issue, got: %q", warm)
	}
}
```

**Step 2: 运行测试验证失败**

Run: `go test ./internal/plugins/store-sqlite/... -run TestRecallWarm -v`
Expected: FAIL (RecallWarm not defined)

**Step 3: 写实现**

在 `memory.go` 追加：

```go
func (m *SQLiteMemory) RecallWarm(issueID string) (string, error) {
	issue, err := m.store.GetIssue(strings.TrimSpace(issueID))
	if err != nil || issue == nil {
		return "", nil
	}
	if strings.TrimSpace(issue.ParentID) == "" {
		return "", nil
	}

	parent, err := m.store.GetIssue(issue.ParentID)
	if err != nil || parent == nil {
		return "", nil
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("## 父任务\n标题: %s\n描述: %s\n", parent.Title, truncateRunes(parent.Body, 300)))

	siblings, err := m.store.GetChildIssues(issue.ParentID)
	if err != nil || len(siblings) == 0 {
		return b.String(), nil
	}

	b.WriteString("\n## 兄弟任务\n")
	count := 0
	for _, sib := range siblings {
		if sib.ID == issueID {
			continue
		}
		b.WriteString(fmt.Sprintf("- %s [状态: %s]\n", sib.Title, sib.Status))
		count++
		if count >= 10 {
			break
		}
	}
	return b.String(), nil
}
```

**Step 4: 运行测试验证通过**

Run: `go test ./internal/plugins/store-sqlite/... -run TestRecallWarm -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/plugins/store-sqlite/memory.go internal/plugins/store-sqlite/memory_test.go
git commit -m "feat(store): add RecallWarm with parent/sibling summaries"
```

---

### Task 4: SQLiteMemory 实现 — RecallHot

**Files:**
- Modify: `internal/plugins/store-sqlite/memory.go`
- Modify: `internal/plugins/store-sqlite/memory_test.go`

**Step 1: 写失败测试**

在 `memory_test.go` 追加：

```go
func TestRecallHot(t *testing.T) {
	s, mem := setupMemoryTest(t)
	defer s.Close()

	issue := &core.Issue{
		ID:        "issue-hot-1",
		ProjectID: "proj-mem",
		Title:     "Hot context test",
		Status:    core.IssueStatusExecuting,
	}
	if err := s.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	// Add TaskSteps
	s.SaveTaskStep(&core.TaskStep{
		ID:      "step-1",
		IssueID: "issue-hot-1",
		Action:  core.StepExecutionStarted,
		AgentID: "system",
		Note:    "run dispatched",
		CreatedAt: time.Now(),
	})

	// Add ReviewRecord
	score := 85
	s.SaveReviewRecord(&core.ReviewRecord{
		IssueID:  "issue-hot-1",
		Round:    1,
		Reviewer: "completeness",
		Verdict:  "approve",
		Summary:  "Looks good, all requirements covered",
		Score:    &score,
	})

	hot, err := mem.RecallHot("issue-hot-1", "")
	if err != nil {
		t.Fatalf("RecallHot: %v", err)
	}
	if !strings.Contains(hot, "execution_started") {
		t.Errorf("hot should contain step action, got: %s", hot)
	}
	if !strings.Contains(hot, "completeness") {
		t.Errorf("hot should contain reviewer name, got: %s", hot)
	}
	if !strings.Contains(hot, "Looks good") {
		t.Errorf("hot should contain review summary, got: %s", hot)
	}
}

func TestRecallHot_Empty(t *testing.T) {
	s, mem := setupMemoryTest(t)
	defer s.Close()

	issue := &core.Issue{
		ID:        "issue-hot-empty",
		ProjectID: "proj-mem",
		Title:     "Empty hot",
		Status:    core.IssueStatusDraft,
	}
	if err := s.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	hot, err := mem.RecallHot("issue-hot-empty", "")
	if err != nil {
		t.Fatalf("RecallHot: %v", err)
	}
	if hot != "" {
		t.Errorf("RecallHot should return empty for no-history issue, got: %q", hot)
	}
}
```

需要在文件顶部 import 中追加 `"time"`（如果尚未导入）。

**Step 2: 运行测试验证失败**

Run: `go test ./internal/plugins/store-sqlite/... -run TestRecallHot -v`
Expected: FAIL (RecallHot not defined)

**Step 3: 写实现**

在 `memory.go` 追加：

```go
func (m *SQLiteMemory) RecallHot(issueID string, runID string) (string, error) {
	issueID = strings.TrimSpace(issueID)
	if issueID == "" {
		return "", nil
	}

	var sections []string

	// 1. Recent TaskSteps (last 20)
	steps, err := m.store.ListTaskSteps(issueID)
	if err == nil && len(steps) > 0 {
		var b strings.Builder
		b.WriteString("## 最近事件\n")
		start := 0
		if len(steps) > 20 {
			start = len(steps) - 20
		}
		for _, step := range steps[start:] {
			note := truncateRunes(step.Note, 100)
			if note != "" {
				b.WriteString(fmt.Sprintf("- %s %s: %s\n", step.CreatedAt.Format("15:04:05"), step.Action, note))
			} else {
				b.WriteString(fmt.Sprintf("- %s %s\n", step.CreatedAt.Format("15:04:05"), step.Action))
			}
		}
		sections = append(sections, b.String())
	}

	// 2. Filtered RunEvents (last 5 of type prompt/done/agent_message)
	runID = strings.TrimSpace(runID)
	if runID != "" {
		events, err := m.store.ListRunEvents(runID)
		if err == nil && len(events) > 0 {
			var filtered []core.RunEvent
			for _, ev := range events {
				switch ev.EventType {
				case "prompt", "done", "agent_message":
					filtered = append(filtered, ev)
				}
			}
			if len(filtered) > 5 {
				filtered = filtered[len(filtered)-5:]
			}
			if len(filtered) > 0 {
				var b strings.Builder
				b.WriteString("## 最近执行\n")
				for _, ev := range filtered {
					content := truncateRunes(ev.DataJSON, 200)
					b.WriteString(fmt.Sprintf("- %s: %s\n", ev.EventType, content))
				}
				sections = append(sections, b.String())
			}
		}
	}

	// 3. ReviewRecords (all)
	reviews, err := m.store.GetReviewRecords(issueID)
	if err == nil && len(reviews) > 0 {
		var b strings.Builder
		b.WriteString("## 审查记录\n")
		for _, r := range reviews {
			b.WriteString(fmt.Sprintf("- 第%d轮 %s: %s - %s\n", r.Round, r.Reviewer, r.Verdict, truncateRunes(r.Summary, 200)))
		}
		sections = append(sections, b.String())
	}

	if len(sections) == 0 {
		return "", nil
	}
	return strings.Join(sections, "\n"), nil
}
```

**Step 4: 运行测试验证通过**

Run: `go test ./internal/plugins/store-sqlite/... -run TestRecallHot -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/plugins/store-sqlite/memory.go internal/plugins/store-sqlite/memory_test.go
git commit -m "feat(store): add RecallHot with TaskSteps, RunEvents, ReviewRecords"
```

---

### Task 5: PromptVars 扩展 + 模板改造

**Files:**
- Modify: `internal/engine/prompts.go`
- Modify: `internal/engine/prompt_templates/implement.tmpl`
- Modify: `internal/engine/prompt_templates/code_review.tmpl`
- Modify: `internal/engine/prompt_templates/fixup.tmpl`
- Modify: `internal/engine/prompt_templates/requirements.tmpl`

**Step 1: 在 PromptVars 中新增 3 个字段**

在 `internal/engine/prompts.go` 的 `PromptVars` 结构体末尾追加：

```go
type PromptVars struct {
	ProjectName       string
	RepoPath          string
	WorktreePath      string
	Requirements      string
	ExecutionContext   string
	PreviousReview    string
	HumanFeedback     string
	RetryError        string
	MergeConflictHint string
	RetryCount        int
	// Layered context injected by PromptBuilder.
	ColdContext       string
	WarmContext       string
	HotContext        string
}
```

**Step 2: 改造 implement.tmpl**

```
{{if .ColdContext}}
{{.ColdContext}}

{{end}}
{{if .WarmContext}}
{{.WarmContext}}

{{end}}
你正在项目 {{.ProjectName}} 的 worktree ({{.WorktreePath}}) 中工作。

{{if .RetryError}}上次执行失败，错误信息：{{.RetryError}}
请避免同样的问题。{{end}}
{{if .MergeConflictHint}}合并冲突提示：{{.MergeConflictHint}}
请先处理 rebase 与冲突，再继续编码。{{end}}
{{if .HumanFeedback}}用户反馈：{{.HumanFeedback}}
请根据以上反馈调整方案。{{end}}

请根据以下需求实现代码：

{{.Requirements}}

完成后请确保代码可编译、测试通过，并提交变更。
{{if .HotContext}}

{{.HotContext}}
{{end}}
```

**Step 3: 改造 code_review.tmpl**

```
{{if .ColdContext}}
{{.ColdContext}}

{{end}}
{{if .WarmContext}}
{{.WarmContext}}

{{end}}
你正在对项目 {{.ProjectName}} 的改动进行代码审查。

请重点检查：
1. 功能是否满足需求：{{.Requirements}}
2. 是否存在潜在缺陷与回归风险
3. 测试覆盖是否充分

输出要求：
- 给出问题清单（按严重级别）
- 给出可执行修复建议
{{if .HotContext}}

{{.HotContext}}
{{end}}
```

**Step 4: 改造 fixup.tmpl**

```
{{if .ColdContext}}
{{.ColdContext}}

{{end}}
{{if .WarmContext}}
{{.WarmContext}}

{{end}}
你正在项目 {{.ProjectName}} 中修复上一轮审查问题。

{{if .PreviousReview}}上一轮审查结论：
{{.PreviousReview}}
{{end}}
{{if .RetryError}}最近错误：
{{.RetryError}}
{{end}}

请完成修复并确保测试通过。
{{if .HotContext}}

{{.HotContext}}
{{end}}
```

**Step 5: 改造 requirements.tmpl**

```
{{if .ColdContext}}
{{.ColdContext}}

{{end}}
{{if .WarmContext}}
{{.WarmContext}}

{{end}}
你正在项目 {{.ProjectName}} ({{.RepoPath}}) 中工作。

请将以下需求结构化，输出一份清晰的需求文档：

{{.Requirements}}

{{if .ExecutionContext}}执行上下文（JSON）：
{{.ExecutionContext}}
{{end}}

要求：
1. 明确功能边界
2. 列出验收标准
3. 识别技术约束
{{if .HotContext}}

{{.HotContext}}
{{end}}
```

**Step 6: 验证编译**

Run: `go build ./internal/engine/...`
Expected: BUILD SUCCESS

**Step 7: Commit**

```bash
git add internal/engine/prompts.go internal/engine/prompt_templates/
git commit -m "feat(engine): extend PromptVars with layered context and update templates"
```

---

### Task 6: PromptBuilder 实现

**Files:**
- Create: `internal/engine/prompt_builder.go`
- Create: `internal/engine/prompt_builder_test.go`

**Step 1: 写失败测试**

```go
package engine

import (
	"strings"
	"testing"
)

// mockMemory implements core.Memory for testing.
type mockMemory struct {
	cold string
	warm string
	hot  string
}

func (m *mockMemory) RecallCold(issueID string) (string, error) { return m.cold, nil }
func (m *mockMemory) RecallWarm(issueID string) (string, error) { return m.warm, nil }
func (m *mockMemory) RecallHot(issueID, runID string) (string, error) { return m.hot, nil }

func TestPromptBuilder_WithAllLayers(t *testing.T) {
	mem := &mockMemory{
		cold: "## 任务背景\n标题: Auth system",
		warm: "## 父任务\n标题: Platform",
		hot:  "## 最近事件\n- review_approved",
	}
	pb := NewPromptBuilder(mem)

	vars := PromptVars{
		ProjectName:  "test-project",
		WorktreePath: "/tmp/wt",
		Requirements: "Build login page",
	}
	prompt, err := pb.Build("issue-1", "run-1", "implement", vars)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !strings.Contains(prompt, "Auth system") {
		t.Error("prompt should contain cold context")
	}
	if !strings.Contains(prompt, "Platform") {
		t.Error("prompt should contain warm context")
	}
	if !strings.Contains(prompt, "review_approved") {
		t.Error("prompt should contain hot context")
	}
	if !strings.Contains(prompt, "Build login page") {
		t.Error("prompt should contain requirements")
	}
}

func TestPromptBuilder_NoMemory(t *testing.T) {
	pb := NewPromptBuilder(nil)

	vars := PromptVars{
		ProjectName:  "test-project",
		WorktreePath: "/tmp/wt",
		Requirements: "Build login page",
	}
	prompt, err := pb.Build("issue-1", "run-1", "implement", vars)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !strings.Contains(prompt, "Build login page") {
		t.Error("prompt should still contain requirements when memory is nil")
	}
}

func TestPromptBuilder_PartialMemory(t *testing.T) {
	mem := &mockMemory{
		cold: "## 任务背景\n标题: Auth",
		warm: "",
		hot:  "",
	}
	pb := NewPromptBuilder(mem)

	vars := PromptVars{
		ProjectName:  "test-project",
		WorktreePath: "/tmp/wt",
		Requirements: "Build login page",
	}
	prompt, err := pb.Build("issue-1", "run-1", "implement", vars)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !strings.Contains(prompt, "Auth") {
		t.Error("prompt should contain cold context")
	}
	if !strings.Contains(prompt, "Build login page") {
		t.Error("prompt should contain requirements")
	}
}
```

**Step 2: 运行测试验证失败**

Run: `go test ./internal/engine/... -run TestPromptBuilder -v`
Expected: FAIL (NewPromptBuilder not defined)

**Step 3: 写实现**

```go
package engine

import (
	"log/slog"

	"github.com/yoke233/ai-workflow/internal/core"
)

// PromptBuilder assembles layered prompts using Memory context.
type PromptBuilder struct {
	memory core.Memory
}

// NewPromptBuilder creates a PromptBuilder. memory may be nil (graceful degradation).
func NewPromptBuilder(memory core.Memory) *PromptBuilder {
	return &PromptBuilder{memory: memory}
}

// Build assembles a prompt with layered context injected into the template.
// issueID and runID are used to query Memory. stage selects the .tmpl template.
// baseVars contains the existing prompt variables (ProjectName, Requirements, etc.).
func (b *PromptBuilder) Build(issueID, runID, stage string, baseVars PromptVars) (string, error) {
	if b.memory != nil {
		cold, err := b.memory.RecallCold(issueID)
		if err != nil {
			slog.Warn("PromptBuilder: RecallCold failed", "error", err, "issue", issueID)
		} else {
			baseVars.ColdContext = cold
		}

		warm, err := b.memory.RecallWarm(issueID)
		if err != nil {
			slog.Warn("PromptBuilder: RecallWarm failed", "error", err, "issue", issueID)
		} else {
			baseVars.WarmContext = warm
		}

		hot, err := b.memory.RecallHot(issueID, runID)
		if err != nil {
			slog.Warn("PromptBuilder: RecallHot failed", "error", err, "issue", issueID)
		} else {
			baseVars.HotContext = hot
		}
	}

	return RenderPrompt(stage, baseVars)
}
```

**Step 4: 运行测试验证通过**

Run: `go test ./internal/engine/... -run TestPromptBuilder -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/engine/prompt_builder.go internal/engine/prompt_builder_test.go
git commit -m "feat(engine): add PromptBuilder with layered context injection"
```

---

### Task 7: Executor 集成 PromptBuilder

**Files:**
- Modify: `internal/engine/executor.go`
- Modify: `internal/engine/executor_stages.go`

**Step 1: 在 Executor 添加 promptBuilder 字段**

在 `internal/engine/executor.go` 的 Executor 结构体中，`testStageFunc` 之前添加：

```go
	promptBuilder *PromptBuilder
```

**Step 2: 添加 SetMemory 方法**

在 `SetWorkspace` 方法之后添加：

```go
// SetMemory configures the layered memory for prompt building.
func (e *Executor) SetMemory(memory core.Memory) {
	e.promptBuilder = NewPromptBuilder(memory)
}
```

**Step 3: 改造 executor_stages.go 的 executeStage()**

将 `executor_stages.go` 中第 55-67 行的 prompt 构建代码：

```go
	prompt, err := RenderPrompt(promptStage, PromptVars{
		ProjectName:       project.Name,
		RepoPath:          project.RepoPath,
		WorktreePath:      p.WorktreePath,
		Requirements:      p.Description,
		ExecutionContext:   executionContext,
		RetryError:        p.ErrorMessage,
		MergeConflictHint: mergeConflictHintFromConfig(p.Config),
		RetryCount:        p.TotalRetries,
	})
```

替换为：

```go
	vars := PromptVars{
		ProjectName:       project.Name,
		RepoPath:          project.RepoPath,
		WorktreePath:      p.WorktreePath,
		Requirements:      p.Description,
		ExecutionContext:   executionContext,
		RetryError:        p.ErrorMessage,
		MergeConflictHint: mergeConflictHintFromConfig(p.Config),
		RetryCount:        p.TotalRetries,
	}
	var prompt string
	if e.promptBuilder != nil {
		prompt, err = e.promptBuilder.Build(p.IssueID, p.ID, promptStage, vars)
	} else {
		prompt, err = RenderPrompt(promptStage, vars)
	}
```

**Step 4: 验证编译**

Run: `go build ./internal/engine/...`
Expected: BUILD SUCCESS

**Step 5: 运行现有 engine 测试确保不破坏**

Run: `go test ./internal/engine/... -v -timeout 60s`
Expected: ALL PASS（promptBuilder 为 nil 时走原路径）

**Step 6: Commit**

```bash
git add internal/engine/executor.go internal/engine/executor_stages.go
git commit -m "feat(engine): integrate PromptBuilder into stage execution"
```

---

### Task 8: 启动链路注入

**Files:**
- Modify: `cmd/ai-flow/commands.go` 或创建 Executor 的位置

**Step 1: 找到 Executor 创建的位置**

在 `cmd/ai-flow/commands.go` 中搜索 `engine.NewExecutor` 或 `executor.Set`。

**Step 2: 在 Executor 创建后注入 Memory**

在 `executor := engine.NewExecutor(...)` 之后添加：

```go
executor.SetMemory(storesqlite.NewSQLiteMemory(sqliteStore))
```

其中 `sqliteStore` 是 `*storesqlite.SQLiteStore` 类型的实例。如果当前只有 `core.Store` 接口，需要用类型断言：

```go
if ss, ok := store.(*storesqlite.SQLiteStore); ok {
	executor.SetMemory(storesqlite.NewSQLiteMemory(ss))
}
```

**Step 3: 验证编译**

Run: `go build ./cmd/ai-flow/...`
Expected: BUILD SUCCESS

**Step 4: Commit**

```bash
git add cmd/ai-flow/commands.go
git commit -m "feat: wire SQLiteMemory into executor startup chain"
```

---

### Task 9: 全量测试

**Step 1: 运行全部后端测试**

Run: `pwsh -NoProfile -File ./scripts/test/backend-all.ps1`
Expected: ALL PASS

**Step 2: 运行前端类型检查**

Run: `npm --prefix web run typecheck`
Expected: NO ERRORS

**Step 3: 如有失败，修复并 commit**

```bash
git commit -m "fix(promptbuilder): address test failures from integration"
```
