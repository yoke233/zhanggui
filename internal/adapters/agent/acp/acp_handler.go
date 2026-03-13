package acphandler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
	"github.com/yoke233/ai-workflow/internal/legacy/core"
)

type acpEventPublisher interface {
	Publish(ctx context.Context, evt core.Event) error
}

type ChatRunEventRecorder interface {
	AppendChatRunEvent(event core.ChatRunEvent) error
}

type SessionStateCallback func(commands []acpproto.AvailableCommand, configOptions []acpproto.SessionConfigOptionSelect)

type ACPHandlerSessionContext struct {
	SessionID    string
	ChangedFiles []string
}

type ACPHandler struct {
	acpclient.NopHandler

	cwd              string
	sessionID        string
	chatSessionID    string
	projectID        string
	permissionPolicy []acpclient.PermissionRule
	publisher        acpEventPublisher
	recorder         ChatRunEventRecorder

	mu             sync.Mutex
	changedSet     map[string]struct{}
	changedList    []string
	suppressEvents bool
	stateCallback  SessionStateCallback

	runEventMu        sync.Mutex
	pendingChunkEvent *pendingChatRunChunkEvent

	terminalSeq atomic.Uint64
	terminalMu  sync.Mutex
	terminals   map[string]*acpTerminalState
}

type pendingChatRunChunkEvent struct {
	updateType     string
	sessionID      string
	projectID      string
	agentSessionID string
	text           string
	createdAt      time.Time
}

type acpTerminalState struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	outbuf   *lockedBuffer
	done     chan struct{}
	waitErr  error
	exitCode *int
	signal   *string
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) Snapshot(maxBytes int) (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	data := b.buf.Bytes()
	if maxBytes > 0 && len(data) > maxBytes {
		return string(data[len(data)-maxBytes:]), true
	}
	return string(data), false
}

var _ acpproto.Client = (*ACPHandler)(nil)
var _ acpclient.EventHandler = (*ACPHandler)(nil)

func NewACPHandler(cwd string, sessionID string, publisher acpEventPublisher) *ACPHandler {
	return &ACPHandler{
		cwd:        strings.TrimSpace(cwd),
		sessionID:  strings.TrimSpace(sessionID),
		publisher:  publisher,
		changedSet: make(map[string]struct{}),
		terminals:  make(map[string]*acpTerminalState),
	}
}

func (h *ACPHandler) SetRunEventRecorder(recorder ChatRunEventRecorder) {
	if h == nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.recorder = recorder
}

func (h *ACPHandler) SetSuppressEvents(suppress bool) {
	if h == nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.suppressEvents = suppress
}

func (h *ACPHandler) SetSessionStateCallback(cb SessionStateCallback) {
	if h == nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.stateCallback = cb
}

func (h *ACPHandler) SetSessionID(sessionID string) {
	if h == nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.sessionID = strings.TrimSpace(sessionID)
}

func (h *ACPHandler) SetProjectID(projectID string) {
	if h == nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.projectID = strings.TrimSpace(projectID)
}

func (h *ACPHandler) SetChatSessionID(chatSessionID string) {
	if h == nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.chatSessionID = strings.TrimSpace(chatSessionID)
}

