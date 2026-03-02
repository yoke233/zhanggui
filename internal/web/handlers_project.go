package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/user/ai-workflow/internal/core"
)

type projectHandlers struct {
	store           core.Store
	hub             *Hub
	repoProvisioner ProjectRepoProvisioner
	createRequests  *projectCreateRequestStore
}

type createProjectRequest struct {
	Name     string `json:"name"`
	RepoPath string `json:"repo_path"`
	GitHub   struct {
		Owner string `json:"owner"`
		Repo  string `json:"repo"`
	} `json:"github"`
}

type createProjectAsyncRequest struct {
	Name       string `json:"name,omitempty"`
	SourceType string `json:"source_type"`
	RepoPath   string `json:"repo_path,omitempty"`
	Slug       string `json:"slug,omitempty"`
	Owner      string `json:"owner,omitempty"`
	Repo       string `json:"repo,omitempty"`
	Ref        string `json:"ref,omitempty"`
	GitHub     struct {
		Owner string `json:"owner"`
		Repo  string `json:"repo"`
		Ref   string `json:"ref,omitempty"`
	} `json:"github,omitempty"`
}

type createProjectRequestAcceptedResponse struct {
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
}

type getProjectCreateRequestResponse struct {
	RequestID  string `json:"request_id"`
	SourceType string `json:"source_type"`
	Status     string `json:"status"`
	ProjectID  string `json:"project_id,omitempty"`
	RepoPath   string `json:"repo_path,omitempty"`
	Step       string `json:"step,omitempty"`
	Message    string `json:"message,omitempty"`
	Progress   int    `json:"progress"`
	Error      string `json:"error,omitempty"`
}

type projectCreateRequestStatus string

const (
	projectCreateRequestStatusPending   projectCreateRequestStatus = "pending"
	projectCreateRequestStatusRunning   projectCreateRequestStatus = "running"
	projectCreateRequestStatusSucceeded projectCreateRequestStatus = "succeeded"
	projectCreateRequestStatusFailed    projectCreateRequestStatus = "failed"
)

type projectCreateRequestState struct {
	RequestID  string
	SourceType string
	Status     projectCreateRequestStatus
	Name       string
	RepoPath   string
	Slug       string
	GitHub     struct {
		Owner string
		Repo  string
		Ref   string
	}
	ProjectID string
	Step      string
	Message   string
	Progress  int
	Error     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type projectCreateRequestStore struct {
	mu    sync.RWMutex
	items map[string]projectCreateRequestState
}

type apiError struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

func registerProjectRoutes(r chi.Router, store core.Store, hub *Hub, provisioner ProjectRepoProvisioner) {
	if provisioner == nil {
		provisioner = NewProjectRepoProvisioner("")
	}
	h := &projectHandlers{
		store:           store,
		hub:             hub,
		repoProvisioner: provisioner,
		createRequests:  newProjectCreateRequestStore(),
	}
	r.Get("/projects", h.listProjects)
	r.Post("/projects", h.createProject)
	r.Post("/projects/create-requests", h.createProjectRequestAsync)
	r.Get("/projects/create-requests/{id}", h.getProjectCreateRequest)
	r.Get("/projects/{id}", h.getProject)
}

func (h *projectHandlers) listProjects(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}

	filter := core.ProjectFilter{
		NameContains: strings.TrimSpace(r.URL.Query().Get("q")),
	}
	items, err := h.store.ListProjects(filter)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to list projects", "LIST_PROJECTS_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (h *projectHandlers) getProject(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}

	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		writeAPIError(w, http.StatusBadRequest, "project id is required", "PROJECT_ID_REQUIRED")
		return
	}

	project, err := h.store.GetProject(id)
	if err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("project %s not found", id), "PROJECT_NOT_FOUND")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load project", "GET_PROJECT_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, project)
}

