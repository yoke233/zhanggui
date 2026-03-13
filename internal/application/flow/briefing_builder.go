package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yoke233/ai-workflow/internal/core"
)

// ContextRefType classifies the kind of context reference used in input building.
type ContextRefType string

const (
	CtxIssueSummary      ContextRefType = "issue_summary"
	CtxProjectBrief      ContextRefType = "project_brief"
	CtxUpstreamArtifact  ContextRefType = "upstream_artifact"
	CtxAgentMemory       ContextRefType = "agent_memory"
	CtxFeatureManifest   ContextRefType = "feature_manifest"
	CtxResourceManifest  ContextRefType = "resource_manifest"
)

// ContextRef points to a piece of context used when building input.
type ContextRef struct {
	Type   ContextRefType `json:"type"`
	RefID  int64          `json:"ref_id"`
	Label  string         `json:"label,omitempty"`
	Inline string         `json:"inline,omitempty"`
}

// DefaultInputBuilder assembles input text by reading upstream Deliverables
// and action configuration.
type DefaultInputBuilder struct {
	store Store
}

// NewInputBuilder creates an InputBuilder backed by the given store.
func NewInputBuilder(store Store) *DefaultInputBuilder {
	return &DefaultInputBuilder{store: store}
}

// Build constructs the input string for the given action.
// Context refs are appended in priority order: work item summary → upstream deliverables → feature manifest.
func (b *DefaultInputBuilder) Build(ctx context.Context, action *core.Action) (string, error) {
	var refs []ContextRef
	constraints := action.AcceptanceCriteria

	// Fetch work item once — used by both work item summary and manifest injection.
	workItem, _ := b.store.GetWorkItem(ctx, action.WorkItemID)

	// 1. Work item summary — small, provides orientation.
	refs = b.injectWorkItemContext(workItem, refs)

	// 2. Upstream deliverables — tiered by distance (L2 immediate, L0 distant).
	refs = b.injectUpstreamContext(ctx, action, refs)

	// 3. Feature manifest snapshot.
	refs = b.injectManifestContext(ctx, workItem, refs)

	return buildInputFromRefs(action, refs, constraints), nil
}

// buildInputFromRefs constructs the input string from objective, context refs and constraints.
func buildInputFromRefs(action *core.Action, refs []ContextRef, constraints []string) string {
	objective := buildObjective(action)
	var sb strings.Builder
	sb.WriteString(objective)

	if len(refs) > 0 {
		remaining := maxInputTotalChars - len(objective)
		if remaining > 0 {
			const contextHeader = "\n\n# Context\n"
			wroteContextHeader := false
			for _, ref := range refs {
				content := strings.TrimSpace(ref.Inline)
				if content == "" || remaining <= 0 {
					continue
				}
				budget := refBudget(ref)
				content = truncateText(content, budget)
				if len(content) > remaining {
					content = truncateText(content, remaining)
				}
				if strings.TrimSpace(content) == "" {
					continue
				}

				label := strings.TrimSpace(ref.Label)
				if label == "" {
					label = fmt.Sprintf("%s:%d", ref.Type, ref.RefID)
				}

				sectionHeader := "\n\n## " + label + "\n\n"
				available := remaining - len(sectionHeader)
				if !wroteContextHeader {
					available -= len(contextHeader)
				}
				if available <= 0 {
					break
				}
				content = truncateText(content, minInt(budget, available))
				if strings.TrimSpace(content) == "" {
					continue
				}

				if !wroteContextHeader {
					sb.WriteString(contextHeader)
					remaining -= len(contextHeader)
					wroteContextHeader = true
				}
				sb.WriteString(sectionHeader)
				sb.WriteString(content)
				remaining -= len(sectionHeader) + len(content)
			}
		}
	}

	if len(constraints) > 0 {
		sb.WriteString("\n\n# Acceptance Criteria\n\n")
		for _, c := range constraints {
			sb.WriteString("- ")
			sb.WriteString(c)
			sb.WriteString("\n")
		}
	}

	return strings.TrimSpace(sb.String())
}

// injectWorkItemContext adds the parent WorkItem title and body as a CtxIssueSummary reference.
func (b *DefaultInputBuilder) injectWorkItemContext(workItem *core.WorkItem, refs []ContextRef) []ContextRef {
	if workItem == nil {
		return refs
	}
	title := strings.TrimSpace(workItem.Title)
	if title == "" {
		return refs
	}

	var sb strings.Builder
	sb.WriteString("**")
	sb.WriteString(title)
	sb.WriteString("**")

	if body := strings.TrimSpace(workItem.Body); body != "" {
		const maxBody = 500
		if len(body) > maxBody {
			body = body[:maxBody] + " [...]"
		}
		sb.WriteString("\n\n")
		sb.WriteString(body)
	}

	return append(refs, ContextRef{
		Type:   CtxIssueSummary,
		RefID:  workItem.ID,
		Label:  "work item",
		Inline: sb.String(),
	})
}

