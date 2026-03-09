package engine

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/acpclient"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/observability"
)

// ACPEventPublisher receives events from ACP handlers.
type ACPEventPublisher interface {
	Publish(ctx context.Context, evt core.Event) error
}

// ACPHandlerFactory creates ACP protocol handlers for pipeline stage execution.
type ACPHandlerFactory interface {
	NewHandler(cwd string, publisher ACPEventPublisher) acpproto.Client
	SetPermissionPolicy(handler acpproto.Client, policy []acpclient.PermissionRule)
}

type Executor struct {
	store             core.Store
	bus               core.EventBus
	roleResolver      *acpclient.RoleResolver
	stageRoles        map[core.StageID]string
	workspace         core.WorkspacePlugin
	acpHandlerFactory ACPHandlerFactory
	mcpServerResolver func(role acpclient.RoleProfile, sseSupported bool) []acpproto.McpServer
	logger            *slog.Logger
	promptBuilder     *PromptBuilder

	// testStageFunc is a test-only hook that bypasses real ACP execution.
	testStageFunc func(ctx context.Context, runID string, stage core.StageID, agentName, prompt string) error

	// acpPool keeps ACP sessions alive across stages for the same run.
	// Key: "runID:stageID" where stageID is the stage that created the session.
	acpPoolMu sync.Mutex
	acpPool   map[string]*acpSessionEntry

	heartbeatInterval time.Duration
}

func NewExecutor(
	store core.Store,
	bus core.EventBus,
	logger *slog.Logger,
) *Executor {
	return &Executor{
		store:             store,
		bus:               bus,
		logger:            logger,
		acpPool:           make(map[string]*acpSessionEntry),
		heartbeatInterval: 10 * time.Second,
	}
}

func (e *Executor) SetRoleResolver(resolver *acpclient.RoleResolver) {
	e.roleResolver = resolver
}

func (e *Executor) SetACPHandlerFactory(factory ACPHandlerFactory) {
	e.acpHandlerFactory = factory
}

func (e *Executor) SetMCPServerResolver(fn func(role acpclient.RoleProfile, sseSupported bool) []acpproto.McpServer) {
	e.mcpServerResolver = fn
}

func (e *Executor) SetWorkspace(workspace core.WorkspacePlugin) {
	e.workspace = workspace
}

// SetMemory configures layered prompt memory for prompt generation.
func (e *Executor) SetMemory(memory core.Memory) {
	e.promptBuilder = NewPromptBuilder(memory)
}

// TestSetStageFunc sets a test-only hook that bypasses real ACP stage execution.
func (e *Executor) TestSetStageFunc(fn func(ctx context.Context, runID string, stage core.StageID, agentName, prompt string) error) {
	e.testStageFunc = fn
}

