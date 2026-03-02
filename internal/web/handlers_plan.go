package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/user/ai-workflow/internal/core"
	"github.com/user/ai-workflow/internal/secretary"
)

const (
	defaultPlanParserRoleID      = "plan_parser"
	maxPlanSourceFileBytes       = 1 << 20 // 1MB
	maxPlanSourceFilesTotalBytes = 5 << 20 // 5MB
)

type planHandlers struct {
	store       core.Store
	planManager PlanManager
	planRoleID  string
}

type createPlanRequest struct {
	SessionID  string `json:"session_id"`
	Name       string `json:"name"`
	FailPolicy string `json:"fail_policy"`
}

type createPlanFromFilesRequest struct {
	SessionID  string   `json:"session_id"`
	Name       string   `json:"name"`
	FailPolicy string   `json:"fail_policy"`
	FilePaths  []string `json:"file_paths"`
}

type planListResponse struct {
	Items  []core.TaskPlan `json:"items"`
	Total  int             `json:"total"`
	Offset int             `json:"offset"`
}

type taskPlanStatusResponse struct {
	Status string `json:"status"`
}

type planDAGNode struct {
	ID         string              `json:"id"`
	Title      string              `json:"title"`
	Status     core.TaskItemStatus `json:"status"`
	PipelineID string              `json:"pipeline_id"`
}

type planDAGEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type planDAGStats struct {
	Total   int `json:"total"`
	Pending int `json:"pending"`
	Ready   int `json:"ready"`
	Running int `json:"running"`
	Done    int `json:"done"`
	Failed  int `json:"failed"`
}

type planDAGResponse struct {
	Nodes []planDAGNode `json:"nodes"`
	Edges []planDAGEdge `json:"edges"`
	Stats planDAGStats  `json:"stats"`
}

type planActionRequest struct {
	Action   string              `json:"action"`
	Feedback *planActionFeedback `json:"feedback,omitempty"`
}

type planActionFeedback struct {
	Category          string `json:"category"`
	Detail            string `json:"detail"`
	ExpectedDirection string `json:"expected_direction,omitempty"`
}

func registerPlanRoutes(r chi.Router, store core.Store, planManager PlanManager, planParserRoleID string) {
	h := &planHandlers{
		store:       store,
		planManager: planManager,
		planRoleID:  resolvePlanParserRoleID(planParserRoleID),
	}
	r.Post("/projects/{projectID}/plans", h.createPlan)
	r.Post("/projects/{projectID}/plans/from-files", h.createPlanFromFiles)
	r.Get("/projects/{projectID}/plans", h.listPlans)
	r.Get("/projects/{projectID}/plans/{id}", h.getPlan)
	r.Get("/projects/{projectID}/plans/{id}/dag", h.getPlanDAG)
	r.Post("/projects/{projectID}/plans/{id}/review", h.submitReview)
	r.Post("/projects/{projectID}/plans/{id}/action", h.applyAction)
}

func (h *planHandlers) createPlan(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}

	projectID := strings.TrimSpace(chi.URLParam(r, "projectID"))
	if projectID == "" {
		writeAPIError(w, http.StatusBadRequest, "project id is required", "PROJECT_ID_REQUIRED")
		return
	}
	project, err := h.store.GetProject(projectID)
	if err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("project %s not found", projectID), "PROJECT_NOT_FOUND")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load project", "GET_PROJECT_FAILED")
		return
	}

	var req createPlanRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid json body", "INVALID_JSON")
		return
	}

	req.SessionID = strings.TrimSpace(req.SessionID)
	req.Name = strings.TrimSpace(req.Name)
	req.FailPolicy = strings.ToLower(strings.TrimSpace(req.FailPolicy))
	if req.SessionID == "" {
		writeAPIError(w, http.StatusBadRequest, "session_id is required", "SESSION_ID_REQUIRED")
		return
	}

	session, err := h.store.GetChatSession(req.SessionID)
	if err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("chat session %s not found", req.SessionID), "CHAT_SESSION_NOT_FOUND")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load chat session", "GET_CHAT_SESSION_FAILED")
		return
	}
	if session.ProjectID != projectID {
		writeAPIError(w, http.StatusNotFound, fmt.Sprintf("chat session %s not found in project %s", req.SessionID, projectID), "CHAT_SESSION_NOT_FOUND")
		return
	}

	failPolicy, err := parseFailPolicy(req.FailPolicy)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error(), "INVALID_FAIL_POLICY")
		return
	}

	if h.planManager == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "plan manager is not configured", "PLAN_MANAGER_UNAVAILABLE")
		return
	}

	createReq := secretary.Request{
		Conversation: summarizeChatMessages(session.Messages),
		ProjectName:  strings.TrimSpace(project.Name),
		RepoPath:     strings.TrimSpace(project.RepoPath),
		Role:         h.planRoleID,
		WorkDir:      strings.TrimSpace(project.RepoPath),
	}
	if createReq.WorkDir == "" {
		createReq.WorkDir = "."
	}
	created, err := h.planManager.CreateDraft(r.Context(), secretary.CreateDraftInput{
		ProjectID:  projectID,
		SessionID:  req.SessionID,
		Name:       req.Name,
		FailPolicy: failPolicy,
		Request:    createReq,
	})
	if err != nil {
		log.Printf("[web][plan] create draft failed project=%s session=%s err=%v", projectID, req.SessionID, err)
		writeAPIError(w, http.StatusInternalServerError, "failed to create task plan", "CREATE_TASK_PLAN_FAILED")
		return
	}
	writeJSON(w, http.StatusCreated, normalizeTaskPlanForAPI(created))
}

