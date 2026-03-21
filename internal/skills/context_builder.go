package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yoke233/zhanggui/internal/core"
)

// ContextMaterial represents one reference file in the action-context skill.
type ContextMaterial struct {
	Path        string // relative path within skill dir, e.g. "work-item.md"
	Description string // one-line description for the index table
	Hint        string // when the agent should read this
	Content     string // file content
}

// ActionContextBuilder generates a per-run action-context skill directory
// containing full reference materials that the agent can read on demand.
type ActionContextBuilder struct {
	store core.Store
}

// NewActionContextBuilder creates a new ActionContextBuilder.
func NewActionContextBuilder(store core.Store) *ActionContextBuilder {
	return &ActionContextBuilder{store: store}
}

// Build generates the action-context directory under parentDir/run-<id>/action-context/
// and returns the full path. The caller must defer Cleanup(dir).
//
// Materials generated (when data exists):
//   - work-item.md:       full WorkItem title + body + metadata
//   - upstream/<name>.md: full ResultMarkdown of each predecessor action
//   - acceptance.md:      numbered acceptance criteria
//   - gate-feedback.md:   last_gate_feedback + rework_history
//   - manifest.json:      full feature manifest entries
//   - SKILL.md:           auto-generated index of all above
func (b *ActionContextBuilder) Build(ctx context.Context, parentDir string,
	action *core.Action, run *core.Run) (string, error) {

	if action == nil || run == nil {
		return "", fmt.Errorf("action and run must not be nil")
	}

	dir := filepath.Join(parentDir, fmt.Sprintf("run-%d", run.ID), "action-context")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create action-context dir: %w", err)
	}

	// Query work item once; reused by buildWorkItemMaterial and buildManifestMaterial.
	workItem, _ := b.store.GetWorkItem(ctx, action.WorkItemID)

	var materials []ContextMaterial

	// 1. Work item (full body, not truncated)
	if m := buildWorkItemMaterial(workItem); m != nil {
		materials = append(materials, *m)
	}

	// 2. Upstream deliverables (full ResultMarkdown of each predecessor action)
	// TODO: Position-based filtering assumes linear pipeline; revisit for DAG scheduling
	// where parallel actions may share the same Position.
	materials = append(materials, b.buildUpstreamMaterials(ctx, action)...)

	// 3. Acceptance criteria
	if m := buildAcceptanceMaterial(action); m != nil {
		materials = append(materials, *m)
	}

	// 4. Gate feedback + rework history
	if m := buildGateFeedbackMaterial(action); m != nil {
		materials = append(materials, *m)
	}

	// 5. Feature manifest
	if m := b.buildManifestMaterial(ctx, workItem); m != nil {
		materials = append(materials, *m)
	}

	if len(materials) == 0 {
		// Nothing to write — clean up the empty directory.
		_ = os.RemoveAll(filepath.Join(parentDir, fmt.Sprintf("run-%d", run.ID)))
		return "", nil
	}

	// Write material files.
	for _, m := range materials {
		// Convert forward-slash path to OS path for filesystem operations.
		filePath := filepath.Join(dir, filepath.FromSlash(m.Path))
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			return "", fmt.Errorf("create dir for %s: %w", m.Path, err)
		}
		if err := os.WriteFile(filePath, []byte(m.Content), 0o644); err != nil {
			return "", fmt.Errorf("write %s: %w", m.Path, err)
		}
	}

	// Generate and write SKILL.md index.
	skillMD := generateSkillMD(materials)
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
		return "", fmt.Errorf("write SKILL.md: %w", err)
	}

	return dir, nil
}

// Cleanup removes the action-context directory and its empty parent (run-<id>/).
// Safe to call with empty string.
func Cleanup(dir string) {
	if dir == "" {
		return
	}
	if err := os.RemoveAll(dir); err != nil {
		slog.Warn("action-context: cleanup failed", "dir", dir, "error", err)
	}
	// Try to remove the parent run-<id>/ directory if now empty.
	parent := filepath.Dir(dir)
	_ = os.Remove(parent) // fails silently if non-empty or already gone
}

func buildWorkItemMaterial(wi *core.WorkItem) *ContextMaterial {
	if wi == nil {
		return nil
	}

	var sb strings.Builder
	sb.WriteString("# ")
	sb.WriteString(wi.Title)
	sb.WriteString("\n\n")

	if wi.Priority != "" {
		sb.WriteString("**Priority:** ")
		sb.WriteString(string(wi.Priority))
		sb.WriteString("\n")
	}
	if len(wi.Labels) > 0 {
		sb.WriteString("**Labels:** ")
		sb.WriteString(strings.Join(wi.Labels, ", "))
		sb.WriteString("\n")
	}
	if wi.Priority != "" || len(wi.Labels) > 0 {
		sb.WriteString("\n")
	}

	if strings.TrimSpace(wi.Body) != "" {
		sb.WriteString(wi.Body)
		sb.WriteString("\n")
	}

	return &ContextMaterial{
		Path:        "work-item.md",
		Description: "Full work item details (title, body, priority, labels)",
		Hint:        "At start — understand the full task",
		Content:     sb.String(),
	}
}