func (h *ACPHandler) SetPermissionPolicy(policy []acpclient.PermissionRule) {
	if h == nil {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if len(policy) == 0 {
		h.permissionPolicy = nil
		return
	}
	h.permissionPolicy = append([]acpclient.PermissionRule(nil), policy...)
}

func (h *ACPHandler) ReadTextFile(_ context.Context, req acpproto.ReadTextFileRequest) (acpproto.ReadTextFileResponse, error) {
	if h == nil {
		return acpproto.ReadTextFileResponse{}, errors.New("acp handler is nil")
	}

	targetPath, _, err := h.normalizePathInScope(req.Path)
	if err != nil {
		return acpproto.ReadTextFileResponse{}, err
	}
	raw, err := os.ReadFile(targetPath)
	if err != nil {
		return acpproto.ReadTextFileResponse{}, fmt.Errorf("read file: %w", err)
	}

	content := string(raw)
	content = applyReadLineWindow(content, req.Line, req.Limit)
	return acpproto.ReadTextFileResponse{Content: content}, nil
}

func (h *ACPHandler) RequestPermission(_ context.Context, req acpproto.RequestPermissionRequest) (acpproto.RequestPermissionResponse, error) {
	if h == nil {
		return acpproto.RequestPermissionResponse{}, errors.New("acp handler is nil")
	}
	decisionAction := h.resolvePermissionPolicy(req)
	if selected := selectPermissionOptionID(req.Options, decisionAction); selected != "" {
		return acpproto.RequestPermissionResponse{
			Outcome: acpproto.RequestPermissionOutcome{
				Selected: &acpproto.RequestPermissionOutcomeSelected{
					Outcome:  "selected",
					OptionId: acpproto.PermissionOptionId(selected),
				},
			},
		}, nil
	}
	return acpproto.RequestPermissionResponse{
		Outcome: acpproto.RequestPermissionOutcome{
			Cancelled: &acpproto.RequestPermissionOutcomeCancelled{Outcome: "cancelled"},
		},
	}, nil
}

func (h *ACPHandler) CreateTerminal(_ context.Context, req acpproto.CreateTerminalRequest) (acpproto.CreateTerminalResponse, error) {
	if h == nil {
		return acpproto.CreateTerminalResponse{}, errors.New("acp handler is nil")
	}
	command := strings.TrimSpace(req.Command)
	if command == "" {
		return acpproto.CreateTerminalResponse{}, errors.New("terminal command is required")
	}

	cwd, err := h.normalizeDirInScope(stringPtrValue(req.Cwd))
	if err != nil {
		return acpproto.CreateTerminalResponse{}, err
	}

	commandParts := append([]string{command}, req.Args...)
	cmd := exec.Command(commandParts[0], commandParts[1:]...)
	cmd.Dir = cwd
	cmd.Env = mergeTerminalEnv(req.Env)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return acpproto.CreateTerminalResponse{}, fmt.Errorf("create terminal stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return acpproto.CreateTerminalResponse{}, fmt.Errorf("create terminal stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return acpproto.CreateTerminalResponse{}, fmt.Errorf("create terminal stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return acpproto.CreateTerminalResponse{}, fmt.Errorf("start terminal command: %w", err)
	}

	terminalID := fmt.Sprintf("term-%d", h.terminalSeq.Add(1))
	state := &acpTerminalState{
		cmd:    cmd,
		stdin:  stdin,
		outbuf: &lockedBuffer{},
		done:   make(chan struct{}),
	}

	go func() {
		_, _ = io.Copy(state.outbuf, stdout)
	}()
	go func() {
		_, _ = io.Copy(state.outbuf, stderr)
	}()
	go func() {
		waitErr := cmd.Wait()
		if cmd.ProcessState != nil {
			code := cmd.ProcessState.ExitCode()
			state.exitCode = &code
		}
		state.waitErr = waitErr
		close(state.done)
	}()

	h.terminalMu.Lock()
	h.terminals[terminalID] = state
	h.terminalMu.Unlock()
	return acpproto.CreateTerminalResponse{TerminalId: terminalID}, nil
}

func (h *ACPHandler) KillTerminalCommand(_ context.Context, req acpproto.KillTerminalCommandRequest) (acpproto.KillTerminalCommandResponse, error) {
	state, ok := h.getTerminal(req.TerminalId)
	if !ok {
		return acpproto.KillTerminalCommandResponse{}, nil
	}
	if state.cmd == nil || state.cmd.Process == nil {
		return acpproto.KillTerminalCommandResponse{}, nil
	}
	if err := state.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return acpproto.KillTerminalCommandResponse{}, fmt.Errorf("kill terminal process: %w", err)
	}
	return acpproto.KillTerminalCommandResponse{}, nil
}

func (h *ACPHandler) TerminalOutput(_ context.Context, req acpproto.TerminalOutputRequest) (acpproto.TerminalOutputResponse, error) {
	state, ok := h.getTerminal(req.TerminalId)
	if !ok {
		return acpproto.TerminalOutputResponse{}, fmt.Errorf("terminal %q not found", req.TerminalId)
	}
	output, truncated := state.outbuf.Snapshot(0)
	return acpproto.TerminalOutputResponse{
		Output:    output,
		Truncated: truncated,
	}, nil
}

func (h *ACPHandler) ReleaseTerminal(_ context.Context, req acpproto.ReleaseTerminalRequest) (acpproto.ReleaseTerminalResponse, error) {
	state, ok := h.removeTerminal(req.TerminalId)
	if !ok {
		return acpproto.ReleaseTerminalResponse{}, nil
	}
	if state.stdin != nil {
		_ = state.stdin.Close()
	}
	return acpproto.ReleaseTerminalResponse{}, nil
}

func (h *ACPHandler) WaitForTerminalExit(ctx context.Context, req acpproto.WaitForTerminalExitRequest) (acpproto.WaitForTerminalExitResponse, error) {
	state, ok := h.getTerminal(req.TerminalId)
	if !ok {
		return acpproto.WaitForTerminalExitResponse{}, fmt.Errorf("terminal %q not found", req.TerminalId)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-state.done:
		return acpproto.WaitForTerminalExitResponse{
			ExitCode: state.exitCode,
			Signal:   state.signal,
		}, nil
	case <-ctx.Done():
		return acpproto.WaitForTerminalExitResponse{}, ctx.Err()
	}
}

func (h *ACPHandler) WriteTextFile(_ context.Context, req acpproto.WriteTextFileRequest) (acpproto.WriteTextFileResponse, error) {
	if h == nil {
		return acpproto.WriteTextFileResponse{}, errors.New("acp handler is nil")
	}

	targetPath, relPath, err := h.normalizePathInScope(req.Path)
	if err != nil {
		return acpproto.WriteTextFileResponse{}, err
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return acpproto.WriteTextFileResponse{}, fmt.Errorf("ensure parent dir: %w", err)
	}

	content := []byte(req.Content)
	if err := os.WriteFile(targetPath, content, 0o644); err != nil {
		return acpproto.WriteTextFileResponse{}, fmt.Errorf("write file %q: %w", relPath, err)
	}

	filePaths := h.recordChangedFile(relPath)
	h.publishFilesChanged(filePaths)

	return acpproto.WriteTextFileResponse{}, nil
}

func (h *ACPHandler) SessionUpdate(context.Context, acpproto.SessionNotification) error {
	return nil
}

func (h *ACPHandler) SessionContext() ACPHandlerSessionContext {
	if h == nil {
		return ACPHandlerSessionContext{}
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	changed := make([]string, len(h.changedList))
	copy(changed, h.changedList)
	sessionID := h.sessionID
	if trimmedChatSessionID := strings.TrimSpace(h.chatSessionID); trimmedChatSessionID != "" {
		sessionID = trimmedChatSessionID
	}
	return ACPHandlerSessionContext{
		SessionID:    sessionID,
		ChangedFiles: changed,
	}
}

func (h *ACPHandler) HandleSessionUpdate(ctx context.Context, update acpclient.SessionUpdate) error {
	if h == nil {
		return nil
	}

	h.mu.Lock()
	projectID := strings.TrimSpace(h.projectID)
	chatSessionID := strings.TrimSpace(h.chatSessionID)
	agentSessionID := strings.TrimSpace(h.sessionID)
	recorder := h.recorder
	suppress := h.suppressEvents
	stateCallback := h.stateCallback
	h.mu.Unlock()
	if chatSessionID == "" {
		chatSessionID = agentSessionID
	}
	if agentSessionID == "" {
		agentSessionID = strings.TrimSpace(update.SessionID)
	}

	switch update.Type {
	case "available_commands_update":
		if stateCallback != nil {
			commands := append([]acpproto.AvailableCommand(nil), update.Commands...)
			if commands == nil {
				commands = []acpproto.AvailableCommand{}
			}
			stateCallback(commands, nil)
		}
	case "config_option_update":
		if stateCallback != nil {
			configOptions := append([]acpproto.SessionConfigOptionSelect(nil), update.ConfigOptions...)
			if configOptions == nil {
				configOptions = []acpproto.SessionConfigOptionSelect{}
			}
			stateCallback(nil, configOptions)
		}
	}
	if suppress {
		return nil
	}

	data := map[string]string{
		"session_id":       chatSessionID,
		"agent_session_id": agentSessionID,
	}
	if rawUpdate := strings.TrimSpace(string(update.RawJSON)); rawUpdate != "" {
		data["acp_update_json"] = rawUpdate
	}

	updateType := strings.TrimSpace(update.Type)
	if recorder != nil && chatSessionID != "" && projectID != "" {
		if isAggregatedACPChunkUpdateType(updateType) {
			if err := h.appendPendingChatRunChunk(projectID, chatSessionID, agentSessionID, update); err != nil {
				log.Printf("[acp] aggregate chat run chunk failed project_id=%s session_id=%s update_type=%s err=%v", projectID, chatSessionID, updateType, err)
			}
		} else {
			if err := h.FlushPendingChatRunEvents(); err != nil {
				log.Printf("[acp] flush pending chat run chunks failed project_id=%s session_id=%s err=%v", projectID, chatSessionID, err)
			}
			if !isACPChunkUpdateType(updateType) {
				payload := map[string]any{
					"session_id":       chatSessionID,
					"agent_session_id": agentSessionID,
				}
				if text := strings.TrimSpace(update.Text); text != "" {
					payload["text"] = text
				}
				if status := strings.TrimSpace(update.Status); status != "" {
					payload["status"] = status
				}
				if rawUpdate := strings.TrimSpace(string(update.RawJSON)); rawUpdate != "" {
					var acpPayload any
					if err := json.Unmarshal(update.RawJSON, &acpPayload); err == nil {
						payload["acp"] = acpPayload
					} else {
						payload["acp_raw"] = rawUpdate
					}
				}
				if err := recorder.AppendChatRunEvent(core.ChatRunEvent{
					SessionID:  chatSessionID,
					ProjectID:  projectID,
					EventType:  string(core.EventRunUpdate),
					UpdateType: updateType,
					Payload:    payload,
					CreatedAt:  time.Now().UTC(),
				}); err != nil {
					log.Printf("[acp] persist chat run event failed project_id=%s session_id=%s update_type=%s err=%v", projectID, chatSessionID, updateType, err)
				}
			}
		}
	}

	if h.publisher != nil {
		h.publisher.Publish(ctx, core.Event{
			Type:      core.EventRunUpdate,
			ProjectID: projectID,
			Data:      data,
			Timestamp: time.Now(),
		})
	}
	return nil
}

func (h *ACPHandler) FlushPendingChatRunEvents() error {
	if h == nil {
		return nil
	}

	h.mu.Lock()
	recorder := h.recorder
	h.mu.Unlock()
	if recorder == nil {
		return nil
	}

	h.runEventMu.Lock()
	defer h.runEventMu.Unlock()
	return h.flushPendingChatRunEventsLocked(recorder)
}

func (h *ACPHandler) appendPendingChatRunChunk(
	projectID string,
	chatSessionID string,
	agentSessionID string,
	update acpclient.SessionUpdate,
) error {
	if h == nil {
		return nil
	}

	h.mu.Lock()
	recorder := h.recorder
	h.mu.Unlock()
	if recorder == nil {
		return nil
	}

	chunkText := update.Text
	if chunkText == "" {
		chunkText = extractACPChunkText(string(update.RawJSON))
	}
	if chunkText == "" {
		return nil
	}

	updateType := strings.TrimSpace(update.Type)
	now := time.Now().UTC()

	h.runEventMu.Lock()
	defer h.runEventMu.Unlock()

	if h.pendingChunkEvent != nil && (h.pendingChunkEvent.updateType != updateType ||
		h.pendingChunkEvent.sessionID != chatSessionID ||
		h.pendingChunkEvent.projectID != projectID ||
		h.pendingChunkEvent.agentSessionID != agentSessionID) {
		if err := h.flushPendingChatRunEventsLocked(recorder); err != nil {
			return err
		}
	}
	if h.pendingChunkEvent == nil {
		h.pendingChunkEvent = &pendingChatRunChunkEvent{
			updateType:     updateType,
			sessionID:      chatSessionID,
			projectID:      projectID,
			agentSessionID: agentSessionID,
			createdAt:      now,
		}
	}
	h.pendingChunkEvent.text += chunkText
	h.pendingChunkEvent.createdAt = now
	return nil
}

func (h *ACPHandler) flushPendingChatRunEventsLocked(recorder ChatRunEventRecorder) error {
	if h == nil || recorder == nil || h.pendingChunkEvent == nil {
		return nil
	}

	pending := h.pendingChunkEvent
	h.pendingChunkEvent = nil

	aggregatedType := aggregatedACPChunkUpdateType(pending.updateType)
	if aggregatedType == "" || pending.text == "" {
		return nil
	}

	payload := map[string]any{
		"session_id":       pending.sessionID,
		"agent_session_id": pending.agentSessionID,
		"text":             pending.text,
		"acp": map[string]any{
			"sessionUpdate": aggregatedType,
			"content": map[string]any{
				"type": "text",
				"text": pending.text,
			},
		},
	}
	return recorder.AppendChatRunEvent(core.ChatRunEvent{
		SessionID:  pending.sessionID,
		ProjectID:  pending.projectID,
		EventType:  string(core.EventRunUpdate),
		UpdateType: aggregatedType,
		Payload:    payload,
		CreatedAt:  pending.createdAt,
	})
}

func extractACPChunkText(rawUpdateJSON string) string {
	trimmed := strings.TrimSpace(rawUpdateJSON)
	if trimmed == "" {
		return ""
	}

	var parsed struct {
		Content struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return ""
	}
	return parsed.Content.Text
}

func aggregatedACPChunkUpdateType(updateType string) string {
	switch strings.TrimSpace(updateType) {
	case "agent_message_chunk":
		return "agent_message"
	case "agent_thought_chunk":
		return "agent_thought"
	case "user_message_chunk":
		return "user_message"
	default:
		return ""
	}
}

func isAggregatedACPChunkUpdateType(updateType string) bool {
	return aggregatedACPChunkUpdateType(updateType) != ""
}

func isACPChunkUpdateType(updateType string) bool {
	switch strings.TrimSpace(updateType) {
	case "agent_message_chunk", "assistant_message_chunk", "message_chunk", "agent_thought_chunk", "user_message_chunk":
		return true
	default:
		return false
	}
}

func (h *ACPHandler) normalizePathInScope(rawPath string) (string, string, error) {
	cwd := strings.TrimSpace(h.cwd)
	if cwd == "" {
		return "", "", errors.New("handler cwd is required")
	}
	cwdAbs, err := filepath.Abs(cwd)
	if err != nil {
		return "", "", fmt.Errorf("resolve cwd: %w", err)
	}

	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" {
		return "", "", errors.New("write file path is required")
	}

	target := trimmed
	if !filepath.IsAbs(target) {
		target = filepath.Join(cwdAbs, target)
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", "", fmt.Errorf("resolve path: %w", err)
	}

	rel, err := filepath.Rel(cwdAbs, targetAbs)
	if err != nil {
		return "", "", fmt.Errorf("check path scope: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", "", fmt.Errorf("path %q is outside cwd scope", trimmed)
	}

	rel = filepath.ToSlash(filepath.Clean(rel))
	return targetAbs, rel, nil
}

func (h *ACPHandler) recordChangedFile(path string) []string {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, ok := h.changedSet[path]; !ok {
		h.changedSet[path] = struct{}{}
		h.changedList = append(h.changedList, path)
	}

	out := make([]string, len(h.changedList))
	copy(out, h.changedList)
	return out
}

func (h *ACPHandler) publishFilesChanged(filePaths []string) {
	if h.publisher == nil {
		return
	}

	h.mu.Lock()
	projectID := strings.TrimSpace(h.projectID)
	sessionID := strings.TrimSpace(h.chatSessionID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(h.sessionID)
	}
	h.mu.Unlock()

	h.publisher.Publish(context.Background(), core.Event{
		Type:      core.EventTeamLeaderFilesChanged,
		ProjectID: projectID,
		Data: map[string]string{
			"session_id": sessionID,
			"file_paths": strings.Join(filePaths, ","),
		},
		Timestamp: time.Now(),
	})
}

func (h *ACPHandler) normalizeDirInScope(rawDir string) (string, error) {
	cwd := strings.TrimSpace(h.cwd)
	if cwd == "" {
		return "", errors.New("handler cwd is required")
	}
	cwdAbs, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Errorf("resolve cwd: %w", err)
	}

	trimmedDir := strings.TrimSpace(rawDir)
	if trimmedDir == "" {
		return cwdAbs, nil
	}

	target := trimmedDir
	if !filepath.IsAbs(target) {
		target = filepath.Join(cwdAbs, target)
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolve terminal cwd: %w", err)
	}

	rel, err := filepath.Rel(cwdAbs, targetAbs)
	if err != nil {
		return "", fmt.Errorf("check terminal cwd scope: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("terminal cwd %q is outside handler cwd scope", trimmedDir)
	}
	return targetAbs, nil
}

func applyReadLineWindow(content string, line *int, limit *int) string {
	if line == nil && limit == nil {
		return content
	}

	start := 1
	if line != nil && *line > 0 {
		start = *line
	}

	lines := strings.Split(content, "\n")
	if start > len(lines) {
		return ""
	}

	from := start - 1
	to := len(lines)
	if limit != nil && *limit > 0 {
		max := from + *limit
		if max < to {
			to = max
		}
	}
	return strings.Join(lines[from:to], "\n")
}

func (h *ACPHandler) resolvePermissionPolicy(req acpproto.RequestPermissionRequest) string {
	action, resource := permissionRequestContext(req)
	policy := h.permissionPolicySnapshot()
	matchedRule := false

	for i := range policy {
		rule := policy[i]
		if !matchPermissionPattern(action, rule.Pattern) {
			continue
		}
		if !h.permissionScopeAllowed(resource, rule.Scope) {
			continue
		}
		matchedRule = true
		ruleAction := normalizePermissionAction(rule.Action)
		if ruleAction != "" {
			return ruleAction
		}
	}
	if matchedRule {
		return "cancelled"
	}
	if !isKnownPermissionAction(action) {
		if len(req.Options) == 0 {
			return "allow_once"
		}
		if hasPermissionOptionPrefix(req.Options, "allow") {
			return "allow_once"
		}
		if hasPermissionOptionPrefix(req.Options, "reject") {
			return "cancelled"
		}
		// Unknown tool kinds default to allow-once to avoid accidental disruption.
		return "allow_once"
	}

	return "allow_once"
}

func permissionRequestContext(req acpproto.RequestPermissionRequest) (string, string) {
	action := ""
	if value, ok := req.Meta["action"]; ok {
		if s, ok := value.(string); ok {
			action = s
		}
	}
	if strings.TrimSpace(action) == "" {
		if value, ok := req.ToolCall.Meta["action"]; ok {
			if s, ok := value.(string); ok {
				action = s
			}
		}
	}
	if strings.TrimSpace(action) == "" && req.ToolCall.Kind != nil {
		switch *req.ToolCall.Kind {
		case acpproto.ToolKindRead:
			action = "fs/read_text_file"
		case acpproto.ToolKindEdit, acpproto.ToolKindDelete, acpproto.ToolKindMove:
			action = "fs/write_text_file"
		case acpproto.ToolKindExecute:
			action = "terminal/create"
		default:
			action = string(*req.ToolCall.Kind)
		}
	}

	resource := ""
	if value, ok := req.Meta["resource"]; ok {
		if s, ok := value.(string); ok {
			resource = s
		}
	}
	if strings.TrimSpace(resource) == "" {
		if value, ok := req.ToolCall.Meta["resource"]; ok {
			if s, ok := value.(string); ok {
				resource = s
			}
		}
	}
	if strings.TrimSpace(resource) == "" && len(req.ToolCall.Locations) > 0 {
		resource = strings.TrimSpace(req.ToolCall.Locations[0].Path)
	}
	if strings.TrimSpace(resource) == "" {
		resource = permissionResourceFromRawInput(req.ToolCall.RawInput)
	}

	return normalizePermissionPattern(action), strings.TrimSpace(resource)
}

func permissionResourceFromRawInput(raw any) string {
	readPath := func(values map[string]any) string {
		if path, ok := values["path"].(string); ok {
			return strings.TrimSpace(path)
		}
		if path, ok := values["filePath"].(string); ok {
			return strings.TrimSpace(path)
		}
		return ""
	}

	switch value := raw.(type) {
	case map[string]any:
		return readPath(value)
	case json.RawMessage:
		decoded := map[string]any{}
		if err := json.Unmarshal(value, &decoded); err == nil {
			return readPath(decoded)
		}
	case []byte:
		decoded := map[string]any{}
		if err := json.Unmarshal(value, &decoded); err == nil {
			return readPath(decoded)
		}
	}
	return ""
}

func (h *ACPHandler) permissionPolicySnapshot() []acpclient.PermissionRule {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.permissionPolicy) == 0 {
		return nil
	}
	out := make([]acpclient.PermissionRule, len(h.permissionPolicy))
	copy(out, h.permissionPolicy)
	return out
}

func (h *ACPHandler) permissionScopeAllowed(resource string, scope string) bool {
	normalizedScope := strings.ToLower(strings.TrimSpace(scope))
	switch normalizedScope {
	case "", "global":
		return true
	case "cwd":
		trimmedResource := strings.TrimSpace(resource)
		if trimmedResource == "" {
			return true
		}
		_, _, err := h.normalizePathInScope(trimmedResource)
		return err == nil
	default:
		return false
	}
}

func normalizePermissionPattern(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "write_file", "write_text_file", "fs/write_file", "fs/write_text_file":
		return "fs/write_text_file"
	case "read_file", "read_text_file", "fs/read_file", "fs/read_text_file":
		return "fs/read_text_file"
	case "terminal_create", "terminal/create":
		return "terminal/create"
	default:
		return normalized
	}
}

