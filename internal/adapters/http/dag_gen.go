package api

import (
	"encoding/json"
	"net/http"

	"github.com/yoke233/ai-workflow/internal/core"
)

type generateStepsRequest struct {
	Description string `json:"description"`
}

// generateActions uses AI to decompose a task description into a DAG of Steps
// and creates them in the given issue.
// POST /work-items/{issueID}/generate-steps
func (h *Handler) generateActions(w http.ResponseWriter, r *http.Request) {
	if h.dagGen == nil {
		writeError(w, http.StatusServiceUnavailable, "DAG generator is not configured (requires LLM)", "DAG_GEN_UNAVAILABLE")
		return
	}

	issueID, ok := urlParamInt64(r, "issueID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid issue ID", "BAD_ID")
		return
	}

	// Verify issue exists and is open.
	iss, err := h.store.GetWorkItem(r.Context(), issueID)
	if err == core.ErrNotFound {
		writeError(w, http.StatusNotFound, "issue not found", "NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if iss.Status != core.WorkItemOpen {
		writeError(w, http.StatusConflict, "issue is not open, cannot generate steps", "INVALID_STATE")
		return
	}

	var req generateStepsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	if req.Description == "" {
		writeError(w, http.StatusBadRequest, "description is required", "MISSING_DESCRIPTION")
		return
	}

	// Call LLM to generate DAG.
	dag, err := h.dagGen.Generate(r.Context(), req.Description)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "DAG_GEN_ERROR")
		return
	}

	// Materialize steps into the store.
	store, ok := any(h.store).(core.Store)
	if !ok {
		writeError(w, http.StatusInternalServerError, "handler store does not implement core.Store", "STORE_ERROR")
		return
	}
	steps, err := h.dagGen.Materialize(r.Context(), store, issueID, dag)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "MATERIALIZE_ERROR")
		return
	}

	writeJSON(w, http.StatusCreated, steps)
}
