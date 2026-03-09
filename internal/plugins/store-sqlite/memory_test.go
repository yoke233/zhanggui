package storesqlite

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func setupMemoryTest(t *testing.T) *SQLiteStore {
	t.Helper()

	store, err := New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	project := &core.Project{ID: "proj-mem", Name: "memory-test", RepoPath: t.TempDir()}
	if err := store.CreateProject(project); err != nil {
		store.Close()
		t.Fatalf("CreateProject: %v", err)
	}

	return store
}

func TestRecallCold(t *testing.T) {
	store := setupMemoryTest(t)
	defer store.Close()

	issue := &core.Issue{
		ID:        "issue-cold-1",
		ProjectID: "proj-mem",
		Title:     "Implement user auth",
		Body:      "We need JWT-based authentication with refresh tokens.",
		Template:  "standard",
		Status:    core.IssueStatusDraft,
	}
	if err := store.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	mem := NewSQLiteMemory(store)
	cold, err := mem.RecallCold(issue.ID)
	if err != nil {
		t.Fatalf("RecallCold: %v", err)
	}
	if !strings.Contains(cold, "Implement user auth") {
		t.Fatalf("RecallCold() missing title: %q", cold)
	}
	if !strings.Contains(cold, "JWT-based authentication") {
		t.Fatalf("RecallCold() missing body preview: %q", cold)
	}
}

func TestRecallCold_NotFound(t *testing.T) {
	store := setupMemoryTest(t)
	defer store.Close()

	mem := NewSQLiteMemory(store)
	cold, err := mem.RecallCold("missing-issue")
	if err != nil {
		t.Fatalf("RecallCold() error = %v", err)
	}
	if cold != "" {
		t.Fatalf("RecallCold() = %q, want empty", cold)
	}
}

func TestRecallWarm_WithParent(t *testing.T) {
	store := setupMemoryTest(t)
	defer store.Close()

	parent := &core.Issue{
		ID:        "issue-parent",
		ProjectID: "proj-mem",
		Title:     "Build auth system",
		Body:      "Complete authentication and authorization system.",
		Template:  "standard",
		Status:    core.IssueStatusDecomposed,
	}
	if err := store.CreateIssue(parent); err != nil {
		t.Fatalf("CreateIssue(parent): %v", err)
	}

	child1 := &core.Issue{
		ID:        "issue-child-1",
		ProjectID: "proj-mem",
		ParentID:  parent.ID,
		Title:     "Implement JWT tokens",
		Template:  "standard",
		Status:    core.IssueStatusDone,
	}
	child2 := &core.Issue{
		ID:        "issue-child-2",
		ProjectID: "proj-mem",
		ParentID:  parent.ID,
		Title:     "Implement user management",
		Template:  "standard",
		Status:    core.IssueStatusExecuting,
	}
	if err := store.CreateIssue(child1); err != nil {
		t.Fatalf("CreateIssue(child1): %v", err)
	}
	if err := store.CreateIssue(child2); err != nil {
		t.Fatalf("CreateIssue(child2): %v", err)
	}

	mem := NewSQLiteMemory(store)
	warm, err := mem.RecallWarm(child2.ID)
	if err != nil {
		t.Fatalf("RecallWarm: %v", err)
	}
	if !strings.Contains(warm, "Build auth system") {
		t.Fatalf("RecallWarm() missing parent title: %q", warm)
	}
	if !strings.Contains(warm, "Implement JWT tokens") {
		t.Fatalf("RecallWarm() missing sibling title: %q", warm)
	}
	if !strings.Contains(warm, string(core.IssueStatusDone)) {
		t.Fatalf("RecallWarm() missing sibling status: %q", warm)
	}
	if strings.Contains(warm, "Implement user management") {
		t.Fatalf("RecallWarm() should not include self: %q", warm)
	}
}

func TestRecallWarm_NoParent(t *testing.T) {
	store := setupMemoryTest(t)
	defer store.Close()

	issue := &core.Issue{
		ID:        "issue-no-parent",
		ProjectID: "proj-mem",
		Title:     "Standalone issue",
		Template:  "standard",
		Status:    core.IssueStatusDraft,
	}
	if err := store.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	mem := NewSQLiteMemory(store)
	warm, err := mem.RecallWarm(issue.ID)
	if err != nil {
		t.Fatalf("RecallWarm() error = %v", err)
	}
	if warm != "" {
		t.Fatalf("RecallWarm() = %q, want empty", warm)
	}
}

