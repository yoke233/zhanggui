package flow

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// GateResult represents the outcome of a gate evaluation.
type GateResult struct {
	Passed  bool
	Reason  string
	ResetTo []int64 // Action IDs to reset on reject (upstream rework)
	// Metadata is copied from the gate action's Deliverable metadata when available.
	// It may include fields like pr_number, pr_url, reject_targets, etc.
	Metadata map[string]any
}

// ProcessGate handles a gate Action: pass → downstream continue, reject → reset upstream + gate re-enters loop.
func (e *WorkItemEngine) ProcessGate(ctx context.Context, action *core.Action, result GateResult) error {
	if action.Type != core.ActionGate {
		return fmt.Errorf("action %d is not a gate (type=%s)", action.ID, action.Type)
	}

	if result.Passed {
		if err := e.transitionAction(ctx, action, core.ActionDone); err != nil {
			return err
		}
		e.bus.Publish(ctx, core.Event{
			Type:       core.EventGatePassed,
			WorkItemID: action.WorkItemID,
			ActionID:   action.ID,
			Timestamp:  time.Now().UTC(),
			Data:       map[string]any{"reason": result.Reason},
		})
		return nil
	}

	// Gate rejected — check rework round limit before cycling.
	maxReworkRounds := 3 // default
	if action.Config != nil {
		if v, ok := action.Config["max_rework_rounds"].(float64); ok && v > 0 {
			maxReworkRounds = int(v)
		}
	}

	// Read rework_count from signal count (single source of truth).
	reworkCount := 0
	if cnt, err := e.store.CountActionSignals(ctx, action.ID, core.SignalReject); err == nil {
		reworkCount = cnt
	}

	if reworkCount >= maxReworkRounds {
		// Rework limit reached — caller will transition to blocked.
		e.bus.Publish(ctx, core.Event{
			Type:       core.EventGateReworkLimitReached,
			WorkItemID: action.WorkItemID,
			ActionID:   action.ID,
			Timestamp:  time.Now().UTC(),
			Data: map[string]any{
				"reason":            result.Reason,
				"rework_count":      reworkCount,
				"max_rework_rounds": maxReworkRounds,
			},
		})
		return core.ErrMaxRetriesExceeded
	}

	// Record a SignalReject on the gate action — single source of truth for rework_count.
	if _, err := e.store.CreateActionSignal(ctx, &core.ActionSignal{
		ActionID:   action.ID,
		WorkItemID: action.WorkItemID,
		Type:       core.SignalReject,
		Source:     core.SignalSourceSystem,
		Summary:    strings.TrimSpace(result.Reason),
		Payload:    result.Metadata,
		Actor:      "gate",
		CreatedAt:  time.Now().UTC(),
	}); err != nil {
		slog.Error("failed to record gate reject signal", "action_id", action.ID, "error", err)
	}

	e.bus.Publish(ctx, core.Event{
		Type:       core.EventGateRejected,
		WorkItemID: action.WorkItemID,
		ActionID:   action.ID,
		Timestamp:  time.Now().UTC(),
		Data:       map[string]any{"reason": result.Reason, "rework_round": reworkCount + 1},
	})

	// Reset upstream actions for rework — persist retry_count via UpdateAction.
	for _, upID := range result.ResetTo {
		up, err := e.store.GetAction(ctx, upID)
		if err != nil {
			return fmt.Errorf("get upstream action %d: %w", upID, err)
		}
		if up.MaxRetries > 0 && up.RetryCount >= up.MaxRetries {
			return core.ErrMaxRetriesExceeded
		}
		e.recordGateRework(ctx, up, action.ID, result.Reason, result.Metadata)
		up.RetryCount++
		up.Status = core.ActionPending
		if err := e.store.UpdateAction(ctx, up); err != nil {
			return fmt.Errorf("reset action %d: %w", upID, err)
		}
	}

	// Gate itself → pending (will be re-promoted after upstream completes).
	return e.transitionAction(ctx, action, core.ActionPending)
}

// processGateReject delegates to ProcessGate and handles ErrMaxRetriesExceeded
// by transitioning the action to blocked (instead of propagating the error).
func (e *WorkItemEngine) processGateReject(ctx context.Context, action *core.Action, result GateResult) error {
	rejectErr := e.ProcessGate(ctx, action, result)
	if rejectErr == core.ErrMaxRetriesExceeded {
		_ = e.transitionAction(ctx, action, core.ActionBlocked)
		return nil
	}
	return rejectErr
}

// GateVerdict represents the outcome of a single gate evaluator.
type GateVerdict struct {
	Decided  bool               // true if this evaluator made a decision
	Passed   bool               // pass or reject (only meaningful when Decided)
	Reason   string             // human-readable reason
	ResetTo  []int64            // action IDs to reset on reject
	Metadata map[string]any     // source context (deliverable.Metadata, signal.Payload); used for merge failure recovery
	Signal   *core.ActionSignal // non-nil for signal-driven verdicts (carries Source for event data)
}

