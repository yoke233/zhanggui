package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// listInspections returns inspection reports with optional filters.
func (h *Handler) listInspections(w http.ResponseWriter, r *http.Request) {
	filter := core.InspectionFilter{
		Limit: queryInt(r, "limit", 20),
	}
	if pid, ok := queryInt64(r, "project_id"); ok {
		filter.ProjectID = &pid
	}
	if s := r.URL.Query().Get("status"); s != "" {
		status := core.InspectionStatus(s)
		filter.Status = &status
	}
	if s := r.URL.Query().Get("since"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			filter.Since = &t
		}
	}
	if s := r.URL.Query().Get("until"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			filter.Until = &t
		}
	}
	filter.Offset = queryInt(r, "offset", 0)

	reports, err := h.store.ListInspections(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "INSPECTION_ERROR")
		return
	}
	if reports == nil {
		reports = []*core.InspectionReport{}
	}
	writeJSON(w, http.StatusOK, reports)
}

// getInspection returns a single inspection report with all findings and insights.
func (h *Handler) getInspection(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "inspectionID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid inspection ID", "INVALID_ID")
		return
	}
	report, err := h.store.GetInspection(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "INSPECTION_ERROR")
		return
	}
	if report == nil {
		writeError(w, http.StatusNotFound, "inspection not found", "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// triggerInspection creates a new manual inspection run.
func (h *Handler) triggerInspection(w http.ResponseWriter, r *http.Request) {
	if h.inspectionEngine == nil {
		writeError(w, http.StatusServiceUnavailable, "inspection engine not configured", "NOT_CONFIGURED")
		return
	}

	var req struct {
		ProjectID  *int64 `json:"project_id,omitempty"`
		LookbackH  int    `json:"lookback_hours,omitempty"`
	}
	if r.Body != nil {
		defer r.Body.Close()
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	if req.LookbackH <= 0 {
		req.LookbackH = 24
	}

	now := time.Now()
	periodStart := now.Add(-time.Duration(req.LookbackH) * time.Hour)

	report, err := h.inspectionEngine.RunInspection(r.Context(), core.InspectionTriggerManual, req.ProjectID, periodStart, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "INSPECTION_ERROR")
		return
	}

	writeJSON(w, http.StatusCreated, report)
}

// listInspectionFindings returns findings for a specific inspection.
func (h *Handler) listInspectionFindings(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "inspectionID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid inspection ID", "INVALID_ID")
		return
	}
	findings, err := h.store.ListFindingsByInspection(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "INSPECTION_ERROR")
		return
	}
	if findings == nil {
		findings = []*core.InspectionFinding{}
	}
	writeJSON(w, http.StatusOK, findings)
}

// listInspectionInsights returns insights for a specific inspection.
func (h *Handler) listInspectionInsights(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "inspectionID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid inspection ID", "INVALID_ID")
		return
	}
	insights, err := h.store.ListInsightsByInspection(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "INSPECTION_ERROR")
		return
	}
	if insights == nil {
		insights = []*core.InspectionInsight{}
	}
	writeJSON(w, http.StatusOK, insights)
}
