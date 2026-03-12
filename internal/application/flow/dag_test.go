package flow

import (
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestEntrySteps_ByPosition(t *testing.T) {
	steps := []*core.Step{
		{ID: 1, Name: "A", Position: 0},
		{ID: 2, Name: "B", Position: 1},
		{ID: 3, Name: "C", Position: 0},
	}
	entries := EntrySteps(steps)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestPromotableSteps_ByPosition(t *testing.T) {
	steps := []*core.Step{
		{ID: 1, Name: "A", Status: core.StepDone, Position: 0},
		{ID: 2, Name: "B", Status: core.StepPending, Position: 1},
		{ID: 3, Name: "C", Status: core.StepPending, Position: 2},
	}
	promotable := PromotableSteps(steps)
	if len(promotable) != 1 || promotable[0].ID != 2 {
		t.Fatalf("expected only step 2 promotable, got %v", promotable)
	}
}

func TestRunnableSteps(t *testing.T) {
	steps := []*core.Step{
		{ID: 1, Name: "A", Status: core.StepReady},
		{ID: 2, Name: "B", Status: core.StepPending},
		{ID: 3, Name: "C", Status: core.StepReady},
	}
	runnable := RunnableSteps(steps)
	if len(runnable) != 2 {
		t.Fatalf("expected 2 runnable, got %d", len(runnable))
	}
}

func TestPredecessorStepIDs(t *testing.T) {
	steps := []*core.Step{
		{ID: 1, Position: 0},
		{ID: 2, Position: 1},
		{ID: 3, Position: 2},
	}
	ids := predecessorStepIDs(steps, steps[2])
	if len(ids) != 2 {
		t.Fatalf("expected 2 predecessors, got %d", len(ids))
	}
}
