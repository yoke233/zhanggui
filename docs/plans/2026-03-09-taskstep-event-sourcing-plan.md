# TaskStep 事件溯源 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 在现有 Issue + Run 双模型上引入 TaskStep 事件溯源层，让 Issue.Status 成为从 TaskStep 派生的缓存，并在前端构建 Issue 流程树视图。

**Architecture:** TaskStep 作为业务事实的唯一来源，写入时在同一 SQLite 事务内原子更新 Issue.Status。三层数据分离：TaskStep（业务事实）、run_events（执行追溯）、review_records（审核细节）。前端 IssueFlowTree 组件按层级懒加载展示完整流程。

**Tech Stack:** Go 1.22+ / SQLite (modernc.org) / React + TypeScript + Tailwind / Zustand / WebSocket

**Design doc:** `docs/plans/2026-03-09-taskstep-event-sourcing-design.md`

---

## Task 1: TaskStep 模型和 Action 常量

**Files:**
- Create: `internal/core/task_step.go`
- Test: `internal/core/task_step_test.go`

**Step 1: Write the test**

```go
// internal/core/task_step_test.go
package core

import (
	"testing"
	"time"
)

func TestTaskStepActionDeriveStatus(t *testing.T) {
	tests := []struct {
		action     TaskStepAction
		wantStatus IssueStatus
		wantDerived bool
	}{
		{StepCreated, IssueStatusDraft, true},
		{StepSubmittedForReview, IssueStatusReviewing, true},
		{StepReviewApproved, IssueStatusQueued, true},
		{StepReviewRejected, IssueStatusDraft, true},
		{StepReady, IssueStatusReady, true},
		{StepExecutionStarted, IssueStatusExecuting, true},
		{StepMergeStarted, IssueStatusMerging, true},
		{StepMergeCompleted, IssueStatusDone, true},
		{StepFailed, IssueStatusFailed, true},
		{StepAbandoned, IssueStatusAbandoned, true},
		{StepDecomposeStarted, IssueStatusDecomposing, true},
		{StepDecomposed, IssueStatusDecomposed, true},
		{StepSuperseded, IssueStatusSuperseded, true},
		// Run-level actions don't derive Issue status
		{StepRunCreated, "", false},
		{StepRunStarted, "", false},
		{StepStageStarted, "", false},
		{StepStageCompleted, "", false},
		{StepStageFailed, "", false},
		{StepRunCompleted, "", false},
	}
	for _, tt := range tests {
		t.Run(string(tt.action), func(t *testing.T) {
			got, ok := tt.action.DeriveIssueStatus()
			if ok != tt.wantDerived {
				t.Fatalf("DeriveIssueStatus(%q) derived=%v, want %v", tt.action, ok, tt.wantDerived)
			}
			if ok && got != tt.wantStatus {
				t.Fatalf("DeriveIssueStatus(%q) = %q, want %q", tt.action, got, tt.wantStatus)
			}
		})
	}
}

func TestTaskStepValidate(t *testing.T) {
	valid := TaskStep{
		ID:        "step-001",
		IssueID:   "issue-20260309-abc",
		Action:    StepCreated,
		CreatedAt: time.Now(),
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	noID := valid
	noID.ID = ""
	if err := noID.Validate(); err == nil {
		t.Fatal("expected error for empty ID")
	}

	noIssue := valid
	noIssue.IssueID = ""
	if err := noIssue.Validate(); err == nil {
		t.Fatal("expected error for empty IssueID")
	}

	badAction := valid
	badAction.Action = "invalid_action"
	if err := badAction.Validate(); err == nil {
		t.Fatal("expected error for invalid action")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestTaskStep -v`
Expected: FAIL (types not defined)

**Step 3: Write the implementation**

