package acp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/zhanggui/internal/adapters/agent/acpclient"
	eventbridge "github.com/yoke233/zhanggui/internal/adapters/events/bridge"
	v2sandbox "github.com/yoke233/zhanggui/internal/adapters/sandbox"
	workspaceclone "github.com/yoke233/zhanggui/internal/adapters/workspace/clone"
	workspacegit "github.com/yoke233/zhanggui/internal/adapters/workspace/git"
	chatapp "github.com/yoke233/zhanggui/internal/application/chat"
	flowapp "github.com/yoke233/zhanggui/internal/application/flow"
	"github.com/yoke233/zhanggui/internal/core"
	"github.com/yoke233/zhanggui/internal/platform/config"
	"github.com/yoke233/zhanggui/internal/platform/profilellm"
)

const (
	defaultLeadProfileID = "lead"
)

// TextCompleter generates free-form text from a prompt (e.g. branch name
// generation).  Implemented by *llm.Client.
type TextCompleter interface {
	CompleteText(ctx context.Context, prompt string) (string, error)
}

type DriverResolver func(ctx context.Context, driverID string) (*core.DriverConfig, error)
type LLMConfigResolver func(ctx context.Context, llmConfigID string) (*config.RuntimeLLMEntryConfig, error)

type LeadAgentConfig struct {
	Registry           core.AgentRegistry
	DriverResolver     DriverResolver
	LLMConfigResolver  LLMConfigResolver
	Bus                core.EventBus
	ResourceSpaceStore core.ResourceSpaceStore
	LLM                TextCompleter
	ProfileID          string
	IdleTTL            time.Duration
	Sandbox            v2sandbox.Sandbox
	DataDir            string
	NewClient          func(cfg acpclient.LaunchConfig, h acpproto.Client, opts ...acpclient.Option) (ChatACPClient, error)

	// SCM provider factory for PR/MR automation in chat sessions.
	ChangeRequestProviders func(token string) []flowapp.ChangeRequestProvider
	// GitPAT is the personal access token for SCM operations.
	GitPAT string

	// GC controls workspace resource reclamation.
	GC GCConfig
	// BackgroundContext is the application-scoped context used for async chat work.
	BackgroundContext context.Context
}

// GCConfig controls workspace resource reclamation for chat sessions.
type GCConfig struct {
	// ArchiveCleanup deletes workspace (worktree/sandbox dir) when a session is archived.
	ArchiveCleanup bool
	// StartupCleanup removes orphan workspaces on server start.
	StartupCleanup bool
	// Interval is the periodic GC sweep interval. Zero disables periodic GC.
	Interval time.Duration
	// RepoMaxAge is how long an unused cloned repo is kept. Zero means keep forever.
	RepoMaxAge time.Duration
}

type LeadAgent struct {
	cfg    LeadAgentConfig
	broker *permissionBroker

	mu          sync.Mutex
	sessions    map[string]*leadSession
	catalog     map[string]*persistedLeadSession
	catalogPath string

	activeMu   sync.Mutex
	activeRuns map[string]context.CancelFunc

	pendingMu   sync.Mutex
	pendingMsgs map[string]*chatapp.PendingMessage // at most 1 per session
}

type leadSession struct {
	client    ChatACPClient
	handler   *leadChatHandler
	sessionID acpproto.SessionId
	bridge    *eventbridge.EventBridge
	events    *suppressibleEventHandler
	workDir   string
	scope     string
	// isolation tracks how the working directory was provisioned so we can
	// clean it up when the session is closed.  Possible values:
	//   ""          – workDir was supplied by the caller (no cleanup)
	//   "sandbox"   – a temporary directory was created under DataDir
	//   "worktree"  – a git worktree was created for a project
	isolation string
	repoPath  string // original repo path; set when isolation == "worktree"
	branch    string

	mu        sync.Mutex
	idleTimer *time.Timer
	closed    bool
}

type ChatACPClient interface {
	Initialize(ctx context.Context, caps acpclient.ClientCapabilities) error
	NewSession(ctx context.Context, req acpproto.NewSessionRequest) (acpproto.SessionId, error)
	NewSessionResult(ctx context.Context, req acpproto.NewSessionRequest) (acpclient.SessionResult, error)
	LoadSessionResult(ctx context.Context, req acpproto.LoadSessionRequest) (acpclient.SessionResult, error)
	LoadSession(ctx context.Context, req acpproto.LoadSessionRequest) (acpproto.SessionId, error)
	Prompt(ctx context.Context, req acpproto.PromptRequest) (*acpclient.PromptResult, error)
	SetConfigOption(ctx context.Context, req acpproto.SetSessionConfigOptionRequest) ([]acpproto.SessionConfigOptionSelect, error)
	SetSessionMode(ctx context.Context, req acpproto.SetSessionModeRequest) error
	Cancel(ctx context.Context, req acpproto.CancelNotification) error
	Close(ctx context.Context) error
}

type suppressibleEventHandler struct {
	mu       sync.RWMutex
	suppress bool
	inner    acpclient.EventHandler
	onUpdate func(acpclient.SessionUpdate)
}

func (h *suppressibleEventHandler) SetSuppress(v bool) {
	h.mu.Lock()
	h.suppress = v
	h.mu.Unlock()
}

func (h *suppressibleEventHandler) SetUpdateCallback(cb func(acpclient.SessionUpdate)) {
	h.mu.Lock()
	h.onUpdate = cb
	h.mu.Unlock()
}

func (h *suppressibleEventHandler) HandleSessionUpdate(ctx context.Context, update acpclient.SessionUpdate) error {
	h.mu.RLock()
	suppress := h.suppress
	inner := h.inner
	onUpdate := h.onUpdate
	h.mu.RUnlock()
	if onUpdate != nil {
		onUpdate(update)
	}
	if suppress || inner == nil {
		return nil
	}
	return inner.HandleSessionUpdate(ctx, update)
}

func NewLeadAgent(cfg LeadAgentConfig) *LeadAgent {
	if cfg.ProfileID == "" {
		cfg.ProfileID = defaultLeadProfileID
	}
	if cfg.NewClient == nil {
		cfg.NewClient = func(launch acpclient.LaunchConfig, h acpproto.Client, opts ...acpclient.Option) (ChatACPClient, error) {
			return acpclient.New(launch, h, opts...)
		}
	}

	catalogPath := ""
	if strings.TrimSpace(cfg.DataDir) != "" {
		catalogPath = filepath.Join(cfg.DataDir, leadSessionCatalogFileName)
	}
	catalog, err := loadLeadCatalog(catalogPath)
	if err != nil {
		slog.Warn("lead chat: load catalog failed", "path", catalogPath, "error", err)
		catalog = map[string]*persistedLeadSession{}
	}

	agent := &LeadAgent{
		cfg:         cfg,
		broker:      newPermissionBroker(),
		sessions:    make(map[string]*leadSession),
		catalog:     catalog,
		catalogPath: catalogPath,
		activeRuns:  make(map[string]context.CancelFunc),
		pendingMsgs: make(map[string]*chatapp.PendingMessage),
	}
	if cfg.GC.StartupCleanup {
		agent.gcOrphanWorkspaces()
	}
	return agent
}

func (l *LeadAgent) backgroundContext() context.Context {
	if l != nil && l.cfg.BackgroundContext != nil {
		return l.cfg.BackgroundContext
	}
	return context.Background()
}

// StartGC launches the periodic GC goroutine. Call this after the server is
// ready.  The goroutine stops when ctx is cancelled.
func (l *LeadAgent) StartGC(ctx context.Context) {
	interval := l.cfg.GC.Interval
	if interval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				l.gcOrphanWorkspaces()
				l.gcStaleRepos()
			}
		}
	}()
	slog.Info("lead chat gc: periodic GC started", "interval", interval)
}

// gcOrphanWorkspaces removes worktree and sandbox directories that are not
// referenced by any active (non-archived) catalog entry.  This recovers disk
// space leaked by server crashes.
func (l *LeadAgent) gcOrphanWorkspaces() {
	dataDir := strings.TrimSpace(l.cfg.DataDir)
	if dataDir == "" {
		return
	}

	// Collect active workspace paths from catalog.
	l.mu.Lock()
	activeWorkDirs := make(map[string]struct{}, len(l.catalog))
	for _, record := range l.catalog {
		if !record.Archived || !l.cfg.GC.ArchiveCleanup {
			if wd := strings.TrimSpace(record.WorkDir); wd != "" {
				activeWorkDirs[filepath.Clean(wd)] = struct{}{}
			}
		}
	}
	l.mu.Unlock()

	// Scan worktrees/<projectID>/ directories.
	worktreeRoot := filepath.Join(dataDir, "worktrees")
	l.gcOrphanDirs(worktreeRoot, activeWorkDirs, "worktree")

	// Scan chat-sandboxes/ directories.
	sandboxRoot := filepath.Join(dataDir, "chat-sandboxes")
	l.gcOrphanDirs(sandboxRoot, activeWorkDirs, "sandbox")
}

func (l *LeadAgent) gcOrphanDirs(root string, activeWorkDirs map[string]struct{}, label string) {
	projectDirs, err := os.ReadDir(root)
	if err != nil {
		return // directory doesn't exist yet — nothing to clean
	}

	for _, projectEntry := range projectDirs {
		projectPath := filepath.Join(root, projectEntry.Name())

		if !projectEntry.IsDir() {
			continue
		}

		// For worktrees: root/<projectID>/<chat-slug>; for sandboxes: root/<sess-xxx>.
		if label == "sandbox" {
			// Sandbox dirs are directly under root.
			if _, active := activeWorkDirs[filepath.Clean(projectPath)]; !active {
				if err := os.RemoveAll(projectPath); err == nil {
					slog.Info("lead chat gc: removed orphan sandbox", "path", projectPath)
				}
			}
			continue
		}

		// Worktree: enumerate children.
		children, err := os.ReadDir(projectPath)
		if err != nil {
			continue
		}
		for _, child := range children {
			childPath := filepath.Join(projectPath, child.Name())
			if !child.IsDir() {
				continue
			}
			if _, active := activeWorkDirs[filepath.Clean(childPath)]; !active {
				// Try git worktree remove first; fall back to RemoveAll.
				cleanupIsolatedDir("worktree", childPath, "")
				_ = os.RemoveAll(childPath) // ensure removal even if git fails
				slog.Info("lead chat gc: removed orphan worktree", "path", childPath)
			}
		}
		// Remove empty project dir.
		if remaining, _ := os.ReadDir(projectPath); len(remaining) == 0 {
			_ = os.Remove(projectPath)
		}
	}
}

