package flow

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// StepExecutor is the callback that actually runs a step (e.g. calls an ACP agent).
// The engine does not know how steps are executed — this is injected.
type StepExecutor func(ctx context.Context, step *core.Step, exec *core.Execution) error

// IssueEngine orchestrates Issue execution: sequential step scheduling, state transitions, events.
type IssueEngine struct {
	store      Store
	bus        EventPublisher
	sem        *Semaphore
	executor   StepExecutor
	resolver   Resolver              // optional: agent selection
	briefer    BriefingBuilder       // optional: briefing assembly
	collector  Collector             // optional: metadata extraction
	expander   CompositeExpander     // optional: composite decomposition
	wsProvider WorkspaceProvider     // optional: workspace isolation
	ghTokens   GitHubTokens          // optional: PR automation tokens (commit/merge)
	prPrompts  PRFlowPromptsProvider // optional: configurable PR flow prompts
	crFactory  ChangeRequestProviderFactory
}

// Option configures the IssueEngine.
type Option func(*IssueEngine)

// WithConcurrency sets the max concurrent step executions.
func WithConcurrency(n int) Option {
	return func(e *IssueEngine) {
		e.sem = NewSemaphore(n)
	}
}

// WithResolver sets the agent resolver for the prepare phase.
func WithResolver(r Resolver) Option {
	return func(e *IssueEngine) { e.resolver = r }
}

// WithBriefingBuilder sets the briefing builder for the prepare phase.
func WithBriefingBuilder(b BriefingBuilder) Option {
	return func(e *IssueEngine) { e.briefer = b }
}

// WithCollector sets the metadata collector for the finalize phase.
func WithCollector(c Collector) Option {
	return func(e *IssueEngine) { e.collector = c }
}

// WithExpander sets the composite step expander.
func WithExpander(x CompositeExpander) Option {
	return func(e *IssueEngine) { e.expander = x }
}

// WithWorkspaceProvider sets the workspace provider for issue execution.
func WithWorkspaceProvider(p WorkspaceProvider) Option {
	return func(e *IssueEngine) { e.wsProvider = p }
}

// WithGitHubTokens sets optional GitHub tokens used by builtin PR automation (push/open PR/merge).
func WithGitHubTokens(t GitHubTokens) Option {
	return func(e *IssueEngine) { e.ghTokens = t }
}

// WithPRFlowPromptsProvider sets a provider for configurable PR flow prompts.
func WithPRFlowPromptsProvider(provider PRFlowPromptsProvider) Option {
	return func(e *IssueEngine) { e.prPrompts = provider }
}

// ChangeRequestProviderFactory resolves provider implementations for PR/MR automation.
type ChangeRequestProviderFactory func(token string) []ChangeRequestProvider

// WithChangeRequestProviders sets the provider factory used by gate auto-merge flow.
func WithChangeRequestProviders(factory ChangeRequestProviderFactory) Option {
	return func(e *IssueEngine) { e.crFactory = factory }
}

