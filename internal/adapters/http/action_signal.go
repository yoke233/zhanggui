package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	httpx "github.com/yoke233/zhanggui/internal/adapters/http/server"
	"github.com/yoke233/zhanggui/internal/core"
)

func (h *Handler) actionDecision(w http.ResponseWriter, r *http.Request) {
	actionID, ok := urlParamInt64(r, "actionID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid action ID", "BAD_ID")
		return
	}

	var raw map[string]any
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	decision, reason, rejectTargets, payload, err := parseActionDecisionPayload(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error(), "BAD_REQUEST")
		return
	}
	if strings.TrimSpace(reason) == "" {
		writeError(w, http.StatusBadRequest, "reason is required", "MISSING_REASON")
		return
	}

	var sigType core.SignalType
	switch strings.ToLower(strings.TrimSpace(decision)) {
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

	action, err := h.store.GetAction(r.Context(), actionID)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "action not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	// Only allow decisions on running or blocked actions.
	if action.Status != core.ActionRunning && action.Status != core.ActionBlocked {
		writeError(w, http.StatusConflict, "action is not in a decidable state", "INVALID_STATE")
		return
	}

	payload["reason"] = reason
	if sigType == core.SignalReject && len(rejectTargets) > 0 {
		payload["reject_targets"] = rejectTargets
	}
	signalSource := core.SignalSourceHuman
	actor := "human"
	if info, ok := httpx.AuthFromContext(r.Context()); ok {
		role := strings.TrimSpace(info.Role)
		if role != "" && role != "admin" {
			signalSource = core.SignalSourceAgent
			actor = role
		}
	}

	sig := &core.ActionSignal{
		ActionID:   actionID,
		WorkItemID: action.WorkItemID,
		Type:       sigType,
		Source:     signalSource,
		Payload:    payload,
		Actor:      actor,
		CreatedAt:  time.Now().UTC(),
	}
	id, err := h.store.CreateActionSignal(r.Context(), sig)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	sig.ID = id

	// Publish event for engine to pick up.
	h.bus.Publish(r.Context(), core.Event{
		Type:       core.EventActionSignal,
		WorkItemID: action.WorkItemID,
		ActionID:   actionID,
		Timestamp:  time.Now().UTC(),
		Data:       map[string]any{"signal_id": id, "type": string(sigType), "source": string(signalSource)},
	})

	writeJSON(w, http.StatusCreated, sig)
}

func parseActionDecisionPayload(raw map[string]any) (decision string, reason string, rejectTargets []any, payload map[string]any, err error) {
	if raw == nil {
		return "", "", nil, nil, fmt.Errorf("invalid JSON body")
	}
	payload = make(map[string]any, len(raw))
	for k, v := range raw {
		payload[k] = v
	}

	decision, _ = payload["decision"].(string)
	reason, _ = payload["reason"].(string)
	delete(payload, "decision")
	delete(payload, "reason")

	if rawTargets, ok := payload["reject_targets"]; ok {
		targets, convErr := normalizeRejectTargets(rawTargets)
		if convErr != nil {
			return "", "", nil, nil, convErr
		}
		rejectTargets = targets
		payload["reject_targets"] = targets
	}
	return decision, reason, rejectTargets, payload, nil
}

func normalizeRejectTargets(raw any) ([]any, error) {
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("reject_targets must be an array")
	}
	targets := make([]any, 0, len(items))
	for _, item := range items {
		switch v := item.(type) {
		case float64:
			targets = append(targets, int64(v))
		case int64:
			targets = append(targets, v)
		case int:
			targets = append(targets, int64(v))
		default:
			return nil, fmt.Errorf("reject_targets must contain numeric action IDs")
		}
	}
	return targets, nil
}

// actionUnblockRequest is the request body for POST /actions/{actionID}/unblock.
type actionUnblockRequest struct {
	Reason       string `json:"reason"`                 // required
	Instructions string `json:"instructions,omitempty"` // optional: forwarded to agent as SignalInstruction
}