// TestSetHeartbeatInterval adjusts the run heartbeat cadence for tests.
func (e *Executor) TestSetHeartbeatInterval(interval time.Duration) {
	e.heartbeatInterval = interval
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

func (e *Executor) CreateRun(projectID, name, description, template string, maxTotalRetries int) (*core.Run, error) {
	stageIDs, ok := Templates[template]
	if !ok {
		return nil, fmt.Errorf("unknown template: %s", template)
	}
	if maxTotalRetries <= 0 {
		maxTotalRetries = 5
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
		MaxTotalRetries: maxTotalRetries,
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
		}
		p.LastHeartbeatAt = time.Now()
		if err := e.store.SaveRun(p); err != nil {
			return err
		}
	} else {
		if err := p.TransitionStatus(core.StatusInProgress); err != nil {
			return err
		}
		p.StartedAt = time.Now()
		p.LastHeartbeatAt = time.Now()
		if err := e.store.SaveRun(p); err != nil {
			return err
		}
		e.recordRunTaskStep(p, core.StepRunStarted, "", "", "run started")
	}

	stopHeartbeat := e.startRunHeartbeat(ctx, p.ID)
	defer stopHeartbeat()

	startIndex := e.resolveStartIndex(p, allowAlreadyRunning)
	for i := startIndex; i < len(p.Stages); i++ {
		stage := p.Stages[i]

		// Release ACP sessions before cleanup so file handles are freed
		// (prevents Windows "Permission Denied" on worktree removal).
		if stage.Name == core.StageCleanup {
			e.acpPoolCleanup(p.ID)
		}

		p.CurrentStage = stage.Name
		if err := e.store.SaveRun(p); err != nil {
			return err
		}
		stageStartedAt := time.Now()
		stageAgent := ""
		if resolvedAgent, _, resolveErr := e.resolveStageAgentName(&stage); resolveErr == nil {
			stageAgent = resolvedAgent
		}
		e.recordRunTaskStep(p, core.StepStageStarted, stage.Name, stageAgent, "")

		stageStartTS := time.Now()
		e.bus.Publish(ctx, core.Event{
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
			// Capture ACP session ID from the pool (direct or reused).
			if sid := e.acpPoolGetSessionID(p.ID, stage.Name); sid != "" {
				cp.AgentSessionID = sid
			} else if stage.ReuseSessionFrom != "" {
				cp.AgentSessionID = e.acpPoolGetSessionID(p.ID, stage.ReuseSessionFrom)
			}
			if err == nil {
				cp.Status = core.CheckpointSuccess
				if saveErr := e.store.SaveCheckpoint(cp); saveErr != nil {
					return saveErr
				}
				e.recordRunTaskStep(p, core.StepStageCompleted, stage.Name, agentUsed, "")
				stageCompleteTS := time.Now()
				e.bus.Publish(ctx, core.Event{
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
			e.recordRunTaskStep(p, core.StepStageFailed, stage.Name, agentUsed, err.Error())
			stageFailedTS := time.Now()
			e.bus.Publish(ctx, core.Event{
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
				if transErr := p.TransitionStatus(core.StatusActionRequired); transErr != nil {
					return transErr
				}
				p.ErrorMessage = err.Error()
				if saveErr := e.store.SaveRun(p); saveErr != nil {
					return saveErr
				}
				humanRequiredTS := time.Now()
				e.bus.Publish(ctx, core.Event{
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
			if err := p.TransitionStatus(core.StatusActionRequired); err != nil {
				return err
			}
			if err := e.store.SaveRun(p); err != nil {
				return err
			}
			humanRequiredTS := time.Now()
			e.bus.Publish(ctx, core.Event{
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

	if err := p.TransitionStatus(core.StatusCompleted); err != nil {
		return err
	}
	p.Conclusion = core.ConclusionSuccess
	p.FinishedAt = time.Now()
	p.ErrorMessage = ""
	if err := e.store.SaveRun(p); err != nil {
		return err
	}
	e.recordRunTaskStep(p, core.StepRunCompleted, "", "", "run completed: success")
	e.bus.Publish(ctx, core.Event{
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

type runHeartbeatStore interface {
	TouchRunHeartbeat(runID string, at time.Time) error
}

func (e *Executor) startRunHeartbeat(ctx context.Context, runID string) func() {
	if e == nil || strings.TrimSpace(runID) == "" {
		return func() {}
	}
	interval := e.heartbeatInterval
	if interval <= 0 {
		return func() {}
	}
	store, ok := e.store.(runHeartbeatStore)
	if !ok {
		return func() {}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	heartbeatCtx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case tickAt := <-ticker.C:
				if err := store.TouchRunHeartbeat(runID, tickAt); err != nil && e.logger != nil {
					e.logger.Warn("failed to update run heartbeat", "run_id", runID, "error", err)
				}
			}
		}
	}()
	return cancel
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

func (e *Executor) isRunActionRequired(RunID string) (bool, error) {
	p, err := e.store.GetRun(RunID)
	if err != nil {
		return false, err
	}
	return p.Status == core.StatusActionRequired, nil
}

func (e *Executor) failRun(p *core.Run, message string, cause error) error {
	if err := p.TransitionStatus(core.StatusCompleted); err != nil {
		return err
	}
	p.Conclusion = core.ConclusionFailure
	p.ErrorMessage = message
	p.FinishedAt = time.Now()
	if err := e.store.SaveRun(p); err != nil {
		return err
	}
	e.recordRunTaskStep(p, core.StepRunFailed, "", "", "run completed: failure")
	traceID := RunTraceID(p)
	issueNumber := issueNumberFromRun(p)
	extra := map[string]string{}
	if prNumber := prNumberFromRunData(p); prNumber > 0 {
		extra["pr_number"] = strconv.Itoa(prNumber)
	}
	e.bus.Publish(context.Background(), core.Event{
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

func (e *Executor) recordRunTaskStep(run *core.Run, action core.TaskStepAction, stageID core.StageID, agentID, note string) {
	if e == nil || e.store == nil || run == nil {
		return
	}
	issueID := strings.TrimSpace(run.IssueID)
	if issueID == "" {
		return
	}
	if _, err := e.store.SaveTaskStep(&core.TaskStep{
		ID:        core.NewTaskStepID(),
		IssueID:   issueID,
		RunID:     run.ID,
		Action:    action,
		StageID:   stageID,
		AgentID:   strings.TrimSpace(agentID),
		Note:      strings.TrimSpace(note),
		CreatedAt: time.Now(),
	}); err != nil {
		logger := e.logger
		if logger == nil {
			logger = slog.Default()
		}
		logger.Warn("failed to save task step", "run_id", run.ID, "issue_id", issueID, "action", action, "stage", stageID, "error", err)
	}
}
