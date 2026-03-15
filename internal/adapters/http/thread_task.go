package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/yoke233/ai-workflow/internal/application/threadtaskapp"
	"github.com/yoke233/ai-workflow/internal/core"
)

// ---------------------------------------------------------------------------
// Request / response types
// ---------------------------------------------------------------------------

type createTaskGroupRequest struct {
	Tasks            []createTaskRequest `json:"tasks"`
	SourceMessageID  *int64              `json:"source_message_id,omitempty"`
	NotifyOnComplete *bool               `json:"notify_on_complete,omitempty"`
}

type createTaskRequest struct {
	Assignee       string `json:"assignee"`
	Type           string `json:"type,omitempty"`
	Instruction    string `json:"instruction"`
	DependsOnIndex []int  `json:"depends_on_index,omitempty"`
	MaxRetries     *int   `json:"max_retries,omitempty"`
	OutputFileName string `json:"output_file_name,omitempty"`
}

type taskSignalRequest struct {
	Action         string `json:"action"`
	OutputFilePath string `json:"output_file_path,omitempty"`
	Feedback       string `json:"feedback,omitempty"`
}

// ---------------------------------------------------------------------------
// Route registration
// ---------------------------------------------------------------------------

func registerThreadTaskRoutes(r chi.Router, h *Handler) {
	r.Post("/threads/{threadID}/task-groups", h.createThreadTaskGroup)
	r.Get("/threads/{threadID}/task-groups", h.listThreadTaskGroups)
	r.Get("/task-groups/{groupID}", h.getThreadTaskGroup)
	r.Delete("/task-groups/{groupID}", h.deleteThreadTaskGroup)
	r.Post("/thread-tasks/{taskID}/signal", h.signalThreadTask)
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func (h *Handler) createThreadTaskGroup(w http.ResponseWriter, r *http.Request) {
	threadID, ok := urlParamInt64(r, "threadID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid thread ID", "BAD_ID")
		return
	}

	var req createTaskGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}

	if len(req.Tasks) == 0 {
		writeError(w, http.StatusBadRequest, "at least one task is required", threadtaskapp.CodeMissingTasks)
		return
	}

	notifyOnComplete := true
	if req.NotifyOnComplete != nil {
		notifyOnComplete = *req.NotifyOnComplete
	}

	taskInputs := make([]threadtaskapp.CreateTaskInput, len(req.Tasks))
	for i, t := range req.Tasks {
		taskInputs[i] = threadtaskapp.CreateTaskInput{
			Assignee:       t.Assignee,
			Type:           t.Type,
			Instruction:    t.Instruction,
			DependsOnIndex: t.DependsOnIndex,
			MaxRetries:     t.MaxRetries,
			OutputFileName: t.OutputFileName,
		}
	}

	svc := h.threadTaskService()
	result, err := svc.CreateTaskGroup(r.Context(), threadtaskapp.CreateTaskGroupInput{
		ThreadID:         threadID,
		SourceMessageID:  req.SourceMessageID,
		NotifyOnComplete: notifyOnComplete,
		Tasks:            taskInputs,
	})
	if err != nil {
		writeThreadTaskAppFailure(w, err, "CREATE_TASK_GROUP_FAILED")
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *Handler) listThreadTaskGroups(w http.ResponseWriter, r *http.Request) {
	threadID, ok := urlParamInt64(r, "threadID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid thread ID", "BAD_ID")
		return
	}

	groups, err := h.store.ListThreadTaskGroups(r.Context(), core.ThreadTaskGroupFilter{
		ThreadID: &threadID,
		Limit:    queryInt(r, "limit", 50),
		Offset:   queryInt(r, "offset", 0),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	if groups == nil {
		groups = []*core.ThreadTaskGroup{}
	}
	writeJSON(w, http.StatusOK, groups)
}

func (h *Handler) getThreadTaskGroup(w http.ResponseWriter, r *http.Request) {
	groupID, ok := urlParamInt64(r, "groupID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid group ID", "BAD_ID")
		return
	}

	svc := h.threadTaskService()
	detail, err := svc.GetGroupDetail(r.Context(), groupID)
	if err != nil {
		writeThreadTaskAppFailure(w, err, "GET_TASK_GROUP_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (h *Handler) deleteThreadTaskGroup(w http.ResponseWriter, r *http.Request) {
	groupID, ok := urlParamInt64(r, "groupID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid group ID", "BAD_ID")
		return
	}

	if err := h.store.DeleteThreadTaskGroup(r.Context(), groupID); err != nil {
		if err == core.ErrNotFound {
			writeError(w, http.StatusNotFound, "task group not found", threadtaskapp.CodeGroupNotFound)
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error(), "STORE_ERROR")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) signalThreadTask(w http.ResponseWriter, r *http.Request) {
	taskID, ok := urlParamInt64(r, "taskID")
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid task ID", "BAD_ID")
		return
	}

	var req taskSignalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "BAD_REQUEST")
		return
	}

	action := strings.TrimSpace(req.Action)
	if action == "" {
		writeError(w, http.StatusBadRequest, "action is required", threadtaskapp.CodeInvalidAction)
		return
	}

	svc := h.threadTaskService()
	if err := svc.Signal(r.Context(), threadtaskapp.SignalInput{
		TaskID:         taskID,
		Action:         action,
		OutputFilePath: strings.TrimSpace(req.OutputFilePath),
		Feedback:       strings.TrimSpace(req.Feedback),
	}); err != nil {
		writeThreadTaskAppFailure(w, err, "SIGNAL_TASK_FAILED")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ---------------------------------------------------------------------------
// Service factory & error mapping
// ---------------------------------------------------------------------------

func (h *Handler) threadTaskService() *threadtaskapp.Service {
	if h == nil {
		return nil
	}
	var notifier threadtaskapp.NotificationSender
	if ns, ok := h.store.(threadtaskapp.NotificationSender); ok {
		notifier = ns
	}
	var agentPool threadtaskapp.AgentDispatcher
	if h.threadPool != nil {
		agentPool = h.threadPool
	}
	return threadtaskapp.New(threadtaskapp.Config{
		Store:     h.store,
		Bus:       h.bus,
		Notifier:  notifier,
		AgentPool: agentPool,
	})
}

func writeThreadTaskAppError(w http.ResponseWriter, err error) bool {
	switch threadtaskapp.CodeOf(err) {
	case threadtaskapp.CodeThreadNotFound:
		writeError(w, http.StatusNotFound, "thread not found", threadtaskapp.CodeThreadNotFound)
	case threadtaskapp.CodeGroupNotFound:
		writeError(w, http.StatusNotFound, "task group not found", threadtaskapp.CodeGroupNotFound)
	case threadtaskapp.CodeTaskNotFound:
		writeError(w, http.StatusNotFound, "task not found", threadtaskapp.CodeTaskNotFound)
	case threadtaskapp.CodeMissingThreadID:
		writeError(w, http.StatusBadRequest, "thread_id is required", threadtaskapp.CodeMissingThreadID)
	case threadtaskapp.CodeMissingTasks:
		writeError(w, http.StatusBadRequest, "at least one task is required", threadtaskapp.CodeMissingTasks)
	case threadtaskapp.CodeMissingAssignee:
		writeError(w, http.StatusBadRequest, err.Error(), threadtaskapp.CodeMissingAssignee)
	case threadtaskapp.CodeMissingInstruction:
		writeError(w, http.StatusBadRequest, err.Error(), threadtaskapp.CodeMissingInstruction)
	case threadtaskapp.CodeInvalidTaskType:
		writeError(w, http.StatusBadRequest, err.Error(), threadtaskapp.CodeInvalidTaskType)
	case threadtaskapp.CodeInvalidAction:
		writeError(w, http.StatusBadRequest, err.Error(), threadtaskapp.CodeInvalidAction)
	case threadtaskapp.CodeInvalidState:
		writeError(w, http.StatusConflict, err.Error(), threadtaskapp.CodeInvalidState)
	case threadtaskapp.CodeInvalidDependency:
		writeError(w, http.StatusBadRequest, err.Error(), threadtaskapp.CodeInvalidDependency)
	case threadtaskapp.CodeDependencyCycle:
		writeError(w, http.StatusBadRequest, err.Error(), threadtaskapp.CodeDependencyCycle)
	case threadtaskapp.CodeRetryExhausted:
		writeError(w, http.StatusConflict, err.Error(), threadtaskapp.CodeRetryExhausted)
	default:
		return false
	}
	return true
}

func writeThreadTaskAppFailure(w http.ResponseWriter, err error, fallbackCode string) {
	if writeThreadTaskAppError(w, err) {
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error(), fallbackCode)
}
