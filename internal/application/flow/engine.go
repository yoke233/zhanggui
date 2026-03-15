package flow

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// ActionExecutor is the callback that actually runs an action (e.g. calls an ACP agent).
// The engine does not know how actions are executed — this is injected.
type ActionExecutor func(ctx context.Context, action *core.Action, run *core.Run) error

type workflowRuntime struct {
	store    Store
	bus      EventPublisher
	sem      *Semaphore
	executor ActionExecutor
}

type preparationService struct {
	resolver     Resolver
	inputBuilder InputBuilder
	collector    Collector
	expander     CompositeExpander
	workspace    WorkspaceProvider
	resources    *ResourceResolver
}

type gateService struct {
	scmTokens      SCMTokens
	prPrompts      PRFlowPromptsProvider
	crFactory      ChangeRequestProviderFactory
	gateEvaluators []GateEvaluator
}

// WorkItemEngine orchestrates WorkItem execution: sequential action scheduling, state transitions, events.
type WorkItemEngine struct {
	workflow    workflowRuntime
	preparation preparationService
	gates       gateService
}

// Option configures the WorkItemEngine.
type Option func(*WorkItemEngine)

// WithConcurrency sets the max concurrent action executions.
func WithConcurrency(n int) Option {
	return func(e *WorkItemEngine) {
		e.workflow.sem = NewSemaphore(n)
	}
}

// WithResolver sets the agent resolver for the prepare phase.
func WithResolver(r Resolver) Option {
	return func(e *WorkItemEngine) { e.preparation.resolver = r }
}

// WithInputBuilder sets the input builder for the prepare phase.
func WithInputBuilder(b InputBuilder) Option {
	return func(e *WorkItemEngine) { e.preparation.inputBuilder = b }
}

// WithBriefingBuilder is an alias for WithInputBuilder for backward compatibility.
func WithBriefingBuilder(b InputBuilder) Option {
	return WithInputBuilder(b)
}

// WithCollector sets the metadata collector for the finalize phase.
func WithCollector(c Collector) Option {
	return func(e *WorkItemEngine) { e.preparation.collector = c }
}

// WithExpander sets the composite action expander.
func WithExpander(x CompositeExpander) Option {
	return func(e *WorkItemEngine) { e.preparation.expander = x }
}

// WithWorkspaceProvider sets the workspace provider for work item execution.
func WithWorkspaceProvider(p WorkspaceProvider) Option {
	return func(e *WorkItemEngine) { e.preparation.workspace = p }
}

// WithSCMTokens sets optional GitHub tokens used by builtin PR automation (push/open PR/merge).
func WithSCMTokens(t SCMTokens) Option {
	return func(e *WorkItemEngine) { e.gates.scmTokens = t }
}

// WithPRFlowPromptsProvider sets a provider for configurable PR flow prompts.
func WithPRFlowPromptsProvider(provider PRFlowPromptsProvider) Option {
	return func(e *WorkItemEngine) { e.gates.prPrompts = provider }
}

// ChangeRequestProviderFactory resolves provider implementations for PR/MR automation.
type ChangeRequestProviderFactory func(token string) []ChangeRequestProvider

// WithChangeRequestProviders sets the provider factory used by gate auto-merge flow.
func WithChangeRequestProviders(factory ChangeRequestProviderFactory) Option {
	return func(e *WorkItemEngine) { e.gates.crFactory = factory }
}

// WithGateEvaluators overrides the default gate evaluation chain (signal→manifest→deliverable).
// Evaluators are tried in order; the first one that returns Decided=true wins.
func WithGateEvaluators(evaluators ...GateEvaluator) Option {
	return func(e *WorkItemEngine) { e.gates.gateEvaluators = evaluators }
}

// WithResourceResolver sets the external resource resolver for input fetch / output deposit.
func WithResourceResolver(rr *ResourceResolver) Option {
	return func(e *WorkItemEngine) { e.preparation.resources = rr }
}

