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

	"github.com/yoke233/ai-workflow/internal/core"
)

// ContextMaterial represents one reference file in the step-context skill.
type ContextMaterial struct {
	Path        string // relative path within skill dir, e.g. "issue.md"
	Description string // one-line description for the index table
	Hint        string // when the agent should read this
	Content     string // file content
}

// StepContextBuilder generates a per-execution step-context skill directory
// containing full reference materials that the agent can read on demand.
type StepContextBuilder struct {
	store core.Store
}

// NewStepContextBuilder creates a new StepContextBuilder.
func NewStepContextBuilder(store core.Store) *StepContextBuilder {
	return &StepContextBuilder{store: store}
}

// Build generates the step-context directory under parentDir/exec-<id>/step-context/
// and returns the full path. The caller must defer Cleanup(dir).
//
// Materials generated (when data exists):
//   - issue.md:           full Issue title + body + metadata
//   - upstream/<name>.md: full ResultMarkdown of each predecessor step
//   - acceptance.md:      numbered acceptance criteria
//   - gate-feedback.md:   last_gate_feedback + rework_history
//   - manifest.json:      full feature manifest entries
//   - SKILL.md:           auto-generated index of all above
func (b *StepContextBuilder) Build(ctx context.Context, parentDir string,
	step *core.Step, exec *core.Execution) (string, error) {

	if step == nil || exec == nil {
		return "", fmt.Errorf("step and execution must not be nil")
	}

	dir := filepath.Join(parentDir, fmt.Sprintf("exec-%d", exec.ID), "step-context")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create step-context dir: %w", err)
	}

	// Query issue once; reused by buildIssueMaterial and buildManifestMaterial.
	issue, _ := b.store.GetIssue(ctx, step.IssueID)

	var materials []ContextMaterial

	// 1. Issue (full body, not truncated)
	if m := buildIssueMaterial(issue); m != nil {
		materials = append(materials, *m)
	}

	// 2. Upstream artifacts (full ResultMarkdown of each predecessor step)
	// TODO: Position-based filtering assumes linear pipeline; revisit for DAG scheduling
	// where parallel steps may share the same Position.
	materials = append(materials, b.buildUpstreamMaterials(ctx, step)...)

	// 3. Acceptance criteria
	if m := buildAcceptanceMaterial(step); m != nil {
		materials = append(materials, *m)
	}

	// 4. Gate feedback + rework history
	if m := buildGateFeedbackMaterial(step); m != nil {
		materials = append(materials, *m)
	}

	// 5. Feature manifest
	if m := b.buildManifestMaterial(ctx, issue); m != nil {
		materials = append(materials, *m)
	}

	if len(materials) == 0 {
		// Nothing to write — clean up the empty directory.
		_ = os.RemoveAll(filepath.Join(parentDir, fmt.Sprintf("exec-%d", exec.ID)))
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

// Cleanup removes the step-context directory and its empty parent (exec-<id>/).
// Safe to call with empty string.
func Cleanup(dir string) {
	if dir == "" {
		return
	}
	if err := os.RemoveAll(dir); err != nil {
		slog.Warn("step-context: cleanup failed", "dir", dir, "error", err)
	}
	// Try to remove the parent exec-<id>/ directory if now empty.
	parent := filepath.Dir(dir)
	_ = os.Remove(parent) // fails silently if non-empty or already gone
}

func buildIssueMaterial(issue *core.Issue) *ContextMaterial {
	if issue == nil {
		return nil
	}

	var sb strings.Builder
	sb.WriteString("# ")
	sb.WriteString(issue.Title)
	sb.WriteString("\n\n")

	if issue.Priority != "" {
		sb.WriteString("**Priority:** ")
		sb.WriteString(string(issue.Priority))
		sb.WriteString("\n")
	}
	if len(issue.Labels) > 0 {
		sb.WriteString("**Labels:** ")
		sb.WriteString(strings.Join(issue.Labels, ", "))
		sb.WriteString("\n")
	}
	if issue.Priority != "" || len(issue.Labels) > 0 {
		sb.WriteString("\n")
	}

	if strings.TrimSpace(issue.Body) != "" {
		sb.WriteString(issue.Body)
		sb.WriteString("\n")
	}

	return &ContextMaterial{
		Path:        "issue.md",
		Description: "Full work item details (title, body, priority, labels)",
		Hint:        "At start — understand the full task",
		Content:     sb.String(),
	}
}

func (b *StepContextBuilder) buildUpstreamMaterials(ctx context.Context, step *core.Step) []ContextMaterial {
	// Find all steps in the same issue that precede this step.
	allSteps, err := b.store.ListStepsByIssue(ctx, step.IssueID)
	if err != nil {
		return nil
	}

	var materials []ContextMaterial
	for _, s := range allSteps {
		if s.Position >= step.Position || s.ID == step.ID {
			continue
		}
		art, err := b.store.GetLatestArtifactByStep(ctx, s.ID)
		if err != nil || art == nil || strings.TrimSpace(art.ResultMarkdown) == "" {
			continue
		}

		name := sanitizeFileName(s.Name)
		if name == "" {
			name = fmt.Sprintf("step-%d", s.ID)
		}

		materials = append(materials, ContextMaterial{
			Path:        "upstream/" + name + ".md",
			Description: fmt.Sprintf("Full output of upstream step %q", s.Name),
			Hint:        "When you need context from this predecessor step",
			Content:     art.ResultMarkdown,
		})
	}
	return materials
}

func buildAcceptanceMaterial(step *core.Step) *ContextMaterial {
	if step == nil || len(step.AcceptanceCriteria) == 0 {
		return nil
	}

	var sb strings.Builder
	sb.WriteString("# Acceptance Criteria\n\n")
	for i, c := range step.AcceptanceCriteria {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, c))
	}

	return &ContextMaterial{
		Path:        "acceptance.md",
		Description: "Detailed acceptance criteria for this step",
		Hint:        "Before signaling completion — verify all criteria",
		Content:     sb.String(),
	}
}

func buildGateFeedbackMaterial(step *core.Step) *ContextMaterial {
	if step == nil || step.Config == nil {
		return nil
	}

	lastFeedback, _ := step.Config["last_gate_feedback"].(string)
	reworkHistory, _ := step.Config["rework_history"].(string)

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

func (b *StepContextBuilder) buildManifestMaterial(ctx context.Context, issue *core.Issue) *ContextMaterial {
	if issue == nil || issue.ProjectID == nil {
		return nil
	}

	manifest, err := b.store.GetFeatureManifestByProject(ctx, *issue.ProjectID)
	if err != nil || manifest == nil {
		return nil
	}

	entries, err := b.store.ListFeatureEntries(ctx, core.FeatureEntryFilter{
		ManifestID: manifest.ID,
	})
	if err != nil {
		return nil
	}

	data := map[string]any{
		"manifest_id": manifest.ID,
		"project_id":  manifest.ProjectID,
		"version":     manifest.Version,
		"summary":     manifest.Summary,
		"entries":     entries,
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
name: step-context
description: Pre-loaded reference materials for the current step execution
assign_when: Automatically injected for exec and gate steps
version: 1
---

# Step Context

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

// sanitizeFileName converts a step name into a safe file name.
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
