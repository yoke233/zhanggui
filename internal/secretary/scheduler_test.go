package secretary

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/user/ai-workflow/internal/core"
	storesqlite "github.com/user/ai-workflow/internal/plugins/store-sqlite"
)

func TestScheduler_StartPlanAndProgression(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-scheduler-normal")
	plan := mustCreateTaskPlanWithItems(t, store, project.ID, "plan-normal", core.FailBlock, []core.TaskItem{
		newTaskItem("task-a", "A", nil),
		newTaskItem("task-b", "B", []string{"task-a"}),
	})

	runner := &schedulerRunner{}
	s := NewDepScheduler(store, nil, runner.Run, nil, 2)

	if err := s.StartPlan(context.Background(), plan); err != nil {
		t.Fatalf("StartPlan() error = %v", err)
	}

	taskA := waitTaskStatus(t, store, "task-a", core.ItemRunning, 2*time.Second)
	if taskA.PipelineID == "" {
		t.Fatalf("expected task-a pipeline id assigned")
	}

	if err := s.OnEvent(context.Background(), core.Event{Type: core.EventPipelineDone, PipelineID: taskA.PipelineID, Timestamp: time.Now()}); err != nil {
		t.Fatalf("OnEvent(done A) error = %v", err)
	}

	taskB := waitTaskStatus(t, store, "task-b", core.ItemRunning, 2*time.Second)
	if taskB.PipelineID == "" {
		t.Fatalf("expected task-b pipeline id assigned")
	}

	if err := s.OnEvent(context.Background(), core.Event{Type: core.EventPipelineDone, PipelineID: taskB.PipelineID, Timestamp: time.Now()}); err != nil {
		t.Fatalf("OnEvent(done B) error = %v", err)
	}

	waitPlanStatus(t, store, plan.ID, core.PlanDone, 2*time.Second)
	waitTaskStatus(t, store, "task-a", core.ItemDone, 2*time.Second)
	waitTaskStatus(t, store, "task-b", core.ItemDone, 2*time.Second)

	if got := runner.CallCount(); got != 2 {
		t.Fatalf("runner calls = %d, want 2", got)
	}
}

func TestDepScheduler_StartPlan_IdempotentForManagedPlan(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-scheduler-idempotent")
	plan := mustCreateTaskPlanWithItems(t, store, project.ID, "plan-idempotent", core.FailBlock, []core.TaskItem{
		newTaskItem("task-a", "A", nil),
	})

	runner := &schedulerRunner{}
	s := NewDepScheduler(store, nil, runner.Run, nil, 1)

	if err := s.StartPlan(context.Background(), plan); err != nil {
		t.Fatalf("StartPlan(first) error = %v", err)
	}
	taskA := waitTaskStatus(t, store, "task-a", core.ItemRunning, 2*time.Second)
	firstPipelineID := taskA.PipelineID
	if firstPipelineID == "" {
		t.Fatalf("expected task-a pipeline id assigned")
	}

	if err := s.StartPlan(context.Background(), plan); err != nil {
		t.Fatalf("StartPlan(second) error = %v", err)
	}

	taskAAfter := waitTaskStatus(t, store, "task-a", core.ItemRunning, 2*time.Second)
	if taskAAfter.PipelineID != firstPipelineID {
		t.Fatalf("pipeline id changed on idempotent start: got %q want %q", taskAAfter.PipelineID, firstPipelineID)
	}
	if got := runner.CallCount(); got != 1 {
		t.Fatalf("runner calls = %d, want 1", got)
	}
}

func TestScheduler_FailPolicyBlock(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-scheduler-block")
	plan := mustCreateTaskPlanWithItems(t, store, project.ID, "plan-block", core.FailBlock, []core.TaskItem{
		newTaskItem("task-a", "A", nil),
		newTaskItem("task-b", "B", []string{"task-a"}),
		newTaskItem("task-c", "C", []string{"task-b"}),
	})

	s := NewDepScheduler(store, nil, (&schedulerRunner{}).Run, nil, 1)
	if err := s.StartPlan(context.Background(), plan); err != nil {
		t.Fatalf("StartPlan() error = %v", err)
	}

	taskA := waitTaskStatus(t, store, "task-a", core.ItemRunning, 2*time.Second)
	if err := s.OnEvent(context.Background(), core.Event{Type: core.EventPipelineFailed, PipelineID: taskA.PipelineID, Timestamp: time.Now(), Error: "boom"}); err != nil {
		t.Fatalf("OnEvent(failed A) error = %v", err)
	}

	waitTaskStatus(t, store, "task-a", core.ItemFailed, 2*time.Second)
	taskB := waitTaskStatus(t, store, "task-b", core.ItemBlockedByFailure, 2*time.Second)
	taskC := waitTaskStatus(t, store, "task-c", core.ItemBlockedByFailure, 2*time.Second)

	if taskB.PipelineID != "" || taskC.PipelineID != "" {
		t.Fatalf("blocked tasks should not be dispatched, got task-b=%q task-c=%q", taskB.PipelineID, taskC.PipelineID)
	}

	waitPlanStatus(t, store, plan.ID, core.PlanFailed, 2*time.Second)
}

