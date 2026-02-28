package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/user/ai-workflow/internal/core"
	"github.com/user/ai-workflow/internal/eventbus"
	gitops "github.com/user/ai-workflow/internal/git"
)

type Executor struct {
	store   core.Store
	bus     *eventbus.Bus
	agents  map[string]core.AgentPlugin
	runtime core.RuntimePlugin
	logger  *slog.Logger
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
	}
}

func (e *Executor) CreatePipeline(projectID, name, description, template string) (*core.Pipeline, error) {
	stageIDs, ok := Templates[template]
	if !ok {
		return nil, fmt.Errorf("unknown template: %s", template)
	}

	stages := make([]core.StageConfig, len(stageIDs))
	for i, sid := range stageIDs {
		stages[i] = defaultStageConfig(sid)
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

	for i := range p.Stages {
		stage := p.Stages[i]
		p.CurrentStage = stage.Name
		if err := e.store.SavePipeline(p); err != nil {
			return err
		}

		e.bus.Publish(core.Event{
			Type:       core.EventStageStart,
			PipelineID: p.ID,
			ProjectID:  p.ProjectID,
			Stage:      stage.Name,
			Timestamp:  time.Now(),
		})

		maxRetries := stage.MaxRetries
		if maxRetries < 0 {
			maxRetries = 0
		}

		stageSucceeded := false
		stageSkipped := false
		for attempt := 0; ; attempt++ {
			cp := &core.Checkpoint{
				PipelineID: p.ID,
				StageName:  stage.Name,
				Status:     core.CheckpointInProgress,
				StartedAt:  time.Now(),
				AgentUsed:  stage.Agent,
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
					Stage:      stage.Name,
					Timestamp:  time.Now(),
				})
				stageSucceeded = true
				break
			}

			cp.Status = core.CheckpointFailed
			cp.Error = err.Error()
			if saveErr := e.store.SaveCheckpoint(cp); saveErr != nil {
				return saveErr
			}
			e.bus.Publish(core.Event{
				Type:       core.EventStageFailed,
				PipelineID: p.ID,
				Stage:      stage.Name,
				Error:      err.Error(),
				Timestamp:  time.Now(),
			})

			p.TotalRetries++
			if saveErr := e.store.SavePipeline(p); saveErr != nil {
				return saveErr
			}
			if p.TotalRetries >= p.MaxTotalRetries {
				return e.failPipeline(p, fmt.Sprintf("retry budget exhausted at stage %s: %v", stage.Name, err), err)
			}

			if stage.OnFailure == core.OnFailureRetry && attempt < maxRetries {
				continue
			}

			switch stage.OnFailure {
			case core.OnFailureSkip:
				stageSkipped = true
				cpSkip := &core.Checkpoint{
					PipelineID: p.ID,
					StageName:  stage.Name,
					Status:     core.CheckpointSkipped,
					StartedAt:  time.Now(),
					FinishedAt: time.Now(),
					AgentUsed:  stage.Agent,
					RetryCount: attempt,
					Error:      err.Error(),
				}
				if saveErr := e.store.SaveCheckpoint(cpSkip); saveErr != nil {
					return saveErr
				}
			case core.OnFailureHuman:
				p.Status = core.StatusWaitingHuman
				p.ErrorMessage = err.Error()
				if saveErr := e.store.SavePipeline(p); saveErr != nil {
					return saveErr
				}
				e.bus.Publish(core.Event{
					Type:       core.EventHumanRequired,
					PipelineID: p.ID,
					Stage:      stage.Name,
					Error:      err.Error(),
					Timestamp:  time.Now(),
				})
				return nil
			case core.OnFailureAbort:
				return e.failPipeline(p, fmt.Sprintf("stage %s failed: %v", stage.Name, err), err)
			case core.OnFailureRetry:
				return e.failPipeline(p, fmt.Sprintf("stage %s exhausted retries(%d): %v", stage.Name, maxRetries, err), err)
			default:
				return e.failPipeline(p, fmt.Sprintf("stage %s failed with unknown policy: %v", stage.Name, err), err)
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
				Stage:      stage.Name,
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
		Timestamp:  time.Now(),
	})
	return nil
}

func (e *Executor) failPipeline(p *core.Pipeline, message string, cause error) error {
	p.Status = core.StatusFailed
	p.ErrorMessage = message
	p.FinishedAt = time.Now()
	if err := e.store.SavePipeline(p); err != nil {
		return err
	}
	e.bus.Publish(core.Event{
		Type:       core.EventPipelineFailed,
		PipelineID: p.ID,
		Error:      message,
		Timestamp:  time.Now(),
	})
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

	agent, ok := e.agents[stage.Agent]
	if !ok {
		return fmt.Errorf("agent %q not found", stage.Agent)
	}

	promptStage := stage.PromptTemplate
	if promptStage == "" {
		promptStage = string(stage.Name)
	}
	prompt, err := RenderPrompt(promptStage, PromptVars{
		ProjectName:  project.Name,
		ChangeName:   p.Name,
		RepoPath:     project.RepoPath,
		WorktreePath: p.WorktreePath,
		Requirements: p.Description,
		RetryError:   p.ErrorMessage,
		RetryCount:   p.TotalRetries,
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
			Agent:      stage.Agent,
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
	case core.StageRequirements, core.StageSpecGen, core.StageSpecReview, core.StageCodeReview:
		cfg.Agent = "claude"
	case core.StageImplement, core.StageFixup:
		cfg.Agent = "codex"
	case core.StageWorktreeSetup, core.StageMerge, core.StageCleanup:
		cfg.Agent = ""
		cfg.Timeout = 2 * time.Minute
	}
	cfg.PromptTemplate = string(id)
	return cfg
}
