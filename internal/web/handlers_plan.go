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
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/yoke233/ai-workflow/internal/core"
)

const (
	defaultIssueParserRoleID      = "plan_parser"
	maxIssueSourceFileBytes       = 1 << 20 // 1MB
	maxIssueSourceFilesTotalBytes = 5 << 20 // 5MB
	minIssueFeedbackDetailRunes   = 20
)

type issueHandlers struct {
	store        core.Store
	issueManager IssueManager
	issueRoleID  string
}

type createIssuesRequest struct {
	SessionID  string `json:"session_id"`
	Name       string `json:"name"`
	FailPolicy string `json:"fail_policy"`
	AutoMerge  *bool  `json:"auto_merge"`
}

type createIssuesFromFilesRequest struct {
	SessionID  string   `json:"session_id"`
	Name       string   `json:"name"`
	FailPolicy string   `json:"fail_policy"`
	FilePaths  []string `json:"file_paths"`
	AutoMerge  *bool    `json:"auto_merge"`
}

type issueListResponse struct {
	Items  []core.Issue `json:"items"`
	Total  int          `json:"total"`
	Offset int          `json:"offset"`
}

type issueStatusResponse struct {
	Status string `json:"status"`
}

type issueAutoMergeRequest struct {
	AutoMerge *bool `json:"auto_merge"`
}

type issueAutoMergeResponse struct {
	Status    string `json:"status"`
	AutoMerge bool   `json:"auto_merge"`
}

type issueTimelineResponse struct {
	Items  []issueTimelineItem `json:"items"`
	Total  int                 `json:"total"`
	Offset int                 `json:"offset"`
}

type issueTimelineRefs struct {
	IssueID string `json:"issue_id"`
	RunID   string `json:"run_id,omitempty"`
	Stage   string `json:"stage,omitempty"`
}

type issueTimelineItem struct {
	EventID         string            `json:"event_id"`
	Kind            string            `json:"kind"`
	CreatedAt       string            `json:"created_at"`
	ActorType       string            `json:"actor_type"`
	ActorName       string            `json:"actor_name"`
	ActorAvatarSeed string            `json:"actor_avatar_seed"`
	Title           string            `json:"title"`
	Body            string            `json:"body"`
	Status          string            `json:"status"`
	Refs            issueTimelineRefs `json:"refs"`
	Meta            map[string]any    `json:"meta"`
}

type issueTimelineEvent struct {
	item    issueTimelineItem
	at      time.Time
	hasTime bool
	seq     int
}

type issueDAGNode struct {
	ID     string           `json:"id"`
	Title  string           `json:"title"`
	Status core.IssueStatus `json:"status"`
	RunID  string           `json:"run_id"`
}

type issueDAGEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type issueDAGStats struct {
	Total   int `json:"total"`
	Pending int `json:"pending"`
	Ready   int `json:"ready"`
	Running int `json:"running"`
	Done    int `json:"done"`
	Failed  int `json:"failed"`
}

type issueDAGResponse struct {
	Nodes []issueDAGNode `json:"nodes"`
	Edges []issueDAGEdge `json:"edges"`
	Stats issueDAGStats  `json:"stats"`
}

type issueActionRequest struct {
	Action   string               `json:"action"`
	Feedback *issueActionFeedback `json:"feedback,omitempty"`
}

type issueActionFeedback struct {
	Category          string `json:"category"`
	Detail            string `json:"detail"`
	ExpectedDirection string `json:"expected_direction,omitempty"`
}

var allowedIssueFeedbackCategories = map[string]struct{}{
	"missing_node":    {},
	"cycle":           {},
	"self_dependency": {},
	"bad_granularity": {},
	"coverage_gap":    {},
	"other":           {},
}

var issueTimelineKinds = map[string]struct{}{
	"review":     {},
	"action":     {},
	"checkpoint": {},
	"log":        {},
	"change":     {},
	"audit":      {},
}

func registerIssueRoutes(r chi.Router, store core.Store, issueManager IssueManager, issueParserRoleID string) {
	h := &issueHandlers{
		store:        store,
		issueManager: issueManager,
		issueRoleID:  resolveIssueParserRoleID(issueParserRoleID),
	}

	registerResourceRoutes := func(base string) {
		r.Post(base, h.createIssues)
		r.Post(base+"/from-files", h.createIssuesFromFiles)
		r.Get(base, h.listIssues)
		r.Get(base+"/{id}", h.getIssue)
		r.Get(base+"/{id}/dag", h.getIssueDAG)
		r.Get(base+"/{id}/reviews", h.listIssueReviews)
		r.Get(base+"/{id}/changes", h.listIssueChanges)
		r.Get(base+"/{id}/timeline", h.getIssueTimeline)
		r.Post(base+"/{id}/review", h.submitForReview)
		r.Post(base+"/{id}/action", h.applyIssueAction)
		r.Post(base+"/{id}/auto-merge", h.setIssueAutoMerge)
	}

	registerResourceRoutes("/projects/{projectID}/issues")
}