```go
// internal/core/task_step.go
package core

import (
	"fmt"
	"strings"
	"time"
)

// TaskStepAction represents an action recorded in a TaskStep.
type TaskStepAction string

// Issue state-transition actions.
const (
	StepCreated            TaskStepAction = "created"
	StepSubmittedForReview TaskStepAction = "submitted_for_review"
	StepReviewApproved     TaskStepAction = "review_approved"
	StepReviewRejected     TaskStepAction = "review_rejected"
	StepQueued             TaskStepAction = "queued"
	StepReady              TaskStepAction = "ready"
	StepExecutionStarted   TaskStepAction = "execution_started"
	StepMergeStarted       TaskStepAction = "merge_started"
	StepMergeCompleted     TaskStepAction = "merge_completed"
	StepFailed             TaskStepAction = "failed"
	StepAbandoned          TaskStepAction = "abandoned"
	StepDecomposeStarted   TaskStepAction = "decompose_started"
	StepDecomposed         TaskStepAction = "decomposed"
	StepSuperseded         TaskStepAction = "superseded"
)

// Run-level actions (do not change Issue.Status).
const (
	StepRunCreated     TaskStepAction = "run_created"
	StepRunStarted     TaskStepAction = "run_started"
	StepStageStarted   TaskStepAction = "stage_started"
	StepStageCompleted TaskStepAction = "stage_completed"
	StepStageFailed    TaskStepAction = "stage_failed"
	StepRunCompleted   TaskStepAction = "run_completed"
)

// actionToStatus maps issue-level actions to their resulting IssueStatus.
var actionToStatus = map[TaskStepAction]IssueStatus{
	StepCreated:            IssueStatusDraft,
	StepSubmittedForReview: IssueStatusReviewing,
	StepReviewApproved:     IssueStatusQueued,
	StepReviewRejected:     IssueStatusDraft,
	StepQueued:             IssueStatusQueued,
	StepReady:              IssueStatusReady,
	StepExecutionStarted:   IssueStatusExecuting,
	StepMergeStarted:       IssueStatusMerging,
	StepMergeCompleted:     IssueStatusDone,
	StepFailed:             IssueStatusFailed,
	StepAbandoned:          IssueStatusAbandoned,
	StepDecomposeStarted:   IssueStatusDecomposing,
	StepDecomposed:         IssueStatusDecomposed,
	StepSuperseded:         IssueStatusSuperseded,
}

var validActions = map[TaskStepAction]struct{}{
	StepCreated: {}, StepSubmittedForReview: {}, StepReviewApproved: {},
	StepReviewRejected: {}, StepQueued: {}, StepReady: {},
	StepExecutionStarted: {}, StepMergeStarted: {}, StepMergeCompleted: {},
	StepFailed: {}, StepAbandoned: {}, StepDecomposeStarted: {},
	StepDecomposed: {}, StepSuperseded: {},
	StepRunCreated: {}, StepRunStarted: {}, StepStageStarted: {},
	StepStageCompleted: {}, StepStageFailed: {}, StepRunCompleted: {},
}

// DeriveIssueStatus returns the IssueStatus this action implies.
// Returns ("", false) for run-level actions that don't change Issue status.
func (a TaskStepAction) DeriveIssueStatus() (IssueStatus, bool) {
	s, ok := actionToStatus[a]
	return s, ok
}

// TaskStep records a single business fact in the issue lifecycle.
type TaskStep struct {
	ID        string         `json:"id"`
	IssueID   string         `json:"issue_id"`
	RunID     string         `json:"run_id,omitempty"`
	AgentID   string         `json:"agent_id,omitempty"`
	Action    TaskStepAction `json:"action"`
	StageID   string         `json:"stage_id,omitempty"`
	Input     string         `json:"input,omitempty"`
	Output    string         `json:"output,omitempty"`
	Note      string         `json:"note,omitempty"`
	RefID     string         `json:"ref_id,omitempty"`
	RefType   string         `json:"ref_type,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// Validate checks required TaskStep fields.