func TestScheduler_FailPolicySkip(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-scheduler-skip")
	taskC := newTaskItem("task-c", "C", []string{"task-a", "task-x"})
	taskC.Labels = []string{"weak_dep:task-a"}
	plan := mustCreateTaskPlanWithItems(t, store, project.ID, "plan-skip", core.FailSkip, []core.TaskItem{
		newTaskItem("task-a", "A", nil),
		newTaskItem("task-x", "X", nil),
		newTaskItem("task-b", "B", []string{"task-a"}),
		taskC,
	})

	s := NewDepScheduler(store, nil, (&schedulerRunner{}).Run, nil, 3)
	if err := s.StartPlan(context.Background(), plan); err != nil {
		t.Fatalf("StartPlan() error = %v", err)
	}

	taskA := waitTaskStatus(t, store, "task-a", core.ItemRunning, 2*time.Second)
	taskX := waitTaskStatus(t, store, "task-x", core.ItemRunning, 2*time.Second)

	if err := s.OnEvent(context.Background(), core.Event{Type: core.EventPipelineDone, PipelineID: taskX.PipelineID, Timestamp: time.Now()}); err != nil {
		t.Fatalf("OnEvent(done X) error = %v", err)
	}
	if err := s.OnEvent(context.Background(), core.Event{Type: core.EventPipelineFailed, PipelineID: taskA.PipelineID, Timestamp: time.Now(), Error: "boom"}); err != nil {
		t.Fatalf("OnEvent(failed A) error = %v", err)
	}

	waitTaskStatus(t, store, "task-a", core.ItemFailed, 2*time.Second)
	waitTaskStatus(t, store, "task-b", core.ItemSkipped, 2*time.Second)
	taskCRunning := waitTaskStatus(t, store, "task-c", core.ItemRunning, 2*time.Second)
	if taskCRunning.PipelineID == "" {
		t.Fatalf("task-c should be dispatched under skip policy")
	}

	if err := s.OnEvent(context.Background(), core.Event{Type: core.EventPipelineDone, PipelineID: taskCRunning.PipelineID, Timestamp: time.Now()}); err != nil {
		t.Fatalf("OnEvent(done C) error = %v", err)
	}

	waitPlanStatus(t, store, plan.ID, core.PlanPartial, 2*time.Second)
}

func TestDepScheduler_FailPolicySkip_HardByDefault(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-scheduler-skip-hard-default")
	plan := mustCreateTaskPlanWithItems(t, store, project.ID, "plan-skip-hard-default", core.FailSkip, []core.TaskItem{
		newTaskItem("task-a", "A", nil),
		newTaskItem("task-x", "X", nil),
		newTaskItem("task-c", "C", []string{"task-a", "task-x"}),
	})

	s := NewDepScheduler(store, nil, (&schedulerRunner{}).Run, nil, 3)
	if err := s.StartPlan(context.Background(), plan); err != nil {
		t.Fatalf("StartPlan() error = %v", err)
	}

	taskA := waitTaskStatus(t, store, "task-a", core.ItemRunning, 2*time.Second)
	taskX := waitTaskStatus(t, store, "task-x", core.ItemRunning, 2*time.Second)

	if err := s.OnEvent(context.Background(), core.Event{Type: core.EventPipelineDone, PipelineID: taskX.PipelineID, Timestamp: time.Now()}); err != nil {
		t.Fatalf("OnEvent(done X) error = %v", err)
	}
	if err := s.OnEvent(context.Background(), core.Event{Type: core.EventPipelineFailed, PipelineID: taskA.PipelineID, Timestamp: time.Now(), Error: "boom"}); err != nil {
		t.Fatalf("OnEvent(failed A) error = %v", err)
	}

	waitTaskStatus(t, store, "task-a", core.ItemFailed, 2*time.Second)
	taskC := waitTaskStatus(t, store, "task-c", core.ItemSkipped, 2*time.Second)
	if taskC.PipelineID != "" {
		t.Fatalf("task-c should not be dispatched under hard-by-default skip policy, got pipeline=%q", taskC.PipelineID)
	}
}