// gcStaleRepos removes auto-cloned repositories that haven't been used by any
// catalog session for longer than RepoMaxAge.
func (l *LeadAgent) gcStaleRepos() {
	maxAge := l.cfg.GC.RepoMaxAge
	if maxAge <= 0 {
		return
	}
	dataDir := strings.TrimSpace(l.cfg.DataDir)
	if dataDir == "" {
		return
	}

	// Collect project IDs still referenced by any catalog entry.
	l.mu.Lock()
	activeProjectIDs := make(map[int64]struct{})
	for _, record := range l.catalog {
		if record.ProjectID > 0 {
			activeProjectIDs[record.ProjectID] = struct{}{}
		}
	}
	l.mu.Unlock()

	repoRoot := filepath.Join(dataDir, "repos")
	entries, err := os.ReadDir(repoRoot)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-maxAge)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Parse project ID from directory name.
		projectID, err := strconv.ParseInt(entry.Name(), 10, 64)
		if err != nil {
			continue
		}
		// Skip repos still referenced by active sessions.
		if _, active := activeProjectIDs[projectID]; active {
			continue
		}
		// Check modification time.
		repoPath := filepath.Join(repoRoot, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		if err := os.RemoveAll(repoPath); err == nil {
			slog.Info("lead chat gc: removed stale repo clone",
				"project_id", projectID, "path", repoPath, "age", time.Since(info.ModTime()))
		}
	}
}

func (l *LeadAgent) Chat(ctx context.Context, req chatapp.Request) (*chatapp.Response, error) {
	sess, publicSessionID, message, err := l.prepareChat(ctx, req)
	if err != nil {
		return nil, err
	}

	reply, err := l.runPrompt(ctx, publicSessionID, sess, message, req.Attachments)
	if err != nil {
		return nil, err
	}

	return &chatapp.Response{
		SessionID: publicSessionID,
		Reply:     reply,
		WSPath:    buildChatWSPath(publicSessionID),
	}, nil
}

func (l *LeadAgent) StartChat(ctx context.Context, req chatapp.Request) (*chatapp.AcceptedResponse, error) {
	sess, publicSessionID, message, err := l.prepareChat(ctx, req)
	if err != nil {
		return nil, err
	}

	// If session is busy, queue the message for later dispatch.
	if l.IsSessionRunning(publicSessionID) {
		l.setPending(publicSessionID, &chatapp.PendingMessage{
			Message:     message,
			Attachments: req.Attachments,
		})
		return &chatapp.AcceptedResponse{
			SessionID: publicSessionID,
			WSPath:    buildChatWSPath(publicSessionID),
			Status:    "queued",
		}, nil
	}

	attachments := req.Attachments
	go func() {
		runCtx := l.backgroundContext()
		if _, runErr := l.runPrompt(runCtx, publicSessionID, sess, message, attachments); runErr != nil {
			sess.bridge.PublishData(context.Background(), map[string]any{
				"type":    "error",
				"content": runErr.Error(),
			})
			slog.Warn("lead chat async prompt failed", "session_id", publicSessionID, "error", runErr)
		}
	}()

	return &chatapp.AcceptedResponse{
		SessionID: publicSessionID,
		WSPath:    buildChatWSPath(publicSessionID),
		Status:    "accepted",
	}, nil
}

func (l *LeadAgent) prepareChat(ctx context.Context, req chatapp.Request) (*leadSession, string, string, error) {
	message := strings.TrimSpace(req.Message)
	if message == "" {
		return nil, "", "", errors.New("message is required")
	}

	// For existing sessions, skip workspace provisioning (already done).
	if strings.TrimSpace(req.SessionID) != "" {
		workDir, err := resolveLeadWorkDir(req.WorkDir)
		if err != nil {
			return nil, "", "", err
		}
		sess, publicSessionID, err := l.getOrCreateSession(ctx, req, workDir)
		if err != nil {
			return nil, "", "", err
		}
		sess.stopIdleTimer()
		return sess, publicSessionID, message, nil
	}

	// New session — resolve an isolated working directory.
	workDir, isolation, repoPath, branch, err := l.resolveIsolatedWorkDir(ctx, req)
	if err != nil {
		return nil, "", "", err
	}

	sess, publicSessionID, err := l.createSession(ctx, workDir, req.ProjectID, req.ProjectName, req.ProfileID, req.DriverID, req.LLMConfigID)
	if err != nil {
		// Cleanup on failure.
		cleanupIsolatedDir(isolation, workDir, repoPath)
		return nil, "", "", err
	}
	sess.isolation = isolation
	sess.repoPath = repoPath
	sess.branch = branch

	// Persist isolation metadata in catalog.
	if isolation != "" {
		l.mu.Lock()
		if record := l.catalog[publicSessionID]; record != nil {
			record.Isolation = isolation
			record.RepoPath = repoPath
			record.Branch = branch
			_ = l.saveCatalogLocked()
		}
		l.mu.Unlock()
	}

	sess.stopIdleTimer()
	return sess, publicSessionID, message, nil
}

func (l *LeadAgent) runPrompt(ctx context.Context, publicSessionID string, sess *leadSession, message string, attachments []chatapp.Attachment) (string, error) {
	if sess == nil {
		return "", errors.New("session is not initialized")
	}

	promptCtx, promptCancel := context.WithCancel(ctx)
	if err := l.beginRun(publicSessionID, promptCancel); err != nil {
		promptCancel()
		l.resetSessionIdle(publicSessionID, sess)
		return "", err
	}
	defer l.endRun(publicSessionID)
	defer promptCancel()

	l.appendMessage(publicSessionID, "user", message)

	promptBlocks := buildPromptBlocks(message, attachments, sess.workDir)

	result, err := sess.client.Prompt(promptCtx, acpproto.PromptRequest{
		SessionId: sess.sessionID,
		Prompt:    promptBlocks,
	})

	sess.bridge.FlushPending(context.Background())

	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			l.resetSessionIdle(publicSessionID, sess)
		} else {
			l.removeSession(publicSessionID)
		}
		return "", fmt.Errorf("prompt failed: %w", err)
	}
	if result == nil {
		l.resetSessionIdle(publicSessionID, sess)
		return "", errors.New("empty result from agent")
	}

	reply := strings.TrimSpace(result.Text)
	if reply == "" {
		l.resetSessionIdle(publicSessionID, sess)
		return "", errors.New("empty reply from agent")
	}

	sess.bridge.PublishData(context.Background(), map[string]any{
		"type": "done",
	})

	l.appendMessage(publicSessionID, "assistant", reply)
	l.resetSessionIdle(publicSessionID, sess)
	return reply, nil
}

func (l *LeadAgent) beginRun(sessionID string, cancel context.CancelFunc) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session_id is required")
	}

	l.activeMu.Lock()
	defer l.activeMu.Unlock()
	if _, exists := l.activeRuns[sessionID]; exists {
		return errors.New("session is already running")
	}
	l.activeRuns[sessionID] = cancel
	return nil
}

func (l *LeadAgent) endRun(sessionID string) {
	id := strings.TrimSpace(sessionID)

	// Atomically: check pending + update activeRuns under both locks.
	// Lock ordering: pendingMu → activeMu (consistent everywhere).
	var dispatchCtx context.Context
	var dispatchCancel context.CancelFunc

	l.pendingMu.Lock()
	pending := l.pendingMsgs[id]
	delete(l.pendingMsgs, id)
	l.activeMu.Lock()
	if pending == nil {
		delete(l.activeRuns, id)
	} else {
		dispatchCtx, dispatchCancel = context.WithCancel(l.backgroundContext())
		l.activeRuns[id] = dispatchCancel
	}
	l.activeMu.Unlock()
	l.pendingMu.Unlock()

	if pending == nil {
		return
	}

	l.mu.Lock()
	sess := l.sessions[id]
	l.mu.Unlock()
	if sess == nil {
		dispatchCancel()
		l.activeMu.Lock()
		delete(l.activeRuns, id)
		l.activeMu.Unlock()
		return
	}

	sess.bridge.PublishData(dispatchCtx, map[string]any{
		"type": "pending_dispatched",
	})

	go l.runPending(dispatchCtx, dispatchCancel, id, sess, pending)
}

func (l *LeadAgent) runPending(ctx context.Context, cancel context.CancelFunc, sessionID string, sess *leadSession, pending *chatapp.PendingMessage) {
	defer l.endRun(sessionID) // recursive: will check for next pending
	defer cancel()

	l.appendMessage(sessionID, "user", pending.Message)
	promptBlocks := buildPromptBlocks(pending.Message, pending.Attachments, sess.workDir)

	result, err := sess.client.Prompt(ctx, acpproto.PromptRequest{
		SessionId: sess.sessionID,
		Prompt:    promptBlocks,
	})
	sess.bridge.FlushPending(context.Background())

	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			l.resetSessionIdle(sessionID, sess)
		} else {
			l.removeSession(sessionID)
		}
		sess.bridge.PublishData(context.Background(), map[string]any{
			"type":    "error",
			"content": fmt.Sprintf("pending dispatch failed: %v", err),
		})
		return
	}
	if result == nil {
		l.resetSessionIdle(sessionID, sess)
		sess.bridge.PublishData(context.Background(), map[string]any{
			"type":    "error",
			"content": "empty result from agent",
		})
		return
	}

	reply := strings.TrimSpace(result.Text)
	if reply == "" {
		l.resetSessionIdle(sessionID, sess)
		sess.bridge.PublishData(context.Background(), map[string]any{
			"type":    "error",
			"content": "empty reply from agent",
		})
		return
	}

	sess.bridge.PublishData(context.Background(), map[string]any{"type": "done"})
	l.appendMessage(sessionID, "assistant", reply)
	l.resetSessionIdle(sessionID, sess)
}

