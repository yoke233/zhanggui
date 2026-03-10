package api

import (
	"context"
	"net/http"
	"time"

	"github.com/yoke233/ai-workflow/internal/v2/core"
)

type statsResponse struct {
	TotalFlows  int     `json:"total_flows"`
	ActiveFlows int     `json:"active_flows"`
	SuccessRate float64 `json:"success_rate"`
	AvgDuration string  `json:"avg_duration"`
}

func (h *Handler) getStats(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}

	flows, err := listAllFlows(r.Context(), h.store)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	total := len(flows)
	active := 0
	finished := 0
	success := 0
	var totalDuration time.Duration
	var durationCount int

	for _, f := range flows {
		if f == nil {
			continue
		}
		switch f.Status {
		case core.FlowPending, core.FlowQueued, core.FlowRunning, core.FlowBlocked:
			active++
		case core.FlowDone, core.FlowFailed, core.FlowCancelled:
			finished++
			if f.Status == core.FlowDone {
				success++
			}
			if !f.CreatedAt.IsZero() && !f.UpdatedAt.IsZero() && f.UpdatedAt.After(f.CreatedAt) {
				totalDuration += f.UpdatedAt.Sub(f.CreatedAt)
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
		TotalFlows:  total,
		ActiveFlows: active,
		SuccessRate: successRate,
		AvgDuration: avgDuration.String(),
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

func listAllFlows(ctx context.Context, store core.Store) ([]*core.Flow, error) {
	const pageSize = 500
	offset := 0
	var out []*core.Flow

	for {
		page, err := store.ListFlows(ctx, core.FlowFilter{
			Limit:  pageSize,
			Offset: offset,
		})
		if err != nil {
			return nil, err
		}
		if page == nil {
			page = []*core.Flow{}
		}
		out = append(out, page...)
		if len(page) < pageSize {
			return out, nil
		}
		offset += pageSize
	}
}
