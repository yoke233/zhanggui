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
		{StatusCreated, StatusRunning},
		{StatusRunning, StatusWaitingReview},
		{StatusRunning, StatusFailed},
		{StatusRunning, StatusDone},
		{StatusRunning, StatusTimeout},
		{StatusWaitingReview, StatusRunning},
		{StatusWaitingReview, StatusDone},
		{StatusWaitingReview, StatusFailed},
		{StatusWaitingReview, StatusTimeout},
		{StatusFailed, StatusRunning},
		{StatusTimeout, StatusRunning},
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
		{StatusCreated, StatusDone},
		{StatusDone, StatusRunning},
		{StatusFailed, StatusDone},
		{StatusTimeout, StatusDone},
	}
	for _, tt := range invalid {
		if err := ValidateTransition(tt.from, tt.to); err == nil {
			t.Errorf("expected invalid: %s -> %s, got nil", tt.from, tt.to)
		}
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