func TestRecallHot(t *testing.T) {
	store := setupMemoryTest(t)
	defer store.Close()

	issue := &core.Issue{
		ID:        "issue-hot-1",
		ProjectID: "proj-mem",
		Title:     "Hot context test",
		Template:  "standard",
		Status:    core.IssueStatusExecuting,
	}
	if err := store.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	runID := "run-hot-1"
	if _, err := store.SaveTaskStep(&core.TaskStep{
		ID:        "step-hot-1",
		IssueID:   issue.ID,
		RunID:     runID,
		AgentID:   "system",
		Action:    core.StepExecutionStarted,
		Note:      "run dispatched",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveTaskStep: %v", err)
	}

	if err := store.SaveRunEvent(core.RunEvent{
		RunID:     runID,
		ProjectID: issue.ProjectID,
		IssueID:   issue.ID,
		EventType: string(core.EventStageFailed),
		Stage:     string(core.StageImplement),
		Error:     "compile failed",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveRunEvent: %v", err)
	}

	score := 85
	if err := store.SaveReviewRecord(&core.ReviewRecord{
		IssueID:  issue.ID,
		Round:    1,
		Reviewer: "completeness",
		Verdict:  "approve",
		Summary:  "Looks good, all requirements covered",
		Score:    &score,
	}); err != nil {
		t.Fatalf("SaveReviewRecord: %v", err)
	}

	mem := NewSQLiteMemory(store)
	hot, err := mem.RecallHot(issue.ID, runID)
	if err != nil {
		t.Fatalf("RecallHot: %v", err)
	}
	if !strings.Contains(hot, string(core.StepExecutionStarted)) {
		t.Fatalf("RecallHot() missing task step action: %q", hot)
	}
	if !strings.Contains(hot, string(core.EventStageFailed)) {
		t.Fatalf("RecallHot() missing run event type: %q", hot)
	}
	if !strings.Contains(hot, "compile failed") {
		t.Fatalf("RecallHot() missing run event error: %q", hot)
	}
	if !strings.Contains(hot, "completeness") {
		t.Fatalf("RecallHot() missing reviewer: %q", hot)
	}
	if !strings.Contains(hot, "Looks good") {
		t.Fatalf("RecallHot() missing review summary: %q", hot)
	}
}

func TestRecallHot_FiltersTaskStepsByRun(t *testing.T) {
	store := setupMemoryTest(t)
	defer store.Close()

	issue := &core.Issue{
		ID:        "issue-hot-filter",
		ProjectID: "proj-mem",
		Title:     "Hot filter test",
		Template:  "standard",
		Status:    core.IssueStatusExecuting,
	}
	if err := store.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	if _, err := store.SaveTaskStep(&core.TaskStep{
		ID:        "step-old",
		IssueID:   issue.ID,
		RunID:     "run-old",
		Action:    core.StepStageFailed,
		Note:      "old run failure",
		CreatedAt: time.Now().UTC().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("SaveTaskStep(old): %v", err)
	}
	if _, err := store.SaveTaskStep(&core.TaskStep{
		ID:        "step-new",
		IssueID:   issue.ID,
		RunID:     "run-new",
		Action:    core.StepExecutionStarted,
		Note:      "current run started",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveTaskStep(new): %v", err)
	}

	mem := NewSQLiteMemory(store)
	hot, err := mem.RecallHot(issue.ID, "run-new")
	if err != nil {
		t.Fatalf("RecallHot: %v", err)
	}
	if strings.Contains(hot, "old run failure") {
		t.Fatalf("RecallHot() leaked old run task step: %q", hot)
	}
	if !strings.Contains(hot, "current run started") {
		t.Fatalf("RecallHot() missing current run task step: %q", hot)
	}
}

func TestRecallHot_IgnoresAgentOutputRunEvents(t *testing.T) {
	store := setupMemoryTest(t)
	defer store.Close()

	issue := &core.Issue{
		ID:        "issue-hot-events",
		ProjectID: "proj-mem",
		Title:     "Hot event filtering",
		Template:  "standard",
		Status:    core.IssueStatusExecuting,
	}
	if err := store.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	runID := "run-hot-events"
	if err := store.SaveRunEvent(core.RunEvent{
		RunID:     runID,
		ProjectID: issue.ProjectID,
		IssueID:   issue.ID,
		EventType: string(core.EventAgentOutput),
		Stage:     string(core.StageImplement),
		Data: map[string]string{
			"type":    "prompt",
			"content": "SECRET PROMPT CONTENT",
		},
		CreatedAt: time.Now().UTC().Add(-time.Minute),
	}); err != nil {
		t.Fatalf("SaveRunEvent(prompt): %v", err)
	}
	if err := store.SaveRunEvent(core.RunEvent{
		RunID:     runID,
		ProjectID: issue.ProjectID,
		IssueID:   issue.ID,
		EventType: string(core.EventStageComplete),
		Stage:     string(core.StageImplement),
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveRunEvent(stage_complete): %v", err)
	}

	mem := NewSQLiteMemory(store)
	hot, err := mem.RecallHot(issue.ID, runID)
	if err != nil {
		t.Fatalf("RecallHot: %v", err)
	}
	if strings.Contains(hot, "SECRET PROMPT CONTENT") || strings.Contains(hot, string(core.EventAgentOutput)) {
		t.Fatalf("RecallHot() should not include agent output payloads: %q", hot)
	}
	if !strings.Contains(hot, string(core.EventStageComplete)) {
		t.Fatalf("RecallHot() missing safe lifecycle event: %q", hot)
	}
}

func TestRecallHot_LimitsReviewHistory(t *testing.T) {
	store := setupMemoryTest(t)
	defer store.Close()

	issue := &core.Issue{
		ID:        "issue-hot-reviews",
		ProjectID: "proj-mem",
		Title:     "Hot review limit",
		Template:  "standard",
		Status:    core.IssueStatusExecuting,
	}
	if err := store.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	for i := 1; i <= 7; i++ {
		if err := store.SaveReviewRecord(&core.ReviewRecord{
			IssueID:  issue.ID,
			Round:    i,
			Reviewer: "reviewer",
			Verdict:  "approve",
			Summary:  "summary round " + strconv.Itoa(i),
		}); err != nil {
			t.Fatalf("SaveReviewRecord(%d): %v", i, err)
		}
	}

	mem := NewSQLiteMemory(store)
	hot, err := mem.RecallHot(issue.ID, "")
	if err != nil {
		t.Fatalf("RecallHot: %v", err)
	}
	if strings.Contains(hot, "summary round 1") || strings.Contains(hot, "summary round 2") {
		t.Fatalf("RecallHot() should trim old reviews: %q", hot)
	}
	for _, want := range []string{"summary round 3", "summary round 7"} {
		if !strings.Contains(hot, want) {
			t.Fatalf("RecallHot() missing retained review %q: %q", want, hot)
		}
	}
}

func TestRecallHot_Empty(t *testing.T) {
	store := setupMemoryTest(t)
	defer store.Close()

	issue := &core.Issue{
		ID:        "issue-hot-empty",
		ProjectID: "proj-mem",
		Title:     "Empty hot",
		Template:  "standard",
		Status:    core.IssueStatusDraft,
	}
	if err := store.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	mem := NewSQLiteMemory(store)
	hot, err := mem.RecallHot(issue.ID, "")
	if err != nil {
		t.Fatalf("RecallHot() error = %v", err)
	}
	if hot != "" {
		t.Fatalf("RecallHot() = %q, want empty", hot)
	}
}

func TestRecallHot_MissingIssueIgnoresRunEvents(t *testing.T) {
	store := setupMemoryTest(t)
	defer store.Close()

	runID := "run-hot-missing"
	if err := store.SaveRunEvent(core.RunEvent{
		RunID:     runID,
		ProjectID: "proj-mem",
		EventType: string(core.EventStageFailed),
		Error:     "should stay hidden",
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveRunEvent: %v", err)
	}

	mem := NewSQLiteMemory(store)
	hot, err := mem.RecallHot("missing-issue", runID)
	if err != nil {
		t.Fatalf("RecallHot() error = %v", err)
	}
	if hot != "" {
		t.Fatalf("RecallHot() = %q, want empty for missing issue", hot)
	}
}

func TestRecallHot_NilMemory(t *testing.T) {
	var mem *SQLiteMemory

	hot, err := mem.RecallHot("issue-hot-nil", "run-hot-nil")
	if err != nil {
		t.Fatalf("RecallHot() error = %v", err)
	}
	if hot != "" {
		t.Fatalf("RecallHot() = %q, want empty", hot)
	}
}
