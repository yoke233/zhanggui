package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/yoke233/ai-workflow/internal/core"
)

type chatHandlers struct {
	store     core.Store
	assistant ChatAssistant
	publisher chatEventPublisher

	runMu     sync.Mutex
	activeRun map[string]context.CancelFunc
}

type chatSessionDeleter interface {
	DeleteChatSession(id string) error
}

type chatRunEventReader interface {
	ListChatRunEvents(sessionID string) ([]core.ChatRunEvent, error)
}

type chatEventPublisher interface {
	Publish(ctx context.Context, evt core.Event) error
}

type createChatSessionRequest struct {
	Message   string `json:"message"`
	Role      string `json:"role,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

type createChatSessionResponse struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
}

type cancelChatSessionResponse struct {
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
}

type listChatSessionEventsResponse struct {
	SessionID  string              `json:"session_id"`
	ProjectID  string              `json:"project_id"`
	UpdatedAt  time.Time           `json:"updated_at"`
	Messages   []core.ChatMessage  `json:"messages"`
	Events     []core.ChatRunEvent `json:"events"`
	NextCursor string              `json:"next_cursor,omitempty"`
}

type getChatEventGroupResponse struct {
	SessionID string              `json:"session_id"`
	ProjectID string              `json:"project_id"`
	GroupID   string              `json:"group_id"`
	Events    []core.ChatRunEvent `json:"events"`
}

type chatSessionTimelineItem struct {
	token        string
	time         time.Time
	messageIndex int
	eventIndex   int
	isMessage    bool
}

const (
	defaultChatSessionEventsLimit = 50
	maxChatSessionEventsLimit     = 200
)

func registerChatRoutes(r chi.Router, store core.Store, assistant ChatAssistant, publisher chatEventPublisher) {
	h := &chatHandlers{
		store:     store,
		assistant: assistant,
		publisher: publisher,
		activeRun: make(map[string]context.CancelFunc),
	}
	r.With(RequireScope(ScopeChatRead)).Get("/projects/{projectID}/chat", h.listSessions)
	r.With(RequireScope(ScopeChatWrite)).Post("/projects/{projectID}/chat", h.createSession)
	r.With(RequireScope(ScopeChatWrite)).Post("/projects/{projectID}/chat/{sessionID}/cancel", h.cancelSession)
	r.With(RequireScope(ScopeChatRead)).Get("/projects/{projectID}/chat/{sessionID}/events", h.listSessionEvents)
	r.With(RequireScope(ScopeChatRead)).Get("/projects/{projectID}/chat/{sessionID}/event-groups/{groupID}", h.getSessionEventGroup)
	r.With(RequireScope(ScopeChatRead)).Get("/projects/{projectID}/chat/{sessionID}/status", h.getSessionStatus)
	r.With(RequireScope(ScopeChatRead)).Get("/projects/{projectID}/chat/{sessionID}", h.getSession)
	r.With(RequireScope(ScopeChatWrite)).Delete("/projects/{projectID}/chat/{sessionID}", h.deleteSession)
}

func (h *chatHandlers) listSessions(w http.ResponseWriter, r *http.Request) {
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

	sessions, err := h.store.ListChatSessions(projectID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to list chat sessions", "LIST_CHAT_SESSIONS_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}

func (h *chatHandlers) createSession(w http.ResponseWriter, r *http.Request) {
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

	var req createChatSessionRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid json body", "INVALID_JSON")
		return
	}

	req.Message = strings.TrimSpace(req.Message)
	req.Role = strings.ToLower(strings.TrimSpace(req.Role))
	req.SessionID = strings.TrimSpace(req.SessionID)
	if req.Message == "" {
		writeAPIError(w, http.StatusBadRequest, "message is required", "MESSAGE_REQUIRED")
		return
	}
	if req.Role == "" {
		req.Role = "team_leader"
	}
	if !isValidRoleID(req.Role) {
		writeAPIError(w, http.StatusBadRequest, "invalid role", "INVALID_ROLE")
		return
	}
	if h.assistant == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "chat assistant is not configured", "CHAT_ASSISTANT_UNAVAILABLE")
		return
	}

	isNewSession := req.SessionID == ""
	var session *core.ChatSession
	if isNewSession {
		session = &core.ChatSession{
			ID:        core.NewChatSessionID(),
			ProjectID: projectID,
		}
	} else {
		existing, err := h.store.GetChatSession(req.SessionID)
		if err != nil {
			if isNotFoundError(err) {
				writeAPIError(w, http.StatusNotFound, fmt.Sprintf("chat session %s not found", req.SessionID), "CHAT_SESSION_NOT_FOUND")
				return
			}
			writeAPIError(w, http.StatusInternalServerError, "failed to load chat session", "GET_CHAT_SESSION_FAILED")
			return
		}
		if existing.ProjectID != projectID {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("chat session %s not found in project %s", req.SessionID, projectID), "CHAT_SESSION_NOT_FOUND")
			return
		}
		session = existing
	}

	runCtx, cancelRun := context.WithCancel(context.Background())
	if !h.registerSessionRun(session.ID, cancelRun) {
		cancelRun()
		writeAPIError(w, http.StatusConflict, "chat session is already running", "CHAT_SESSION_BUSY")
		return
	}

	now := time.Now().UTC()
	session.Messages = append(session.Messages, core.ChatMessage{
		Role:    "user",
		Content: req.Message,
		Time:    now,
	})

	if isNewSession {
		if err := h.store.CreateChatSession(session); err != nil {
			h.unregisterSessionRun(session.ID)
			cancelRun()
			writeAPIError(w, http.StatusInternalServerError, "failed to create chat session", "CREATE_CHAT_SESSION_FAILED")
			return
		}
	} else {
		if err := h.store.UpdateChatSession(session); err != nil {
			h.unregisterSessionRun(session.ID)
			cancelRun()
			writeAPIError(w, http.StatusInternalServerError, "failed to update chat session", "UPDATE_CHAT_SESSION_FAILED")
			return
		}
	}

	writeJSON(w, http.StatusAccepted, createChatSessionResponse{
		SessionID: session.ID,
		Status:    "accepted",
	})

	go h.executeChatTurn(runCtx, chatRunInput{
		ProjectID:      projectID,
		WorkDir:        strings.TrimSpace(project.RepoPath),
		Role:           req.Role,
		Message:        req.Message,
		SessionID:      session.ID,
		AgentSessionID: strings.TrimSpace(session.AgentSessionID),
	})
}

type chatRunInput struct {
	ProjectID      string
	WorkDir        string
	Role           string
	Message        string
	SessionID      string
	AgentSessionID string
}

func (h *chatHandlers) executeChatTurn(ctx context.Context, input chatRunInput) {
	defer h.unregisterSessionRun(input.SessionID)

	h.publishChatEvent(core.EventRunStarted, input.ProjectID, input.SessionID, input.Role, nil)

	assistantResp, err := h.assistant.Reply(ctx, ChatAssistantRequest{
		Message:        input.Message,
		Role:           input.Role,
		WorkDir:        input.WorkDir,
		AgentSessionID: input.AgentSessionID,
		ProjectID:      input.ProjectID,
		ChatSessionID:  input.SessionID,
	})
	if err != nil {
		eventType := core.EventRunFailed
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			eventType = core.EventRunCancelled
		}
		log.Printf("[chat] assistant reply failed project_id=%s role=%s session_id=%s err=%v", input.ProjectID, input.Role, input.SessionID, err)
		h.publishChatEvent(eventType, input.ProjectID, input.SessionID, input.Role, map[string]string{
			"error": err.Error(),
		})
		return
	}

	reply := strings.TrimSpace(assistantResp.Reply)
	if reply == "" {
		h.publishChatEvent(core.EventRunFailed, input.ProjectID, input.SessionID, input.Role, map[string]string{
			"error": "chat assistant returned empty reply",
		})
		return
	}

	session, err := h.store.GetChatSession(input.SessionID)
	if err != nil {
		h.publishChatEvent(core.EventRunFailed, input.ProjectID, input.SessionID, input.Role, map[string]string{
			"error": "failed to load chat session",
		})
		return
	}
	if session.ProjectID != input.ProjectID {
		h.publishChatEvent(core.EventRunFailed, input.ProjectID, input.SessionID, input.Role, map[string]string{
			"error": "chat session project mismatch",
		})
		return
	}

	if strings.TrimSpace(assistantResp.AgentSessionID) != "" {
		session.AgentSessionID = strings.TrimSpace(assistantResp.AgentSessionID)
	}
	session.Messages = append(session.Messages, core.ChatMessage{
		Role:    "assistant",
		Content: reply,
		Time:    time.Now().UTC(),
	})
	if err := h.store.UpdateChatSession(session); err != nil {
		h.publishChatEvent(core.EventRunFailed, input.ProjectID, input.SessionID, input.Role, map[string]string{
			"error": "failed to update chat session",
		})
		return
	}

	eventData := map[string]string{
		"reply": reply,
	}
	if strings.TrimSpace(session.AgentSessionID) != "" {
		eventData["agent_session_id"] = strings.TrimSpace(session.AgentSessionID)
	}
	h.publishChatEvent(core.EventRunCompleted, input.ProjectID, input.SessionID, input.Role, eventData)
}

func (h *chatHandlers) cancelSession(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}

	projectID := strings.TrimSpace(chi.URLParam(r, "projectID"))
	sessionID := strings.TrimSpace(chi.URLParam(r, "sessionID"))
	if projectID == "" || sessionID == "" {
		writeAPIError(w, http.StatusBadRequest, "project id and session id are required", "INVALID_PATH_PARAM")
		return
	}

	session, err := h.store.GetChatSession(sessionID)
	if err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("chat session %s not found", sessionID), "CHAT_SESSION_NOT_FOUND")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load chat session", "GET_CHAT_SESSION_FAILED")
		return
	}
	if session.ProjectID != projectID {
		writeAPIError(w, http.StatusNotFound, fmt.Sprintf("chat session %s not found in project %s", sessionID, projectID), "CHAT_SESSION_NOT_FOUND")
		return
	}

	if !h.cancelSessionRun(sessionID) {
		writeAPIError(w, http.StatusConflict, "chat session is not running", "CHAT_SESSION_NOT_RUNNING")
		return
	}

	if canceler, ok := h.assistant.(ChatAssistantCanceler); ok {
		_ = canceler.CancelChat(sessionID)
	}

	writeJSON(w, http.StatusAccepted, cancelChatSessionResponse{
		SessionID: sessionID,
		Status:    "cancelling",
	})
}

func isValidRoleID(role string) bool {
	if role == "" {
		return false
	}
	for _, r := range role {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

func (h *chatHandlers) getSession(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}

	projectID := strings.TrimSpace(chi.URLParam(r, "projectID"))
	sessionID := strings.TrimSpace(chi.URLParam(r, "sessionID"))
	if projectID == "" || sessionID == "" {
		writeAPIError(w, http.StatusBadRequest, "project id and session id are required", "INVALID_PATH_PARAM")
		return
	}

	session, err := h.store.GetChatSession(sessionID)
	if err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("chat session %s not found", sessionID), "CHAT_SESSION_NOT_FOUND")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load chat session", "GET_CHAT_SESSION_FAILED")
		return
	}
	if session.ProjectID != projectID {
		writeAPIError(w, http.StatusNotFound, fmt.Sprintf("chat session %s not found in project %s", sessionID, projectID), "CHAT_SESSION_NOT_FOUND")
		return
	}

	writeJSON(w, http.StatusOK, session)
}

func (h *chatHandlers) getSessionStatus(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(chi.URLParam(r, "projectID"))
	sessionID := strings.TrimSpace(chi.URLParam(r, "sessionID"))
	if projectID == "" || sessionID == "" {
		writeAPIError(w, http.StatusBadRequest, "project id and session id are required", "INVALID_PATH_PARAM")
		return
	}

	alive := false
	running := false

	// Check running via handler-level activeRun.
	h.runMu.Lock()
	_, running = h.activeRun[sessionID]
	h.runMu.Unlock()

	// Check alive via pooled ACP session.
	if provider, ok := h.assistant.(ChatSessionStatusProvider); ok {
		alive = provider.IsChatSessionAlive(sessionID)
		if !running {
			running = provider.IsChatSessionRunning(sessionID)
		}
	}

	writeJSON(w, http.StatusOK, map[string]bool{
		"alive":   alive,
		"running": running,
	})
}

func (h *chatHandlers) listSessionEvents(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}

	projectID := strings.TrimSpace(chi.URLParam(r, "projectID"))
	sessionID := strings.TrimSpace(chi.URLParam(r, "sessionID"))
	if projectID == "" || sessionID == "" {
		writeAPIError(w, http.StatusBadRequest, "project id and session id are required", "INVALID_PATH_PARAM")
		return
	}

	session, err := h.store.GetChatSession(sessionID)
	if err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("chat session %s not found", sessionID), "CHAT_SESSION_NOT_FOUND")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load chat session", "GET_CHAT_SESSION_FAILED")
		return
	}
	if session.ProjectID != projectID {
		writeAPIError(w, http.StatusNotFound, fmt.Sprintf("chat session %s not found in project %s", sessionID, projectID), "CHAT_SESSION_NOT_FOUND")
		return
	}

	reader, ok := h.store.(chatRunEventReader)
	events := []core.ChatRunEvent{}
	if ok {
		var err error
		events, err = reader.ListChatRunEvents(sessionID)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "failed to list chat run events", "LIST_CHAT_RUN_EVENTS_FAILED")
			return
		}
	}

	limit, cursor, err := parseChatSessionEventsPaginationParams(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error(), "INVALID_PAGINATION")
		return
	}

	normalizedEvents := normalizeStoredChatRunEvents(events)
	groupedEvents, _ := groupChatRunEventsForList(normalizedEvents)
	messagesPage, eventsPage, nextCursor, err := paginateChatSessionTimeline(session.Messages, groupedEvents, cursor, limit)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error(), "INVALID_CURSOR")
		return
	}

	writeJSON(w, http.StatusOK, listChatSessionEventsResponse{
		SessionID:  session.ID,
		ProjectID:  session.ProjectID,
		UpdatedAt:  session.UpdatedAt,
		Messages:   messagesPage,
		Events:     eventsPage,
		NextCursor: nextCursor,
	})
}

func (h *chatHandlers) getSessionEventGroup(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}

	projectID := strings.TrimSpace(chi.URLParam(r, "projectID"))
	sessionID := strings.TrimSpace(chi.URLParam(r, "sessionID"))
	groupID := strings.TrimSpace(chi.URLParam(r, "groupID"))
	if projectID == "" || sessionID == "" || groupID == "" {
		writeAPIError(w, http.StatusBadRequest, "project id, session id and group id are required", "INVALID_PATH_PARAM")
		return
	}

	session, err := h.store.GetChatSession(sessionID)
	if err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("chat session %s not found", sessionID), "CHAT_SESSION_NOT_FOUND")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load chat session", "GET_CHAT_SESSION_FAILED")
		return
	}
	if session.ProjectID != projectID {
		writeAPIError(w, http.StatusNotFound, fmt.Sprintf("chat session %s not found in project %s", sessionID, projectID), "CHAT_SESSION_NOT_FOUND")
		return
	}

	reader, ok := h.store.(chatRunEventReader)
	if !ok {
		writeAPIError(w, http.StatusNotFound, fmt.Sprintf("event group %s not found", groupID), "CHAT_EVENT_GROUP_NOT_FOUND")
		return
	}
	events, err := reader.ListChatRunEvents(sessionID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "failed to list chat run events", "LIST_CHAT_RUN_EVENTS_FAILED")
		return
	}

	normalizedEvents := normalizeStoredChatRunEvents(events)
	_, groupMap := groupChatRunEventsForList(normalizedEvents)
	groupItems, ok := groupMap[groupID]
	if !ok {
		writeAPIError(w, http.StatusNotFound, fmt.Sprintf("event group %s not found", groupID), "CHAT_EVENT_GROUP_NOT_FOUND")
		return
	}

	writeJSON(w, http.StatusOK, getChatEventGroupResponse{
		SessionID: session.ID,
		ProjectID: session.ProjectID,
		GroupID:   groupID,
		Events:    groupItems,
	})
}

func parseChatSessionEventsPaginationParams(r *http.Request) (int, string, error) {
	limit := defaultChatSessionEventsLimit
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed <= 0 {
			return 0, "", fmt.Errorf("limit must be a positive integer")
		}
		if parsed > maxChatSessionEventsLimit {
			parsed = maxChatSessionEventsLimit
		}
		limit = parsed
	}
	return limit, strings.TrimSpace(r.URL.Query().Get("cursor")), nil
}

func paginateChatSessionTimeline(
	messages []core.ChatMessage,
	events []core.ChatRunEvent,
	cursor string,
	limit int,
) ([]core.ChatMessage, []core.ChatRunEvent, string, error) {
	timeline := buildChatSessionTimeline(messages, events)
	if len(timeline) == 0 {
		return []core.ChatMessage{}, []core.ChatRunEvent{}, "", nil
	}

	end := len(timeline)
	if cursor == "" {
		start := end - limit
		if start < 0 {
			start = 0
		}
		selected := timeline[start:end]
		pageMessages, pageEvents := splitChatSessionTimelinePage(messages, events, selected)
		return pageMessages, pageEvents, nextChatSessionCursor(timeline, start), nil
	}

	cursorIndex := -1
	for index, item := range timeline {
		if item.token == cursor {
			cursorIndex = index
			break
		}
	}
	if cursorIndex < 0 {
		return nil, nil, "", fmt.Errorf("cursor not found")
	}
	if cursorIndex == 0 {
		return []core.ChatMessage{}, []core.ChatRunEvent{}, "", nil
	}

	start := cursorIndex - limit
	if start < 0 {
		start = 0
	}
	selected := timeline[start:cursorIndex]
	pageMessages, pageEvents := splitChatSessionTimelinePage(messages, events, selected)
	return pageMessages, pageEvents, nextChatSessionCursor(timeline, start), nil
}

func buildChatSessionTimeline(messages []core.ChatMessage, events []core.ChatRunEvent) []chatSessionTimelineItem {
	timeline := make([]chatSessionTimelineItem, 0, len(messages)+len(events))
	messageIndex := 0
	eventIndex := 0
	for messageIndex < len(messages) && eventIndex < len(events) {
		messageTime := messages[messageIndex].Time.UTC()
		eventTime := events[eventIndex].CreatedAt.UTC()
		if messageTime.Before(eventTime) || messageTime.Equal(eventTime) {
			timeline = append(timeline, chatSessionTimelineItem{
				token:        fmt.Sprintf("m:%d", messageIndex),
				time:         messageTime,
				messageIndex: messageIndex,
				isMessage:    true,
			})
			messageIndex += 1
			continue
		}
		timeline = append(timeline, chatSessionTimelineItem{
			token:      fmt.Sprintf("e:%d", events[eventIndex].ID),
			time:       eventTime,
			eventIndex: eventIndex,
		})
		eventIndex += 1
	}
	for messageIndex < len(messages) {
		timeline = append(timeline, chatSessionTimelineItem{
			token:        fmt.Sprintf("m:%d", messageIndex),
			time:         messages[messageIndex].Time.UTC(),
			messageIndex: messageIndex,
			isMessage:    true,
		})
		messageIndex += 1
	}
	for eventIndex < len(events) {
		timeline = append(timeline, chatSessionTimelineItem{
			token:      fmt.Sprintf("e:%d", events[eventIndex].ID),
			time:       events[eventIndex].CreatedAt.UTC(),
			eventIndex: eventIndex,
		})
		eventIndex += 1
	}
	return timeline
}

func splitChatSessionTimelinePage(
	messages []core.ChatMessage,
	events []core.ChatRunEvent,
	selected []chatSessionTimelineItem,
) ([]core.ChatMessage, []core.ChatRunEvent) {
	pageMessages := make([]core.ChatMessage, 0)
	pageEvents := make([]core.ChatRunEvent, 0)
	for _, item := range selected {
		if item.isMessage {
			pageMessages = append(pageMessages, messages[item.messageIndex])
			continue
		}
		pageEvents = append(pageEvents, events[item.eventIndex])
	}
	return pageMessages, pageEvents
}

func nextChatSessionCursor(timeline []chatSessionTimelineItem, start int) string {
	if start <= 0 || start >= len(timeline) {
		return ""
	}
	return timeline[start].token
}

func normalizeStoredChatRunEvents(events []core.ChatRunEvent) []core.ChatRunEvent {
	if len(events) == 0 {
		return []core.ChatRunEvent{}
	}

	normalized := make([]core.ChatRunEvent, 0, len(events))
	var pending *core.ChatRunEvent
	pendingText := ""

	flushPending := func() {
		if pending == nil {
			return
		}
		pending.Payload = buildAggregatedChatRunEventPayload(pending.Payload, pending.UpdateType, pendingText)
		normalized = append(normalized, *pending)
		pending = nil
		pendingText = ""
	}

	for _, event := range events {
		aggregatedType := aggregatedStoredChatRunEventType(event.UpdateType)
		if aggregatedType == "" {
			flushPending()
			normalized = append(normalized, event)
			continue
		}

		text := extractStoredChatRunEventText(event.Payload)
		if text == "" {
			continue
		}
		if pending != nil && pending.SessionID == event.SessionID && pending.ProjectID == event.ProjectID && pending.UpdateType == aggregatedType {
			pending.ID = event.ID
			pending.CreatedAt = event.CreatedAt
			pendingText += text
			continue
		}

		flushPending()
		clone := event
		clone.UpdateType = aggregatedType
		pending = &clone
		pendingText = text
	}

	flushPending()
	return normalized
}

func groupChatRunEventsForList(events []core.ChatRunEvent) ([]core.ChatRunEvent, map[string][]core.ChatRunEvent) {
	if len(events) == 0 {
		return []core.ChatRunEvent{}, map[string][]core.ChatRunEvent{}
	}

	grouped := make([]core.ChatRunEvent, 0, len(events))
	groupMap := make(map[string][]core.ChatRunEvent)
	pending := make([]core.ChatRunEvent, 0)

	flushPending := func() {
		if len(pending) == 0 {
			return
		}
		if len(pending) == 1 {
			grouped = append(grouped, pending[0])
			pending = pending[:0]
			return
		}

		first := pending[0]
		last := pending[len(pending)-1]
		groupID := buildToolCallGroupID(first.ID, last.ID)
		groupMap[groupID] = append([]core.ChatRunEvent(nil), pending...)
		grouped = append(grouped, core.ChatRunEvent{
			ID:         last.ID,
			SessionID:  last.SessionID,
			ProjectID:  last.ProjectID,
			EventType:  last.EventType,
			UpdateType: "tool_call_group",
			Payload: map[string]any{
				"session_id": last.SessionID,
				"group_id":   groupID,
				"item_count": len(pending),
				"preview":    buildToolCallGroupPreview(pending),
			},
			CreatedAt: last.CreatedAt,
		})
		pending = pending[:0]
	}

	for _, event := range events {
		if isToolCallRunEventType(event.UpdateType) {
			pending = append(pending, event)
			continue
		}
		flushPending()
		grouped = append(grouped, event)
	}
	flushPending()
	return grouped, groupMap
}

func isToolCallRunEventType(updateType string) bool {
	switch strings.TrimSpace(updateType) {
	case "tool_call", "tool_call_update", "tool_call_completed":
		return true
	default:
		return false
	}
}

func buildToolCallGroupID(firstID, lastID int64) string {
	return fmt.Sprintf("tool-call-group:%d:%d", firstID, lastID)
}

func buildToolCallGroupPreview(events []core.ChatRunEvent) string {
	if len(events) == 0 {
		return ""
	}
	title := "工具调用"
	if payload := events[0].Payload; payload != nil {
		if acp := toStringAnyMap(payload["acp"]); acp != nil {
			if rawTitle, ok := acp["title"].(string); ok && strings.TrimSpace(rawTitle) != "" {
				title = strings.TrimSpace(rawTitle)
			}
		}
	}
	return fmt.Sprintf("%s 等 %d 个工具调用", title, len(events))
}

func aggregatedStoredChatRunEventType(updateType string) string {
	switch strings.TrimSpace(updateType) {
	case "agent_message_chunk", "assistant_message_chunk", "message_chunk":
		return "agent_message"
	case "agent_thought_chunk":
		return "agent_thought"
	case "user_message_chunk":
		return "user_message"
	default:
		return ""
	}
}

func buildAggregatedChatRunEventPayload(payload map[string]any, updateType string, text string) map[string]any {
	next := cloneJSONMap(payload)
	next["text"] = text
	acp := cloneJSONMap(toStringAnyMap(next["acp"]))
	acp["sessionUpdate"] = updateType
	acp["content"] = map[string]any{
		"type": "text",
		"text": text,
	}
	next["acp"] = acp
	return next
}

func cloneJSONMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		if nested, ok := value.(map[string]any); ok {
			out[key] = cloneJSONMap(nested)
			continue
		}
		out[key] = value
	}
	return out
}

func toStringAnyMap(value any) map[string]any {
	if value == nil {
		return nil
	}
	record, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	return record
}

func extractStoredChatRunEventText(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	if text, ok := payload["text"].(string); ok && strings.TrimSpace(text) != "" {
		return text
	}
	acp := toStringAnyMap(payload["acp"])
	if acp == nil {
		return ""
	}
	content := acp["content"]
	if text, ok := content.(string); ok && strings.TrimSpace(text) != "" {
		return text
	}
	contentMap := toStringAnyMap(content)
	if contentMap == nil {
		return ""
	}
	if text, ok := contentMap["text"].(string); ok {
		return text
	}
	return ""
}

func (h *chatHandlers) deleteSession(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "store is not configured", "STORE_UNAVAILABLE")
		return
	}

	projectID := strings.TrimSpace(chi.URLParam(r, "projectID"))
	sessionID := strings.TrimSpace(chi.URLParam(r, "sessionID"))
	if projectID == "" || sessionID == "" {
		writeAPIError(w, http.StatusBadRequest, "project id and session id are required", "INVALID_PATH_PARAM")
		return
	}

	session, err := h.store.GetChatSession(sessionID)
	if err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("chat session %s not found", sessionID), "CHAT_SESSION_NOT_FOUND")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to load chat session", "GET_CHAT_SESSION_FAILED")
		return
	}
	if session.ProjectID != projectID {
		writeAPIError(w, http.StatusNotFound, fmt.Sprintf("chat session %s not found in project %s", sessionID, projectID), "CHAT_SESSION_NOT_FOUND")
		return
	}

	deleter, ok := h.store.(chatSessionDeleter)
	if !ok {
		writeAPIError(w, http.StatusNotImplemented, "chat session delete is not supported by current store", "DELETE_CHAT_SESSION_UNSUPPORTED")
		return
	}
	if err := deleter.DeleteChatSession(sessionID); err != nil {
		if isNotFoundError(err) {
			writeAPIError(w, http.StatusNotFound, fmt.Sprintf("chat session %s not found", sessionID), "CHAT_SESSION_NOT_FOUND")
			return
		}
		writeAPIError(w, http.StatusInternalServerError, "failed to delete chat session", "DELETE_CHAT_SESSION_FAILED")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *chatHandlers) registerSessionRun(sessionID string, cancel context.CancelFunc) bool {
	trimmed := strings.TrimSpace(sessionID)
	if trimmed == "" || cancel == nil {
		return false
	}
	h.runMu.Lock()
	defer h.runMu.Unlock()
	if _, exists := h.activeRun[trimmed]; exists {
		return false
	}
	h.activeRun[trimmed] = cancel
	return true
}

func (h *chatHandlers) unregisterSessionRun(sessionID string) {
	trimmed := strings.TrimSpace(sessionID)
	if trimmed == "" {
		return
	}
	h.runMu.Lock()
	defer h.runMu.Unlock()
	delete(h.activeRun, trimmed)
}

func (h *chatHandlers) cancelSessionRun(sessionID string) bool {
	trimmed := strings.TrimSpace(sessionID)
	if trimmed == "" {
		return false
	}
	h.runMu.Lock()
	cancel, exists := h.activeRun[trimmed]
	h.runMu.Unlock()
	if !exists || cancel == nil {
		return false
	}
	cancel()
	return true
}

func (h *chatHandlers) publishChatEvent(
	eventType core.EventType,
	projectID string,
	sessionID string,
	role string,
	data map[string]string,
) {
	if h.publisher == nil {
		return
	}
	payload := map[string]string{
		"session_id": strings.TrimSpace(sessionID),
	}
	if trimmedRole := strings.TrimSpace(role); trimmedRole != "" {
		payload["role"] = trimmedRole
	}
	for key, value := range data {
		payload[key] = value
	}
	h.publisher.Publish(context.Background(), core.Event{
		Type:      eventType,
		ProjectID: strings.TrimSpace(projectID),
		Data:      payload,
		Timestamp: time.Now().UTC(),
	})
}