func (h *projectHandlers) createProject(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}

	var req createProjectRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid json body", "INVALID_JSON")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.RepoPath = strings.TrimSpace(req.RepoPath)
	req.GitHub.Owner = strings.TrimSpace(req.GitHub.Owner)
	req.GitHub.Repo = strings.TrimSpace(req.GitHub.Repo)
	if req.Name == "" {
		writeAPIError(w, http.StatusBadRequest, "name is required", "NAME_REQUIRED")
		return
	}
	if req.RepoPath == "" {
		writeAPIError(w, http.StatusBadRequest, "repo_path is required", "REPO_PATH_REQUIRED")
		return
	}

	project := &core.Project{
		ID:          uuid.NewString(),
		Name:        req.Name,
		RepoPath:    req.RepoPath,
		GitHubOwner: req.GitHub.Owner,
		GitHubRepo:  req.GitHub.Repo,
	}

	if err := h.store.CreateProject(project); err != nil {
		if isConflictError(err) {
			writeAPIError(w, http.StatusConflict, "project already exists", "PROJECT_ALREADY_EXISTS")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to create project", "CREATE_PROJECT_FAILED")
		return
	}

	created, err := h.store.GetProject(project.ID)
	if err != nil && !isNotFoundError(err) {
		writeAPIError(w, http.StatusInternalServerError, "project created but reload failed", "PROJECT_RELOAD_FAILED")
		return
	}
	if created == nil {
		created = project
	}

	writeJSON(w, http.StatusCreated, created)
}

func (h *projectHandlers) createProjectRequestAsync(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}
	if h.repoProvisioner == nil || h.createRequests == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "project create request service is not configured", "PROJECT_CREATE_SERVICE_UNAVAILABLE")
		return
	}

	var req createProjectAsyncRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid json body", "INVALID_JSON")
		return
	}
	h.normalizeCreateProjectAsyncRequest(&req)
	if message, code := validateCreateProjectAsyncRequest(req); code != "" {
		writeAPIError(w, http.StatusBadRequest, message, code)
		return
	}

	now := time.Now().UTC()
	state := projectCreateRequestState{
		RequestID:  uuid.NewString(),
		SourceType: req.SourceType,
		Status:     projectCreateRequestStatusPending,
		Name:       req.Name,
		RepoPath:   req.RepoPath,
		Slug:       req.Slug,
		Step:       "queued",
		Message:    "project create request accepted",
		Progress:   0,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	state.GitHub.Owner = req.GitHub.Owner
	state.GitHub.Repo = req.GitHub.Repo
	state.GitHub.Ref = req.GitHub.Ref
	h.createRequests.create(state)

	writeJSON(w, http.StatusAccepted, createProjectRequestAcceptedResponse{
		RequestID: state.RequestID,
		Status:    string(state.Status),
	})

	go h.processProjectCreateRequest(state.RequestID, req)
}

func (h *projectHandlers) getProjectCreateRequest(w http.ResponseWriter, r *http.Request) {
	if h.createRequests == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "project create request service is not configured", "PROJECT_CREATE_SERVICE_UNAVAILABLE")
		return
	}

	requestID := strings.TrimSpace(chi.URLParam(r, "id"))
	if requestID == "" {
		writeAPIError(w, http.StatusBadRequest, "request id is required", "REQUEST_ID_REQUIRED")
		return
	}

	state, ok := h.createRequests.get(requestID)
	if !ok {
		writeAPIError(w, http.StatusNotFound, fmt.Sprintf("project create request %s not found", requestID), "CREATE_REQUEST_NOT_FOUND")
		return
	}
	writeJSON(w, http.StatusOK, buildProjectCreateRequestResponse(state))
}