// New creates an IssueEngine.
func New(store Store, bus EventPublisher, executor StepExecutor, opts ...Option) *IssueEngine {
	e := &IssueEngine{
		store:    store,
		bus:      bus,
		sem:      NewSemaphore(4),
		executor: executor,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

func (e *IssueEngine) getPRFlowPrompts() PRFlowPrompts {
	if e != nil && e.prPrompts != nil {
		return MergePRFlowPrompts(e.prPrompts())
	}
	return DefaultPRFlowPrompts()
}

// Run starts executing an Issue. It blocks until the Issue completes, fails, or the context is cancelled.
func (e *IssueEngine) Run(ctx context.Context, issueID int64) error {
	issue, err := e.store.GetIssue(ctx, issueID)
	if err != nil {
		return fmt.Errorf("get issue: %w", err)
	}
	if issue.Status != core.IssueOpen && issue.Status != core.IssueAccepted && issue.Status != core.IssueQueued {
		return fmt.Errorf("issue %d is %s, expected open, accepted, or queued", issueID, issue.Status)
	}

	steps, err := e.store.ListStepsByIssue(ctx, issueID)
	if err != nil {
		return fmt.Errorf("list steps: %w", err)
	}
	if len(steps) == 0 {
		return core.ErrFlowNotRunnable
	}

	// Validate step ordering (sequential by Position).
	if err := ValidateSteps(steps); err != nil {
		return err
	}

	// Prepare workspace if project has resource bindings.
	if issue.ProjectID != nil && e.wsProvider != nil {
		project, err := e.store.GetProject(ctx, *issue.ProjectID)
		if err != nil {
			return fmt.Errorf("get project %d for workspace: %w", *issue.ProjectID, err)
		}
		bindings, err := e.store.ListResourceBindings(ctx, *issue.ProjectID)
		if err != nil {
			return fmt.Errorf("list resource bindings for project %d: %w", *issue.ProjectID, err)
		}
		if len(bindings) > 0 {
			ws, err := e.wsProvider.Prepare(ctx, project, bindings, issueID)
			if err != nil {
				return fmt.Errorf("prepare workspace for issue %d: %w", issueID, err)
			}
			defer e.wsProvider.Release(ctx, ws)
			ctx = ContextWithWorkspace(ctx, ws)
		}
	}

	// Transition issue to running.
	if err := e.store.UpdateIssueStatus(ctx, issueID, core.IssueRunning); err != nil {
		return fmt.Errorf("start issue: %w", err)
	}
	e.bus.Publish(ctx, core.Event{
		Type:      core.EventIssueStarted,
		IssueID:   issueID,
		Timestamp: time.Now().UTC(),
	})

	// Mark the first step (by Position) as ready.
	firstSteps := EntrySteps(steps)
	for _, s := range firstSteps {
		if s.Status != core.StepPending {
			continue
		}
		if err := e.transitionStep(ctx, s, core.StepReady); err != nil {
			return err
		}
	}

	// Scheduling loop.
	if err := e.scheduleLoop(ctx, issueID); err != nil {
		_ = e.store.UpdateIssueStatus(ctx, issueID, core.IssueFailed)
		e.bus.Publish(ctx, core.Event{
			Type:      core.EventIssueFailed,
			IssueID:   issueID,
			Timestamp: time.Now().UTC(),
			Data:      map[string]any{"error": err.Error()},
		})
		return err
	}

	_ = e.store.UpdateIssueStatus(ctx, issueID, core.IssueDone)
	e.bus.Publish(ctx, core.Event{
		Type:      core.EventIssueCompleted,
		IssueID:   issueID,
		Timestamp: time.Now().UTC(),
	})
	return nil
}

// Cancel cancels a running Issue.
func (e *IssueEngine) Cancel(ctx context.Context, issueID int64) error {
	issue, err := e.store.GetIssue(ctx, issueID)
	if err != nil {
		return err
	}
	if !ValidIssueTransition(issue.Status, core.IssueCancelled) {
		return core.ErrInvalidTransition
	}
	if err := e.store.UpdateIssueStatus(ctx, issueID, core.IssueCancelled); err != nil {
		return err
	}
	e.bus.Publish(ctx, core.Event{
		Type:      core.EventIssueCancelled,
		IssueID:   issueID,
		Timestamp: time.Now().UTC(),
	})
	return nil
}

// scheduleLoop executes steps sequentially by Position until all are done or an error occurs.
func (e *IssueEngine) scheduleLoop(ctx context.Context, issueID int64) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		steps, err := e.store.ListStepsByIssue(ctx, issueID)
		if err != nil {
			return fmt.Errorf("list steps in loop: %w", err)
		}

		// Check termination conditions.
		allDone := true
		anyFailed := false
		for _, s := range steps {
			switch s.Status {
			case core.StepDone, core.StepCancelled:
				continue
			case core.StepFailed:
				anyFailed = true
				allDone = false
			default:
				allDone = false
			}
		}
		if allDone {
			return nil
		}
		if anyFailed {
			return fmt.Errorf("step(s) failed in issue %d", issueID)
		}

		// Phase 1: promote pending steps whose predecessors (by Position) are all done → ready.
		for _, s := range PromotableSteps(steps) {
			if err := e.transitionStep(ctx, s, core.StepReady); err != nil {
				return err
			}
		}

		// Re-fetch after promotions to get updated statuses.
		steps, err = e.store.ListStepsByIssue(ctx, issueID)
		if err != nil {
			return fmt.Errorf("list steps after promote: %w", err)
		}

		// Phase 2: dispatch all ready steps for execution.
		runnable := RunnableSteps(steps)
		if len(runnable) == 0 {
			hasActive := false
			for _, s := range steps {
				if s.Status == core.StepRunning || s.Status == core.StepWaitingGate {
					hasActive = true
					break
				}
			}
			if !hasActive {
				return fmt.Errorf("issue %d is stuck: no runnable, running, or waiting steps", issueID)
			}
			time.Sleep(10 * time.Millisecond)
			continue
		}

		var mu sync.Mutex
		var execErr error
		var wg sync.WaitGroup

		for _, step := range runnable {
			step := step
			if err := e.transitionStep(ctx, step, core.StepRunning); err != nil {
				return err
			}

			wg.Add(1)
			if step.Type == core.StepComposite {
				// Composite steps don't hold a semaphore slot to avoid deadlock:
				// the child issue's steps need semaphore slots from the same pool.
				go func() {
					defer wg.Done()
					err := e.executeStep(ctx, step)
					if err != nil {
						mu.Lock()
						if execErr == nil {
							execErr = err
						}
						mu.Unlock()
					}
				}()
			} else {
				e.sem.Acquire()
				go func() {
					defer wg.Done()
					defer e.sem.Release()

					err := e.executeStep(ctx, step)
					if err != nil {
						mu.Lock()
						if execErr == nil {
							execErr = err
						}
						mu.Unlock()
					}
				}()
			}
		}
		wg.Wait()

		if execErr != nil {
			return execErr
		}
	}
}