// GateEvaluator evaluates a gate action and optionally returns a verdict.
// If Decided is false, the next evaluator in the chain is tried.
type GateEvaluator func(ctx context.Context, action *core.Action) (GateVerdict, error)

// finalizeGate is called after a gate action's executor succeeds.
// It runs the evaluator chain in order; the first evaluator that returns Decided=true wins.
// Default chain: ActionSignal (MCP/HTTP) → Manifest check → Deliverable metadata.
func (e *WorkItemEngine) finalizeGate(ctx context.Context, action *core.Action) error {
	evaluators := e.gateEvaluators
	if len(evaluators) == 0 {
		evaluators = []GateEvaluator{
			e.evalSignalVerdict,
			e.evalManifestCheck,
			e.evalDeliverableMetadata,
		}
	}
	for _, eval := range evaluators {
		v, err := eval(ctx, action)
		if err != nil {
			return err
		}
		if v.Decided {
			return e.applyGateVerdict(ctx, action, v)
		}
	}
	// No evaluator decided — default pass.
	return e.applyGatePass(ctx, action, GateVerdict{})
}

// applyGateVerdict dispatches a decided verdict to the pass or reject path.
func (e *WorkItemEngine) applyGateVerdict(ctx context.Context, action *core.Action, v GateVerdict) error {
	if v.Passed {
		return e.applyGatePass(ctx, action, v)
	}
	return e.processGateReject(ctx, action, GateResult{
		Passed:   false,
		Reason:   v.Reason,
		ResetTo:  v.ResetTo,
		Metadata: v.Metadata,
	})
}

// applyGatePass handles gate pass: merge PR (if configured), emit event, transition done.
func (e *WorkItemEngine) applyGatePass(ctx context.Context, action *core.Action, v GateVerdict) error {
	if err := e.mergePRIfConfigured(ctx, action); err != nil {
		if e.handleMergeConflictBlock(ctx, action, err) {
			return nil
		}
		mergeReason, mergeMetadata := e.formatMergeFailureFeedback(action, err)
		resetTo, _ := e.defaultGateResetTargets(ctx, action, v.Metadata)
		return e.processGateReject(ctx, action, GateResult{
			Passed:   false,
			Reason:   mergeReason,
			ResetTo:  resetTo,
			Metadata: mergeMetadata,
		})
	}

	// Build event data — signal-driven verdicts carry extra fields.
	var data map[string]any
	if v.Signal != nil {
		data = map[string]any{"signal_source": string(v.Signal.Source)}
		if v.Reason != "" {
			data["reason"] = v.Reason
		}
	}
	e.bus.Publish(ctx, core.Event{
		Type:       core.EventGatePassed,
		WorkItemID: action.WorkItemID,
		ActionID:   action.ID,
		Timestamp:  time.Now().UTC(),
		Data:       data,
	})
	return e.transitionAction(ctx, action, core.ActionDone)
}

// --- Gate Evaluators ---

// evalSignalVerdict checks for an explicit ActionSignal (MCP tool call or human HTTP API).
// System-sourced signals are skipped — those are internal bookkeeping.
func (e *WorkItemEngine) evalSignalVerdict(ctx context.Context, action *core.Action) (GateVerdict, error) {
	signal, _ := e.store.GetLatestActionSignal(ctx, action.ID, core.SignalApprove, core.SignalReject)
	if signal == nil || signal.Source == core.SignalSourceSystem {
		return GateVerdict{}, nil
	}

	if signal.Type == core.SignalApprove {
		reason, _ := signal.Payload["reason"].(string)
		return GateVerdict{
			Decided:  true,
			Passed:   true,
			Reason:   reason,
			Metadata: signal.Payload,
			Signal:   signal,
		}, nil
	}

	// SignalReject
	reason, _ := signal.Payload["reason"].(string)
	if strings.TrimSpace(reason) == "" {
		reason = "gate rejected"
	}
	resetTo := e.immediatePredecessorIDs(ctx, action)
	resetTo = extractResetTargets(signal.Payload, resetTo)
	return GateVerdict{
		Decided:  true,
		Passed:   false,
		Reason:   reason,
		ResetTo:  resetTo,
		Metadata: signal.Payload,
		Signal:   signal,
	}, nil
}