func (l *LeadAgent) setPending(sessionID string, msg *chatapp.PendingMessage) {
	l.pendingMu.Lock()
	l.pendingMsgs[strings.TrimSpace(sessionID)] = msg
	l.pendingMu.Unlock()
}

func (l *LeadAgent) takePending(sessionID string) *chatapp.PendingMessage {
	id := strings.TrimSpace(sessionID)
	l.pendingMu.Lock()
	msg := l.pendingMsgs[id]
	delete(l.pendingMsgs, id)
	l.pendingMu.Unlock()
	return msg
}

func (l *LeadAgent) CancelPending(sessionID string) bool {
	id := strings.TrimSpace(sessionID)
	l.pendingMu.Lock()
	_, existed := l.pendingMsgs[id]
	delete(l.pendingMsgs, id)
	l.pendingMu.Unlock()
	return existed
}

func (l *LeadAgent) ListSessions(context.Context) ([]chatapp.SessionSummary, error) {
	running := l.runningSessionSet()

	l.mu.Lock()
	defer l.mu.Unlock()

	items := make([]chatapp.SessionSummary, 0, len(l.catalog))
	for _, record := range l.catalog {
		if record == nil || strings.TrimSpace(record.SessionID) == "" {
			continue
		}
		live := false
		if sess, ok := l.sessions[record.SessionID]; ok && !sess.isClosed() {
			live = true
		}
		items = append(items, buildSessionSummary(record, live, running[record.SessionID]))
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].SessionID < items[j].SessionID
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	return items, nil
}

func (l *LeadAgent) GetSession(_ context.Context, sessionID string) (*chatapp.SessionDetail, error) {
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return nil, errors.New("session_id is required")
	}
	running := l.runningSessionSet()

	l.mu.Lock()
	defer l.mu.Unlock()

	record, ok := l.catalog[id]
	if !ok {
		return nil, core.ErrNotFound
	}
	live := false
	if sess, ok := l.sessions[id]; ok && !sess.isClosed() {
		live = true
	}

	detail := &chatapp.SessionDetail{
		SessionSummary:    buildSessionSummary(record, live, running[id]),
		Messages:          append([]chatapp.Message(nil), record.Messages...),
		AvailableCommands: cloneAvailableCommands(record.AvailableCommands),
		ConfigOptions:     cloneConfigOptions(record.ConfigOptions),
		Modes:             cloneModeState(record.Modes),
	}
	return detail, nil
}

// ResolvePermission resolves a pending permission request initiated by the ACP
// agent.  Called from the WebSocket handler when the user clicks allow/reject.
func (l *LeadAgent) ResolvePermission(permissionID, optionID string, cancel bool) error {
	if strings.TrimSpace(permissionID) == "" {
		return errors.New("permission_id is required")
	}
	if !l.broker.Resolve(permissionID, optionID, cancel) {
		return errors.New("permission request not found or already resolved")
	}
	return nil
}

func (l *LeadAgent) CancelChat(sessionID string) error {
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return errors.New("session_id is required")
	}

	// Discard any queued pending message so endRun won't auto-dispatch it.
	l.takePending(id)

	l.activeMu.Lock()
	cancel, ok := l.activeRuns[id]
	l.activeMu.Unlock()
	if !ok {
		return errors.New("session is not running")
	}
	cancel()

	l.mu.Lock()
	sess := l.sessions[id]
	l.mu.Unlock()
	if sess != nil {
		cancelCtx, c := context.WithTimeout(l.backgroundContext(), 3*time.Second)
		defer c()
		_ = sess.client.Cancel(cancelCtx, acpproto.CancelNotification{SessionId: sess.sessionID})
	}
	return nil
}

// SetConfigOption changes a session config option via the underlying ACP client
// and returns the updated config options list.
func (l *LeadAgent) SetConfigOption(ctx context.Context, sessionID, configID, value string) ([]chatapp.ConfigOption, error) {
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return nil, errors.New("session_id is required")
	}

	l.mu.Lock()
	sess, ok := l.sessions[id]
	l.mu.Unlock()
	if !ok || sess == nil || sess.isClosed() {
		return nil, errors.New("session is not alive")
	}

	updated, err := sess.client.SetConfigOption(ctx, acpproto.SetSessionConfigOptionRequest{
		SessionId: sess.sessionID,
		ConfigId:  acpproto.SessionConfigId(strings.TrimSpace(configID)),
		Value:     acpproto.SessionConfigValueId(strings.TrimSpace(value)),
	})
	if err != nil {
		return nil, fmt.Errorf("set config option: %w", err)
	}

	// Persist updated config options in catalog.
	chatOpts := toChatConfigOptions(updated)
	l.mu.Lock()
	if record := l.catalog[id]; record != nil {
		record.ConfigOptions = chatOpts
		record.UpdatedAt = time.Now().UTC()
		_ = l.saveCatalogLocked()
	}
	l.mu.Unlock()

	return chatOpts, nil
}

// SetSessionMode changes the session mode via the underlying ACP client
// and persists the change.
func (l *LeadAgent) SetSessionMode(ctx context.Context, sessionID, modeID string) (*chatapp.SessionModeState, error) {
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return nil, errors.New("session_id is required")
	}

	l.mu.Lock()
	sess, ok := l.sessions[id]
	l.mu.Unlock()
	if !ok || sess == nil || sess.isClosed() {
		return nil, errors.New("session is not alive")
	}

	if err := sess.client.SetSessionMode(ctx, acpproto.SetSessionModeRequest{
		SessionId: sess.sessionID,
		ModeId:    acpproto.SessionModeId(strings.TrimSpace(modeID)),
	}); err != nil {
		return nil, fmt.Errorf("set session mode: %w", err)
	}

	// Persist updated mode in catalog.
	l.mu.Lock()
	var result *chatapp.SessionModeState
	if record := l.catalog[id]; record != nil && record.Modes != nil {
		record.Modes.CurrentModeId = strings.TrimSpace(modeID)
		record.UpdatedAt = time.Now().UTC()
		_ = l.saveCatalogLocked()
		result = cloneModeState(record.Modes)
	}
	l.mu.Unlock()

	return result, nil
}

func (l *LeadAgent) CloseSession(sessionID string) {
	l.removeSession(strings.TrimSpace(sessionID))
}

// DeleteSession permanently removes a session: terminates the agent process,
// cleans up the isolated workspace (sandbox or worktree), and removes the
// catalog entry so the session can no longer be resumed.
func (l *LeadAgent) DeleteSession(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}

	l.takePending(sessionID) // discard any orphaned pending message

	// Remove agent from memory.
	l.mu.Lock()
	sess, ok := l.sessions[sessionID]
	if ok {
		delete(l.sessions, sessionID)
	}
	record := l.catalog[sessionID]
	delete(l.catalog, sessionID)
	_ = l.saveCatalogLocked()
	l.mu.Unlock()

	if sess != nil {
		sess.close()
		cleanupIsolatedDir(sess.isolation, sess.workDir, sess.repoPath)
	} else if record != nil {
		// Session not in memory but in catalog — clean up workspace.
		cleanupIsolatedDir(record.Isolation, record.WorkDir, record.RepoPath)
	}
}

// RenameSession updates the title of a persisted session.
func (l *LeadAgent) RenameSession(sessionID string, title string) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session_id is required")
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("title is required")
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	record, ok := l.catalog[sessionID]
	if !ok {
		return core.ErrNotFound
	}
	record.Title = title
	record.UpdatedAt = time.Now().UTC()
	return l.saveCatalogLocked()
}

// ArchiveSession toggles the archived flag on a session. Archived sessions
// are hidden from the default listing but remain on disk.
func (l *LeadAgent) ArchiveSession(sessionID string, archived bool) error {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session_id is required")
	}

	l.mu.Lock()
	record, ok := l.catalog[sessionID]
	if !ok {
		l.mu.Unlock()
		return core.ErrNotFound
	}
	record.Archived = archived
	if err := l.saveCatalogLocked(); err != nil {
		l.mu.Unlock()
		return err
	}

	// When archiving (not un-archiving), reclaim workspace resources.
	var sess *leadSession
	var isolation, workDir, repoPath string
	if archived && l.cfg.GC.ArchiveCleanup {
		sess = l.sessions[sessionID]
		if sess != nil {
			delete(l.sessions, sessionID)
			isolation = sess.isolation
			workDir = sess.workDir
			repoPath = sess.repoPath
		} else {
			isolation = record.Isolation
			workDir = record.WorkDir
			repoPath = record.RepoPath
		}
	}
	l.mu.Unlock()

	if archived && l.cfg.GC.ArchiveCleanup && workDir != "" {
		if sess != nil {
			sess.close()
		}
		cleanupIsolatedDir(isolation, workDir, repoPath)
		slog.Info("lead chat gc: archived session workspace cleaned",
			"session_id", sessionID, "isolation", isolation, "work_dir", workDir)
	}
	return nil
}

func (l *LeadAgent) Shutdown() {
	l.mu.Lock()
	sessions := make([]*leadSession, 0, len(l.sessions))
	for id, sess := range l.sessions {
		sessions = append(sessions, sess)
		delete(l.sessions, id)
	}
	l.mu.Unlock()

	for _, sess := range sessions {
		sess.close()
	}
}

func (l *LeadAgent) IsSessionAlive(sessionID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	sess, ok := l.sessions[strings.TrimSpace(sessionID)]
	return ok && !sess.isClosed()
}

func (l *LeadAgent) IsSessionRunning(sessionID string) bool {
	l.activeMu.Lock()
	defer l.activeMu.Unlock()
	_, ok := l.activeRuns[strings.TrimSpace(sessionID)]
	return ok
}

