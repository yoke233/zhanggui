package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	workitemtrackapp "github.com/yoke233/ai-workflow/internal/application/workitemtrackapp"
	"github.com/yoke233/ai-workflow/internal/core"
)

type createWorkItemTrackRequest struct {
	Title     string         `json:"title"`
	Objective string         `json:"objective,omitempty"`
	CreatedBy string         `json:"created_by,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

type attachWorkItemTrackThreadRequest struct {
	ThreadID     int64  `json:"thread_id"`
	RelationType string `json:"relation_type,omitempty"`
}

type materializeWorkItemTrackRequest struct {
	ProjectID *int64 `json:"project_id,omitempty"`
}

type submitReviewWorkItemTrackRequest struct {
	LatestSummary string         `json:"latest_summary,omitempty"`
	PlannerOutput map[string]any `json:"planner_output_json,omitempty"`
}

type reviewDecisionWorkItemTrackRequest struct {
	LatestSummary string         `json:"latest_summary,omitempty"`
	ReviewOutput  map[string]any `json:"review_output_json,omitempty"`
}

func registerWorkItemTrackRoutes(r chi.Router, h *Handler) {
	r.Post("/threads/{threadID}/tracks", h.createWorkItemTrack)
	r.Get("/threads/{threadID}/tracks", h.listWorkItemTracksByThread)
	r.Get("/tracks/{trackID}", h.getWorkItemTrack)
	r.Post("/tracks/{trackID}/threads", h.attachWorkItemTrackThread)
	r.Post("/tracks/{trackID}/submit-review", h.submitWorkItemTrackReview)
	r.Post("/tracks/{trackID}/approve-review", h.approveWorkItemTrackReview)
	r.Post("/tracks/{trackID}/reject-review", h.rejectWorkItemTrackReview)
	r.Post("/tracks/{trackID}/pause", h.pauseWorkItemTrack)
	r.Post("/tracks/{trackID}/cancel", h.cancelWorkItemTrack)
	r.Post("/tracks/{trackID}/materialize", h.materializeWorkItemTrack)
	r.Post("/tracks/{trackID}/confirm-execution", h.confirmWorkItemTrackExecution)
}

func (h *Handler) createWorkItemTrack(w http.ResponseWriter, r *http.Request) {
	threadID, ok := urlParamInt64(r, "threadID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid thread ID", "BAD_ID")
		return
	}

	var req createWorkItemTrackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}

	track, err := h.workItemTrackService().StartTrack(r.Context(), workitemtrackapp.StartTrackInput{
		ThreadID:  threadID,
		Title:     req.Title,
		Objective: req.Objective,
		CreatedBy: req.CreatedBy,
		Metadata:  req.Metadata,
	})
	if err != nil {
		writeWorkItemTrackAppFailure(w, err, "CREATE_TRACK_FAILED")
		return
	}
	writeJSON(w, http.StatusCreated, track)
}

func (h *Handler) listWorkItemTracksByThread(w http.ResponseWriter, r *http.Request) {
	threadID, ok := urlParamInt64(r, "threadID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid thread ID", "BAD_ID")
		return
	}

	tracks, err := h.store.ListWorkItemTracksByThread(r.Context(), threadID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if tracks == nil {
		tracks = []*core.WorkItemTrack{}
	}
	writeJSON(w, http.StatusOK, tracks)
}

func (h *Handler) getWorkItemTrack(w http.ResponseWriter, r *http.Request) {
	trackID, ok := urlParamInt64(r, "trackID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid track ID", "BAD_ID")
		return
	}

	track, err := h.store.GetWorkItemTrack(r.Context(), trackID)
	if err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "track not found", workitemtrackapp.CodeTrackNotFound)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, track)
}

func (h *Handler) attachWorkItemTrackThread(w http.ResponseWriter, r *http.Request) {
	trackID, ok := urlParamInt64(r, "trackID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid track ID", "BAD_ID")
		return
	}

	var req attachWorkItemTrackThreadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}

	link, err := h.workItemTrackService().AttachThreadContext(r.Context(), workitemtrackapp.AttachThreadContextInput{
		TrackID:      trackID,
		ThreadID:     req.ThreadID,
		RelationType: strings.TrimSpace(req.RelationType),
	})
	if err != nil {
		writeWorkItemTrackAppFailure(w, err, "ATTACH_TRACK_THREAD_FAILED")
		return
	}
	writeJSON(w, http.StatusCreated, link)
}

func (h *Handler) materializeWorkItemTrack(w http.ResponseWriter, r *http.Request) {
	trackID, ok := urlParamInt64(r, "trackID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid track ID", "BAD_ID")
		return
	}

	var req materializeWorkItemTrackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}

	result, err := h.workItemTrackService().MaterializeWorkItem(r.Context(), workitemtrackapp.MaterializeWorkItemInput{
		TrackID:   trackID,
		ProjectID: req.ProjectID,
	})
	if err != nil {
		writeWorkItemTrackAppFailure(w, err, "MATERIALIZE_TRACK_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) submitWorkItemTrackReview(w http.ResponseWriter, r *http.Request) {
	trackID, ok := urlParamInt64(r, "trackID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid track ID", "BAD_ID")
		return
	}

	var req submitReviewWorkItemTrackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}

	track, err := h.workItemTrackService().SubmitForReview(r.Context(), workitemtrackapp.SubmitForReviewInput{
		TrackID:       trackID,
		LatestSummary: req.LatestSummary,
		PlannerOutput: req.PlannerOutput,
	})
	if err != nil {
		writeWorkItemTrackAppFailure(w, err, "SUBMIT_TRACK_REVIEW_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, track)
}

func (h *Handler) approveWorkItemTrackReview(w http.ResponseWriter, r *http.Request) {
	trackID, ok := urlParamInt64(r, "trackID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid track ID", "BAD_ID")
		return
	}

	var req reviewDecisionWorkItemTrackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}

	track, err := h.workItemTrackService().ApproveReview(r.Context(), workitemtrackapp.ApproveReviewInput{
		TrackID:       trackID,
		LatestSummary: req.LatestSummary,
		ReviewOutput:  req.ReviewOutput,
	})
	if err != nil {
		writeWorkItemTrackAppFailure(w, err, "APPROVE_TRACK_REVIEW_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, track)
}

func (h *Handler) rejectWorkItemTrackReview(w http.ResponseWriter, r *http.Request) {
	trackID, ok := urlParamInt64(r, "trackID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid track ID", "BAD_ID")
		return
	}

	var req reviewDecisionWorkItemTrackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}

	track, err := h.workItemTrackService().RejectReview(r.Context(), workitemtrackapp.RejectReviewInput{
		TrackID:       trackID,
		LatestSummary: req.LatestSummary,
		ReviewOutput:  req.ReviewOutput,
	})
	if err != nil {
		writeWorkItemTrackAppFailure(w, err, "REJECT_TRACK_REVIEW_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, track)
}

func (h *Handler) pauseWorkItemTrack(w http.ResponseWriter, r *http.Request) {
	trackID, ok := urlParamInt64(r, "trackID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid track ID", "BAD_ID")
		return
	}

	track, err := h.workItemTrackService().PauseTrack(r.Context(), workitemtrackapp.PauseTrackInput{TrackID: trackID})
	if err != nil {
		writeWorkItemTrackAppFailure(w, err, "PAUSE_TRACK_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, track)
}

func (h *Handler) cancelWorkItemTrack(w http.ResponseWriter, r *http.Request) {
	trackID, ok := urlParamInt64(r, "trackID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid track ID", "BAD_ID")
		return
	}

	track, err := h.workItemTrackService().CancelTrack(r.Context(), workitemtrackapp.CancelTrackInput{TrackID: trackID})
	if err != nil {
		writeWorkItemTrackAppFailure(w, err, "CANCEL_TRACK_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, track)
}

func (h *Handler) confirmWorkItemTrackExecution(w http.ResponseWriter, r *http.Request) {
	trackID, ok := urlParamInt64(r, "trackID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid track ID", "BAD_ID")
		return
	}

	var req materializeWorkItemTrackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}

	result, err := h.workItemTrackService().ConfirmExecution(r.Context(), workitemtrackapp.ConfirmExecutionInput{
		TrackID:   trackID,
		ProjectID: req.ProjectID,
	})
	if err != nil {
		writeWorkItemTrackAppFailure(w, err, "CONFIRM_TRACK_EXECUTION_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, result)
}
