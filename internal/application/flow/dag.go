package flow

import (
	"fmt"
	"math"
	"sort"

	"github.com/yoke233/ai-workflow/internal/core"
)

// ValidateActions checks that actions have valid positions.
// Actions are sequential by Position, so positions must be non-negative and unique.
func ValidateActions(actions []*core.Action) error {
	seen := make(map[int]struct{}, len(actions))
	for _, action := range actions {
		if action == nil {
			return fmt.Errorf("action is nil")
		}
		if action.Position < 0 {
			return fmt.Errorf("action %d has negative position %d", action.ID, action.Position)
		}
		if _, ok := seen[action.Position]; ok {
			return fmt.Errorf("duplicate action position %d", action.Position)
		}
		seen[action.Position] = struct{}{}
	}
	return nil
}

// EntryActions returns actions that should run first (those with the lowest Position).
func EntryActions(actions []*core.Action) []*core.Action {
	if len(actions) == 0 {
		return nil
	}
	sorted := make([]*core.Action, len(actions))
	copy(sorted, actions)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Position < sorted[j].Position })
	minPos := sorted[0].Position
	var entries []*core.Action
	for _, a := range sorted {
		if a.Position == minPos {
			entries = append(entries, a)
		}
	}
	return entries
}

// PromotableActions returns actions that are pending and whose predecessors (by Position) are all done.
func PromotableActions(actions []*core.Action) []*core.Action {
	doneSet := make(map[int64]bool, len(actions))
	for _, a := range actions {
		if a.Status == core.ActionDone {
			doneSet[a.ID] = true
		}
	}

	var promotable []*core.Action
	for _, a := range actions {
		if a.Status != core.ActionPending {
			continue
		}
		allPriorDone := true
		for _, other := range actions {
			if other.Position < a.Position && !doneSet[other.ID] {
				allPriorDone = false
				break
			}
		}
		if allPriorDone {
			promotable = append(promotable, a)
		}
	}
	return promotable
}

// RunnableActions returns actions that have status "ready" and can be dispatched for execution.
func RunnableActions(actions []*core.Action) []*core.Action {
	var runnable []*core.Action
	for _, a := range actions {
		if a.Status == core.ActionReady {
			runnable = append(runnable, a)
		}
	}
	return runnable
}

// predecessorActionIDs returns the IDs of actions with Position strictly less than the given action.
// This replaces the old DependsOn field for gate reset targets.
func predecessorActionIDs(actions []*core.Action, action *core.Action) []int64 {
	var ids []int64
	for _, a := range actions {
		if a.Position < action.Position {
			ids = append(ids, a.ID)
		}
	}
	return ids
}

// immediatePredecessorActionIDs returns the IDs of actions at the closest lower Position.
func immediatePredecessorActionIDs(actions []*core.Action, action *core.Action) []int64 {
	closest := math.MinInt
	for _, a := range actions {
		if a.Position < action.Position && a.Position > closest {
			closest = a.Position
		}
	}
	if closest == math.MinInt {
		return nil
	}

	var ids []int64
	for _, a := range actions {
		if a.Position == closest {
			ids = append(ids, a.ID)
		}
	}
	return ids
}
