package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/skills"
)

// ContextRefType classifies the kind of context reference used in input building.
type ContextRefType string

const (
	CtxIssueSummary     ContextRefType = "issue_summary"
	CtxProjectBrief     ContextRefType = "project_brief"
	CtxUpstreamArtifact ContextRefType = "upstream_artifact"
	CtxAgentMemory      ContextRefType = "agent_memory"
	CtxFeatureManifest  ContextRefType = "feature_manifest"
	CtxResourceManifest ContextRefType = "resource_manifest"
	CtxProgressSummary  ContextRefType = "progress_summary"
	CtxSkillsSummary    ContextRefType = "skills_summary"
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
	store      Store
	registry   core.AgentRegistry // optional: for skills resolution
	skillsRoot string             // optional: on-disk skills directory
}

// InputBuilderOption configures the DefaultInputBuilder.
type InputBuilderOption func(*DefaultInputBuilder)

// WithRegistry enables skills injection by providing agent profile lookup.
func WithRegistry(r core.AgentRegistry) InputBuilderOption {
	return func(b *DefaultInputBuilder) { b.registry = r }
}

// WithSkillsRoot sets the on-disk skills directory for skills metadata lookup.
func WithSkillsRoot(root string) InputBuilderOption {
	return func(b *DefaultInputBuilder) { b.skillsRoot = root }
}

