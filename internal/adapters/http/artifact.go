package api

import (
	"net/http"

	"github.com/yoke233/ai-workflow/internal/core"
)

func (h *Handler) getDeliverable(w http.ResponseWriter, r *http.Request) {
	id, ok := urlParamInt64(r, "artifactID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid artifact ID", "BAD_ID")
		return
	}

	a, err := h.store.GetDeliverable(r.Context(), id)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "artifact not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (h *Handler) getLatestDeliverable(w http.ResponseWriter, r *http.Request) {
	stepID, ok := urlParamInt64(r, "stepID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid step ID", "BAD_ID")
		return
	}

	a, err := h.store.GetLatestDeliverableByAction(r.Context(), stepID)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "no artifact for this step", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (h *Handler) listDeliverablesByRun(w http.ResponseWriter, r *http.Request) {
	execID, ok := urlParamInt64(r, "execID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid execution ID", "BAD_ID")
		return
	}

	artifacts, err := h.store.ListDeliverablesByRun(r.Context(), execID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if artifacts == nil {
		artifacts = []*core.Deliverable{}
	}
	writeJSON(w, http.StatusOK, artifacts)
}