func (l *LeadAgent) getOrCreateSession(ctx context.Context, req chatapp.Request, workDir string) (*leadSession, string, error) {
	requestedSessionID := strings.TrimSpace(req.SessionID)
	if requestedSessionID != "" {
		l.mu.Lock()
		if sess, ok := l.sessions[requestedSessionID]; ok && !sess.isClosed() {
			l.mu.Unlock()
			return sess, requestedSessionID, nil
		}
		record := l.cloneRecordLocked(requestedSessionID)
		l.mu.Unlock()

		if record == nil {
			return nil, "", core.ErrNotFound
		}
		if strings.TrimSpace(workDir) == "" {
			workDir = record.WorkDir
		}
		sess, err := l.loadSession(ctx, record, workDir)
		if err != nil {
			return nil, "", err
		}
		return sess, requestedSessionID, nil
	}

	sess, sessionID, err := l.createSession(ctx, workDir, req.ProjectID, req.ProjectName, req.ProfileID, req.DriverID, req.LLMConfigID)
	if err != nil {
		return nil, "", err
	}
	return sess, sessionID, nil
}

func (l *LeadAgent) createSession(ctx context.Context, workDir string, projectID int64, projectName, profileID, driverID, llmConfigID string) (*leadSession, string, error) {
	scope := fmt.Sprintf("lead-chat-%d", time.Now().UnixNano())

	driverID = strings.TrimSpace(driverID)
	client, handler, bridge, events, profile, sessionCwd, err := l.launchClient(ctx, workDir, scope, "", profileID, driverID, llmConfigID)
	if err != nil {
		return nil, "", err
	}

	initCtx, initCancel := context.WithTimeout(ctx, 30*time.Second)
	defer initCancel()

	sessionResult, err := client.NewSessionResult(initCtx, acpproto.NewSessionRequest{
		Cwd:        sessionCwd,
		McpServers: []acpproto.McpServer{},
	})
	if err != nil {
		_ = client.Close(context.Background())
		return nil, "", fmt.Errorf("create lead session: %w", err)
	}

	publicID := strings.TrimSpace(string(sessionResult.SessionID))
	if publicID == "" {
		_ = client.Close(context.Background())
		return nil, "", errors.New("create lead session returned empty session id")
	}

	handler.SetSessionID(publicID)
	sess := &leadSession{
		client:    client,
		handler:   handler,
		sessionID: sessionResult.SessionID,
		bridge:    bridge,
		events:    events,
		workDir:   workDir,
		scope:     scope,
	}
	bridge.SetSessionID(publicID)

	var initialModes *chatapp.SessionModeState
	if sessionResult.Modes != nil {
		initialModes = toChatModeState(sessionResult.Modes)
	}
	initialConfigOpts := toChatConfigOptions(sessionResult.ConfigOptions)

	l.mu.Lock()
	if old, ok := l.sessions[publicID]; ok {
		go old.close()
	}
	l.sessions[publicID] = sess
	now := time.Now().UTC()
	record := l.catalog[publicID]
	if record == nil {
		record = &persistedLeadSession{
			SessionID: publicID,
			CreatedAt: now,
		}
		l.catalog[publicID] = record
	}
	record.Scope = scope
	record.WorkDir = workDir
	record.ProjectID = projectID
	record.ProjectName = strings.TrimSpace(projectName)
	record.ProfileID = profile.ID
	record.ProfileName = strings.TrimSpace(profile.Name)
	record.DriverID = driverID
	record.LLMConfigID = llmConfigID
	record.AvailableCommands = nil
	record.ConfigOptions = initialConfigOpts
	record.Modes = initialModes
	record.UpdatedAt = now
	_ = l.saveCatalogLocked()
	l.mu.Unlock()
	events.SetUpdateCallback(func(update acpclient.SessionUpdate) {
		l.captureSessionState(publicID, update)
	})

	slog.Info("runtime lead session created", "session_id", publicID, "profile", profile.ID)
	return sess, publicID, nil
}

func (l *LeadAgent) loadSession(ctx context.Context, record *persistedLeadSession, workDir string) (*leadSession, error) {
	if record == nil || strings.TrimSpace(record.SessionID) == "" {
		return nil, core.ErrNotFound
	}
	if strings.TrimSpace(record.Scope) == "" {
		return nil, fmt.Errorf("session %s has no persisted scope", record.SessionID)
	}
	if strings.TrimSpace(workDir) == "" {
		workDir = record.WorkDir
	}
	if workDir == "" {
		var err error
		workDir, err = resolveLeadWorkDir("")
		if err != nil {
			return nil, err
		}
	}

	client, handler, bridge, events, _, sessionCwd, err := l.launchClient(ctx, workDir, record.Scope, record.SessionID, record.ProfileID, record.DriverID, record.LLMConfigID)
	if err != nil {
		return nil, err
	}
	events.SetUpdateCallback(func(update acpclient.SessionUpdate) {
		l.captureSessionState(record.SessionID, update)
	})

	events.SetSuppress(true)
	initCtx, initCancel := context.WithTimeout(ctx, 30*time.Second)
	defer initCancel()

	loadResult, err := client.LoadSessionResult(initCtx, acpproto.LoadSessionRequest{
		SessionId:  acpproto.SessionId(record.SessionID),
		Cwd:        sessionCwd,
		McpServers: []acpproto.McpServer{},
	})
	events.SetSuppress(false)
	if err != nil {
		_ = client.Close(context.Background())
		return nil, fmt.Errorf("load lead session %s: %w", record.SessionID, err)
	}
	loadedID := loadResult.SessionID
	if strings.TrimSpace(string(loadedID)) == "" {
		loadedID = acpproto.SessionId(record.SessionID)
	}

	handler.SetSessionID(record.SessionID)
	sess := &leadSession{
		client:    client,
		handler:   handler,
		sessionID: loadedID,
		bridge:    bridge,
		events:    events,
		workDir:   workDir,
		scope:     record.Scope,
		isolation: record.Isolation,
		repoPath:  record.RepoPath,
		branch:    record.Branch,
	}

	l.mu.Lock()
	if old, ok := l.sessions[record.SessionID]; ok {
		go old.close()
	}
	l.sessions[record.SessionID] = sess
	stored := l.catalog[record.SessionID]
	if stored != nil {
		stored.WorkDir = workDir
		stored.UpdatedAt = time.Now().UTC()
		if loadResult.Modes != nil {
			stored.Modes = toChatModeState(loadResult.Modes)
		}
		if len(loadResult.ConfigOptions) > 0 {
			stored.ConfigOptions = toChatConfigOptions(loadResult.ConfigOptions)
		}
		_ = l.saveCatalogLocked()
	}
	l.mu.Unlock()

	slog.Info("runtime lead session loaded", "session_id", record.SessionID, "scope", record.Scope)
	return sess, nil
}

func (l *LeadAgent) launchClient(ctx context.Context, workDir, scope, publicSessionID, requestedProfileID, requestedDriverID, requestedLLMConfigID string) (ChatACPClient, *leadChatHandler, *eventbridge.EventBridge, *suppressibleEventHandler, *core.AgentProfile, string, error) {
	if l.cfg.Registry == nil {
		return nil, nil, nil, nil, nil, "", errors.New("agent registry is not configured")
	}

	profileID := strings.TrimSpace(requestedProfileID)
	if profileID == "" {
		profileID = l.cfg.ProfileID
	}
	profile, err := l.cfg.Registry.ResolveByID(ctx, profileID)
	if err != nil {
		return nil, nil, nil, nil, nil, "", fmt.Errorf("resolve lead profile %q: %w", profileID, err)
	}
	profile = cloneLeadProfile(profile)

	requestedDriverID = strings.TrimSpace(requestedDriverID)
	if requestedDriverID != "" {
		if l.cfg.DriverResolver == nil {
			return nil, nil, nil, nil, nil, "", fmt.Errorf("resolve lead driver %q: driver resolver is not configured", requestedDriverID)
		}
		driverCfg, err := l.cfg.DriverResolver(ctx, requestedDriverID)
		if err != nil {
			return nil, nil, nil, nil, nil, "", fmt.Errorf("resolve lead driver %q: %w", requestedDriverID, err)
		}
		profile.DriverID = requestedDriverID
		profile.Driver = cloneLeadDriverConfig(driverCfg)
		if !profile.Driver.CapabilitiesMax.Covers(profile.EffectiveCapabilities()) {
			return nil, nil, nil, nil, nil, "", fmt.Errorf("%w: profile %q exceeds selected driver %q capabilities_max", core.ErrCapabilityOverflow, profile.ID, requestedDriverID)
		}
	}

	// Resolve LLM config: request override > profile default.
	// "system" (or empty) means the driver inherits API keys from the
	// system environment — skip config resolution entirely.
	llmConfigID := strings.TrimSpace(requestedLLMConfigID)
	if llmConfigID == "" {
		llmConfigID = strings.TrimSpace(profile.LLMConfigID)
	}
	if !profilellm.IsSystemLLMConfig(llmConfigID) {
		if l.cfg.LLMConfigResolver == nil {
			return nil, nil, nil, nil, nil, "", fmt.Errorf("resolve llm config %q for profile %q: llm config resolver is not configured", llmConfigID, profile.ID)
		}
		llmCfg, err := l.cfg.LLMConfigResolver(ctx, llmConfigID)
		if err != nil {
			return nil, nil, nil, nil, nil, "", fmt.Errorf("resolve llm config %q for profile %q: %w", llmConfigID, profile.ID, err)
		}
		if err := profilellm.ValidateDriverProviderCompatibility(profile.DriverID, profile.Driver.LaunchCommand, profile.Driver.LaunchArgs, llmCfg.Type); err != nil {
			return nil, nil, nil, nil, nil, "", fmt.Errorf("profile %q driver %q incompatible with llm config %q: %w", profile.ID, profile.DriverID, llmConfigID, err)
		}
		profile.Driver.Env = profilellm.MergeEnv(profilellm.BuildEnv(profilellm.ProviderConfig{
			ID:                   llmCfg.ID,
			Type:                 llmCfg.Type,
			BaseURL:              llmCfg.BaseURL,
			APIKey:               llmCfg.APIKey,
			Model:                llmCfg.Model,
			Temperature:          llmCfg.Temperature,
			MaxOutputTokens:      llmCfg.MaxOutputTokens,
			ReasoningEffort:      llmCfg.ReasoningEffort,
			ThinkingBudgetTokens: llmCfg.ThinkingBudgetTokens,
		}), profile.Driver.Env)
	}

	// Build launch config: profile → env merge → sandbox.
	launchCfg, err := acpclient.PrepareLaunch(ctx, acpclient.BootstrapConfig{
		Profile: profile,
		WorkDir: workDir,
	})
	if err != nil {
		return nil, nil, nil, nil, nil, "", err
	}

	if !acpclient.UsesInProcAdapterProfile(profile) {
		sb := l.cfg.Sandbox
		if sb == nil {
			sb = v2sandbox.NoopSandbox{}
		}
		launchCfg, err = sb.Prepare(ctx, v2sandbox.PrepareInput{
			Profile: profile,
			Launch:  launchCfg,
			Scope:   scope,
		})
		if err != nil {
			return nil, nil, nil, nil, nil, "", fmt.Errorf("prepare sandbox: %w", err)
		}
	}

	bridge := eventbridge.New(l.cfg.Bus, core.EventChatOutput, eventbridge.Scope{
		SessionID: publicSessionID,
	})
	events := &suppressibleEventHandler{inner: bridge}

	handler := newLeadChatHandler(workDir, l.cfg.Bus, l.broker)
	client, err := l.cfg.NewClient(launchCfg, handler, acpclient.WithEventHandler(events))
	if err != nil {
		return nil, nil, nil, nil, nil, "", fmt.Errorf("launch lead agent: %w", err)
	}

	initCtx, initCancel := context.WithTimeout(ctx, 30*time.Second)
	defer initCancel()

	if err := client.Initialize(initCtx, acpclient.InitCapabilities(profile)); err != nil {
		_ = client.Close(context.Background())
		return nil, nil, nil, nil, nil, "", fmt.Errorf("initialize lead agent: %w", err)
	}

	sessionCwd := launchCfg.SessionCwd
	if sessionCwd == "" {
		sessionCwd = workDir
	}
	return client, handler, bridge, events, profile, sessionCwd, nil
}

