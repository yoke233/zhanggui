package flow

import (
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
)

func TestEntryActions_ByPosition(t *testing.T) {
	actions := []*core.Action{
		{ID: 1, Name: "A", Position: 0},
		{ID: 2, Name: "B", Position: 1},
		{ID: 3, Name: "C", Position: 2},
	}
	entries := EntryActions(actions)
	if len(entries) != 1 || entries[0].ID != 1 {
		t.Fatalf("expected only action 1 entry, got %v", entries)
	}
}

func TestPromotableActions_ByPosition(t *testing.T) {
	actions := []*core.Action{
		{ID: 1, Name: "A", Status: core.ActionDone, Position: 0},
		{ID: 2, Name: "B", Status: core.ActionPending, Position: 1},
		{ID: 3, Name: "C", Status: core.ActionPending, Position: 2},
	}
	promotable := PromotableActions(actions)
	if len(promotable) != 1 || promotable[0].ID != 2 {
		t.Fatalf("expected only action 2 promotable, got %v", promotable)
	}
}

func TestRunnableActions(t *testing.T) {
	actions := []*core.Action{
		{ID: 1, Name: "A", Status: core.ActionReady},
		{ID: 2, Name: "B", Status: core.ActionPending},
		{ID: 3, Name: "C", Status: core.ActionReady},
	}
	runnable := RunnableActions(actions)
	if len(runnable) != 2 {
		t.Fatalf("expected 2 runnable, got %d", len(runnable))
	}
}

func TestPredecessorActionIDs(t *testing.T) {
	actions := []*core.Action{
		{ID: 1, Position: 0},
		{ID: 2, Position: 1},
		{ID: 3, Position: 2},
	}
	ids := predecessorActionIDs(actions, actions[2])
	if len(ids) != 2 {
		t.Fatalf("expected 2 predecessors, got %d", len(ids))
	}
}

func TestImmediatePredecessorActionIDs(t *testing.T) {
	actions := []*core.Action{
		{ID: 1, Position: 0},
		{ID: 2, Position: 1},
		{ID: 3, Position: 2},
	}
	ids := immediatePredecessorActionIDs(actions, actions[2])
	if len(ids) != 1 || ids[0] != 2 {
		t.Fatalf("expected only action 2 as immediate predecessor, got %v", ids)
	}
}

func TestValidateActions_RejectsDuplicatePosition(t *testing.T) {
	actions := []*core.Action{
		{ID: 1, Position: 0},
		{ID: 2, Position: 0},
	}
	if err := ValidateActions(actions); err == nil {
		t.Fatal("expected duplicate position validation error")
	}
}

func TestValidateActions_RejectsNegativePosition(t *testing.T) {
	actions := []*core.Action{
		{ID: 1, Position: -1},
	}
	if err := ValidateActions(actions); err == nil {
		t.Fatal("expected negative position validation error")
	}
}
