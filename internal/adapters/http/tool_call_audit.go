package api

import (
	"errors"
	"net/http"

	"github.com/yoke233/ai-workflow/internal/core"
)

func (h *Handler) listToolCallAuditsByRun(w http.ResponseWriter, r *http.Request) {
	execID, ok := urlParamInt64(r, "execID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid execution ID", "BAD_ID")
		return
	}

	items, err := h.store.ListToolCallAuditsByRun(r.Context(), execID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if items == nil {
		items = []*core.ToolCallAudit{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *Handler) getToolCallAudit(w http.ResponseWriter, r *http.Request) {
	auditID, ok := urlParamInt64(r, "auditID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid audit ID", "BAD_ID")
		return
	}

	item, err := h.store.GetToolCallAudit(r.Context(), auditID)
	if errors.Is(err, core.ErrNotFound) {
		writeError(w, http.StatusNotFound, "tool call audit not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, item)
}
