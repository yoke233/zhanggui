package threadtaskapp

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// ---------------------------------------------------------------------------
// In-memory store stub (no ACP, no real DB)
// ---------------------------------------------------------------------------

type memStore struct {
	mu       sync.Mutex
	threads  map[int64]*core.Thread
	messages []*core.ThreadMessage
	groups   map[int64]*core.ThreadTaskGroup
	tasks    map[int64]*core.ThreadTask
	nextID   int64
}

func newMemStore() *memStore {
	s := &memStore{
		threads: map[int64]*core.Thread{
			1: {ID: 1, Title: "Test Thread", Status: core.ThreadActive},
		},
		messages: nil,
		groups:   make(map[int64]*core.ThreadTaskGroup),
		tasks:    make(map[int64]*core.ThreadTask),
		nextID:   100,
	}
	return s
}

func (s *memStore) nextAutoID() int64 {
	s.nextID++
	return s.nextID
}

func (s *memStore) GetThread(_ context.Context, id int64) (*core.Thread, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.threads[id]
	if !ok {
		return nil, core.ErrNotFound
	}
	cp := *t
	return &cp, nil
}

func (s *memStore) CreateThreadMessage(_ context.Context, msg *core.ThreadMessage) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextAutoID()
	msg.ID = id
	msg.CreatedAt = time.Now().UTC()
	s.messages = append(s.messages, msg)
	return id, nil
}

func (s *memStore) CreateThreadTaskGroup(_ context.Context, g *core.ThreadTaskGroup) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextAutoID()
	g.ID = id
	g.CreatedAt = time.Now().UTC()
	cp := *g
	s.groups[id] = &cp
	return id, nil
}

func (s *memStore) GetThreadTaskGroup(_ context.Context, id int64) (*core.ThreadTaskGroup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.groups[id]
	if !ok {
		return nil, core.ErrNotFound
	}
	cp := *g
	return &cp, nil
}

func (s *memStore) ListThreadTaskGroups(_ context.Context, filter core.ThreadTaskGroupFilter) ([]*core.ThreadTaskGroup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*core.ThreadTaskGroup
	for _, g := range s.groups {
		if filter.ThreadID != nil && g.ThreadID != *filter.ThreadID {
			continue
		}
		cp := *g
		out = append(out, &cp)
	}
	return out, nil
}

func (s *memStore) UpdateThreadTaskGroup(_ context.Context, g *core.ThreadTaskGroup) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.groups[g.ID]; !ok {
		return core.ErrNotFound
	}
	cp := *g
	s.groups[g.ID] = &cp
	return nil
}

func (s *memStore) DeleteThreadTaskGroup(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.groups, id)
	// Also delete tasks
	for tid, t := range s.tasks {
		if t.GroupID == id {
			delete(s.tasks, tid)
		}
	}
	return nil
}

func (s *memStore) CreateThreadTask(_ context.Context, t *core.ThreadTask) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextAutoID()
	t.ID = id
	t.CreatedAt = time.Now().UTC()
	if t.DependsOn == nil {
		t.DependsOn = []int64{}
	}
	cp := *t
	cp.DependsOn = append([]int64(nil), t.DependsOn...)
	s.tasks[id] = &cp
	return id, nil
}

func (s *memStore) GetThreadTask(_ context.Context, id int64) (*core.ThreadTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tasks[id]
	if !ok {
		return nil, core.ErrNotFound
	}
	cp := *t
	cp.DependsOn = append([]int64(nil), t.DependsOn...)
	return &cp, nil
}