func (h *projectHandlers) normalizeCreateProjectAsyncRequest(req *createProjectAsyncRequest) {
	req.Name = strings.TrimSpace(req.Name)
	req.SourceType = strings.TrimSpace(req.SourceType)
	req.RepoPath = strings.TrimSpace(req.RepoPath)
	req.Slug = normalizeProjectSlug(req.Slug)
	req.Owner = strings.TrimSpace(req.Owner)
	req.Repo = strings.TrimSpace(req.Repo)
	req.Ref = strings.TrimSpace(req.Ref)
	req.GitHub.Owner = strings.TrimSpace(req.GitHub.Owner)
	req.GitHub.Repo = strings.TrimSpace(req.GitHub.Repo)
	req.GitHub.Ref = strings.TrimSpace(req.GitHub.Ref)
	if req.GitHub.Owner == "" {
		req.GitHub.Owner = req.Owner
	}
	if req.GitHub.Repo == "" {
		req.GitHub.Repo = req.Repo
	}
	if req.GitHub.Ref == "" {
		req.GitHub.Ref = req.Ref
	}
	if req.Owner == "" {
		req.Owner = req.GitHub.Owner
	}
	if req.Repo == "" {
		req.Repo = req.GitHub.Repo
	}
	if req.Ref == "" {
		req.Ref = req.GitHub.Ref
	}
	if req.SourceType == projectSourceTypeLocalNew && req.Slug == "" && req.Name != "" {
		req.Slug = normalizeProjectSlug(req.Name)
	}
}

func validateCreateProjectAsyncRequest(req createProjectAsyncRequest) (message, code string) {
	sourceType := strings.TrimSpace(req.SourceType)
	if sourceType == "" {
		return "source_type is required", "SOURCE_TYPE_REQUIRED"
	}
	switch sourceType {
	case projectSourceTypeLocalPath:
		if strings.TrimSpace(req.RepoPath) == "" {
			return "repo_path is required", "REPO_PATH_REQUIRED"
		}
	case projectSourceTypeLocalNew:
		if strings.TrimSpace(req.Slug) == "" {
			return "slug is required for local_new", "SLUG_REQUIRED"
		}
	case projectSourceTypeGitHubClone:
		if strings.TrimSpace(req.GitHub.Owner) == "" {
			return "github.owner is required", "GITHUB_OWNER_REQUIRED"
		}
		if strings.TrimSpace(req.GitHub.Repo) == "" {
			return "github.repo is required", "GITHUB_REPO_REQUIRED"
		}
	default:
		return fmt.Sprintf("unsupported source_type: %s", sourceType), "INVALID_SOURCE_TYPE"
	}
	return "", ""
}

