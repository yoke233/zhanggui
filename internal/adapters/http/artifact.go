package api

import (
	"net/http"

	"github.com/yoke233/zhanggui/internal/core"
)

func runToDeliverableResponse(run *core.Run, assets []*core.Resource) map[string]any {
	resp := map[string]any{
		"id":              run.ID,
		"run_id":          run.ID,
		"action_id":       run.ActionID,
		"work_item_id":    run.WorkItemID,
		"result_markdown": run.ResultMarkdown,
		"metadata":        run.ResultMetadata,
		"assets":          assets,
		"created_at":      run.CreatedAt,
	}
	if deliverable := core.RunResultToDeliverable(run); deliverable != nil {
		resp["deliverable"] = map[string]any{
			"kind":          deliverable.Kind,
			"title":         deliverable.Title,
			"summary":       deliverable.Summary,
			"payload":       deliverable.Payload,
			"producer_type": deliverable.ProducerType,
			"producer_id":   deliverable.ProducerID,
			"status":        deliverable.Status,
			"created_at":    deliverable.CreatedAt,
		}
	}
	if artifact := core.NormalizeArtifactMetadata(run.ResultMetadata); artifact != nil {
		resp["artifact"] = artifact
	}
	return resp
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
	if !run.HasResult() {
		writeError(w, http.StatusNotFound, "artifact not found", "NOT_FOUND")
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
	actionID, ok := urlParamInt64(r, "actionID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid action ID", "BAD_ID")
		return
	}

	run, err := h.store.GetLatestRunWithResult(r.Context(), actionID)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "no artifact for this action", "NOT_FOUND")
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
	runID, ok := urlParamInt64(r, "runID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid run ID", "BAD_ID")
		return
	}

	run, err := h.store.GetRun(r.Context(), runID)
	if err == core.ErrNotFound {
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	// Return the run's inline result as a single-element array for backward compat.
	if !run.HasResult() {
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
