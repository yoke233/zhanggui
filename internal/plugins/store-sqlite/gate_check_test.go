package storesqlite

import (
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestGateCheckCRUD(t *testing.T) {
	store, err := New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer store.Close()

	project := &core.Project{ID: "proj-gc", Name: "test", RepoPath: t.TempDir()}
	if err := store.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	issue := &core.Issue{
		ID: "issue-gc-test", ProjectID: project.ID,
		Title: "gate test", Template: "standard",
		Status: core.IssueStatusDraft, State: core.IssueStateOpen,
	}
	if err := store.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	gc1 := &core.GateCheck{
		ID: core.NewGateCheckID(), IssueID: issue.ID,
		GateName: "lint", GateType: core.GateTypeAuto,
		Attempt: 1, Status: core.GateStatusPassed,
		Reason: "all checks passed", CheckedBy: "auto",
		CreatedAt: time.Now(),
	}
	if err := store.SaveGateCheck(gc1); err != nil {
		t.Fatalf("SaveGateCheck: %v", err)
	}

	gc2 := &core.GateCheck{
		ID: core.NewGateCheckID(), IssueID: issue.ID,
		GateName: "review", GateType: core.GateTypeOwnerReview,
		Attempt: 1, Status: core.GateStatusPending,
		Reason: "", CheckedBy: "human",
		CreatedAt: time.Now(),
	}
	if err := store.SaveGateCheck(gc2); err != nil {
		t.Fatalf("SaveGateCheck(gc2): %v", err)
	}

	checks, err := store.GetGateChecks(issue.ID)
	if err != nil {
		t.Fatalf("GetGateChecks: %v", err)
	}
	if len(checks) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(checks))
	}

	latest, err := store.GetLatestGateCheck(issue.ID, "lint")
	if err != nil {
		t.Fatalf("GetLatestGateCheck: %v", err)
	}
	if latest.Status != core.GateStatusPassed {
		t.Errorf("expected passed, got %q", latest.Status)
	}

	_, err = store.GetLatestGateCheck(issue.ID, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent gate")
	}
}

func TestGateCheckConstraints(t *testing.T) {
	store, err := New(":memory:")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer store.Close()

	gc := &core.GateCheck{
		ID:        core.NewGateCheckID(),
		IssueID:   "missing-issue",
		GateName:  "review",
		GateType:  core.GateTypeOwnerReview,
		Attempt:   1,
		Status:    core.GateStatusPending,
		CheckedBy: "human",
		CreatedAt: time.Now(),
	}
	if err := store.SaveGateCheck(gc); err == nil {
		t.Fatal("expected foreign key failure for missing issue")
	}

	project := &core.Project{ID: "proj-gc-constraints", Name: "test", RepoPath: t.TempDir()}
	if err := store.CreateProject(project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	issue := &core.Issue{
		ID: "issue-gc-constraints", ProjectID: project.ID,
		Title: "gate test", Template: "standard",
		Status: core.IssueStatusDraft, State: core.IssueStateOpen,
	}
	if err := store.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}

	dup1 := &core.GateCheck{
		ID:        core.NewGateCheckID(),
		IssueID:   issue.ID,
		GateName:  "review",
		GateType:  core.GateTypeOwnerReview,
		Attempt:   1,
		Status:    core.GateStatusPending,
		CheckedBy: "human",
		CreatedAt: time.Now(),
	}
	if err := store.SaveGateCheck(dup1); err != nil {
		t.Fatalf("SaveGateCheck(dup1): %v", err)
	}
	dup2 := &core.GateCheck{
		ID:        core.NewGateCheckID(),
		IssueID:   issue.ID,
		GateName:  "review",
		GateType:  core.GateTypeOwnerReview,
		Attempt:   1,
		Status:    core.GateStatusPassed,
		CheckedBy: "human",
		CreatedAt: time.Now(),
	}
	if err := store.SaveGateCheck(dup2); err == nil {
		t.Fatal("expected unique constraint failure for duplicate gate attempt")
	}
}
