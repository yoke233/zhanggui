package api

import (
	"net/http"

	"github.com/yoke233/ai-workflow/internal/core"
)

// getUsageSummary returns aggregated usage analytics: totals, by project, by agent, by profile.
func (h *Handler) getUsageSummary(w http.ResponseWriter, r *http.Request) {
	filter := parseAnalyticsFilter(r)
	ctx := r.Context()

	totals, err := h.store.UsageTotals(ctx, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "USAGE_ERROR")
		return
	}

	byProject, err := h.store.UsageByProject(ctx, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "USAGE_ERROR")
		return
	}

	byAgent, err := h.store.UsageByAgent(ctx, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "USAGE_ERROR")
		return
	}

	byProfile, err := h.store.UsageByProfile(ctx, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "USAGE_ERROR")
		return
	}

	if byProject == nil {
		byProject = []core.ProjectUsageSummary{}
	}
	if byAgent == nil {
		byAgent = []core.AgentUsageSummary{}
	}
	if byProfile == nil {
		byProfile = []core.ProfileUsageSummary{}
	}

	writeJSON(w, http.StatusOK, core.UsageAnalyticsSummary{
		Totals:    totals,
		ByProject: byProject,
		ByAgent:   byAgent,
		ByProfile: byProfile,
	})
}

// getUsageByProject returns usage aggregated per project.
func (h *Handler) getUsageByProject(w http.ResponseWriter, r *http.Request) {
	filter := parseAnalyticsFilter(r)
	data, err := h.store.UsageByProject(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "USAGE_ERROR")
		return
	}
	if data == nil {
		data = []core.ProjectUsageSummary{}
	}
	writeJSON(w, http.StatusOK, data)
}

// getUsageByAgent returns usage aggregated per agent.
func (h *Handler) getUsageByAgent(w http.ResponseWriter, r *http.Request) {
	filter := parseAnalyticsFilter(r)
	data, err := h.store.UsageByAgent(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "USAGE_ERROR")
		return
	}
	if data == nil {
		data = []core.AgentUsageSummary{}
	}
	writeJSON(w, http.StatusOK, data)
}

// getUsageByProfile returns usage aggregated per profile.
func (h *Handler) getUsageByProfile(w http.ResponseWriter, r *http.Request) {
	filter := parseAnalyticsFilter(r)
	data, err := h.store.UsageByProfile(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "USAGE_ERROR")
		return
	}
	if data == nil {
		data = []core.ProfileUsageSummary{}
	}
	writeJSON(w, http.StatusOK, data)
}

// getUsageByRun returns usage for a specific execution.
func (h *Handler) getUsageByRun(w http.ResponseWriter, r *http.Request) {
	execID, ok := urlParamInt64(r, "execID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid execution ID", "BAD_REQUEST")
		return
	}

	data, err := h.store.GetUsageByRun(r.Context(), execID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "USAGE_ERROR")
		return
	}
	if data == nil {
		writeError(w, http.StatusNotFound, "no usage record for this execution", "NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, data)
}
