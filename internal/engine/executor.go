package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/acpclient"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/eventbus"
	gitops "github.com/yoke233/ai-workflow/internal/git"
	"github.com/yoke233/ai-workflow/internal/observability"
)

// ACPEventPublisher receives events from ACP handlers.
type ACPEventPublisher interface {
	Publish(evt core.Event)
}

// ACPHandlerFactory creates ACP protocol handlers for pipeline stage execution.
type ACPHandlerFactory interface {
	NewHandler(cwd string, publisher ACPEventPublisher) acpproto.Client
	SetPermissionPolicy(handler acpproto.Client, policy []acpclient.PermissionRule)
}

// acpSessionEntry holds a live ACP client and session for cross-stage reuse.
type acpSessionEntry struct {
	client    *acpclient.Client
	sessionID acpproto.SessionId
}

type Executor struct {
	store             core.Store
	bus               *eventbus.Bus
	agents            map[string]core.AgentPlugin
	roleResolver      *acpclient.RoleResolver
	stageRoles        map[core.StageID]string
	runtime           core.RuntimePlugin
	workspace         core.WorkspacePlugin
	acpHandlerFactory ACPHandlerFactory
	logger            *slog.Logger

	sessionMu     sync.Mutex
	activeSession map[string]string

	// acpPool keeps ACP sessions alive across stages for the same run.
	// Key: "runID:stageID" where stageID is the stage that created the session.
	acpPoolMu sync.Mutex
	acpPool   map[string]*acpSessionEntry
}

func NewExecutor(
	store core.Store,
	bus *eventbus.Bus,
	agents map[string]core.AgentPlugin,
	runtime core.RuntimePlugin,
	logger *slog.Logger,
) *Executor {
	return &Executor{
		store:   store,
		bus:     bus,
		agents:  agents,
		runtime: runtime,
		logger:  logger,

		activeSession: make(map[string]string),
		acpPool:       make(map[string]*acpSessionEntry),
	}
}

func (e *Executor) SetRoleResolver(resolver *acpclient.RoleResolver) {
	e.roleResolver = resolver
}

func (e *Executor) SetACPHandlerFactory(factory ACPHandlerFactory) {
	e.acpHandlerFactory = factory
}

func (e *Executor) SetWorkspace(workspace core.WorkspacePlugin) {
	e.workspace = workspace
}

func (e *Executor) SetRunstageRoles(stageRoles map[string]string) {
	if len(stageRoles) == 0 {
		e.stageRoles = nil
		return
	}

	normalized := make(map[core.StageID]string, len(stageRoles))
	for rawStage, rawRole := range stageRoles {
		stage := core.StageID(strings.TrimSpace(rawStage))
		role := strings.TrimSpace(rawRole)
		if stage == "" || role == "" {
			continue
		}
		normalized[stage] = role
	}
	e.stageRoles = normalized
}

