package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type generateTitleRequest struct {
	Description string `json:"description"`
}

type generateTitleResponse struct {
	Title string `json:"title"`
}

// generateTitle uses the LLM to generate a concise title from a description.
// POST /issues/generate-title
func (h *Handler) generateTitle(w http.ResponseWriter, r *http.Request) {
	if h.textCompleter == nil {
		writeError(w, http.StatusServiceUnavailable, "text completer is not configured (requires LLM)", "LLM_UNAVAILABLE")
		return
	}

	var req generateTitleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	if strings.TrimSpace(req.Description) == "" {
		writeError(w, http.StatusBadRequest, "description is required", "MISSING_DESCRIPTION")
		return
	}

	prompt := fmt.Sprintf(`Generate a concise title (under 60 characters) for the following work item description. Return ONLY the title text, nothing else.

Description:
---
%s
---`, req.Description)

	title, err := h.textCompleter.CompleteText(r.Context(), prompt)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "LLM_ERROR")
		return
	}

	// Clean up: trim whitespace and quotes
	title = strings.TrimSpace(title)
	title = strings.Trim(title, "\"'`")
	title = strings.TrimSpace(title)

	writeJSON(w, http.StatusOK, generateTitleResponse{Title: title})
}
