package flow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// mergePRIfConfigured attempts to merge the associated PR/MR when merge_on_pass is enabled.
func (e *WorkItemEngine) mergePRIfConfigured(ctx context.Context, action *core.Action) error {
	mergeOnPass := false
	mergeMethod := "squash"
	if action.Config != nil {
		if v, ok := action.Config["merge_on_pass"].(bool); ok {
			mergeOnPass = v
		}
		if v, ok := action.Config["merge_method"].(string); ok && strings.TrimSpace(v) != "" {
			mergeMethod = strings.TrimSpace(v)
		}
	}
	if !mergeOnPass {
		return nil
	}

	prNumber, err := e.resolvePRNumber(ctx, action)
	if err != nil {
		return err
	}

	ws := WorkspaceFromContext(ctx)
	if ws == nil {
		return fmt.Errorf("workspace is required for merge")
	}
	originURL, err := gitOutput(ctx, ws.Path, nil, "remote", "get-url", "origin")
	if err != nil {
		return fmt.Errorf("resolve origin url: %w", err)
	}
	originURL = strings.TrimSpace(originURL)

	token := e.scmTokens.EffectivePAT()
	if strings.TrimSpace(token) == "" {
		return fmt.Errorf("missing merge PAT")
	}

	if e.crFactory == nil {
		return fmt.Errorf("change request provider factory is not configured")
	}

	providers := e.crFactory(token)
	provider, repo, ok, err := detectChangeRequestProvider(ctx, originURL, providers)
	if err != nil {
		return err
	}
	if !ok || provider == nil {
		return fmt.Errorf("unsupported origin for merge: %s", originURL)
	}

	extra := map[string]any{}
	if ws.Metadata != nil {
		for _, key := range []string{
			"organization_id",
			"repository_id",
			"project_id",
			"source_project_id",
			"target_project_id",
			"remove_source_branch",
		} {
			if value, exists := ws.Metadata[key]; exists {
				extra[key] = value
			}
		}
	}
	if action.Config != nil {
		for _, key := range []string{
			"organization_id",
			"repository_id",
			"project_id",
			"source_project_id",
			"target_project_id",
			"remove_source_branch",
		} {
			if value, exists := action.Config[key]; exists {
				extra[key] = value
			}
		}
	}

	return provider.Merge(ctx, repo, prNumber, MergeInput{
		Method:        mergeMethod,
		CommitTitle:   fmt.Sprintf("merge: work item %d", action.WorkItemID),
		CommitMessage: fmt.Sprintf("merged by ai-workflow gate action %d", action.ID),
		Extra:         extra,
	})
}

// resolvePRNumber finds the PR number from gate deliverable or predecessor deliverables.
func (e *WorkItemEngine) resolvePRNumber(ctx context.Context, action *core.Action) (int, error) {
	// Prefer gate deliverable metadata.
	deliverable, err := e.store.GetLatestDeliverableByAction(ctx, action.ID)
	if err == nil && deliverable != nil && deliverable.Metadata != nil {
		if n, ok := toInt64(deliverable.Metadata["pr_number"]); ok && n > 0 {
			return int(n), nil
		}
	}

	// Fallback: scan predecessor deliverables.
	predecessors := e.predecessorIDs(ctx, action)
	for _, id := range predecessors {
		d, err := e.store.GetLatestDeliverableByAction(ctx, id)
		if err != nil || d == nil || d.Metadata == nil {
			continue
		}
		if n, ok := toInt64(d.Metadata["pr_number"]); ok && n > 0 {
			return int(n), nil
		}
	}
	return 0, fmt.Errorf("pr_number not found for merge")
}

// handleMergeConflictBlock detects merge conflicts (dirty) and immediately blocks
// the gate for human resolution instead of entering a rework cycle.
// Returns true if the error was a merge conflict that was handled.
func (e *WorkItemEngine) handleMergeConflictBlock(ctx context.Context, action *core.Action, err error) bool {
	reason, metadata := e.formatMergeFailureFeedback(action, err)

	var mergeErr *MergeError
	if !errors.As(err, &mergeErr) || mergeErr == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(mergeErr.MergeableState), "dirty") {
		return false
	}

	e.recordMergeConflict(ctx, action, reason, metadata)
	e.bus.Publish(ctx, core.Event{
		Type:       core.EventGateAwaitingHuman,
		WorkItemID: action.WorkItemID,
		ActionID:   action.ID,
		Timestamp:  time.Now().UTC(),
		Data:       metadata,
	})
	_ = e.transitionAction(ctx, action, core.ActionBlocked)
	return true
}