func (e *Executor) CreateRun(projectID, name, description, template string) (*core.Run, error) {
	stageIDs, ok := Templates[template]
	if !ok {
		return nil, fmt.Errorf("unknown template: %s", template)
	}

	stages := make([]core.StageConfig, len(stageIDs))
	for i, sid := range stageIDs {
		stages[i] = defaultStageConfig(sid)
		if role, ok := e.stageRoles[sid]; ok {
			stages[i].Role = role
		}
	}

	p := &core.Run{
		ID:              NewRunID(),
		ProjectID:       projectID,
		Name:            name,
		Description:     description,
		Template:        template,
		Status:          core.StatusQueued,
		Stages:          stages,
		Artifacts:       map[string]string{},
		Config:          map[string]any{},
		MaxTotalRetries: 5,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	if err := e.store.SaveRun(p); err != nil {
		return nil, err
	}
	return p, nil
}

func (e *Executor) Run(ctx context.Context, RunID string) error {
	return e.run(ctx, RunID, false)
}

// RunScheduled executes a Run that has already been CAS-marked as running by scheduler.
func (e *Executor) RunScheduled(ctx context.Context, RunID string) error {
	return e.run(ctx, RunID, true)
}

func (e *Executor) run(ctx context.Context, RunID string, allowAlreadyRunning bool) error {
	p, err := e.store.GetRun(RunID)
	if err != nil {
		return err
	}
	// Clean up any pooled ACP sessions when the run finishes (success, failure, or panic).
	defer e.acpPoolCleanup(p.ID)

	project, err := e.store.GetProject(p.ProjectID)
	if err != nil {
		return err
	}

	logger := e.logger
	if logger == nil {
		logger = slog.Default()
	}
	ctx, traceID := observability.EnsureTraceID(ctx, p.ID)
	issueNumber := issueNumberFromRun(p)
	prNumber := prNumberFromRunData(p)
	baseEventData := make(map[string]string, 1)
	if prNumber > 0 {
		baseEventData["pr_number"] = strconv.Itoa(prNumber)
	}
	if p.Config == nil {
		p.Config = map[string]any{}
	}
	if existingTraceID, _ := p.Config["trace_id"].(string); strings.TrimSpace(existingTraceID) == "" {
		p.Config["trace_id"] = traceID
	}

	if allowAlreadyRunning && p.Status == core.StatusInProgress {
		if p.StartedAt.IsZero() {
			p.StartedAt = time.Now()
			if err := e.store.SaveRun(p); err != nil {
				return err
			}
		}
	} else {
		if err := core.ValidateTransition(p.Status, core.StatusInProgress); err != nil {
			return err
		}
		p.Status = core.StatusInProgress
		p.StartedAt = time.Now()
		if err := e.store.SaveRun(p); err != nil {
			return err
		}
	}

	startIndex := e.resolveStartIndex(p, allowAlreadyRunning)
	for i := startIndex; i < len(p.Stages); i++ {
		stage := p.Stages[i]
		p.CurrentStage = stage.Name
		if err := e.store.SaveRun(p); err != nil {
			return err
		}
		stageStartedAt := time.Now()

		stageStartTS := time.Now()
		e.bus.Publish(core.Event{
			Type:      core.EventStageStart,
			RunID:     p.ID,
			ProjectID: p.ProjectID,
			Stage:     stage.Name,
			Data:      RunEventData(traceID, issueNumber, "stage_start", baseEventData),
			Timestamp: stageStartTS,
		})
		logger.Info("Run stage started", observability.StructuredLogArgs(observability.StructuredLogInput{
			TraceID:     traceID,
			ProjectID:   p.ProjectID,
			RunID:       p.ID,
			IssueNumber: issueNumber,
			Operation:   "stage_start",
			Latency:     0,
		})...)

		maxRetries := stage.MaxRetries
		if maxRetries < 0 {
			maxRetries = 0
		}

		stageSucceeded := false
		stageSkipped := false
		for attempt := 0; ; attempt++ {
			agentUsed := ""
			if resolvedAgent, _, resolveErr := e.resolveStageAgentName(&stage); resolveErr == nil {
				agentUsed = resolvedAgent
			}

			cp := &core.Checkpoint{
				RunID:      p.ID,
				StageName:  stage.Name,
				Status:     core.CheckpointInProgress,
				StartedAt:  time.Now(),
				AgentUsed:  agentUsed,
				RetryCount: attempt,
			}
			if err := e.store.SaveCheckpoint(cp); err != nil {
				return err
			}

			err := e.executeStage(ctx, project, p, &stage)
			cp.FinishedAt = time.Now()
			if err == nil {
				cp.Status = core.CheckpointSuccess
				if saveErr := e.store.SaveCheckpoint(cp); saveErr != nil {
					return saveErr
				}
				stageCompleteTS := time.Now()
				e.bus.Publish(core.Event{
					Type:      core.EventStageComplete,
					RunID:     p.ID,
					ProjectID: p.ProjectID,
					Stage:     stage.Name,
					Data:      RunEventData(traceID, issueNumber, "stage_complete", baseEventData),
					Timestamp: stageCompleteTS,
				})
				logger.Info("Run stage completed", observability.StructuredLogArgs(observability.StructuredLogInput{
					TraceID:     traceID,
					ProjectID:   p.ProjectID,
					RunID:       p.ID,
					IssueNumber: issueNumber,
					Operation:   "stage_complete",
					Latency:     time.Since(stageStartedAt),
				})...)
				stageSucceeded = true
				break
			}

			actionRequired, stateErr := e.isRunActionRequired(p.ID)
			if stateErr != nil {
				return stateErr
			}
			if actionRequired {
				// Pause keeps current stage in-progress for a later explicit resume.
				return nil
			}

			cp.Status = core.CheckpointFailed
			cp.Error = err.Error()
			if saveErr := e.store.SaveCheckpoint(cp); saveErr != nil {
				return saveErr
			}
			stageFailedTS := time.Now()
			e.bus.Publish(core.Event{
				Type:      core.EventStageFailed,
				RunID:     p.ID,
				ProjectID: p.ProjectID,
				Stage:     stage.Name,
				Data:      RunEventData(traceID, issueNumber, "stage_failed", baseEventData),
				Error:     err.Error(),
				Timestamp: stageFailedTS,
			})
			logger.Error("Run stage failed", observability.StructuredLogArgs(observability.StructuredLogInput{
				TraceID:     traceID,
				ProjectID:   p.ProjectID,
				RunID:       p.ID,
				IssueNumber: issueNumber,
				Operation:   "stage_failed",
				Latency:     time.Since(stageStartedAt),
			})...)

			p.TotalRetries++
			if saveErr := e.store.SaveRun(p); saveErr != nil {
				return saveErr
			}
			if p.TotalRetries >= p.MaxTotalRetries {
				return e.failRun(p, fmt.Sprintf("retry budget exhausted at stage %s: %v", stage.Name, err), err)
			}

			action, matched := EvaluateReactionRules(ReactionContext{
				Stage:    stage,
				Attempt:  attempt,
				MaxRetry: maxRetries,
				Err:      err,
			}, CompileOnFailureReactions(stage))
			if !matched {
				action = ReactionAbortRun
			}

			switch action {
			case ReactionRetry:
				if attempt < maxRetries {
					continue
				}
				return e.failRun(p, fmt.Sprintf("stage %s exhausted retries(%d): %v", stage.Name, maxRetries, err), err)
			case ReactionSkipStage:
				stageSkipped = true
				cpSkip := &core.Checkpoint{
					RunID:      p.ID,
					StageName:  stage.Name,
					Status:     core.CheckpointSkipped,
					StartedAt:  time.Now(),
					FinishedAt: time.Now(),
					AgentUsed:  agentUsed,
					RetryCount: attempt,
					Error:      err.Error(),
				}
				if saveErr := e.store.SaveCheckpoint(cpSkip); saveErr != nil {
					return saveErr
				}
			case ReactionEscalateHuman:
				p.Status = core.StatusActionRequired
				p.ErrorMessage = err.Error()
				if saveErr := e.store.SaveRun(p); saveErr != nil {
					return saveErr
				}
				humanRequiredTS := time.Now()
				e.bus.Publish(core.Event{
					Type:      core.EventHumanRequired,
					RunID:     p.ID,
					ProjectID: p.ProjectID,
					Stage:     stage.Name,
					Data:      RunEventData(traceID, issueNumber, "human_required", baseEventData),
					Error:     err.Error(),
					Timestamp: humanRequiredTS,
				})
				return nil
			case ReactionAbortRun:
				return e.failRun(p, fmt.Sprintf("stage %s failed: %v", stage.Name, err), err)
			default:
				return e.failRun(p, fmt.Sprintf("stage %s failed with unknown reaction %q: %v", stage.Name, action, err), err)
			}

			break
		}

		if stageSkipped {
			continue
		}
		if !stageSucceeded {
			return e.failRun(p, fmt.Sprintf("stage %s did not succeed", stage.Name), errors.New("stage not successful"))
		}

		if stage.RequireHuman {
			p.Status = core.StatusActionRequired
			if err := e.store.SaveRun(p); err != nil {
				return err
			}
			humanRequiredTS := time.Now()
			e.bus.Publish(core.Event{
				Type:      core.EventHumanRequired,
				RunID:     p.ID,
				ProjectID: p.ProjectID,
				Stage:     stage.Name,
				Data:      RunEventData(traceID, issueNumber, "human_required", baseEventData),
				Timestamp: humanRequiredTS,
			})
			return nil
		}
	}

	p.Status = core.StatusCompleted
	p.Conclusion = core.ConclusionSuccess
	p.FinishedAt = time.Now()
	p.ErrorMessage = ""
	if err := e.store.SaveRun(p); err != nil {
		return err
	}
	e.bus.Publish(core.Event{
		Type:      core.EventRunDone,
		RunID:     p.ID,
		ProjectID: p.ProjectID,
		Data:      RunEventData(traceID, issueNumber, "run_done", baseEventData),
		Timestamp: time.Now(),
	})
	logger.Info("Run done", observability.StructuredLogArgs(observability.StructuredLogInput{
		TraceID:     traceID,
		ProjectID:   p.ProjectID,
		RunID:       p.ID,
		IssueNumber: issueNumber,
		Operation:   "run_done",
		Latency:     0,
	})...)
	return nil
}

func (e *Executor) resolveStartIndex(p *core.Run, allowAlreadyRunning bool) int {
	if !allowAlreadyRunning || p.CurrentStage == "" {
		return 0
	}

	currentIndex := findStageIndex(p.Stages, p.CurrentStage)
	if currentIndex < 0 {
		return 0
	}

	checkpoints, err := e.store.GetCheckpoints(p.ID)
	if err != nil {
		e.logger.Warn("resolve start index fallback to current stage due checkpoint read error", "run_id", p.ID, "error", err)
		return currentIndex
	}

	last := latestCheckpointForStage(checkpoints, p.CurrentStage)
	if last != nil && last.Status == core.CheckpointSuccess {
		next := currentIndex + 1
		if next > len(p.Stages) {
			return len(p.Stages)
		}
		return next
	}
	return currentIndex
}

func findStageIndex(stages []core.StageConfig, stage core.StageID) int {
	for i := range stages {
		if stages[i].Name == stage {
			return i
		}
	}
	return -1
}

func latestCheckpointForStage(checkpoints []core.Checkpoint, stage core.StageID) *core.Checkpoint {
	for i := len(checkpoints) - 1; i >= 0; i-- {
		if checkpoints[i].StageName == stage {
			return &checkpoints[i]
		}
	}
	return nil
}

func (e *Executor) registerSession(RunID, sessionID string) {
	e.sessionMu.Lock()
	defer e.sessionMu.Unlock()
	e.activeSession[RunID] = sessionID
}

func (e *Executor) unregisterSession(RunID, sessionID string) {
	e.sessionMu.Lock()
	defer e.sessionMu.Unlock()

	existing := e.activeSession[RunID]
	if existing == sessionID {
		delete(e.activeSession, RunID)
	}
}

func (e *Executor) killActiveSession(RunID string) error {
	e.sessionMu.Lock()
	sessionID := e.activeSession[RunID]
	e.sessionMu.Unlock()

	if sessionID == "" {
		return nil
	}
	return e.runtime.Kill(sessionID)
}

func (e *Executor) isRunActionRequired(RunID string) (bool, error) {
	p, err := e.store.GetRun(RunID)
	if err != nil {
		return false, err
	}
	return p.Status == core.StatusActionRequired, nil
}

func (e *Executor) failRun(p *core.Run, message string, cause error) error {
	p.Status = core.StatusCompleted
	p.Conclusion = core.ConclusionFailure
	p.ErrorMessage = message
	p.FinishedAt = time.Now()
	if err := e.store.SaveRun(p); err != nil {
		return err
	}
	traceID := RunTraceID(p)
	issueNumber := issueNumberFromRun(p)
	extra := map[string]string{}
	if prNumber := prNumberFromRunData(p); prNumber > 0 {
		extra["pr_number"] = strconv.Itoa(prNumber)
	}
	e.bus.Publish(core.Event{
		Type:      core.EventRunFailed,
		RunID:     p.ID,
		ProjectID: p.ProjectID,
		Data:      RunEventData(traceID, issueNumber, "run_failed", extra),
		Error:     message,
		Timestamp: time.Now(),
	})
	logger := e.logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Error("Run failed", observability.StructuredLogArgs(observability.StructuredLogInput{
		TraceID:     traceID,
		ProjectID:   p.ProjectID,
		RunID:       p.ID,
		IssueNumber: issueNumber,
		Operation:   "run_failed",
		Latency:     0,
	})...)
	if cause == nil {
		return errors.New(message)
	}
	return fmt.Errorf("%s: %w", message, cause)
}

func (e *Executor) executeStage(ctx context.Context, project *core.Project, p *core.Run, stage *core.StageConfig) error {
	switch stage.Name {
	case core.StageSetup:
		return e.runWorktreeSetup(project, p)
	case core.StageMerge:
		return e.runMerge(project, p)
	case core.StageCleanup:
		return e.runCleanup(project, p)
	}

	if p.WorktreePath == "" {
		return fmt.Errorf("worktree path is empty for agent stage %s", stage.Name)
	}

	roleName := strings.TrimSpace(stage.Role)
	if roleName == "" {
		return fmt.Errorf("stage role is required for stage %q", stage.Name)
	}
	if e.roleResolver == nil {
		return fmt.Errorf("role resolver is not configured for stage %q", stage.Name)
	}
	agentProfile, roleProfile, err := e.roleResolver.Resolve(roleName)
	if err != nil {
		return fmt.Errorf("resolve role %q for stage %q: %w", roleName, stage.Name, err)
	}
	agentName := strings.TrimSpace(agentProfile.ID)
	if agentName == "" {
		return fmt.Errorf("resolved empty agent id for stage %q role %q", stage.Name, roleName)
	}

	promptStage := stage.PromptTemplate
	if promptStage == "" {
		promptStage = string(stage.Name)
	}
	executionContext, err := buildPromptExecutionContext(p, stage.Name)
	if err != nil {
		return fmt.Errorf("build prompt execution context: %w", err)
	}
	prompt, err := RenderPrompt(promptStage, PromptVars{
		ProjectName:      project.Name,
		RepoPath:         project.RepoPath,
		WorktreePath:     p.WorktreePath,
		Requirements:     p.Description,
		ExecutionContext: executionContext,
		RetryError:       p.ErrorMessage,
		RetryCount:       p.TotalRetries,
	})
	if err != nil {
		return fmt.Errorf("render prompt: %w", err)
	}

	// Prefer ACP protocol when launch command and handler factory are available.
	if strings.TrimSpace(agentProfile.LaunchCommand) != "" && e.acpHandlerFactory != nil {
		return e.runACPStage(ctx, agentName, agentProfile, roleProfile, p, stage, prompt)
	}

	// Fallback to CLI agent plugin path.
	return e.runCLIStage(ctx, agentName, p, stage, prompt)
}

// runCLIStage executes a pipeline stage via CLI agent plugin (legacy path).
func (e *Executor) runCLIStage(
	ctx context.Context,
	agentName string,
	p *core.Run,
	stage *core.StageConfig,
	prompt string,
) error {
	agent, ok := e.agents[agentName]
	if !ok {
		return fmt.Errorf("agent plugin %q not found for stage %q", agentName, stage.Name)
	}
	opts := core.ExecOpts{
		Prompt:   prompt,
		WorkDir:  p.WorktreePath,
		MaxTurns: 30,
		Timeout:  stage.Timeout,
	}
	cmd, err := agent.BuildCommand(opts)
	if err != nil {
		return fmt.Errorf("build command: %w", err)
	}

	stageCtx := ctx
	if stage.Timeout > 0 {
		var cancel context.CancelFunc
		stageCtx, cancel = context.WithTimeout(ctx, stage.Timeout)
		defer cancel()
	}

	sess, err := e.runtime.Create(stageCtx, core.RuntimeOpts{
		Command: cmd,
		WorkDir: p.WorktreePath,
	})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	e.registerSession(p.ID, sess.ID)
	defer e.unregisterSession(p.ID, sess.ID)

	parser := agent.NewStreamParser(sess.Stdout)
	gotDone := false
	for {
		evt, err := parser.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("parse stream: %w", err)
		}
		if evt.Type == "done" {
			gotDone = true
		}
		e.bus.Publish(core.Event{
			Type:  core.EventAgentOutput,
			RunID: p.ID,
			Stage: stage.Name,
			Agent: agentName,
			Data: map[string]string{
				"content": evt.Content,
				"type":    evt.Type,
			},
			Timestamp: evt.Timestamp,
		})
	}

	if err := sess.Wait(); err != nil {
		if gotDone {
			return nil
		}
		return fmt.Errorf("wait session: %w", err)
	}
	return nil
}