// injectUpstreamContext adds upstream deliverables tiered by distance:
//   - L2 (immediate predecessor): full ResultMarkdown
//   - L0 (distant predecessors): Metadata["summary"] or first 300 chars fallback
func (b *DefaultInputBuilder) injectUpstreamContext(ctx context.Context, action *core.Action, refs []ContextRef) []ContextRef {
	actions, err := b.store.ListActionsByWorkItem(ctx, action.WorkItemID)
	if err != nil {
		return refs
	}

	immediateIDs := immediatePredecessorActionIDs(actions, action)
	immediateSet := make(map[int64]bool, len(immediateIDs))
	for _, id := range immediateIDs {
		immediateSet[id] = true
	}

	for _, depID := range predecessorActionIDs(actions, action) {
		deliverable, err := b.store.GetLatestDeliverableByAction(ctx, depID)
		if err != nil {
			continue
		}

		if immediateSet[depID] {
			// L2: immediate predecessor — full content.
			refs = append(refs, ContextRef{
				Type:   CtxUpstreamArtifact,
				RefID:  deliverable.ID,
				Label:  fmt.Sprintf("upstream action %d output", depID),
				Inline: deliverable.ResultMarkdown,
			})
		} else {
			// L0: distant predecessor — summary only.
			summary := extractDeliverableSummary(deliverable)
			if summary != "" {
				refs = append(refs, ContextRef{
					Type:   CtxUpstreamArtifact,
					RefID:  deliverable.ID,
					Label:  fmt.Sprintf("upstream action %d summary", depID),
					Inline: summary,
				})
			}
		}
	}
	return refs
}

const maxSummaryFallbackChars = 300

// extractDeliverableSummary returns a compact summary of a Deliverable.
// Prefers the Collector-extracted "summary" from Metadata; falls back to
// the first 300 characters of ResultMarkdown.
func extractDeliverableSummary(deliverable *core.Deliverable) string {
	if deliverable.Metadata != nil {
		if s, ok := deliverable.Metadata["summary"].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	md := strings.TrimSpace(deliverable.ResultMarkdown)
	if md == "" {
		return ""
	}
	if len(md) <= maxSummaryFallbackChars {
		return md
	}
	return md[:maxSummaryFallbackChars] + " [...]"
}

// injectManifestContext adds the project's feature manifest as a ContextRef.
func (b *DefaultInputBuilder) injectManifestContext(ctx context.Context, workItem *core.WorkItem, refs []ContextRef) []ContextRef {
	if workItem == nil || workItem.ProjectID == nil {
		return refs
	}
	manifest, err := b.store.GetFeatureManifestByProject(ctx, *workItem.ProjectID)
	if err != nil {
		return refs
	}
	entries, err := b.store.ListFeatureEntries(ctx, core.FeatureEntryFilter{
		ManifestID: manifest.ID,
		Limit:      500,
	})
	if err != nil || len(entries) == 0 {
		return refs
	}

	// Inject a compact snapshot: only key + status for pass/skipped entries,
	// full details for fail/pending entries (these are actionable).
	type compactEntry struct {
		Key         string   `json:"key"`
		Status      string   `json:"status"`
		Description string   `json:"description,omitempty"`
		WorkItemID  *int64   `json:"work_item_id,omitempty"`
		Tags        []string `json:"tags,omitempty"`
	}
	compact := make([]compactEntry, 0, len(entries))
	for _, e := range entries {
		ce := compactEntry{Key: e.Key, Status: string(e.Status)}
		if e.Status == core.FeatureFail || e.Status == core.FeaturePending {
			ce.Description = e.Description
			ce.WorkItemID = e.WorkItemID
			ce.Tags = e.Tags
		}
		compact = append(compact, ce)
	}

	snapshot, _ := json.Marshal(compact)
	return append(refs, ContextRef{
		Type:   CtxFeatureManifest,
		RefID:  manifest.ID,
		Label:  "feature manifest",
		Inline: string(snapshot),
	})
}

// buildObjective derives a brief objective string from action config or name.
func buildObjective(action *core.Action) string {
	if action.Config != nil {
		if obj, ok := action.Config["objective"].(string); ok && obj != "" {
			return obj
		}
	}
	return fmt.Sprintf("Execute action: %s", action.Name)
}
