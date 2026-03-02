package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/user/ai-workflow/internal/core"
	"github.com/user/ai-workflow/internal/engine"
)

type pipelineHandlers struct {
	store      core.Store
	executor   PipelineExecutor
	stageRoles map[core.StageID]string
}

type createPipelineRequest struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Template    string         `json:"template"`
	Config      map[string]any `json:"config"`
}

type pipelineListResponse struct {
	Items  []core.Pipeline `json:"items"`
	Total  int             `json:"total"`
	Offset int             `json:"offset"`
}

type pipelineActionRequest struct {
	Action  string `json:"action"`
	Stage   string `json:"stage,omitempty"`
	Message string `json:"message,omitempty"`
	Role    string `json:"role,omitempty"`
}

type pipelineActionResponse struct {
	Status       string `json:"status"`
	CurrentStage string `json:"current_stage,omitempty"`
}

func registerPipelineRoutes(r chi.Router, store core.Store, executor PipelineExecutor, stageRoleBindings map[string]string) {
	h := &pipelineHandlers{
		store:      store,
		executor:   executor,
		stageRoles: normalizeStageRoleBindings(stageRoleBindings),
	}
	r.Get("/pipelines/{id}", h.getPipelineByID)
	r.Get("/projects/{projectID}/pipelines", h.listPipelines)
	r.Post("/projects/{projectID}/pipelines", h.createPipeline)
	r.Get("/projects/{projectID}/pipelines/{id}", h.getPipelineByProject)
	r.Get("/projects/{projectID}/pipelines/{id}/checkpoints", h.getPipelineCheckpoints)
	r.Post("/projects/{projectID}/pipelines/{id}/action", h.applyPipelineAction)
}

func (h *pipelineHandlers) listPipelines(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}

	projectID := strings.TrimSpace(chi.URLParam(r, "projectID"))
	if projectID == "" {
		writeAPIError(w, http.StatusBadRequest, "project id is required", "PROJECT_ID_REQUIRED")
		return
	}

	if _, err := h.store.GetProject(projectID); err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("project %s not found", projectID), "PROJECT_NOT_FOUND")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load project", "GET_PROJECT_FAILED")
		return
	}

	limit, offset, err := parsePaginationParams(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error(), "INVALID_QUERY_PARAM")
		return
	}

	items, err := h.store.ListPipelines(projectID, core.PipelineFilter{
		Status: strings.TrimSpace(r.URL.Query().Get("status")),
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to list pipelines", "LIST_PIPELINES_FAILED")
		return
	}

	writeJSON(w, http.StatusOK, pipelineListResponse{
		Items:  normalizePipelinesForAPI(items),
		Total:  len(items),
		Offset: offset,
	})
}

func (h *pipelineHandlers) createPipeline(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}

	projectID := strings.TrimSpace(chi.URLParam(r, "projectID"))
	if projectID == "" {
		writeAPIError(w, http.StatusBadRequest, "project id is required", "PROJECT_ID_REQUIRED")
		return
	}
	if _, err := h.store.GetProject(projectID); err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("project %s not found", projectID), "PROJECT_NOT_FOUND")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load project", "GET_PROJECT_FAILED")
		return
	}

	var req createPipelineRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid json body", "INVALID_JSON")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	req.Template = strings.TrimSpace(req.Template)
	if req.Name == "" {
		writeAPIError(w, http.StatusBadRequest, "name is required", "NAME_REQUIRED")
		return
	}
	if req.Template == "" {
		req.Template = "standard"
	}
	stages, err := buildPipelineStages(req.Template, h.stageRoles)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error(), "INVALID_TEMPLATE")
		return
	}
	if req.Config == nil {
		req.Config = map[string]any{}
	}

	now := time.Now()
	pipeline := &core.Pipeline{
		ID:              engine.NewPipelineID(),
		ProjectID:       projectID,
		Name:            req.Name,
		Description:     req.Description,
		Template:        req.Template,
		Status:          core.StatusCreated,
		Stages:          stages,
		Artifacts:       map[string]string{},
		Config:          req.Config,
		MaxTotalRetries: 5,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := h.store.SavePipeline(pipeline); err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to create pipeline", "CREATE_PIPELINE_FAILED")
		return
	}

	created, err := h.store.GetPipeline(pipeline.ID)
	if err != nil && !isNotFoundError(err) {
		writeAPIError(w, http.StatusInternalServerError, "pipeline created but reload failed", "PIPELINE_RELOAD_FAILED")
		return
	}
	if created == nil {
		created = pipeline
	}

	normalized := normalizePipelineForAPI(*created)
	writeJSON(w, http.StatusCreated, normalized)
}

