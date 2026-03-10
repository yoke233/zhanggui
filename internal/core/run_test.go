package core

import (
	"os"
	"strings"
	"testing"
)

func TestValidateTransition(t *testing.T) {
	valid := []struct {
		from RunStatus
		to   RunStatus
	}{
		{StatusQueued, StatusInProgress},
		{StatusQueued, StatusCompleted}, // abort
		{StatusInProgress, StatusCompleted},
		{StatusInProgress, StatusActionRequired},
		{StatusInProgress, StatusQueued}, // re-enqueue
		{StatusActionRequired, StatusInProgress},
		{StatusActionRequired, StatusCompleted},
		{StatusActionRequired, StatusQueued}, // re-enqueue
		{StatusCompleted, StatusInProgress},  // retry
	}
	for _, tt := range valid {
		if err := ValidateTransition(tt.from, tt.to); err != nil {
			t.Errorf("expected valid: %s -> %s, got err: %v", tt.from, tt.to, err)
		}
	}

	invalid := []struct {
		from RunStatus
		to   RunStatus
	}{
		{StatusQueued, StatusActionRequired},
		{StatusCompleted, StatusQueued},
		{StatusCompleted, StatusActionRequired},
	}
	for _, tt := range invalid {
		if err := ValidateTransition(tt.from, tt.to); err == nil {
			t.Errorf("expected invalid: %s -> %s, got nil", tt.from, tt.to)
		}
	}
}

func TestValidateTransition_Idempotent(t *testing.T) {
	for _, s := range []RunStatus{StatusQueued, StatusInProgress, StatusCompleted, StatusActionRequired} {
		if err := ValidateTransition(s, s); err != nil {
			t.Errorf("idempotent %s -> %s should be valid, got: %v", s, s, err)
		}
	}
}

func TestRun_TransitionStatus(t *testing.T) {
	r := &Run{Status: StatusQueued}

	if err := r.TransitionStatus(StatusInProgress); err != nil {
		t.Fatalf("queued -> in_progress: %v", err)
	}
	if r.Status != StatusInProgress {
		t.Fatalf("status = %s, want in_progress", r.Status)
	}
	if r.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt should be set")
	}

	// invalid transition
	if err := r.TransitionStatus(StatusQueued); err != nil {
		// in_progress -> queued is now valid (re-enqueue)
		t.Fatalf("unexpected error: %v", err)
	}

	// idempotent
	r.Status = StatusCompleted
	if err := r.TransitionStatus(StatusCompleted); err != nil {
		t.Fatalf("idempotent completed -> completed: %v", err)
	}

	// truly invalid
	r.Status = StatusCompleted
	if err := r.TransitionStatus(StatusActionRequired); err == nil {
		t.Fatal("completed -> action_required should be invalid")
	}
}

func TestStageSource_NoLegacySpecStages(t *testing.T) {
	content, err := os.ReadFile("stage.go")
	if err != nil {
		t.Fatalf("read stage.go: %v", err)
	}

	src := string(content)
	for _, legacy := range []string{
		"Stage" + "SpecGen",
		"Stage" + "SpecReview",
		"spec" + "_gen",
		"spec" + "_review",
	} {
		if strings.Contains(src, legacy) {
			t.Fatalf("legacy stage marker %q should be removed from stage.go", legacy)
		}
	}
}
