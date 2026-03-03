package secretary

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	storesqlite "github.com/yoke233/ai-workflow/internal/plugins/store-sqlite"
)

func TestA2ABridge_SendMessageDelegatesToCreateDraft(t *testing.T) {
	store := newA2ABridgeTestStore(t)
	project := mustCreateA2ABridgeProject(t, store, "proj-a2a-send")

	manager := &fakeA2ABridgeManager{
		createDraftFn: func(_ context.Context, input CreateDraftInput) (*core.TaskPlan, error) {
			if input.ProjectID != project.ID {
				t.Fatalf("create draft project_id = %q, want %q", input.ProjectID, project.ID)
			}
			if input.Request.Conversation != "A2A send request" {
				t.Fatalf("create draft conversation = %q, want %q", input.Request.Conversation, "A2A send request")
			}
			return &core.TaskPlan{
				ID:        "plan-a2a-send",
				ProjectID: project.ID,
				SessionID: "chat-a2a-send",
				Status:    core.PlanDraft,
				UpdatedAt: time.Now(),
			}, nil
		},
	}

	bridge, err := NewA2ABridge(store, manager)
	if err != nil {
		t.Fatalf("NewA2ABridge() error = %v", err)
	}

	task, err := bridge.SendMessage(context.Background(), A2ASendMessageInput{
		ProjectID:    project.ID,
		SessionID:    "chat-a2a-send",
		Conversation: "A2A send request",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if task.TaskID != "plan-a2a-send" {
		t.Fatalf("task id = %q, want %q", task.TaskID, "plan-a2a-send")
	}
	if task.State != A2ATaskStateSubmitted {
		t.Fatalf("task state = %q, want %q", task.State, A2ATaskStateSubmitted)
	}
	if manager.createDraftCalls != 1 {
		t.Fatalf("create draft calls = %d, want 1", manager.createDraftCalls)
	}
}

func TestA2ABridge_GetTaskReturnsSnapshot(t *testing.T) {
	store := newA2ABridgeTestStore(t)
	project := mustCreateA2ABridgeProject(t, store, "proj-a2a-get")

	manager := &fakeA2ABridgeManager{
		getPlanFn: func(_ context.Context, planID string) (*core.TaskPlan, error) {
			if planID != "plan-a2a-get" {
				t.Fatalf("get plan id = %q, want %q", planID, "plan-a2a-get")
			}
			return &core.TaskPlan{
				ID:        "plan-a2a-get",
				ProjectID: project.ID,
				SessionID: "chat-a2a-get",
				Status:    core.PlanExecuting,
				UpdatedAt: time.Now(),
			}, nil
		},
	}

	bridge, err := NewA2ABridge(store, manager)
	if err != nil {
		t.Fatalf("NewA2ABridge() error = %v", err)
	}

	task, err := bridge.GetTask(context.Background(), A2AGetTaskInput{
		ProjectID: project.ID,
		TaskID:    "plan-a2a-get",
	})
	if err != nil {
		t.Fatalf("GetTask() error = %v", err)
	}
	if task.State != A2ATaskStateWorking {
		t.Fatalf("task state = %q, want %q", task.State, A2ATaskStateWorking)
	}
	if manager.getPlanCalls != 1 {
		t.Fatalf("get plan calls = %d, want 1", manager.getPlanCalls)
	}
}

func TestA2ABridge_CancelTaskDelegatesToCancelPlan(t *testing.T) {
	store := newA2ABridgeTestStore(t)
	project := mustCreateA2ABridgeProject(t, store, "proj-a2a-cancel")

	manager := &fakeA2ABridgeManager{
		getPlanFn: func(_ context.Context, _ string) (*core.TaskPlan, error) {
			return &core.TaskPlan{
				ID:        "plan-a2a-cancel",
				ProjectID: project.ID,
				Status:    core.PlanExecuting,
				UpdatedAt: time.Now(),
			}, nil
		},
		cancelPlanFn: func(_ context.Context, planID string) (*core.TaskPlan, error) {
			if planID != "plan-a2a-cancel" {
				t.Fatalf("cancel plan id = %q, want %q", planID, "plan-a2a-cancel")
			}
			return &core.TaskPlan{
				ID:        "plan-a2a-cancel",
				ProjectID: project.ID,
				Status:    core.PlanAbandoned,
				UpdatedAt: time.Now(),
			}, nil
		},
	}

	bridge, err := NewA2ABridge(store, manager)
	if err != nil {
		t.Fatalf("NewA2ABridge() error = %v", err)
	}

	task, err := bridge.CancelTask(context.Background(), A2ACancelTaskInput{
		ProjectID: project.ID,
		TaskID:    "plan-a2a-cancel",
	})
	if err != nil {
		t.Fatalf("CancelTask() error = %v", err)
	}
	if task.State != A2ATaskStateCanceled {
		t.Fatalf("task state = %q, want %q", task.State, A2ATaskStateCanceled)
	}
	if manager.cancelPlanCalls != 1 {
		t.Fatalf("cancel plan calls = %d, want 1", manager.cancelPlanCalls)
	}
}

func TestA2ABridge_ProjectScopeMismatchFails(t *testing.T) {
	store := newA2ABridgeTestStore(t)
	mustCreateA2ABridgeProject(t, store, "proj-a2a-scope-a")
	projectB := mustCreateA2ABridgeProject(t, store, "proj-a2a-scope-b")

	manager := &fakeA2ABridgeManager{
		getPlanFn: func(_ context.Context, _ string) (*core.TaskPlan, error) {
			return &core.TaskPlan{
				ID:        "plan-a2a-scope",
				ProjectID: "proj-a2a-scope-a",
				Status:    core.PlanExecuting,
				UpdatedAt: time.Now(),
			}, nil
		},
	}

	bridge, err := NewA2ABridge(store, manager)
	if err != nil {
		t.Fatalf("NewA2ABridge() error = %v", err)
	}

	_, err = bridge.GetTask(context.Background(), A2AGetTaskInput{
		ProjectID: projectB.ID,
		TaskID:    "plan-a2a-scope",
	})
	if !errors.Is(err, ErrA2AProjectScope) {
		t.Fatalf("GetTask() error = %v, want ErrA2AProjectScope", err)
	}
}

type fakeA2ABridgeManager struct {
	createDraftCalls int
	getPlanCalls     int
	cancelPlanCalls  int

	createDraftFn func(ctx context.Context, input CreateDraftInput) (*core.TaskPlan, error)
	getPlanFn     func(ctx context.Context, planID string) (*core.TaskPlan, error)
	cancelPlanFn  func(ctx context.Context, planID string) (*core.TaskPlan, error)
}

func (f *fakeA2ABridgeManager) CreateDraft(ctx context.Context, input CreateDraftInput) (*core.TaskPlan, error) {
	f.createDraftCalls++
	if f.createDraftFn == nil {
		return nil, errors.New("unexpected CreateDraft call")
	}
	return f.createDraftFn(ctx, input)
}

func (f *fakeA2ABridgeManager) GetPlan(ctx context.Context, planID string) (*core.TaskPlan, error) {
	f.getPlanCalls++
	if f.getPlanFn == nil {
		return nil, errors.New("unexpected GetPlan call")
	}
	return f.getPlanFn(ctx, planID)
}

func (f *fakeA2ABridgeManager) CancelPlan(ctx context.Context, planID string) (*core.TaskPlan, error) {
	f.cancelPlanCalls++
	if f.cancelPlanFn == nil {
		return nil, errors.New("unexpected CancelPlan call")
	}
	return f.cancelPlanFn(ctx, planID)
}

func newA2ABridgeTestStore(t *testing.T) *storesqlite.SQLiteStore {
	t.Helper()

	store, err := storesqlite.New(":memory:")
	if err != nil {
		t.Fatalf("create sqlite store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}

func mustCreateA2ABridgeProject(t *testing.T, store core.Store, id string) *core.Project {
	t.Helper()

	project := &core.Project{
		ID:       id,
		Name:     id,
		RepoPath: t.TempDir(),
	}
	if err := store.CreateProject(project); err != nil {
		t.Fatalf("create project %q: %v", id, err)
	}
	return project
}
