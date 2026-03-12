package flow

import (
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestValidIssueTransitions(t *testing.T) {
	cases := []struct {
		from, to core.IssueStatus
		valid    bool
	}{
		{core.IssueOpen, core.IssueRunning, true},
		{core.IssueOpen, core.IssueCancelled, true},
		{core.IssueOpen, core.IssueDone, false},
		{core.IssueRunning, core.IssueDone, true},
		{core.IssueRunning, core.IssueFailed, true},
		{core.IssueRunning, core.IssueOpen, false},
		{core.IssueDone, core.IssueRunning, false},
	}
	for _, tc := range cases {
		got := ValidIssueTransition(tc.from, tc.to)
		if got != tc.valid {
			t.Errorf("Issue %s -> %s: expected %v, got %v", tc.from, tc.to, tc.valid, got)
		}
	}
}

func TestValidStepTransitions(t *testing.T) {
	cases := []struct {
		from, to core.StepStatus
		valid    bool
	}{
		{core.StepPending, core.StepReady, true},
		{core.StepReady, core.StepRunning, true},
		{core.StepRunning, core.StepDone, true},
		{core.StepRunning, core.StepFailed, true},
		{core.StepDone, core.StepRunning, false},
		{core.StepFailed, core.StepPending, true}, // retry
		{core.StepBlocked, core.StepReady, true},
	}
	for _, tc := range cases {
		got := ValidStepTransition(tc.from, tc.to)
		if got != tc.valid {
			t.Errorf("Step %s -> %s: expected %v, got %v", tc.from, tc.to, tc.valid, got)
		}
	}
}

func TestValidExecTransitions(t *testing.T) {
	cases := []struct {
		from, to core.ExecutionStatus
		valid    bool
	}{
		{core.ExecCreated, core.ExecRunning, true},
		{core.ExecRunning, core.ExecSucceeded, true},
		{core.ExecRunning, core.ExecFailed, true},
		{core.ExecSucceeded, core.ExecRunning, false},
	}
	for _, tc := range cases {
		got := ValidExecTransition(tc.from, tc.to)
		if got != tc.valid {
			t.Errorf("Exec %s -> %s: expected %v, got %v", tc.from, tc.to, tc.valid, got)
		}
	}
}