func (h *pipelineHandlers) getPipelineByProject(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}

	projectID := strings.TrimSpace(chi.URLParam(r, "projectID"))
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if projectID == "" || id == "" {
		writeAPIError(w, http.StatusBadRequest, "project id and pipeline id are required", "INVALID_PATH_PARAM")
		return
	}

	pipeline, err := h.store.GetPipeline(id)
	if err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("pipeline %s not found", id), "PIPELINE_NOT_FOUND")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load pipeline", "GET_PIPELINE_FAILED")
		return
	}
	if pipeline.ProjectID != projectID {
		writeAPIError(w, http.StatusNotFound, fmt.Sprintf("pipeline %s not found in project %s", id, projectID), "PIPELINE_NOT_FOUND")
		return
	}
	normalized := normalizePipelineForAPI(*pipeline)
	writeJSON(w, http.StatusOK, normalized)
}

func (h *pipelineHandlers) getPipelineCheckpoints(w http.ResponseWriter, r *http.Request) {
	pipeline, ok := h.loadPipelineForProject(w, r)
	if !ok {
		return
	}

	checkpoints, err := h.store.GetCheckpoints(pipeline.ID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load checkpoints", "GET_CHECKPOINTS_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, checkpoints)
}

func (h *pipelineHandlers) applyPipelineAction(w http.ResponseWriter, r *http.Request) {
	if h.executor == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "pipeline executor is not configured", "PIPELINE_EXECUTOR_UNAVAILABLE")
		return
	}

	pipeline, ok := h.loadPipelineForProject(w, r)
	if !ok {
		return
	}

	var req pipelineActionRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid json body", "INVALID_JSON")
		return
	}

	actionType, err := parsePipelineActionType(req.Action)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error(), "INVALID_ACTION")
		return
	}

	action := core.PipelineAction{
		PipelineID: pipeline.ID,
		Type:       actionType,
		Stage:      core.StageID(strings.TrimSpace(req.Stage)),
		Role:       strings.TrimSpace(req.Role),
		Message:    strings.TrimSpace(req.Message),
	}

	if err := h.executor.ApplyAction(r.Context(), action); err != nil {
		msg := strings.ToLower(err.Error())
		switch {
		case strings.Contains(msg, "unknown action"):
			writeAPIError(w, http.StatusBadRequest, err.Error(), "INVALID_ACTION")
		case strings.Contains(msg, "requires"):
			writeAPIError(w, http.StatusConflict, err.Error(), "PIPELINE_ACTION_CONFLICT")
		default:
			writeAPIError(w, http.StatusInternalServerError, "failed to apply pipeline action", "APPLY_PIPELINE_ACTION_FAILED")
		}
		return
	}

	updated, err := h.store.GetPipeline(pipeline.ID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "pipeline action applied but reload failed", "PIPELINE_RELOAD_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, pipelineActionResponse{
		Status:       string(updated.Status),
		CurrentStage: string(updated.CurrentStage),
	})
}

func (h *pipelineHandlers) getPipelineByID(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}

	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeAPIError(w, http.StatusBadRequest, "pipeline id is required", "PIPELINE_ID_REQUIRED")
		return
	}

	pipeline, err := h.store.GetPipeline(id)
	if err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("pipeline %s not found", id), "PIPELINE_NOT_FOUND")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load pipeline", "GET_PIPELINE_FAILED")
		return
	}
	normalized := normalizePipelineForAPI(*pipeline)
	writeJSON(w, http.StatusOK, normalized)
}