func (b *ActionContextBuilder) buildUpstreamMaterials(ctx context.Context, action *core.Action) []ContextMaterial {
	// Find all actions in the same work item that precede this action.
	allActions, err := b.store.ListActionsByWorkItem(ctx, action.WorkItemID)
	if err != nil {
		return nil
	}

	var materials []ContextMaterial
	for _, a := range allActions {
		if a.Position >= action.Position || a.ID == action.ID {
			continue
		}
		run, err := b.store.GetLatestRunWithResult(ctx, a.ID)
		if err != nil || run == nil || strings.TrimSpace(run.ResultMarkdown) == "" {
			continue
		}

		name := sanitizeFileName(a.Name)
		if name == "" {
			name = fmt.Sprintf("action-%d", a.ID)
		}

		materials = append(materials, ContextMaterial{
			Path:        "upstream/" + name + ".md",
			Description: fmt.Sprintf("Full output of upstream action %q", a.Name),
			Hint:        "When you need context from this predecessor action",
			Content:     run.ResultMarkdown,
		})
	}
	return materials
}

func buildAcceptanceMaterial(action *core.Action) *ContextMaterial {
	if action == nil || len(action.AcceptanceCriteria) == 0 {
		return nil
	}

	var sb strings.Builder
	sb.WriteString("# Acceptance Criteria\n\n")
	for i, c := range action.AcceptanceCriteria {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, c))
	}

	return &ContextMaterial{
		Path:        "acceptance.md",
		Description: "Detailed acceptance criteria for this action",
		Hint:        "Before signaling completion — verify all criteria",
		Content:     sb.String(),
	}
}

func buildGateFeedbackMaterial(action *core.Action) *ContextMaterial {
	if action == nil || action.Config == nil {
		return nil
	}

	lastFeedback, _ := action.Config["last_gate_feedback"].(string)
	reworkHistory, _ := action.Config["rework_history"].(string)

	if strings.TrimSpace(lastFeedback) == "" && strings.TrimSpace(reworkHistory) == "" {
		return nil
	}

	var sb strings.Builder
	sb.WriteString("# Gate Feedback\n\n")

	if strings.TrimSpace(lastFeedback) != "" {
		sb.WriteString("## Latest Feedback\n\n")
		sb.WriteString(lastFeedback)
		sb.WriteString("\n\n")
	}

	if strings.TrimSpace(reworkHistory) != "" {
		sb.WriteString("## Rework History\n\n")
		sb.WriteString(reworkHistory)
		sb.WriteString("\n")
	}

	return &ContextMaterial{
		Path:        "gate-feedback.md",
		Description: "Previous review feedback and rework history",
		Hint:        "Immediately — understand what to fix",
		Content:     sb.String(),
	}
}

func (b *ActionContextBuilder) buildManifestMaterial(ctx context.Context, wi *core.WorkItem) *ContextMaterial {
	if wi == nil || wi.ProjectID == nil {
		return nil
	}

	entries, err := b.store.ListFeatureEntries(ctx, core.FeatureEntryFilter{
		ProjectID: *wi.ProjectID,
	})
	if err != nil || len(entries) == 0 {
		return nil
	}

	data := map[string]any{
		"project_id": *wi.ProjectID,
		"entries":    entries,
	}

	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil
	}

	return &ContextMaterial{
		Path:        "manifest.json",
		Description: "Full feature manifest with all entries",
		Hint:        "When you need to understand the project's feature scope",
		Content:     string(jsonBytes),
	}
}

func generateSkillMD(materials []ContextMaterial) string {
	// Check which materials are present for conditional hints.
	hasAcceptance := false
	hasGateFeedback := false
	for _, m := range materials {
		switch m.Path {
		case "acceptance.md":
			hasAcceptance = true
		case "gate-feedback.md":
			hasGateFeedback = true
		}
	}

	var sb strings.Builder
	sb.WriteString(`---
name: action-context
description: Pre-loaded reference materials for the current action run
---

# Action Context

Reference materials for your current task are in this directory.
**Read files on demand** — you don't need to load everything upfront.

## Available Materials

| File | Description | When to read |
|------|-------------|--------------|
`)

	for _, m := range materials {
		sb.WriteString(fmt.Sprintf("| `%s` | %s | %s |\n", m.Path, m.Description, m.Hint))
	}

	sb.WriteString("\n## How to use\n\n")
	sb.WriteString("1. Your task prompt already contains the objective and a brief summary\n")
	sb.WriteString("2. Read individual files above when you need more detail\n")
	if hasAcceptance {
		sb.WriteString("3. Check `acceptance.md` before signaling completion — verify all criteria\n")
	}
	if hasGateFeedback {
		sb.WriteString("4. Read `gate-feedback.md` first — it contains rework instructions\n")
	}

	return sb.String()
}

var invalidFileNameRe = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// sanitizeFileName converts an action name into a safe file name.
func sanitizeFileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	name = strings.ToLower(name)
	name = invalidFileNameRe.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-._")
	if len(name) > 60 {
		name = name[:60]
	}
	return name
}