// acpPoolKey builds the session pool key for a given run and stage.
func acpPoolKey(runID string, stage core.StageID) string {
	return runID + ":" + string(stage)
}

// acpPoolGet retrieves a cached ACP session for the given run+stage.
func (e *Executor) acpPoolGet(runID string, stage core.StageID) *acpSessionEntry {
	e.acpPoolMu.Lock()
	defer e.acpPoolMu.Unlock()
	return e.acpPool[acpPoolKey(runID, stage)]
}

// acpPoolPut stores an ACP session in the pool for later reuse.
func (e *Executor) acpPoolPut(runID string, stage core.StageID, entry *acpSessionEntry) {
	e.acpPoolMu.Lock()
	defer e.acpPoolMu.Unlock()
	e.acpPool[acpPoolKey(runID, stage)] = entry
}

// acpPoolCleanup closes and removes all pooled sessions for a given run.
func (e *Executor) acpPoolCleanup(runID string) {
	e.acpPoolMu.Lock()
	var toClose []string
	var entries []*acpSessionEntry
	for key, entry := range e.acpPool {
		if strings.HasPrefix(key, runID+":") {
			toClose = append(toClose, key)
			entries = append(entries, entry)
			delete(e.acpPool, key)
		}
	}
	e.acpPoolMu.Unlock()

	if len(entries) > 0 && e.logger != nil {
		e.logger.Info("acp pool cleanup", "run_id", runID, "sessions", len(entries))
	}
	for i, entry := range entries {
		if entry.client == nil {
			continue
		}
		if e.logger != nil {
			e.logger.Info("acp pool closing session", "key", toClose[i])
		}
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = entry.client.Close(closeCtx)
		cancel()
	}
}

