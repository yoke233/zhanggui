package agui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yoke233/zhanggui/internal/gateway"
)

type Options struct {
	RunsDir   string
	BasePath  string
	Protocol  string
	Logger    *slog.Logger
	ActorID   string
	ActorRole string

	ThreadSink ThreadSink
}

type ThreadSink interface {
	EnsureThread(threadID string) error
	OnRunStarted(threadID string, runID string) error
	OnRunFinished(threadID string, runID string, outcome string) error
}

type Handler struct {
	runsDir  string
	basePath string
	protocol string
	logger   *slog.Logger

	actor gateway.Actor

	manager *Manager

	threadSink ThreadSink
}

func NewHandler(opts Options) (*Handler, error) {
	if strings.TrimSpace(opts.RunsDir) == "" {
		return nil, fmt.Errorf("RunsDir 不能为空")
	}
	if strings.TrimSpace(opts.BasePath) == "" {
		return nil, fmt.Errorf("BasePath 不能为空")
	}
	if strings.TrimSpace(opts.Protocol) == "" {
		return nil, fmt.Errorf("Protocol 不能为空")
	}
	if opts.Logger == nil {
		return nil, fmt.Errorf("Logger 不能为空")
	}

	basePath := strings.TrimSuffix(strings.TrimSpace(opts.BasePath), "/")
	if !strings.HasPrefix(basePath, "/") {
		basePath = "/" + basePath
	}

	actorID := strings.TrimSpace(opts.ActorID)
	if actorID == "" {
		actorID = "zhanggui"
	}
	actorRole := strings.TrimSpace(opts.ActorRole)
	if actorRole == "" {
		actorRole = "system"
	}

	return &Handler{
		runsDir:    filepath.Clean(opts.RunsDir),
		basePath:   basePath,
		protocol:   opts.Protocol,
		logger:     opts.Logger.With("component", "agui"),
		actor:      gateway.Actor{AgentID: actorID, Role: actorRole},
		manager:    NewManager(),
		threadSink: opts.ThreadSink,
	}, nil
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", h.handleHealthz)
	mux.HandleFunc(h.basePath+"/run", h.handleRun)
	mux.HandleFunc(h.basePath+"/tool_result", h.handleToolResult)
}

func (h *Handler) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, "ok\n")
}