func (s *memStore) ListThreadTasksByGroup(_ context.Context, groupID int64) ([]*core.ThreadTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*core.ThreadTask
	for _, t := range s.tasks {
		if t.GroupID == groupID {
			cp := *t
			cp.DependsOn = append([]int64(nil), t.DependsOn...)
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (s *memStore) UpdateThreadTask(_ context.Context, t *core.ThreadTask) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tasks[t.ID]; !ok {
		return core.ErrNotFound
	}
	cp := *t
	cp.DependsOn = append([]int64(nil), t.DependsOn...)
	s.tasks[t.ID] = &cp
	return nil
}

func (s *memStore) DeleteThreadTasksByGroup(_ context.Context, groupID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for tid, t := range s.tasks {
		if t.GroupID == groupID {
			delete(s.tasks, tid)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Minimal event bus stub
// ---------------------------------------------------------------------------

type memBus struct {
	mu     sync.Mutex
	events []core.Event
}

func (b *memBus) Publish(_ context.Context, event core.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, event)
}

func (b *memBus) eventsByType(t core.EventType) []core.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []core.Event
	for _, e := range b.events {
		if e.Type == t {
			out = append(out, e)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestCreateTaskGroup_SingleWorkTask(t *testing.T) {
	store := newMemStore()
	bus := &memBus{}
	svc := New(Config{Store: store, Bus: bus})

	detail, err := svc.CreateTaskGroup(context.Background(), CreateTaskGroupInput{
		ThreadID:         1,
		NotifyOnComplete: false,
		Tasks: []CreateTaskInput{
			{
				Assignee:       "researcher",
				Type:           "work",
				Instruction:    "research competitive pricing",
				OutputFileName: "pricing-research.md",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateTaskGroup: %v", err)
	}

	if detail.ID == 0 {
		t.Fatal("expected non-zero group ID")
	}
	if detail.Status != core.TaskGroupRunning {
		t.Fatalf("group status = %q, want running", detail.Status)
	}
	if len(detail.Tasks) != 1 {
		t.Fatalf("tasks count = %d, want 1", len(detail.Tasks))
	}

	// Single task with no deps → should be ready or running after initial tick
	task := detail.Tasks[0]
	if task.Status != core.ThreadTaskRunning {
		t.Fatalf("task status = %q, want running (dispatched by tick)", task.Status)
	}
	if task.OutputFilePath != "outputs/pricing-research.md" {
		t.Fatalf("output_file_path = %q", task.OutputFilePath)
	}

	// Events
	groupCreatedEvents := bus.eventsByType(core.EventThreadTaskGroupCreated)
	if len(groupCreatedEvents) == 0 {
		t.Fatal("expected EventThreadTaskGroupCreated event")
	}
	taskStartedEvents := bus.eventsByType(core.EventThreadTaskStarted)
	if len(taskStartedEvents) == 0 {
		t.Fatal("expected EventThreadTaskStarted event")
	}
}

func TestSignalComplete_FinishesGroup(t *testing.T) {
	store := newMemStore()
	bus := &memBus{}
	svc := New(Config{Store: store, Bus: bus})

	detail, err := svc.CreateTaskGroup(context.Background(), CreateTaskGroupInput{
		ThreadID:         1,
		NotifyOnComplete: false,
		Tasks: []CreateTaskInput{
			{Assignee: "worker", Instruction: "do work", OutputFileName: "result.md"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTaskGroup: %v", err)
	}

	taskID := detail.Tasks[0].ID

	// Signal complete
	err = svc.Signal(context.Background(), SignalInput{
		TaskID:         taskID,
		Action:         "complete",
		OutputFilePath: "outputs/result.md",
	})
	if err != nil {
		t.Fatalf("Signal complete: %v", err)
	}

	// Task should be done
	task, _ := store.GetThreadTask(context.Background(), taskID)
	if task.Status != core.ThreadTaskDone {
		t.Fatalf("task status = %q, want done", task.Status)
	}

	// Group should be done
	group, _ := store.GetThreadTaskGroup(context.Background(), detail.ID)
	if group.Status != core.TaskGroupDone {
		t.Fatalf("group status = %q, want done", group.Status)
	}
	if group.CompletedAt == nil {
		t.Fatal("expected CompletedAt to be set")
	}

	completedEvents := bus.eventsByType(core.EventThreadTaskGroupCompleted)
	if len(completedEvents) == 0 {
		t.Fatal("expected EventThreadTaskGroupCompleted event")
	}
}

func TestSerialDAG_WorkThenReview(t *testing.T) {
	store := newMemStore()
	bus := &memBus{}
	svc := New(Config{Store: store, Bus: bus})

	detail, err := svc.CreateTaskGroup(context.Background(), CreateTaskGroupInput{
		ThreadID:         1,
		NotifyOnComplete: false,
		Tasks: []CreateTaskInput{
			{Assignee: "researcher", Type: "work", Instruction: "research", OutputFileName: "research.md"},
			{Assignee: "reviewer", Type: "review", Instruction: "review research", DependsOnIndex: []int{0}, OutputFileName: "review.md"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTaskGroup: %v", err)
	}

	if len(detail.Tasks) != 2 {
		t.Fatalf("tasks count = %d, want 2", len(detail.Tasks))
	}

	workTask := findTaskByAssignee(detail.Tasks, "researcher")
	reviewTask := findTaskByAssignee(detail.Tasks, "reviewer")
	if workTask == nil || reviewTask == nil {
		t.Fatal("expected both work and review tasks")
	}

	// Work task should be running, review should be pending
	if workTask.Status != core.ThreadTaskRunning {
		t.Fatalf("work task status = %q, want running", workTask.Status)
	}
	if reviewTask.Status != core.ThreadTaskPending {
		t.Fatalf("review task status = %q, want pending", reviewTask.Status)
	}

	// Complete work task
	err = svc.Signal(context.Background(), SignalInput{
		TaskID: workTask.ID, Action: "complete", OutputFilePath: "outputs/research.md",
	})
	if err != nil {
		t.Fatalf("Signal work complete: %v", err)
	}

	// Review task should now be running
	reviewTask, _ = store.GetThreadTask(context.Background(), reviewTask.ID)
	if reviewTask.Status != core.ThreadTaskRunning {
		t.Fatalf("review task status after work done = %q, want running", reviewTask.Status)
	}

	// Complete review (approve)
	err = svc.Signal(context.Background(), SignalInput{
		TaskID: reviewTask.ID, Action: "complete", OutputFilePath: "outputs/review.md",
	})
	if err != nil {
		t.Fatalf("Signal review complete: %v", err)
	}

	// Group should be done
	group, _ := store.GetThreadTaskGroup(context.Background(), detail.ID)
	if group.Status != core.TaskGroupDone {
		t.Fatalf("group status = %q, want done", group.Status)
	}
}

func TestReviewReject_RetryAndComplete(t *testing.T) {
	store := newMemStore()
	svc := New(Config{Store: store})

	maxRetries := 2
	detail, err := svc.CreateTaskGroup(context.Background(), CreateTaskGroupInput{
		ThreadID:         1,
		NotifyOnComplete: false,
		Tasks: []CreateTaskInput{
			{Assignee: "worker", Type: "work", Instruction: "do work", MaxRetries: &maxRetries, OutputFileName: "output.md"},
			{Assignee: "reviewer", Type: "review", Instruction: "review", DependsOnIndex: []int{0}, OutputFileName: "review.md"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTaskGroup: %v", err)
	}

	workTask := findTaskByAssignee(detail.Tasks, "worker")
	reviewTask := findTaskByAssignee(detail.Tasks, "reviewer")

	// Complete work
	_ = svc.Signal(context.Background(), SignalInput{TaskID: workTask.ID, Action: "complete", OutputFilePath: "outputs/output.md"})

	// Reject review → work should retry
	err = svc.Signal(context.Background(), SignalInput{
		TaskID: reviewTask.ID, Action: "reject", OutputFilePath: "outputs/review.md", Feedback: "needs more data",
	})
	if err != nil {
		t.Fatalf("Signal reject: %v", err)
	}

	// Work task should be running again (retry)
	workTask, _ = store.GetThreadTask(context.Background(), workTask.ID)
	if workTask.Status != core.ThreadTaskRunning {
		t.Fatalf("work task after reject = %q, want running (retried)", workTask.Status)
	}
	if workTask.RetryCount != 1 {
		t.Fatalf("retry_count = %d, want 1", workTask.RetryCount)
	}
	if workTask.ReviewFeedback != "needs more data" {
		t.Fatalf("review_feedback = %q", workTask.ReviewFeedback)
	}

	// Complete work again
	_ = svc.Signal(context.Background(), SignalInput{TaskID: workTask.ID, Action: "complete", OutputFilePath: "outputs/output.md"})

	// Complete review (approve this time)
	reviewTask, _ = store.GetThreadTask(context.Background(), reviewTask.ID)
	_ = svc.Signal(context.Background(), SignalInput{TaskID: reviewTask.ID, Action: "complete", OutputFilePath: "outputs/review.md"})

	// Group should be done
	group, _ := store.GetThreadTaskGroup(context.Background(), detail.ID)
	if group.Status != core.TaskGroupDone {
		t.Fatalf("group status = %q, want done", group.Status)
	}
}

func TestReviewReject_ExhaustedRetries_FailsGroup(t *testing.T) {
	store := newMemStore()
	svc := New(Config{Store: store})

	maxRetries := 0 // no retries allowed
	detail, err := svc.CreateTaskGroup(context.Background(), CreateTaskGroupInput{
		ThreadID:         1,
		NotifyOnComplete: false,
		Tasks: []CreateTaskInput{
			{Assignee: "worker", Type: "work", Instruction: "do work", MaxRetries: &maxRetries, OutputFileName: "output.md"},
			{Assignee: "reviewer", Type: "review", Instruction: "review", DependsOnIndex: []int{0}, OutputFileName: "review.md"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTaskGroup: %v", err)
	}

	workTask := findTaskByAssignee(detail.Tasks, "worker")
	reviewTask := findTaskByAssignee(detail.Tasks, "reviewer")

	// Complete work
	_ = svc.Signal(context.Background(), SignalInput{TaskID: workTask.ID, Action: "complete", OutputFilePath: "outputs/output.md"})

	// Reject review → retries exhausted → group fails
	_ = svc.Signal(context.Background(), SignalInput{
		TaskID: reviewTask.ID, Action: "reject", Feedback: "terrible",
	})

	// Work task should be failed
	workTask, _ = store.GetThreadTask(context.Background(), workTask.ID)
	if workTask.Status != core.ThreadTaskFailed {
		t.Fatalf("work task = %q, want failed", workTask.Status)
	}

	// Group should be failed
	group, _ := store.GetThreadTaskGroup(context.Background(), detail.ID)
	if group.Status != core.TaskGroupFailed {
		t.Fatalf("group status = %q, want failed", group.Status)
	}
}

func TestParallelDAG_TwoWorkOneSummary(t *testing.T) {
	store := newMemStore()
	svc := New(Config{Store: store})

	detail, err := svc.CreateTaskGroup(context.Background(), CreateTaskGroupInput{
		ThreadID:         1,
		NotifyOnComplete: false,
		Tasks: []CreateTaskInput{
			{Assignee: "a", Type: "work", Instruction: "research A", OutputFileName: "a.md"},
			{Assignee: "b", Type: "work", Instruction: "research B", OutputFileName: "b.md"},
			{Assignee: "c", Type: "work", Instruction: "summarize", DependsOnIndex: []int{0, 1}, OutputFileName: "summary.md"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTaskGroup: %v", err)
	}

	taskA := findTaskByAssignee(detail.Tasks, "a")
	taskB := findTaskByAssignee(detail.Tasks, "b")
	taskC := findTaskByAssignee(detail.Tasks, "c")

	// A and B should both be running, C pending
	if taskA.Status != core.ThreadTaskRunning {
		t.Fatalf("task A = %q, want running", taskA.Status)
	}
	if taskB.Status != core.ThreadTaskRunning {
		t.Fatalf("task B = %q, want running", taskB.Status)
	}
	if taskC.Status != core.ThreadTaskPending {
		t.Fatalf("task C = %q, want pending", taskC.Status)
	}

	// Complete A
	_ = svc.Signal(context.Background(), SignalInput{TaskID: taskA.ID, Action: "complete"})
	// C should still be pending (B not done yet)
	taskC, _ = store.GetThreadTask(context.Background(), taskC.ID)
	if taskC.Status != core.ThreadTaskPending {
		t.Fatalf("task C after A done = %q, want pending", taskC.Status)
	}

	// Complete B
	_ = svc.Signal(context.Background(), SignalInput{TaskID: taskB.ID, Action: "complete"})
	// C should now be running
	taskC, _ = store.GetThreadTask(context.Background(), taskC.ID)
	if taskC.Status != core.ThreadTaskRunning {
		t.Fatalf("task C after A+B done = %q, want running", taskC.Status)
	}

	// Complete C
	_ = svc.Signal(context.Background(), SignalInput{TaskID: taskC.ID, Action: "complete"})

	group, _ := store.GetThreadTaskGroup(context.Background(), detail.ID)
	if group.Status != core.TaskGroupDone {
		t.Fatalf("group status = %q, want done", group.Status)
	}
}

func TestValidation_Errors(t *testing.T) {
	store := newMemStore()
	svc := New(Config{Store: store})
	ctx := context.Background()

	// Missing thread ID
	_, err := svc.CreateTaskGroup(ctx, CreateTaskGroupInput{ThreadID: 0})
	if CodeOf(err) != CodeMissingThreadID {
		t.Fatalf("expected MISSING_THREAD_ID, got %q", CodeOf(err))
	}

	// Thread not found
	_, err = svc.CreateTaskGroup(ctx, CreateTaskGroupInput{ThreadID: 999, Tasks: []CreateTaskInput{{Assignee: "a", Instruction: "x"}}})
	if CodeOf(err) != CodeThreadNotFound {
		t.Fatalf("expected THREAD_NOT_FOUND, got %q", CodeOf(err))
	}

	// No tasks
	_, err = svc.CreateTaskGroup(ctx, CreateTaskGroupInput{ThreadID: 1})
	if CodeOf(err) != CodeMissingTasks {
		t.Fatalf("expected MISSING_TASKS, got %q", CodeOf(err))
	}

	// Self-dependency
	_, err = svc.CreateTaskGroup(ctx, CreateTaskGroupInput{
		ThreadID: 1,
		Tasks:    []CreateTaskInput{{Assignee: "a", Instruction: "x", DependsOnIndex: []int{0}}},
	})
	if CodeOf(err) != CodeDependencyCycle {
		t.Fatalf("expected DEPENDENCY_CYCLE, got %q: %v", CodeOf(err), err)
	}

	// Invalid dependency index
	_, err = svc.CreateTaskGroup(ctx, CreateTaskGroupInput{
		ThreadID: 1,
		Tasks:    []CreateTaskInput{{Assignee: "a", Instruction: "x", DependsOnIndex: []int{5}}},
	})
	if CodeOf(err) != CodeInvalidDependency {
		t.Fatalf("expected INVALID_DEPENDENCY, got %q: %v", CodeOf(err), err)
	}
}

func TestBuildTaskInput_ContainsEnvVars(t *testing.T) {
	svc := New(Config{})
	task := &core.ThreadTask{
		ID:             42,
		GroupID:         7,
		Type:           core.TaskTypeWork,
		Instruction:    "do some work",
		OutputFilePath: "outputs/result.md",
	}

	input := svc.buildTaskInput(task, nil)
	for _, want := range []string{
		"AI_WORKFLOW_TASK_ID=42",
		"AI_WORKFLOW_TASK_GROUP_ID=7",
		"AI_WORKFLOW_TASK_TYPE=work",
		"AI_WORKFLOW_OUTPUT_FILE=outputs/result.md",
		"task-signal",
	} {
		if !containsStr(input, want) {
			t.Errorf("buildTaskInput missing %q", want)
		}
	}
}

func TestDispatch_AgentInviteFails_TaskAndGroupFail(t *testing.T) {
	store := newMemStore()
	bus := &memBus{}
	failPool := &failingAgentPool{inviteErr: fmt.Errorf("ACP boot timeout")}
	svc := New(Config{Store: store, Bus: bus, AgentPool: failPool})

	detail, err := svc.CreateTaskGroup(context.Background(), CreateTaskGroupInput{
		ThreadID:         1,
		NotifyOnComplete: false,
		Tasks: []CreateTaskInput{
			{Assignee: "worker", Instruction: "do work", OutputFileName: "out.md"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTaskGroup: %v", err)
	}

	// dispatch runs in goroutine — wait briefly for it to complete
	time.Sleep(50 * time.Millisecond)

	task, _ := store.GetThreadTask(context.Background(), detail.Tasks[0].ID)
	if task.Status != core.ThreadTaskFailed {
		t.Fatalf("task status = %q, want failed", task.Status)
	}

	group, _ := store.GetThreadTaskGroup(context.Background(), detail.ID)
	if group.Status != core.TaskGroupFailed {
		t.Fatalf("group status = %q, want failed", group.Status)
	}
}

func TestDispatch_AgentSendMessageFails_TaskAndGroupFail(t *testing.T) {
	store := newMemStore()
	failPool := &failingAgentPool{sendErr: fmt.Errorf("connection refused")}
	svc := New(Config{Store: store, AgentPool: failPool})

	detail, err := svc.CreateTaskGroup(context.Background(), CreateTaskGroupInput{
		ThreadID:         1,
		NotifyOnComplete: false,
		Tasks: []CreateTaskInput{
			{Assignee: "worker", Instruction: "do work"},
		},
	})
	if err != nil {
		t.Fatalf("CreateTaskGroup: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	task, _ := store.GetThreadTask(context.Background(), detail.Tasks[0].ID)
	if task.Status != core.ThreadTaskFailed {
		t.Fatalf("task status = %q, want failed", task.Status)
	}

	group, _ := store.GetThreadTaskGroup(context.Background(), detail.ID)
	if group.Status != core.TaskGroupFailed {
		t.Fatalf("group status = %q, want failed", group.Status)
	}
}

// failingAgentPool simulates ACP launch failures.
type failingAgentPool struct {
	inviteErr error
	sendErr   error
}

func (p *failingAgentPool) InviteAgent(_ context.Context, _ int64, _ string) (*core.ThreadMember, error) {
	if p.inviteErr != nil {
		return nil, p.inviteErr
	}
	return &core.ThreadMember{ID: 1}, nil
}

func (p *failingAgentPool) WaitAgentReady(_ context.Context, _ int64, _ string) error {
	return nil
}

func (p *failingAgentPool) SendMessage(_ context.Context, _ int64, _ string, _ string) error {
	return p.sendErr
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func findTaskByAssignee(tasks []*core.ThreadTask, assignee string) *core.ThreadTask {
	for _, t := range tasks {
		if t.Assignee == assignee {
			return t
		}
	}
	return nil
}

func containsStr(haystack, needle string) bool {
	return len(haystack) > 0 && len(needle) > 0 && fmt.Sprintf("%s", haystack) != "" && // avoid unused import
		stringContains(haystack, needle)
}

func stringContains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