func (h *issueHandlers) createIssues(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}
	if h.issueManager == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "issue manager is not configured", "ISSUE_MANAGER_UNAVAILABLE")
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

	var req createIssuesRequest
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

	createReq := IssueCreateRequest{
		Conversation: summarizeChatMessages(session.Messages),
		ProjectName:  strings.TrimSpace(project.Name),
		RepoPath:     strings.TrimSpace(project.RepoPath),
		Role:         h.issueRoleID,
		WorkDir:      strings.TrimSpace(project.RepoPath),
	}
	if createReq.WorkDir == "" {
		createReq.WorkDir = "."
	}

	issues, err := h.issueManager.CreateIssues(r.Context(), IssueCreateInput{
		ProjectID:  projectID,
		SessionID:  req.SessionID,
		Name:       req.Name,
		FailPolicy: failPolicy,
		AutoMerge:  req.AutoMerge,
		Request:    createReq,
	})
	if err != nil {
		log.Printf("[web][issue] create issues failed project=%s session=%s err=%v", projectID, req.SessionID, err)
		writeAPIError(w, http.StatusInternalServerError, "failed to create issues", "CREATE_ISSUES_FAILED")
		return
	}
	if len(issues) == 0 {
		writeAPIError(w, http.StatusInternalServerError, "failed to create issues", "CREATE_ISSUES_FAILED")
		return
	}

	writeJSON(w, http.StatusCreated, buildCreateIssuesResponse(issues))
}

func (h *issueHandlers) createIssuesFromFiles(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}
	if h.issueManager == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "issue manager is not configured", "ISSUE_MANAGER_UNAVAILABLE")
		return
	}

	projectID := strings.TrimSpace(chi.URLParam(r, "projectID"))
	if projectID == "" {
		writeAPIError(w, http.StatusBadRequest, "project id is required", "PROJECT_ID_REQUIRED")
		return
	}

	var req createIssuesFromFilesRequest
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

	sourceFiles, fileContents, err := loadIssueSourceFiles(repoPath, req.FilePaths)
	if err != nil {
		var validationErr *planFilesValidationError
		if errors.As(err, &validationErr) {
			writeAPIError(w, http.StatusBadRequest, validationErr.Error(), validationErr.Code)
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to read source files", "READ_SOURCE_FILES_FAILED")
		return
	}

	createReq := IssueCreateRequest{
		Conversation: summarizeChatMessages(session.Messages),
		ProjectName:  strings.TrimSpace(project.Name),
		RepoPath:     repoPath,
		Role:         h.issueRoleID,
		WorkDir:      repoPath,
	}
	if createReq.WorkDir == "" {
		createReq.WorkDir = "."
	}

	createdIssues, err := h.issueManager.CreateIssues(r.Context(), IssueCreateInput{
		ProjectID:    projectID,
		SessionID:    req.SessionID,
		Name:         req.Name,
		FailPolicy:   failPolicy,
		AutoMerge:    req.AutoMerge,
		Request:      createReq,
		SourceFiles:  sourceFiles,
		FileContents: cloneIssueStringMap(fileContents),
	})
	if err != nil {
		log.Printf("[web][issue] create issues from files failed project=%s session=%s err=%v", projectID, req.SessionID, err)
		writeAPIError(w, http.StatusInternalServerError, "failed to create issues", "CREATE_ISSUES_FAILED")
		return
	}
	if len(createdIssues) == 0 {
		writeAPIError(w, http.StatusInternalServerError, "failed to create issues", "CREATE_ISSUES_FAILED")
		return
	}

	submittedIssues := make([]core.Issue, 0, len(createdIssues))
	for i := range createdIssues {
		issueID := strings.TrimSpace(createdIssues[i].ID)
		if issueID == "" {
			writeAPIError(w, http.StatusInternalServerError, "failed to create issues", "CREATE_ISSUES_FAILED")
			return
		}
		reviewInput := h.buildReviewInput(&createdIssues[i])
		reviewInput.FileContents = cloneIssueStringMap(fileContents)
		updated, err := h.issueManager.SubmitForReview(r.Context(), issueID, reviewInput)
		if err != nil {
			if isIssueStatusConflictError(err) {
				writeAPIError(w, http.StatusConflict, err.Error(), "ISSUE_STATUS_INVALID")
				return
			}
			writeAPIError(w, http.StatusInternalServerError, "failed to update issue", "SAVE_ISSUE_FAILED")
			return
		}
		if normalized := normalizeIssueForAPI(updated); normalized != nil {
			submittedIssues = append(submittedIssues, *normalized)
			continue
		}
		if normalized := normalizeIssueForAPI(&createdIssues[i]); normalized != nil {
			submittedIssues = append(submittedIssues, *normalized)
		}
	}

	if len(submittedIssues) == 0 {
		writeAPIError(w, http.StatusInternalServerError, "failed to update issue", "SAVE_ISSUE_FAILED")
		return
	}

	writeJSON(w, http.StatusCreated, buildIssueFromFilesResponse(submittedIssues, sourceFiles, fileContents))
}

func resolveIssueParserRoleID(roleID string) string {
	trimmed := strings.TrimSpace(roleID)
	if trimmed == "" {
		return defaultIssueParserRoleID
	}
	return trimmed
}