func cloneLeadProfile(profile *core.AgentProfile) *core.AgentProfile {
	if profile == nil {
		return nil
	}
	cloned := *profile
	cloned.Driver = cloneLeadDriverConfig(&profile.Driver)
	if profile.Capabilities != nil {
		cloned.Capabilities = append([]string(nil), profile.Capabilities...)
	}
	if profile.ActionsAllowed != nil {
		cloned.ActionsAllowed = append([]core.AgentAction(nil), profile.ActionsAllowed...)
	}
	if profile.Skills != nil {
		cloned.Skills = append([]string(nil), profile.Skills...)
	}
	if profile.MCP.Tools != nil {
		cloned.MCP.Tools = append([]string(nil), profile.MCP.Tools...)
	}
	return &cloned
}

func cloneLeadDriverConfig(driver *core.DriverConfig) core.DriverConfig {
	if driver == nil {
		return core.DriverConfig{}
	}
	cloned := *driver
	if driver.LaunchArgs != nil {
		cloned.LaunchArgs = append([]string(nil), driver.LaunchArgs...)
	}
	if driver.Env != nil {
		cloned.Env = make(map[string]string, len(driver.Env))
		for k, v := range driver.Env {
			cloned.Env[k] = v
		}
	}
	return cloned
}

func (l *LeadAgent) removeSession(sessionID string) {
	if sessionID == "" {
		return
	}
	l.takePending(sessionID) // discard any orphaned pending message
	l.mu.Lock()
	sess, ok := l.sessions[sessionID]
	if ok {
		delete(l.sessions, sessionID)
	}
	l.mu.Unlock()
	if sess != nil {
		sess.close()
	}
}

func (l *LeadAgent) resetSessionIdle(sessionID string, sess *leadSession) {
	if l.cfg.IdleTTL <= 0 {
		return
	}
	sess.resetIdleTimer(l.cfg.IdleTTL, func() {
		l.removeSession(sessionID)
	})
}

func (l *LeadAgent) appendMessage(sessionID, role, content string) {
	content = strings.TrimSpace(content)
	if sessionID == "" || content == "" {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	record := l.catalog[sessionID]
	if record == nil {
		now := time.Now().UTC()
		record = &persistedLeadSession{
			SessionID: sessionID,
			CreatedAt: now,
			UpdatedAt: now,
		}
		l.catalog[sessionID] = record
	}
	record.Messages = append(record.Messages, chatapp.Message{
		Role:    role,
		Content: content,
		Time:    time.Now().UTC(),
	})
	if record.Title == "" && role == "user" {
		record.Title = buildLeadTitle(content)
	}
	// If the user message is long enough, asynchronously generate a better title via LLM.
	if role == "user" && l.cfg.LLM != nil && len([]rune(strings.TrimSpace(content))) > 10 {
		// Snapshot recent messages for context.
		msgs := record.Messages
		if len(msgs) > 6 {
			msgs = msgs[len(msgs)-6:]
		}
		snapshot := make([]chatapp.Message, len(msgs))
		copy(snapshot, msgs)
		go l.generateSessionTitle(sessionID, snapshot)
	}
	record.UpdatedAt = time.Now().UTC()
	_ = l.saveCatalogLocked()
}

func (l *LeadAgent) cloneRecordLocked(sessionID string) *persistedLeadSession {
	record := l.catalog[sessionID]
	if record == nil {
		return nil
	}
	cloned := *record
	cloned.Messages = append([]chatapp.Message(nil), record.Messages...)
	return &cloned
}

func (l *LeadAgent) saveCatalogLocked() error {
	return saveLeadCatalog(l.catalogPath, l.catalog)
}

// CreatePR creates a PR/MR for the session's branch. It tries configured SCM
// providers first (token-based), then falls back to the gh CLI for GitHub repos.
func (l *LeadAgent) CreatePR(ctx context.Context, sessionID, title, body string) (*chatapp.GitStats, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("session_id is required")
	}

	l.mu.Lock()
	record := l.catalog[sessionID]
	l.mu.Unlock()
	if record == nil {
		return nil, errors.New("session not found")
	}
	if record.Branch == "" {
		return nil, errors.New("session has no branch (not a worktree session)")
	}
	if record.PrURL != "" {
		return nil, fmt.Errorf("session already has a PR: %s", record.PrURL)
	}

	if strings.TrimSpace(title) == "" {
		title = record.Title
	}
	if strings.TrimSpace(title) == "" {
		title = record.Branch
	}

	// Try configured SCM provider (token-based) first.
	if provider, repo, err := l.detectSCMProvider(ctx, record.RepoPath); err == nil {
		cr, _, createErr := provider.EnsureOpen(ctx, repo, flowapp.EnsureOpenInput{
			Head:  record.Branch,
			Base:  "main",
			Title: strings.TrimSpace(title),
			Body:  strings.TrimSpace(body),
		})
		if createErr == nil {
			return l.savePR(record, cr.URL, cr.Number, "open"), nil
		}
		slog.Warn("SCM provider create PR failed, trying gh CLI fallback", "error", createErr)
	}

	// Fallback: use gh CLI.
	prURL, prNumber, err := ghCLICreatePR(ctx, record.RepoPath, record.Branch, strings.TrimSpace(title), strings.TrimSpace(body))
	if err != nil {
		return nil, fmt.Errorf("create PR failed: %w", err)
	}
	return l.savePR(record, prURL, prNumber, "open"), nil
}

func (l *LeadAgent) SubmitCode(ctx context.Context, sessionID string, message string) (*chatapp.GitStats, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("session_id is required")
	}

	l.mu.Lock()
	record := l.catalog[sessionID]
	l.mu.Unlock()
	if record == nil {
		return nil, errors.New("session not found")
	}
	if record.Branch == "" {
		return nil, errors.New("session has no branch (not a worktree session)")
	}
	if strings.TrimSpace(record.WorkDir) == "" {
		return nil, errors.New("session has no working directory")
	}

	commitMessage := strings.TrimSpace(message)
	if commitMessage == "" {
		title := strings.TrimSpace(record.Title)
		if title == "" {
			title = record.Branch
		}
		commitMessage = "chore(chat): " + title
	}

	if err := gitRun(ctx, record.WorkDir, nil, "add", "-A"); err != nil {
		return nil, err
	}
	hasChanges, err := gitHasChanges(ctx, record.WorkDir)
	if err != nil {
		return nil, err
	}
	if !hasChanges {
		if strings.TrimSpace(record.HeadSHA) != "" {
			return l.buildGitStats(record), nil
		}
		return nil, errors.New("no changes to submit")
	}

	if err := gitRun(ctx, record.WorkDir, nil,
		"-c", "user.name=ai-flow",
		"-c", "user.email=ai-flow@local",
		"commit", "-m", commitMessage,
	); err != nil {
		return nil, err
	}

	if err := l.pushChatBranch(ctx, record.WorkDir, record.Branch); err != nil {
		return nil, err
	}

	sha, err := gitOutput(ctx, record.WorkDir, nil, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	return l.saveSubmitted(record, strings.TrimSpace(sha)), nil
}

// RefreshPR queries the SCM provider or gh CLI to update the PR state.
func (l *LeadAgent) RefreshPR(ctx context.Context, sessionID string) (*chatapp.GitStats, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, errors.New("session_id is required")
	}

	l.mu.Lock()
	record := l.catalog[sessionID]
	l.mu.Unlock()
	if record == nil {
		return nil, errors.New("session not found")
	}
	if record.PrURL == "" || record.PrNumber <= 0 {
		return nil, errors.New("session has no PR to refresh")
	}

	// Try configured SCM provider first.
	if provider, repo, err := l.detectSCMProvider(ctx, record.RepoPath); err == nil {
		state, getErr := provider.GetState(ctx, repo, record.PrNumber)
		if getErr == nil {
			return l.savePRState(record, state), nil
		}
		slog.Warn("SCM provider refresh PR failed, trying gh CLI fallback", "error", getErr)
	}

	// Fallback: use gh CLI.
	state, err := ghCLIGetPRState(ctx, record.RepoPath, record.PrNumber)
	if err != nil {
		return nil, fmt.Errorf("refresh PR state failed: %w", err)
	}
	return l.savePRState(record, state), nil
}