// runACPStage executes a pipeline stage via ACP protocol.
// If stage.ReuseSessionFrom is set, it reuses the ACP client+session from that source stage.
func (e *Executor) runACPStage(
	ctx context.Context,
	agentName string,
	agentProfile acpclient.AgentProfile,
	roleProfile acpclient.RoleProfile,
	p *core.Run,
	stage *core.StageConfig,
	prompt string,
) error {
	stageCtx := ctx
	if stage.Timeout > 0 {
		var cancel context.CancelFunc
		stageCtx, cancel = context.WithTimeout(ctx, stage.Timeout)
		defer cancel()
	}

	bridge := &stageEventBridge{
		executor:  e,
		runID:     p.ID,
		stage:     stage.Name,
		agentName: agentName,
	}

	// Try to reuse a pooled session from a previous stage.
	if source := stage.ReuseSessionFrom; source != "" {
		entry := e.acpPoolGet(p.ID, source)
		if entry != nil {
			if e.logger != nil {
				e.logger.Info("acp session reuse",
					"run_id", p.ID, "stage", stage.Name, "source_stage", source,
					"session_id", string(entry.sessionID))
			}
			return e.promptACPSession(stageCtx, entry, p, stage, agentName, prompt, bridge)
		}
		// Source session not found — fall through to create a new one.
		if e.logger != nil {
			e.logger.Warn("acp session pool miss, creating new session",
				"run_id", p.ID, "stage", stage.Name, "source", source)
		}
	}

	// Create a new ACP client + session.
	if e.acpHandlerFactory == nil {
		return fmt.Errorf("acp handler factory is not configured for stage %s", stage.Name)
	}

	launchCfg := acpclient.LaunchConfig{
		Command: strings.TrimSpace(agentProfile.LaunchCommand),
		Args:    append([]string(nil), agentProfile.LaunchArgs...),
		WorkDir: p.WorktreePath,
		Env:     cloneStringMapForEngine(agentProfile.Env),
	}
	handler := e.acpHandlerFactory.NewHandler(p.WorktreePath, e.bus)
	e.acpHandlerFactory.SetPermissionPolicy(handler, roleProfile.PermissionPolicy)

	acpOpts := []acpclient.Option{
		acpclient.WithEventHandler(bridge),
	}
	client, err := acpclient.New(launchCfg, handler, acpOpts...)
	if err != nil {
		return fmt.Errorf("create acp client for stage %s: %w", stage.Name, err)
	}

	if err := client.Initialize(stageCtx, roleProfile.Capabilities); err != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = client.Close(closeCtx)
		cancel()
		return fmt.Errorf("acp initialize for stage %s: %w", stage.Name, err)
	}

	session, err := client.NewSession(stageCtx, acpproto.NewSessionRequest{
		Cwd: p.WorktreePath,
	})
	if err != nil {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = client.Close(closeCtx)
		cancel()
		return fmt.Errorf("acp new session for stage %s: %w", stage.Name, err)
	}

	entry := &acpSessionEntry{client: client, sessionID: session}

	// Always pool the session — it will be cleaned up at run end or reused by a later stage.
	e.acpPoolPut(p.ID, stage.Name, entry)
	if e.logger != nil {
		e.logger.Info("acp session created and pooled",
			"run_id", p.ID, "stage", stage.Name, "session_id", string(session))
	}

	return e.promptACPSession(stageCtx, entry, p, stage, agentName, prompt, bridge)
}

