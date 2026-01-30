package a2a

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	a2ago "github.com/a2aproject/a2a-go/a2a"
)

type Options struct {
	BasePath        string
	Logger          *slog.Logger
	ProtocolVersion string
	Name            string
	Description     string
	Store           *Store
}

type Handler struct {
	basePath        string
	logger          *slog.Logger
	protocolVersion string
	name            string
	description     string
	store           *Store
}

func NewHandler(opts Options) (*Handler, error) {
	if opts.Logger == nil {
		return nil, fmt.Errorf("Logger 不能为空")
	}
	basePath := strings.TrimSuffix(strings.TrimSpace(opts.BasePath), "/")
	if basePath == "" {
		basePath = "/a2a"
	}
	if !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}
	version := strings.TrimSpace(opts.ProtocolVersion)
	if version == "" {
		version = "1.0"
	}
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = "zhanggui A2A"
	}
	desc := strings.TrimSpace(opts.Description)
	if desc == "" {
		desc = "A2A HTTP+JSON/REST demo endpoint"
	}

	return &Handler{
		basePath:        basePath,
		logger:          opts.Logger.With("component", "a2a"),
		protocolVersion: version,
		name:            name,
		description:     desc,
		store:           pickStore(opts.Store),
	}, nil
}

func pickStore(store *Store) *Store {
	if store != nil {
		return store
	}
	return NewStore()
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/.well-known/agent-card.json", h.handleAgentCard)
	mux.HandleFunc(h.basePath+"/message:send", h.handleSendMessage)
	mux.HandleFunc(h.basePath+"/message:stream", h.handleStreamMessage)
	mux.HandleFunc(h.basePath+"/tasks", h.handleTasks)
	mux.HandleFunc(h.basePath+"/tasks/", h.handleTasks)
	mux.HandleFunc(h.basePath+"/extendedAgentCard", h.handleExtendedAgentCard)
}

func (h *Handler) handleAgentCard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	baseURL := baseURLFromRequest(r) + h.basePath
	card := map[string]any{
		"protocolVersions": []string{h.protocolVersion},
		"name":             h.name,
		"description":      h.description,
		"supportedInterfaces": []map[string]any{
			{"url": baseURL, "protocolBinding": "HTTP+JSON"},
		},
		"capabilities": map[string]any{
			"streaming":              true,
			"pushNotifications":      false,
			"stateTransitionHistory": false,
			"extendedAgentCard":      false,
		},
		"defaultInputModes":  []string{"application/a2a+json", "application/json", "text/plain"},
		"defaultOutputModes": []string{"application/a2a+json", "application/json", "text/plain"},
	}
	writeJSON(w, card)
}

func (h *Handler) handleExtendedAgentCard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeProblem(w, http.StatusBadRequest, "https://a2a-protocol.org/errors/unsupported-operation", "Unsupported Operation", "extended agent card not supported", map[string]any{
		"timestamp": nowTimestamp(),
	})
}

func (h *Handler) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		writeProblem(w, http.StatusUnsupportedMediaType, "https://a2a-protocol.org/errors/content-type-not-supported", "Content Type Not Supported", "unsupported Content-Type", map[string]any{
			"timestamp": nowTimestamp(),
		})
		return
	}

	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "about:blank", "Bad Request", "invalid json", map[string]any{
			"timestamp": nowTimestamp(),
		})
		return
	}
	if strings.TrimSpace(req.Message.MessageID) == "" || strings.TrimSpace(req.Message.Role) == "" || len(req.Message.Parts) == 0 {
		writeProblem(w, http.StatusBadRequest, "about:blank", "Bad Request", "missing messageId/role/parts", map[string]any{
			"timestamp": nowTimestamp(),
		})
		return
	}

	msg := toA2AMessage(req.Message)
	task := h.store.UpsertMessage(msg)
	if task == nil {
		writeProblem(w, http.StatusBadRequest, "about:blank", "Bad Request", "invalid message", map[string]any{
			"timestamp": nowTimestamp(),
		})
		return
	}
	now := time.Now().UTC()
	task.Status.State = a2ago.TaskStateCompleted
	task.Status.Timestamp = &now
	task.Artifacts = []*a2ago.Artifact{buildEchoArtifact(msg)}
	h.store.UpdateTask(task)

	resp := map[string]any{"task": fromA2ATask(task, true, nil)}
	writeJSON(w, resp)
}