func gitHasChanges(ctx context.Context, dir string) (bool, error) {
	out, err := gitOutput(ctx, dir, nil, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

func gitRun(ctx context.Context, dir string, extraEnv []string, args ...string) error {
	_, err := gitOutput(ctx, dir, extraEnv, args...)
	return err
}

func gitOutput(ctx context.Context, dir string, extraEnv []string, args ...string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}

func (l *LeadAgent) savePR(record *persistedLeadSession, url string, number int, state string) *chatapp.GitStats {
	l.mu.Lock()
	record.PrURL = url
	record.PrNumber = number
	record.PrState = state
	record.UpdatedAt = time.Now().UTC()
	_ = l.saveCatalogLocked()
	l.mu.Unlock()
	return l.buildGitStats(record)
}

func (l *LeadAgent) saveSubmitted(record *persistedLeadSession, headSHA string) *chatapp.GitStats {
	l.mu.Lock()
	record.HeadSHA = strings.TrimSpace(headSHA)
	record.UpdatedAt = time.Now().UTC()
	_ = l.saveCatalogLocked()
	l.mu.Unlock()
	return l.buildGitStats(record)
}

func (l *LeadAgent) savePRState(record *persistedLeadSession, state string) *chatapp.GitStats {
	l.mu.Lock()
	record.PrState = state
	record.UpdatedAt = time.Now().UTC()
	_ = l.saveCatalogLocked()
	l.mu.Unlock()
	return l.buildGitStats(record)
}

func (l *LeadAgent) pushChatBranch(ctx context.Context, workDir, branch string) error {
	pat := strings.TrimSpace(l.cfg.GitPAT)
	if pat == "" {
		return gitRun(ctx, workDir, nil, "push", "-u", "origin", branch)
	}

	askpassPath, cleanup, err := writeLeadGitAskPass(pat)
	if err != nil {
		return err
	}
	defer cleanup()

	env := []string{
		"GIT_ASKPASS=" + askpassPath,
		"GIT_TERMINAL_PROMPT=0",
	}
	return gitRun(ctx, workDir, env, "push", "-u", "origin", branch)
}

func writeLeadGitAskPass(token string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "ai-workflow-chat-askpass-*")
	if err != nil {
		return "", nil, fmt.Errorf("create askpass dir: %w", err)
	}

	scriptPath := filepath.Join(dir, "askpass.sh")
	script := fmt.Sprintf("#!/bin/sh\ncase \"$1\" in\n*sername*) echo \"x-access-token\" ;;\n*) echo \"%s\" ;;\nesac\n",
		strings.ReplaceAll(token, "\"", "\\\""))
	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		_ = os.RemoveAll(dir)
		return "", nil, fmt.Errorf("write askpass script: %w", err)
	}

	cmdPath := filepath.Join(dir, "askpass.cmd")
	cmdScript := strings.Join([]string{
		"@echo off",
		"set prompt=%~1",
		"echo %prompt% | findstr /i \"username\" >nul",
		"if %errorlevel%==0 (",
		"  echo x-access-token",
		"  exit /b 0",
		")",
		"echo " + token,
		"",
	}, "\r\n")
	_ = os.WriteFile(cmdPath, []byte(cmdScript), 0o600)
	return scriptPath, func() { _ = os.RemoveAll(dir) }, nil
}

// detectSCMProvider resolves the SCM provider and repo from the git remote.
// Returns an error if no token or no matching provider — callers should fall
// back to gh CLI on error.
func (l *LeadAgent) detectSCMProvider(ctx context.Context, repoPath string) (flowapp.ChangeRequestProvider, flowapp.ChangeRequestRepo, error) {
	token := strings.TrimSpace(l.cfg.GitPAT)
	if token == "" || l.cfg.ChangeRequestProviders == nil {
		return nil, flowapp.ChangeRequestRepo{}, errors.New("no SCM token configured")
	}

	originURL, err := gitRemoteOriginURL(repoPath)
	if err != nil {
		return nil, flowapp.ChangeRequestRepo{}, err
	}

	for _, p := range l.cfg.ChangeRequestProviders(token) {
		if p == nil {
			continue
		}
		repo, ok, detectErr := p.Detect(ctx, originURL)
		if detectErr != nil {
			return nil, flowapp.ChangeRequestRepo{}, detectErr
		}
		if ok {
			return p, repo, nil
		}
	}
	return nil, flowapp.ChangeRequestRepo{}, fmt.Errorf("no SCM provider matched remote: %s", originURL)
}

// gitRemoteOriginURL returns the origin remote URL for a git repo.
func gitRemoteOriginURL(repoPath string) (string, error) {
	dir := strings.TrimSpace(repoPath)
	if dir == "" {
		return "", errors.New("repo path is empty")
	}
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git remote get-url origin failed: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ---------------------------------------------------------------------------
// gh CLI fallback — uses the system's `gh` tool with its own auth.
// ---------------------------------------------------------------------------

// ghCLICreatePR creates a GitHub PR using the gh CLI. Returns (url, number, error).
func ghCLICreatePR(ctx context.Context, repoPath, head, title, body string) (string, int, error) {
	args := []string{"pr", "create", "--head", head, "--base", "main", "--title", title}
	if body != "" {
		args = append(args, "--body", body)
	} else {
		args = append(args, "--body", "")
	}

	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", 0, fmt.Errorf("gh pr create failed: %s: %w", strings.TrimSpace(string(out)), err)
	}

	prURL := strings.TrimSpace(string(out))
	prNumber := extractPRNumberFromURL(prURL)
	if prURL == "" || prNumber <= 0 {
		return "", 0, fmt.Errorf("gh pr create returned unexpected output: %s", prURL)
	}
	return prURL, prNumber, nil
}

// ghCLIGetPRState queries PR state using the gh CLI. Returns "open", "merged", or "closed".
func ghCLIGetPRState(ctx context.Context, repoPath string, number int) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", "pr", "view", strconv.Itoa(number), "--json", "state")
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gh pr view failed: %s: %w", strings.TrimSpace(string(out)), err)
	}

	var result struct {
		State string `json:"state"`
	}
	if jsonErr := json.Unmarshal(out, &result); jsonErr != nil {
		return "", fmt.Errorf("parse gh pr view output failed: %w", jsonErr)
	}

	state := strings.ToLower(strings.TrimSpace(result.State))
	switch state {
	case "merged":
		return "merged", nil
	case "closed":
		return "closed", nil
	default:
		return "open", nil
	}
}

// extractPRNumberFromURL extracts the PR number from a GitHub PR URL like
// "https://github.com/owner/repo/pull/47".
func extractPRNumberFromURL(prURL string) int {
	parts := strings.Split(strings.TrimRight(prURL, "/"), "/")
	if len(parts) < 2 {
		return 0
	}
	// Last segment after "pull/" should be the number.
	n, _ := strconv.Atoi(parts[len(parts)-1])
	return n
}

// buildGitStats builds GitStats from a persisted record, including PR overlay.
func (l *LeadAgent) buildGitStats(record *persistedLeadSession) *chatapp.GitStats {
	stats := computeGitStats(record.WorkDir)
	if stats == nil {
		stats = &chatapp.GitStats{}
	}
	stats.HeadSHA = strings.TrimSpace(record.HeadSHA)
	if record.PrURL != "" {
		stats.PrURL = record.PrURL
		stats.PrNumber = record.PrNumber
		stats.PrState = record.PrState
		if record.PrState == "merged" {
			stats.Merged = true
		}
	}
	return stats
}

func (s *leadSession) stopIdleTimer() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.idleTimer != nil {
		s.idleTimer.Stop()
		s.idleTimer = nil
	}
}

func (s *leadSession) resetIdleTimer(d time.Duration, onExpire func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if s.idleTimer != nil {
		s.idleTimer.Stop()
	}
	s.idleTimer = time.AfterFunc(d, onExpire)
}

func (s *leadSession) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *leadSession) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	if s.idleTimer != nil {
		s.idleTimer.Stop()
		s.idleTimer = nil
	}
	client := s.client
	s.mu.Unlock()

	if client != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = client.Close(closeCtx)
	}
	// NOTE: workspace (sandbox dir / worktree) is intentionally NOT cleaned
	// up here.  The agent process is recycled but the workspace survives so
	// the session can be resumed later.  Workspace cleanup only happens on
	// explicit session deletion via DeleteSession.
}

func resolveLeadWorkDir(workDir string) (string, error) {
	if strings.TrimSpace(workDir) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve working directory: %w", err)
		}
		workDir = cwd
	}
	abs, err := filepath.Abs(workDir)
	if err != nil {
		return "", fmt.Errorf("resolve working directory %q: %w", workDir, err)
	}
	return abs, nil
}