// promptACPSession sends a prompt to an existing ACP session and publishes the result.
func (e *Executor) promptACPSession(
	ctx context.Context,
	entry *acpSessionEntry,
	p *core.Run,
	stage *core.StageConfig,
	agentName string,
	prompt string,
	bridge *stageEventBridge,
) error {
	// Update the event bridge for the current stage.
	bridge.stage = stage.Name

	result, err := entry.client.Prompt(ctx, acpproto.PromptRequest{
		SessionId: entry.sessionID,
		Prompt: []acpproto.ContentBlock{
			{Text: &acpproto.ContentBlockText{Text: prompt}},
		},
	})
	if err != nil {
		return fmt.Errorf("acp prompt for stage %s: %w", stage.Name, err)
	}

	replyText := ""
	if result != nil {
		replyText = strings.TrimSpace(result.Text)
	}
	e.bus.Publish(core.Event{
		Type:  core.EventAgentOutput,
		RunID: p.ID,
		Stage: stage.Name,
		Agent: agentName,
		Data: map[string]string{
			"content": replyText,
			"type":    "done",
		},
		Timestamp: time.Now(),
	})
	return nil
}

// stageEventBridge converts ACP session updates to EventAgentOutput events.
type stageEventBridge struct {
	executor  *Executor
	runID     string
	stage     core.StageID
	agentName string
}