func (h *Handler) handleStreamMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		writeProblem(w, http.StatusUnsupportedMediaType, "https://a2a-protocol.org/errors/content-type-not-supported", "Content Type Not Supported", "unsupported Content-Type", map[string]any{
			"timestamp": nowTimestamp(),
		})
		return
	}

	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "about:blank", "Bad Request", "invalid json", map[string]any{
			"timestamp": nowTimestamp(),
		})
		return
	}
	if strings.TrimSpace(req.Message.MessageID) == "" || strings.TrimSpace(req.Message.Role) == "" || len(req.Message.Parts) == 0 {
		writeProblem(w, http.StatusBadRequest, "about:blank", "Bad Request", "missing messageId/role/parts", map[string]any{
			"timestamp": nowTimestamp(),
		})
		return
	}

	msg := toA2AMessage(req.Message)
	task := h.store.UpsertMessage(msg)
	if task == nil {
		writeProblem(w, http.StatusBadRequest, "about:blank", "Bad Request", "invalid message", map[string]any{
			"timestamp": nowTimestamp(),
		})
		return
	}
	now := time.Now().UTC()
	task.Status.State = a2ago.TaskStateWorking
	task.Status.Timestamp = &now
	h.store.UpdateTask(task)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	taskResp := fromA2ATask(task, false, nil)
	_ = writeSSE(w, StreamResponse{Task: &taskResp})
	flusher.Flush()

	artifact := buildEchoArtifact(msg)
	task.Artifacts = []*a2ago.Artifact{artifact}
	finalTime := time.Now().UTC()
	task.Status.State = a2ago.TaskStateCompleted
	task.Status.Timestamp = &finalTime
	h.store.UpdateTask(task)

	_ = writeSSE(w, StreamResponse{ArtifactUpdate: &TaskArtifactUpdateEvent{
		TaskID:    string(task.ID),
		ContextID: task.ContextID,
		Artifact:  fromA2AArtifact(artifact),
	}})
	flusher.Flush()

	_ = writeSSE(w, StreamResponse{StatusUpdate: &TaskStatusUpdateEvent{
		TaskID:    string(task.ID),
		ContextID: task.ContextID,
		Status:    fromA2AStatus(task.Status),
		Final:     true,
	}})
	flusher.Flush()
}

func (h *Handler) handleTasks(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, h.basePath)
	if path == "/tasks" || path == "/tasks/" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleListTasks(w, r)
		return
	}

	taskPath := strings.TrimPrefix(path, "/tasks/")
	if taskPath == "" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	if strings.Contains(taskPath, "/pushNotificationConfigs") {
		writeProblem(w, http.StatusBadRequest, "https://a2a-protocol.org/errors/push-notification-not-supported", "Push Notification Not Supported", "push notifications are not supported", map[string]any{
			"timestamp": nowTimestamp(),
		})
		return
	}

	if strings.Contains(taskPath, ":") {
		id, action := splitAction(taskPath)
		switch action {
		case "cancel":
			h.handleCancelTask(w, r, id)
		case "subscribe":
			h.handleSubscribeTask(w, r, id)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
		return
	}

	h.handleGetTask(w, r, taskPath)
}

func (h *Handler) handleGetTask(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	task, ok := h.store.GetTask(id)
	if !ok {
		writeProblem(w, http.StatusNotFound, "https://a2a-protocol.org/errors/task-not-found", "Task Not Found", "task not found", map[string]any{
			"taskId":    id,
			"timestamp": nowTimestamp(),
		})
		return
	}
	includeArtifacts := parseBoolQuery(r, "includeArtifacts")
	historyLength := parseIntQuery(r, "historyLength")
	resp := map[string]any{"task": fromA2ATask(task, includeArtifacts, historyLength)}
	writeJSON(w, resp)
}