func normalizePermissionAction(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "allow_once":
		return "allow_once"
	case "allow_always":
		return "allow_always"
	case "reject_once":
		return "reject_once"
	case "reject_always":
		return "reject_always"
	case "cancelled":
		return "cancelled"
	default:
		return ""
	}
}

func isKnownPermissionAction(action string) bool {
	switch action {
	case "fs/read_text_file", "fs/write_text_file", "terminal/create":
		return true
	default:
		return false
	}
}

func matchPermissionPattern(action string, pattern string) bool {
	normalizedPattern := normalizePermissionPattern(pattern)
	if normalizedPattern == "*" {
		return true
	}
	if normalizedPattern == "" {
		return false
	}
	return normalizedPattern == action
}

func selectPermissionOptionID(options []acpproto.PermissionOption, action string) string {
	preferredKinds := []string{}
	fallbackPrefix := ""
	switch action {
	case "allow_always":
		preferredKinds = []string{"allow_always", "allow_once"}
		fallbackPrefix = "allow"
	case "allow_once":
		preferredKinds = []string{"allow_once", "allow_always"}
		fallbackPrefix = "allow"
	case "reject_always":
		preferredKinds = []string{"reject_always", "reject_once"}
		fallbackPrefix = "reject"
	case "reject_once":
		preferredKinds = []string{"reject_once", "reject_always"}
		fallbackPrefix = "reject"
	default:
		return ""
	}

	for _, wantedKind := range preferredKinds {
		for i := range options {
			kind := strings.TrimSpace(string(options[i].Kind))
			optionID := strings.TrimSpace(string(options[i].OptionId))
			if strings.EqualFold(kind, wantedKind) || strings.EqualFold(optionID, wantedKind) {
				if optionID := strings.TrimSpace(string(options[i].OptionId)); optionID != "" {
					return optionID
				}
			}
		}
	}

	if fallbackPrefix != "" {
		for i := range options {
			kind := strings.ToLower(strings.TrimSpace(string(options[i].Kind)))
			optionID := strings.TrimSpace(string(options[i].OptionId))
			if optionID == "" {
				continue
			}
			if strings.HasPrefix(kind, fallbackPrefix) {
				return optionID
			}
		}
	}

	for i := range options {
		if optionID := strings.TrimSpace(string(options[i].OptionId)); optionID != "" {
			return optionID
		}
	}

	return ""
}

