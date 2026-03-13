package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/yoke233/ai-workflow/internal/core"
)

type createThreadRequest struct {
	Title    string         `json:"title"`
	OwnerID  string         `json:"owner_id,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type updateThreadRequest struct {
	Title    *string        `json:"title,omitempty"`
	Status   *string        `json:"status,omitempty"`
	OwnerID  *string        `json:"owner_id,omitempty"`
	Summary  *string        `json:"summary,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type createThreadMessageRequest struct {
	SenderID string         `json:"sender_id"`
	Role     string         `json:"role,omitempty"`
	Content  string         `json:"content"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type addThreadParticipantRequest struct {
	UserID string `json:"user_id"`
	Role   string `json:"role,omitempty"`
}

type inviteThreadAgentRequest struct {
	AgentProfileID string `json:"agent_profile_id"`
}

type createThreadWorkItemLinkRequest struct {
	WorkItemID   int64  `json:"work_item_id"`
	RelationType string `json:"relation_type,omitempty"`
	IsPrimary    bool   `json:"is_primary,omitempty"`
}

// registerThreadRoutes mounts thread endpoints onto the given router.
func registerThreadRoutes(r chi.Router, h *Handler) {
	r.Post("/threads", h.createThread)
	r.Get("/threads", h.listThreads)
	r.Get("/threads/{threadID}", h.getThread)
	r.Put("/threads/{threadID}", h.updateThread)
	r.Delete("/threads/{threadID}", h.deleteThread)

	r.Post("/threads/{threadID}/messages", h.createThreadMessage)
	r.Get("/threads/{threadID}/messages", h.listThreadMessages)

	r.Post("/threads/{threadID}/participants", h.addThreadParticipant)
	r.Get("/threads/{threadID}/participants", h.listThreadParticipants)
	r.Delete("/threads/{threadID}/participants/{userID}", h.removeThreadParticipant)

	r.Post("/threads/{threadID}/create-work-item", h.createWorkItemFromThread)

	r.Post("/threads/{threadID}/agents", h.inviteThreadAgent)
	r.Get("/threads/{threadID}/agents", h.listThreadAgents)
	r.Delete("/threads/{threadID}/agents/{agentSessionID}", h.removeThreadAgent)

	r.Post("/threads/{threadID}/links/work-items", h.createThreadWorkItemLink)
	r.Get("/threads/{threadID}/work-items", h.listWorkItemsByThread)
	r.Delete("/threads/{threadID}/links/work-items/{workItemID}", h.deleteThreadWorkItemLink)

	r.Get("/work-items/{issueID}/threads", h.listThreadsByWorkItem)
}

func (h *Handler) createThread(w http.ResponseWriter, r *http.Request) {
	var req createThreadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeError(w, http.StatusBadRequest, "title is required", "MISSING_TITLE")
		return
	}

	thread := &core.Thread{
		Title:    title,
		Status:   core.ThreadActive,
		OwnerID:  strings.TrimSpace(req.OwnerID),
		Metadata: req.Metadata,
	}

	id, err := h.store.CreateThread(r.Context(), thread)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "CREATE_THREAD_FAILED")
		return
	}
	thread.ID = id
	writeJSON(w, http.StatusCreated, thread)
}

func (h *Handler) listThreads(w http.ResponseWriter, r *http.Request) {
	filter := core.ThreadFilter{
		Limit:  queryInt(r, "limit", 50),
		Offset: queryInt(r, "offset", 0),
	}
	if s := r.URL.Query().Get("status"); s != "" {
		st := core.ThreadStatus(s)
		filter.Status = &st
	}

	threads, err := h.store.ListThreads(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if threads == nil {
		threads = []*core.Thread{}
	}
	writeJSON(w, http.StatusOK, threads)
}

func (h *Handler) getThread(w http.ResponseWriter, r *http.Request) {
	threadID, ok := urlParamInt64(r, "threadID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid thread ID", "BAD_ID")
		return
	}

	thread, err := h.store.GetThread(r.Context(), threadID)
	if err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "thread not found", "THREAD_NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, thread)
}

func (h *Handler) updateThread(w http.ResponseWriter, r *http.Request) {
	threadID, ok := urlParamInt64(r, "threadID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid thread ID", "BAD_ID")
		return
	}

	thread, err := h.store.GetThread(r.Context(), threadID)
	if err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "thread not found", "THREAD_NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	var req updateThreadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}

	if req.Title != nil {
		thread.Title = strings.TrimSpace(*req.Title)
	}
	if req.Status != nil {
		thread.Status = core.ThreadStatus(strings.TrimSpace(*req.Status))
	}
	if req.OwnerID != nil {
		thread.OwnerID = strings.TrimSpace(*req.OwnerID)
	}
	if req.Summary != nil {
		thread.Summary = strings.TrimSpace(*req.Summary)
	}
	if req.Metadata != nil {
		thread.Metadata = req.Metadata
	}

	if err := h.store.UpdateThread(r.Context(), thread); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "UPDATE_THREAD_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, thread)
}

func (h *Handler) deleteThread(w http.ResponseWriter, r *http.Request) {
	threadID, ok := urlParamInt64(r, "threadID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid thread ID", "BAD_ID")
		return
	}

	if err := h.store.DeleteThread(r.Context(), threadID); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "thread not found", "THREAD_NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "DELETE_THREAD_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ---------------------------------------------------------------------------
// Thread Messages
// ---------------------------------------------------------------------------

func (h *Handler) createThreadMessage(w http.ResponseWriter, r *http.Request) {
	threadID, ok := urlParamInt64(r, "threadID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid thread ID", "BAD_ID")
		return
	}

	// Verify thread exists.
	if _, err := h.store.GetThread(r.Context(), threadID); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "thread not found", "THREAD_NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	var req createThreadMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		writeError(w, http.StatusBadRequest, "content is required", "MISSING_CONTENT")
		return
	}

	msg := &core.ThreadMessage{
		ThreadID: threadID,
		SenderID: strings.TrimSpace(req.SenderID),
		Role:     req.Role,
		Content:  req.Content,
		Metadata: req.Metadata,
	}

	id, err := h.store.CreateThreadMessage(r.Context(), msg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "CREATE_MESSAGE_FAILED")
		return
	}
	msg.ID = id
	writeJSON(w, http.StatusCreated, msg)
}

func (h *Handler) listThreadMessages(w http.ResponseWriter, r *http.Request) {
	threadID, ok := urlParamInt64(r, "threadID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid thread ID", "BAD_ID")
		return
	}

	limit := queryInt(r, "limit", 50)
	offset := queryInt(r, "offset", 0)

	msgs, err := h.store.ListThreadMessages(r.Context(), threadID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if msgs == nil {
		msgs = []*core.ThreadMessage{}
	}
	writeJSON(w, http.StatusOK, msgs)
}

// ---------------------------------------------------------------------------
// Thread Participants
// ---------------------------------------------------------------------------

func (h *Handler) addThreadParticipant(w http.ResponseWriter, r *http.Request) {
	threadID, ok := urlParamInt64(r, "threadID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid thread ID", "BAD_ID")
		return
	}

	// Verify thread exists.
	if _, err := h.store.GetThread(r.Context(), threadID); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "thread not found", "THREAD_NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	var req addThreadParticipantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	if strings.TrimSpace(req.UserID) == "" {
		writeError(w, http.StatusBadRequest, "user_id is required", "MISSING_USER_ID")
		return
	}

	p := &core.ThreadParticipant{
		ThreadID: threadID,
		UserID:   strings.TrimSpace(req.UserID),
		Role:     req.Role,
	}

	id, err := h.store.AddThreadParticipant(r.Context(), p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "ADD_PARTICIPANT_FAILED")
		return
	}
	p.ID = id
	writeJSON(w, http.StatusCreated, p)
}

func (h *Handler) listThreadParticipants(w http.ResponseWriter, r *http.Request) {
	threadID, ok := urlParamInt64(r, "threadID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid thread ID", "BAD_ID")
		return
	}

	participants, err := h.store.ListThreadParticipants(r.Context(), threadID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if participants == nil {
		participants = []*core.ThreadParticipant{}
	}
	writeJSON(w, http.StatusOK, participants)
}

func (h *Handler) removeThreadParticipant(w http.ResponseWriter, r *http.Request) {
	threadID, ok := urlParamInt64(r, "threadID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid thread ID", "BAD_ID")
		return
	}

	userID := strings.TrimSpace(chi.URLParam(r, "userID"))
	if userID == "" {
		writeError(w, http.StatusBadRequest, "user_id is required", "MISSING_USER_ID")
		return
	}

	if err := h.store.RemoveThreadParticipant(r.Context(), threadID, userID); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "participant not found", "PARTICIPANT_NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "REMOVE_PARTICIPANT_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// ---------------------------------------------------------------------------
// Thread-WorkItem Links
// ---------------------------------------------------------------------------

func (h *Handler) createThreadWorkItemLink(w http.ResponseWriter, r *http.Request) {
	threadID, ok := urlParamInt64(r, "threadID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid thread ID", "BAD_ID")
		return
	}

	// Verify thread exists.
	if _, err := h.store.GetThread(r.Context(), threadID); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "thread not found", "THREAD_NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	var req createThreadWorkItemLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	if req.WorkItemID <= 0 {
		writeError(w, http.StatusBadRequest, "work_item_id is required", "MISSING_WORK_ITEM_ID")
		return
	}

	// Verify work item (issue) exists.
	if _, err := h.store.GetWorkItem(r.Context(), req.WorkItemID); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "work item not found", "WORK_ITEM_NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	link := &core.ThreadWorkItemLink{
		ThreadID:     threadID,
		WorkItemID:   req.WorkItemID,
		RelationType: req.RelationType,
		IsPrimary:    req.IsPrimary,
	}
	if link.RelationType == "" {
		link.RelationType = "related"
	}

	id, err := h.store.CreateThreadWorkItemLink(r.Context(), link)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "CREATE_LINK_FAILED")
		return
	}
	link.ID = id
	writeJSON(w, http.StatusCreated, link)
}

func (h *Handler) listWorkItemsByThread(w http.ResponseWriter, r *http.Request) {
	threadID, ok := urlParamInt64(r, "threadID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid thread ID", "BAD_ID")
		return
	}

	links, err := h.store.ListWorkItemsByThread(r.Context(), threadID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if links == nil {
		links = []*core.ThreadWorkItemLink{}
	}
	writeJSON(w, http.StatusOK, links)
}

func (h *Handler) deleteThreadWorkItemLink(w http.ResponseWriter, r *http.Request) {
	threadID, ok := urlParamInt64(r, "threadID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid thread ID", "BAD_ID")
		return
	}
	workItemID, ok := urlParamInt64(r, "workItemID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid work item ID", "BAD_ID")
		return
	}

	if err := h.store.DeleteThreadWorkItemLink(r.Context(), threadID, workItemID); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "link not found", "LINK_NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "DELETE_LINK_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handler) listThreadsByWorkItem(w http.ResponseWriter, r *http.Request) {
	issueID, ok := urlParamInt64(r, "issueID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid issue ID", "BAD_ID")
		return
	}

	links, err := h.store.ListThreadsByWorkItem(r.Context(), issueID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if links == nil {
		links = []*core.ThreadWorkItemLink{}
	}
	writeJSON(w, http.StatusOK, links)
}

// ---------------------------------------------------------------------------
// Create WorkItem from Thread (convenience endpoint)
// ---------------------------------------------------------------------------

func (h *Handler) createWorkItemFromThread(w http.ResponseWriter, r *http.Request) {
	threadID, ok := urlParamInt64(r, "threadID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid thread ID", "BAD_ID")
		return
	}

	// Verify thread exists.
	if _, err := h.store.GetThread(r.Context(), threadID); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "thread not found", "THREAD_NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	var req struct {
		Title     string `json:"title"`
		Body      string `json:"body,omitempty"`
		ProjectID *int64 `json:"project_id,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		writeError(w, http.StatusBadRequest, "title is required", "MISSING_TITLE")
		return
	}

	// Create issue.
	issue := &core.WorkItem{
		Title:     title,
		Body:      req.Body,
		Status:    core.WorkItemOpen,
		Priority:  core.PriorityMedium,
		ProjectID: req.ProjectID,
	}
	issueID, err := h.store.CreateWorkItem(r.Context(), issue)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "CREATE_ISSUE_FAILED")
		return
	}
	issue.ID = issueID

	// Auto-create primary link.
	link := &core.ThreadWorkItemLink{
		ThreadID:     threadID,
		WorkItemID:   issueID,
		RelationType: "drives",
		IsPrimary:    true,
	}
	h.store.CreateThreadWorkItemLink(r.Context(), link)

	writeJSON(w, http.StatusCreated, issue)
}