func (h *issueHandlers) listIssues(w http.ResponseWriter, r *http.Request) {
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

	items, total, err := h.store.ListIssues(projectID, core.IssueFilter{
		Status: status,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to list issues", "LIST_ISSUES_FAILED")
		return
	}

	writeJSON(w, http.StatusOK, issueListResponse{
		Items:  normalizeIssuesForAPI(items),
		Total:  total,
		Offset: offset,
	})
}

func (h *issueHandlers) getIssue(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForProject(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, normalizeIssueForAPI(issue))
}

func (h *issueHandlers) getIssueDAG(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForProject(w, r)
	if !ok {
		return
	}

	allIssues, _, err := h.store.ListIssues(issue.ProjectID, core.IssueFilter{})
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to list issues", "LIST_ISSUES_FAILED")
		return
	}

	allByID := make(map[string]core.Issue, len(allIssues)+1)
	for i := range allIssues {
		normalized := normalizeIssueForAPI(&allIssues[i])
		if normalized == nil {
			continue
		}
		allByID[strings.TrimSpace(normalized.ID)] = *normalized
	}
	if normalized := normalizeIssueForAPI(issue); normalized != nil {
		allByID[strings.TrimSpace(normalized.ID)] = *normalized
	}

	rootID := strings.TrimSpace(issue.ID)
	inScope := map[string]struct{}{}
	addInScope := func(id string) {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			return
		}
		if _, exists := allByID[trimmed]; !exists {
			return
		}
		inScope[trimmed] = struct{}{}
	}

	addInScope(rootID)
	for _, dep := range issue.DependsOn {
		addInScope(dep)
	}
	for _, blocked := range issue.Blocks {
		addInScope(blocked)
	}
	for _, candidate := range allByID {
		if hasIssueReference(candidate.DependsOn, rootID) || hasIssueReference(candidate.Blocks, rootID) {
			addInScope(candidate.ID)
		}
	}

	nodeIDs := make([]string, 0, len(inScope))
	for id := range inScope {
		nodeIDs = append(nodeIDs, id)
	}
	sort.Strings(nodeIDs)

	nodes := make([]issueDAGNode, 0, len(nodeIDs))
	stats := issueDAGStats{}
	for _, id := range nodeIDs {
		item, ok := allByID[id]
		if !ok {
			continue
		}
		nodes = append(nodes, issueDAGNode{
			ID:     item.ID,
			Title:  item.Title,
			Status: item.Status,
			RunID:  item.RunID,
		})
		stats.Total++
		accumulateIssueStats(&stats, item.Status)
	}

	edges := make([]issueDAGEdge, 0, len(nodeIDs)*2)
	edgeSeen := make(map[string]struct{}, len(nodeIDs)*2)
	addEdge := func(from, to string) {
		from = strings.TrimSpace(from)
		to = strings.TrimSpace(to)
		if from == "" || to == "" {
			return
		}
		if _, ok := inScope[from]; !ok {
			return
		}
		if _, ok := inScope[to]; !ok {
			return
		}
		key := from + "->" + to
		if _, exists := edgeSeen[key]; exists {
			return
		}
		edgeSeen[key] = struct{}{}
		edges = append(edges, issueDAGEdge{From: from, To: to})
	}

	for _, id := range nodeIDs {
		item, ok := allByID[id]
		if !ok {
			continue
		}
		for _, dep := range item.DependsOn {
			addEdge(dep, item.ID)
		}
		for _, blocked := range item.Blocks {
			addEdge(item.ID, blocked)
		}
	}

	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From == edges[j].From {
			return edges[i].To < edges[j].To
		}
		return edges[i].From < edges[j].From
	})

	writeJSON(w, http.StatusOK, issueDAGResponse{
		Nodes: nodes,
		Edges: edges,
		Stats: stats,
	})
}

func accumulateIssueStats(stats *issueDAGStats, status core.IssueStatus) {
	if stats == nil {
		return
	}
	switch status {
	case core.IssueStatusReady:
		stats.Ready++
	case core.IssueStatusExecuting:
		stats.Running++
	case core.IssueStatusDone:
		stats.Done++
	case core.IssueStatusFailed, core.IssueStatusSuperseded, core.IssueStatusAbandoned:
		stats.Failed++
	default:
		stats.Pending++
	}
}

func hasIssueReference(values []string, target string) bool {
	trimmedTarget := strings.TrimSpace(target)
	if trimmedTarget == "" {
		return false
	}
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), trimmedTarget) {
			return true
		}
	}
	return false
}

func (h *issueHandlers) submitForReview(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForProject(w, r)
	if !ok {
		return
	}

	if h.issueManager == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "issue manager is not configured", "ISSUE_MANAGER_UNAVAILABLE")
		return
	}

	updated, err := h.issueManager.SubmitForReview(r.Context(), issue.ID, h.buildReviewInput(issue))
	if err != nil {
		if isIssueStatusConflictError(err) {
			writeAPIError(w, http.StatusConflict, err.Error(), "ISSUE_STATUS_INVALID")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to update issue", "SAVE_ISSUE_FAILED")
		return
	}

	status := issue.Status
	if updated != nil {
		status = updated.Status
	}
	writeJSON(w, http.StatusOK, issueStatusResponse{
		Status: string(status),
	})
}

func (h *issueHandlers) listIssueReviews(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForProject(w, r)
	if !ok {
		return
	}

	records, err := h.store.GetReviewRecords(issue.ID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load review records", "GET_REVIEW_RECORDS_FAILED")
		return
	}
	if records == nil {
		records = []core.ReviewRecord{}
	}
	writeJSON(w, http.StatusOK, records)
}

