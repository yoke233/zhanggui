package storesqlite

import (
	"fmt"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestSaveTaskStep_IssueStatusDerivation(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	project := &core.Project{ID: "proj-task-step-1", Name: "task-step", RepoPath: t.TempDir()}
	if err := s.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	issue := &core.Issue{
		ID:        "issue-task-step-1",
		ProjectID: project.ID,
		Title:     "TaskStep status derivation",
		Template:  "standard",
		State:     core.IssueStateOpen,
		Status:    core.IssueStatusDraft,
	}
	if err := s.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	status, err := s.SaveTaskStep(&core.TaskStep{
		ID:        "step-review-1",
		IssueID:   issue.ID,
		AgentID:   "system",
		Action:    core.StepSubmittedForReview,
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("SaveTaskStep: %v", err)
	}
	if status != core.IssueStatusReviewing {
		t.Fatalf("SaveTaskStep status = %q, want %q", status, core.IssueStatusReviewing)
	}

	gotIssue, err := s.GetIssue(issue.ID)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if gotIssue.Status != core.IssueStatusReviewing {
		t.Fatalf("issue status = %q, want %q", gotIssue.Status, core.IssueStatusReviewing)
	}
}

func TestSaveTaskStep_RunLevelAction_NoStatusChange(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	project := &core.Project{ID: "proj-task-step-2", Name: "task-step", RepoPath: t.TempDir()}
	if err := s.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	issue := &core.Issue{
		ID:        "issue-task-step-2",
		ProjectID: project.ID,
		Title:     "TaskStep run-level action",
		Template:  "standard",
		State:     core.IssueStateOpen,
		Status:    core.IssueStatusExecuting,
	}
	if err := s.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	status, err := s.SaveTaskStep(&core.TaskStep{
		ID:        "step-stage-1",
		IssueID:   issue.ID,
		RunID:     "run-1",
		StageID:   core.StageImplement,
		Action:    core.StepStageStarted,
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("SaveTaskStep: %v", err)
	}
	if status != core.IssueStatusExecuting {
		t.Fatalf("SaveTaskStep status = %q, want %q", status, core.IssueStatusExecuting)
	}

	gotIssue, err := s.GetIssue(issue.ID)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if gotIssue.Status != core.IssueStatusExecuting {
		t.Fatalf("issue status = %q, want %q", gotIssue.Status, core.IssueStatusExecuting)
	}
}

func TestListTaskSteps(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	project := &core.Project{ID: "proj-task-step-3", Name: "task-step", RepoPath: t.TempDir()}
	if err := s.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	issue := &core.Issue{
		ID:        "issue-task-step-3",
		ProjectID: project.ID,
		Title:     "TaskStep list",
		Template:  "standard",
		State:     core.IssueStateOpen,
		Status:    core.IssueStatusDraft,
	}
	if err := s.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	for index, action := range []core.TaskStepAction{core.StepCreated, core.StepSubmittedForReview} {
		if _, err := s.SaveTaskStep(&core.TaskStep{
			ID:        fmt.Sprintf("step-%02d", index+1),
			IssueID:   issue.ID,
			Action:    action,
			CreatedAt: time.Now().UTC().Add(time.Duration(index) * time.Second),
		}); err != nil {
			t.Fatalf("SaveTaskStep(%d): %v", index, err)
		}
	}

	steps, err := s.ListTaskSteps(issue.ID)
	if err != nil {
		t.Fatalf("ListTaskSteps: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("ListTaskSteps len = %d, want 2", len(steps))
	}
	if steps[0].Action != core.StepCreated {
		t.Fatalf("first action = %q, want %q", steps[0].Action, core.StepCreated)
	}
	if steps[1].Action != core.StepSubmittedForReview {
		t.Fatalf("second action = %q, want %q", steps[1].Action, core.StepSubmittedForReview)
	}
}

func TestRebuildIssueStatus(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	project := &core.Project{ID: "proj-task-step-4", Name: "task-step", RepoPath: t.TempDir()}
	if err := s.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	issue := &core.Issue{
		ID:        "issue-task-step-4",
		ProjectID: project.ID,
		Title:     "TaskStep rebuild",
		Template:  "standard",
		State:     core.IssueStateOpen,
		Status:    core.IssueStatusDraft,
	}
	if err := s.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	for index, action := range []core.TaskStepAction{core.StepCreated, core.StepExecutionStarted, core.StepMergeCompleted} {
		if _, err := s.SaveTaskStep(&core.TaskStep{
			ID:        fmt.Sprintf("step-rebuild-%02d", index+1),
			IssueID:   issue.ID,
			Action:    action,
			CreatedAt: time.Now().UTC().Add(time.Duration(index) * time.Second),
		}); err != nil {
			t.Fatalf("SaveTaskStep(%d): %v", index, err)
		}
	}

	if _, err := s.db.Exec(`UPDATE issues SET status = ? WHERE id = ?`, string(core.IssueStatusDraft), issue.ID); err != nil {
		t.Fatalf("force issue status: %v", err)
	}

	status, err := s.RebuildIssueStatus(issue.ID)
	if err != nil {
		t.Fatalf("RebuildIssueStatus: %v", err)
	}
	if status != core.IssueStatusDone {
		t.Fatalf("RebuildIssueStatus = %q, want %q", status, core.IssueStatusDone)
	}

	gotIssue, err := s.GetIssue(issue.ID)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if gotIssue.Status != core.IssueStatusDone {
		t.Fatalf("issue status = %q, want %q", gotIssue.Status, core.IssueStatusDone)
	}
}

func TestRebuildIssueStatus_NoDerivedActionKeepsCurrentStatus(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	project := &core.Project{ID: "proj-task-step-5", Name: "task-step", RepoPath: t.TempDir()}
	if err := s.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	issue := &core.Issue{
		ID:        "issue-task-step-5",
		ProjectID: project.ID,
		Title:     "TaskStep rebuild no derived",
		Template:  "standard",
		State:     core.IssueStateOpen,
		Status:    core.IssueStatusExecuting,
	}
	if err := s.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	if _, err := s.SaveTaskStep(&core.TaskStep{
		ID:        "step-run-only-1",
		IssueID:   issue.ID,
		RunID:     "run-only-1",
		Action:    core.StepRunStarted,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveTaskStep: %v", err)
	}

	status, err := s.RebuildIssueStatus(issue.ID)
	if err != nil {
		t.Fatalf("RebuildIssueStatus: %v", err)
	}
	if status != core.IssueStatusExecuting {
		t.Fatalf("RebuildIssueStatus = %q, want %q", status, core.IssueStatusExecuting)
	}

	gotIssue, err := s.GetIssue(issue.ID)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if gotIssue.Status != core.IssueStatusExecuting {
		t.Fatalf("issue status = %q, want %q", gotIssue.Status, core.IssueStatusExecuting)
	}
}

func TestRebuildIssueStatus_NoDerivedStepsPreservesCurrentStatus(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	project := &core.Project{ID: "proj-task-step-5", Name: "task-step", RepoPath: t.TempDir()}
	if err := s.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	issue := &core.Issue{
		ID:        "issue-task-step-5",
		ProjectID: project.ID,
		Title:     "TaskStep rebuild preserve",
		Template:  "standard",
		State:     core.IssueStateOpen,
		Status:    core.IssueStatusExecuting,
	}
	if err := s.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	if _, err := s.SaveTaskStep(&core.TaskStep{
		ID:        "step-run-only-01",
		IssueID:   issue.ID,
		RunID:     "run-keep-status",
		Action:    core.StepRunStarted,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveTaskStep: %v", err)
	}

	status, err := s.RebuildIssueStatus(issue.ID)
	if err != nil {
		t.Fatalf("RebuildIssueStatus: %v", err)
	}
	if status != core.IssueStatusExecuting {
		t.Fatalf("RebuildIssueStatus = %q, want %q", status, core.IssueStatusExecuting)
	}

	gotIssue, err := s.GetIssue(issue.ID)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if gotIssue.Status != core.IssueStatusExecuting {
		t.Fatalf("issue status = %q, want %q", gotIssue.Status, core.IssueStatusExecuting)
	}
}