// ---------------------------------------------------------------------------
// Thread Agent Sessions
// ---------------------------------------------------------------------------

func (h *Handler) inviteThreadAgent(w http.ResponseWriter, r *http.Request) {
	threadID, ok := urlParamInt64(r, "threadID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid thread ID", "BAD_ID")
		return
	}

	// Verify thread exists.
	if _, err := h.store.GetThread(r.Context(), threadID); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "thread not found", "THREAD_NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	var req inviteThreadAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	profileID := strings.TrimSpace(req.AgentProfileID)
	if profileID == "" {
		writeError(w, http.StatusBadRequest, "agent_profile_id is required", "MISSING_PROFILE_ID")
		return
	}

	// If runtime pool is available, delegate to it for real ACP session.
	if h.threadPool != nil {
		sess, err := h.threadPool.InviteAgent(r.Context(), threadID, profileID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error(), "INVITE_AGENT_FAILED")
			return
		}
		writeJSON(w, http.StatusCreated, sess)
		return
	}

	// Fallback: pure DB CRUD (no ACP runtime).
	sess := &core.ThreadAgentSession{
		ThreadID:       threadID,
		AgentProfileID: profileID,
		Status:         core.ThreadAgentActive,
	}
	id, err := h.store.CreateThreadAgentSession(r.Context(), sess)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "INVITE_AGENT_FAILED")
		return
	}
	sess.ID = id
	writeJSON(w, http.StatusCreated, sess)
}