func (h *issueHandlers) listIssueChanges(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForProject(w, r)
	if !ok {
		return
	}

	changes, err := h.store.GetIssueChanges(issue.ID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to load issue changes", "GET_ISSUE_CHANGES_FAILED")
		return
	}
	if changes == nil {
		changes = []core.IssueChange{}
	}
	writeJSON(w, http.StatusOK, changes)
}

func (h *issueHandlers) getIssueTimeline(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForProject(w, r)
	if !ok {
		return
	}

	limit, offset, err := parsePaginationParams(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error(), "INVALID_QUERY_PARAM")
		return
	}

	kinds, err := parseIssueTimelineKinds(r.URL.Query()["kinds"])
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error(), "INVALID_QUERY_PARAM")
		return
	}

	events, err := h.collectIssueTimelineEvents(issue, kinds)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error(), "GET_ISSUE_TIMELINE_FAILED")
		return
	}

	sort.Slice(events, func(i, j int) bool {
		left := events[i]
		right := events[j]
		switch {
		case left.hasTime && right.hasTime && !left.at.Equal(right.at):
			return left.at.Before(right.at)
		case left.hasTime != right.hasTime:
			return left.hasTime
		case left.item.Kind != right.item.Kind:
			return left.item.Kind < right.item.Kind
		default:
			return left.seq < right.seq
		}
	})

	total := len(events)
	if offset >= total {
		writeJSON(w, http.StatusOK, issueTimelineResponse{
			Items:  []issueTimelineItem{},
			Total:  total,
			Offset: offset,
		})
		return
	}
	end := offset + limit
	if end > total {
		end = total
	}

	items := make([]issueTimelineItem, 0, end-offset)
	for i := offset; i < end; i++ {
		items = append(items, events[i].item)
	}

	writeJSON(w, http.StatusOK, issueTimelineResponse{
		Items:  items,
		Total:  total,
		Offset: offset,
	})
}

func (h *issueHandlers) applyIssueAction(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForProject(w, r)
	if !ok {
		return
	}

	var req issueActionRequest
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

	if h.issueManager == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "issue manager is not configured", "ISSUE_MANAGER_UNAVAILABLE")
		return
	}

	managerAction := IssueAction{Action: action}
	switch action {
	case "approve":
		// no-op
	case "reject":
		if err := validateIssueRejectFeedback(req.Feedback); err != nil {
			writeAPIError(w, http.StatusBadRequest, err.Error(), feedbackErrorCode(err))
			return
		}
		managerAction.Feedback = &IssueFeedback{
			Category:          strings.TrimSpace(req.Feedback.Category),
			Detail:            strings.TrimSpace(req.Feedback.Detail),
			ExpectedDirection: strings.TrimSpace(req.Feedback.ExpectedDirection),
		}
	case "abort", "abandon":
		managerAction.Action = "abandon"
	default:
		writeAPIError(w, http.StatusBadRequest, fmt.Sprintf("unsupported issue action %q", action), "INVALID_ACTION")
		return
	}

	updated, err := h.issueManager.ApplyIssueAction(r.Context(), issue.ID, managerAction)
	if err != nil {
		switch {
		case isIssueStatusConflictError(err):
			writeAPIError(w, http.StatusConflict, err.Error(), "ISSUE_STATUS_INVALID")
		case isFeedbackValidationError(err):
			writeAPIError(w, http.StatusBadRequest, err.Error(), feedbackErrorCode(err))
		case strings.Contains(strings.ToLower(err.Error()), "unsupported issue action"),
			strings.Contains(strings.ToLower(err.Error()), "unsupported plan action"):
			writeAPIError(w, http.StatusBadRequest, err.Error(), "INVALID_ACTION")
		default:
			writeAPIError(w, http.StatusInternalServerError, "failed to update issue", "SAVE_ISSUE_FAILED")
		}
		return
	}

	status := issue.Status
	if updated != nil {
		status = updated.Status
	}
	writeJSON(w, http.StatusOK, issueStatusResponse{
		Status: string(status),
	})
}

func (h *issueHandlers) setIssueAutoMerge(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForProject(w, r)
	if !ok {
		return
	}
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}

	var req issueAutoMergeRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid json body", "INVALID_JSON")
		return
	}
	if req.AutoMerge == nil {
		writeAPIError(w, http.StatusBadRequest, "auto_merge is required", "AUTO_MERGE_REQUIRED")
		return
	}

	current := issue.AutoMerge
	next := *req.AutoMerge
	if current != next {
		issue.AutoMerge = next
		if err := h.store.SaveIssue(issue); err != nil {
			writeAPIError(w, http.StatusInternalServerError, "failed to update issue", "SAVE_ISSUE_FAILED")
			return
		}
		if err := h.store.SaveIssueChange(&core.IssueChange{
			IssueID:   issue.ID,
			Field:     "auto_merge",
			OldValue:  strconv.FormatBool(current),
			NewValue:  strconv.FormatBool(next),
			Reason:    "set_auto_merge",
			ChangedBy: "web",
		}); err != nil {
			writeAPIError(w, http.StatusInternalServerError, "failed to save issue change", "SAVE_ISSUE_CHANGE_FAILED")
			return
		}
	}

	writeJSON(w, http.StatusOK, issueAutoMergeResponse{
		Status:    string(issue.Status),
		AutoMerge: issue.AutoMerge,
	})
}