func (h *projectHandlers) processProjectCreateRequest(requestID string, req createProjectAsyncRequest) {
	runningState, ok := h.createRequests.update(requestID, func(state *projectCreateRequestState) {
		state.Status = projectCreateRequestStatusRunning
		state.Step = "start"
		state.Message = "project create request started"
		state.Progress = estimateCreateProgress(state.Step, state.Status)
		state.Error = ""
		state.UpdatedAt = time.Now().UTC()
	})
	if !ok {
		return
	}
	h.broadcastProjectCreateEvent("project_create_started", runningState, runningState.Step, runningState.Message)

	input := ProjectRepoProvisionInput{
		SourceType:  req.SourceType,
		RepoPath:    req.RepoPath,
		Slug:        req.Slug,
		GitHubOwner: req.GitHub.Owner,
		GitHubRepo:  req.GitHub.Repo,
		GitHubRef:   req.GitHub.Ref,
		Progress: func(step, message string) {
			currentState, ok := h.createRequests.update(requestID, func(state *projectCreateRequestState) {
				state.Step = strings.TrimSpace(step)
				state.Message = strings.TrimSpace(message)
				state.Progress = estimateCreateProgress(state.Step, state.Status)
				state.UpdatedAt = time.Now().UTC()
			})
			if !ok {
				return
			}
			h.broadcastProjectCreateEvent("project_create_progress", currentState, currentState.Step, currentState.Message)
		},
	}
	result, err := h.repoProvisioner.Provision(context.Background(), input)
	if err != nil {
		h.failProjectCreateRequest(requestID, "provision_repo", "failed to prepare repository", err)
		return
	}

	repoPath := strings.TrimSpace(result.RepoPath)
	if repoPath == "" {
		h.failProjectCreateRequest(requestID, "provision_repo", "repository path is empty", errors.New("empty repository path"))
		return
	}

	updatedState, ok := h.createRequests.update(requestID, func(state *projectCreateRequestState) {
		state.RepoPath = repoPath
		if strings.TrimSpace(result.GitHubOwner) != "" {
			state.GitHub.Owner = strings.TrimSpace(result.GitHubOwner)
		}
		if strings.TrimSpace(result.GitHubRepo) != "" {
			state.GitHub.Repo = strings.TrimSpace(result.GitHubRepo)
		}
		state.UpdatedAt = time.Now().UTC()
	})
	if !ok {
		return
	}
	updatedState, ok = h.createRequests.update(requestID, func(state *projectCreateRequestState) {
		state.Step = "repository_ready"
		state.Message = "repository is ready"
		state.Progress = estimateCreateProgress(state.Step, state.Status)
		state.UpdatedAt = time.Now().UTC()
	})
	if !ok {
		return
	}
	h.broadcastProjectCreateEvent("project_create_progress", updatedState, updatedState.Step, updatedState.Message)

	projectName := resolveCreateProjectName(req, updatedState.RepoPath)
	if projectName == "" {
		h.failProjectCreateRequest(requestID, "resolve_project_name", "project name is empty", errors.New("empty project name"))
		return
	}

	updatedState, ok = h.createRequests.update(requestID, func(state *projectCreateRequestState) {
		state.Step = "create_project_record"
		state.Message = "creating project record"
		state.Progress = estimateCreateProgress(state.Step, state.Status)
		state.UpdatedAt = time.Now().UTC()
	})
	if !ok {
		return
	}
	h.broadcastProjectCreateEvent("project_create_progress", updatedState, updatedState.Step, updatedState.Message)
	project := &core.Project{
		ID:          uuid.NewString(),
		Name:        projectName,
		RepoPath:    updatedState.RepoPath,
		GitHubOwner: updatedState.GitHub.Owner,
		GitHubRepo:  updatedState.GitHub.Repo,
	}
	if err := h.store.CreateProject(project); err != nil {
		if isConflictError(err) {
			h.failProjectCreateRequest(requestID, "create_project_record", "project already exists", err)
			return
		}
		h.failProjectCreateRequest(requestID, "create_project_record", "failed to create project record", err)
		return
	}

	succeededState, ok := h.createRequests.update(requestID, func(state *projectCreateRequestState) {
		state.Status = projectCreateRequestStatusSucceeded
		state.ProjectID = project.ID
		state.RepoPath = project.RepoPath
		state.Step = "complete"
		state.Message = "project created"
		state.Progress = estimateCreateProgress(state.Step, state.Status)
		state.Error = ""
		state.UpdatedAt = time.Now().UTC()
	})
	if !ok {
		return
	}
	h.broadcastProjectCreateEvent("project_create_succeeded", succeededState, succeededState.Step, succeededState.Message)
}

func resolveCreateProjectName(req createProjectAsyncRequest, repoPath string) string {
	if strings.TrimSpace(req.Name) != "" {
		return strings.TrimSpace(req.Name)
	}
	switch strings.TrimSpace(req.SourceType) {
	case projectSourceTypeLocalNew:
		return strings.TrimSpace(req.Slug)
	case projectSourceTypeGitHubClone:
		return strings.TrimSpace(req.GitHub.Repo)
	case projectSourceTypeLocalPath:
		base := strings.TrimSpace(filepath.Base(strings.TrimSpace(repoPath)))
		if base == "." || base == string(filepath.Separator) {
			return ""
		}
		return base
	default:
		return ""
	}
}