func TestDepScheduler_FailPolicySkip_WeakEdgeRequiresOtherUnfailedParent(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-scheduler-skip-weak-guard")
	taskB := newTaskItem("task-b", "B", []string{"task-a"})
	taskB.Labels = []string{"weak_dep:task-a"}
	plan := mustCreateTaskPlanWithItems(t, store, project.ID, "plan-skip-weak-guard", core.FailSkip, []core.TaskItem{
		newTaskItem("task-a", "A", nil),
		taskB,
	})

	s := NewDepScheduler(store, nil, (&schedulerRunner{}).Run, nil, 2)
	if err := s.StartPlan(context.Background(), plan); err != nil {
		t.Fatalf("StartPlan() error = %v", err)
	}

	taskA := waitTaskStatus(t, store, "task-a", core.ItemRunning, 2*time.Second)
	if err := s.OnEvent(context.Background(), core.Event{Type: core.EventPipelineFailed, PipelineID: taskA.PipelineID, Timestamp: time.Now(), Error: "boom"}); err != nil {
		t.Fatalf("OnEvent(failed A) error = %v", err)
	}

	waitTaskStatus(t, store, "task-a", core.ItemFailed, 2*time.Second)
	taskBSkipped := waitTaskStatus(t, store, "task-b", core.ItemSkipped, 2*time.Second)
	if taskBSkipped.PipelineID != "" {
		t.Fatalf("task-b should be skipped when weak edge has no other unfailed upstream, got pipeline=%q", taskBSkipped.PipelineID)
	}
}

func TestScheduler_FailPolicyHuman(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-scheduler-human")
	plan := mustCreateTaskPlanWithItems(t, store, project.ID, "plan-human", core.FailHuman, []core.TaskItem{
		newTaskItem("task-a", "A", nil),
		newTaskItem("task-b", "B", []string{"task-a"}),
	})

	s := NewDepScheduler(store, nil, (&schedulerRunner{}).Run, nil, 2)
	if err := s.StartPlan(context.Background(), plan); err != nil {
		t.Fatalf("StartPlan() error = %v", err)
	}

	taskA := waitTaskStatus(t, store, "task-a", core.ItemRunning, 2*time.Second)
	if err := s.OnEvent(context.Background(), core.Event{Type: core.EventPipelineFailed, PipelineID: taskA.PipelineID, Timestamp: time.Now(), Error: "need human"}); err != nil {
		t.Fatalf("OnEvent(failed A) error = %v", err)
	}

	waitTaskStatus(t, store, "task-a", core.ItemFailed, 2*time.Second)
	taskB := waitTaskStatus(t, store, "task-b", core.ItemPending, 2*time.Second)
	if taskB.PipelineID != "" {
		t.Fatalf("task-b should not be dispatched under human policy, got pipeline=%q", taskB.PipelineID)
	}

	waitPlanStatus(t, store, plan.ID, core.PlanWaitingHuman, 2*time.Second)
	gotPlan, err := store.GetTaskPlan(plan.ID)
	if err != nil {
		t.Fatalf("GetTaskPlan() error = %v", err)
	}
	if gotPlan.WaitReason != core.WaitFeedbackReq {
		t.Fatalf("WaitReason = %q, want %q", gotPlan.WaitReason, core.WaitFeedbackReq)
	}
}

func TestScheduler_RecoverExecutingPlans_DispatchesReadyTasks(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-scheduler-recovery")
	plan := mustCreateTaskPlanWithItems(t, store, project.ID, "plan-recovery", core.FailBlock, []core.TaskItem{
		{
			ID:          "task-a",
			Title:       "A",
			Description: "A",
			Status:      core.ItemDone,
		},
		{
			ID:          "task-b",
			Title:       "B",
			Description: "B",
			DependsOn:   []string{"task-a"},
			Status:      core.ItemPending,
		},
	})
	plan.Status = core.PlanExecuting
	if err := store.SaveTaskPlan(plan); err != nil {
		t.Fatalf("SaveTaskPlan(executing) error = %v", err)
	}

	runner := &schedulerRunner{}
	s := NewDepScheduler(store, nil, runner.Run, nil, 2)
	if err := s.RecoverExecutingPlans(context.Background()); err != nil {
		t.Fatalf("RecoverExecutingPlans() error = %v", err)
	}

	taskB := waitTaskStatus(t, store, "task-b", core.ItemRunning, 2*time.Second)
	if taskB.PipelineID == "" {
		t.Fatalf("expected task-b dispatched after recovery")
	}
	if got := runner.CallCount(); got != 1 {
		t.Fatalf("runner calls = %d, want 1", got)
	}
}

