package flow

import (
	"context"
	"encoding/json"
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

	// Inject feature manifest snapshot if the issue's project has one.
	b.injectManifestContext(ctx, step, briefing)

	return briefing, nil
}

// injectManifestContext adds the project's feature manifest as a ContextRef.
func (b *DefaultBriefingBuilder) injectManifestContext(ctx context.Context, step *core.Step, briefing *core.Briefing) {
	issue, err := b.store.GetIssue(ctx, step.IssueID)
	if err != nil || issue == nil || issue.ProjectID == nil {
		return
	}
	manifest, err := b.store.GetFeatureManifestByProject(ctx, *issue.ProjectID)
	if err != nil {
		return
	}
	entries, err := b.store.ListFeatureEntries(ctx, core.FeatureEntryFilter{
		ManifestID: manifest.ID,
		Limit:      500,
	})
	if err != nil || len(entries) == 0 {
		return
	}

	// Inject a compact snapshot: only key + status for pass/skipped entries,
	// full details for fail/pending entries (these are actionable).
	type compactEntry struct {
		Key         string        `json:"key"`
		Status      string        `json:"status"`
		Description string        `json:"description,omitempty"`
		IssueID     *int64        `json:"issue_id,omitempty"`
		Tags        []string      `json:"tags,omitempty"`
	}
	compact := make([]compactEntry, 0, len(entries))
	for _, e := range entries {
		ce := compactEntry{Key: e.Key, Status: string(e.Status)}
		if e.Status == core.FeatureFail || e.Status == core.FeaturePending {
			ce.Description = e.Description
			ce.IssueID = e.IssueID
			ce.Tags = e.Tags
		}
		compact = append(compact, ce)
	}

	snapshot, _ := json.Marshal(compact)
	briefing.ContextRefs = append(briefing.ContextRefs, core.ContextRef{
		Type:   core.CtxFeatureManifest,
		RefID:  manifest.ID,
		Label:  "feature manifest",
		Inline: string(snapshot),
	})
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