func (h *projectHandlers) failProjectCreateRequest(requestID, step, message string, cause error) {
	state, ok := h.createRequests.update(requestID, func(s *projectCreateRequestState) {
		s.Status = projectCreateRequestStatusFailed
		s.ProjectID = ""
		s.Step = strings.TrimSpace(step)
		s.Message = strings.TrimSpace(message)
		s.Progress = estimateCreateProgress(s.Step, s.Status)
		s.Error = strings.TrimSpace(cause.Error())
		s.UpdatedAt = time.Now().UTC()
	})
	if !ok {
		return
	}
	h.broadcastProjectCreateEvent("project_create_failed", state, state.Step, state.Message)
}

func (h *projectHandlers) broadcastProjectCreateEvent(eventType string, state projectCreateRequestState, step, message string) {
	if h.hub == nil {
		return
	}
	step = strings.TrimSpace(step)
	if step == "" {
		step = "update"
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = eventType
	}

	data := map[string]any{
		"request_id":  state.RequestID,
		"source_type": state.SourceType,
		"status":      string(state.Status),
		"step":        step,
		"message":     message,
	}
	if strings.TrimSpace(state.ProjectID) != "" {
		data["project_id"] = state.ProjectID
	}
	if strings.TrimSpace(state.RepoPath) != "" {
		data["repo_path"] = state.RepoPath
	}
	if strings.TrimSpace(state.Error) != "" {
		data["error"] = state.Error
	}
	data["progress"] = state.Progress
	if strings.TrimSpace(state.GitHub.Owner) != "" {
		data["github_owner"] = state.GitHub.Owner
	}
	if strings.TrimSpace(state.GitHub.Repo) != "" {
		data["github_repo"] = state.GitHub.Repo
	}

	h.hub.Broadcast(WSMessage{
		Type:      eventType,
		ProjectID: state.ProjectID,
		Data:      data,
	})
}

func newProjectCreateRequestStore() *projectCreateRequestStore {
	return &projectCreateRequestStore{
		items: make(map[string]projectCreateRequestState),
	}
}

func (s *projectCreateRequestStore) create(state projectCreateRequestState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[state.RequestID] = state
}

func (s *projectCreateRequestStore) get(requestID string) (projectCreateRequestState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.items[requestID]
	return state, ok
}

func (s *projectCreateRequestStore) update(
	requestID string,
	mutator func(state *projectCreateRequestState),
) (projectCreateRequestState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.items[requestID]
	if !ok {
		return projectCreateRequestState{}, false
	}
	if mutator != nil {
		mutator(&state)
	}
	s.items[requestID] = state
	return state, true
}

func buildProjectCreateRequestResponse(state projectCreateRequestState) getProjectCreateRequestResponse {
	return getProjectCreateRequestResponse{
		RequestID:  state.RequestID,
		SourceType: state.SourceType,
		Status:     string(state.Status),
		ProjectID:  state.ProjectID,
		RepoPath:   state.RepoPath,
		Step:       state.Step,
		Message:    state.Message,
		Progress:   state.Progress,
		Error:      state.Error,
	}
}

func estimateCreateProgress(step string, status projectCreateRequestStatus) int {
	switch status {
	case projectCreateRequestStatusSucceeded:
		return 100
	case projectCreateRequestStatusFailed:
		if progress, ok := createProgressByStep[strings.TrimSpace(step)]; ok {
			return progress
		}
		return 100
	default:
		if progress, ok := createProgressByStep[strings.TrimSpace(step)]; ok {
			return progress
		}
		return 0
	}
}

var createProgressByStep = map[string]int{
	"queued":                0,
	"start":                 5,
	"resolve_local_path":    15,
	"ensure_repo_root":      20,
	"create_directory":      35,
	"git_init":              45,
	"clone_repository":      35,
	"update_repository":     45,
	"checkout_ref":          60,
	"repository_ready":      75,
	"create_project_record": 90,
	"complete":              100,
}

func writeAPIError(w http.ResponseWriter, statusCode int, message, code string) {
	writeJSON(w, statusCode, apiError{
		Error: message,
		Code:  code,
	})
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	var target interface{ NotFound() bool }
	if errors.As(err, &target) {
		return target.NotFound()
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}

func isConflictError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "constraint")
}