func TestDepScheduler_RecoverExecutingPlans_ReplaysPipelineDoneFromStore(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-scheduler-recover-done")
	plan := mustCreateTaskPlanWithItems(t, store, project.ID, "plan-recover-done", core.FailBlock, []core.TaskItem{
		{
			ID:          "task-a",
			Title:       "A",
			Description: "A",
			Status:      core.ItemRunning,
		},
		{
			ID:          "task-b",
			Title:       "B",
			Description: "B",
			DependsOn:   []string{"task-a"},
			Status:      core.ItemPending,
		},
	})
	plan.Status = core.PlanExecuting
	if err := store.SaveTaskPlan(plan); err != nil {
		t.Fatalf("SaveTaskPlan(executing) error = %v", err)
	}
	if err := store.SavePipeline(&core.Pipeline{
		ID:        "pipeline-recover-done",
		ProjectID: project.ID,
		Name:      "pipeline-recover-done",
		Status:    core.StatusDone,
	}); err != nil {
		t.Fatalf("SavePipeline(done) error = %v", err)
	}
	taskA, err := store.GetTaskItem("task-a")
	if err != nil {
		t.Fatalf("GetTaskItem(task-a) error = %v", err)
	}
	taskA.PipelineID = "pipeline-recover-done"
	taskA.Status = core.ItemRunning
	if err := store.SaveTaskItem(taskA); err != nil {
		t.Fatalf("SaveTaskItem(task-a) error = %v", err)
	}

	runner := &schedulerRunner{}
	s := NewDepScheduler(store, nil, runner.Run, nil, 2)
	if err := s.RecoverExecutingPlans(context.Background()); err != nil {
		t.Fatalf("RecoverExecutingPlans() error = %v", err)
	}

	waitTaskStatus(t, store, "task-a", core.ItemDone, 2*time.Second)
	taskB := waitTaskStatus(t, store, "task-b", core.ItemRunning, 2*time.Second)
	if taskB.PipelineID == "" {
		t.Fatalf("expected task-b dispatched after replaying pipeline done")
	}
}

func TestDepScheduler_RecoverExecutingPlans_ReplaysPipelineFailedFromStore(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-scheduler-recover-failed")
	plan := mustCreateTaskPlanWithItems(t, store, project.ID, "plan-recover-failed", core.FailBlock, []core.TaskItem{
		{
			ID:          "task-a",
			Title:       "A",
			Description: "A",
			Status:      core.ItemRunning,
		},
		{
			ID:          "task-b",
			Title:       "B",
			Description: "B",
			DependsOn:   []string{"task-a"},
			Status:      core.ItemPending,
		},
	})
	plan.Status = core.PlanExecuting
	if err := store.SaveTaskPlan(plan); err != nil {
		t.Fatalf("SaveTaskPlan(executing) error = %v", err)
	}
	if err := store.SavePipeline(&core.Pipeline{
		ID:        "pipeline-recover-failed",
		ProjectID: project.ID,
		Name:      "pipeline-recover-failed",
		Status:    core.StatusFailed,
	}); err != nil {
		t.Fatalf("SavePipeline(failed) error = %v", err)
	}
	taskA, err := store.GetTaskItem("task-a")
	if err != nil {
		t.Fatalf("GetTaskItem(task-a) error = %v", err)
	}
	taskA.PipelineID = "pipeline-recover-failed"
	taskA.Status = core.ItemRunning
	if err := store.SaveTaskItem(taskA); err != nil {
		t.Fatalf("SaveTaskItem(task-a) error = %v", err)
	}

	s := NewDepScheduler(store, nil, (&schedulerRunner{}).Run, nil, 2)
	if err := s.RecoverExecutingPlans(context.Background()); err != nil {
		t.Fatalf("RecoverExecutingPlans() error = %v", err)
	}

	waitTaskStatus(t, store, "task-a", core.ItemFailed, 2*time.Second)
	waitTaskStatus(t, store, "task-b", core.ItemBlockedByFailure, 2*time.Second)
	waitPlanStatus(t, store, plan.ID, core.PlanFailed, 2*time.Second)
}

