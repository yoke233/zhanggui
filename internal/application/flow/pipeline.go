package flow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// Resolver selects an agent for an action based on role and required capabilities.
type Resolver interface {
	Resolve(ctx context.Context, action *core.Action) (agentID string, err error)
}

// InputBuilder assembles the run input string for an action, injecting upstream context.
type InputBuilder interface {
	Build(ctx context.Context, action *core.Action) (string, error)
}

// Collector extracts structured metadata from agent markdown output (via small model + tool_use).
type Collector interface {
	Extract(ctx context.Context, actionType core.ActionType, markdown string) (map[string]any, error)
}

// CompositeExpander decomposes a plan action into child actions for its child work item.
type CompositeExpander interface {
	Expand(ctx context.Context, action *core.Action) ([]*core.Action, error)
}

// ExpanderFunc adapts a plain function into a CompositeExpander.
type ExpanderFunc func(ctx context.Context, action *core.Action) ([]*core.Action, error)

func (f ExpanderFunc) Expand(ctx context.Context, action *core.Action) ([]*core.Action, error) {
	return f(ctx, action)
}

// CollectorFunc adapts a plain function into a Collector.
type CollectorFunc func(ctx context.Context, actionType core.ActionType, markdown string) (map[string]any, error)

func (f CollectorFunc) Extract(ctx context.Context, actionType core.ActionType, markdown string) (map[string]any, error) {
	return f(ctx, actionType, markdown)
}

// prepare resolves agent, builds input (with external resources), and returns values for the Run record.
func (e *WorkItemEngine) prepare(ctx context.Context, action *core.Action) (agentID, inputSnapshot string, err error) {
	if e.preparation.resolver != nil {
		agentID, err = e.preparation.resolver.Resolve(ctx, action)
		if err != nil {
			return "", "", fmt.Errorf("resolve agent for action %d: %w", action.ID, err)
		}
	}

	if e.preparation.inputBuilder != nil {
		inputSnapshot, err = e.preparation.inputBuilder.Build(ctx, action)
		if err != nil {
			return "", "", fmt.Errorf("build input for action %d: %w", action.ID, err)
		}
	}

	// Fetch declared input resources and append their context to the input.
	if e.preparation.resources != nil {
		ws := WorkspaceFromContext(ctx)
		destDir := "/tmp/action-resources/" + fmt.Sprintf("%d", action.ID)
		if ws != nil && ws.Path != "" {
			destDir = ws.Path + "/.resources"
		}
		resolved, fetchErr := e.preparation.resources.FetchInputs(ctx, action.ID, destDir)
		if fetchErr != nil {
			return "", "", fmt.Errorf("fetch input resources for action %d: %w", action.ID, fetchErr)
		}
		if len(resolved) > 0 {
			resourceCtx := FormatInputResourceContext(resolved)
			if resourceCtx != "" {
				if inputSnapshot != "" {
					inputSnapshot += "\n\n# Input Resources\n\n" + resourceCtx
				} else {
					inputSnapshot = "# Input Resources\n\n" + resourceCtx
				}
			}
		}
	}

	return agentID, inputSnapshot, nil
}

// finalize handles the run result: failure path or success path.
func (e *WorkItemEngine) finalize(ctx context.Context, action *core.Action, run *core.Run, runErr error) error {
	finished := time.Now().UTC()
	run.FinishedAt = &finished

	if runErr != nil {
		return e.handleFailure(ctx, action, run, runErr)
	}
	return e.handleSuccess(ctx, action, run)
}

