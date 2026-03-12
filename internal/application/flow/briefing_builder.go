package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
// Context refs are appended in priority order: issue summary → upstream artifacts → feature manifest.
func (b *DefaultBriefingBuilder) Build(ctx context.Context, step *core.Step) (*core.Briefing, error) {
	briefing := &core.Briefing{
		StepID:      step.ID,
		Objective:   buildObjective(step),
		Constraints: step.AcceptanceCriteria,
	}

	// Fetch issue once — used by both issue summary and manifest injection.
	issue, _ := b.store.GetIssue(ctx, step.IssueID)

	// 1. Issue summary — small, provides orientation.
	b.injectIssueContext(issue, briefing)

	// 2. Upstream artifacts — tiered by distance (L2 immediate, L0 distant).
	b.injectUpstreamContext(ctx, step, briefing)

	// 3. Feature manifest snapshot.
	b.injectManifestContext(ctx, issue, briefing)

	return briefing, nil
}

// injectIssueContext adds the parent Issue title and body as a CtxIssueSummary reference.
func (b *DefaultBriefingBuilder) injectIssueContext(issue *core.Issue, briefing *core.Briefing) {
	if issue == nil {
		return
	}
	title := strings.TrimSpace(issue.Title)
	if title == "" {
		return
	}

	var sb strings.Builder
	sb.WriteString("**")
	sb.WriteString(title)
	sb.WriteString("**")

	if body := strings.TrimSpace(issue.Body); body != "" {
		const maxBody = 500
		if len(body) > maxBody {
			body = body[:maxBody] + " [...]"
		}
		sb.WriteString("\n\n")
		sb.WriteString(body)
	}

	briefing.ContextRefs = append(briefing.ContextRefs, core.ContextRef{
		Type:   core.CtxIssueSummary,
		RefID:  issue.ID,
		Label:  "work item",
		Inline: sb.String(),
	})
}

// injectUpstreamContext adds upstream artifacts tiered by distance:
//   - L2 (immediate predecessor): full ResultMarkdown
//   - L0 (distant predecessors): Metadata["summary"] or first 300 chars fallback
func (b *DefaultBriefingBuilder) injectUpstreamContext(ctx context.Context, step *core.Step, briefing *core.Briefing) {
	steps, err := b.store.ListStepsByIssue(ctx, step.IssueID)
	if err != nil {
		return
	}

	immediateIDs := immediatePredecessorStepIDs(steps, step)
	immediateSet := make(map[int64]bool, len(immediateIDs))
	for _, id := range immediateIDs {
		immediateSet[id] = true
	}

	for _, depID := range predecessorStepIDs(steps, step) {
		art, err := b.store.GetLatestArtifactByStep(ctx, depID)
		if err != nil {
			continue
		}

		if immediateSet[depID] {
			// L2: immediate predecessor — full content.
			briefing.ContextRefs = append(briefing.ContextRefs, core.ContextRef{
				Type:   core.CtxUpstreamArtifact,
				RefID:  art.ID,
				Label:  fmt.Sprintf("upstream step %d output", depID),
				Inline: art.ResultMarkdown,
			})
		} else {
			// L0: distant predecessor — summary only.
			summary := extractArtifactSummary(art)
			if summary != "" {
				briefing.ContextRefs = append(briefing.ContextRefs, core.ContextRef{
					Type:   core.CtxUpstreamArtifact,
					RefID:  art.ID,
					Label:  fmt.Sprintf("upstream step %d summary", depID),
					Inline: summary,
				})
			}
		}
	}
}

const maxSummaryFallbackChars = 300

// extractArtifactSummary returns a compact summary of an Artifact.
// Prefers the Collector-extracted "summary" from Metadata; falls back to
// the first 300 characters of ResultMarkdown.
func extractArtifactSummary(art *core.Artifact) string {
	if art.Metadata != nil {
		if s, ok := art.Metadata["summary"].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	md := strings.TrimSpace(art.ResultMarkdown)
	if md == "" {
		return ""
	}
	if len(md) <= maxSummaryFallbackChars {
		return md
	}
	return md[:maxSummaryFallbackChars] + " [...]"
}

// injectManifestContext adds the project's feature manifest as a ContextRef.
func (b *DefaultBriefingBuilder) injectManifestContext(ctx context.Context, issue *core.Issue, briefing *core.Briefing) {
	if issue == nil || issue.ProjectID == nil {
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

// buildObjective derives a brief objective string from step config or name.
func buildObjective(step *core.Step) string {
	if step.Config != nil {
		if obj, ok := step.Config["objective"].(string); ok && obj != "" {
			return obj
		}
	}
	return fmt.Sprintf("Execute step: %s", step.Name)
}