// NewInputBuilder creates an InputBuilder backed by the given store.
func NewInputBuilder(store Store, opts ...InputBuilderOption) *DefaultInputBuilder {
	b := &DefaultInputBuilder{store: store}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// Build constructs the input string for the given action.
// Context refs are appended in priority order:
//
//	project brief → work item summary → progress summary → upstream deliverables → feature manifest → skills summary
func (b *DefaultInputBuilder) Build(ctx context.Context, action *core.Action) (string, error) {
	var refs []ContextRef
	constraints := action.AcceptanceCriteria

	// Fetch work item once — used by multiple injectors.
	workItem, _ := b.store.GetWorkItem(ctx, action.WorkItemID)

	// Fetch sibling actions once — used by progress + upstream injectors.
	actions, _ := b.store.ListActionsByWorkItem(ctx, action.WorkItemID)

	// 1. Project brief — project name, description, resource bindings.
	refs = b.injectProjectBriefContext(ctx, workItem, refs)

	// 2. Work item summary — small, provides orientation.
	refs = b.injectWorkItemContext(workItem, refs)

	// 3. Progress summary — where the current work item execution stands.
	refs = b.injectProgressContext(action, actions, refs)

	// 4. Upstream deliverables — tiered by distance (L2 immediate, L0 distant).
	refs = b.injectUpstreamContext(ctx, action, actions, refs)

	// 5. Feature manifest snapshot.
	refs = b.injectManifestContext(ctx, workItem, refs)

	// 6. Skills summary — agent profile's available skills.
	refs = b.injectSkillsContext(ctx, action, refs)

	// Log the assembled context refs for observability.
	result := buildInputFromRefs(action, refs, constraints)
	logContextRefs(action, refs, len(result))

	return result, nil
}

// logContextRefs emits a structured log entry summarizing the injected context refs.
// finalLen is the length of the fully rendered (post-truncation) input string.
func logContextRefs(action *core.Action, refs []ContextRef, finalLen int) {
	if len(refs) == 0 {
		return
	}
	entries := make([]string, 0, len(refs))
	rawChars := 0
	for _, ref := range refs {
		n := len(ref.Inline)
		rawChars += n
		entries = append(entries, fmt.Sprintf("%s(%d):%d", ref.Type, ref.RefID, n))
	}
	slog.Info("briefing context assembled",
		"action_id", action.ID,
		"work_item_id", action.WorkItemID,
		"ref_count", len(refs),
		"raw_chars", rawChars,
		"final_chars", finalLen,
		"refs", strings.Join(entries, ", "),
	)
}

// buildInputFromRefs constructs the input string from objective, context refs and constraints.
func buildInputFromRefs(action *core.Action, refs []ContextRef, constraints []string) string {
	objective := truncateText(buildObjective(action), maxInputTotalChars)
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

	return truncateText(strings.TrimSpace(sb.String()), maxInputTotalChars)
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

// injectUpstreamContext adds upstream run results tiered by distance:
//   - L2 (immediate predecessor): full ResultMarkdown
//   - L0 (distant predecessors): ResultMetadata["summary"] or first 300 chars fallback
func (b *DefaultInputBuilder) injectUpstreamContext(ctx context.Context, action *core.Action, actions []*core.Action, refs []ContextRef) []ContextRef {
	if len(actions) == 0 {
		return refs
	}

	immediateIDs := immediatePredecessorActionIDs(actions, action)
	immediateSet := make(map[int64]bool, len(immediateIDs))
	for _, id := range immediateIDs {
		immediateSet[id] = true
	}

	for _, depID := range predecessorActionIDs(actions, action) {
		run, err := b.store.GetLatestRunWithResult(ctx, depID)
		if err != nil {
			continue
		}

		if immediateSet[depID] {
			// L2: immediate predecessor — full content.
			refs = append(refs, ContextRef{
				Type:   CtxUpstreamArtifact,
				RefID:  run.ID,
				Label:  fmt.Sprintf("upstream action %d output", depID),
				Inline: run.ResultMarkdown,
			})
		} else {
			// L0: distant predecessor — summary only.
			summary := extractRunResultSummary(run)
			if summary != "" {
				refs = append(refs, ContextRef{
					Type:   CtxUpstreamArtifact,
					RefID:  run.ID,
					Label:  fmt.Sprintf("upstream action %d summary", depID),
					Inline: summary,
				})
			}
		}
	}
	return refs
}

const maxSummaryFallbackChars = 300

// extractRunResultSummary returns a compact summary of a Run's result.
// Prefers the Collector-extracted "summary" from ResultMetadata; falls back to
// the first 300 characters of ResultMarkdown.
func extractRunResultSummary(run *core.Run) string {
	if run.ResultMetadata != nil {
		if s, ok := run.ResultMetadata["summary"].(string); ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	md := strings.TrimSpace(run.ResultMarkdown)
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
	entries, err := b.store.ListFeatureEntries(ctx, core.FeatureEntryFilter{
		ProjectID: *workItem.ProjectID,
		Limit:     500,
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
		RefID:  *workItem.ProjectID,
		Label:  "feature manifest",
		Inline: string(snapshot),
	})
}

// injectProjectBriefContext adds the project name, kind, description and resource spaces
// as a CtxProjectBrief reference so the agent understands the project context.
func (b *DefaultInputBuilder) injectProjectBriefContext(ctx context.Context, workItem *core.WorkItem, refs []ContextRef) []ContextRef {
	if workItem == nil || workItem.ProjectID == nil {
		return refs
	}
	project, err := b.store.GetProject(ctx, *workItem.ProjectID)
	if err != nil || project == nil {
		return refs
	}

	var sb strings.Builder
	sb.WriteString("**")
	sb.WriteString(strings.TrimSpace(project.Name))
	sb.WriteString("**")
	if project.Kind != "" {
		sb.WriteString(" (")
		sb.WriteString(string(project.Kind))
		sb.WriteString(")")
	}
	if desc := strings.TrimSpace(project.Description); desc != "" {
		sb.WriteString("\n\n")
		sb.WriteString(desc)
	}

	spaces, err := b.store.ListResourceSpaces(ctx, project.ID)
	if err == nil && len(spaces) > 0 {
		sb.WriteString("\n\nResources:\n")
		for _, rs := range spaces {
			label := strings.TrimSpace(rs.Label)
			if label == "" {
				label = rs.Kind
			}
			sb.WriteString("- ")
			sb.WriteString(label)
			if role := strings.TrimSpace(rs.Role); role != "" {
				sb.WriteString(" [")
				sb.WriteString(role)
				sb.WriteString("]")
			}
			sb.WriteString(": ")
			sb.WriteString(rs.RootURI)
			sb.WriteString("\n")
		}
	}

	content := strings.TrimSpace(sb.String())
	if content == "" {
		return refs
	}
	return append(refs, ContextRef{
		Type:   CtxProjectBrief,
		RefID:  project.ID,
		Label:  "project",
		Inline: content,
	})
}

// injectProgressContext adds a compact progress summary of the work item's action pipeline
// so the agent knows where execution currently stands.
func (b *DefaultInputBuilder) injectProgressContext(action *core.Action, actions []*core.Action, refs []ContextRef) []ContextRef {
	if len(actions) <= 1 {
		return refs
	}

	var sb strings.Builder
	total := len(actions)
	doneCount := 0
	for _, a := range actions {
		if a.Status == core.ActionDone {
			doneCount++
		}
	}
	sb.WriteString(fmt.Sprintf("Progress: %d/%d actions completed\n", doneCount, total))

	for _, a := range actions {
		var marker string
		switch a.Status {
		case core.ActionDone:
			marker = "done"
		case core.ActionRunning:
			marker = "running"
		case core.ActionReady:
			marker = "ready"
		case core.ActionFailed:
			marker = "failed"
		case core.ActionBlocked:
			marker = "blocked"
		case core.ActionWaitingGate:
			marker = "waiting"
		default:
			marker = "pending"
		}
		current := ""
		if a.ID == action.ID {
			current = " ← current"
		}
		sb.WriteString(fmt.Sprintf("- [%s] %s%s\n", marker, a.Name, current))
	}

	return append(refs, ContextRef{
		Type:   CtxProgressSummary,
		RefID:  action.WorkItemID,
		Label:  "execution progress",
		Inline: strings.TrimSpace(sb.String()),
	})
}

// injectSkillsContext adds a compact list of the agent profile's available skills
// (name + description) so the agent knows what capabilities it has.
func (b *DefaultInputBuilder) injectSkillsContext(ctx context.Context, action *core.Action, refs []ContextRef) []ContextRef {
	if b.registry == nil || b.skillsRoot == "" {
		return refs
	}

	// Resolve the same profile that the Resolver would pick for this action,
	// ensuring the skills list matches the agent's actual identity.
	profile, err := b.registry.ResolveForAction(ctx, action)
	if err != nil || profile == nil || len(profile.Skills) == 0 {
		return refs
	}

	var sb strings.Builder
	injected := 0
	for _, skillName := range profile.Skills {
		parsed, err := skills.InspectSkill(b.skillsRoot, skillName)
		if err != nil || !parsed.Valid || parsed.Metadata == nil {
			continue
		}
		desc := strings.TrimSpace(parsed.Metadata.Description)
		if desc == "" || desc == "TODO" {
			continue
		}
		sb.WriteString("- **")
		sb.WriteString(parsed.Metadata.Name)
		sb.WriteString("**: ")
		sb.WriteString(desc)
		sb.WriteString("\n")
		injected++
	}

	if injected == 0 {
		return refs
	}
	return append(refs, ContextRef{
		Type:   CtxSkillsSummary,
		RefID:  0,
		Label:  "available skills",
		Inline: strings.TrimSpace(sb.String()),
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