// New creates a WorkItemEngine.
func New(store Store, bus EventPublisher, executor ActionExecutor, opts ...Option) *WorkItemEngine {
	e := &WorkItemEngine{
		workflow: workflowRuntime{
			store:    store,
			bus:      bus,
			sem:      NewSemaphore(4),
			executor: executor,
		},
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

func (e *WorkItemEngine) getPRFlowPrompts() PRFlowPrompts {
	if e != nil && e.gates.prPrompts != nil {
		return MergePRFlowPrompts(e.gates.prPrompts())
	}
	return DefaultPRFlowPrompts()
}

// MaxConcurrency returns the engine's configured action execution concurrency.
func (e *WorkItemEngine) MaxConcurrency() int {
	if e == nil || e.workflow.sem == nil {
		return 0
	}
	return e.workflow.sem.Capacity()
}

// Run starts executing a WorkItem. It blocks until the WorkItem completes, fails, or the context is cancelled.
func (e *WorkItemEngine) Run(ctx context.Context, workItemID int64) error {
	workItem, err := e.workflow.store.GetWorkItem(ctx, workItemID)
	if err != nil {
		return fmt.Errorf("get work item: %w", err)
	}
	if workItem.Status != core.WorkItemOpen && workItem.Status != core.WorkItemAccepted && workItem.Status != core.WorkItemQueued {
		return fmt.Errorf("work item %d is %s, expected open, accepted, or queued", workItemID, workItem.Status)
	}

	actions, err := e.workflow.store.ListActionsByWorkItem(ctx, workItemID)
	if err != nil {
		return fmt.Errorf("list actions: %w", err)
	}
	if len(actions) == 0 {
		return core.ErrWorkItemNotRunnable
	}

	// Validate action ordering (sequential by Position).
	if err := ValidateActions(actions); err != nil {
		return err
	}

	// Prepare workspace if project has resource spaces.
	if workItem.ProjectID != nil && e.preparation.workspace != nil {
		project, err := e.workflow.store.GetProject(ctx, *workItem.ProjectID)
		if err != nil {
			return fmt.Errorf("get project %d for workspace: %w", *workItem.ProjectID, err)
		}
		spaces, err := e.workflow.store.ListResourceSpaces(ctx, *workItem.ProjectID)
		if err != nil {
			return fmt.Errorf("list resource spaces for project %d: %w", *workItem.ProjectID, err)
		}
		if workItem.ResourceBindingID != nil {
			filtered := make([]*core.ResourceSpace, 0, 1)
			for _, space := range spaces {
				if space != nil && space.ID == *workItem.ResourceBindingID {
					filtered = append(filtered, space)
					break
				}
			}
			if len(filtered) == 0 {
				return fmt.Errorf("resource space %d not found in project %d", *workItem.ResourceBindingID, *workItem.ProjectID)
			}
			spaces = filtered
		}
		if len(spaces) > 0 {
			ws, err := e.preparation.workspace.Prepare(ctx, project, spaces, workItemID)
			if err != nil {
				return fmt.Errorf("prepare workspace for work item %d: %w", workItemID, err)
			}
			defer e.preparation.workspace.Release(ctx, ws)
			ctx = ContextWithWorkspace(ctx, ws)

			// Surface workspace preparation warnings to the user via event bus.
			if ws.Metadata != nil {
				if warnings, ok := ws.Metadata["warnings"].([]string); ok && len(warnings) > 0 {
					e.workflow.bus.Publish(ctx, core.Event{
						Type:       core.EventWorkspaceWarning,
						WorkItemID: workItemID,
						Data: map[string]any{
							"warnings": warnings,
						},
						Timestamp: time.Now().UTC(),
					})
				}
			}
		}
	}

	// Transition work item to running.
	if err := e.workflow.store.UpdateWorkItemStatus(ctx, workItemID, core.WorkItemRunning); err != nil {
		return fmt.Errorf("start work item: %w", err)
	}
	e.workflow.bus.Publish(ctx, core.Event{
		Type:       core.EventWorkItemStarted,
		WorkItemID: workItemID,
		Timestamp:  time.Now().UTC(),
	})

	// Mark the first action (by Position) as ready.
	firstActions := EntryActions(actions)
	for _, a := range firstActions {
		if a.Status != core.ActionPending {
			continue
		}
		if err := e.transitionAction(ctx, a, core.ActionReady); err != nil {
			return err
		}
	}

	// Scheduling loop.
	if err := e.scheduleLoop(ctx, workItemID); err != nil {
		_ = e.workflow.store.UpdateWorkItemStatus(ctx, workItemID, core.WorkItemFailed)
		e.workflow.bus.Publish(ctx, core.Event{
			Type:       core.EventWorkItemFailed,
			WorkItemID: workItemID,
			Timestamp:  time.Now().UTC(),
			Data:       map[string]any{"error": err.Error()},
		})
		return err
	}

	_ = e.workflow.store.UpdateWorkItemStatus(ctx, workItemID, core.WorkItemDone)
	e.workflow.bus.Publish(ctx, core.Event{
		Type:       core.EventWorkItemCompleted,
		WorkItemID: workItemID,
		Timestamp:  time.Now().UTC(),
	})
	return nil
}

// Cancel cancels a running WorkItem.
func (e *WorkItemEngine) Cancel(ctx context.Context, workItemID int64) error {
	workItem, err := e.workflow.store.GetWorkItem(ctx, workItemID)
	if err != nil {
		return err
	}
	if !ValidWorkItemTransition(workItem.Status, core.WorkItemCancelled) {
		return core.ErrInvalidTransition
	}
	if err := e.workflow.store.UpdateWorkItemStatus(ctx, workItemID, core.WorkItemCancelled); err != nil {
		return err
	}
	e.workflow.bus.Publish(ctx, core.Event{
		Type:       core.EventWorkItemCancelled,
		WorkItemID: workItemID,
		Timestamp:  time.Now().UTC(),
	})
	return nil
}

// scheduleLoop executes actions sequentially by Position until all are done or an error occurs.
func (e *WorkItemEngine) scheduleLoop(ctx context.Context, workItemID int64) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		actions, err := e.workflow.store.ListActionsByWorkItem(ctx, workItemID)
		if err != nil {
			return fmt.Errorf("list actions in loop: %w", err)
		}

		// Check termination conditions.
		allDone := true
		anyFailed := false
		for _, a := range actions {
			switch a.Status {
			case core.ActionDone, core.ActionCancelled:
				continue
			case core.ActionFailed:
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
			return fmt.Errorf("action(s) failed in work item %d", workItemID)
		}

		// Phase 1: promote pending actions whose predecessors (by Position) are all done → ready.
		for _, a := range PromotableActions(actions) {
			if err := e.transitionAction(ctx, a, core.ActionReady); err != nil {
				return err
			}
		}

		// Re-fetch after promotions to get updated statuses.
		actions, err = e.workflow.store.ListActionsByWorkItem(ctx, workItemID)
		if err != nil {
			return fmt.Errorf("list actions after promote: %w", err)
		}

		// Phase 2: dispatch all ready actions for execution.
		runnable := RunnableActions(actions)
		if len(runnable) == 0 {
			hasActive := false
			for _, a := range actions {
				if a.Status == core.ActionRunning || a.Status == core.ActionWaitingGate {
					hasActive = true
					break
				}
			}
			if !hasActive {
				return fmt.Errorf("work item %d is stuck: no runnable, running, or waiting actions", workItemID)
			}
			time.Sleep(10 * time.Millisecond)
			continue
		}

		var mu sync.Mutex
		var runErr error
		var wg sync.WaitGroup

		for _, action := range runnable {
			action := action
			if err := e.transitionAction(ctx, action, core.ActionRunning); err != nil {
				return err
			}

			wg.Add(1)
			if action.Type == core.ActionPlan {
				// Composite actions don't hold a semaphore slot to avoid deadlock:
				// the child work item's actions need semaphore slots from the same pool.
				go func() {
					defer wg.Done()
					err := e.executeAction(ctx, action)
					if err != nil {
						mu.Lock()
						if runErr == nil {
							runErr = err
						}
						mu.Unlock()
					}
				}()
			} else {
				e.workflow.sem.Acquire()
				go func() {
					defer wg.Done()
					defer e.workflow.sem.Release()

					err := e.executeAction(ctx, action)
					if err != nil {
						mu.Lock()
						if runErr == nil {
							runErr = err
						}
						mu.Unlock()
					}
				}()
			}
		}
		wg.Wait()

		if runErr != nil {
			return runErr
		}
	}
}

// executeAction runs the three-phase engine pipeline: prepare → execute → finalize.
// Composite actions take a separate path: expand → run child work item → done/fail.
func (e *WorkItemEngine) executeAction(ctx context.Context, action *core.Action) error {
	if action.Type == core.ActionPlan {
		return e.executeComposite(ctx, action)
	}

	// --- prepare: resolve agent + build input ---
	agentID, inputSnapshot, err := e.prepare(ctx, action)
	if err != nil {
		return err
	}

	run := &core.Run{
		ActionID:         action.ID,
		WorkItemID:       action.WorkItemID,
		Status:           core.RunCreated,
		AgentID:          agentID,
		BriefingSnapshot: inputSnapshot,
		Attempt:          action.RetryCount + 1,
	}
	runID, err := e.workflow.store.CreateRun(ctx, run)
	if err != nil {
		return fmt.Errorf("create run for action %d: %w", action.ID, err)
	}
	run.ID = runID

	e.workflow.bus.Publish(ctx, core.Event{
		Type:       core.EventRunCreated,
		WorkItemID: action.WorkItemID,
		ActionID:   action.ID,
		RunID:      runID,
		Timestamp:  time.Now().UTC(),
	})

	// --- execute: run via callback, with optional timeout ---
	now := time.Now().UTC()
	run.Status = core.RunRunning
	run.StartedAt = &now
	_ = e.workflow.store.UpdateRun(ctx, run)

	e.workflow.bus.Publish(ctx, core.Event{
		Type:       core.EventRunStarted,
		WorkItemID: action.WorkItemID,
		ActionID:   action.ID,
		RunID:      runID,
		Timestamp:  time.Now().UTC(),
	})

	runCtx := ctx
	if action.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, action.Timeout)
		defer cancel()
	}

	runErr := e.workflow.executor(runCtx, action, run)

	// --- finalize: classify result → retry/block/fail/gate/done ---
	return e.finalize(ctx, action, run, runErr)
}

// transitionAction validates and applies an action status transition.
func (e *WorkItemEngine) transitionAction(ctx context.Context, action *core.Action, to core.ActionStatus) error {
	if !ValidActionTransition(action.Status, to) {
		return fmt.Errorf("%w: action %d %s → %s", core.ErrInvalidTransition, action.ID, action.Status, to)
	}
	if err := e.workflow.store.UpdateActionStatus(ctx, action.ID, to); err != nil {
		return fmt.Errorf("update action %d to %s: %w", action.ID, to, err)
	}
	action.Status = to

	// Emit appropriate event.
	var evType core.EventType
	switch to {
	case core.ActionReady:
		evType = core.EventActionReady
	case core.ActionRunning:
		evType = core.EventActionStarted
	case core.ActionDone:
		evType = core.EventActionCompleted
	case core.ActionFailed:
		evType = core.EventActionFailed
	case core.ActionBlocked:
		evType = core.EventActionBlocked
	default:
		return nil
	}
	e.workflow.bus.Publish(ctx, core.Event{
		Type:       evType,
		WorkItemID: action.WorkItemID,
		ActionID:   action.ID,
		Timestamp:  time.Now().UTC(),
	})
	return nil
}