func (h *planHandlers) createPlanFromFiles(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}
	if h.planManager == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "plan manager is not configured", "PLAN_MANAGER_UNAVAILABLE")
		return
	}

	projectID := strings.TrimSpace(chi.URLParam(r, "projectID"))
	if projectID == "" {
		writeAPIError(w, http.StatusBadRequest, "project id is required", "PROJECT_ID_REQUIRED")
		return
	}

	var req createPlanFromFilesRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid json body", "INVALID_JSON")
		return
	}

	req.SessionID = strings.TrimSpace(req.SessionID)
	req.Name = strings.TrimSpace(req.Name)
	req.FailPolicy = strings.ToLower(strings.TrimSpace(req.FailPolicy))
	if req.SessionID == "" {
		writeAPIError(w, http.StatusBadRequest, "session_id is required", "SESSION_ID_REQUIRED")
		return
	}
	if len(req.FilePaths) == 0 {
		writeAPIError(w, http.StatusBadRequest, "file_paths is required", "FILE_PATHS_REQUIRED")
		return
	}

	project, err := h.store.GetProject(projectID)
	if err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("project %s not found", projectID), "PROJECT_NOT_FOUND")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load project", "GET_PROJECT_FAILED")
		return
	}
	repoPath := strings.TrimSpace(project.RepoPath)
	if repoPath == "" {
		writeAPIError(w, http.StatusBadRequest, "project repo_path is required", "REPO_PATH_REQUIRED")
		return
	}

	session, err := h.store.GetChatSession(req.SessionID)
	if err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("chat session %s not found", req.SessionID), "CHAT_SESSION_NOT_FOUND")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load chat session", "GET_CHAT_SESSION_FAILED")
		return
	}
	if session.ProjectID != projectID {
		writeAPIError(w, http.StatusNotFound, fmt.Sprintf("chat session %s not found in project %s", req.SessionID, projectID), "CHAT_SESSION_NOT_FOUND")
		return
	}

	failPolicy, err := parseFailPolicy(req.FailPolicy)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error(), "INVALID_FAIL_POLICY")
		return
	}

	sourceFiles, fileContents, err := loadPlanSourceFiles(repoPath, req.FilePaths)
	if err != nil {
		var validationErr *planFilesValidationError
		if errors.As(err, &validationErr) {
			writeAPIError(w, http.StatusBadRequest, validationErr.Error(), validationErr.Code)
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to read source files", "READ_SOURCE_FILES_FAILED")
		return
	}

	createReq := secretary.Request{
		Conversation: summarizeChatMessages(session.Messages),
		ProjectName:  strings.TrimSpace(project.Name),
		RepoPath:     repoPath,
		Role:         h.planRoleID,
		WorkDir:      repoPath,
	}
	if createReq.WorkDir == "" {
		createReq.WorkDir = "."
	}

	input := secretary.CreateDraftInput{
		ProjectID:    projectID,
		SessionID:    req.SessionID,
		Name:         req.Name,
		FailPolicy:   failPolicy,
		Request:      createReq,
		SourceFiles:  sourceFiles,
		FileContents: clonePlanStringMap(fileContents),
	}

	plan, err := h.planManager.CreateDraftFromFiles(r.Context(), input)
	if err != nil {
		log.Printf("[web][plan] create draft from files failed project=%s session=%s err=%v", projectID, req.SessionID, err)
		writeAPIError(w, http.StatusInternalServerError, "failed to create task plan", "CREATE_TASK_PLAN_FAILED")
		return
	}
	if plan == nil || strings.TrimSpace(plan.ID) == "" {
		writeAPIError(w, http.StatusInternalServerError, "failed to create task plan", "CREATE_TASK_PLAN_FAILED")
		return
	}

	reviewInput := h.buildReviewInput(plan)
	reviewInput.PlanFileContents = clonePlanStringMap(fileContents)
	if _, err := h.planManager.SubmitReview(r.Context(), plan.ID, reviewInput); err != nil {
		if isPlanStatusConflictError(err) {
			writeAPIError(w, http.StatusConflict, err.Error(), "PLAN_STATUS_INVALID")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to update task plan", "SAVE_TASK_PLAN_FAILED")
		return
	}

	writeJSON(w, http.StatusCreated, buildPlanFromFilesResponse(plan, sourceFiles, fileContents))
}

func resolvePlanParserRoleID(roleID string) string {
	trimmed := strings.TrimSpace(roleID)
	if trimmed == "" {
		return defaultPlanParserRoleID
	}
	return trimmed
}

func (h *planHandlers) listPlans(w http.ResponseWriter, r *http.Request) {
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
	status := strings.TrimSpace(r.URL.Query().Get("status"))

	// Keep `total` as unpaginated count for client-side paginator semantics.
	allItems, err := h.store.ListTaskPlans(projectID, core.TaskPlanFilter{
		Status: status,
	})
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to count task plans", "COUNT_TASK_PLANS_FAILED")
		return
	}
	total := len(allItems)

	items, err := h.store.ListTaskPlans(projectID, core.TaskPlanFilter{
		Status: status,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to list task plans", "LIST_TASK_PLANS_FAILED")
		return
	}

	writeJSON(w, http.StatusOK, planListResponse{
		Items:  normalizeTaskPlansForAPI(items),
		Total:  total,
		Offset: offset,
	})
}

func (h *planHandlers) getPlan(w http.ResponseWriter, r *http.Request) {
	plan, ok := h.loadPlanForProject(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, normalizeTaskPlanForAPI(plan))
}

func (h *planHandlers) getPlanDAG(w http.ResponseWriter, r *http.Request) {
	plan, ok := h.loadPlanForProject(w, r)
	if !ok {
		return
	}

	dag := secretary.Build(plan.Tasks)
	if err := dag.Validate(); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error(), "INVALID_TASK_DAG")
		return
	}

	nodes := make([]planDAGNode, len(plan.Tasks))
	stats := planDAGStats{Total: len(plan.Tasks)}
	for i := range plan.Tasks {
		item := plan.Tasks[i]
		nodes[i] = planDAGNode{
			ID:         item.ID,
			Title:      item.Title,
			Status:     item.Status,
			PipelineID: item.PipelineID,
		}

		switch item.Status {
		case core.ItemPending:
			stats.Pending++
		case core.ItemReady:
			stats.Ready++
		case core.ItemRunning:
			stats.Running++
		case core.ItemDone:
			stats.Done++
		case core.ItemFailed:
			stats.Failed++
		}
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].ID < nodes[j].ID
	})

	edges := make([]planDAGEdge, 0, len(plan.Tasks))
	for from, downstream := range dag.Downstream {
		for _, to := range downstream {
			edges = append(edges, planDAGEdge{
				From: from,
				To:   to,
			})
		}
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From == edges[j].From {
			return edges[i].To < edges[j].To
		}
		return edges[i].From < edges[j].From
	})

	writeJSON(w, http.StatusOK, planDAGResponse{
		Nodes: nodes,
		Edges: edges,
		Stats: stats,
	})
}