func TestDepScheduler_GlobalReadyDispatch_AvoidsCrossPlanStarvation(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-scheduler-cross-plan")
	planA := mustCreateTaskPlanWithItems(t, store, project.ID, "plan-a", core.FailBlock, []core.TaskItem{
		newTaskItem("task-a-1", "A1", nil),
		newTaskItem("task-a-2", "A2", []string{"task-a-1"}),
	})
	planB := mustCreateTaskPlanWithItems(t, store, project.ID, "plan-b", core.FailBlock, []core.TaskItem{
		newTaskItem("task-b-1", "B1", nil),
	})

	runner := &schedulerRunner{}
	s := NewDepScheduler(store, nil, runner.Run, nil, 1)
	if err := s.StartPlan(context.Background(), planA); err != nil {
		t.Fatalf("StartPlan(planA) error = %v", err)
	}
	if err := s.StartPlan(context.Background(), planB); err != nil {
		t.Fatalf("StartPlan(planB) error = %v", err)
	}

	taskA1 := waitTaskStatus(t, store, "task-a-1", core.ItemRunning, 2*time.Second)
	waitTaskStatus(t, store, "task-b-1", core.ItemReady, 2*time.Second)

	if err := s.OnEvent(context.Background(), core.Event{Type: core.EventPipelineDone, PipelineID: taskA1.PipelineID, Timestamp: time.Now()}); err != nil {
		t.Fatalf("OnEvent(done A1) error = %v", err)
	}

	taskB1 := waitTaskStatus(t, store, "task-b-1", core.ItemRunning, 2*time.Second)
	if taskB1.PipelineID == "" {
		t.Fatalf("expected task-b-1 dispatched after slot release")
	}
	taskA2 := waitTaskStatus(t, store, "task-a-2", core.ItemReady, 2*time.Second)
	if taskA2.PipelineID != "" {
		t.Fatalf("task-a-2 should wait while task-b-1 is running, got pipeline=%q", taskA2.PipelineID)
	}

	if err := s.OnEvent(context.Background(), core.Event{Type: core.EventPipelineDone, PipelineID: taskB1.PipelineID, Timestamp: time.Now()}); err != nil {
		t.Fatalf("OnEvent(done B1) error = %v", err)
	}
	waitTaskStatus(t, store, "task-a-2", core.ItemRunning, 2*time.Second)
}

func TestDepScheduler_OnEvent_PersistenceFailureRetainsTerminalEventForRetry(t *testing.T) {
	baseStore := newSchedulerTestStore(t)
	defer baseStore.Close()
	store := &flakyTaskSaveStore{
		Store:      baseStore,
		failTaskID: "task-a",
		failStatus: core.ItemDone,
	}

	project := mustCreateSchedulerProject(t, store, "proj-scheduler-event-retry")
	plan := mustCreateTaskPlanWithItems(t, store, project.ID, "plan-event-retry", core.FailBlock, []core.TaskItem{
		newTaskItem("task-a", "A", nil),
	})

	s := NewDepScheduler(store, nil, (&schedulerRunner{}).Run, nil, 1)
	if err := s.StartPlan(context.Background(), plan); err != nil {
		t.Fatalf("StartPlan() error = %v", err)
	}

	taskA := waitTaskStatus(t, store, "task-a", core.ItemRunning, 2*time.Second)
	err := s.OnEvent(context.Background(), core.Event{Type: core.EventPipelineDone, PipelineID: taskA.PipelineID, Timestamp: time.Now()})
	if err == nil {
		t.Fatalf("OnEvent(first done) should fail due to injected SaveTaskItem error")
	}
	if !errors.Is(err, errInjectedTaskSave) {
		t.Fatalf("OnEvent(first done) error = %v, want %v", err, errInjectedTaskSave)
	}

	if _, ok := s.pipelineIndex[taskA.PipelineID]; !ok {
		t.Fatalf("pipeline index removed before persistence succeeded")
	}
	if got := len(s.sem); got != 1 {
		t.Fatalf("slot should remain occupied after failed persistence, got len(sem)=%d", got)
	}

	if err := s.OnEvent(context.Background(), core.Event{Type: core.EventPipelineDone, PipelineID: taskA.PipelineID, Timestamp: time.Now()}); err != nil {
		t.Fatalf("OnEvent(second done) error = %v", err)
	}
	waitTaskStatus(t, store, "task-a", core.ItemDone, 2*time.Second)
	waitPlanStatus(t, store, plan.ID, core.PlanDone, 2*time.Second)
	if _, ok := s.pipelineIndex[taskA.PipelineID]; ok {
		t.Fatalf("pipeline index should be removed after successful retry")
	}
	if got := len(s.sem); got != 0 {
		t.Fatalf("slot should be released after successful retry, got len(sem)=%d", got)
	}
}

