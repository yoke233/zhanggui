package api

import (
	"net/http"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func (h *Handler) getProjectErrorRanking(w http.ResponseWriter, r *http.Request) {
	filter := parseAnalyticsFilter(r)
	data, err := h.store.ProjectErrorRanking(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "ANALYTICS_ERROR")
		return
	}
	if data == nil {
		data = []core.ProjectErrorRank{}
	}
	writeJSON(w, http.StatusOK, data)
}

func (h *Handler) getWorkItemBottleneckActions(w http.ResponseWriter, r *http.Request) {
	filter := parseAnalyticsFilter(r)
	data, err := h.store.WorkItemBottleneckActions(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "ANALYTICS_ERROR")
		return
	}
	if data == nil {
		data = []core.ActionBottleneck{}
	}
	writeJSON(w, http.StatusOK, data)
}

func (h *Handler) getRunDurationStats(w http.ResponseWriter, r *http.Request) {
	filter := parseAnalyticsFilter(r)
	data, err := h.store.RunDurationStats(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "ANALYTICS_ERROR")
		return
	}
	if data == nil {
		data = []core.WorkItemDurationStat{}
	}
	writeJSON(w, http.StatusOK, data)
}

func (h *Handler) getErrorBreakdown(w http.ResponseWriter, r *http.Request) {
	filter := parseAnalyticsFilter(r)
	data, err := h.store.ErrorBreakdown(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "ANALYTICS_ERROR")
		return
	}
	if data == nil {
		data = []core.ErrorKindCount{}
	}
	writeJSON(w, http.StatusOK, data)
}

func (h *Handler) getRecentFailures(w http.ResponseWriter, r *http.Request) {
	filter := parseAnalyticsFilter(r)
	data, err := h.store.RecentFailures(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "ANALYTICS_ERROR")
		return
	}
	if data == nil {
		data = []core.FailureRecord{}
	}
	writeJSON(w, http.StatusOK, data)
}

func (h *Handler) getWorkItemStatusDistribution(w http.ResponseWriter, r *http.Request) {
	filter := parseAnalyticsFilter(r)
	data, err := h.store.WorkItemStatusDistribution(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "ANALYTICS_ERROR")
		return
	}
	if data == nil {
		data = []core.StatusCount{}
	}
	writeJSON(w, http.StatusOK, data)
}

// getAnalyticsSummary returns all analytics data in a single request for the dashboard.
func (h *Handler) getAnalyticsSummary(w http.ResponseWriter, r *http.Request) {
	filter := parseAnalyticsFilter(r)

	type summary struct {
		ProjectErrors  []core.ProjectErrorRank     `json:"project_errors"`
		Bottlenecks    []core.ActionBottleneck     `json:"bottlenecks"`
		DurationStats  []core.WorkItemDurationStat `json:"duration_stats"`
		ErrorBreakdown []core.ErrorKindCount       `json:"error_breakdown"`
		RecentFailures []core.FailureRecord        `json:"recent_failures"`
		StatusDist     []core.StatusCount          `json:"status_distribution"`
	}

	ctx := r.Context()
	s := summary{}
	var err error

	if s.ProjectErrors, err = h.store.ProjectErrorRanking(ctx, filter); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "ANALYTICS_ERROR")
		return
	}
	if s.Bottlenecks, err = h.store.WorkItemBottleneckActions(ctx, filter); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "ANALYTICS_ERROR")
		return
	}
	if s.DurationStats, err = h.store.RunDurationStats(ctx, filter); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "ANALYTICS_ERROR")
		return
	}
	if s.ErrorBreakdown, err = h.store.ErrorBreakdown(ctx, filter); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "ANALYTICS_ERROR")
		return
	}
	if s.RecentFailures, err = h.store.RecentFailures(ctx, filter); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "ANALYTICS_ERROR")
		return
	}
	if s.StatusDist, err = h.store.WorkItemStatusDistribution(ctx, filter); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "ANALYTICS_ERROR")
		return
	}

	// Ensure no nil slices in JSON output.
	if s.ProjectErrors == nil {
		s.ProjectErrors = []core.ProjectErrorRank{}
	}
	if s.Bottlenecks == nil {
		s.Bottlenecks = []core.ActionBottleneck{}
	}
	if s.DurationStats == nil {
		s.DurationStats = []core.WorkItemDurationStat{}
	}
	if s.ErrorBreakdown == nil {
		s.ErrorBreakdown = []core.ErrorKindCount{}
	}
	if s.RecentFailures == nil {
		s.RecentFailures = []core.FailureRecord{}
	}
	if s.StatusDist == nil {
		s.StatusDist = []core.StatusCount{}
	}

	writeJSON(w, http.StatusOK, s)
}

func parseAnalyticsFilter(r *http.Request) core.AnalyticsFilter {
	f := core.AnalyticsFilter{}
	if pid, ok := queryInt64(r, "project_id"); ok {
		f.ProjectID = &pid
	}
	if s := r.URL.Query().Get("since"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			f.Since = &t
		}
	}
	if s := r.URL.Query().Get("until"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			f.Until = &t
		}
	}
	f.Limit = queryInt(r, "limit", 0)
	return f
}
