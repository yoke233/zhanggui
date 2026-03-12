package flow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// GateResult represents the outcome of a gate evaluation.
type GateResult struct {
	Passed  bool
	Reason  string
	ResetTo []int64 // Step IDs to reset on reject (upstream rework)
	// Metadata is copied from the gate step's Artifact metadata when available.
	// It may include fields like pr_number, pr_url, reject_targets, etc.
	Metadata map[string]any
}

// ProcessGate handles a gate Step: pass → downstream continue, reject → reset upstream + gate re-enters loop.
func (e *IssueEngine) ProcessGate(ctx context.Context, step *core.Step, result GateResult) error {
	if step.Type != core.StepGate {
		return fmt.Errorf("step %d is not a gate (type=%s)", step.ID, step.Type)
	}

	if result.Passed {
		if err := e.transitionStep(ctx, step, core.StepDone); err != nil {
			return err
		}
		e.bus.Publish(ctx, core.Event{
			Type:      core.EventGatePassed,
			IssueID:   step.IssueID,
			StepID:    step.ID,
			Timestamp: time.Now().UTC(),
			Data:      map[string]any{"reason": result.Reason},
		})
		return nil
	}

	// Gate rejected.
	e.bus.Publish(ctx, core.Event{
		Type:      core.EventGateRejected,
		IssueID:   step.IssueID,
		StepID:    step.ID,
		Timestamp: time.Now().UTC(),
		Data:      map[string]any{"reason": result.Reason},
	})

	// Reset upstream steps for rework — persist retry_count via UpdateStep.
	for _, upID := range result.ResetTo {
		up, err := e.store.GetStep(ctx, upID)
		if err != nil {
			return fmt.Errorf("get upstream step %d: %w", upID, err)
		}
		if up.MaxRetries > 0 && up.RetryCount >= up.MaxRetries {
			return core.ErrMaxRetriesExceeded
		}
		recordGateRework(up, step.ID, result.Reason, result.Metadata)
		up.RetryCount++
		up.Status = core.StepPending
		if err := e.store.UpdateStep(ctx, up); err != nil {
			return fmt.Errorf("reset step %d: %w", upID, err)
		}
	}

	// Gate itself → pending (will be re-promoted after upstream completes).
	return e.transitionStep(ctx, step, core.StepPending)
}

// finalizeGate is called after a gate step's executor succeeds.
// It reads the latest Artifact's metadata.verdict to decide pass/reject.
func (e *IssueEngine) finalizeGate(ctx context.Context, step *core.Step) error {
	art, err := e.store.GetLatestArtifactByStep(ctx, step.ID)
	if err == core.ErrNotFound {
		// No artifact — default to pass.
		e.bus.Publish(ctx, core.Event{
			Type:      core.EventGatePassed,
			IssueID:   step.IssueID,
			StepID:    step.ID,
			Timestamp: time.Now().UTC(),
		})
		return e.transitionStep(ctx, step, core.StepDone)
	}
	if err != nil {
		return fmt.Errorf("get gate artifact for step %d: %w", step.ID, err)
	}

	verdict, _ := art.Metadata["verdict"].(string)
	if verdict != "reject" {
		// "pass" or unrecognized → default pass.
		if err := e.mergePRIfConfigured(ctx, step); err != nil {
			// Merge failure: treat as reject and reset upstream for rework.
			resetTo, _ := e.defaultGateResetTargets(ctx, step, art.Metadata)
			reason, metadata := e.formatMergeFailureFeedback(step, err)
			rejectErr := e.ProcessGate(ctx, step, GateResult{
				Passed:   false,
				Reason:   reason,
				ResetTo:  resetTo,
				Metadata: metadata,
			})
			if rejectErr == core.ErrMaxRetriesExceeded {
				_ = e.transitionStep(ctx, step, core.StepBlocked)
				return nil
			}
			return rejectErr
		}

		e.bus.Publish(ctx, core.Event{
			Type:      core.EventGatePassed,
			IssueID:   step.IssueID,
			StepID:    step.ID,
			Timestamp: time.Now().UTC(),
		})
		return e.transitionStep(ctx, step, core.StepDone)
	}

	// Reject — determine targets and delegate to ProcessGate.
	resetTo, reason := e.defaultGateResetTargets(ctx, step, art.Metadata)

	rejectErr := e.ProcessGate(ctx, step, GateResult{
		Passed:  false,
		Reason:  reason,
		ResetTo: resetTo,
		Metadata: func() map[string]any {
			if art != nil {
				return art.Metadata
			}
			return nil
		}(),
	})
	if rejectErr == core.ErrMaxRetriesExceeded {
		_ = e.transitionStep(ctx, step, core.StepBlocked)
		return nil
	}
	return rejectErr
}

// extractResetTargets reads reject_targets from metadata, falling back to predecessors.
func extractResetTargets(metadata map[string]any, fallback []int64) []int64 {
	targets, ok := metadata["reject_targets"].([]any)
	if !ok || len(targets) == 0 {
		return fallback
	}
	var result []int64
	for _, t := range targets {
		if id, ok := toInt64(t); ok {
			result = append(result, id)
		}
	}
	if len(result) == 0 {
		return fallback
	}
	return result
}

func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case int:
		return int64(n), true
	default:
		return 0, false
	}
}