// handleFailure classifies the error and decides: retry, block, or fail.
func (e *WorkItemEngine) handleFailure(ctx context.Context, action *core.Action, run *core.Run, runErr error) error {
	run.Status = core.RunFailed
	run.ErrorMessage = runErr.Error()

	// Auto-classify timeout as transient.
	if run.ErrorKind == "" && errors.Is(runErr, context.DeadlineExceeded) {
		run.ErrorKind = core.ErrKindTransient
	}

	_ = e.workflow.store.UpdateRun(ctx, run)

	e.workflow.bus.Publish(ctx, core.Event{
		Type:       core.EventRunFailed,
		WorkItemID: action.WorkItemID,
		ActionID:   action.ID,
		RunID:      run.ID,
		Timestamp:  time.Now().UTC(),
		Data:       map[string]any{"error": runErr.Error(), "error_kind": string(run.ErrorKind)},
	})

	// Permanent → fail immediately, skip retry.
	if run.ErrorKind == core.ErrKindPermanent {
		_ = e.transitionAction(ctx, action, core.ActionFailed)
		return fmt.Errorf("action %d failed (permanent): %w", action.ID, runErr)
	}

	// Need help → block action for external intervention.
	if run.ErrorKind == core.ErrKindNeedHelp {
		_ = e.transitionAction(ctx, action, core.ActionBlocked)
		return nil // Not an engine error; other actions can continue.
	}

	// Transient or unclassified → retry if budget remains.
	if action.RetryCount < action.MaxRetries {
		action.RetryCount++
		action.Status = core.ActionPending
		if err := e.workflow.store.UpdateAction(ctx, action); err != nil {
			return fmt.Errorf("retry action %d: %w", action.ID, err)
		}
		return nil
	}

	_ = e.transitionAction(ctx, action, core.ActionFailed)
	return fmt.Errorf("action %d failed: %w", action.ID, runErr)
}

// handleSuccess processes a successful run: check signals, collect metadata, then gate finalize or action done.
func (e *WorkItemEngine) handleSuccess(ctx context.Context, action *core.Action, run *core.Run) error {
	run.Status = core.RunSucceeded
	_ = e.workflow.store.UpdateRun(ctx, run)

	e.workflow.bus.Publish(ctx, core.Event{
		Type:       core.EventRunSucceeded,
		WorkItemID: action.WorkItemID,
		ActionID:   action.ID,
		RunID:      run.ID,
		Timestamp:  time.Now().UTC(),
	})

	// 1. Check if agent declared need_help via MCP tool.
	helpSignal, _ := e.workflow.store.GetLatestActionSignal(ctx, action.ID, core.SignalNeedHelp, core.SignalBlocked)
	if helpSignal != nil && helpSignal.RunID == run.ID {
		_ = e.transitionAction(ctx, action, core.ActionBlocked)
		e.workflow.bus.Publish(ctx, core.Event{
			Type:       core.EventActionNeedHelp,
			WorkItemID: action.WorkItemID,
			ActionID:   action.ID,
			RunID:      run.ID,
			Timestamp:  time.Now().UTC(),
			Data:       helpSignal.Payload,
		})
		return nil // non-fatal; other actions can continue
	}

	// 2. Check if agent provided structured completion signal → skip Collector.
	completeSignal, _ := e.workflow.store.GetLatestActionSignal(ctx, action.ID, core.SignalComplete)
	if completeSignal != nil && completeSignal.RunID == run.ID {
		e.applySignalMetadata(ctx, action, run, completeSignal.Payload)
	} else {
		// 3. Fallback: LLM Collector extracts metadata (existing behavior).
		if err := e.collectMetadata(ctx, action); err != nil {
			e.workflow.bus.Publish(ctx, core.Event{
				Type:       core.EventRunFailed,
				WorkItemID: action.WorkItemID,
				ActionID:   action.ID,
				Timestamp:  time.Now().UTC(),
				Data:       map[string]any{"collect_error": err.Error()},
			})
		}
	}

	// Deposit declared output resources after successful execution.
	if e.preparation.resources != nil {
		ws := WorkspaceFromContext(ctx)
		sourceDir := "/tmp/action-resources/" + fmt.Sprintf("%d", action.ID)
		if ws != nil && ws.Path != "" {
			sourceDir = ws.Path
		}
		if depositErr := e.preparation.resources.DepositOutputs(ctx, action, run, sourceDir); depositErr != nil {
			e.workflow.bus.Publish(ctx, core.Event{
				Type:       core.EventRunFailed,
				WorkItemID: action.WorkItemID,
				ActionID:   action.ID,
				RunID:      run.ID,
				Timestamp:  time.Now().UTC(),
				Data:       map[string]any{"deposit_error": depositErr.Error()},
			})
			// Don't fail the action for non-required deposit errors; required errors
			// are already returned as errors from DepositOutputs.
		}
	}

	switch action.Type {
	case core.ActionGate:
		return e.finalizeGate(ctx, action)
	default:
		return e.transitionAction(ctx, action, core.ActionDone)
	}
}