func (h *Handler) handleListTasks(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	pageSize := 50
	if v := query.Get("pageSize"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if n > 100 {
				n = 100
			}
			pageSize = n
		}
	}
	offset := parsePageToken(query.Get("pageToken"))
	tasks, total := h.store.ListTasks(offset, pageSize)

	contextFilter := strings.TrimSpace(query.Get("contextId"))
	statusFilter := strings.TrimSpace(query.Get("status"))
	statusAfter := strings.TrimSpace(query.Get("statusTimestampAfter"))
	includeArtifacts := parseBoolQuery(r, "includeArtifacts")
	historyLength := parseIntQuery(r, "historyLength")

	filtered := make([]Task, 0, len(tasks))
	statusWanted := toA2AState(statusFilter)
	for _, task := range tasks {
		if task == nil {
			continue
		}
		if contextFilter != "" && task.ContextID != contextFilter {
			continue
		}
		if statusFilter != "" && task.Status.State != statusWanted {
			continue
		}
		if statusAfter != "" && task.Status.Timestamp != nil {
			if after, err := time.Parse(time.RFC3339Nano, statusAfter); err == nil && task.Status.Timestamp.Before(after) {
				continue
			}
		}
		filtered = append(filtered, fromA2ATask(task, includeArtifacts, historyLength))
	}

	resp := ListTasksResponse{
		Tasks:         filtered,
		NextPageToken: nextPageToken(offset, pageSize, total),
		PageSize:      pageSize,
		TotalSize:     total,
	}
	writeJSON(w, resp)
}

func (h *Handler) handleCancelTask(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	task, ok := h.store.GetTask(id)
	if !ok {
		writeProblem(w, http.StatusNotFound, "https://a2a-protocol.org/errors/task-not-found", "Task Not Found", "task not found", map[string]any{
			"taskId":    id,
			"timestamp": nowTimestamp(),
		})
		return
	}
	if task.Status.State.Terminal() {
		writeProblem(w, http.StatusConflict, "https://a2a-protocol.org/errors/task-not-cancelable", "Task Not Cancelable", "task is already terminal", map[string]any{
			"taskId":    id,
			"timestamp": nowTimestamp(),
		})
		return
	}
	now := time.Now().UTC()
	task.Status.State = a2ago.TaskStateCanceled
	task.Status.Timestamp = &now
	h.store.UpdateTask(task)
	resp := map[string]any{"task": fromA2ATask(task, true, nil)}
	writeJSON(w, resp)
}

func (h *Handler) handleSubscribeTask(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	task, ok := h.store.GetTask(id)
	if !ok {
		writeProblem(w, http.StatusNotFound, "https://a2a-protocol.org/errors/task-not-found", "Task Not Found", "task not found", map[string]any{
			"taskId":    id,
			"timestamp": nowTimestamp(),
		})
		return
	}
	if task.Status.State.Terminal() {
		writeProblem(w, http.StatusBadRequest, "https://a2a-protocol.org/errors/unsupported-operation", "Unsupported Operation", "task already terminal", map[string]any{
			"taskId":    id,
			"timestamp": nowTimestamp(),
		})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	taskResp := fromA2ATask(task, false, nil)
	_ = writeSSE(w, StreamResponse{Task: &taskResp})
}

func baseURLFromRequest(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func isJSONContentType(ct string) bool {
	ct = strings.ToLower(strings.TrimSpace(ct))
	if ct == "" {
		return true
	}
	return strings.HasPrefix(ct, "application/json") || strings.HasPrefix(ct, "application/a2a+json")
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/a2a+json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeSSE(w io.Writer, resp StreamResponse) error {
	b, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", string(b))
	return err
}

func writeProblem(w http.ResponseWriter, status int, typ string, title string, detail string, extras map[string]any) {
	payload := map[string]any{
		"type":   typ,
		"title":  title,
		"status": status,
		"detail": detail,
	}
	for k, v := range extras {
		payload[k] = v
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func splitAction(path string) (string, string) {
	parts := strings.SplitN(path, ":", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

func parseBoolQuery(r *http.Request, key string) bool {
	v := strings.TrimSpace(r.URL.Query().Get(key))
	if v == "" {
		return false
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func parseIntQuery(r *http.Request, key string) *int {
	v := strings.TrimSpace(r.URL.Query().Get(key))
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return nil
	}
	return &n
}

func parsePageToken(token string) int {
	if token == "" {
		return 0
	}
	var offset int
	_, err := fmt.Sscanf(token, "offset:%d", &offset)
	if err != nil || offset < 0 {
		return 0
	}
	return offset
}

func nextPageToken(offset int, limit int, total int) string {
	next := offset + limit
	if next >= total {
		return ""
	}
	return fmt.Sprintf("offset:%d", next)
}