func TestDepScheduler_EmitsPlanScopedLifecycleEvents(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-scheduler-events-done")
	plan := mustCreateTaskPlanWithItems(t, store, project.ID, "plan-events-done", core.FailBlock, []core.TaskItem{
		newTaskItem("task-a", "A", nil),
	})

	bus := &recordingSchedulerBus{}
	runner := &schedulerRunner{}
	s := NewDepScheduler(store, bus, runner.Run, nil, 1)

	if err := s.StartPlan(context.Background(), plan); err != nil {
		t.Fatalf("StartPlan() error = %v", err)
	}
	taskA := waitTaskStatus(t, store, "task-a", core.ItemRunning, 2*time.Second)
	if taskA.PipelineID == "" {
		t.Fatalf("expected running task pipeline id")
	}

	if !bus.HasEvent(core.EventPlanApproved, plan.ID) {
		t.Fatalf("expected %q event with plan_id=%s", core.EventPlanApproved, plan.ID)
	}
	if !bus.HasEvent(core.EventTaskReady, plan.ID) {
		t.Fatalf("expected %q event with plan_id=%s", core.EventTaskReady, plan.ID)
	}
	if !bus.HasEvent(core.EventTaskRunning, plan.ID) {
		t.Fatalf("expected %q event with plan_id=%s", core.EventTaskRunning, plan.ID)
	}
	if !bus.HasEvent(core.EventSecretaryThinking, plan.ID) {
		t.Fatalf("expected %q event with plan_id=%s", core.EventSecretaryThinking, plan.ID)
	}

	if err := s.OnEvent(context.Background(), core.Event{
		Type:       core.EventPipelineDone,
		PipelineID: taskA.PipelineID,
		Timestamp:  time.Now(),
	}); err != nil {
		t.Fatalf("OnEvent(done A) error = %v", err)
	}
	waitPlanStatus(t, store, plan.ID, core.PlanDone, 2*time.Second)

	if !bus.HasEvent(core.EventTaskDone, plan.ID) {
		t.Fatalf("expected %q event with plan_id=%s", core.EventTaskDone, plan.ID)
	}
	if !bus.HasEvent(core.EventPlanDone, plan.ID) {
		t.Fatalf("expected %q event with plan_id=%s", core.EventPlanDone, plan.ID)
	}

	taskRunningEvt, ok := bus.FirstEvent(core.EventTaskRunning, plan.ID)
	if !ok {
		t.Fatalf("missing %q event with plan_id=%s", core.EventTaskRunning, plan.ID)
	}
	if taskRunningEvt.PipelineID == "" {
		t.Fatalf("%q should include pipeline_id", core.EventTaskRunning)
	}
	if taskRunningEvt.Data["task_id"] != "task-a" {
		t.Fatalf("%q should include task_id=task-a, got %+v", core.EventTaskRunning, taskRunningEvt.Data)
	}

	planDoneEvt, ok := bus.FirstEvent(core.EventPlanDone, plan.ID)
	if !ok {
		t.Fatalf("missing %q event with plan_id=%s", core.EventPlanDone, plan.ID)
	}
	if planDoneEvt.Data["stats_total"] != "1" {
		t.Fatalf("%q should include stats_total=1, got %+v", core.EventPlanDone, planDoneEvt.Data)
	}
	if planDoneEvt.Data["stats_done"] != "1" {
		t.Fatalf("%q should include stats_done=1, got %+v", core.EventPlanDone, planDoneEvt.Data)
	}
	for _, evt := range bus.Events() {
		if !isPlanScopedSecretaryEvent(evt.Type) {
			continue
		}
		if evt.PlanID == "" {
			t.Fatalf("event %q should carry plan_id, got %+v", evt.Type, evt)
		}
	}
}