// applySignalMetadata writes agent-provided metadata directly to the action's latest run result,
// bypassing the LLM Collector.
func (e *WorkItemEngine) applySignalMetadata(ctx context.Context, action *core.Action, run *core.Run, payload map[string]any) {
	r, err := e.workflow.store.GetLatestRunWithResult(ctx, action.ID)
	if err != nil {
		return
	}
	if r.ResultMetadata == nil {
		r.ResultMetadata = map[string]any{}
	}
	for k, v := range payload {
		r.ResultMetadata[k] = v
	}
	r.ResultMetadata["signal_source"] = "agent"
	_ = e.workflow.store.UpdateRun(ctx, r)
}

// collectMetadata runs the Collector (if set) to extract structured metadata from the action's latest Run result.
func (e *WorkItemEngine) collectMetadata(ctx context.Context, action *core.Action) error {
	if e.preparation.collector == nil {
		return nil
	}
	r, err := e.workflow.store.GetLatestRunWithResult(ctx, action.ID)
	if err != nil {
		return nil // no result to collect from
	}
	if r.ResultMarkdown == "" {
		return nil
	}

	metadata, err := e.preparation.collector.Extract(ctx, action.Type, r.ResultMarkdown)
	if err != nil {
		return fmt.Errorf("collect metadata for action %d: %w", action.ID, err)
	}

	// Merge extracted metadata into existing metadata (don't overwrite).
	if r.ResultMetadata == nil {
		r.ResultMetadata = metadata
	} else {
		for k, v := range metadata {
			if _, exists := r.ResultMetadata[k]; !exists {
				r.ResultMetadata[k] = v
			}
		}
	}

	return e.workflow.store.UpdateRun(ctx, r)
}

const (
	maxInputRefChars   = 4000
	maxInputTotalChars = 12000
)

// refBudget returns a per-ref character budget based on context type.
// Smaller types get tighter caps so they don't starve higher-value refs.
func refBudget(ref ContextRef) int {
	switch ref.Type {
	case CtxIssueSummary, CtxProjectBrief:
		return 800
	case CtxFeatureManifest:
		return 2000
	case CtxAgentMemory:
		return 1500
	case CtxResourceManifest:
		return 1500
	case CtxProgressSummary:
		return 800
	case CtxSkillsSummary:
		return 1000
	case CtxUpstreamArtifact:
		return maxInputRefChars
	default:
		return maxInputRefChars
	}
}

// renderInputSnapshot is a convenience wrapper around buildInputFromRefs
// for callers that only have an objective + context refs (no action or constraints).
func renderInputSnapshot(objective string, contextRefs []ContextRef) string {
	stub := &core.Action{Config: map[string]any{"objective": objective}}
	return buildInputFromRefs(stub, contextRefs, nil)
}

func truncateText(text string, maxChars int) string {
	text = strings.TrimSpace(text)
	if maxChars <= 0 || len(text) <= maxChars {
		return text
	}
	const suffix = "\n\n[truncated]"
	if maxChars <= len(suffix) {
		return text[:maxChars]
	}
	return strings.TrimSpace(text[:maxChars-len(suffix)]) + suffix
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
