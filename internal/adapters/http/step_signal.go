package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// stepDecisionRequest is the request body for POST /steps/{stepID}/decision.
type stepDecisionRequest struct {
	Decision      string  `json:"decision"`                 // approve | reject | complete | need_help
	Reason        string  `json:"reason"`                   // required
	RejectTargets []int64 `json:"reject_targets,omitempty"` // for reject only
}

func (h *Handler) stepDecision(w http.ResponseWriter, r *http.Request) {
	stepID, ok := urlParamInt64(r, "stepID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid step ID", "BAD_ID")
		return
	}

	var req stepDecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	if strings.TrimSpace(req.Reason) == "" {
		writeError(w, http.StatusBadRequest, "reason is required", "MISSING_REASON")
		return
	}

	var sigType core.SignalType
	switch strings.ToLower(strings.TrimSpace(req.Decision)) {
	case "approve":
		sigType = core.SignalApprove
	case "reject":
		sigType = core.SignalReject
	case "complete":
		sigType = core.SignalComplete
	case "need_help":
		sigType = core.SignalNeedHelp
	default:
		writeError(w, http.StatusBadRequest, "decision must be one of: approve, reject, complete, need_help", "INVALID_DECISION")
		return
	}

	step, err := h.store.GetStep(r.Context(), stepID)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "step not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	// Only allow decisions on running or blocked steps.
	if step.Status != core.StepRunning && step.Status != core.StepBlocked {
		writeError(w, http.StatusConflict, "step is not in a decidable state", "INVALID_STATE")
		return
	}

	payload := map[string]any{"reason": req.Reason}
	if sigType == core.SignalReject && len(req.RejectTargets) > 0 {
		targets := make([]any, len(req.RejectTargets))
		for i, t := range req.RejectTargets {
			targets[i] = t
		}
		payload["reject_targets"] = targets
	}

	sig := &core.StepSignal{
		StepID:    stepID,
		IssueID:   step.IssueID,
		Type:      sigType,
		Source:    core.SignalSourceHuman,
		Payload:   payload,
		Actor:     "human",
		CreatedAt: time.Now().UTC(),
	}
	id, err := h.store.CreateStepSignal(r.Context(), sig)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	sig.ID = id

	// Publish event for engine to pick up.
	h.bus.Publish(r.Context(), core.Event{
		Type:      core.EventStepSignal,
		IssueID:   step.IssueID,
		StepID:    stepID,
		Timestamp: time.Now().UTC(),
		Data:      map[string]any{"signal_id": id, "type": string(sigType), "source": "human"},
	})

	writeJSON(w, http.StatusCreated, sig)
}

// stepUnblockRequest is the request body for POST /steps/{stepID}/unblock.
type stepUnblockRequest struct {
	Reason       string `json:"reason"`                  // required
	Instructions string `json:"instructions,omitempty"`   // optional: forwarded to agent as SignalInstruction
}

func (h *Handler) stepUnblock(w http.ResponseWriter, r *http.Request) {
	stepID, ok := urlParamInt64(r, "stepID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid step ID", "BAD_ID")
		return
	}

	var req stepUnblockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	if strings.TrimSpace(req.Reason) == "" {
		writeError(w, http.StatusBadRequest, "reason is required", "MISSING_REASON")
		return
	}

	step, err := h.store.GetStep(r.Context(), stepID)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "step not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if step.Status != core.StepBlocked {
		writeError(w, http.StatusConflict, "step is not blocked", "INVALID_STATE")
		return
	}

	// Create unblock signal.
	sig := &core.StepSignal{
		StepID:    stepID,
		IssueID:   step.IssueID,
		Type:      core.SignalUnblock,
		Source:    core.SignalSourceHuman,
		Payload:   map[string]any{"reason": req.Reason},
		Actor:     "human",
		CreatedAt: time.Now().UTC(),
	}
	sigID, err := h.store.CreateStepSignal(r.Context(), sig)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	sig.ID = sigID

	// If instructions are provided, create an additional SignalInstruction
	// that will be picked up by ResolveLatestFeedback and forwarded to the agent.
	if strings.TrimSpace(req.Instructions) != "" {
		instrSig := &core.StepSignal{
			StepID:    stepID,
			IssueID:   step.IssueID,
			Type:      core.SignalInstruction,
			Source:    core.SignalSourceHuman,
			Summary:   "human instruction on unblock",
			Content:   strings.TrimSpace(req.Instructions),
			Payload:   map[string]any{"reason": req.Reason, "instructions": req.Instructions},
			Actor:     "human",
			CreatedAt: time.Now().UTC(),
		}
		_, _ = h.store.CreateStepSignal(r.Context(), instrSig)
	}

	// Transition step back to pending for retry.
	step.Status = core.StepPending
	if err := h.store.UpdateStep(r.Context(), step); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	h.bus.Publish(r.Context(), core.Event{
		Type:      core.EventStepUnblocked,
		IssueID:   step.IssueID,
		StepID:    stepID,
		Timestamp: time.Now().UTC(),
		Data:      map[string]any{"signal_id": sigID, "reason": req.Reason},
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "unblocked",
		"signal": sig,
		"step":   step,
	})
}

// pendingDecisionItem wraps a step with its latest context signals for richer inbox display.
type pendingDecisionItem struct {
	Step          *core.Step        `json:"step"`
	LatestContext *core.StepSignal  `json:"latest_context,omitempty"`
	Signals       []*core.StepSignal `json:"signals,omitempty"`
}

func (h *Handler) listPendingDecisions(w http.ResponseWriter, r *http.Request) {
	issueID, hasIssue := queryInt64(r, "issue_id")

	var steps []*core.Step
	var err error
	if hasIssue {
		steps, err = h.store.ListPendingHumanSteps(r.Context(), issueID)
	} else {
		steps, err = h.store.ListAllPendingHumanSteps(r.Context())
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if steps == nil {
		steps = []*core.Step{}
	}

	// Enrich each step with latest context signal and recent signals.
	items := make([]pendingDecisionItem, 0, len(steps))
	for _, step := range steps {
		item := pendingDecisionItem{Step: step}
		// Attach latest context/feedback signal.
		if latestCtx, _ := h.store.GetLatestStepSignal(r.Context(), step.ID, core.SignalContext, core.SignalFeedback); latestCtx != nil {
			item.LatestContext = latestCtx
		}
		// Attach recent signals (up to 10).
		if signals, _ := h.store.ListStepSignals(r.Context(), step.ID); len(signals) > 0 {
			if len(signals) > 10 {
				signals = signals[len(signals)-10:]
			}
			item.Signals = signals
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) listStepSignals(w http.ResponseWriter, r *http.Request) {
	stepID, ok := urlParamInt64(r, "stepID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid step ID", "BAD_ID")
		return
	}
	signals, err := h.store.ListStepSignals(r.Context(), stepID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if signals == nil {
		signals = []*core.StepSignal{}
	}
	writeJSON(w, http.StatusOK, signals)
}