// evalManifestCheck evaluates the feature manifest if manifest_check is enabled.
func (e *WorkItemEngine) evalManifestCheck(ctx context.Context, action *core.Action) (GateVerdict, error) {
	if !manifestCheckEnabled(action) {
		return GateVerdict{}, nil
	}
	passed, reason, err := e.checkManifestEntries(ctx, action)
	if err != nil {
		return GateVerdict{}, fmt.Errorf("manifest check: %w", err)
	}
	if passed {
		return GateVerdict{}, nil // manifest passed — continue to next evaluator
	}
	return GateVerdict{
		Decided: true,
		Passed:  false,
		Reason:  reason,
		ResetTo: e.immediatePredecessorIDs(ctx, action),
	}, nil
}

// evalDeliverableMetadata checks the gate action's deliverable for a verdict field.
func (e *WorkItemEngine) evalDeliverableMetadata(ctx context.Context, action *core.Action) (GateVerdict, error) {
	deliverable, err := e.store.GetLatestDeliverableByAction(ctx, action.ID)
	if err == core.ErrNotFound {
		return GateVerdict{}, nil // no deliverable → continue to default pass
	}
	if err != nil {
		return GateVerdict{}, fmt.Errorf("get gate deliverable for action %d: %w", action.ID, err)
	}

	verdict, _ := deliverable.Metadata["verdict"].(string)
	if verdict != "reject" {
		// "pass" or unrecognized → pass
		return GateVerdict{Decided: true, Passed: true, Metadata: deliverable.Metadata}, nil
	}

	resetTo, reason := e.defaultGateResetTargets(ctx, action, deliverable.Metadata)
	return GateVerdict{
		Decided:  true,
		Passed:   false,
		Reason:   reason,
		ResetTo:  resetTo,
		Metadata: deliverable.Metadata,
	}, nil
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

// recordGateRework creates a SignalFeedback on the upstream action,
// recording the gate rejection as a structured signal.
func (e *WorkItemEngine) recordGateRework(ctx context.Context, upstreamAction *core.Action, gateActionID int64, reason string, metadata map[string]any) {
	summary := strings.TrimSpace(reason)
	if summary == "" {
		summary = "gate rejected"
	}

	// Build formatted content.
	var content strings.Builder
	content.WriteString("Reason: ")
	content.WriteString(summary)
	if metadata != nil {
		if prURL, ok := metadata["pr_url"].(string); ok && strings.TrimSpace(prURL) != "" {
			content.WriteString("\nPR: ")
			content.WriteString(strings.TrimSpace(prURL))
		}
		if n, ok := metadata["pr_number"]; ok {
			content.WriteString("\nPR Number: ")
			content.WriteString(fmt.Sprint(n))
		}
		if hint, ok := metadata["merge_action_hint"].(string); ok && strings.TrimSpace(hint) != "" {
			content.WriteString("\nHint: ")
			content.WriteString(strings.TrimSpace(hint))
		}
	}

	sig := &core.ActionSignal{
		ActionID:       upstreamAction.ID,
		WorkItemID:     upstreamAction.WorkItemID,
		Type:           core.SignalFeedback,
		Source:         core.SignalSourceSystem,
		Summary:        summary,
		Content:        content.String(),
		SourceActionID: gateActionID,
		Payload:        metadata,
		Actor:          "gate",
		CreatedAt:      time.Now().UTC(),
	}
	if _, err := e.store.CreateActionSignal(ctx, sig); err != nil {
		slog.Error("failed to record gate rework signal", "action_id", upstreamAction.ID, "error", err)
	}
}

func (e *WorkItemEngine) defaultGateResetTargets(ctx context.Context, action *core.Action, metadata map[string]any) (resetTo []int64, reason string) {
	// By default only reset the closest upstream position.
	// Full upstream closure is opt-in via reset_upstream_closure.
	immediatePredecessors := e.immediatePredecessorIDs(ctx, action)
	resetTo = extractResetTargets(metadata, immediatePredecessors)
	if len(resetTo) == 0 {
		resetTo = append([]int64(nil), immediatePredecessors...)
	}
	if action.Config != nil {
		if v, ok := action.Config["reset_upstream_closure"].(bool); ok && v {
			resetTo = e.predecessorIDs(ctx, action)
		}
	}
	reason, _ = metadata["reason"].(string)
	if strings.TrimSpace(reason) == "" {
		reason = "gate rejected"
	}
	return resetTo, reason
}

// predecessorIDs returns IDs of all actions with lower Position in the same work item.
func (e *WorkItemEngine) predecessorIDs(ctx context.Context, action *core.Action) []int64 {
	actions, err := e.store.ListActionsByWorkItem(ctx, action.WorkItemID)
	if err != nil || len(actions) == 0 {
		return nil
	}
	return predecessorActionIDs(actions, action)
}

func (e *WorkItemEngine) immediatePredecessorIDs(ctx context.Context, action *core.Action) []int64 {
	actions, err := e.store.ListActionsByWorkItem(ctx, action.WorkItemID)
	if err != nil || len(actions) == 0 {
		return nil
	}
	return immediatePredecessorActionIDs(actions, action)
}
