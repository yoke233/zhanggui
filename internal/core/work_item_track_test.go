package core

import "testing"

func TestParseWorkItemTrackStatus(t *testing.T) {
	status, err := ParseWorkItemTrackStatus("planning")
	if err != nil {
		t.Fatalf("parse planning: %v", err)
	}
	if status != WorkItemTrackPlanning {
		t.Fatalf("expected planning, got %q", status)
	}

	if _, err := ParseWorkItemTrackStatus("broken"); err == nil {
		t.Fatal("expected invalid work item track status error")
	}
}

func TestCanTransitionWorkItemTrackStatus(t *testing.T) {
	if !CanTransitionWorkItemTrackStatus(WorkItemTrackDraft, WorkItemTrackPlanning) {
		t.Fatal("expected draft -> planning to be allowed")
	}
	if !CanTransitionWorkItemTrackStatus(WorkItemTrackReviewing, WorkItemTrackAwaitingConfirmation) {
		t.Fatal("expected reviewing -> awaiting_confirmation to be allowed")
	}
	if CanTransitionWorkItemTrackStatus(WorkItemTrackDone, WorkItemTrackPlanning) {
		t.Fatal("expected done -> planning to be rejected")
	}
}

func TestParseWorkItemTrackThreadRelation(t *testing.T) {
	relation, err := ParseWorkItemTrackThreadRelation("source")
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}
	if relation != WorkItemTrackThreadSource {
		t.Fatalf("expected source, got %q", relation)
	}

	if _, err := ParseWorkItemTrackThreadRelation("broken"); err == nil {
		t.Fatal("expected invalid work item track thread relation error")
	}
}