func TestDepScheduler_EmitsPlanWaitingHumanAndTaskFailedEvents(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-scheduler-events-human")
	plan := mustCreateTaskPlanWithItems(t, store, project.ID, "plan-events-human", core.FailHuman, []core.TaskItem{
		newTaskItem("task-a", "A", nil),
		newTaskItem("task-b", "B", []string{"task-a"}),
	})

	bus := &recordingSchedulerBus{}
	s := NewDepScheduler(store, bus, (&schedulerRunner{}).Run, nil, 1)

	if err := s.StartPlan(context.Background(), plan); err != nil {
		t.Fatalf("StartPlan() error = %v", err)
	}
	taskA := waitTaskStatus(t, store, "task-a", core.ItemRunning, 2*time.Second)
	if err := s.OnEvent(context.Background(), core.Event{
		Type:       core.EventPipelineFailed,
		PipelineID: taskA.PipelineID,
		Timestamp:  time.Now(),
		Error:      "need human",
	}); err != nil {
		t.Fatalf("OnEvent(failed A) error = %v", err)
	}
	waitPlanStatus(t, store, plan.ID, core.PlanWaitingHuman, 2*time.Second)

	if !bus.HasEvent(core.EventTaskFailed, plan.ID) {
		t.Fatalf("expected %q event with plan_id=%s", core.EventTaskFailed, plan.ID)
	}
	if !bus.HasEvent(core.EventPlanWaitingHuman, plan.ID) {
		t.Fatalf("expected %q event with plan_id=%s", core.EventPlanWaitingHuman, plan.ID)
	}

	taskFailedEvt, ok := bus.FirstEvent(core.EventTaskFailed, plan.ID)
	if !ok {
		t.Fatalf("missing %q event with plan_id=%s", core.EventTaskFailed, plan.ID)
	}
	if taskFailedEvt.Error != "need human" {
		t.Fatalf("%q should include error message, got %+v", core.EventTaskFailed, taskFailedEvt)
	}

	waitEvt, ok := bus.FirstEvent(core.EventPlanWaitingHuman, plan.ID)
	if !ok {
		t.Fatalf("missing %q event with plan_id=%s", core.EventPlanWaitingHuman, plan.ID)
	}
	if waitEvt.Data["wait_reason"] != string(core.WaitFeedbackReq) {
		t.Fatalf("%q should include wait_reason=%s, got %+v", core.EventPlanWaitingHuman, core.WaitFeedbackReq, waitEvt.Data)
	}
}

func TestDepScheduler_StartPlanRejectsInvalidState(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-scheduler-invalid-state")
	plan := mustCreateTaskPlanWithItems(t, store, project.ID, "plan-invalid-state", core.FailBlock, []core.TaskItem{
		newTaskItem("task-a", "A", nil),
	})
	plan.Status = core.PlanDraft
	plan.WaitReason = core.WaitNone
	if err := store.SaveTaskPlan(plan); err != nil {
		t.Fatalf("SaveTaskPlan(draft) error = %v", err)
	}

	s := NewDepScheduler(store, nil, (&schedulerRunner{}).Run, nil, 1)
	err := s.StartPlan(context.Background(), plan)
	if err == nil {
		t.Fatalf("StartPlan() expected error for draft plan")
	}
	if !strings.Contains(err.Error(), "start plan requires approved or waiting_human/final_approval") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSchedulerDefaultStageConfig_DefaultAgentAndE2E(t *testing.T) {
	for _, stageID := range []core.StageID{
		core.StageRequirements,
		core.StageCodeReview,
	} {
		cfg := schedulerDefaultStageConfig(stageID)
		if cfg.Agent != "claude" {
			t.Fatalf("stage %s should default to claude, got %q", stageID, cfg.Agent)
		}
	}

	for _, stageID := range []core.StageID{
		core.StageImplement,
		core.StageFixup,
		core.StageE2ETest,
	} {
		cfg := schedulerDefaultStageConfig(stageID)
		if cfg.Agent != "codex" {
			t.Fatalf("stage %s should default to codex, got %q", stageID, cfg.Agent)
		}
	}

	cfg := schedulerDefaultStageConfig(core.StageE2ETest)
	if cfg.Timeout != 15*time.Minute {
		t.Fatalf("e2e_test timeout mismatch, got %s want %s", cfg.Timeout, 15*time.Minute)
	}
}

type schedulerRunner struct {
	mu    sync.Mutex
	calls []string
}

type recordingSchedulerBus struct {
	mu     sync.Mutex
	events []core.Event
}

func (b *recordingSchedulerBus) Subscribe() chan core.Event {
	return make(chan core.Event, 1)
}

func (b *recordingSchedulerBus) Unsubscribe(ch chan core.Event) {
	close(ch)
}

func (b *recordingSchedulerBus) Publish(evt core.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	clone := evt
	if len(evt.Data) > 0 {
		clone.Data = make(map[string]string, len(evt.Data))
		for k, v := range evt.Data {
			clone.Data[k] = v
		}
	}
	b.events = append(b.events, clone)
}

func (b *recordingSchedulerBus) HasEvent(eventType core.EventType, planID string) bool {
	for _, evt := range b.Events() {
		if evt.Type == eventType && evt.PlanID == planID {
			return true
		}
	}
	return false
}

func (b *recordingSchedulerBus) FirstEvent(eventType core.EventType, planID string) (core.Event, bool) {
	for _, evt := range b.Events() {
		if evt.Type == eventType && evt.PlanID == planID {
			return evt, true
		}
	}
	return core.Event{}, false
}

func (b *recordingSchedulerBus) Events() []core.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]core.Event, len(b.events))
	copy(out, b.events)
	return out
}