func hasPermissionOptionPrefix(options []acpproto.PermissionOption, prefix string) bool {
	normalizedPrefix := strings.ToLower(strings.TrimSpace(prefix))
	if normalizedPrefix == "" {
		return false
	}
	for i := range options {
		kind := strings.ToLower(strings.TrimSpace(string(options[i].Kind)))
		if strings.HasPrefix(kind, normalizedPrefix) {
			return true
		}
	}
	return false
}

func (h *ACPHandler) getTerminal(terminalID string) (*acpTerminalState, bool) {
	trimmed := strings.TrimSpace(terminalID)
	if h == nil || trimmed == "" {
		return nil, false
	}
	h.terminalMu.Lock()
	defer h.terminalMu.Unlock()
	state, ok := h.terminals[trimmed]
	return state, ok
}

func (h *ACPHandler) removeTerminal(terminalID string) (*acpTerminalState, bool) {
	trimmed := strings.TrimSpace(terminalID)
	if h == nil || trimmed == "" {
		return nil, false
	}
	h.terminalMu.Lock()
	defer h.terminalMu.Unlock()
	state, ok := h.terminals[trimmed]
	if ok {
		delete(h.terminals, trimmed)
	}
	return state, ok
}

func isTerminalDone(state *acpTerminalState) bool {
	if state == nil {
		return true
	}
	select {
	case <-state.done:
		return true
	default:
		return false
	}
}

func mergeTerminalEnv(extra []acpproto.EnvVariable) []string {
	env := os.Environ()
	for _, item := range extra {
		key := strings.TrimSpace(item.Name)
		if key == "" {
			continue
		}
		env = append(env, key+"="+item.Value)
	}
	return env
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}


