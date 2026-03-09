package core

import (
	"strings"
	"testing"
	"time"
)

func TestTaskStepActionDeriveStatus(t *testing.T) {
	tests := []struct {
		action      TaskStepAction
		wantStatus  IssueStatus
		wantDerived bool
	}{
		{StepCreated, IssueStatusDraft, true},
		{StepSubmittedForReview, IssueStatusReviewing, true},
		{StepReviewApproved, IssueStatusQueued, true},
		{StepReviewRejected, IssueStatusDraft, true},
		{StepReady, IssueStatusReady, true},
		{StepExecutionStarted, IssueStatusExecuting, true},
		{StepMergeStarted, IssueStatusMerging, true},
		{StepCompleted, IssueStatusDone, true},
		{StepMergeCompleted, IssueStatusDone, true},
		{StepFailed, IssueStatusFailed, true},
		{StepAbandoned, IssueStatusAbandoned, true},
		{StepDecomposeStarted, IssueStatusDecomposing, true},
		{StepDecomposed, IssueStatusDecomposed, true},
		{StepSuperseded, IssueStatusSuperseded, true},
		{StepRunCreated, "", false},
		{StepRunStarted, "", false},
		{StepStageStarted, "", false},
		{StepStageCompleted, "", false},
		{StepStageFailed, "", false},
		{StepRunCompleted, "", false},
		{StepRunFailed, "", false},
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

func TestNewTaskStepID(t *testing.T) {
	id1 := NewTaskStepID()
	id2 := NewTaskStepID()
	if id1 == "" || id2 == "" {
		t.Fatal("expected non-empty task step ids")
	}
	if !strings.HasPrefix(id1, "step-") {
		t.Fatalf("expected id %q to start with step-", id1)
	}
	if id1 == id2 {
		t.Fatalf("expected unique ids, got %q and %q", id1, id2)
	}
}
