package api

import (
	"context"
	"net/http"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

type statsResponse struct {
	TotalIssues  int     `json:"total_issues"`
	ActiveIssues int     `json:"active_issues"`
	SuccessRate  float64 `json:"success_rate"`
	AvgDuration  string  `json:"avg_duration"`
}

func (h *Handler) getStats(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}

	issues, err := listAllIssues(r.Context(), h.store)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	total := len(issues)
	active := 0
	finished := 0
	success := 0
	var totalDuration time.Duration
	var durationCount int

	for _, iss := range issues {
		if iss == nil {
			continue
		}
		switch iss.Status {
		case core.IssueOpen, core.IssueQueued, core.IssueRunning, core.IssueBlocked:
			active++
		case core.IssueDone, core.IssueFailed, core.IssueCancelled:
			finished++
			if iss.Status == core.IssueDone {
				success++
			}
			if !iss.CreatedAt.IsZero() && !iss.UpdatedAt.IsZero() && iss.UpdatedAt.After(iss.CreatedAt) {
				totalDuration += iss.UpdatedAt.Sub(iss.CreatedAt)
				durationCount++
			}
		default:
			// unknown status: ignore
		}
	}

	successRate := 0.0
	if finished > 0 {
		successRate = float64(success) / float64(finished)
	}
	avgDuration := time.Duration(0)
	if durationCount > 0 {
		avgDuration = time.Duration(int64(totalDuration) / int64(durationCount))
	}

	writeJSON(w, http.StatusOK, statsResponse{
		TotalIssues:  total,
		ActiveIssues: active,
		SuccessRate:  successRate,
		AvgDuration:  avgDuration.String(),
	})
}

func (h *Handler) getSchedulerStats(w http.ResponseWriter, r *http.Request) {
	if h.scheduler == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled": false,
			"message": "scheduler is not configured",
		})
		return
	}
	stats := h.scheduler.Stats()
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": true,
		"stats":   stats,
	})
}

func listAllIssues(ctx context.Context, store core.IssueStore) ([]*core.Issue, error) {
	const pageSize = 500
	offset := 0
	var out []*core.Issue

	for {
		page, err := store.ListIssues(ctx, core.IssueFilter{
			Limit:  pageSize,
			Offset: offset,
		})
		if err != nil {
			return nil, err
		}
		if page == nil {
			page = []*core.Issue{}
		}
		out = append(out, page...)
		if len(page) < pageSize {
			return out, nil
		}
		offset += pageSize
	}
}