func (s TaskStep) Validate() error {
	if strings.TrimSpace(s.ID) == "" {
		return fmt.Errorf("task step ID is required")
	}
	if strings.TrimSpace(s.IssueID) == "" {
		return fmt.Errorf("task step issue_id is required")
	}
	if _, ok := validActions[s.Action]; !ok {
		return fmt.Errorf("invalid task step action %q", s.Action)
	}
	return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestTaskStep -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/core/task_step.go internal/core/task_step_test.go
git commit -m "feat(core): add TaskStep model and Action constants"
```

---

## Task 2: Store 接口扩展

**Files:**
- Modify: `internal/core/store.go:58-113`

**Step 1: Add TaskStep methods to Store interface**

Add these lines before `Close() error` in the Store interface (after line 110):

```go
	// TaskStep event sourcing.
	// SaveTaskStep persists a step and atomically updates Issue.Status if the
	// action implies a state transition. Returns the (possibly new) IssueStatus.
	SaveTaskStep(step *TaskStep) (IssueStatus, error)
	ListTaskSteps(issueID string) ([]TaskStep, error)
	RebuildIssueStatus(issueID string) (IssueStatus, error)
```

**Step 2: Run compilation check**

Run: `go build ./internal/core/...`
Expected: PASS (interface change, implementations will fail later)

**Step 3: Commit**

```bash
git add internal/core/store.go
git commit -m "feat(core): add TaskStep methods to Store interface"
```

---

## Task 3: SQLite 迁移 — task_steps 表

**Files:**
- Modify: `internal/plugins/store-sqlite/migrations.go`

**Step 1: Add migration V10**

In `migrations.go`, bump `schemaVersion` from 9 to 10, add migration call in `applyMigrations()`, and add the migration function:

Change `const schemaVersion = 9` to `const schemaVersion = 10`.

Add after the `currentVersion < 9` block (before `migrateBackfillLegacyColumns`):

```go
	if currentVersion < 10 {
		if err := migrateAddTaskSteps(db); err != nil {
			return fmt.Errorf("migration v10 (task_steps): %w", err)
		}
	}
```

Add the migration function at the end of the file:

```go
func migrateAddTaskSteps(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS task_steps (
	id         TEXT PRIMARY KEY,
	issue_id   TEXT NOT NULL,
	run_id     TEXT NOT NULL DEFAULT '',
	agent_id   TEXT NOT NULL DEFAULT '',
	action     TEXT NOT NULL,
	stage_id   TEXT NOT NULL DEFAULT '',
	input      TEXT NOT NULL DEFAULT '',
	output     TEXT NOT NULL DEFAULT '',
	note       TEXT NOT NULL DEFAULT '',
	ref_id     TEXT NOT NULL DEFAULT '',
	ref_type   TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_task_steps_issue ON task_steps(issue_id, created_at);
CREATE INDEX IF NOT EXISTS idx_task_steps_run ON task_steps(run_id, created_at);
`)
	return err
}
```

**Step 2: Test migration runs**

Run: `go test ./internal/plugins/store-sqlite/ -run TestNew -v -count=1`
Expected: PASS (existing NewStore tests open DB and run migrations)

**Step 3: Commit**

```bash
git add internal/plugins/store-sqlite/migrations.go
git commit -m "feat(store-sqlite): add task_steps table migration V10"
```

---

## Task 4: SQLite SaveTaskStep 实现（原子事务）

**Files:**
- Modify: `internal/plugins/store-sqlite/store.go`
- Test: `internal/plugins/store-sqlite/store_test.go` (if exists, otherwise create `internal/plugins/store-sqlite/task_step_test.go`)

**Step 1: Write the test**

```go
// internal/plugins/store-sqlite/task_step_test.go
package storesqlite

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func setupTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	s, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSaveTaskStep_IssueStatusDerivation(t *testing.T) {
	s := setupTestStore(t)

	// Create a project and issue first.
	proj := &core.Project{ID: "proj-1", Name: "test"}
	if err := s.CreateProject(proj); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	issue := &core.Issue{
		ID:        "issue-test-001",
		ProjectID: "proj-1",
		Title:     "Test Issue",
		Template:  "standard",
		Status:    core.IssueStatusDraft,
		State:     core.IssueStateOpen,
	}
	if err := s.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	// Write a TaskStep that transitions to reviewing.
	step := &core.TaskStep{
		ID:        "step-001",
		IssueID:   issue.ID,
		Action:    core.StepSubmittedForReview,
		AgentID:   "system",
		CreatedAt: time.Now(),
	}
	newStatus, err := s.SaveTaskStep(step)
	if err != nil {
		t.Fatalf("SaveTaskStep: %v", err)
	}
	if newStatus != core.IssueStatusReviewing {
		t.Fatalf("expected status %q, got %q", core.IssueStatusReviewing, newStatus)
	}

	// Verify issue status was updated in DB.
	got, err := s.GetIssue(issue.ID)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if got.Status != core.IssueStatusReviewing {
		t.Fatalf("issue.Status = %q, want %q", got.Status, core.IssueStatusReviewing)
	}
}

func TestSaveTaskStep_RunLevelAction_NoStatusChange(t *testing.T) {
	s := setupTestStore(t)

	proj := &core.Project{ID: "proj-1", Name: "test"}
	s.CreateProject(proj)
	issue := &core.Issue{
		ID: "issue-test-002", ProjectID: "proj-1",
		Title: "Test", Template: "standard",
		Status: core.IssueStatusExecuting, State: core.IssueStateOpen,
	}
	s.CreateIssue(issue)

	step := &core.TaskStep{
		ID:        "step-run-001",
		IssueID:   issue.ID,
		RunID:     "run-001",
		Action:    core.StepStageStarted,
		StageID:   "implement",
		CreatedAt: time.Now(),
	}
	newStatus, err := s.SaveTaskStep(step)
	if err != nil {
		t.Fatalf("SaveTaskStep: %v", err)
	}
	// Run-level action should not change issue status.
	if newStatus != core.IssueStatusExecuting {
		t.Fatalf("expected status %q unchanged, got %q", core.IssueStatusExecuting, newStatus)
	}
}

func TestListTaskSteps(t *testing.T) {
	s := setupTestStore(t)

	proj := &core.Project{ID: "proj-1", Name: "test"}
	s.CreateProject(proj)
	issue := &core.Issue{
		ID: "issue-test-003", ProjectID: "proj-1",
		Title: "Test", Template: "standard",
		Status: core.IssueStatusDraft, State: core.IssueStateOpen,
	}
	s.CreateIssue(issue)

	for i, action := range []core.TaskStepAction{core.StepCreated, core.StepSubmittedForReview} {
		s.SaveTaskStep(&core.TaskStep{
			ID:        fmt.Sprintf("step-%03d", i),
			IssueID:   issue.ID,
			Action:    action,
			CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
		})
	}

	steps, err := s.ListTaskSteps(issue.ID)
	if err != nil {
		t.Fatalf("ListTaskSteps: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}
	if steps[0].Action != core.StepCreated {
		t.Fatalf("first step action = %q, want %q", steps[0].Action, core.StepCreated)
	}
}
```

Need to add `"fmt"` to imports.

**Step 2: Run test to verify it fails**

Run: `go test ./internal/plugins/store-sqlite/ -run TestSaveTaskStep -v`
Expected: FAIL (SaveTaskStep not implemented)

**Step 3: Implement SaveTaskStep, ListTaskSteps, RebuildIssueStatus**

Add to `internal/plugins/store-sqlite/store.go`:

```go
func (s *Store) SaveTaskStep(step *core.TaskStep) (core.IssueStatus, error) {
	if err := step.Validate(); err != nil {
		return "", fmt.Errorf("invalid task step: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// 1. Insert the task step.
	createdAt := step.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	_, err = tx.Exec(`INSERT INTO task_steps
		(id, issue_id, run_id, agent_id, action, stage_id, input, output, note, ref_id, ref_type, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		step.ID, step.IssueID, step.RunID, step.AgentID,
		string(step.Action), step.StageID,
		step.Input, step.Output, step.Note,
		step.RefID, step.RefType, createdAt,
	)
	if err != nil {
		return "", fmt.Errorf("insert task_step: %w", err)
	}

	// 2. Derive and update Issue.Status if applicable.
	derivedStatus, shouldUpdate := step.Action.DeriveIssueStatus()
	if shouldUpdate {
		_, err = tx.Exec(`UPDATE issues SET status = ?, updated_at = ? WHERE id = ?`,
			string(derivedStatus), time.Now(), step.IssueID,
		)
		if err != nil {
			return "", fmt.Errorf("update issue status: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit tx: %w", err)
	}

	if shouldUpdate {
		return derivedStatus, nil
	}
	// Return current status when no derivation happened.
	issue, err := s.GetIssue(step.IssueID)
	if err != nil {
		return "", err
	}
	return issue.Status, nil
}

func (s *Store) ListTaskSteps(issueID string) ([]core.TaskStep, error) {
	rows, err := s.db.Query(`SELECT id, issue_id, run_id, agent_id, action, stage_id,
		input, output, note, ref_id, ref_type, created_at
		FROM task_steps WHERE issue_id = ? ORDER BY created_at ASC, rowid ASC`, issueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var steps []core.TaskStep
	for rows.Next() {
		var st core.TaskStep
		var action string
		if err := rows.Scan(&st.ID, &st.IssueID, &st.RunID, &st.AgentID,
			&action, &st.StageID, &st.Input, &st.Output, &st.Note,
			&st.RefID, &st.RefType, &st.CreatedAt); err != nil {
			return nil, err
		}
		st.Action = core.TaskStepAction(action)
		steps = append(steps, st)
	}
	return steps, rows.Err()
}

func (s *Store) RebuildIssueStatus(issueID string) (core.IssueStatus, error) {
	steps, err := s.ListTaskSteps(issueID)
	if err != nil {
		return "", err
	}
	status := core.IssueStatusDraft
	for _, st := range steps {
		if derived, ok := st.Action.DeriveIssueStatus(); ok {
			status = derived
		}
	}
	_, err = s.db.Exec(`UPDATE issues SET status = ?, updated_at = ? WHERE id = ?`,
		string(status), time.Now(), issueID)
	return status, err
}
```

Also need to add `"time"` to imports if not already present.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/plugins/store-sqlite/ -run "TestSaveTaskStep|TestListTaskSteps" -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/plugins/store-sqlite/store.go internal/plugins/store-sqlite/task_step_test.go
git commit -m "feat(store-sqlite): implement SaveTaskStep with atomic status derivation"
```

---

## Task 5: 补全其他 Store 实现的编译兼容

**Files:**
- Modify: any other Store implementation or mock that needs the new methods

**Step 1: Find all Store implementations**

Run: `grep -rn "func.*Store.*SaveIssue\b" internal/ --include="*.go" | grep -v _test.go`

Check which files implement the Store interface. Add stub implementations for `SaveTaskStep`, `ListTaskSteps`, `RebuildIssueStatus` to any mock or alternative store.

**Step 2: Build the whole project**

Run: `go build ./...`
Expected: PASS (all Store implementations compile)

**Step 3: Commit**

```bash
git add -A
git commit -m "fix: add TaskStep stub methods to all Store implementations"
```

---

## Task 6: 创建 transition helper — 写 TaskStep 替代直接状态修改

**Files:**
- Modify: `internal/teamleader/issue_transition.go`
- Test: `internal/teamleader/issue_transition_test.go`

**Step 1: Write the test**

```go
// internal/teamleader/issue_transition_test.go
package teamleader

import (
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// mockStepStore is a minimal Store mock for testing TaskStep writes.
// (Use existing mock patterns from the package, or define inline.)

func TestTransitionIssueViaStep(t *testing.T) {
	// Test that transitionIssueStatus still works as before.
	issue := &core.Issue{
		ID:     "issue-001",
		Status: core.IssueStatusDraft,
	}
	err := transitionIssueStatus(issue, core.IssueStatusReviewing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if issue.Status != core.IssueStatusReviewing {
		t.Fatalf("status = %q, want %q", issue.Status, core.IssueStatusReviewing)
	}
}
```

**Step 2: Run test**

Run: `go test ./internal/teamleader/ -run TestTransitionIssueViaStep -v`
Expected: PASS (existing function still works)

**Step 3: Add NewTaskStepID helper to core**

Add to `internal/core/task_step.go`:

```go
// NewTaskStepID generates a unique TaskStep ID.
func NewTaskStepID() string {
	return fmt.Sprintf("step-%s-%s", time.Now().Format("20060102-150405"), randomHex(4))
}
```

**Step 4: Commit**

```bash
git add internal/core/task_step.go internal/teamleader/issue_transition_test.go
git commit -m "feat(core): add NewTaskStepID helper"
```

---

## Task 7: 重构 teamleader/manager.go — 用 TaskStep 记录状态变更

**Files:**
- Modify: `internal/teamleader/manager.go`

This is the largest refactoring task. The pattern at each status change point is:

**Before:**
```go
transitionIssueStatus(&issue, core.IssueStatusQueued)
issue.UpdatedAt = time.Now()
m.store.SaveIssue(&issue)
```

**After:**
```go
transitionIssueStatus(&issue, core.IssueStatusQueued)
m.store.SaveTaskStep(&core.TaskStep{
    ID:        core.NewTaskStepID(),
    IssueID:   issue.ID,
    Action:    core.StepReviewApproved,
    AgentID:   "system",
    Note:      "review approved",
    CreatedAt: time.Now(),
})
// SaveTaskStep atomically updates issue.Status in DB
issue.UpdatedAt = time.Now()
m.store.SaveIssue(&issue) // still needed for other fields (RunID, etc.)
```

**Step 1: Refactor each status change location in manager.go**

Key locations to modify (based on research):

1. **Line ~250**: `SubmitForReview()` — `reviewing → queued` — add `StepReviewApproved` step
2. **Line ~340**: `applyIssueApprove()` epic path — `reviewing → decomposing` — add `StepDecomposeStarted` step
3. **Line ~365**: `applyIssueApprove()` normal path — `reviewing → queued` — add `StepReviewApproved` step
4. **Line ~390**: `markApproveDispatchFailure()` — `reviewing → failed` — add `StepFailed` step
5. **Line ~422**: `applyIssueReject()` — `reviewing → draft` — add `StepReviewRejected` step
6. **Line ~440**: `applyIssueAbandon()` — `* → abandoned` — add `StepAbandoned` step

**Pattern for each change:**

After the `transitionIssueStatus()` call and before or alongside `SaveIssue()`, add:

```go
if _, err := m.store.SaveTaskStep(&core.TaskStep{
    ID:        core.NewTaskStepID(),
    IssueID:   updated.ID,
    Action:    core.StepReviewApproved, // appropriate action
    AgentID:   "system",
    Note:      "approved by review gate",
    CreatedAt: time.Now(),
}); err != nil {
    slog.Warn("failed to save task step", "error", err, "issue", updated.ID)
}
```

Note: TaskStep write failures are logged but don't block the main flow. The `SaveIssue()` call remains for updating non-status fields.

**Step 2: Run existing tests**

Run: `go test ./internal/teamleader/ -v -count=1`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/teamleader/manager.go
git commit -m "feat(teamleader): record TaskStep on manager status transitions"
```

---

## Task 8: 重构 scheduler_events.go + scheduler_dispatch.go

**Files:**
- Modify: `internal/teamleader/scheduler_events.go`
- Modify: `internal/teamleader/scheduler_dispatch.go`
- Modify: `internal/teamleader/scheduler.go`

**Step 1: Add TaskStep writes to scheduler_events.go**

Key locations:
1. **Line ~46**: `executing → merging` — add `StepMergeStarted` step
2. **Line ~55**: `executing → done` — add `StepMergeCompleted` step (auto_merge=false)
3. **Line ~63**: `executing → failed` — add `StepFailed` step
4. **Line ~80**: `merging → done` — add `StepMergeCompleted` step
5. **Line ~88**: `merging → failed` — add `StepFailed` step
6. **Line ~109**: `merging → queued` (retry) — add `StepQueued` step
7. **Line ~250**: block policy `* → failed` — add `StepFailed` step

**Step 2: Add TaskStep writes to scheduler_dispatch.go**

Key locations:
1. **Line ~59**: `ready → executing` — add `StepExecutionStarted` step
2. **Line ~109**: rollback `executing → ready` — add `StepReady` step
3. **Line ~245**: `queued → ready` — add `StepReady` step

**Step 3: Add TaskStep writes to scheduler.go**

Key location:
1. **Line ~235**: recovery `executing/merging → queued` — add `StepQueued` step with note "recovered on restart"

**Step 4: Run tests**

Run: `go test ./internal/teamleader/ -v -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/teamleader/scheduler_events.go internal/teamleader/scheduler_dispatch.go internal/teamleader/scheduler.go
git commit -m "feat(teamleader): record TaskStep on scheduler status transitions"
```

---

## Task 9: 重构 decompose + child_completion + a2a_bridge

**Files:**
- Modify: `internal/teamleader/decompose_handler.go`
- Modify: `internal/teamleader/child_completion.go`
- Modify: `internal/teamleader/a2a_bridge.go`

**Step 1: Add TaskStep writes to decompose_handler.go**

1. **Line ~175**: `decomposing → decomposed` — add `StepDecomposed` step
2. **Line ~209**: `decomposing → failed` — add `StepFailed` step

**Step 2: Add TaskStep writes to child_completion.go**

1. **Line ~134**: `decomposed → done` — add `StepMergeCompleted` step with note "all children done"
2. **Line ~167**: `decomposed → failed` — add `StepFailed` step with note "child failed (block policy)"

**Step 3: Add TaskStep writes to a2a_bridge.go**

1. **Line ~162**: `draft → reviewing` — add `StepSubmittedForReview` step

**Step 4: Run tests**

Run: `go test ./internal/teamleader/ -v -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/teamleader/decompose_handler.go internal/teamleader/child_completion.go internal/teamleader/a2a_bridge.go
git commit -m "feat(teamleader): record TaskStep on decompose/child/a2a transitions"
```

---

## Task 10: 重构 engine/executor.go — 记录 Run 级别 TaskStep

**Files:**
- Modify: `internal/engine/executor.go`

**Step 1: Add TaskStep writes at stage boundaries**

The executor needs the store and issue ID to write TaskSteps. Check how `executor.go` accesses the store and issue info.

Key locations:
1. **Run start** (~line 186): add `StepRunStarted` step
2. **Stage start** (~line 212): add `StepStageStarted` step with `StageID`
3. **Stage complete** (~line 268): add `StepStageCompleted` step with `StageID`
4. **Stage failed** (~line 303): add `StepStageFailed` step with `StageID` and error
5. **Run done** (~line 424): add `StepRunCompleted` step
6. **Run failed** (failRun): add `StepRunCompleted` step with conclusion=failure in note

Pattern for each:

```go
if issueID := p.IssueID; issueID != "" {
    e.store.SaveTaskStep(&core.TaskStep{
        ID:        core.NewTaskStepID(),
        IssueID:   issueID,
        RunID:     p.ID,
        Action:    core.StepStageStarted,
        StageID:   string(stage.Name),
        AgentID:   agentUsed,
        CreatedAt: time.Now(),
    })
}
```

Note: Run-level TaskSteps don't change Issue.Status (DeriveIssueStatus returns false), so they're purely for tracing.

**Step 2: Run engine tests**

Run: `go test ./internal/engine/ -v -count=1`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/engine/executor.go
git commit -m "feat(engine): record TaskStep at stage boundaries"
```

---

## Task 11: Admin ops 补充

**Files:**
- Modify: `internal/web/handlers_admin_ops.go`

**Step 1: Add TaskStep for admin force operations**

At ~line 124 where admin forces status change, add a TaskStep recording the forced transition.

```go
if _, err := h.store.SaveTaskStep(&core.TaskStep{
    ID:      core.NewTaskStepID(),
    IssueID: issue.ID,
    Action:  core.StepReady, // or derive from target status
    AgentID: "admin",
    Note:    fmt.Sprintf("admin force: %s", targetStatus),
    CreatedAt: time.Now(),
}); err != nil {
    slog.Warn("failed to save task step for admin op", "error", err)
}
```

**Step 2: Build check**

Run: `go build ./...`
Expected: PASS

**Step 3: Commit**

```bash
git add internal/web/handlers_admin_ops.go
git commit -m "feat(web): record TaskStep on admin force operations"
```

---

## Task 12: Timeline API 端点

**Files:**
- Modify: `internal/web/handlers_v2.go` (or appropriate handlers file)
- Modify: `internal/web/handlers_v3.go` (route registration)

**Step 1: Write the handler**

The frontend already expects `GET /api/v1/projects/{projectId}/issues/{issueId}/timeline` (defined in `apiClient.ts:886`).

Add a handler that:
1. Loads all TaskSteps for the issue
2. For each `stage_started`/`stage_completed`/`stage_failed` step, include stage metadata
3. For steps with `ref_id`, include ref_type for client-side drill-down
4. Returns JSON array sorted by created_at

```go
func (h *v2IssueHandlers) listIssueTimeline(w http.ResponseWriter, r *http.Request) {
    issueID := chi.URLParam(r, "issueId")
    if issueID == "" {
        http.Error(w, "issue id required", http.StatusBadRequest)
        return
    }

    steps, err := h.store.ListTaskSteps(issueID)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    writeJSON(w, http.StatusOK, map[string]any{
        "steps": steps,
        "total": len(steps),
    })
}
```

**Step 2: Register the route**

In the route registration section, add:

```go
r.Get("/projects/{projectId}/issues/{issueId}/timeline", issueH.listIssueTimeline)
```

**Step 3: Run build check**

Run: `go build ./...`
Expected: PASS

**Step 4: Commit**

```bash
git add internal/web/handlers_v2.go internal/web/handlers_v3.go
git commit -m "feat(api): add GET /issues/{id}/timeline endpoint returning TaskSteps"
```

---

## Task 13: 前端类型定义

**Files:**
- Modify: `web/src/types/api.ts`

**Step 1: Add TaskStep type and API response**

```typescript
// TaskStep represents a single business fact in the issue lifecycle.
export interface TaskStep {
  id: string;
  issue_id: string;
  run_id: string;
  agent_id: string;
  action: TaskStepAction;
  stage_id: string;
  input: string;
  output: string;
  note: string;
  ref_id: string;
  ref_type: string;
  created_at: string;
}

export type TaskStepAction =
  | "created" | "submitted_for_review" | "review_approved" | "review_rejected"
  | "queued" | "ready" | "execution_started"
  | "merge_started" | "merge_completed"
  | "failed" | "abandoned"
  | "decompose_started" | "decomposed" | "superseded"
  | "run_created" | "run_started"
  | "stage_started" | "stage_completed" | "stage_failed"
  | "run_completed";

export interface ListTaskStepsResponse {
  steps: TaskStep[];
  total: number;
}
```

**Step 2: Update apiClient.ts**

Update the existing `listIssueTimeline` method (if it exists) or add one that returns `ListTaskStepsResponse`:

```typescript
async listIssueTaskSteps(projectId: string, issueId: string): Promise<ListTaskStepsResponse> {
  const resp = await this.fetch(`/projects/${projectId}/issues/${issueId}/timeline`);
  return resp.json();
}
```

**Step 3: Type check**

Run: `npm --prefix web run typecheck`
Expected: PASS

**Step 4: Commit**

```bash
git add web/src/types/api.ts web/src/lib/apiClient.ts
git commit -m "feat(web): add TaskStep types and API client method"
```

---

## Task 14: IssueFlowTree 前端组件

**Files:**
- Create: `web/src/components/IssueFlowTree.tsx`

**Step 1: Build the component**

Core structure:
- Level 1: Issue status nodes derived from TaskSteps
- Level 2: Run/review details
- Level 3: Stage list
- Level 4: Lazy-loaded run_events (via existing API)

```typescript
// web/src/components/IssueFlowTree.tsx
import { useState, useEffect } from "react";
import type { TaskStep, RunEvent } from "../types/api";
import { apiClient } from "../lib/apiClient";

interface FlowNodeData {
  step: TaskStep;
  children: TaskStep[];
  expanded: boolean;
}

interface IssueFlowTreeProps {
  projectId: string;
  issueId: string;
  steps: TaskStep[];
}

// Group steps into a tree structure:
// Issue-level steps are top nodes, run-level steps are children.
function buildFlowTree(steps: TaskStep[]): FlowNodeData[] {
  const issueSteps = steps.filter(s => !s.action.startsWith("run_") && !s.action.startsWith("stage_"));
  const runSteps = steps.filter(s => s.action.startsWith("run_") || s.action.startsWith("stage_"));

  return issueSteps.map((step, idx) => {
    const nextIssueStep = issueSteps[idx + 1];
    const children = runSteps.filter(rs => {
      const after = new Date(rs.created_at) >= new Date(step.created_at);
      const before = !nextIssueStep || new Date(rs.created_at) < new Date(nextIssueStep.created_at);
      return after && before;
    });
    // Auto-expand active (last) node
    const isLast = idx === issueSteps.length - 1;
    return { step, children, expanded: isLast };
  });
}

const ACTION_ICONS: Record<string, string> = {
  created: "📋", submitted_for_review: "📤", review_approved: "✅",
  review_rejected: "❌", queued: "📥", ready: "🟢",
  execution_started: "⚡", merge_started: "🔀", merge_completed: "✅",
  failed: "💥", abandoned: "🚫", decompose_started: "🔨",
  decomposed: "📦", superseded: "🔄",
  stage_started: "▶️", stage_completed: "✅", stage_failed: "❌",
  run_created: "🏗️", run_started: "🚀", run_completed: "🏁",
};

export function IssueFlowTree({ projectId, issueId, steps }: IssueFlowTreeProps) {
  const [tree, setTree] = useState<FlowNodeData[]>([]);

  useEffect(() => {
    setTree(buildFlowTree(steps));
  }, [steps]);

  const toggle = (idx: number) => {
    setTree(prev => prev.map((n, i) =>
      i === idx ? { ...n, expanded: !n.expanded } : n
    ));
  };

  if (steps.length === 0) {
    return <div className="text-sm text-zinc-500 p-4">No steps recorded yet.</div>;
  }

  return (
    <div className="space-y-1 font-mono text-sm">
      {tree.map((node, idx) => (
        <FlowNode
          key={node.step.id}
          node={node}
          onToggle={() => toggle(idx)}
          projectId={projectId}
        />
      ))}
    </div>
  );
}

function FlowNode({ node, onToggle, projectId }: {
  node: FlowNodeData; onToggle: () => void; projectId: string;
}) {
  const { step, children, expanded } = node;
  const icon = ACTION_ICONS[step.action] || "●";
  const time = new Date(step.created_at).toLocaleTimeString();
  const hasChildren = children.length > 0;

  return (
    <div className="border-l-2 border-zinc-700 pl-3 ml-2">
      <div
        className="flex items-center gap-2 cursor-pointer hover:bg-zinc-800/50 rounded px-1 py-0.5"
        onClick={onToggle}
      >
        {hasChildren && (
          <span className="text-xs text-zinc-500">{expanded ? "▼" : "▶"}</span>
        )}
        <span>{icon}</span>
        <span className="text-zinc-300">{step.action.replace(/_/g, " ")}</span>
        {step.stage_id && (
          <span className="text-zinc-500">[{step.stage_id}]</span>
        )}
        {step.note && (
          <span className="text-zinc-600 truncate max-w-xs">— {step.note}</span>
        )}
        <span className="text-zinc-600 ml-auto text-xs">{time}</span>
      </div>
      {expanded && children.length > 0 && (
        <div className="ml-4 space-y-0.5 mt-1">
          {children.map(child => (
            <div key={child.id} className="flex items-center gap-2 text-xs text-zinc-400 pl-2">
              <span>{ACTION_ICONS[child.action] || "·"}</span>
              <span>{child.action.replace(/_/g, " ")}</span>
              {child.stage_id && <span className="text-zinc-600">[{child.stage_id}]</span>}
              {child.note && <span className="text-zinc-600 truncate max-w-xs">— {child.note}</span>}
              <span className="text-zinc-700 ml-auto">{new Date(child.created_at).toLocaleTimeString()}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
```

**Step 2: Type check**

Run: `npm --prefix web run typecheck`
Expected: PASS

**Step 3: Commit**

```bash
git add web/src/components/IssueFlowTree.tsx
git commit -m "feat(web): add IssueFlowTree component for issue timeline"
```

---

## Task 15: 集成 IssueFlowTree 到 BoardView

**Files:**
- Modify: `web/src/views/BoardView.tsx`

**Step 1: Import and wire up**

1. Import `IssueFlowTree` component
2. In the Issue detail panel, add a "Flow" tab or replace the existing timeline section
3. Load TaskSteps via `apiClient.listIssueTaskSteps()` when issue detail opens
4. Pass steps to `<IssueFlowTree>`

Key integration point: the existing `timelineEntries` state and `buildTimeline()` function in `BoardView.tsx`.

Add state:

```typescript
const [taskSteps, setTaskSteps] = useState<TaskStep[]>([]);
```

Load steps when issue detail opens (alongside or replacing existing timeline load):

```typescript
apiClient.listIssueTaskSteps(projectId, selectedTaskId).then(res => {
  setTaskSteps(res.steps);
});
```

Render in the detail panel:

```tsx
<IssueFlowTree projectId={projectId} issueId={selectedTaskId} steps={taskSteps} />
```

**Step 2: Type check and build**

Run: `npm --prefix web run typecheck && npm --prefix web run build`
Expected: PASS

**Step 3: Commit**

```bash
git add web/src/views/BoardView.tsx
git commit -m "feat(web): integrate IssueFlowTree into BoardView issue detail"
```

---

## Task 16: 全量构建和测试验证

**Step 1: Run all backend tests**

Run: `pwsh -NoProfile -File ./scripts/test/backend-all.ps1`
Expected: PASS

**Step 2: Run frontend build**

Run: `npm --prefix web run build`
Expected: PASS

**Step 3: Run smoke test (if applicable)**

Run: `pwsh -NoProfile -File ./scripts/test/v2-smoke.ps1`
Expected: PASS

**Step 4: Final commit (if any fixes needed)**

```bash
git add -A
git commit -m "fix: address test failures from TaskStep integration"
```

---

## Summary

| Task | Description | Files | Estimated Steps |
|------|-------------|-------|----------------|
| 1 | TaskStep model + Action constants | core/task_step.go | 5 |
| 2 | Store interface extension | core/store.go | 3 |
| 3 | SQLite migration V10 | store-sqlite/migrations.go | 3 |
| 4 | SaveTaskStep implementation | store-sqlite/store.go | 5 |
| 5 | Compilation compat for all stores | various | 3 |
| 6 | Transition helper | teamleader/issue_transition.go | 4 |
| 7 | Refactor manager.go | teamleader/manager.go | 3 |
| 8 | Refactor scheduler_*.go | teamleader/scheduler*.go | 5 |
| 9 | Refactor decompose/child/a2a | teamleader/*.go | 5 |
| 10 | Refactor executor.go | engine/executor.go | 3 |
| 11 | Admin ops | web/handlers_admin_ops.go | 3 |
| 12 | Timeline API | web/handlers_v2.go | 4 |
| 13 | Frontend types | web/src/types/api.ts | 4 |
| 14 | IssueFlowTree component | web/src/components/ | 3 |
| 15 | BoardView integration | web/src/views/BoardView.tsx | 3 |
| 16 | Full verification | all | 4 |
