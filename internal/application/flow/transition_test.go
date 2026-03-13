package flow

import (
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestValidWorkItemTransitions(t *testing.T) {
	cases := []struct {
		from, to core.WorkItemStatus
		valid    bool
	}{
		{core.WorkItemOpen, core.WorkItemRunning, true},
		{core.WorkItemOpen, core.WorkItemCancelled, true},
		{core.WorkItemOpen, core.WorkItemDone, false},
		{core.WorkItemRunning, core.WorkItemDone, true},
		{core.WorkItemRunning, core.WorkItemFailed, true},
		{core.WorkItemRunning, core.WorkItemOpen, false},
		{core.WorkItemDone, core.WorkItemRunning, false},
	}
	for _, tc := range cases {
		got := ValidWorkItemTransition(tc.from, tc.to)
		if got != tc.valid {
			t.Errorf("WorkItem %s -> %s: expected %v, got %v", tc.from, tc.to, tc.valid, got)
		}
	}
}

func TestValidActionTransitions(t *testing.T) {
	cases := []struct {
		from, to core.ActionStatus
		valid    bool
	}{
		{core.ActionPending, core.ActionReady, true},
		{core.ActionReady, core.ActionRunning, true},
		{core.ActionRunning, core.ActionDone, true},
		{core.ActionRunning, core.ActionFailed, true},
		{core.ActionDone, core.ActionRunning, false},
		{core.ActionFailed, core.ActionPending, true}, // retry
		{core.ActionBlocked, core.ActionReady, true},
	}
	for _, tc := range cases {
		got := ValidActionTransition(tc.from, tc.to)
		if got != tc.valid {
			t.Errorf("Action %s -> %s: expected %v, got %v", tc.from, tc.to, tc.valid, got)
		}
	}
}

func TestValidRunTransitions(t *testing.T) {
	cases := []struct {
		from, to core.RunStatus
		valid    bool
	}{
		{core.RunCreated, core.RunRunning, true},
		{core.RunRunning, core.RunSucceeded, true},
		{core.RunRunning, core.RunFailed, true},
		{core.RunSucceeded, core.RunRunning, false},
	}
	for _, tc := range cases {
		got := ValidRunTransition(tc.from, tc.to)
		if got != tc.valid {
			t.Errorf("Run %s -> %s: expected %v, got %v", tc.from, tc.to, tc.valid, got)
		}
	}
}