func (h *planHandlers) submitReview(w http.ResponseWriter, r *http.Request) {
	plan, ok := h.loadPlanForProject(w, r)
	if !ok {
		return
	}

	if h.planManager == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "plan manager is not configured", "PLAN_MANAGER_UNAVAILABLE")
		return
	}

	updated, err := h.planManager.SubmitReview(r.Context(), plan.ID, h.buildReviewInput(plan))
	if err != nil {
		if isPlanStatusConflictError(err) {
			writeAPIError(w, http.StatusConflict, err.Error(), "PLAN_STATUS_INVALID")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to update task plan", "SAVE_TASK_PLAN_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, taskPlanStatusResponse{
		Status: string(updated.Status),
	})
}

func (h *planHandlers) applyAction(w http.ResponseWriter, r *http.Request) {
	plan, ok := h.loadPlanForProject(w, r)
	if !ok {
		return
	}

	var req planActionRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid json body", "INVALID_JSON")
		return
	}

	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action == "" {
		writeAPIError(w, http.StatusBadRequest, "action is required", "ACTION_REQUIRED")
		return
	}

	if h.planManager == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "plan manager is not configured", "PLAN_MANAGER_UNAVAILABLE")
		return
	}

	managerAction := secretary.PlanAction{Action: action}
	switch action {
	case "approve":
		// no-op
	case "reject":
		if err := validatePlanRejectFeedback(req.Feedback); err != nil {
			writeAPIError(w, http.StatusBadRequest, err.Error(), feedbackErrorCode(err))
			return
		}
		managerAction.Feedback = &secretary.HumanFeedback{
			Category:          secretary.FeedbackCategory(strings.TrimSpace(req.Feedback.Category)),
			Detail:            strings.TrimSpace(req.Feedback.Detail),
			ExpectedDirection: strings.TrimSpace(req.Feedback.ExpectedDirection),
		}
	case "abort", "abandon":
		managerAction.Action = secretary.PlanActionAbandon
	default:
		writeAPIError(w, http.StatusBadRequest, fmt.Sprintf("unsupported plan action %q", action), "INVALID_ACTION")
		return
	}

	updated, err := h.planManager.ApplyPlanAction(r.Context(), plan.ID, managerAction)
	if err != nil {
		switch {
		case isPlanStatusConflictError(err):
			writeAPIError(w, http.StatusConflict, err.Error(), "PLAN_STATUS_INVALID")
		case isFeedbackValidationError(err):
			writeAPIError(w, http.StatusBadRequest, err.Error(), feedbackErrorCode(err))
		case strings.Contains(strings.ToLower(err.Error()), "unsupported plan action"):
			writeAPIError(w, http.StatusBadRequest, err.Error(), "INVALID_ACTION")
		default:
			writeAPIError(w, http.StatusInternalServerError, "failed to update task plan", "SAVE_TASK_PLAN_FAILED")
		}
		return
	}
	writeJSON(w, http.StatusOK, taskPlanStatusResponse{
		Status: string(updated.Status),
	})
}

