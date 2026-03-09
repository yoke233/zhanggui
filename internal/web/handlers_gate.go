package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/yoke233/ai-workflow/internal/core"
)

type gateHandlers struct {
	store    core.Store
	resolver interface {
		ResolveGate(ctx context.Context, issueID, gateName, action, reason string) (*core.Issue, error)
	}
}

// gateStatusResponse aggregates checks by gate name.
type gateStatusResponse struct {
	Name     string           `json:"name"`
	Type     string           `json:"type"`
	Status   string           `json:"status"`
	Attempts int              `json:"attempts"`
	Checks   []core.GateCheck `json:"checks"`
}

type gatesListResponse struct {
	Gates []gateStatusResponse `json:"gates"`
}

func (h *gateHandlers) listGates(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}

	issueID := strings.TrimSpace(chi.URLParam(r, "id"))
	if issueID == "" {
		writeAPIError(w, http.StatusBadRequest, "issue id is required", "ISSUE_ID_REQUIRED")
		return
	}

	checks, err := h.store.GetGateChecks(issueID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to get gate checks", "GET_GATE_CHECKS_FAILED")
		return
	}

	// Aggregate checks by gate_name, preserving order of first appearance.
	gateOrder := make([]string, 0)
	gateMap := make(map[string]*gateStatusResponse)

	for _, check := range checks {
		gs, ok := gateMap[check.GateName]
		if !ok {
			gs = &gateStatusResponse{
				Name:   check.GateName,
				Type:   string(check.GateType),
				Status: string(core.GateStatusPending),
				Checks: make([]core.GateCheck, 0),
			}
			gateMap[check.GateName] = gs
			gateOrder = append(gateOrder, check.GateName)
		}
		gs.Checks = append(gs.Checks, check)
		gs.Attempts = len(gs.Checks)
		// The latest check determines overall gate status.
		gs.Status = string(check.Status)
	}

	gates := make([]gateStatusResponse, 0, len(gateOrder))
	for _, name := range gateOrder {
		gates = append(gates, *gateMap[name])
	}

	writeJSON(w, http.StatusOK, gatesListResponse{Gates: gates})
}

type resolveGateRequest struct {
	Action string `json:"action"` // "pass" or "fail"
	Reason string `json:"reason"`
}

func (h *gateHandlers) resolveGate(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}
	if h.resolver == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "gate resolver is not configured", "GATE_RESOLVER_UNAVAILABLE")
		return
	}

	issueID := strings.TrimSpace(chi.URLParam(r, "id"))
	gateName := strings.TrimSpace(chi.URLParam(r, "gateName"))
	if issueID == "" {
		writeAPIError(w, http.StatusBadRequest, "issue id is required", "ISSUE_ID_REQUIRED")
		return
	}
	if gateName == "" {
		writeAPIError(w, http.StatusBadRequest, "gate name is required", "GATE_NAME_REQUIRED")
		return
	}

	var body resolveGateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid request body", "INVALID_BODY")
		return
	}

	action := strings.TrimSpace(strings.ToLower(body.Action))
	if action != "pass" && action != "fail" {
		writeAPIError(w, http.StatusBadRequest, "action must be 'pass' or 'fail'", "INVALID_ACTION")
		return
	}

	updated, err := h.resolver.ResolveGate(r.Context(), issueID, gateName, action, strings.TrimSpace(body.Reason))
	if err != nil {
		switch {
		case isNotFoundError(err):
			writeAPIError(w, http.StatusNotFound, err.Error(), "GATE_NOT_FOUND")
		case strings.Contains(strings.ToLower(err.Error()), "not pending"),
			strings.Contains(strings.ToLower(err.Error()), "not configured"),
			strings.Contains(strings.ToLower(err.Error()), "unsupported gate action"),
			strings.Contains(strings.ToLower(err.Error()), "gate name is required"):
			writeAPIError(w, http.StatusBadRequest, err.Error(), "INVALID_GATE_RESOLUTION")
		default:
			writeAPIError(w, http.StatusInternalServerError, "failed to resolve gate", "RESOLVE_GATE_FAILED")
		}
		return
	}

	response := map[string]string{"status": "ok"}
	if updated != nil {
		response["issue_status"] = string(updated.Status)
	}
	writeJSON(w, http.StatusOK, response)
}