// executeStep runs the three-phase engine pipeline: prepare → execute → finalize.
// Composite steps take a separate path: expand → run child issue → done/fail.
func (e *IssueEngine) executeStep(ctx context.Context, step *core.Step) error {
	if step.Type == core.StepComposite {
		return e.executeComposite(ctx, step)
	}

	// --- prepare: resolve agent + build briefing ---
	agentID, snapshot, err := e.prepare(ctx, step)
	if err != nil {
		return err
	}

	exec := &core.Execution{
		StepID:           step.ID,
		IssueID:          step.IssueID,
		Status:           core.ExecCreated,
		AgentID:          agentID,
		BriefingSnapshot: snapshot,
		Attempt:          step.RetryCount + 1,
	}
	execID, err := e.store.CreateExecution(ctx, exec)
	if err != nil {
		return fmt.Errorf("create execution for step %d: %w", step.ID, err)
	}
	exec.ID = execID

	e.bus.Publish(ctx, core.Event{
		Type:      core.EventExecCreated,
		IssueID:   step.IssueID,
		StepID:    step.ID,
		ExecID:    execID,
		Timestamp: time.Now().UTC(),
	})

	// --- execute: run via callback, with optional timeout ---
	now := time.Now().UTC()
	exec.Status = core.ExecRunning
	exec.StartedAt = &now
	_ = e.store.UpdateExecution(ctx, exec)

	e.bus.Publish(ctx, core.Event{
		Type:      core.EventExecStarted,
		IssueID:   step.IssueID,
		StepID:    step.ID,
		ExecID:    execID,
		Timestamp: time.Now().UTC(),
	})

	execCtx := ctx
	if step.Timeout > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, step.Timeout)
		defer cancel()
	}

	execErr := e.executor(execCtx, step, exec)

	// --- finalize: classify result → retry/block/fail/gate/done ---
	return e.finalize(ctx, step, exec, execErr)
}

// transitionStep validates and applies a step status transition.
func (e *IssueEngine) transitionStep(ctx context.Context, step *core.Step, to core.StepStatus) error {
	if !ValidStepTransition(step.Status, to) {
		return fmt.Errorf("%w: step %d %s → %s", core.ErrInvalidTransition, step.ID, step.Status, to)
	}
	if err := e.store.UpdateStepStatus(ctx, step.ID, to); err != nil {
		return fmt.Errorf("update step %d to %s: %w", step.ID, to, err)
	}
	step.Status = to

	// Emit appropriate event.
	var evType core.EventType
	switch to {
	case core.StepReady:
		evType = core.EventStepReady
	case core.StepRunning:
		evType = core.EventStepStarted
	case core.StepDone:
		evType = core.EventStepCompleted
	case core.StepFailed:
		evType = core.EventStepFailed
	case core.StepBlocked:
		evType = core.EventStepBlocked
	default:
		return nil
	}
	e.bus.Publish(ctx, core.Event{
		Type:      evType,
		IssueID:   step.IssueID,
		StepID:    step.ID,
		Timestamp: time.Now().UTC(),
	})
	return nil
}
