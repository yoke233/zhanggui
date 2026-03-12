package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/yoke233/ai-workflow/internal/core"
)

// --- Manifest endpoints ---

func (h *Handler) createManifest(w http.ResponseWriter, r *http.Request) {
	projectID, ok := urlParamInt64(r, "projectID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid project_id", "bad_request")
		return
	}

	var body struct {
		Summary  string         `json:"summary"`
		Metadata map[string]any `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err.Error() != "EOF" {
		writeError(w, http.StatusBadRequest, "invalid JSON", "bad_request")
		return
	}

	m := &core.FeatureManifest{
		ProjectID: projectID,
		Summary:   body.Summary,
		Metadata:  body.Metadata,
	}
	_, err := h.store.CreateFeatureManifest(r.Context(), m)
	if err != nil {
		if errors.Is(err, core.ErrManifestAlreadyExists) {
			writeError(w, http.StatusConflict, "manifest already exists for this project", "conflict")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "internal")
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (h *Handler) getManifest(w http.ResponseWriter, r *http.Request) {
	projectID, ok := urlParamInt64(r, "projectID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid project_id", "bad_request")
		return
	}

	m, err := h.store.GetFeatureManifestByProject(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			writeError(w, http.StatusNotFound, "manifest not found", "not_found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "internal")
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *Handler) updateManifest(w http.ResponseWriter, r *http.Request) {
	projectID, ok := urlParamInt64(r, "projectID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid project_id", "bad_request")
		return
	}

	m, err := h.store.GetFeatureManifestByProject(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			writeError(w, http.StatusNotFound, "manifest not found", "not_found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "internal")
		return
	}

	var body struct {
		Summary  *string        `json:"summary"`
		Metadata map[string]any `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON", "bad_request")
		return
	}

	if body.Summary != nil {
		m.Summary = *body.Summary
	}
	if body.Metadata != nil {
		m.Metadata = body.Metadata
	}
	// Version is atomically incremented by the store layer.
	if err := h.store.UpdateFeatureManifest(r.Context(), m); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "internal")
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *Handler) deleteManifest(w http.ResponseWriter, r *http.Request) {
	projectID, ok := urlParamInt64(r, "projectID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid project_id", "bad_request")
		return
	}

	m, err := h.store.GetFeatureManifestByProject(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			writeError(w, http.StatusNotFound, "manifest not found", "not_found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "internal")
		return
	}

	if err := h.store.DeleteFeatureManifest(r.Context(), m.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "internal")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Entry endpoints ---

func (h *Handler) createManifestEntry(w http.ResponseWriter, r *http.Request) {
	projectID, ok := urlParamInt64(r, "projectID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid project_id", "bad_request")
		return
	}

	m, err := h.store.GetFeatureManifestByProject(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			writeError(w, http.StatusNotFound, "manifest not found", "not_found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "internal")
		return
	}

	var body struct {
		Key         string         `json:"key"`
		Description string         `json:"description"`
		Status      string         `json:"status"`
		IssueID     *int64         `json:"issue_id"`
		StepID      *int64         `json:"step_id"`
		Tags        []string       `json:"tags"`
		Metadata    map[string]any `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON", "bad_request")
		return
	}
	if strings.TrimSpace(body.Key) == "" {
		writeError(w, http.StatusBadRequest, "key is required", "bad_request")
		return
	}

	entry := &core.FeatureEntry{
		ManifestID:  m.ID,
		Key:         body.Key,
		Description: body.Description,
		IssueID:     body.IssueID,
		StepID:      body.StepID,
		Tags:        body.Tags,
		Metadata:    body.Metadata,
	}
	if body.Status != "" {
		entry.Status = core.FeatureStatus(body.Status)
	}

	_, err = h.store.CreateFeatureEntry(r.Context(), entry)
	if err != nil {
		if errors.Is(err, core.ErrDuplicateEntryKey) {
			writeError(w, http.StatusConflict, "duplicate entry key", "conflict")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "internal")
		return
	}

	// Bump manifest version (atomic increment in store layer).
	if err := h.store.UpdateFeatureManifest(r.Context(), m); err != nil {
		slog.Warn("feature manifest: version bump failed after entry create", "manifest_id", m.ID, "error", err)
	}

	writeJSON(w, http.StatusCreated, entry)
}

func (h *Handler) listManifestEntries(w http.ResponseWriter, r *http.Request) {
	projectID, ok := urlParamInt64(r, "projectID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid project_id", "bad_request")
		return
	}

	m, err := h.store.GetFeatureManifestByProject(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			writeError(w, http.StatusNotFound, "manifest not found", "not_found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "internal")
		return
	}

	filter := core.FeatureEntryFilter{
		ManifestID: m.ID,
		Limit:      queryInt(r, "limit", 200),
		Offset:     queryInt(r, "offset", 0),
	}
	if s := r.URL.Query().Get("status"); s != "" {
		st := core.FeatureStatus(s)
		filter.Status = &st
	}
	if issueID, ok := queryInt64(r, "issue_id"); ok {
		filter.IssueID = &issueID
	}

	entries, err := h.store.ListFeatureEntries(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "internal")
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (h *Handler) getManifestEntry(w http.ResponseWriter, r *http.Request) {
	entryID, ok := urlParamInt64(r, "entryID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid entry_id", "bad_request")
		return
	}
	entry, err := h.store.GetFeatureEntry(r.Context(), entryID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			writeError(w, http.StatusNotFound, "entry not found", "not_found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "internal")
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

func (h *Handler) updateManifestEntry(w http.ResponseWriter, r *http.Request) {
	entryID, ok := urlParamInt64(r, "entryID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid entry_id", "bad_request")
		return
	}

	entry, err := h.store.GetFeatureEntry(r.Context(), entryID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			writeError(w, http.StatusNotFound, "entry not found", "not_found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "internal")
		return
	}

	var body struct {
		Key         *string        `json:"key"`
		Description *string        `json:"description"`
		Status      *string        `json:"status"`
		IssueID     *int64         `json:"issue_id"`
		StepID      *int64         `json:"step_id"`
		Tags        []string       `json:"tags"`
		Metadata    map[string]any `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON", "bad_request")
		return
	}
	if body.Key != nil {
		entry.Key = *body.Key
	}
	if body.Description != nil {
		entry.Description = *body.Description
	}
	if body.Status != nil {
		entry.Status = core.FeatureStatus(*body.Status)
	}
	if body.IssueID != nil {
		entry.IssueID = body.IssueID
	}
	if body.StepID != nil {
		entry.StepID = body.StepID
	}
	if body.Tags != nil {
		entry.Tags = body.Tags
	}
	if body.Metadata != nil {
		entry.Metadata = body.Metadata
	}

	if err := h.store.UpdateFeatureEntry(r.Context(), entry); err != nil {
		if errors.Is(err, core.ErrDuplicateEntryKey) {
			writeError(w, http.StatusConflict, "duplicate entry key", "conflict")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "internal")
		return
	}
	writeJSON(w, http.StatusOK, entry)
}

func (h *Handler) updateManifestEntryStatus(w http.ResponseWriter, r *http.Request) {
	entryID, ok := urlParamInt64(r, "entryID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid entry_id", "bad_request")
		return
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Status == "" {
		writeError(w, http.StatusBadRequest, "status is required", "bad_request")
		return
	}

	if err := h.store.UpdateFeatureEntryStatus(r.Context(), entryID, core.FeatureStatus(body.Status)); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			writeError(w, http.StatusNotFound, "entry not found", "not_found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "internal")
		return
	}

	entry, _ := h.store.GetFeatureEntry(r.Context(), entryID)
	writeJSON(w, http.StatusOK, entry)
}

func (h *Handler) deleteManifestEntry(w http.ResponseWriter, r *http.Request) {
	entryID, ok := urlParamInt64(r, "entryID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid entry_id", "bad_request")
		return
	}
	if err := h.store.DeleteFeatureEntry(r.Context(), entryID); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			writeError(w, http.StatusNotFound, "entry not found", "not_found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "internal")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Aggregation endpoints ---

func (h *Handler) getManifestSummary(w http.ResponseWriter, r *http.Request) {
	projectID, ok := urlParamInt64(r, "projectID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid project_id", "bad_request")
		return
	}

	m, err := h.store.GetFeatureManifestByProject(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			writeError(w, http.StatusNotFound, "manifest not found", "not_found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "internal")
		return
	}

	counts, err := h.store.CountFeatureEntriesByStatus(r.Context(), m.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "internal")
		return
	}

	total := 0
	for _, c := range counts {
		total += c
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"manifest_id": m.ID,
		"version":     m.Version,
		"pass":        counts[core.FeaturePass],
		"fail":        counts[core.FeatureFail],
		"pending":     counts[core.FeaturePending],
		"skipped":     counts[core.FeatureSkipped],
		"total":       total,
	})
}

func (h *Handler) getManifestSnapshot(w http.ResponseWriter, r *http.Request) {
	projectID, ok := urlParamInt64(r, "projectID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid project_id", "bad_request")
		return
	}

	m, err := h.store.GetFeatureManifestByProject(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			writeError(w, http.StatusNotFound, "manifest not found", "not_found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "internal")
		return
	}

	entries, err := h.store.ListFeatureEntries(r.Context(), core.FeatureEntryFilter{
		ManifestID: m.ID,
		Limit:      500,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "internal")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"manifest": m,
		"entries":  entries,
	})
}
