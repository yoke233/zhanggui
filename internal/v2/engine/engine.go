package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/yoke233/ai-workflow/internal/v2/core"
)

// StepExecutor is the callback that actually runs a step (e.g. calls an ACP agent).
// The engine does not know how steps are executed — this is injected.
type StepExecutor func(ctx context.Context, step *core.Step, exec *core.Execution) error

// FlowEngine orchestrates Flow execution: DAG scheduling, state transitions, events.
type FlowEngine struct {
	store      core.Store
	bus        core.EventBus
	sem        *Semaphore
	executor   StepExecutor
	resolver   Resolver           // optional: agent selection
	briefer    BriefingBuilder    // optional: briefing assembly
	collector  Collector          // optional: metadata extraction
	expander   CompositeExpander  // optional: composite decomposition
	wsProvider core.WorkspaceProvider // optional: workspace isolation
}

// Option configures the FlowEngine.
type Option func(*FlowEngine)

// WithConcurrency sets the max concurrent step executions.
func WithConcurrency(n int) Option {
	return func(e *FlowEngine) {
		e.sem = NewSemaphore(n)
	}
}

// WithResolver sets the agent resolver for the prepare phase.
func WithResolver(r Resolver) Option {
	return func(e *FlowEngine) { e.resolver = r }
}

// WithBriefingBuilder sets the briefing builder for the prepare phase.
func WithBriefingBuilder(b BriefingBuilder) Option {
	return func(e *FlowEngine) { e.briefer = b }
}

// WithCollector sets the metadata collector for the finalize phase.
func WithCollector(c Collector) Option {
	return func(e *FlowEngine) { e.collector = c }
}

// WithExpander sets the composite step expander.
func WithExpander(x CompositeExpander) Option {
	return func(e *FlowEngine) { e.expander = x }
}

// WithWorkspaceProvider sets the workspace provider for flow execution.
func WithWorkspaceProvider(p core.WorkspaceProvider) Option {
	return func(e *FlowEngine) { e.wsProvider = p }
}

// New creates a FlowEngine.
func New(store core.Store, bus core.EventBus, executor StepExecutor, opts ...Option) *FlowEngine {
	e := &FlowEngine{
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

// Run starts executing a Flow. It blocks until the Flow completes, fails, or the context is cancelled.
func (e *FlowEngine) Run(ctx context.Context, flowID int64) error {
	flow, err := e.store.GetFlow(ctx, flowID)
	if err != nil {
		return fmt.Errorf("get flow: %w", err)
	}
	if flow.Status != core.FlowPending && flow.Status != core.FlowQueued {
		return fmt.Errorf("flow %d is %s, expected pending or queued", flowID, flow.Status)
	}

	steps, err := e.store.ListStepsByFlow(ctx, flowID)
	if err != nil {
		return fmt.Errorf("list steps: %w", err)
	}
	if len(steps) == 0 {
		return core.ErrFlowNotRunnable
	}

	if err := ValidateDAG(steps); err != nil {
		return err
	}

	// Prepare workspace if project has resource bindings.
	if flow.ProjectID != nil && e.wsProvider != nil {
		project, err := e.store.GetProject(ctx, *flow.ProjectID)
		if err != nil {
			return fmt.Errorf("get project %d for workspace: %w", *flow.ProjectID, err)
		}
		bindings, err := e.store.ListResourceBindings(ctx, *flow.ProjectID)
		if err != nil {
			return fmt.Errorf("list resource bindings for project %d: %w", *flow.ProjectID, err)
		}
		if len(bindings) > 0 {
			ws, err := e.wsProvider.Prepare(ctx, project, bindings, flowID)
			if err != nil {
				return fmt.Errorf("prepare workspace for flow %d: %w", flowID, err)
			}
			defer e.wsProvider.Release(ctx, ws)
			ctx = ContextWithWorkspace(ctx, ws)
		}
	}

	// Transition flow to running.
	if err := e.store.UpdateFlowStatus(ctx, flowID, core.FlowRunning); err != nil {
		return fmt.Errorf("start flow: %w", err)
	}
	e.bus.Publish(ctx, core.Event{
		Type:      core.EventFlowStarted,
		FlowID:    flowID,
		Timestamp: time.Now().UTC(),
	})

	// Mark pending entry steps as ready.
	for _, s := range EntrySteps(steps) {
		if s.Status != core.StepPending {
			continue
		}
		if err := e.transitionStep(ctx, s, core.StepReady); err != nil {
			return err
		}
	}

	// Scheduling loop.
	if err := e.scheduleLoop(ctx, flowID); err != nil {
		_ = e.store.UpdateFlowStatus(ctx, flowID, core.FlowFailed)
		e.bus.Publish(ctx, core.Event{
			Type:      core.EventFlowFailed,
			FlowID:    flowID,
			Timestamp: time.Now().UTC(),
			Data:      map[string]any{"error": err.Error()},
		})
		return err
	}

	_ = e.store.UpdateFlowStatus(ctx, flowID, core.FlowDone)
	e.bus.Publish(ctx, core.Event{
		Type:      core.EventFlowCompleted,
		FlowID:    flowID,
		Timestamp: time.Now().UTC(),
	})
	return nil
}

// Cancel cancels a running Flow.
func (e *FlowEngine) Cancel(ctx context.Context, flowID int64) error {
	flow, err := e.store.GetFlow(ctx, flowID)
	if err != nil {
		return err
	}
	if !ValidFlowTransition(flow.Status, core.FlowCancelled) {
		return core.ErrInvalidTransition
	}
	if err := e.store.UpdateFlowStatus(ctx, flowID, core.FlowCancelled); err != nil {
		return err
	}
	e.bus.Publish(ctx, core.Event{
		Type:      core.EventFlowCancelled,
		FlowID:    flowID,
		Timestamp: time.Now().UTC(),
	})
	return nil
}

// scheduleLoop repeatedly promotes pending steps to ready, then executes ready steps,
// until all steps are done or an unrecoverable error occurs.
func (e *FlowEngine) scheduleLoop(ctx context.Context, flowID int64) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		steps, err := e.store.ListStepsByFlow(ctx, flowID)
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
			return fmt.Errorf("step(s) failed in flow %d", flowID)
		}

		// Phase 1: promote pending steps whose deps are all done → ready.
		for _, s := range PromotableSteps(steps) {
			if err := e.transitionStep(ctx, s, core.StepReady); err != nil {
				return err
			}
		}

		// Re-fetch after promotions to get updated statuses.
		steps, err = e.store.ListStepsByFlow(ctx, flowID)
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
				return fmt.Errorf("flow %d is stuck: no runnable, running, or waiting steps", flowID)
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
				// the sub-flow's children need semaphore slots from the same pool.
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
// Composite steps take a separate path: expand → run sub-flow → done/fail.
func (e *FlowEngine) executeStep(ctx context.Context, step *core.Step) error {
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
		FlowID:           step.FlowID,
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
		FlowID:    step.FlowID,
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
		FlowID:    step.FlowID,
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
func (e *FlowEngine) transitionStep(ctx context.Context, step *core.Step, to core.StepStatus) error {
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
		FlowID:    step.FlowID,
		StepID:    step.ID,
		Timestamp: time.Now().UTC(),
	})
	return nil
}