func (h *issueHandlers) loadIssueForProject(w http.ResponseWriter, r *http.Request) (*core.Issue, bool) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return nil, false
	}

	projectID := strings.TrimSpace(chi.URLParam(r, "projectID"))
	issueID := strings.TrimSpace(chi.URLParam(r, "id"))
	if projectID == "" || issueID == "" {
		writeAPIError(w, http.StatusBadRequest, "project id and issue id are required", "INVALID_PATH_PARAM")
		return nil, false
	}

	issue, err := h.store.GetIssue(issueID)
	if err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("issue %s not found", issueID), "ISSUE_NOT_FOUND")
			return nil, false
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load issue", "GET_ISSUE_FAILED")
		return nil, false
	}
	if issue.ProjectID != projectID {
		writeAPIError(w, http.StatusNotFound, fmt.Sprintf("issue %s not found in project %s", issueID, projectID), "ISSUE_NOT_FOUND")
		return nil, false
	}

	return issue, true
}

func (h *issueHandlers) buildReviewInput(issue *core.Issue) IssueReviewInput {
	if h == nil || h.store == nil || issue == nil {
		return IssueReviewInput{}
	}

	input := IssueReviewInput{}
	sessionID := strings.TrimSpace(issue.SessionID)
	if sessionID != "" {
		if session, err := h.store.GetChatSession(sessionID); err == nil && session != nil {
			input.Conversation = summarizeChatMessages(session.Messages)
		}
	}

	if project, err := h.store.GetProject(issue.ProjectID); err == nil && project != nil {
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

func isIssueStatusConflictError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "submit for review requires") ||
		strings.Contains(msg, "submit review requires") ||
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

func validateIssueRejectFeedback(feedback *issueActionFeedback) error {
	if feedback == nil {
		return fmt.Errorf("reject action requires feedback")
	}

	category := strings.TrimSpace(feedback.Category)
	if category == "" {
		return fmt.Errorf("reject action requires feedback.category")
	}
	detail := strings.TrimSpace(feedback.Detail)
	if detail == "" {
		return fmt.Errorf("reject action requires feedback.detail")
	}
	if _, ok := allowedIssueFeedbackCategories[category]; !ok {
		return fmt.Errorf("invalid feedback category %q", category)
	}
	if utf8.RuneCountInString(detail) < minIssueFeedbackDetailRunes {
		return fmt.Errorf("feedback detail must be at least %d characters", minIssueFeedbackDetailRunes)
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

func parseIssueTimelineKinds(rawKinds []string) (map[string]struct{}, error) {
	if len(rawKinds) == 0 {
		out := make(map[string]struct{}, len(issueTimelineKinds))
		for kind := range issueTimelineKinds {
			out[kind] = struct{}{}
		}
		return out, nil
	}

	selected := make(map[string]struct{}, len(issueTimelineKinds))
	for i := range rawKinds {
		for _, part := range strings.Split(rawKinds[i], ",") {
			kind := strings.ToLower(strings.TrimSpace(part))
			if kind == "" {
				continue
			}
			if _, ok := issueTimelineKinds[kind]; !ok {
				return nil, fmt.Errorf("unsupported timeline kind %q", kind)
			}
			selected[kind] = struct{}{}
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("kinds must include at least one valid kind")
	}
	return selected, nil
}

func (h *issueHandlers) collectIssueTimelineEvents(issue *core.Issue, kinds map[string]struct{}) ([]issueTimelineEvent, error) {
	events := make([]issueTimelineEvent, 0)
	seq := 0
	RunID := strings.TrimSpace(issue.RunID)

	appendEvent := func(item issueTimelineItem, at time.Time, hasTime bool) {
		if item.Meta == nil {
			item.Meta = map[string]any{}
		}
		if strings.TrimSpace(item.CreatedAt) == "" {
			item.CreatedAt = formatIssueTimelineTime(at, hasTime)
		}
		if strings.TrimSpace(item.ActorName) == "" {
			item.ActorName = "system"
		}
		if strings.TrimSpace(item.ActorAvatarSeed) == "" {
			item.ActorAvatarSeed = item.ActorName
		}
		item.ActorType = normalizeTimelineActorType(item.ActorType)
		if strings.TrimSpace(item.Status) == "" {
			item.Status = "info"
		}
		events = append(events, issueTimelineEvent{
			item:    item,
			at:      at,
			hasTime: hasTime,
			seq:     seq,
		})
		seq++
	}

	if _, include := kinds["review"]; include {
		records, err := h.store.GetReviewRecords(issue.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to load review records")
		}
		for i := range records {
			record := records[i]
			parsedTime := record.CreatedAt.UTC()
			hasTime := !record.CreatedAt.IsZero()

			reviewer := normalizeTimelineActorName(record.Reviewer, "reviewer")
			summary := strings.TrimSpace(record.Summary)
			rawOutput := strings.TrimSpace(record.RawOutput)
			body := summary
			if body == "" {
				body = "verdict=" + issueTimelineStringOrFallback(record.Verdict, "unknown")
				if record.Score != nil {
					body += fmt.Sprintf(" · score=%d", *record.Score)
				}
			}
			if rawOutput == "" {
				rawOutput = body
			}
			meta := map[string]any{
				"round":        record.Round,
				"verdict":      record.Verdict,
				"issues_count": len(record.Issues),
				"fixes_count":  len(record.Fixes),
				"summary":      summary,
				"raw_output":   rawOutput,
				"issues":       record.Issues,
				"fixes":        record.Fixes,
			}
			if record.Score != nil {
				meta["score"] = *record.Score
			}
			appendEvent(issueTimelineItem{
				EventID:         numericTimelineEventID("review", record.ID, seq),
				Kind:            "review",
				CreatedAt:       formatIssueTimelineTime(parsedTime, hasTime),
				ActorType:       "agent",
				ActorName:       reviewer,
				ActorAvatarSeed: reviewer,
				Title:           "review · " + reviewer,
				Body:            body,
				Status:          issueTimelineStatusFromReview(record.Verdict),
				Refs: issueTimelineRefs{
					IssueID: issue.ID,
					RunID:   RunID,
				},
				Meta: meta,
			}, parsedTime, hasTime)
		}
	}

	if _, include := kinds["change"]; include {
		changes, err := h.store.GetIssueChanges(issue.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to load issue changes")
		}
		for i := range changes {
			change := changes[i]
			parsedTime, hasTime := parseIssueTimelineTimestamp(change.CreatedAt)

			oldValue := timelineDisplayValue(change.OldValue)
			newValue := timelineDisplayValue(change.NewValue)
			body := oldValue + " -> " + newValue
			if reason := strings.TrimSpace(change.Reason); reason != "" {
				body += " · " + reason
			}
			actorName := normalizeTimelineActorName(change.ChangedBy, "system")
			appendEvent(issueTimelineItem{
				EventID:         stringTimelineEventID("change", change.ID, seq),
				Kind:            "change",
				CreatedAt:       formatIssueTimelineTime(parsedTime, hasTime),
				ActorType:       timelineActorTypeFromName(actorName),
				ActorName:       actorName,
				ActorAvatarSeed: actorName,
				Title:           "change · " + issueTimelineStringOrFallback(change.Field, "field"),
				Body:            body,
				Status:          issueTimelineStatusFromChange(change.Field, change.NewValue),
				Refs: issueTimelineRefs{
					IssueID: issue.ID,
					RunID:   RunID,
				},
				Meta: map[string]any{
					"field":      change.Field,
					"old_value":  change.OldValue,
					"new_value":  change.NewValue,
					"reason":     change.Reason,
					"changed_by": change.ChangedBy,
				},
			}, parsedTime, hasTime)
		}
	}

	includeAction := hasIssueTimelineKind(kinds, "action")
	includeAudit := hasIssueTimelineKind(kinds, "audit")
	if includeAction || includeAudit {
		if RunID != "" {
			actions, err := h.store.GetActions(RunID)
			if err != nil {
				return nil, fmt.Errorf("failed to load Run actions")
			}
			for i := range actions {
				action := actions[i]
				isAudit := isIssueTimelineAuditAction(action)
				if isAudit && !includeAudit {
					continue
				}
				if !isAudit && !includeAction {
					continue
				}

				kind := "action"
				if isAudit {
					kind = "audit"
				}
				parsedTime, hasTime := parseIssueTimelineTimestamp(action.CreatedAt)
				defaultActor := "human"
				if isAudit {
					defaultActor = "admin"
				}
				actorName := normalizeTimelineActorName(action.UserID, defaultActor)
				stage := strings.TrimSpace(action.Stage)
				refs := issueTimelineRefs{
					IssueID: issue.ID,
					RunID:   RunID,
				}
				if stage != "" {
					refs.Stage = stage
				}
				appendEvent(issueTimelineItem{
					EventID:         numericTimelineEventID(kind, action.ID, seq),
					Kind:            kind,
					CreatedAt:       formatIssueTimelineTime(parsedTime, hasTime),
					ActorType:       timelineActorTypeFromAction(action, isAudit),
					ActorName:       actorName,
					ActorAvatarSeed: actorName,
					Title:           kind + " · " + issueTimelineStringOrFallback(action.Action, "unknown"),
					Body:            issueTimelineBodyWithFallback(action.Message, "人工操作已执行"),
					Status:          issueTimelineStatusFromAction(action.Action, isAudit),
					Refs:            refs,
					Meta: map[string]any{
						"action":  action.Action,
						"source":  action.Source,
						"user_id": action.UserID,
						"message": action.Message,
					},
				}, parsedTime, hasTime)
			}
		}
	}

	if RunID == "" {
		return events, nil
	}

	if _, include := kinds["checkpoint"]; include {
		checkpoints, err := h.store.GetCheckpoints(RunID)
		if err != nil {
			return nil, fmt.Errorf("failed to load checkpoints")
		}
		for i := range checkpoints {
			checkpoint := checkpoints[i]
			parsedTime, hasTime := issueTimelineCheckpointTime(checkpoint)

			stage := strings.TrimSpace(string(checkpoint.StageName))
			body := "状态=" + issueTimelineStringOrFallback(string(checkpoint.Status), "unknown")
			if errText := strings.TrimSpace(checkpoint.Error); errText != "" {
				body += " · " + errText
			}
			actorName := normalizeTimelineActorName(checkpoint.AgentUsed, "system")
			appendEvent(issueTimelineItem{
				EventID:         checkpointTimelineEventID(checkpoint, seq),
				Kind:            "checkpoint",
				CreatedAt:       formatIssueTimelineTime(parsedTime, hasTime),
				ActorType:       timelineActorTypeFromCheckpoint(checkpoint),
				ActorName:       actorName,
				ActorAvatarSeed: actorName,
				Title:           "checkpoint · " + issueTimelineStringOrFallback(stage, "stage"),
				Body:            body,
				Status:          issueTimelineStatusFromCheckpoint(checkpoint.Status),
				Refs: issueTimelineRefs{
					IssueID: issue.ID,
					RunID:   RunID,
					Stage:   stage,
				},
				Meta: map[string]any{
					"status":      string(checkpoint.Status),
					"retry_count": checkpoint.RetryCount,
					"tokens_used": checkpoint.TokensUsed,
					"error":       checkpoint.Error,
				},
			}, parsedTime, hasTime)
		}
	}

	if _, include := kinds["log"]; include {
		runEvents, err := h.store.ListRunEvents(RunID)
		if err != nil {
			return nil, fmt.Errorf("failed to load run events")
		}
		for i := range runEvents {
			evt := runEvents[i]
			stage := issueTimelineStringOrFallback(evt.Stage, "unknown")
			actorName := normalizeTimelineActorName(evt.Agent, "system")
			content := ""
			if c, ok := evt.Data["content"]; ok {
				content = c
			}
			appendEvent(issueTimelineItem{
				EventID:         numericTimelineEventID("log", evt.ID, seq),
				Kind:            "log",
				CreatedAt:       evt.CreatedAt.UTC().Format(time.RFC3339),
				ActorType:       timelineActorTypeFromRunEvent(evt),
				ActorName:       actorName,
				ActorAvatarSeed: actorName,
				Title:           "log · " + stage + "/" + evt.EventType,
				Body:            issueTimelineBodyWithFallback(content, "log 输出为空"),
				Status:          issueTimelineStatusFromLog(evt.EventType),
				Refs: issueTimelineRefs{
					IssueID: issue.ID,
					RunID:   RunID,
					Stage:   strings.TrimSpace(evt.Stage),
				},
				Meta: map[string]any{
					"type":  evt.EventType,
					"agent": evt.Agent,
					"data":  evt.Data,
				},
			}, evt.CreatedAt, !evt.CreatedAt.IsZero())
		}
	}

	return events, nil
}

func hasIssueTimelineKind(kinds map[string]struct{}, key string) bool {
	_, ok := kinds[key]
	return ok
}

func issueTimelineBodyWithFallback(body string, fallback string) string {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func issueTimelineStringOrFallback(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func timelineDisplayValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "(empty)"
	}
	return trimmed
}

func normalizeTimelineActorName(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func normalizeTimelineActorType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "human", "agent", "system":
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return "system"
	}
}

func timelineActorTypeFromName(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	switch normalized {
	case "", "system", "scheduler", "engine", "workflow-engine":
		return "system"
	case "human", "admin", "user":
		return "human"
	}
	if strings.Contains(normalized, "agent") ||
		strings.Contains(normalized, "reviewer") ||
		strings.Contains(normalized, "codex") ||
		strings.Contains(normalized, "claude") ||
		strings.Contains(normalized, "team_leader") ||
		strings.Contains(normalized, "teamleader") {
		return "agent"
	}
	return "human"
}

func timelineActorTypeFromCheckpoint(checkpoint core.Checkpoint) string {
	if strings.TrimSpace(checkpoint.AgentUsed) != "" {
		return "agent"
	}
	return "system"
}

func timelineActorTypeFromRunEvent(evt core.RunEvent) string {
	agent := strings.ToLower(strings.TrimSpace(evt.Agent))
	if agent == "" || agent == "system" {
		return "system"
	}
	return "agent"
}

func timelineActorTypeFromAction(action core.HumanAction, isAudit bool) string {
	if isAudit {
		return "human"
	}
	source := strings.ToLower(strings.TrimSpace(action.Source))
	if source == "system" {
		return "system"
	}
	return "human"
}

func isIssueTimelineAuditAction(action core.HumanAction) bool {
	return strings.EqualFold(strings.TrimSpace(action.Source), "admin")
}

func issueTimelineStatusFromReview(verdict string) string {
	switch strings.ToLower(strings.TrimSpace(verdict)) {
	case "pass", "approved":
		return "success"
	case "changes_requested", "fail", "rejected":
		return "warning"
	default:
		return "info"
	}
}

func issueTimelineStatusFromChange(field string, newValue string) string {
	if strings.EqualFold(strings.TrimSpace(field), "status") {
		switch strings.ToLower(strings.TrimSpace(newValue)) {
		case "failed", "blocked_by_failure":
			return "failed"
		case "done", "completed", "success":
			return "success"
		case "running", "executing":
			return "running"
		}
	}
	return "info"
}

func issueTimelineStatusFromAction(action string, isAudit bool) string {
	if isAudit {
		return "info"
	}
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "abort", "abandon", "reject":
		return "warning"
	case "approve", "resume", "retry", "change_role":
		return "success"
	default:
		return "info"
	}
}

func issueTimelineStatusFromCheckpoint(status core.CheckpointStatus) string {
	switch status {
	case core.CheckpointSuccess:
		return "success"
	case core.CheckpointFailed:
		return "failed"
	case core.CheckpointInProgress:
		return "running"
	case core.CheckpointSkipped, core.CheckpointInvalidated:
		return "warning"
	default:
		return "info"
	}
}

func issueTimelineStatusFromLog(logType string) string {
	switch strings.ToLower(strings.TrimSpace(logType)) {
	case "stage_complete":
		return "success"
	case "stage_failed":
		return "failed"
	case "stage_start":
		return "running"
	case "human_required":
		return "warning"
	case "action_applied":
		return "success"
	default:
		return "info"
	}
}

func stringTimelineEventID(prefix string, rawID string, fallback int) string {
	trimmed := strings.TrimSpace(rawID)
	if trimmed != "" {
		return prefix + ":" + trimmed
	}
	return fmt.Sprintf("%s:%d", prefix, fallback)
}

func numericTimelineEventID(prefix string, rawID int64, fallback int) string {
	if rawID > 0 {
		return fmt.Sprintf("%s:%d", prefix, rawID)
	}
	return fmt.Sprintf("%s:%d", prefix, fallback)
}

func checkpointTimelineEventID(checkpoint core.Checkpoint, fallback int) string {
	stage := issueTimelineStringOrFallback(string(checkpoint.StageName), "stage")
	if !checkpoint.FinishedAt.IsZero() {
		return fmt.Sprintf("checkpoint:%s:%d", stage, checkpoint.FinishedAt.UTC().UnixNano())
	}
	if !checkpoint.StartedAt.IsZero() {
		return fmt.Sprintf("checkpoint:%s:%d", stage, checkpoint.StartedAt.UTC().UnixNano())
	}
	return fmt.Sprintf("checkpoint:%s:%d", stage, fallback)
}


func issueTimelineCheckpointTime(checkpoint core.Checkpoint) (time.Time, bool) {
	switch {
	case !checkpoint.FinishedAt.IsZero():
		return checkpoint.FinishedAt.UTC(), true
	case !checkpoint.StartedAt.IsZero():
		return checkpoint.StartedAt.UTC(), true
	default:
		return time.Time{}, false
	}
}

func formatIssueTimelineTime(value time.Time, ok bool) string {
	if !ok || value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func parseIssueTimelineTimestamp(raw string) (time.Time, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	}
	for i := range layouts {
		parsed, err := time.Parse(layouts[i], trimmed)
		if err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
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

func loadIssueSourceFiles(repoPath string, filePaths []string) ([]string, map[string]string, error) {
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
		absPath, normalizedPath, err := resolveIssueSourceFilePath(absRepoRoot, filePaths[i])
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
		if info.Size() > maxIssueSourceFileBytes {
			return nil, nil, &planFilesValidationError{
				Message: fmt.Sprintf("source file %s exceeds 1MB", normalizedPath),
				Code:    "FILE_TOO_LARGE",
			}
		}
		if totalBytes+info.Size() > maxIssueSourceFilesTotalBytes {
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
		if contentBytes > maxIssueSourceFileBytes {
			return nil, nil, &planFilesValidationError{
				Message: fmt.Sprintf("source file %s exceeds 1MB", normalizedPath),
				Code:    "FILE_TOO_LARGE",
			}
		}
		if totalBytes+contentBytes > maxIssueSourceFilesTotalBytes {
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

func resolveIssueSourceFilePath(repoRoot string, rawPath string) (string, string, error) {
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

func buildCreateIssuesResponse(issues []core.Issue) map[string]any {
	normalized := normalizeIssuesForAPI(issues)
	payload := map[string]any{
		"items": normalized,
	}
	if len(normalized) > 0 {
		issue := normalized[0]
		payload["issue"] = issue
	}
	return payload
}

func buildIssueFromFilesResponse(issues []core.Issue, sourceFiles []string, fileContents map[string]string) map[string]any {
	payload := buildCreateIssuesResponse(issues)
	payload["source_files"] = normalizeStringSlice(sourceFiles)
	payload["file_contents"] = cloneIssueStringMap(fileContents)
	return payload
}

func cloneIssueStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func normalizeIssuesForAPI(items []core.Issue) []core.Issue {
	if len(items) == 0 {
		return []core.Issue{}
	}
	out := make([]core.Issue, len(items))
	for i := range items {
		normalized := normalizeIssueForAPI(&items[i])
		if normalized == nil {
			out[i] = core.Issue{}
			continue
		}
		out[i] = *normalized
	}
	return out
}

func normalizeIssueForAPI(issue *core.Issue) *core.Issue {
	if issue == nil {
		return nil
	}
	clone := *issue
	clone.Labels = normalizeStringSlice(issue.Labels)
	clone.Attachments = normalizeStringSlice(issue.Attachments)
	clone.DependsOn = normalizeStringSlice(issue.DependsOn)
	clone.Blocks = normalizeStringSlice(issue.Blocks)
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
