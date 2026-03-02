package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/user/ai-workflow/internal/acpclient"
	"github.com/user/ai-workflow/internal/core"
	"github.com/user/ai-workflow/internal/eventbus"
	gitops "github.com/user/ai-workflow/internal/git"
	"github.com/user/ai-workflow/internal/observability"
)

type Executor struct {
	store        core.Store
	bus          *eventbus.Bus
	agents       map[string]core.AgentPlugin
	roleResolver *acpclient.RoleResolver
	stageRoles   map[core.StageID]string
	runtime      core.RuntimePlugin
	logger       *slog.Logger

	sessionMu     sync.Mutex
	activeSession map[string]string
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
	}
}

func (e *Executor) SetRoleResolver(resolver *acpclient.RoleResolver) {
	e.roleResolver = resolver
}

func (e *Executor) SetPipelineStageRoles(stageRoles map[string]string) {
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

func (e *Executor) CreatePipeline(projectID, name, description, template string) (*core.Pipeline, error) {
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

	p := &core.Pipeline{
		ID:              NewPipelineID(),
		ProjectID:       projectID,
		Name:            name,
		Description:     description,
		Template:        template,
		Status:          core.StatusCreated,
		Stages:          stages,
		Artifacts:       map[string]string{},
		Config:          map[string]any{},
		MaxTotalRetries: 5,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	if err := e.store.SavePipeline(p); err != nil {
		return nil, err
	}
	return p, nil
}

func (e *Executor) Run(ctx context.Context, pipelineID string) error {
	return e.run(ctx, pipelineID, false)
}

// RunScheduled executes a pipeline that has already been CAS-marked as running by scheduler.
func (e *Executor) RunScheduled(ctx context.Context, pipelineID string) error {
	return e.run(ctx, pipelineID, true)
}

func (e *Executor) run(ctx context.Context, pipelineID string, allowAlreadyRunning bool) error {
	p, err := e.store.GetPipeline(pipelineID)
	if err != nil {
		return err
	}
	project, err := e.store.GetProject(p.ProjectID)
	if err != nil {
		return err
	}

	logger := e.logger
	if logger == nil {
		logger = slog.Default()
	}
	ctx, traceID := observability.EnsureTraceID(ctx, p.ID)
	issueNumber := issueNumberFromPipeline(p)
	prNumber := prNumberFromPipelineData(p)
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

	if allowAlreadyRunning && p.Status == core.StatusRunning {
		if p.StartedAt.IsZero() {
			p.StartedAt = time.Now()
			if err := e.store.SavePipeline(p); err != nil {
				return err
			}
		}
	} else {
		if err := core.ValidateTransition(p.Status, core.StatusRunning); err != nil {
			return err
		}
		p.Status = core.StatusRunning
		p.StartedAt = time.Now()
		if err := e.store.SavePipeline(p); err != nil {
			return err
		}
	}

	startIndex := e.resolveStartIndex(p, allowAlreadyRunning)
	for i := startIndex; i < len(p.Stages); i++ {
		stage := p.Stages[i]
		p.CurrentStage = stage.Name
		if err := e.store.SavePipeline(p); err != nil {
			return err
		}
		stageStartedAt := time.Now()

		e.bus.Publish(core.Event{
			Type:       core.EventStageStart,
			PipelineID: p.ID,
			ProjectID:  p.ProjectID,
			Stage:      stage.Name,
			Data:       pipelineEventData(traceID, issueNumber, "stage_start", baseEventData),
			Timestamp:  time.Now(),
		})
		logger.Info("pipeline stage started", observability.StructuredLogArgs(observability.StructuredLogInput{
			TraceID:     traceID,
			ProjectID:   p.ProjectID,
			PipelineID:  p.ID,
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
			agentUsed := stage.Agent
			if resolvedAgent, resolveErr := e.resolveStageAgentName(&stage); resolveErr == nil {
				agentUsed = resolvedAgent
			}

			cp := &core.Checkpoint{
				PipelineID: p.ID,
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
				e.bus.Publish(core.Event{
					Type:       core.EventStageComplete,
					PipelineID: p.ID,
					ProjectID:  p.ProjectID,
					Stage:      stage.Name,
					Data:       pipelineEventData(traceID, issueNumber, "stage_complete", baseEventData),
					Timestamp:  time.Now(),
				})
				logger.Info("pipeline stage completed", observability.StructuredLogArgs(observability.StructuredLogInput{
					TraceID:     traceID,
					ProjectID:   p.ProjectID,
					PipelineID:  p.ID,
					IssueNumber: issueNumber,
					Operation:   "stage_complete",
					Latency:     time.Since(stageStartedAt),
				})...)
				stageSucceeded = true
				break
			}

			paused, stateErr := e.isPipelinePaused(p.ID)
			if stateErr != nil {
				return stateErr
			}
			if paused {
				// Pause keeps current stage in-progress for a later explicit resume.
				return nil
			}

			cp.Status = core.CheckpointFailed
			cp.Error = err.Error()
			if saveErr := e.store.SaveCheckpoint(cp); saveErr != nil {
				return saveErr
			}
			e.bus.Publish(core.Event{
				Type:       core.EventStageFailed,
				PipelineID: p.ID,
				ProjectID:  p.ProjectID,
				Stage:      stage.Name,
				Data:       pipelineEventData(traceID, issueNumber, "stage_failed", baseEventData),
				Error:      err.Error(),
				Timestamp:  time.Now(),
			})
			logger.Error("pipeline stage failed", observability.StructuredLogArgs(observability.StructuredLogInput{
				TraceID:     traceID,
				ProjectID:   p.ProjectID,
				PipelineID:  p.ID,
				IssueNumber: issueNumber,
				Operation:   "stage_failed",
				Latency:     time.Since(stageStartedAt),
			})...)

			p.TotalRetries++
			if saveErr := e.store.SavePipeline(p); saveErr != nil {
				return saveErr
			}
			if p.TotalRetries >= p.MaxTotalRetries {
				return e.failPipeline(p, fmt.Sprintf("retry budget exhausted at stage %s: %v", stage.Name, err), err)
			}

			action, matched := EvaluateReactionRules(ReactionContext{
				Stage:    stage,
				Attempt:  attempt,
				MaxRetry: maxRetries,
				Err:      err,
			}, CompileOnFailureReactions(stage))
			if !matched {
				action = ReactionAbortPipeline
			}

			switch action {
			case ReactionRetry:
				if attempt < maxRetries {
					continue
				}
				return e.failPipeline(p, fmt.Sprintf("stage %s exhausted retries(%d): %v", stage.Name, maxRetries, err), err)
			case ReactionSkipStage:
				stageSkipped = true
				cpSkip := &core.Checkpoint{
					PipelineID: p.ID,
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
				p.Status = core.StatusWaitingHuman
				p.ErrorMessage = err.Error()
				if saveErr := e.store.SavePipeline(p); saveErr != nil {
					return saveErr
				}
				e.bus.Publish(core.Event{
					Type:       core.EventHumanRequired,
					PipelineID: p.ID,
					ProjectID:  p.ProjectID,
					Stage:      stage.Name,
					Data:       pipelineEventData(traceID, issueNumber, "human_required", baseEventData),
					Error:      err.Error(),
					Timestamp:  time.Now(),
				})
				return nil
			case ReactionAbortPipeline:
				return e.failPipeline(p, fmt.Sprintf("stage %s failed: %v", stage.Name, err), err)
			default:
				return e.failPipeline(p, fmt.Sprintf("stage %s failed with unknown reaction %q: %v", stage.Name, action, err), err)
			}

			break
		}

		if stageSkipped {
			continue
		}
		if !stageSucceeded {
			return e.failPipeline(p, fmt.Sprintf("stage %s did not succeed", stage.Name), errors.New("stage not successful"))
		}

		if stage.RequireHuman {
			p.Status = core.StatusWaitingHuman
			if err := e.store.SavePipeline(p); err != nil {
				return err
			}
			e.bus.Publish(core.Event{
				Type:       core.EventHumanRequired,
				PipelineID: p.ID,
				ProjectID:  p.ProjectID,
				Stage:      stage.Name,
				Data:       pipelineEventData(traceID, issueNumber, "human_required", baseEventData),
				Timestamp:  time.Now(),
			})
			return nil
		}
	}

	p.Status = core.StatusDone
	p.FinishedAt = time.Now()
	p.ErrorMessage = ""
	if err := e.store.SavePipeline(p); err != nil {
		return err
	}
	e.bus.Publish(core.Event{
		Type:       core.EventPipelineDone,
		PipelineID: p.ID,
		ProjectID:  p.ProjectID,
		Data:       pipelineEventData(traceID, issueNumber, "pipeline_done", baseEventData),
		Timestamp:  time.Now(),
	})
	logger.Info("pipeline done", observability.StructuredLogArgs(observability.StructuredLogInput{
		TraceID:     traceID,
		ProjectID:   p.ProjectID,
		PipelineID:  p.ID,
		IssueNumber: issueNumber,
		Operation:   "pipeline_done",
		Latency:     0,
	})...)
	return nil
}

func (e *Executor) resolveStartIndex(p *core.Pipeline, allowAlreadyRunning bool) int {
	if !allowAlreadyRunning || p.CurrentStage == "" {
		return 0
	}

	currentIndex := findStageIndex(p.Stages, p.CurrentStage)
	if currentIndex < 0 {
		return 0
	}

	checkpoints, err := e.store.GetCheckpoints(p.ID)
	if err != nil {
		e.logger.Warn("resolve start index fallback to current stage due checkpoint read error", "pipeline_id", p.ID, "error", err)
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

func (e *Executor) registerSession(pipelineID, sessionID string) {
	e.sessionMu.Lock()
	defer e.sessionMu.Unlock()
	e.activeSession[pipelineID] = sessionID
}

func (e *Executor) unregisterSession(pipelineID, sessionID string) {
	e.sessionMu.Lock()
	defer e.sessionMu.Unlock()

	existing := e.activeSession[pipelineID]
	if existing == sessionID {
		delete(e.activeSession, pipelineID)
	}
}

func (e *Executor) killActiveSession(pipelineID string) error {
	e.sessionMu.Lock()
	sessionID := e.activeSession[pipelineID]
	e.sessionMu.Unlock()

	if sessionID == "" {
		return nil
	}
	return e.runtime.Kill(sessionID)
}

func (e *Executor) isPipelinePaused(pipelineID string) (bool, error) {
	p, err := e.store.GetPipeline(pipelineID)
	if err != nil {
		return false, err
	}
	return p.Status == core.StatusPaused, nil
}

func (e *Executor) failPipeline(p *core.Pipeline, message string, cause error) error {
	p.Status = core.StatusFailed
	p.ErrorMessage = message
	p.FinishedAt = time.Now()
	if err := e.store.SavePipeline(p); err != nil {
		return err
	}
	traceID := pipelineTraceID(p)
	issueNumber := issueNumberFromPipeline(p)
	extra := map[string]string{}
	if prNumber := prNumberFromPipelineData(p); prNumber > 0 {
		extra["pr_number"] = strconv.Itoa(prNumber)
	}
	e.bus.Publish(core.Event{
		Type:       core.EventPipelineFailed,
		PipelineID: p.ID,
		ProjectID:  p.ProjectID,
		Data:       pipelineEventData(traceID, issueNumber, "pipeline_failed", extra),
		Error:      message,
		Timestamp:  time.Now(),
	})
	logger := e.logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Error("pipeline failed", observability.StructuredLogArgs(observability.StructuredLogInput{
		TraceID:     traceID,
		ProjectID:   p.ProjectID,
		PipelineID:  p.ID,
		IssueNumber: issueNumber,
		Operation:   "pipeline_failed",
		Latency:     0,
	})...)
	if cause == nil {
		return errors.New(message)
	}
	return fmt.Errorf("%s: %w", message, cause)
}

func (e *Executor) executeStage(ctx context.Context, project *core.Project, p *core.Pipeline, stage *core.StageConfig) error {
	switch stage.Name {
	case core.StageWorktreeSetup:
		return e.runWorktreeSetup(project, p)
	case core.StageMerge:
		return e.runMerge(project, p)
	case core.StageCleanup:
		return e.runCleanup(project, p)
	}

	if p.WorktreePath == "" {
		return fmt.Errorf("worktree path is empty for agent stage %s", stage.Name)
	}

	agentName, agent, err := e.resolveStageAgent(stage)
	if err != nil {
		return err
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
	for {
		evt, err := parser.Next()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("parse stream: %w", err)
		}
		e.bus.Publish(core.Event{
			Type:       core.EventAgentOutput,
			PipelineID: p.ID,
			Stage:      stage.Name,
			Agent:      agentName,
			Data: map[string]string{
				"content": evt.Content,
				"type":    evt.Type,
			},
			Timestamp: evt.Timestamp,
		})
	}

	if err := sess.Wait(); err != nil {
		return fmt.Errorf("wait session: %w", err)
	}
	return nil
}

func (e *Executor) resolveStageAgentName(stage *core.StageConfig) (string, error) {
	if stage == nil {
		return "", errors.New("stage config is nil")
	}
	if !stageRequiresRole(stage.Name) {
		return "", nil
	}

	roleName := strings.TrimSpace(stage.Role)
	if roleName == "" {
		return "", fmt.Errorf("stage role is required for stage %q", stage.Name)
	}
	if e.roleResolver == nil {
		return "", fmt.Errorf("stage role resolver is not configured for stage %q (role=%q)", stage.Name, roleName)
	}

	resolvedAgent, _, err := e.roleResolver.Resolve(roleName)
	if err != nil {
		return "", fmt.Errorf("stage role not resolved for stage %q (role=%q): %w", stage.Name, roleName, err)
	}
	agentName := strings.TrimSpace(resolvedAgent.ID)
	if agentName == "" {
		return "", fmt.Errorf("stage role not resolved for stage %q (role=%q): resolved empty agent id", stage.Name, roleName)
	}
	return agentName, nil
}

func (e *Executor) resolveStageAgent(stage *core.StageConfig) (string, core.AgentPlugin, error) {
	agentName, err := e.resolveStageAgentName(stage)
	if err != nil {
		return "", nil, err
	}

	agent, ok := e.agents[agentName]
	if !ok {
		return "", nil, fmt.Errorf("stage role not resolved for stage %q (role=%q): agent plugin %q not found", stage.Name, strings.TrimSpace(stage.Role), agentName)
	}
	return agentName, agent, nil
}

func stageRequiresRole(stage core.StageID) bool {
	switch stage {
	case core.StageWorktreeSetup, core.StageMerge, core.StageCleanup:
		return false
	default:
		return true
	}
}

func buildPromptExecutionContext(p *core.Pipeline, stage core.StageID) (string, error) {
	ctx := map[string]string{
		"pipeline_id":   p.ID,
		"pipeline_name": p.Name,
		"stage":         string(stage),
		"template":      p.Template,
		"branch_name":   p.BranchName,
	}
	payload, err := json.Marshal(ctx)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func (e *Executor) runWorktreeSetup(project *core.Project, p *core.Pipeline) error {
	if project.RepoPath == "" {
		return errors.New("project repo path is empty")
	}
	runner := gitops.NewRunner(project.RepoPath)

	if p.Config == nil {
		p.Config = map[string]any{}
	}
	if p.BranchName == "" {
		p.BranchName = "ai-flow/" + p.ID
	}
	if p.WorktreePath == "" {
		p.WorktreePath = filepath.Join(project.RepoPath, ".worktrees", p.ID)
	}
	if err := os.MkdirAll(filepath.Dir(p.WorktreePath), 0o755); err != nil {
		return err
	}

	if _, err := os.Stat(p.WorktreePath); errors.Is(err, os.ErrNotExist) {
		if err := runner.WorktreeAdd(p.WorktreePath, p.BranchName); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	baseBranch, err := runner.CurrentBranch()
	if err != nil {
		return err
	}
	p.Config["base_branch"] = baseBranch

	return e.store.SavePipeline(p)
}

func (e *Executor) runMerge(project *core.Project, p *core.Pipeline) error {
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

func (e *Executor) runCleanup(project *core.Project, p *core.Pipeline) error {
	if p.WorktreePath == "" {
		return nil
	}
	runner := gitops.NewRunner(project.RepoPath)
	return runner.WorktreeRemove(p.WorktreePath)
}

func defaultStageConfig(id core.StageID) core.StageConfig {
	cfg := core.StageConfig{
		Name:       id,
		Timeout:    30 * time.Minute,
		MaxRetries: 1,
		OnFailure:  core.OnFailureHuman,
	}
	switch id {
	case core.StageRequirements, core.StageCodeReview:
		cfg.Agent = "codex"
	case core.StageImplement, core.StageFixup:
		cfg.Agent = "codex"
	case core.StageE2ETest:
		cfg.Agent = "codex"
		cfg.Timeout = 15 * time.Minute
	case core.StageWorktreeSetup, core.StageMerge, core.StageCleanup:
		cfg.Agent = ""
		cfg.Timeout = 2 * time.Minute
	}
	cfg.PromptTemplate = string(id)
	return cfg
}

func pipelineEventData(traceID string, issueNumber int, op string, extra map[string]string) map[string]string {
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

func issueNumberFromPipeline(p *core.Pipeline) int {
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

func prNumberFromPipelineData(p *core.Pipeline) int {
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

func pipelineTraceID(p *core.Pipeline) string {
	if p == nil || p.Config == nil {
		return ""
	}
	traceID, _ := p.Config["trace_id"].(string)
	return strings.TrimSpace(traceID)
}
