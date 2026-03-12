package flow

import (
	"context"
	"fmt"

	"github.com/yoke233/ai-workflow/internal/core"
)

// DefaultBriefingBuilder assembles a Briefing by reading upstream Artifacts
// and step configuration.
type DefaultBriefingBuilder struct {
	store Store
}

// NewBriefingBuilder creates a BriefingBuilder backed by the given store.
func NewBriefingBuilder(store Store) *DefaultBriefingBuilder {
	return &DefaultBriefingBuilder{store: store}
}

// Build constructs a Briefing for the given step.
func (b *DefaultBriefingBuilder) Build(ctx context.Context, step *core.Step) (*core.Briefing, error) {
	briefing := &core.Briefing{
		StepID:      step.ID,
		Objective:   buildObjective(step),
		Constraints: step.AcceptanceCriteria,
	}

	// Collect upstream artifact references from predecessor steps (by Position).
	predecessors := predecessorStepIDsFromStore(ctx, b.store, step)

	for _, depID := range predecessors {
		art, err := b.store.GetLatestArtifactByStep(ctx, depID)
		if err == core.ErrNotFound {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("get upstream artifact for step %d: %w", depID, err)
		}
		briefing.ContextRefs = append(briefing.ContextRefs, core.ContextRef{
			Type:   core.CtxUpstreamArtifact,
			RefID:  art.ID,
			Label:  fmt.Sprintf("upstream step %d output", depID),
			Inline: art.ResultMarkdown,
		})
	}

	return briefing, nil
}

// predecessorStepIDsFromStore returns IDs of steps with lower Position in the same issue.
func predecessorStepIDsFromStore(ctx context.Context, store Store, step *core.Step) []int64 {
	steps, err := store.ListStepsByIssue(ctx, step.IssueID)
	if err != nil {
		return nil
	}
	return predecessorStepIDs(steps, step)
}

// buildObjective derives a brief objective string from step config or name.
func buildObjective(step *core.Step) string {
	if step.Config != nil {
		if obj, ok := step.Config["objective"].(string); ok && obj != "" {
			return obj
		}
	}
	return fmt.Sprintf("Execute step: %s", step.Name)
}