func (h *Handler) actionUnblock(w http.ResponseWriter, r *http.Request) {
	actionID, ok := urlParamInt64(r, "actionID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid action ID", "BAD_ID")
		return
	}

	var req actionUnblockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	if strings.TrimSpace(req.Reason) == "" {
		writeError(w, http.StatusBadRequest, "reason is required", "MISSING_REASON")
		return
	}

	action, err := h.store.GetAction(r.Context(), actionID)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "action not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if action.Status != core.ActionBlocked {
		writeError(w, http.StatusConflict, "action is not blocked", "INVALID_STATE")
		return
	}

	// Create unblock signal.
	sig := &core.ActionSignal{
		ActionID:   actionID,
		WorkItemID: action.WorkItemID,
		Type:       core.SignalUnblock,
		Source:     core.SignalSourceHuman,
		Payload:    map[string]any{"reason": req.Reason},
		Actor:      "human",
		CreatedAt:  time.Now().UTC(),
	}
	sigID, err := h.store.CreateActionSignal(r.Context(), sig)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	sig.ID = sigID

	// If instructions are provided, create an additional SignalInstruction
	// that will be picked up by ResolveLatestFeedback and forwarded to the agent.
	if strings.TrimSpace(req.Instructions) != "" {
		instrSig := &core.ActionSignal{
			ActionID:   actionID,
			WorkItemID: action.WorkItemID,
			Type:       core.SignalInstruction,
			Source:     core.SignalSourceHuman,
			Summary:    "human instruction on unblock",
			Content:    strings.TrimSpace(req.Instructions),
			Payload:    map[string]any{"reason": req.Reason, "instructions": req.Instructions},
			Actor:      "human",
			CreatedAt:  time.Now().UTC(),
		}
		_, _ = h.store.CreateActionSignal(r.Context(), instrSig)
	}

	// Transition action back to pending for retry.
	action.Status = core.ActionPending
	if err := h.store.UpdateAction(r.Context(), action); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	h.bus.Publish(r.Context(), core.Event{
		Type:       core.EventActionUnblocked,
		WorkItemID: action.WorkItemID,
		ActionID:   actionID,
		Timestamp:  time.Now().UTC(),
		Data:       map[string]any{"signal_id": sigID, "reason": req.Reason},
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "unblocked",
		"signal": sig,
		"action": action,
	})
}

// pendingDecisionItem wraps an action with its latest context signals for richer inbox display.
type pendingDecisionItem struct {
	Action        *core.Action         `json:"action"`
	LatestContext *core.ActionSignal   `json:"latest_context,omitempty"`
	Signals       []*core.ActionSignal `json:"signals,omitempty"`
}

func (h *Handler) listPendingDecisions(w http.ResponseWriter, r *http.Request) {
	workItemID, hasWorkItem := queryInt64(r, "work_item_id")

	var actions []*core.Action
	var err error
	if hasWorkItem {
		actions, err = h.store.ListPendingHumanActions(r.Context(), workItemID)
	} else {
		actions, err = h.store.ListAllPendingHumanActions(r.Context())
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if actions == nil {
		actions = []*core.Action{}
	}

	// Enrich each action with latest context signal and recent signals.
	items := make([]pendingDecisionItem, 0, len(actions))
	for _, action := range actions {
		item := pendingDecisionItem{Action: action}
		// Attach latest context/feedback signal.
		if latestCtx, _ := h.store.GetLatestActionSignal(r.Context(), action.ID, core.SignalContext, core.SignalFeedback); latestCtx != nil {
			item.LatestContext = latestCtx
		}
		// Attach recent signals (up to 10).
		if signals, _ := h.store.ListActionSignals(r.Context(), action.ID); len(signals) > 0 {
			if len(signals) > 10 {
				signals = signals[len(signals)-10:]
			}
			item.Signals = signals
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) listActionSignals(w http.ResponseWriter, r *http.Request) {
	actionID, ok := urlParamInt64(r, "actionID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid action ID", "BAD_ID")
		return
	}
	signals, err := h.store.ListActionSignals(r.Context(), actionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if signals == nil {
		signals = []*core.ActionSignal{}
	}
	writeJSON(w, http.StatusOK, signals)
}
