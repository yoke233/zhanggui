package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/yoke233/ai-workflow/internal/core"
)

type createThreadRequest struct {
	Title    string         `json:"title"`
	OwnerID  string         `json:"owner_id,omitempty"`
	Summary  string         `json:"summary,omitempty"`
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
	SenderID         string         `json:"sender_id"`
	Role             string         `json:"role,omitempty"`
	Content          string         `json:"content"`
	ReplyToMessageID *int64         `json:"reply_to_msg_id,omitempty"`
	TargetAgentID    string         `json:"target_agent_id,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
}

type addThreadParticipantRequest struct {
	UserID string `json:"user_id"`
	Role   string `json:"role,omitempty"`
}

type inviteThreadAgentRequest struct {
	AgentProfileID string `json:"agent_profile_id"`
}

type threadAggregateStore interface {
	CreateThreadWithParticipants(ctx context.Context, thread *core.Thread, participants []*core.ThreadMember) error
}

type createWorkItemFromThreadRequest struct {
	Title     string `json:"title"`
	Body      string `json:"body,omitempty"`
	ProjectID *int64 `json:"project_id,omitempty"`
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
	r.Get("/threads/{threadID}/events", h.listThreadEvents)

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
		Summary:  strings.TrimSpace(req.Summary),
		Metadata: req.Metadata,
	}

	if txStore, ok := h.store.(threadAggregateStore); ok {
		participants := buildThreadParticipants(thread.OwnerID, nil)
		if err := txStore.CreateThreadWithParticipants(r.Context(), thread, participants); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error(), "CREATE_THREAD_FAILED")
			return
		}
	} else {
		id, err := h.store.CreateThread(r.Context(), thread)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error(), "CREATE_THREAD_FAILED")
			return
		}
		thread.ID = id

		if _, err := h.ensureThreadParticipant(r.Context(), thread.ID, thread.OwnerID, "owner"); err != nil {
			if rollbackErr := h.store.DeleteThread(r.Context(), thread.ID); rollbackErr != nil {
				writeError(w, http.StatusInternalServerError, err.Error()+"; rollback failed: "+rollbackErr.Error(), "CREATE_THREAD_FAILED")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error(), "CREATE_THREAD_FAILED")
			return
		}
	}

	writeJSON(w, http.StatusCreated, thread)
}

func (h *Handler) listThreads(w http.ResponseWriter, r *http.Request) {
	filter := core.ThreadFilter{
		Limit:  queryInt(r, "limit", 50),
		Offset: queryInt(r, "offset", 0),
	}
	if s := r.URL.Query().Get("status"); s != "" {
		st, err := core.ParseThreadStatus(s)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error(), "INVALID_THREAD_STATUS")
			return
		}
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
		nextStatus, err := core.ParseThreadStatus(*req.Status)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error(), "INVALID_THREAD_STATUS")
			return
		}
		if !core.CanTransitionThreadStatus(thread.Status, nextStatus) {
			writeError(
				w,
				http.StatusConflict,
				fmt.Sprintf("invalid thread status transition %q -> %q", thread.Status, nextStatus),
				"INVALID_THREAD_STATUS_TRANSITION",
			)
			return
		}
		thread.Status = nextStatus
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

	if _, err := h.store.GetThread(r.Context(), threadID); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "thread not found", "THREAD_NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	if h.threadPool != nil {
		if err := h.threadPool.CleanupThread(r.Context(), threadID); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error(), "CLEANUP_THREAD_FAILED")
			return
		}
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

	var req createThreadMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}

	_, msg, err := h.createThreadMessageAndRoute(r.Context(), threadMessageInput{
		ThreadID:         threadID,
		SenderID:         req.SenderID,
		Role:             req.Role,
		Content:          req.Content,
		ReplyToMessageID: req.ReplyToMessageID,
		Metadata:         req.Metadata,
		TargetAgentID:    req.TargetAgentID,
	})
	if err != nil {
		if apiErr, ok := err.(*threadMessageAPIError); ok {
			status := http.StatusBadRequest
			switch apiErr.Code {
			case "THREAD_NOT_FOUND":
				status = http.StatusNotFound
			case "TARGET_AGENT_UNAVAILABLE":
				status = http.StatusConflict
			}
			writeError(w, status, apiErr.Message, apiErr.Code)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "CREATE_MESSAGE_FAILED")
		return
	}
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

	p := &core.ThreadMember{
		ThreadID: threadID,
		Kind:     core.ThreadMemberKindHuman,
		UserID:   strings.TrimSpace(req.UserID),
		Role:     req.Role,
	}

	id, err := h.store.AddThreadMember(r.Context(), p)
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

	members, err := h.store.ListThreadMembers(r.Context(), threadID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if members == nil {
		members = []*core.ThreadMember{}
	}
	writeJSON(w, http.StatusOK, members)
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

	members, err := h.store.ListThreadMembers(r.Context(), threadID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	for _, m := range members {
		if m == nil || m.UserID != userID {
			continue
		}
		if m.Kind == core.ThreadMemberKindAgent && threadAgentSessionIsActive(m.Status) {
			writeError(w, http.StatusConflict, "remove agent session before removing participant", "AGENT_SESSION_ACTIVE")
			return
		}
	}

	if err := h.store.RemoveThreadMemberByUser(r.Context(), threadID, userID); err != nil {
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
	thread, err := h.store.GetThread(r.Context(), threadID)
	if err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "thread not found", "THREAD_NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}

	var req createWorkItemFromThreadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}
	issue, err := h.createWorkItemFromThreadData(r.Context(), thread, req.Title, req.Body, req.ProjectID)
	if err != nil {
		if apiErr, ok := err.(*threadMessageAPIError); ok {
			writeError(w, http.StatusBadRequest, apiErr.Message, apiErr.Code)
			return
		}
		code := "CREATE_ISSUE_FAILED"
		if strings.Contains(err.Error(), "rollback failed") {
			code = "CREATE_LINK_FAILED"
		}
		writeError(w, http.StatusInternalServerError, err.Error(), code)
		return
	}

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
		member, err := h.threadPool.InviteAgent(r.Context(), threadID, profileID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error(), "INVITE_AGENT_FAILED")
			return
		}
		writeJSON(w, http.StatusCreated, member)
		return
	}

	// Fallback: pure DB CRUD (no ACP runtime).
	member := &core.ThreadMember{
		ThreadID:       threadID,
		Kind:           core.ThreadMemberKindAgent,
		UserID:         profileID,
		AgentProfileID: profileID,
		Role:           core.ThreadMemberKindAgent,
		Status:         core.ThreadAgentActive,
	}
	id, err := h.store.AddThreadMember(r.Context(), member)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "INVITE_AGENT_FAILED")
		return
	}
	member.ID = id
	writeJSON(w, http.StatusCreated, member)
}

func (h *Handler) listThreadAgents(w http.ResponseWriter, r *http.Request) {
	threadID, ok := urlParamInt64(r, "threadID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid thread ID", "BAD_ID")
		return
	}

	allMembers, err := h.store.ListThreadMembers(r.Context(), threadID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	agents := make([]*core.ThreadMember, 0)
	for _, m := range allMembers {
		if m != nil && m.Kind == core.ThreadMemberKindAgent {
			agents = append(agents, m)
		}
	}
	writeJSON(w, http.StatusOK, agents)
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

	member, err := h.store.GetThreadMember(r.Context(), agentSessionID)
	if err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "agent session not found", "AGENT_SESSION_NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if member.ThreadID != threadID {
		writeError(w, http.StatusNotFound, "agent session not found", "AGENT_SESSION_NOT_FOUND")
		return
	}

	// Fallback: pure DB delete.
	if err := h.store.RemoveThreadMember(r.Context(), agentSessionID); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "agent session not found", "AGENT_SESSION_NOT_FOUND")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "REMOVE_AGENT_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}