func (h *Handler) handleToolResult(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var raw map[string]any
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	runID := firstString(raw, "runId", "run_id")
	threadID := firstString(raw, "threadId", "thread_id")
	toolCallID := firstString(raw, "toolCallId", "tool_call_id")
	if strings.TrimSpace(runID) == "" || strings.TrimSpace(toolCallID) == "" {
		http.Error(w, "missing runId/toolCallId", http.StatusBadRequest)
		return
	}

	content, _ := raw["content"]

	err := h.manager.DeliverToolResult(runID, ToolResult{
		ThreadID:   threadID,
		RunID:      runID,
		ToolCallID: toolCallID,
		Content:    content,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (h *Handler) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var raw map[string]any
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}

	req := parseRunRequest(raw)
	if strings.TrimSpace(req.ThreadID) == "" {
		req.ThreadID = newThreadID(time.Now())
	}
	if strings.TrimSpace(req.RunID) == "" {
		req.RunID = newRunID(time.Now())
	}

	if err := validateID(req.ThreadID); err != nil {
		http.Error(w, fmt.Sprintf("bad threadId: %v", err), http.StatusBadRequest)
		return
	}
	if err := validateID(req.RunID); err != nil {
		http.Error(w, fmt.Sprintf("bad runId: %v", err), http.StatusBadRequest)
		return
	}

	runRoot := filepath.Join(h.runsDir, req.RunID)
	if _, err := os.Stat(runRoot); err == nil {
		http.Error(w, "run already exists", http.StatusConflict)
		return
	}

	if err := os.MkdirAll(filepath.Join(runRoot, "events"), 0o755); err != nil {
		http.Error(w, "mkdir failed", http.StatusInternalServerError)
		return
	}
	if err := os.MkdirAll(filepath.Join(runRoot, "logs"), 0o755); err != nil {
		http.Error(w, "mkdir failed", http.StatusInternalServerError)
		return
	}

	auditor, err := gateway.NewAuditor(filepath.Join(runRoot, "logs", "tool_audit.jsonl"))
	if err != nil {
		http.Error(w, "audit init failed", http.StatusInternalServerError)
		return
	}
	defer func() { _ = auditor.Close() }()

	gw, err := gateway.New(runRoot, h.actor, gateway.Linkage{ThreadID: req.ThreadID, RunID: req.RunID}, gateway.Policy{
		AllowedWritePrefixes: []string{"run.json", "state.json", "events/", "logs/"},
		AppendOnlyFiles:      []string{"events/events.jsonl"},
	}, auditor)
	if err != nil {
		http.Error(w, "gateway init failed", http.StatusInternalServerError)
		return
	}

	stream, err := NewSSEStream(w)
	if err != nil {
		http.Error(w, "sse init failed", http.StatusInternalServerError)
		return
	}

	evlog := &EventLog{
		Gateway:      gw,
		EventsRel:    "events/events.jsonl",
		AppendPerm:   0o644,
		AppendDetail: "events: append",
	}
	emitter := &Emitter{
		Stream:   stream,
		EventLog: evlog,
		ThreadID: req.ThreadID,
		RunID:    req.RunID,
	}

	session, release := h.manager.Start(req.RunID)
	defer release()

	meta := RunMeta{
		SchemaVersion: 1,
		Protocol:      h.protocol,
		ThreadID:      req.ThreadID,
		RunID:         req.RunID,
		CreatedAt:     time.Now().Format(time.RFC3339),
	}
	if req.Resume != nil && strings.TrimSpace(req.Resume.InterruptID) != "" {
		if parentRunID, _, ferr := findInterruptedRun(h.runsDir, req.Resume.InterruptID); ferr == nil {
			meta.ParentRunID = parentRunID
		}
	}
	meta.Input = normalizeRunInput(raw, req)
	if err := writeRunMeta(gw, meta); err != nil {
		_ = emitter.Emit(Event{
			"type":      "RUN_ERROR",
			"message":   err.Error(),
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	st := RunState{
		SchemaVersion: 1,
		Protocol:      h.protocol,
		ThreadID:      req.ThreadID,
		RunID:         req.RunID,
		Status:        "RUNNING",
		UpdatedAt:     time.Now().Format(time.RFC3339),
	}

	if err := writeRunState(gw, st); err != nil {
		_ = emitter.Emit(Event{
			"type":      "RUN_ERROR",
			"message":   err.Error(),
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	started := false
	var runErr error
	defer func() {
		if h.threadSink == nil || !started {
			return
		}
		outcome := "success"
		if runErr != nil {
			outcome = "error"
		} else if st.Status == "INTERRUPTED" {
			outcome = "interrupt"
		}
		_ = h.threadSink.OnRunFinished(req.ThreadID, req.RunID, outcome)
	}()

	if h.threadSink != nil {
		if err := h.threadSink.EnsureThread(req.ThreadID); err != nil {
			_ = emitter.Emit(Event{
				"type":      "RUN_ERROR",
				"message":   err.Error(),
				"timestamp": time.Now().Format(time.RFC3339),
			})
			return
		}
		if err := h.threadSink.OnRunStarted(req.ThreadID, req.RunID); err != nil {
			_ = emitter.Emit(Event{
				"type":      "RUN_ERROR",
				"message":   err.Error(),
				"timestamp": time.Now().Format(time.RFC3339),
			})
			return
		}
	}
	started = true

	runStarted := Event{
		"type":      "RUN_STARTED",
		"threadId":  req.ThreadID,
		"runId":     req.RunID,
		"timestamp": time.Now().Format(time.RFC3339),
		"input":     meta.Input,
	}
	if strings.TrimSpace(meta.ParentRunID) != "" {
		runStarted["parentRunId"] = meta.ParentRunID
	}
	if err := emitter.Emit(runStarted); err != nil {
		runErr = err
		return
	}
	if err := emitter.Emit(Event{"type": "MESSAGES_SNAPSHOT", "messages": []any{}}); err != nil {
		runErr = err
		return
	}
	if err := emitter.Emit(Event{"type": "STATE_SNAPSHOT", "snapshot": st}); err != nil {
		runErr = err
		return
	}

	if req.Resume != nil && strings.TrimSpace(req.Resume.InterruptID) != "" {
		runErr = h.runResume(r.Context(), gw, emitter, &st, req, session)
	} else {
		runErr = h.runNew(r.Context(), gw, emitter, &st, req, session)
	}

	if runErr != nil {
		h.logger.Error("run failed", "run_id", req.RunID, "err", runErr)
		st.Status = "FAILED"
		st.LastError = &ErrorInfo{Message: runErr.Error()}
		st.UpdatedAt = time.Now().Format(time.RFC3339)
		_ = writeRunState(gw, st)

		var de gateway.DenyError
		if errors.As(runErr, &de) {
			_ = emitter.Emit(Event{
				"type":      "RUN_ERROR",
				"message":   de.Message,
				"code":      de.Code,
				"timestamp": time.Now().Format(time.RFC3339),
			})
			return
		}

		_ = emitter.Emit(Event{
			"type":      "RUN_ERROR",
			"message":   runErr.Error(),
			"timestamp": time.Now().Format(time.RFC3339),
		})
	}
}