func (h *Handler) listThreadAgents(w http.ResponseWriter, r *http.Request) {
	threadID, ok := urlParamInt64(r, "threadID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid thread ID", "BAD_ID")
		return
	}

	sessions, err := h.store.ListThreadAgentSessions(r.Context(), threadID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if sessions == nil {
		sessions = []*core.ThreadAgentSession{}
	}
	writeJSON(w, http.StatusOK, sessions)
}

func (h *Handler) removeThreadAgent(w http.ResponseWriter, r *http.Request) {
	threadID, ok := urlParamInt64(r, "threadID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid thread ID", "BAD_ID")
		return
	}
	agentSessionID, ok := urlParamInt64(r, "agentSessionID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid agent session ID", "BAD_ID")
		return
	}

	// If runtime pool is available, delegate to it for graceful shutdown.
	if h.threadPool != nil {
		if err := h.threadPool.RemoveAgent(r.Context(), threadID, agentSessionID); err != nil {
			if err == core.ErrNotFound {
				writeError(w, http.StatusNotFound, "agent session not found", "AGENT_SESSION_NOT_FOUND")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error(), "REMOVE_AGENT_FAILED")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
		return
	}

	sess, err := h.store.GetThreadAgentSession(r.Context(), agentSessionID)
	if err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "agent session not found", "AGENT_SESSION_NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if sess.ThreadID != threadID {
		writeError(w, http.StatusNotFound, "agent session not found", "AGENT_SESSION_NOT_FOUND")
		return
	}

	// Fallback: pure DB delete.
	if err := h.store.DeleteThreadAgentSession(r.Context(), agentSessionID); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "agent session not found", "AGENT_SESSION_NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "REMOVE_AGENT_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}