// resolveIsolatedWorkDir provisions an isolated working directory for a new
// chat session.  It returns (workDir, isolation, repoPath, branch, error).
//
//   - If the caller provides an explicit WorkDir, it is used as-is (isolation="").
//   - If UseWorktree is explicitly false, the project's git root directory is
//     used directly without creating a worktree (isolation="").
//   - If a project with a git ResourceSpace is selected (and UseWorktree is
//     nil or true), a git worktree is created so the agent never touches the
//     default branch (isolation="worktree").
//   - Otherwise a temporary sandbox directory is created under DataDir
//     (isolation="sandbox").
func (l *LeadAgent) resolveIsolatedWorkDir(ctx context.Context, req chatapp.Request) (string, string, string, string, error) {
	// Caller-provided explicit path — honour it, no isolation.
	if strings.TrimSpace(req.WorkDir) != "" {
		abs, err := filepath.Abs(req.WorkDir)
		if err != nil {
			return "", "", "", "", fmt.Errorf("resolve working directory %q: %w", req.WorkDir, err)
		}
		return abs, "", "", "", nil
	}

	// Project with git space.
	if req.ProjectID > 0 && l.cfg.ResourceSpaceStore != nil {
		spaces, err := l.cfg.ResourceSpaceStore.ListResourceSpaces(ctx, req.ProjectID)
		if err != nil {
			slog.Warn("lead chat: list resource spaces failed", "project_id", req.ProjectID, "error", err)
		}
		for _, space := range spaces {
			if space.Kind != core.ResourceKindGit {
				continue
			}

			// Resolve to a local path: use local directory when available,
			// otherwise auto-clone the remote URL into dataDir/repos/<id>.
			repoPath, err := l.resolveLocalRepoPath(ctx, req.ProjectID, space)
			if err != nil {
				slog.Warn("lead chat: resolve local repo failed", "project_id", req.ProjectID, "error", err)
				continue
			}
			if repoPath == "" {
				continue
			}

			// UseWorktree explicitly disabled → run directly in repo directory.
			if req.UseWorktree != nil && !*req.UseWorktree {
				abs, err := filepath.Abs(repoPath)
				if err != nil {
					return "", "", "", "", fmt.Errorf("resolve project directory %q: %w", repoPath, err)
				}
				slog.Info("lead chat: using project directory directly (worktree disabled)", "project_id", req.ProjectID, "path", abs)
				return abs, "", repoPath, "", nil
			}

			// Default / UseWorktree=true → create worktree under dataDir.
			slug := l.generateBranchSlug(ctx, req.Message)
			branchName := fmt.Sprintf("ai-chat/%s", slug)
			worktreePath := filepath.Join(l.cfg.DataDir, "worktrees", fmt.Sprintf("%d", req.ProjectID), "chat-"+slug)

			runner := workspacegit.NewRunner(repoPath)
			if err := runner.WorktreeAdd(worktreePath, branchName, ""); err != nil {
				return "", "", "", "", fmt.Errorf("create chat worktree for project %d: %w", req.ProjectID, err)
			}
			slog.Info("lead chat: created worktree", "project_id", req.ProjectID, "path", worktreePath, "branch", branchName)
			return worktreePath, "worktree", repoPath, branchName, nil
		}
		// No git binding — fall through to sandbox.
	}

	// No project or no git binding → sandbox temp dir.
	sandboxDir, err := l.createSandboxDir()
	if err != nil {
		return "", "", "", "", err
	}
	return sandboxDir, "sandbox", "", "", nil
}

// resolveLocalRepoPath returns a local filesystem path for the git space.
// For spaces that already point at a local directory it returns that path
// directly.  For remote-only URLs (git@…, https://…) it clones the
// repository into <dataDir>/repos/<projectID>/ and returns the clone path.
func (l *LeadAgent) resolveLocalRepoPath(ctx context.Context, projectID int64, space *core.ResourceSpace) (string, error) {
	if localPath := resolveLeadGitSpaceLocalPath(space); localPath != "" {
		return localPath, nil
	}

	remoteURL := extractLeadGitRemoteURL(space)
	if remoteURL == "" {
		return "", nil
	}

	// Determine ref to checkout (base_branch from space config).
	var ref string
	if space.Config != nil {
		if v, ok := space.Config["base_branch"].(string); ok {
			ref = strings.TrimSpace(v)
		}
	}

	cloneDir := filepath.Join(l.cfg.DataDir, "repos", fmt.Sprintf("%d", projectID))
	cloner := workspaceclone.New()
	result, err := cloner.Clone(ctx, workspaceclone.CloneRequest{
		RemoteURL:  remoteURL,
		TargetPath: cloneDir,
		Ref:        ref,
	})
	if err != nil {
		return "", fmt.Errorf("clone %s into %s: %w", remoteURL, cloneDir, err)
	}
	slog.Info("lead chat: ensured local clone", "project_id", projectID, "remote", remoteURL, "path", result.RepoPath)
	return result.RepoPath, nil
}

// resolveLeadGitSpaceLocalPath returns a local filesystem path configured
// in the space (work_dir / local_path / clone_dir / non-remote RootURI).
// Returns "" when no local path is available.
func resolveLeadGitSpaceLocalPath(space *core.ResourceSpace) string {
	if space == nil {
		return ""
	}
	if space.Config != nil {
		for _, key := range []string{"work_dir", "local_path", "clone_dir"} {
			if value, ok := space.Config[key].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	rootURI := strings.TrimSpace(space.RootURI)
	if rootURI == "" || looksLikeRemoteLeadGitURI(rootURI) {
		return ""
	}
	return rootURI
}

// extractLeadGitRemoteURL returns the remote URL from a git ResourceSpace,
// or "" if the RootURI is not a remote git address.
func extractLeadGitRemoteURL(space *core.ResourceSpace) string {
	if space == nil {
		return ""
	}
	rootURI := strings.TrimSpace(space.RootURI)
	if rootURI != "" && looksLikeRemoteLeadGitURI(rootURI) {
		return rootURI
	}
	return ""
}

func looksLikeRemoteLeadGitURI(uri string) bool {
	if strings.Contains(uri, "://") {
		return true
	}
	return strings.HasPrefix(uri, "git@") && strings.Contains(uri, ":")
}

// createSandboxDir creates a temporary directory under DataDir for chat sessions
// that do not have a project context.
func (l *LeadAgent) createSandboxDir() (string, error) {
	base := filepath.Join(l.cfg.DataDir, "chat-sandboxes")
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", fmt.Errorf("create chat sandbox base dir: %w", err)
	}
	dir, err := os.MkdirTemp(base, "sess-")
	if err != nil {
		return "", fmt.Errorf("create chat sandbox dir: %w", err)
	}
	slog.Info("lead chat: created sandbox dir", "path", dir)
	return dir, nil
}

// generateBranchSlug creates a short, git-safe branch slug from the user's
// message.  If an LLM client is configured it asks the model to produce a
// 2-4 word English slug; otherwise it falls back to a timestamp.
func (l *LeadAgent) generateBranchSlug(ctx context.Context, message string) string {
	fallback := strings.ReplaceAll(time.Now().UTC().Format("20060102-150405.000"), ".", "")

	if l.cfg.LLM == nil || strings.TrimSpace(message) == "" {
		return fallback
	}

	llmCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	prompt := fmt.Sprintf(
		"Generate a very short git branch slug (2-4 lowercase English words separated by hyphens, no special characters) that summarises the following task. "+
			"Reply with ONLY the slug, nothing else.\n\nTask: %s", message)

	raw, err := l.cfg.LLM.CompleteText(llmCtx, prompt)
	if err != nil {
		slog.Warn("lead chat: LLM branch slug generation failed, using timestamp", "error", err)
		return fallback
	}

	slug := sanitizeBranchSlug(strings.TrimSpace(raw))
	if slug == "" {
		return fallback
	}
	// Append a short timestamp suffix to guarantee uniqueness.
	return slug + "-" + time.Now().UTC().Format("0102-1504")
}

var branchSlugRe = regexp.MustCompile(`[^a-z0-9-]`)

// sanitizeBranchSlug normalises a string into a valid git branch name component.
func sanitizeBranchSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = branchSlugRe.ReplaceAllString(s, "-")
	// Collapse multiple hyphens.
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	s = strings.Trim(s, "-")
	// Cap length.
	if len(s) > 48 {
		s = s[:48]
		s = strings.TrimRight(s, "-")
	}
	return s
}

// cleanupIsolatedDir removes an isolated working directory that was provisioned
// for a chat session.
func cleanupIsolatedDir(isolation, workDir, repoPath string) {
	switch isolation {
	case "sandbox":
		if err := os.RemoveAll(workDir); err != nil {
			slog.Warn("lead chat: remove sandbox dir failed", "path", workDir, "error", err)
		}
	case "worktree":
		if repoPath != "" {
			runner := workspacegit.NewRunner(repoPath)
			if err := runner.WorktreeRemove(workDir); err != nil {
				slog.Warn("lead chat: remove worktree failed", "path", workDir, "error", err)
			}
		}
	}
}

// buildPromptBlocks constructs ACP ContentBlock slice from the user message
// and any attachments.  Images are sent as native image content blocks; other
// files are saved to the workspace and referenced via resource_link.
func buildPromptBlocks(message string, attachments []chatapp.Attachment, workDir string) []acpproto.ContentBlock {
	blocks := []acpproto.ContentBlock{
		{Text: &acpproto.ContentBlockText{Text: message}},
	}

	for _, att := range attachments {
		if strings.TrimSpace(att.Data) == "" {
			continue
		}
		if isImageMime(att.MimeType) {
			blocks = append(blocks, acpproto.ContentBlock{
				Image: &acpproto.ContentBlockImage{
					Data:     att.Data,
					MimeType: att.MimeType,
				},
			})
			continue
		}

		// Non-image file: save to workspace and reference via resource_link.
		filePath, err := saveAttachmentToWorkspace(att, workDir)
		if err != nil {
			slog.Warn("lead chat: save attachment failed, skipping", "name", att.Name, "error", err)
			continue
		}
		fileURI := "file://" + filePath
		var mimeType *string
		if v := strings.TrimSpace(att.MimeType); v != "" {
			mimeType = &v
		}
		blocks = append(blocks, acpproto.ContentBlock{
			ResourceLink: &acpproto.ContentBlockResourceLink{
				Uri:      fileURI,
				Name:     att.Name,
				MimeType: mimeType,
			},
		})
	}

	return blocks
}

func isImageMime(mime string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(mime)), "image/")
}