func (h *planHandlers) loadPlanForProject(w http.ResponseWriter, r *http.Request) (*core.TaskPlan, bool) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return nil, false
	}

	projectID := strings.TrimSpace(chi.URLParam(r, "projectID"))
	planID := strings.TrimSpace(chi.URLParam(r, "id"))
	if projectID == "" || planID == "" {
		writeAPIError(w, http.StatusBadRequest, "project id and plan id are required", "INVALID_PATH_PARAM")
		return nil, false
	}

	plan, err := h.store.GetTaskPlan(planID)
	if err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("task plan %s not found", planID), "TASK_PLAN_NOT_FOUND")
			return nil, false
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load task plan", "GET_TASK_PLAN_FAILED")
		return nil, false
	}
	if plan.ProjectID != projectID {
		writeAPIError(w, http.StatusNotFound, fmt.Sprintf("task plan %s not found in project %s", planID, projectID), "TASK_PLAN_NOT_FOUND")
		return nil, false
	}

	return plan, true
}

func (h *planHandlers) buildReviewInput(plan *core.TaskPlan) secretary.ReviewInput {
	if h == nil || h.store == nil || plan == nil {
		return secretary.ReviewInput{}
	}

	input := secretary.ReviewInput{}
	sessionID := strings.TrimSpace(plan.SessionID)
	if sessionID != "" {
		if session, err := h.store.GetChatSession(sessionID); err == nil && session != nil {
			input.Conversation = summarizeChatMessages(session.Messages)
		}
	}

	if project, err := h.store.GetProject(plan.ProjectID); err == nil && project != nil {
		projectName := strings.TrimSpace(project.Name)
		repoPath := strings.TrimSpace(project.RepoPath)
		parts := make([]string, 0, 2)
		if projectName != "" {
			parts = append(parts, "project="+projectName)
		}
		if repoPath != "" {
			parts = append(parts, "repo="+repoPath)
		}
		input.ProjectContext = strings.Join(parts, " ")
	}
	return input
}