func isPlanScopedSecretaryEvent(eventType core.EventType) bool {
	switch eventType {
	case core.EventSecretaryThinking,
		core.EventPlanCreated,
		core.EventPlanReviewing,
		core.EventReviewAgentDone,
		core.EventReviewComplete,
		core.EventPlanApproved,
		core.EventPlanWaitingHuman,
		core.EventTaskReady,
		core.EventTaskRunning,
		core.EventTaskDone,
		core.EventTaskFailed,
		core.EventPlanDone,
		core.EventPlanFailed,
		core.EventPlanPartiallyDone:
		return true
	default:
		return false
	}
}

func (r *schedulerRunner) Run(_ context.Context, pipelineID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, pipelineID)
	return nil
}

func (r *schedulerRunner) CallCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

func newSchedulerTestStore(t *testing.T) core.Store {
	t.Helper()
	s, err := storesqlite.New(":memory:")
	if err != nil {
		t.Fatalf("storesqlite.New() error = %v", err)
	}
	return s
}

func mustCreateSchedulerProject(t *testing.T, store core.Store, id string) *core.Project {
	t.Helper()
	p := &core.Project{
		ID:       id,
		Name:     id,
		RepoPath: t.TempDir(),
	}
	if err := store.CreateProject(p); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	return p
}

func mustCreateTaskPlanWithItems(
	t *testing.T,
	store core.Store,
	projectID string,
	planID string,
	failPolicy core.FailurePolicy,
	items []core.TaskItem,
) *core.TaskPlan {
	t.Helper()
	plan := &core.TaskPlan{
		ID:         planID,
		ProjectID:  projectID,
		Name:       planID,
		Status:     core.PlanApproved,
		FailPolicy: failPolicy,
	}
	if err := store.CreateTaskPlan(plan); err != nil {
		t.Fatalf("CreateTaskPlan() error = %v", err)
	}

	plan.Tasks = make([]core.TaskItem, 0, len(items))
	for _, item := range items {
		it := item
		it.PlanID = plan.ID
		if it.Status == "" {
			it.Status = core.ItemPending
		}
		if err := store.CreateTaskItem(&it); err != nil {
			t.Fatalf("CreateTaskItem(%s) error = %v", it.ID, err)
		}
		plan.Tasks = append(plan.Tasks, it)
	}
	return plan
}

func newTaskItem(id, title string, dependsOn []string) core.TaskItem {
	return core.TaskItem{
		ID:          id,
		Title:       title,
		Description: title,
		DependsOn:   dependsOn,
		Status:      core.ItemPending,
	}
}

func waitTaskStatus(t *testing.T, store core.Store, taskID string, want core.TaskItemStatus, timeout time.Duration) *core.TaskItem {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		item, err := store.GetTaskItem(taskID)
		if err != nil {
			t.Fatalf("GetTaskItem(%s) error = %v", taskID, err)
		}
		if item.Status == want {
			return item
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting task %s status %q, got %q", taskID, want, item.Status)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func waitPlanStatus(t *testing.T, store core.Store, planID string, want core.TaskPlanStatus, timeout time.Duration) *core.TaskPlan {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		plan, err := store.GetTaskPlan(planID)
		if err != nil {
			t.Fatalf("GetTaskPlan(%s) error = %v", planID, err)
		}
		if plan.Status == want {
			return plan
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting plan %s status %q, got %q", planID, want, plan.Status)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

var errInjectedTaskSave = errors.New("injected save task error")

type flakyTaskSaveStore struct {
	core.Store

	mu         sync.Mutex
	failTaskID string
	failStatus core.TaskItemStatus
	failedOnce bool
}

func (s *flakyTaskSaveStore) SaveTaskItem(item *core.TaskItem) error {
	s.mu.Lock()
	shouldFail := !s.failedOnce &&
		item != nil &&
		item.ID == s.failTaskID &&
		item.Status == s.failStatus
	if shouldFail {
		s.failedOnce = true
	}
	s.mu.Unlock()

	if shouldFail {
		return errInjectedTaskSave
	}
	return s.Store.SaveTaskItem(item)
}
