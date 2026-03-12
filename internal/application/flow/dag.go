package flow

import (
	"fmt"
	"math"
	"sort"

	"github.com/yoke233/ai-workflow/internal/core"
)

// ValidateSteps checks that steps have valid positions.
// Steps are sequential by Position, so positions must be non-negative and unique.
func ValidateSteps(steps []*core.Step) error {
	seen := make(map[int]struct{}, len(steps))
	for _, step := range steps {
		if step == nil {
			return fmt.Errorf("step is nil")
		}
		if step.Position < 0 {
			return fmt.Errorf("step %d has negative position %d", step.ID, step.Position)
		}
		if _, ok := seen[step.Position]; ok {
			return fmt.Errorf("duplicate step position %d", step.Position)
		}
		seen[step.Position] = struct{}{}
	}
	return nil
}

// EntrySteps returns steps that should run first (those with the lowest Position).
func EntrySteps(steps []*core.Step) []*core.Step {
	if len(steps) == 0 {
		return nil
	}
	sorted := make([]*core.Step, len(steps))
	copy(sorted, steps)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Position < sorted[j].Position })
	minPos := sorted[0].Position
	var entries []*core.Step
	for _, s := range sorted {
		if s.Position == minPos {
			entries = append(entries, s)
		}
	}
	return entries
}

// PromotableSteps returns steps that are pending and whose predecessors (by Position) are all done.
func PromotableSteps(steps []*core.Step) []*core.Step {
	doneSet := make(map[int64]bool, len(steps))
	for _, s := range steps {
		if s.Status == core.StepDone {
			doneSet[s.ID] = true
		}
	}

	var promotable []*core.Step
	for _, s := range steps {
		if s.Status != core.StepPending {
			continue
		}
		allPriorDone := true
		for _, other := range steps {
			if other.Position < s.Position && !doneSet[other.ID] {
				allPriorDone = false
				break
			}
		}
		if allPriorDone {
			promotable = append(promotable, s)
		}
	}
	return promotable
}

// RunnableSteps returns steps that have status "ready" and can be dispatched for execution.
func RunnableSteps(steps []*core.Step) []*core.Step {
	var runnable []*core.Step
	for _, s := range steps {
		if s.Status == core.StepReady {
			runnable = append(runnable, s)
		}
	}
	return runnable
}

// predecessorStepIDs returns the IDs of steps with Position strictly less than the given step.
// This replaces the old DependsOn field for gate reset targets.
func predecessorStepIDs(steps []*core.Step, step *core.Step) []int64 {
	var ids []int64
	for _, s := range steps {
		if s.Position < step.Position {
			ids = append(ids, s.ID)
		}
	}
	return ids
}

// immediatePredecessorStepIDs returns the IDs of steps at the closest lower Position.
func immediatePredecessorStepIDs(steps []*core.Step, step *core.Step) []int64 {
	closest := math.MinInt
	for _, s := range steps {
		if s.Position < step.Position && s.Position > closest {
			closest = s.Position
		}
	}
	if closest == math.MinInt {
		return nil
	}

	var ids []int64
	for _, s := range steps {
		if s.Position == closest {
			ids = append(ids, s.ID)
		}
	}
	return ids
}
