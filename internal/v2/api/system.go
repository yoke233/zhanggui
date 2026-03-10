package api

import (
	"net/http"

	v2sandbox "github.com/yoke233/ai-workflow/internal/v2/sandbox"
)

func (h *Handler) getSandboxSupport(w http.ResponseWriter, r *http.Request) {
	inspector := h.sandbox
	if inspector == nil {
		inspector = v2sandbox.NewDefaultSupportInspector(false, "")
	}
	writeJSON(w, http.StatusOK, inspector.Inspect(r.Context()))
}