func summarizeChatMessages(messages []core.ChatMessage) string {
	if len(messages) == 0 {
		return ""
	}
	lines := make([]string, 0, len(messages))
	for i := range messages {
		content := strings.TrimSpace(messages[i].Content)
		if content == "" {
			continue
		}
		role := strings.TrimSpace(messages[i].Role)
		if role == "" {
			role = "user"
		}
		lines = append(lines, fmt.Sprintf("%s: %s", role, content))
	}
	return strings.Join(lines, "\n")
}

func isPlanStatusConflictError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "submit review requires") ||
		strings.Contains(msg, "approve requires") ||
		strings.Contains(msg, "reject requires") ||
		strings.Contains(msg, "abandon requires")
}

func isFeedbackValidationError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "feedback")
}

func parseFailPolicy(raw string) (core.FailurePolicy, error) {
	switch raw {
	case "", string(core.FailBlock):
		return core.FailBlock, nil
	case string(core.FailSkip):
		return core.FailSkip, nil
	case string(core.FailHuman):
		return core.FailHuman, nil
	default:
		return "", fmt.Errorf("invalid fail_policy %q", raw)
	}
}

func validatePlanRejectFeedback(feedback *planActionFeedback) error {
	if feedback == nil {
		return fmt.Errorf("reject action requires feedback")
	}

	// 第一段：字段必填校验（category + detail）
	category := strings.TrimSpace(feedback.Category)
	if category == "" {
		return fmt.Errorf("reject action requires feedback.category")
	}
	detail := strings.TrimSpace(feedback.Detail)
	if detail == "" {
		return fmt.Errorf("reject action requires feedback.detail")
	}

	// 第二段：复用领域校验（合法类别 + detail 最少长度）
	err := secretary.HumanFeedback{
		Category:          secretary.FeedbackCategory(category),
		Detail:            detail,
		ExpectedDirection: strings.TrimSpace(feedback.ExpectedDirection),
	}.Validate()
	if err != nil {
		return err
	}
	return nil
}

func feedbackErrorCode(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "feedback.category"):
		return "FEEDBACK_CATEGORY_REQUIRED"
	case strings.Contains(msg, "feedback.detail"):
		return "FEEDBACK_DETAIL_REQUIRED"
	case strings.Contains(msg, "requires feedback"):
		return "FEEDBACK_REQUIRED"
	default:
		return "INVALID_FEEDBACK"
	}
}

type planFilesValidationError struct {
	Message string
	Code    string
}

func (e *planFilesValidationError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func loadPlanSourceFiles(repoPath string, filePaths []string) ([]string, map[string]string, error) {
	repoRoot := strings.TrimSpace(repoPath)
	if repoRoot == "" {
		return nil, nil, &planFilesValidationError{
			Message: "project repo_path is required",
			Code:    "REPO_PATH_REQUIRED",
		}
	}
	absRepoRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, nil, &planFilesValidationError{
			Message: "invalid project repo_path",
			Code:    "INVALID_REPO_PATH",
		}
	}

	sourceFiles := make([]string, 0, len(filePaths))
	fileContents := make(map[string]string, len(filePaths))
	seen := make(map[string]struct{}, len(filePaths))
	var totalBytes int64

	for i := range filePaths {
		absPath, normalizedPath, err := resolvePlanSourceFilePath(absRepoRoot, filePaths[i])
		if err != nil {
			return nil, nil, err
		}
		if _, duplicated := seen[normalizedPath]; duplicated {
			continue
		}

		info, err := os.Stat(absPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil, nil, &planFilesValidationError{
					Message: fmt.Sprintf("source file %s not found", normalizedPath),
					Code:    "FILE_NOT_FOUND",
				}
			}
			return nil, nil, &planFilesValidationError{
				Message: fmt.Sprintf("failed to read source file %s", normalizedPath),
				Code:    "FILE_READ_FAILED",
			}
		}
		if info.IsDir() {
			return nil, nil, &planFilesValidationError{
				Message: fmt.Sprintf("source file %s not found", normalizedPath),
				Code:    "FILE_NOT_FOUND",
			}
		}
		if info.Size() > maxPlanSourceFileBytes {
			return nil, nil, &planFilesValidationError{
				Message: fmt.Sprintf("source file %s exceeds 1MB", normalizedPath),
				Code:    "FILE_TOO_LARGE",
			}
		}
		if totalBytes+info.Size() > maxPlanSourceFilesTotalBytes {
			return nil, nil, &planFilesValidationError{
				Message: "total source file size exceeds 5MB",
				Code:    "FILE_TOTAL_TOO_LARGE",
			}
		}

		content, err := os.ReadFile(absPath)
		if err != nil {
			return nil, nil, &planFilesValidationError{
				Message: fmt.Sprintf("failed to read source file %s", normalizedPath),
				Code:    "FILE_READ_FAILED",
			}
		}
		contentBytes := int64(len(content))
		if contentBytes > maxPlanSourceFileBytes {
			return nil, nil, &planFilesValidationError{
				Message: fmt.Sprintf("source file %s exceeds 1MB", normalizedPath),
				Code:    "FILE_TOO_LARGE",
			}
		}
		if totalBytes+contentBytes > maxPlanSourceFilesTotalBytes {
			return nil, nil, &planFilesValidationError{
				Message: "total source file size exceeds 5MB",
				Code:    "FILE_TOTAL_TOO_LARGE",
			}
		}

		sourceFiles = append(sourceFiles, normalizedPath)
		fileContents[normalizedPath] = string(content)
		seen[normalizedPath] = struct{}{}
		totalBytes += contentBytes
	}

	if len(sourceFiles) == 0 {
		return nil, nil, &planFilesValidationError{
			Message: "file_paths is required",
			Code:    "FILE_PATHS_REQUIRED",
		}
	}
	return sourceFiles, fileContents, nil
}

