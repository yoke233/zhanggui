package secretary

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/user/ai-workflow/internal/core"
	"github.com/user/ai-workflow/internal/engine"
)

type eventSubscriber interface {
	Subscribe() chan core.Event
	Unsubscribe(ch chan core.Event)
}

type eventPublisher interface {
	Publish(evt core.Event)
}

type pipelineRef struct {
	planID string
	taskID string
}

type readyDispatch struct {
	planID string
	taskID string
}

type runningPlan struct {
	Plan      *core.TaskPlan
	Graph     *DAG
	Running   map[string]string
	TaskByID  map[string]*core.TaskItem
	Parents   map[string][]string
	HaltNew   bool
	Recovered bool
}

func newRunningPlan(plan *core.TaskPlan, graph *DAG) *runningPlan {
	taskByID := make(map[string]*core.TaskItem, len(plan.Tasks))
	for i := range plan.Tasks {
		taskByID[plan.Tasks[i].ID] = &plan.Tasks[i]
	}

	parents := make(map[string][]string, len(graph.Nodes))
	for taskID := range graph.Nodes {
		parents[taskID] = []string{}
	}
	for from, downstream := range graph.Downstream {
		for _, to := range downstream {
			parents[to] = append(parents[to], from)
		}
	}
	for taskID := range parents {
		sort.Strings(parents[taskID])
	}

	return &runningPlan{
		Plan:     plan,
		Graph:    graph,
		Running:  make(map[string]string),
		TaskByID: taskByID,
		Parents:  parents,
	}
}

// DepScheduler schedules TaskItems by DAG dependencies and maps each task to one pipeline.
type DepScheduler struct {
	store   core.Store
	bus     eventSubscriber
	pub     eventPublisher
	tracker core.Tracker

	runPipeline func(context.Context, string) error
	sem         chan struct{}

	mu            sync.Mutex
	plans         map[string]*runningPlan
	pipelineIndex map[string]pipelineRef
	lastPlanID    string

	loopCancel context.CancelFunc
	loopWG     sync.WaitGroup
}

func NewDepScheduler(
	store core.Store,
	bus eventSubscriber,
	runPipeline func(context.Context, string) error,
	tracker core.Tracker,
	maxConcurrent int,
) *DepScheduler {
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	if runPipeline == nil {
		runPipeline = func(context.Context, string) error { return nil }
	}
	var pub eventPublisher
	if typed, ok := bus.(eventPublisher); ok {
		pub = typed
	}

	return &DepScheduler{
		store:         store,
		bus:           bus,
		pub:           pub,
		tracker:       tracker,
		runPipeline:   runPipeline,
		sem:           make(chan struct{}, maxConcurrent),
		plans:         make(map[string]*runningPlan),
		pipelineIndex: make(map[string]pipelineRef),
	}
}

