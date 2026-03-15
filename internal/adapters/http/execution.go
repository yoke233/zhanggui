package api

import (
	"net/http"

	"github.com/yoke233/ai-workflow/internal/core"
)

func (h *Handler) getRun(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "runID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid run ID", "BAD_ID")
		return
	}

	run, err := h.store.GetRun(r.Context(), id)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "run not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (h *Handler) listRuns(w http.ResponseWriter, r *http.Request) {
	stepID, ok := urlParamInt64(r, "stepID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid step ID", "BAD_ID")
		return
	}

	execs, err := h.store.ListRunsByAction(r.Context(), stepID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if execs == nil {
		execs = []*core.Run{}
	}
	writeJSON(w, http.StatusOK, execs)
}