func (h *pipelineHandlers) loadPipelineForProject(w http.ResponseWriter, r *http.Request) (*core.Pipeline, bool) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return nil, false
	}

	projectID := strings.TrimSpace(chi.URLParam(r, "projectID"))
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if projectID == "" || id == "" {
		writeAPIError(w, http.StatusBadRequest, "project id and pipeline id are required", "INVALID_PATH_PARAM")
		return nil, false
	}

	pipeline, err := h.store.GetPipeline(id)
	if err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("pipeline %s not found", id), "PIPELINE_NOT_FOUND")
			return nil, false
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load pipeline", "GET_PIPELINE_FAILED")
		return nil, false
	}
	if pipeline.ProjectID != projectID {
		writeAPIError(w, http.StatusNotFound, fmt.Sprintf("pipeline %s not found in project %s", id, projectID), "PIPELINE_NOT_FOUND")
		return nil, false
	}
	return pipeline, true
}

func parsePipelineActionType(raw string) (core.HumanActionType, error) {
	switch core.HumanActionType(strings.ToLower(strings.TrimSpace(raw))) {
	case core.ActionApprove,
		core.ActionReject,
		core.ActionModify,
		core.ActionSkip,
		core.ActionRerun,
		core.ActionChangeRole,
		core.ActionAbort,
		core.ActionPause,
		core.ActionResume:
		return core.HumanActionType(strings.ToLower(strings.TrimSpace(raw))), nil
	default:
		return "", fmt.Errorf("unknown action type: %s", raw)
	}
}

func parsePaginationParams(r *http.Request) (int, int, error) {
	limit := 20
	offset := 0

	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed <= 0 {
			return 0, 0, fmt.Errorf("limit must be a positive integer")
		}
		limit = parsed
	}

	if rawOffset := strings.TrimSpace(r.URL.Query().Get("offset")); rawOffset != "" {
		parsed, err := strconv.Atoi(rawOffset)
		if err != nil || parsed < 0 {
			return 0, 0, fmt.Errorf("offset must be a non-negative integer")
		}
		offset = parsed
	}

	return limit, offset, nil
}

func normalizePipelinesForAPI(items []core.Pipeline) []core.Pipeline {
	if len(items) == 0 {
		return []core.Pipeline{}
	}
	out := make([]core.Pipeline, len(items))
	for i := range items {
		out[i] = normalizePipelineForAPI(items[i])
	}
	return out
}

func normalizePipelineForAPI(item core.Pipeline) core.Pipeline {
	item.TaskItemID = strings.TrimSpace(item.TaskItemID)
	return item
}

func buildPipelineStages(template string, stageRoles map[core.StageID]string) ([]core.StageConfig, error) {
	stageIDs, ok := engine.Templates[template]
	if !ok {
		return nil, fmt.Errorf("unknown template: %s", template)
	}

	stages := make([]core.StageConfig, len(stageIDs))
	for i, stageID := range stageIDs {
		stages[i] = defaultPipelineStageConfig(stageID)
		if role, ok := stageRoles[stageID]; ok {
			stages[i].Role = role
		}
	}
	return stages, nil
}

func normalizeStageRoleBindings(stageRoleBindings map[string]string) map[core.StageID]string {
	if len(stageRoleBindings) == 0 {
		return nil
	}
	normalized := make(map[core.StageID]string, len(stageRoleBindings))
	for rawStage, rawRole := range stageRoleBindings {
		stage := core.StageID(strings.TrimSpace(rawStage))
		role := strings.TrimSpace(rawRole)
		if stage == "" || role == "" {
			continue
		}
		normalized[stage] = role
	}
	return normalized
}

func defaultPipelineStageConfig(id core.StageID) core.StageConfig {
	cfg := core.StageConfig{
		Name:           id,
		PromptTemplate: string(id),
		Timeout:        30 * time.Minute,
		MaxRetries:     1,
		OnFailure:      core.OnFailureHuman,
	}

	switch id {
	case core.StageRequirements, core.StageCodeReview:
		cfg.Agent = "claude"
	case core.StageImplement, core.StageFixup:
		cfg.Agent = "codex"
	case core.StageE2ETest:
		cfg.Agent = "codex"
		cfg.Timeout = 15 * time.Minute
	case core.StageWorktreeSetup, core.StageMerge, core.StageCleanup:
		cfg.Timeout = 2 * time.Minute
	}
	return cfg
}
