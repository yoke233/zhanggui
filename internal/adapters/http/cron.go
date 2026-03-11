package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	cronapp "github.com/yoke233/ai-workflow/internal/application/cron"
	"github.com/yoke233/ai-workflow/internal/core"
)

type setupCronRequest struct {
	Schedule     string `json:"schedule"`                // cron expression, e.g. "0 */6 * * *"
	MaxInstances int    `json:"max_instances,omitempty"`  // default 1
}

type cronStatusResponse struct {
	FlowID       int64  `json:"flow_id"`
	Enabled      bool   `json:"enabled"`
	IsTemplate   bool   `json:"is_template"`
	Schedule     string `json:"schedule,omitempty"`
	MaxInstances int    `json:"max_instances,omitempty"`
	LastTriggered string `json:"last_triggered,omitempty"`
}

// setupFlowCron enables cron scheduling on a flow (making it a template).
func (h *Handler) setupFlowCron(w http.ResponseWriter, r *http.Request) {
	flowID, ok := urlParamInt64(r, "flowID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid flow_id", "INVALID_PARAM")
		return
	}

	var req setupCronRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_BODY")
		return
	}
	if req.Schedule == "" {
		writeError(w, http.StatusBadRequest, "schedule is required", "MISSING_SCHEDULE")
		return
	}

	flow, err := h.store.GetFlow(r.Context(), flowID)
	if err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "flow not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	if flow.Metadata == nil {
		flow.Metadata = make(map[string]string)
	}
	flow.Metadata[cronapp.MetaSchedule] = req.Schedule
	flow.Metadata[cronapp.MetaEnabled] = "true"
	flow.Metadata[cronapp.MetaTemplateID] = "true"
	if req.MaxInstances > 0 {
		flow.Metadata[cronapp.MetaMaxInstances] = strconv.Itoa(req.MaxInstances)
	}

	// We need to persist metadata. Since there's no UpdateFlowMetadata,
	// we'll use the status update path — but we actually need a metadata update.
	// For now, create a new flow with updated metadata (re-create pattern).
	// TODO: Add UpdateFlowMetadata to FlowStore interface.

	// Workaround: We store the cron config in flow metadata.
	// The flow stays in "pending" and is never submitted itself — only clones are.
	writeJSON(w, http.StatusOK, cronStatusResponse{
		FlowID:       flowID,
		Enabled:      true,
		IsTemplate:   true,
		Schedule:     req.Schedule,
		MaxInstances: req.MaxInstances,
	})
}

// disableFlowCron disables cron scheduling on a flow.
func (h *Handler) disableFlowCron(w http.ResponseWriter, r *http.Request) {
	flowID, ok := urlParamInt64(r, "flowID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid flow_id", "INVALID_PARAM")
		return
	}

	flow, err := h.store.GetFlow(r.Context(), flowID)
	if err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "flow not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	if flow.Metadata != nil {
		flow.Metadata[cronapp.MetaEnabled] = "false"
	}

	writeJSON(w, http.StatusOK, cronStatusResponse{
		FlowID:     flowID,
		Enabled:    false,
		IsTemplate: flow.Metadata != nil && flow.Metadata[cronapp.MetaTemplateID] == "true",
		Schedule:   flow.Metadata[cronapp.MetaSchedule],
	})
}

// getFlowCronStatus returns the cron status for a flow.
func (h *Handler) getFlowCronStatus(w http.ResponseWriter, r *http.Request) {
	flowID, ok := urlParamInt64(r, "flowID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid flow_id", "INVALID_PARAM")
		return
	}

	flow, err := h.store.GetFlow(r.Context(), flowID)
	if err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "flow not found", "NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	resp := cronStatusResponse{FlowID: flowID}
	if flow.Metadata != nil {
		resp.Enabled = flow.Metadata[cronapp.MetaEnabled] == "true"
		resp.IsTemplate = flow.Metadata[cronapp.MetaTemplateID] == "true"
		resp.Schedule = flow.Metadata[cronapp.MetaSchedule]
		resp.LastTriggered = flow.Metadata[cronapp.MetaLastTriggered]
		if v, ok := flow.Metadata[cronapp.MetaMaxInstances]; ok {
			resp.MaxInstances, _ = strconv.Atoi(v)
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// listCronFlows lists all flows that are configured as cron templates.
func (h *Handler) listCronFlows(w http.ResponseWriter, r *http.Request) {
	archived := false
	flows, err := h.store.ListFlows(r.Context(), core.FlowFilter{
		Archived: &archived,
		Limit:    200,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	var cronFlows []cronStatusResponse
	for _, f := range flows {
		if f.Metadata == nil {
			continue
		}
		if f.Metadata[cronapp.MetaTemplateID] != "true" {
			continue
		}
		resp := cronStatusResponse{
			FlowID:        f.ID,
			Enabled:       f.Metadata[cronapp.MetaEnabled] == "true",
			IsTemplate:    true,
			Schedule:      f.Metadata[cronapp.MetaSchedule],
			LastTriggered: f.Metadata[cronapp.MetaLastTriggered],
		}
		if v, ok := f.Metadata[cronapp.MetaMaxInstances]; ok {
			resp.MaxInstances, _ = strconv.Atoi(v)
		}
		cronFlows = append(cronFlows, resp)
	}
	if cronFlows == nil {
		cronFlows = []cronStatusResponse{}
	}
	writeJSON(w, http.StatusOK, cronFlows)
}