func (s *DepScheduler) Start(ctx context.Context) error {
	if s == nil || s.bus == nil {
		return nil
	}

	s.mu.Lock()
	if s.loopCancel != nil {
		s.mu.Unlock()
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.loopCancel = cancel
	ch := s.bus.Subscribe()
	s.loopWG.Add(1)
	s.mu.Unlock()

	go func() {
		defer s.loopWG.Done()
		defer s.bus.Unsubscribe(ch)
		for {
			select {
			case <-runCtx.Done():
				return
			case evt, ok := <-ch:
				if !ok {
					return
				}
				_ = s.OnEvent(context.Background(), evt)
			}
		}
	}()
	return nil
}

func (s *DepScheduler) Stop(ctx context.Context) error {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	cancel := s.loopCancel
	s.loopCancel = nil
	s.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.loopWG.Wait()
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// StartPlan builds DAG, validates and transitive-reduces it, then dispatches initial ready tasks.
func (s *DepScheduler) StartPlan(ctx context.Context, plan *core.TaskPlan) error {
	if s == nil || s.store == nil {
		return errors.New("scheduler store is not configured")
	}
	if plan == nil {
		return errors.New("plan is nil")
	}
	if plan.ID == "" {
		return errors.New("plan id is required")
	}
	s.publishPlanEvent(core.EventSecretaryThinking, plan, map[string]string{
		"phase": "start_plan",
	})

	s.mu.Lock()
	_, alreadyRunning := s.plans[plan.ID]
	s.mu.Unlock()
	if alreadyRunning {
		// Idempotent re-entry: the plan is already managed by this scheduler instance.
		return nil
	}
	if plan.Status == core.PlanExecuting {
		// Re-entry after process restart should go through recovery path.
		return s.RecoverPlan(ctx, plan.ID)
	}
	if err := validateStartPlanState(plan); err != nil {
		return err
	}

	tasks, err := s.loadPlanTasks(plan)
	if err != nil {
		return err
	}
	plan.Tasks = tasks
	if plan.FailPolicy == "" {
		plan.FailPolicy = core.FailBlock
	}

	graph := Build(plan.Tasks)
	if err := graph.Validate(); err != nil {
		plan.Status = core.PlanWaitingHuman
		plan.WaitReason = core.WaitFeedbackReq
		if saveErr := s.savePlan(plan); saveErr != nil {
			return fmt.Errorf("validate dag: %w (save plan: %v)", err, saveErr)
		}
		s.publishEvent(core.Event{
			Type:      core.EventPlanWaitingHuman,
			ProjectID: plan.ProjectID,
			PlanID:    plan.ID,
			Data: map[string]string{
				"wait_reason": string(plan.WaitReason),
			},
			Error:     err.Error(),
			Timestamp: time.Now(),
		})
		return err
	}
	graph.TransitiveReduce()
	if err := graph.Validate(); err != nil {
		plan.Status = core.PlanWaitingHuman
		plan.WaitReason = core.WaitFeedbackReq
		if saveErr := s.savePlan(plan); saveErr != nil {
			return fmt.Errorf("validate reduced dag: %w (save plan: %v)", err, saveErr)
		}
		s.publishEvent(core.Event{
			Type:      core.EventPlanWaitingHuman,
			ProjectID: plan.ProjectID,
			PlanID:    plan.ID,
			Data: map[string]string{
				"wait_reason": string(plan.WaitReason),
			},
			Error:     err.Error(),
			Timestamp: time.Now(),
		})
		return err
	}

	rp := newRunningPlan(plan, graph)
	for _, taskID := range sortedTaskIDs(rp.TaskByID) {
		item := rp.TaskByID[taskID]
		if isTaskTerminal(item.Status) {
			continue
		}
		item.Status = core.ItemPending
		item.PipelineID = ""
		if err := s.saveTask(item); err != nil {
			return err
		}
	}

	plan.Status = core.PlanExecuting
	plan.WaitReason = core.WaitNone
	if err := s.savePlan(plan); err != nil {
		return err
	}
	s.publishPlanEvent(core.EventPlanApproved, plan, map[string]string{
		"phase": "start_plan",
	})

	for _, taskID := range graph.ReadyNodes() {
		task := rp.TaskByID[taskID]
		if task == nil || task.Status != core.ItemPending {
			continue
		}
		task.Status = core.ItemReady
		if err := s.saveTask(task); err != nil {
			return err
		}
		s.publishTaskEvent(core.EventTaskReady, plan, task, "")
	}

	s.mu.Lock()
	s.plans[plan.ID] = rp
	s.mu.Unlock()

	return s.dispatchReadyAcrossPlans(ctx)
}

// OnEvent handles pipeline_done/pipeline_failed events and advances TaskItem/TaskPlan state.
func (s *DepScheduler) OnEvent(ctx context.Context, evt core.Event) error {
	if s == nil {
		return nil
	}
	if evt.Type != core.EventPipelineDone && evt.Type != core.EventPipelineFailed {
		return nil
	}
	if evt.PipelineID == "" {
		return nil
	}

	s.mu.Lock()
	err := s.handlePipelineEventLocked(evt)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return s.dispatchReadyAcrossPlans(ctx)
}

// RecoverExecutingPlans is the minimal crash-recovery entrypoint.
func (s *DepScheduler) RecoverExecutingPlans(ctx context.Context) error {
	if s == nil || s.store == nil {
		return errors.New("scheduler store is not configured")
	}

	plans, err := s.store.GetActiveTaskPlans()
	if err != nil {
		return err
	}
	for i := range plans {
		if plans[i].Status != core.PlanExecuting {
			continue
		}
		if err := s.RecoverPlan(ctx, plans[i].ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *DepScheduler) RecoverPlan(ctx context.Context, planID string) error {
	if s == nil || s.store == nil {
		return errors.New("scheduler store is not configured")
	}
	if planID == "" {
		return errors.New("plan id is required")
	}

	plan, err := s.store.GetTaskPlan(planID)
	if err != nil {
		return err
	}
	s.publishPlanEvent(core.EventSecretaryThinking, plan, map[string]string{
		"phase": "recover_plan",
	})
	if plan.FailPolicy == "" {
		plan.FailPolicy = core.FailBlock
	}
	tasks, err := s.store.GetTaskItemsByPlan(planID)
	if err != nil {
		return err
	}
	plan.Tasks = tasks

	graph := Build(plan.Tasks)
	if err := graph.Validate(); err != nil {
		return err
	}
	graph.TransitiveReduce()
	if err := graph.Validate(); err != nil {
		return err
	}

	rp := newRunningPlan(plan, graph)
	rp.Recovered = true
	if plan.Status == core.PlanWaitingHuman {
		rp.HaltNew = true
	}
	replayEvents := make([]core.Event, 0)

	// Recompute remaining in-degree by replaying completed/skipped tasks.
	for _, taskID := range sortedTaskIDs(rp.TaskByID) {
		task := rp.TaskByID[taskID]
		switch task.Status {
		case core.ItemDone, core.ItemSkipped:
			rp.unlockDownstream(taskID)
		case core.ItemRunning:
			if task.PipelineID == "" {
				continue
			}
			pipeline, getErr := s.store.GetPipeline(task.PipelineID)
			if getErr != nil {
				return fmt.Errorf("recover plan %s task %s pipeline %s: %w", planID, taskID, task.PipelineID, getErr)
			}
			rp.Running[taskID] = task.PipelineID
			evtType, terminal := pipelineRecoveryEvent(pipeline.Status)
			if terminal {
				replayEvents = append(replayEvents, core.Event{
					Type:       evtType,
					PipelineID: task.PipelineID,
					Error:      pipeline.ErrorMessage,
					Timestamp:  time.Now(),
				})
			}
		}
	}

	for _, taskID := range sortedTaskIDs(rp.TaskByID) {
		task := rp.TaskByID[taskID]
		if task.Status != core.ItemPending {
			continue
		}
		if rp.Graph.InDegree[taskID] == 0 {
			task.Status = core.ItemReady
			if err := s.saveTask(task); err != nil {
				return err
			}
			s.publishTaskEvent(core.EventTaskReady, plan, task, "")
		}
	}

	s.mu.Lock()
	s.plans[plan.ID] = rp
	for taskID, pipelineID := range rp.Running {
		s.pipelineIndex[pipelineID] = pipelineRef{planID: plan.ID, taskID: taskID}
		if !s.tryAcquireSlot() {
			s.mu.Unlock()
			return fmt.Errorf("recover plan %s exceeds max concurrency %d", plan.ID, cap(s.sem))
		}
	}
	s.mu.Unlock()

	for i := range replayEvents {
		if err := s.OnEvent(ctx, replayEvents[i]); err != nil {
			return err
		}
	}
	if rp.HaltNew {
		return nil
	}
	return s.dispatchReadyAcrossPlans(ctx)
}

func (s *DepScheduler) dispatchTask(ctx context.Context, planID, taskID string) (bool, error) {
	if s == nil || s.store == nil {
		return false, errors.New("scheduler store is not configured")
	}
	if planID == "" || taskID == "" {
		return false, errors.New("plan id and task id are required")
	}

	s.mu.Lock()
	rp := s.plans[planID]
	if rp == nil {
		s.mu.Unlock()
		return false, fmt.Errorf("plan %s is not running", planID)
	}
	if rp.HaltNew {
		s.mu.Unlock()
		return false, nil
	}
	task := rp.TaskByID[taskID]
	if task == nil {
		s.mu.Unlock()
		return false, fmt.Errorf("task %s not found in plan %s", taskID, planID)
	}
	if task.Status != core.ItemReady {
		s.mu.Unlock()
		return false, nil
	}
	if _, running := rp.Running[taskID]; running {
		s.mu.Unlock()
		return false, nil
	}
	if !s.tryAcquireSlot() {
		s.mu.Unlock()
		return false, nil
	}

	pipeline, err := buildPipelineFromTask(rp.Plan, task)
	if err != nil {
		s.releaseSlot()
		s.mu.Unlock()
		return false, err
	}

	task.Status = core.ItemRunning
	task.PipelineID = pipeline.ID
	rp.Running[taskID] = pipeline.ID
	s.pipelineIndex[pipeline.ID] = pipelineRef{planID: planID, taskID: taskID}
	s.lastPlanID = planID
	s.mu.Unlock()

	if err := s.store.SavePipeline(pipeline); err != nil {
		s.rollbackDispatch(planID, taskID, pipeline.ID)
		return false, err
	}
	if err := s.saveTask(task); err != nil {
		s.rollbackDispatch(planID, taskID, pipeline.ID)
		return false, err
	}
	s.publishTaskEvent(core.EventTaskRunning, rp.Plan, task, "")

	runCtx := context.Background()
	if ctx != nil {
		runCtx = context.WithoutCancel(ctx)
	}
	go func(runCtx context.Context, pipelineID string) {
		if runErr := s.runPipeline(runCtx, pipelineID); runErr != nil {
			_ = s.OnEvent(context.Background(), core.Event{
				Type:       core.EventPipelineFailed,
				PipelineID: pipelineID,
				Error:      runErr.Error(),
				Timestamp:  time.Now(),
			})
		}
	}(runCtx, pipeline.ID)

	return true, nil
}

func (s *DepScheduler) rollbackDispatch(planID, taskID, pipelineID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rp := s.plans[planID]
	if rp != nil {
		if task := rp.TaskByID[taskID]; task != nil && task.Status == core.ItemRunning && task.PipelineID == pipelineID {
			task.Status = core.ItemReady
			task.PipelineID = ""
			_ = s.saveTask(task)
		}
		delete(rp.Running, taskID)
	}
	delete(s.pipelineIndex, pipelineID)
	s.releaseSlot()
}

func (s *DepScheduler) dispatchReadyAcrossPlans(ctx context.Context) error {
	if s == nil {
		return nil
	}
	for {
		s.mu.Lock()
		if cap(s.sem) > 0 && len(s.sem) >= cap(s.sem) {
			s.mu.Unlock()
			return nil
		}
		candidates := s.globalReadyCandidatesLocked()
		s.mu.Unlock()
		if len(candidates) == 0 {
			return nil
		}

		dispatchedAny := false
		for _, candidate := range candidates {
			dispatched, err := s.dispatchTask(ctx, candidate.planID, candidate.taskID)
			if err != nil {
				return err
			}
			if dispatched {
				dispatchedAny = true
			}
		}
		if !dispatchedAny {
			return nil
		}
	}
}

func (s *DepScheduler) globalReadyCandidatesLocked() []readyDispatch {
	planIDs := make([]string, 0, len(s.plans))
	readyByPlan := make(map[string][]string, len(s.plans))
	maxReady := 0

	for planID, rp := range s.plans {
		if rp == nil || rp.HaltNew {
			continue
		}
		ready := rp.readyToDispatchIDs()
		if len(ready) == 0 {
			continue
		}
		planIDs = append(planIDs, planID)
		readyByPlan[planID] = ready
		if len(ready) > maxReady {
			maxReady = len(ready)
		}
	}
	if len(planIDs) == 0 {
		return nil
	}

	sort.Strings(planIDs)
	start := 0
	if s.lastPlanID != "" {
		idx := sort.SearchStrings(planIDs, s.lastPlanID)
		if idx < len(planIDs) && planIDs[idx] == s.lastPlanID {
			start = (idx + 1) % len(planIDs)
		} else if idx < len(planIDs) {
			start = idx
		}
	}
	orderedPlanIDs := append([]string{}, planIDs[start:]...)
	orderedPlanIDs = append(orderedPlanIDs, planIDs[:start]...)

	candidates := make([]readyDispatch, 0, len(planIDs))
	for i := 0; i < maxReady; i++ {
		for _, planID := range orderedPlanIDs {
			ready := readyByPlan[planID]
			if i >= len(ready) {
				continue
			}
			candidates = append(candidates, readyDispatch{planID: planID, taskID: ready[i]})
		}
	}
	return candidates
}

func (s *DepScheduler) handlePipelineEventLocked(evt core.Event) error {
	ref, ok := s.pipelineIndex[evt.PipelineID]
	if !ok {
		return nil
	}
	rp := s.plans[ref.planID]
	if rp == nil {
		delete(s.pipelineIndex, evt.PipelineID)
		return nil
	}
	task := rp.TaskByID[ref.taskID]
	if task == nil {
		delete(s.pipelineIndex, evt.PipelineID)
		delete(rp.Running, ref.taskID)
		s.releaseSlot()
		return nil
	}
	s.publishPlanEvent(core.EventSecretaryThinking, rp.Plan, map[string]string{
		"phase":       "pipeline_event",
		"pipeline_id": evt.PipelineID,
	})

	snapshot := capturePlanState(rp)

	switch evt.Type {
	case core.EventPipelineDone:
		task.Status = core.ItemDone
		if err := s.saveTask(task); err != nil {
			snapshot.restore(rp)
			return err
		}
		s.publishTaskEvent(core.EventTaskDone, rp.Plan, task, "")
		rp.unlockDownstream(task.ID)
	case core.EventPipelineFailed:
		task.Status = core.ItemFailed
		if err := s.saveTask(task); err != nil {
			snapshot.restore(rp)
			return err
		}
		s.publishTaskEvent(core.EventTaskFailed, rp.Plan, task, evt.Error)
		switch rp.Plan.FailPolicy {
		case core.FailSkip:
			if err := s.applySkipPolicyLocked(rp, task.ID); err != nil {
				snapshot.restore(rp)
				return err
			}
		case core.FailHuman:
			rp.HaltNew = true
			rp.Plan.WaitReason = core.WaitFeedbackReq
		default:
			if err := s.applyBlockPolicyLocked(rp, task.ID); err != nil {
				snapshot.restore(rp)
				return err
			}
		}
	default:
		return nil
	}
	if err := s.markReadyByInDegreeLocked(rp); err != nil {
		snapshot.restore(rp)
		return err
	}

	if err := s.refreshPlanStatusLocked(rp); err != nil {
		snapshot.restore(rp)
		return err
	}

	if _, running := rp.Running[ref.taskID]; running {
		delete(rp.Running, ref.taskID)
		s.releaseSlot()
	}
	delete(s.pipelineIndex, evt.PipelineID)
	return nil
}

func (s *DepScheduler) applyBlockPolicyLocked(rp *runningPlan, failedTaskID string) error {
	queue := []string{failedTaskID}
	seen := map[string]struct{}{failedTaskID: {}}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, downID := range rp.Graph.Downstream[current] {
			if _, ok := seen[downID]; !ok {
				seen[downID] = struct{}{}
				queue = append(queue, downID)
			}
			downTask := rp.TaskByID[downID]
			if downTask == nil {
				continue
			}
			if isTaskTerminal(downTask.Status) || downTask.Status == core.ItemRunning {
				continue
			}
			downTask.Status = core.ItemBlockedByFailure
			if err := s.saveTask(downTask); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *DepScheduler) applySkipPolicyLocked(rp *runningPlan, failedTaskID string) error {
	queue := []string{failedTaskID}
	seen := map[string]struct{}{failedTaskID: {}}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for _, downID := range rp.Graph.Downstream[current] {
			downTask := rp.TaskByID[downID]
			if downTask == nil {
				continue
			}
			if isTaskTerminal(downTask.Status) || downTask.Status == core.ItemRunning {
				continue
			}

			// Skip strategy defaults to hard dependencies.
			// Only weak failed edge + other unfailed upstream can degrade and continue.
			if !isWeakDependencyEdge(downTask, current) || !rp.hasOtherUnfailedParent(downID, current) {
				downTask.Status = core.ItemSkipped
				if err := s.saveTask(downTask); err != nil {
					return err
				}
				if _, ok := seen[downID]; !ok {
					seen[downID] = struct{}{}
					queue = append(queue, downID)
				}
				continue
			}

			rp.decrementInDegree(downID)
			if rp.Graph.InDegree[downID] == 0 && downTask.Status == core.ItemPending {
				downTask.Status = core.ItemReady
				if err := s.saveTask(downTask); err != nil {
					return err
				}
				s.publishTaskEvent(core.EventTaskReady, rp.Plan, downTask, "")
			}
		}
	}
	return nil
}

func (s *DepScheduler) refreshPlanStatusLocked(rp *runningPlan) error {
	if rp == nil || rp.Plan == nil {
		return nil
	}
	prevStatus := rp.Plan.Status
	prevWaitReason := rp.Plan.WaitReason

	stats := collectPlanTaskStats(rp.TaskByID)
	hasPending := stats.pending > 0 || stats.ready > 0 || stats.running > 0
	hasRunning := stats.running > 0
	hasDone := stats.done > 0
	hasFailed := stats.failed > 0
	hasSkipped := stats.skipped > 0
	hasBlocked := stats.blocked > 0

	switch {
	case rp.HaltNew:
		rp.Plan.Status = core.PlanWaitingHuman
		if rp.Plan.WaitReason == core.WaitNone {
			rp.Plan.WaitReason = core.WaitFeedbackReq
		}
	case hasPending || hasRunning:
		rp.Plan.Status = core.PlanExecuting
		rp.Plan.WaitReason = core.WaitNone
	case hasFailed || hasSkipped || hasBlocked:
		if hasDone {
			rp.Plan.Status = core.PlanPartial
		} else {
			rp.Plan.Status = core.PlanFailed
		}
		rp.Plan.WaitReason = core.WaitNone
	default:
		rp.Plan.Status = core.PlanDone
		rp.Plan.WaitReason = core.WaitNone
	}
	if err := s.savePlan(rp.Plan); err != nil {
		return err
	}
	if rp.Plan.Status == prevStatus && rp.Plan.WaitReason == prevWaitReason {
		return nil
	}

	switch rp.Plan.Status {
	case core.PlanWaitingHuman:
		s.publishPlanEvent(core.EventPlanWaitingHuman, rp.Plan, map[string]string{
			"wait_reason": string(rp.Plan.WaitReason),
		})
	case core.PlanDone:
		s.publishPlanEvent(core.EventPlanDone, rp.Plan, stats.eventData())
	case core.PlanFailed:
		data := stats.eventData()
		data["reason"] = deriveFailedPlanReason(stats)
		s.publishPlanEvent(core.EventPlanFailed, rp.Plan, data)
	case core.PlanPartial:
		data := stats.eventData()
		data["reason"] = "partial_failures"
		s.publishPlanEvent(core.EventPlanPartiallyDone, rp.Plan, data)
	}
	return nil
}

func (s *DepScheduler) markReadyByInDegreeLocked(rp *runningPlan) error {
	if rp == nil {
		return nil
	}
	for _, taskID := range sortedTaskIDs(rp.TaskByID) {
		task := rp.TaskByID[taskID]
		if task == nil || task.Status != core.ItemPending {
			continue
		}
		if rp.Graph.InDegree[taskID] != 0 {
			continue
		}
		task.Status = core.ItemReady
		if err := s.saveTask(task); err != nil {
			return err
		}
		s.publishTaskEvent(core.EventTaskReady, rp.Plan, task, "")
	}
	return nil
}

func (s *DepScheduler) loadPlanTasks(plan *core.TaskPlan) ([]core.TaskItem, error) {
	if len(plan.Tasks) > 0 {
		return plan.Tasks, nil
	}
	if s.store == nil {
		return nil, errors.New("scheduler store is not configured")
	}
	tasks, err := s.store.GetTaskItemsByPlan(plan.ID)
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

func (s *DepScheduler) savePlan(plan *core.TaskPlan) error {
	if plan == nil {
		return nil
	}
	plan.UpdatedAt = time.Now()
	return s.store.SaveTaskPlan(plan)
}

func (s *DepScheduler) saveTask(task *core.TaskItem) error {
	if task == nil {
		return nil
	}
	task.UpdatedAt = time.Now()
	if err := s.store.SaveTaskItem(task); err != nil {
		return err
	}

	if s.tracker == nil {
		return nil
	}
	if task.ExternalID == "" {
		externalID, err := s.tracker.CreateTask(context.Background(), task)
		if err == nil && externalID != "" {
			task.ExternalID = externalID
			task.UpdatedAt = time.Now()
			if saveErr := s.store.SaveTaskItem(task); saveErr != nil {
				return saveErr
			}
		}
	}
	if task.ExternalID != "" {
		_ = s.tracker.UpdateStatus(context.Background(), task.ExternalID, task.Status)
	}
	return nil
}

func (s *DepScheduler) publishEvent(evt core.Event) {
	if s == nil || s.pub == nil {
		return
	}
	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now()
	}
	s.pub.Publish(evt)
}

func (s *DepScheduler) publishPlanEvent(eventType core.EventType, plan *core.TaskPlan, data map[string]string) {
	if plan == nil {
		return
	}
	evt := core.Event{
		Type:      eventType,
		ProjectID: plan.ProjectID,
		PlanID:    plan.ID,
		Timestamp: time.Now(),
	}
	if len(data) > 0 {
		evt.Data = data
	}
	s.publishEvent(evt)
}

func (s *DepScheduler) publishTaskEvent(
	eventType core.EventType,
	plan *core.TaskPlan,
	task *core.TaskItem,
	eventErr string,
) {
	if plan == nil || task == nil {
		return
	}

	data := map[string]string{
		"task_id":     task.ID,
		"task_status": string(task.Status),
	}
	if eventErr != "" {
		data["error"] = eventErr
	}

	s.publishEvent(core.Event{
		Type:       eventType,
		PipelineID: task.PipelineID,
		ProjectID:  plan.ProjectID,
		PlanID:     plan.ID,
		Data:       data,
		Error:      eventErr,
		Timestamp:  time.Now(),
	})
}

func (s *DepScheduler) tryAcquireSlot() bool {
	select {
	case s.sem <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *DepScheduler) releaseSlot() {
	select {
	case <-s.sem:
	default:
	}
}

type planStateSnapshot struct {
	inDegree       map[string]int
	taskStatus     map[string]core.TaskItemStatus
	planStatus     core.TaskPlanStatus
	planWaitReason core.WaitReason
	haltNew        bool
}

func capturePlanState(rp *runningPlan) planStateSnapshot {
	snapshot := planStateSnapshot{
		inDegree:   copyInDegree(rp.Graph.InDegree),
		taskStatus: make(map[string]core.TaskItemStatus, len(rp.TaskByID)),
		haltNew:    rp.HaltNew,
	}
	if rp.Plan != nil {
		snapshot.planStatus = rp.Plan.Status
		snapshot.planWaitReason = rp.Plan.WaitReason
	}
	for taskID, task := range rp.TaskByID {
		if task == nil {
			continue
		}
		snapshot.taskStatus[taskID] = task.Status
	}
	return snapshot
}

func (snapshot planStateSnapshot) restore(rp *runningPlan) {
	if rp == nil {
		return
	}
	rp.Graph.InDegree = copyInDegree(snapshot.inDegree)
	for taskID, status := range snapshot.taskStatus {
		task := rp.TaskByID[taskID]
		if task == nil {
			continue
		}
		task.Status = status
	}
	rp.HaltNew = snapshot.haltNew
	if rp.Plan != nil {
		rp.Plan.Status = snapshot.planStatus
		rp.Plan.WaitReason = snapshot.planWaitReason
	}
}

func pipelineRecoveryEvent(status core.PipelineStatus) (core.EventType, bool) {
	switch status {
	case core.StatusDone:
		return core.EventPipelineDone, true
	case core.StatusFailed, core.StatusAborted:
		return core.EventPipelineFailed, true
	default:
		return "", false
	}
}

func isWeakDependencyEdge(task *core.TaskItem, upstreamID string) bool {
	if task == nil || upstreamID == "" {
		return false
	}
	weakDepToken := "weak_dep:" + strings.ToLower(upstreamID)
	weakToken := "weak:" + strings.ToLower(upstreamID)
	for _, label := range task.Labels {
		normalized := strings.ToLower(strings.TrimSpace(label))
		if normalized == weakDepToken || normalized == weakToken {
			return true
		}
	}
	return false
}

func (rp *runningPlan) unlockDownstream(taskID string) {
	for _, downID := range rp.Graph.Downstream[taskID] {
		rp.decrementInDegree(downID)
	}
}

func (rp *runningPlan) decrementInDegree(taskID string) {
	if rp.Graph.InDegree[taskID] > 0 {
		rp.Graph.InDegree[taskID]--
	}
}

func (rp *runningPlan) hasOtherUnfailedParent(taskID, failedParentID string) bool {
	for _, parentID := range rp.Parents[taskID] {
		if parentID == failedParentID {
			continue
		}
		parent := rp.TaskByID[parentID]
		if parent == nil {
			continue
		}
		if isParentStillViable(parent.Status) {
			return true
		}
	}
	return false
}

func isParentStillViable(status core.TaskItemStatus) bool {
	switch status {
	case core.ItemPending, core.ItemReady, core.ItemRunning, core.ItemDone:
		return true
	default:
		return false
	}
}

func (rp *runningPlan) readyToDispatchIDs() []string {
	ready := make([]string, 0, len(rp.TaskByID))
	for _, taskID := range sortedTaskIDs(rp.TaskByID) {
		task := rp.TaskByID[taskID]
		if task == nil {
			continue
		}
		if task.Status != core.ItemReady {
			continue
		}
		if _, running := rp.Running[taskID]; running {
			continue
		}
		ready = append(ready, taskID)
	}
	return ready
}

func buildPipelineFromTask(plan *core.TaskPlan, task *core.TaskItem) (*core.Pipeline, error) {
	if plan == nil || task == nil {
		return nil, errors.New("plan/task cannot be nil")
	}

	template := task.Template
	if template == "" {
		template = "standard"
	}
	stages, err := buildSchedulerStages(template)
	if err != nil {
		return nil, err
	}

	name := task.Title
	if name == "" {
		name = task.ID
	}

	now := time.Now()
	return &core.Pipeline{
		ID:              engine.NewPipelineID(),
		ProjectID:       plan.ProjectID,
		Name:            name,
		Description:     task.Description,
		Template:        template,
		Status:          core.StatusCreated,
		Stages:          stages,
		Artifacts:       map[string]string{},
		Config:          map[string]any{},
		MaxTotalRetries: 5,
		QueuedAt:        now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}, nil
}

func buildSchedulerStages(template string) ([]core.StageConfig, error) {
	stageIDs, ok := engine.Templates[template]
	if !ok {
		return nil, fmt.Errorf("unknown template: %s", template)
	}

	stages := make([]core.StageConfig, len(stageIDs))
	for i, stageID := range stageIDs {
		stages[i] = schedulerDefaultStageConfig(stageID)
	}
	return stages, nil
}

func schedulerDefaultStageConfig(id core.StageID) core.StageConfig {
	cfg := core.StageConfig{
		Name:           id,
		PromptTemplate: string(id),
		Timeout:        30 * time.Minute,
		MaxRetries:     1,
		OnFailure:      core.OnFailureHuman,
	}

	switch id {
	case core.StageRequirements, core.StageCodeReview:
		cfg.Agent = "claude"
	case core.StageImplement, core.StageFixup:
		cfg.Agent = "codex"
	case core.StageE2ETest:
		cfg.Agent = "codex"
		cfg.Timeout = 15 * time.Minute
	case core.StageWorktreeSetup, core.StageMerge, core.StageCleanup:
		cfg.Timeout = 2 * time.Minute
	}
	return cfg
}

func isTaskTerminal(status core.TaskItemStatus) bool {
	switch status {
	case core.ItemDone, core.ItemFailed, core.ItemSkipped, core.ItemBlockedByFailure:
		return true
	default:
		return false
	}
}

func validateStartPlanState(plan *core.TaskPlan) error {
	if plan == nil {
		return errors.New("plan is nil")
	}

	switch plan.Status {
	case core.PlanApproved:
		return nil
	case core.PlanWaitingHuman:
		if plan.WaitReason != core.WaitFinalApproval {
			return fmt.Errorf(
				"start plan requires waiting_human/final_approval, got %s/%s",
				plan.Status,
				plan.WaitReason,
			)
		}
		return nil
	default:
		return fmt.Errorf(
			"start plan requires approved or waiting_human/final_approval, got %s/%s",
			plan.Status,
			plan.WaitReason,
		)
	}
}

type planTaskStats struct {
	total   int
	pending int
	ready   int
	running int
	done    int
	failed  int
	skipped int
	blocked int
}

func collectPlanTaskStats(tasks map[string]*core.TaskItem) planTaskStats {
	stats := planTaskStats{}
	for _, task := range tasks {
		if task == nil {
			continue
		}
		stats.total++
		switch task.Status {
		case core.ItemPending:
			stats.pending++
		case core.ItemReady:
			stats.ready++
		case core.ItemRunning:
			stats.running++
		case core.ItemDone:
			stats.done++
		case core.ItemFailed:
			stats.failed++
		case core.ItemSkipped:
			stats.skipped++
		case core.ItemBlockedByFailure:
			stats.blocked++
		}
	}
	return stats
}

func (s planTaskStats) eventData() map[string]string {
	return map[string]string{
		"stats_total":   strconv.Itoa(s.total),
		"stats_pending": strconv.Itoa(s.pending),
		"stats_ready":   strconv.Itoa(s.ready),
		"stats_running": strconv.Itoa(s.running),
		"stats_done":    strconv.Itoa(s.done),
		"stats_failed":  strconv.Itoa(s.failed),
		"stats_skipped": strconv.Itoa(s.skipped),
		"stats_blocked": strconv.Itoa(s.blocked),
	}
}

func deriveFailedPlanReason(stats planTaskStats) string {
	switch {
	case stats.failed > 0 && stats.done == 0 && stats.skipped == 0 && stats.blocked == 0:
		return "all_tasks_failed"
	case stats.blocked > 0 && stats.failed == 0:
		return "blocked_by_failure"
	case stats.skipped > 0 && stats.failed == 0:
		return "all_tasks_skipped"
	default:
		return "task_failures"
	}
}

func sortedTaskIDs(taskByID map[string]*core.TaskItem) []string {
	ids := make([]string, 0, len(taskByID))
	for taskID := range taskByID {
		ids = append(ids, taskID)
	}
	sort.Strings(ids)
	return ids
}