func recordGateRework(step *core.Step, gateStepID int64, reason string, metadata map[string]any) {
	if step == nil {
		return
	}
	if step.Config == nil {
		step.Config = map[string]any{}
	}

	history, _ := step.Config["rework_history"].([]any)
	entry := map[string]any{
		"gate_step_id": gateStepID,
		"reason":       strings.TrimSpace(reason),
		"at":           time.Now().UTC().Format(time.RFC3339Nano),
		"attempt":      step.RetryCount + 2, // next Attempt after this reset (RetryCount will be incremented)
	}
	if metadata != nil {
		if v, ok := metadata["pr_number"]; ok {
			entry["pr_number"] = v
		}
		if v, ok := metadata["pr_url"]; ok {
			entry["pr_url"] = v
		}
		if v, ok := metadata["merge_error"]; ok {
			entry["merge_error"] = v
		}
		if v, ok := metadata["mergeable_state"]; ok {
			entry["mergeable_state"] = v
		}
		if v, ok := metadata["merge_action_hint"]; ok {
			entry["merge_action_hint"] = v
		}
	}
	if strings.TrimSpace(reason) == "" {
		entry["reason"] = "gate rejected"
	}
	history = append(history, entry)
	const maxHistory = 10
	if len(history) > maxHistory {
		history = history[len(history)-maxHistory:]
	}
	step.Config["rework_history"] = history
	step.Config["last_gate_feedback"] = entry
}

func (e *IssueEngine) formatMergeFailureFeedback(step *core.Step, err error) (string, map[string]any) {
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

func (e *IssueEngine) defaultGateResetTargets(ctx context.Context, step *core.Step, metadata map[string]any) (resetTo []int64, reason string) {
	// By default only reset the closest upstream position.
	// Full upstream closure is opt-in via reset_upstream_closure.
	immediatePredecessors := e.immediatePredecessorIDs(ctx, step)
	resetTo = extractResetTargets(metadata, immediatePredecessors)
	if len(resetTo) == 0 {
		resetTo = append([]int64(nil), immediatePredecessors...)
	}
	if step.Config != nil {
		if v, ok := step.Config["reset_upstream_closure"].(bool); ok && v {
			resetTo = e.predecessorIDs(ctx, step)
		}
	}
	reason, _ = metadata["reason"].(string)
	if strings.TrimSpace(reason) == "" {
		reason = "gate rejected"
	}
	return resetTo, reason
}

// predecessorIDs returns IDs of all steps with lower Position in the same issue.
func (e *IssueEngine) predecessorIDs(ctx context.Context, step *core.Step) []int64 {
	steps, err := e.store.ListStepsByIssue(ctx, step.IssueID)
	if err != nil || len(steps) == 0 {
		return nil
	}
	return predecessorStepIDs(steps, step)
}

func (e *IssueEngine) immediatePredecessorIDs(ctx context.Context, step *core.Step) []int64 {
	steps, err := e.store.ListStepsByIssue(ctx, step.IssueID)
	if err != nil || len(steps) == 0 {
		return nil
	}
	return immediatePredecessorStepIDs(steps, step)
}

func (e *IssueEngine) mergePRIfConfigured(ctx context.Context, step *core.Step) error {
	mergeOnPass := false
	mergeMethod := "squash"
	if step.Config != nil {
		if v, ok := step.Config["merge_on_pass"].(bool); ok {
			mergeOnPass = v
		}
		if v, ok := step.Config["merge_method"].(string); ok && strings.TrimSpace(v) != "" {
			mergeMethod = strings.TrimSpace(v)
		}
	}
	if !mergeOnPass {
		return nil
	}

	prNumber, err := e.resolvePRNumber(ctx, step)
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

	token := e.ghTokens.EffectiveMergePAT()
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
	if step.Config != nil {
		for _, key := range []string{
			"organization_id",
			"repository_id",
			"project_id",
			"source_project_id",
			"target_project_id",
			"remove_source_branch",
		} {
			if value, exists := step.Config[key]; exists {
				extra[key] = value
			}
		}
	}

	return provider.Merge(ctx, repo, prNumber, MergeInput{
		Method:        mergeMethod,
		CommitTitle:   fmt.Sprintf("merge: issue %d", step.IssueID),
		CommitMessage: fmt.Sprintf("merged by ai-workflow gate step %d", step.ID),
		Extra:         extra,
	})
}

func (e *IssueEngine) resolvePRNumber(ctx context.Context, step *core.Step) (int, error) {
	// Prefer gate artifact metadata.
	art, err := e.store.GetLatestArtifactByStep(ctx, step.ID)
	if err == nil && art != nil && art.Metadata != nil {
		if n, ok := toInt64(art.Metadata["pr_number"]); ok && n > 0 {
			return int(n), nil
		}
	}

	// Fallback: scan predecessor artifacts.
	predecessors := e.predecessorIDs(ctx, step)
	for _, id := range predecessors {
		a, err := e.store.GetLatestArtifactByStep(ctx, id)
		if err != nil || a == nil || a.Metadata == nil {
			continue
		}
		if n, ok := toInt64(a.Metadata["pr_number"]); ok && n > 0 {
			return int(n), nil
		}
	}
	return 0, fmt.Errorf("pr_number not found for merge")
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