// formatMergeFailureFeedback builds a human-readable reason and metadata map from a merge error.
func (e *WorkItemEngine) formatMergeFailureFeedback(action *core.Action, err error) (string, map[string]any) {
	metadata := map[string]any{
		"merge_error": err.Error(),
	}
	reason := "merge failed: " + err.Error()

	var mergeErr *MergeError
	if !errors.As(err, &mergeErr) || mergeErr == nil {
		return reason, metadata
	}

	if mergeErr.Number > 0 {
		metadata["pr_number"] = mergeErr.Number
	}
	if strings.TrimSpace(mergeErr.URL) != "" {
		metadata["pr_url"] = strings.TrimSpace(mergeErr.URL)
	}
	if strings.TrimSpace(mergeErr.MergeableState) != "" {
		metadata["mergeable_state"] = strings.TrimSpace(mergeErr.MergeableState)
	}
	if strings.TrimSpace(mergeErr.Provider) != "" {
		metadata["merge_provider"] = strings.TrimSpace(mergeErr.Provider)
	}

	providerPrompts := e.getPRFlowPrompts().Provider(mergeErr.Provider)
	hint := providerPrompts.MergeStates.Default
	switch strings.ToLower(strings.TrimSpace(mergeErr.MergeableState)) {
	case "dirty":
		hint = providerPrompts.MergeStates.Dirty
	case "blocked":
		hint = providerPrompts.MergeStates.Blocked
	case "behind":
		hint = providerPrompts.MergeStates.Behind
	case "unstable":
		hint = providerPrompts.MergeStates.Unstable
	case "draft":
		hint = providerPrompts.MergeStates.Draft
	}
	if strings.TrimSpace(hint) == "" {
		hint = DefaultPRFlowPrompts().Provider(mergeErr.Provider).MergeStates.Default
	}
	metadata["merge_action_hint"] = hint

	return renderMergeReworkFeedbackTemplate(providerPrompts.MergeReworkFeedback, mergeReworkTemplateVars{
		PRNumber:       mergeErr.Number,
		PRURL:          strings.TrimSpace(mergeErr.URL),
		Provider:       strings.TrimSpace(mergeErr.Provider),
		MergeableState: strings.TrimSpace(mergeErr.MergeableState),
		Message:        mergeErr.Message,
		Hint:           hint,
	}), metadata
}

type mergeReworkTemplateVars struct {
	PRNumber       int
	PRURL          string
	Provider       string
	MergeableState string
	Message        string
	Hint           string
}

func renderMergeReworkFeedbackTemplate(tmplText string, vars mergeReworkTemplateVars) string {
	if strings.TrimSpace(tmplText) == "" {
		tmplText = DefaultPRFlowPrompts().Global.MergeReworkFeedback
	}
	tmpl, err := template.New("merge-rework-feedback").Parse(tmplText)
	if err != nil {
		return fmt.Sprintf("自动合并失败。%s", vars.Hint)
	}
	var sb strings.Builder
	if err := tmpl.Execute(&sb, vars); err != nil {
		return fmt.Sprintf("自动合并失败。%s", vars.Hint)
	}
	out := strings.TrimSpace(sb.String())
	if out == "" {
		return fmt.Sprintf("自动合并失败。%s", vars.Hint)
	}
	return out
}

// recordMergeConflict creates a SignalContext on the gate action,
// recording merge conflict details as a structured signal.
func (e *WorkItemEngine) recordMergeConflict(ctx context.Context, gateAction *core.Action, reason string, metadata map[string]any) {
	var content strings.Builder
	content.WriteString(reason)
	if metadata != nil {
		if mergeErr, ok := metadata["merge_error"].(string); ok {
			content.WriteString("\n\nMerge Error: ")
			content.WriteString(mergeErr)
		}
		if hint, ok := metadata["merge_action_hint"].(string); ok && strings.TrimSpace(hint) != "" {
			content.WriteString("\nAction: ")
			content.WriteString(strings.TrimSpace(hint))
		}
	}

	sig := &core.ActionSignal{
		ActionID:   gateAction.ID,
		WorkItemID: gateAction.WorkItemID,
		Type:       core.SignalContext,
		Source:     core.SignalSourceSystem,
		Summary:    "merge_conflict",
		Content:    content.String(),
		Payload:    metadata,
		Actor:      "system",
		CreatedAt:  time.Now().UTC(),
	}
	if _, err := e.store.CreateActionSignal(ctx, sig); err != nil {
		slog.Error("failed to record merge conflict signal", "action_id", gateAction.ID, "error", err)
	}
}

func detectChangeRequestProvider(ctx context.Context, originURL string, providers []ChangeRequestProvider) (ChangeRequestProvider, ChangeRequestRepo, bool, error) {
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		repo, ok, err := provider.Detect(ctx, originURL)
		if err != nil {
			return nil, ChangeRequestRepo{}, false, err
		}
		if ok {
			return provider, repo, true, nil
		}
	}
	return nil, ChangeRequestRepo{}, false, nil
}

func gitOutput(ctx context.Context, dir string, extraEnv []string, args ...string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}