func resolvePlanSourceFilePath(repoRoot string, rawPath string) (string, string, error) {
	trimmed := strings.TrimSpace(rawPath)
	absPath, normalizedPath, err := validateRelativePath(repoRoot, trimmed)
	if err != nil {
		if errors.Is(err, errRelativePathRequired) {
			return "", "", &planFilesValidationError{
				Message: "file_paths contains empty path",
				Code:    "FILE_PATH_REQUIRED",
			}
		}
		return "", "", &planFilesValidationError{
			Message: fmt.Sprintf("invalid file path %q", trimmed),
			Code:    "INVALID_FILE_PATH",
		}
	}
	if normalizedPath == "." {
		return "", "", &planFilesValidationError{
			Message: "file_paths contains empty path",
			Code:    "FILE_PATH_REQUIRED",
		}
	}
	return absPath, normalizedPath, nil
}

func buildPlanFromFilesResponse(plan *core.TaskPlan, sourceFiles []string, fileContents map[string]string) map[string]any {
	payload := map[string]any{}
	normalized := normalizeTaskPlanForAPI(plan)
	if normalized != nil {
		if raw, err := json.Marshal(normalized); err == nil {
			_ = json.Unmarshal(raw, &payload)
		}
	}
	payload["source_files"] = normalizeStringSlice(sourceFiles)
	payload["file_contents"] = clonePlanStringMap(fileContents)
	return payload
}

func clonePlanStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func normalizeTaskPlansForAPI(items []core.TaskPlan) []core.TaskPlan {
	if len(items) == 0 {
		return []core.TaskPlan{}
	}
	out := make([]core.TaskPlan, len(items))
	for i := range items {
		normalized := normalizeTaskPlanForAPI(&items[i])
		if normalized == nil {
			out[i] = core.TaskPlan{}
			continue
		}
		out[i] = *normalized
	}
	return out
}

func normalizeTaskPlanForAPI(plan *core.TaskPlan) *core.TaskPlan {
	if plan == nil {
		return nil
	}
	clone := *plan
	if len(plan.Tasks) == 0 {
		clone.Tasks = []core.TaskItem{}
		return &clone
	}
	clone.Tasks = make([]core.TaskItem, len(plan.Tasks))
	for i := range plan.Tasks {
		task := plan.Tasks[i]
		task.Labels = normalizeStringSlice(task.Labels)
		task.DependsOn = normalizeStringSlice(task.DependsOn)
		task.Inputs = normalizeStringSlice(task.Inputs)
		task.Outputs = normalizeStringSlice(task.Outputs)
		task.Acceptance = normalizeStringSlice(task.Acceptance)
		task.Constraints = normalizeStringSlice(task.Constraints)
		clone.Tasks[i] = task
	}
	return &clone
}

func normalizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}
