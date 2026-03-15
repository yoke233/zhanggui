package api

import (
	"net/http"

	"github.com/yoke233/ai-workflow/internal/core"
)

func runToDeliverableResponse(run *core.Run, assets []*core.Resource) map[string]any {
	return map[string]any{
		"id":              run.ID,
		"run_id":          run.ID,
		"action_id":       run.ActionID,
		"work_item_id":    run.WorkItemID,
		"result_markdown": run.ResultMarkdown,
		"metadata":        run.ResultMetadata,
		"assets":          assets,
		"created_at":      run.CreatedAt,
	}
}

func (h *Handler) getDeliverable(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "artifactID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid artifact ID", "BAD_ID")
		return
	}

	// Artifact IDs now map to Run IDs (result data is inline on the Run).
	run, err := h.store.GetRun(r.Context(), id)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "artifact not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	assets, err := h.store.ListResourcesByRun(r.Context(), run.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, runToDeliverableResponse(run, assets))
}

func (h *Handler) getLatestDeliverable(w http.ResponseWriter, r *http.Request) {
	stepID, ok := urlParamInt64(r, "stepID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid step ID", "BAD_ID")
		return
	}

	run, err := h.store.GetLatestRunWithResult(r.Context(), stepID)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "no artifact for this step", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	assets, err := h.store.ListResourcesByRun(r.Context(), run.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, runToDeliverableResponse(run, assets))
}

func (h *Handler) listDeliverablesByRun(w http.ResponseWriter, r *http.Request) {
	execID, ok := urlParamInt64(r, "execID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid execution ID", "BAD_ID")
		return
	}

	run, err := h.store.GetRun(r.Context(), execID)
	if err == core.ErrNotFound {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	// Return the run's inline result as a single-element array for backward compat.
	if run.ResultMarkdown == "" {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	assets, err := h.store.ListResourcesByRun(r.Context(), run.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, []map[string]any{runToDeliverableResponse(run, assets)})
}