// saveAttachmentToWorkspace decodes a base64 attachment and saves it under
// {workDir}/.uploads/{name}.  Returns the absolute path of the saved file.
func saveAttachmentToWorkspace(att chatapp.Attachment, workDir string) (string, error) {
	uploadsDir := filepath.Join(workDir, ".uploads")
	if err := os.MkdirAll(uploadsDir, 0o755); err != nil {
		return "", fmt.Errorf("create uploads dir: %w", err)
	}

	data, err := base64.StdEncoding.DecodeString(att.Data)
	if err != nil {
		return "", fmt.Errorf("decode attachment %q: %w", att.Name, err)
	}

	name := strings.TrimSpace(att.Name)
	if name == "" {
		name = "upload"
	}
	// Sanitise filename to prevent path traversal.
	name = filepath.Base(name)

	dst := filepath.Join(uploadsDir, name)
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return "", fmt.Errorf("write attachment %q: %w", name, err)
	}
	return dst, nil
}

func buildLeadTitle(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return "新会话"
	}
	runes := []rune(message)
	if len(runes) > 24 {
		return string(runes[:24])
	}
	return message
}

// generateSessionTitle uses the LLM to produce a concise title and updates the
// session catalog + emits a chat.session_title event so the frontend can refresh.
func (l *LeadAgent) generateSessionTitle(sessionID string, messages []chatapp.Message) {
	ctx, cancel := context.WithTimeout(l.backgroundContext(), 15*time.Second)
	defer cancel()

	var sb strings.Builder
	for _, m := range messages {
		sb.WriteString(m.Role)
		sb.WriteString(": ")
		sb.WriteString(m.Content)
		sb.WriteByte('\n')
	}

	prompt := fmt.Sprintf(`Generate a concise chat session title (under 20 characters, in the same language as the conversation) that captures the current topic. Return ONLY the title text, nothing else.

Conversation:
---
%s---`, sb.String())

	title, err := l.cfg.LLM.CompleteText(ctx, prompt)
	if err != nil {
		slog.Warn("auto-generate session title failed", "session_id", sessionID, "error", err)
		return
	}
	title = strings.TrimSpace(title)
	title = strings.Trim(title, "\"'`")
	title = strings.TrimSpace(title)
	if title == "" {
		return
	}

	l.mu.Lock()
	record := l.catalog[sessionID]
	if record != nil {
		record.Title = title
		record.UpdatedAt = time.Now().UTC()
		_ = l.saveCatalogLocked()
	}
	l.mu.Unlock()

	l.cfg.Bus.Publish(ctx, core.Event{
		Type:      core.EventChatSessionTitle,
		Timestamp: time.Now().UTC(),
		Data: map[string]any{
			"session_id": sessionID,
			"title":      title,
		},
	})
}

func (l *LeadAgent) runningSessionSet() map[string]bool {
	l.activeMu.Lock()
	defer l.activeMu.Unlock()
	out := make(map[string]bool, len(l.activeRuns))
	for sessionID := range l.activeRuns {
		out[sessionID] = true
	}
	return out
}

func buildSessionSummary(record *persistedLeadSession, live, running bool) chatapp.SessionSummary {
	status := "closed"
	if running {
		status = "running"
	} else if live {
		status = "alive"
	}
	summary := chatapp.SessionSummary{
		SessionID:    record.SessionID,
		Title:        record.Title,
		WorkDir:      record.WorkDir,
		Branch:       record.Branch,
		WSPath:       buildChatWSPath(record.SessionID),
		ProjectID:    record.ProjectID,
		ProjectName:  record.ProjectName,
		ProfileID:    record.ProfileID,
		ProfileName:  record.ProfileName,
		DriverID:     record.DriverID,
		CreatedAt:    record.CreatedAt,
		UpdatedAt:    record.UpdatedAt,
		Status:       status,
		Archived:     record.Archived,
		MessageCount: len(record.Messages),
	}

	// Best-effort git diff stats for the session's working directory.
	if record.WorkDir != "" && record.Branch != "" {
		if stats := computeGitStats(record.WorkDir); stats != nil {
			summary.Git = stats
		}
	}

	// Overlay PR metadata captured from the event stream.
	if record.PrURL != "" {
		if summary.Git == nil {
			summary.Git = &chatapp.GitStats{}
		}
		summary.Git.PrURL = record.PrURL
		summary.Git.PrNumber = record.PrNumber
		summary.Git.PrState = record.PrState
		if record.PrState == "merged" {
			summary.Git.Merged = true
		}
	}

	return summary
}

// computeGitStats computes diff statistics (vs main) for a session's worktree.
// Returns nil on any error — callers should treat this as optional data.
func computeGitStats(workDir string) *chatapp.GitStats {
	stats := &chatapp.GitStats{}
	hasData := false

	// Diff stats vs main — requires live worktree.
	if workDir != "" {
		if _, err := os.Stat(workDir); err == nil {
			// git diff --shortstat main produces output like:
			//   3 files changed, 120 insertions(+), 45 deletions(-)
			cmd := exec.Command("git", "diff", "--shortstat", "main")
			cmd.Dir = workDir
			out, err := cmd.Output()
			if err == nil {
				parseShortstat(strings.TrimSpace(string(out)), stats)
				if stats.FilesChanged > 0 || stats.Additions > 0 || stats.Deletions > 0 {
					hasData = true
				}
			}
		}
	}

	// Merged status is determined solely by PR state (overlaid by the
	// caller) to avoid false positives when a branch has just been created
	// from main and has no commits yet.

	if !hasData {
		return nil
	}
	return stats
}

// parseShortstat parses git's --shortstat output into the given GitStats.
func parseShortstat(line string, stats *chatapp.GitStats) {
	if line == "" {
		return
	}
	for _, part := range strings.Split(line, ",") {
		part = strings.TrimSpace(part)
		fields := strings.Fields(part)
		if len(fields) < 2 {
			continue
		}
		n, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		switch {
		case strings.Contains(fields[1], "file"):
			stats.FilesChanged = n
		case strings.Contains(fields[1], "insertion"):
			stats.Additions = n
		case strings.Contains(fields[1], "deletion"):
			stats.Deletions = n
		}
	}
}

func (l *LeadAgent) captureSessionState(sessionID string, update acpclient.SessionUpdate) {
	id := strings.TrimSpace(sessionID)
	if id == "" {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	record := l.catalog[id]
	if record == nil {
		return
	}

	changed := false
	switch strings.TrimSpace(update.Type) {
	case "available_commands_update":
		record.AvailableCommands = toChatAvailableCommands(update.Commands)
		changed = true
	case "config_option_update", "config_options_update":
		record.ConfigOptions = toChatConfigOptions(update.ConfigOptions)
		changed = true
	case "current_mode_update":
		if modeId := strings.TrimSpace(update.CurrentModeId); modeId != "" {
			if record.Modes == nil {
				record.Modes = &chatapp.SessionModeState{}
			}
			record.Modes.CurrentModeId = modeId
			changed = true
		}
	}
	if !changed {
		return
	}
	record.UpdatedAt = time.Now().UTC()
	_ = l.saveCatalogLocked()
}

func toChatModeState(state *acpproto.SessionModeState) *chatapp.SessionModeState {
	if state == nil {
		return nil
	}
	modes := make([]chatapp.SessionMode, 0, len(state.AvailableModes))
	for _, m := range state.AvailableModes {
		mode := chatapp.SessionMode{
			ID:   strings.TrimSpace(string(m.Id)),
			Name: strings.TrimSpace(m.Name),
		}
		if m.Description != nil {
			mode.Description = strings.TrimSpace(*m.Description)
		}
		modes = append(modes, mode)
	}
	return &chatapp.SessionModeState{
		AvailableModes: modes,
		CurrentModeId:  strings.TrimSpace(string(state.CurrentModeId)),
	}
}

func toChatAvailableCommands(items []acpproto.AvailableCommand) []chatapp.AvailableCommand {
	if items == nil {
		return nil
	}
	out := make([]chatapp.AvailableCommand, 0, len(items))
	for _, item := range items {
		cmd := chatapp.AvailableCommand{
			Name:        strings.TrimSpace(item.Name),
			Description: strings.TrimSpace(item.Description),
		}
		if item.Input != nil && item.Input.Unstructured != nil {
			cmd.Input = &chatapp.AvailableCommandInput{
				Hint: strings.TrimSpace(item.Input.Unstructured.Hint),
			}
		}
		out = append(out, cmd)
	}
	return out
}

func toChatConfigOptions(items []acpproto.SessionConfigOptionSelect) []chatapp.ConfigOption {
	if items == nil {
		return nil
	}
	out := make([]chatapp.ConfigOption, 0, len(items))
	for _, item := range items {
		option := chatapp.ConfigOption{
			ID:           strings.TrimSpace(string(item.Id)),
			Name:         strings.TrimSpace(item.Name),
			Type:         strings.TrimSpace(item.Type),
			CurrentValue: strings.TrimSpace(string(item.CurrentValue)),
		}
		if item.Description != nil {
			option.Description = strings.TrimSpace(*item.Description)
		}
		if item.Category != nil {
			option.Category = normalizeConfigCategory(item.Category)
		}
		if item.Options.Ungrouped != nil {
			for _, value := range *item.Options.Ungrouped {
				option.Options = append(option.Options, chatapp.ConfigOptionValue{
					Value:       strings.TrimSpace(string(value.Value)),
					Name:        strings.TrimSpace(value.Name),
					Description: derefTrim(value.Description),
				})
			}
		}
		if item.Options.Grouped != nil {
			for _, group := range *item.Options.Grouped {
				for _, value := range group.Options {
					option.Options = append(option.Options, chatapp.ConfigOptionValue{
						Value:       strings.TrimSpace(string(value.Value)),
						Name:        strings.TrimSpace(value.Name),
						Description: derefTrim(value.Description),
						GroupID:     strings.TrimSpace(string(group.Group)),
						GroupName:   strings.TrimSpace(group.Name),
					})
				}
			}
		}
		out = append(out, option)
	}
	return out
}

func derefTrim(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func normalizeConfigCategory(category *acpproto.SessionConfigOptionCategory) string {
	if category == nil || category.Other == nil {
		return ""
	}
	return strings.TrimSpace(string(*category.Other))
}

func buildChatWSPath(sessionID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ""
	}
	query := url.Values{}
	query.Set("types", string(core.EventChatOutput))
	query.Set("session_id", sessionID)
	return "/api/ws?" + query.Encode()
}