func (b *stageEventBridge) HandleSessionUpdate(_ context.Context, update acpclient.SessionUpdate) error {
	if update.Text == "" {
		return nil
	}
	b.executor.bus.Publish(core.Event{
		Type:  core.EventAgentOutput,
		RunID: b.runID,
		Stage: b.stage,
		Agent: b.agentName,
		Data: map[string]string{
			"content": update.Text,
			"type":    update.Type,
		},
		Timestamp: time.Now(),
	})
	return nil
}

func cloneStringMapForEngine(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func (e *Executor) resolveStageAgentName(stage *core.StageConfig) (string, acpclient.ClientCapabilities, error) {
	if stage == nil {
		return "", acpclient.ClientCapabilities{}, errors.New("stage config is nil")
	}
	if !stageRequiresRole(stage.Name) {
		return "", acpclient.ClientCapabilities{}, nil
	}

	roleName := strings.TrimSpace(stage.Role)
	if roleName == "" {
		return "", acpclient.ClientCapabilities{}, fmt.Errorf("stage role is required for stage %q", stage.Name)
	}
	if e.roleResolver == nil {
		return "", acpclient.ClientCapabilities{}, fmt.Errorf("stage role resolver is not configured for stage %q (role=%q)", stage.Name, roleName)
	}

	resolvedAgent, roleProfile, err := e.roleResolver.Resolve(roleName)
	if err != nil {
		return "", acpclient.ClientCapabilities{}, fmt.Errorf("stage role not resolved for stage %q (role=%q): %w", stage.Name, roleName, err)
	}
	agentName := strings.TrimSpace(resolvedAgent.ID)
	if agentName == "" {
		return "", acpclient.ClientCapabilities{}, fmt.Errorf("stage role not resolved for stage %q (role=%q): resolved empty agent id", stage.Name, roleName)
	}
	return agentName, roleProfile.Capabilities, nil
}

func stageRequiresRole(stage core.StageID) bool {
	switch stage {
	case core.StageSetup, core.StageMerge, core.StageCleanup:
		return false
	default:
		return true
	}
}

func buildPromptExecutionContext(p *core.Run, stage core.StageID) (string, error) {
	ctx := map[string]string{
		"run_id":      p.ID,
		"run_name":    p.Name,
		"stage":       string(stage),
		"template":    p.Template,
		"branch_name": p.BranchName,
	}
	payload, err := json.Marshal(ctx)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func (e *Executor) runWorktreeSetup(project *core.Project, p *core.Run) error {
	if p.Config == nil {
		p.Config = map[string]any{}
	}
	if e.workspace == nil {
		return errors.New("workspace plugin is not configured")
	}

	result, err := e.workspace.Setup(context.Background(), core.WorkspaceSetupRequest{
		RepoPath:      project.RepoPath,
		RunID:         p.ID,
		BranchName:    p.BranchName,
		WorktreePath:  p.WorktreePath,
		DefaultBranch: project.DefaultBranch,
	})
	if err != nil {
		return err
	}
	p.BranchName = result.BranchName
	p.WorktreePath = result.WorktreePath
	if result.BaseBranch != "" {
		p.Config["base_branch"] = result.BaseBranch
	}

	return e.store.SaveRun(p)
}

func (e *Executor) runMerge(project *core.Project, p *core.Run) error {
	if p.BranchName == "" {
		return errors.New("branch name is empty")
	}
	runner := gitops.NewRunner(project.RepoPath)

	baseBranch := ""
	if p.Config != nil {
		baseBranch, _ = p.Config["base_branch"].(string)
	}
	if baseBranch == "" {
		var err error
		baseBranch, err = runner.CurrentBranch()
		if err != nil {
			return err
		}
	}

	if err := runner.Checkout(baseBranch); err != nil {
		return err
	}
	_, err := runner.Merge(p.BranchName)
	return err
}

func (e *Executor) runCleanup(project *core.Project, p *core.Run) error {
	if p.WorktreePath == "" {
		return nil
	}
	if e.workspace == nil {
		return errors.New("workspace plugin is not configured")
	}
	return e.workspace.Cleanup(context.Background(), core.WorkspaceCleanupRequest{
		RepoPath:     project.RepoPath,
		WorktreePath: p.WorktreePath,
	})
}

func defaultStageConfig(id core.StageID) core.StageConfig {
	cfg := core.StageConfig{
		Name:       id,
		Timeout:    30 * time.Minute,
		MaxRetries: 1,
		OnFailure:  core.OnFailureHuman,
	}
	switch id {
	case core.StageRequirements, core.StageReview:
		cfg.Agent = ""
	case core.StageImplement:
		cfg.Agent = ""
	case core.StageFixup:
		cfg.Agent = ""
		cfg.ReuseSessionFrom = core.StageImplement
	case core.StageTest:
		cfg.Agent = ""
		cfg.Timeout = 15 * time.Minute
	case core.StageSetup, core.StageMerge, core.StageCleanup:
		cfg.Agent = ""
		cfg.Timeout = 2 * time.Minute
	}
	cfg.PromptTemplate = string(id)
	return cfg
}

func RunEventData(traceID string, issueNumber int, op string, extra map[string]string) map[string]string {
	data := make(map[string]string, len(extra)+2)
	for k, v := range extra {
		data[k] = v
	}
	if issueNumber > 0 {
		data["issue_number"] = strconv.Itoa(issueNumber)
	}
	if strings.TrimSpace(op) != "" {
		data["op"] = strings.TrimSpace(op)
	}
	return observability.EventDataWithTrace(data, traceID)
}

func issueNumberFromRun(p *core.Run) int {
	if p == nil {
		return 0
	}
	if p.Config != nil {
		for _, key := range []string{"issue_number", "github_issue_number"} {
			if n := parseIssueNumberConfigValue(p.Config[key]); n > 0 {
				return n
			}
		}
	}
	if p.Artifacts != nil {
		for _, key := range []string{"issue_number", "github_issue_number"} {
			if n := parseIssueNumberConfigValue(p.Artifacts[key]); n > 0 {
				return n
			}
		}
	}
	return 0
}

func prNumberFromRunData(p *core.Run) int {
	if p == nil {
		return 0
	}
	if p.Config != nil {
		for _, key := range []string{"pr_number", "github_pr_number"} {
			if n := parseIssueNumberConfigValue(p.Config[key]); n > 0 {
				return n
			}
		}
	}
	if p.Artifacts != nil {
		for _, key := range []string{"pr_number", "github_pr_number"} {
			if n := parseIssueNumberConfigValue(p.Artifacts[key]); n > 0 {
				return n
			}
		}
	}
	return 0
}

func parseIssueNumberConfigValue(raw any) int {
	switch v := raw.(type) {
	case int:
		if v > 0 {
			return v
		}
	case int32:
		if v > 0 {
			return int(v)
		}
	case int64:
		if v > 0 {
			return int(v)
		}
	case float64:
		if v > 0 {
			return int(v)
		}
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err == nil && n > 0 {
			return n
		}
	}
	return 0
}

func RunTraceID(p *core.Run) string {
	if p == nil || p.Config == nil {
		return ""
	}
	traceID, _ := p.Config["trace_id"].(string)
	return strings.TrimSpace(traceID)
}
