package secretary

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
	storesqlite "github.com/yoke233/ai-workflow/internal/plugins/store-sqlite"
)

func TestA2ABridge_SendMessageDelegatesToCreateIssues(t *testing.T) {
	store := newA2ABridgeTestStore(t)
	project := mustCreateA2ABridgeProject(t, store, "proj-a2a-send")

	manager := &fakeA2AIssueManager{
		createIssuesFn: func(_ context.Context, input CreateIssuesInput) ([]*core.Issue, error) {
			if input.ProjectID != project.ID {
				t.Fatalf("create issues project_id = %q, want %q", input.ProjectID, project.ID)
			}
			if input.SessionID != "chat-a2a-send" {
				t.Fatalf("create issues session_id = %q, want %q", input.SessionID, "chat-a2a-send")
			}
			if len(input.Issues) != 1 {
				t.Fatalf("create issues count = %d, want 1", len(input.Issues))
			}
			spec := input.Issues[0]
			if spec.Template != "standard" {
				t.Fatalf("issue template = %q, want %q", spec.Template, "standard")
			}
			if !strings.Contains(spec.Body, "A2A send request") {
				t.Fatalf("issue body should contain conversation, got %q", spec.Body)
			}
			return []*core.Issue{
				{
					ID:        "issue-a2a-send",
					ProjectID: project.ID,
					SessionID: "chat-a2a-send",
					Status:    core.IssueStatusDraft,
				},
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
	if task.TaskID != "issue-a2a-send" {
		t.Fatalf("task id = %q, want %q", task.TaskID, "issue-a2a-send")
	}
	if task.State != A2ATaskStateSubmitted {
		t.Fatalf("task state = %q, want %q", task.State, A2ATaskStateSubmitted)
	}
	if manager.createIssuesCalls != 1 {
		t.Fatalf("create issues calls = %d, want 1", manager.createIssuesCalls)
	}
}

func TestA2ABridge_SendMessageFallbackWhenCreateIssuesFails(t *testing.T) {
	store := newA2ABridgeTestStore(t)
	project := mustCreateA2ABridgeProject(t, store, "proj-a2a-send-fallback")
	mustCreateA2ABridgeChatSession(t, store, project.ID, "chat-a2a-fallback")

	manager := &fakeA2AIssueManager{
		createIssuesFn: func(_ context.Context, _ CreateIssuesInput) ([]*core.Issue, error) {
			return nil, errors.New("issue generation unavailable")
		},
	}

	bridge, err := NewA2ABridge(store, manager)
	if err != nil {
		t.Fatalf("NewA2ABridge() error = %v", err)
	}

	task, err := bridge.SendMessage(context.Background(), A2ASendMessageInput{
		ProjectID:    project.ID,
		SessionID:    "chat-a2a-fallback",
		Conversation: "fallback request",
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if strings.TrimSpace(task.TaskID) == "" {
		t.Fatal("expected fallback task id to be present")
	}
	if task.State != A2ATaskStateInputRequired {
		t.Fatalf("task state = %q, want %q", task.State, A2ATaskStateInputRequired)
	}

	fetched, err := bridge.GetTask(context.Background(), A2AGetTaskInput{
		ProjectID: project.ID,
		TaskID:    task.TaskID,
	})
	if err != nil {
		t.Fatalf("GetTask() error = %v", err)
	}
	if fetched.State != A2ATaskStateInputRequired {
		t.Fatalf("fetched state = %q, want %q", fetched.State, A2ATaskStateInputRequired)
	}
	if fetched.SessionID != "chat-a2a-fallback" {
		t.Fatalf("fetched session id = %q, want %q", fetched.SessionID, "chat-a2a-fallback")
	}
}

func TestA2ABridge_GetTaskReturnsSnapshot(t *testing.T) {
	store := newA2ABridgeTestStore(t)
	project := mustCreateA2ABridgeProject(t, store, "proj-a2a-get")
	mustCreateA2ABridgeIssue(t, store, &core.Issue{
		ID:        "issue-a2a-get",
		ProjectID: project.ID,
		Title:     "a2a get",
		Template:  "standard",
		Status:    core.IssueStatusExecuting,
	})

	bridge, err := NewA2ABridge(store, &fakeA2AIssueManager{})
	if err != nil {
		t.Fatalf("NewA2ABridge() error = %v", err)
	}

	task, err := bridge.GetTask(context.Background(), A2AGetTaskInput{
		ProjectID: project.ID,
		TaskID:    "issue-a2a-get",
	})
	if err != nil {
		t.Fatalf("GetTask() error = %v", err)
	}
	if task.State != A2ATaskStateWorking {
		t.Fatalf("task state = %q, want %q", task.State, A2ATaskStateWorking)
	}
}

func TestA2ABridge_CancelTaskDelegatesToApplyIssueAction(t *testing.T) {
	store := newA2ABridgeTestStore(t)
	project := mustCreateA2ABridgeProject(t, store, "proj-a2a-cancel")
	mustCreateA2ABridgeIssue(t, store, &core.Issue{
		ID:        "issue-a2a-cancel",
		ProjectID: project.ID,
		Title:     "a2a cancel",
		Template:  "standard",
		Status:    core.IssueStatusExecuting,
	})

	manager := &fakeA2AIssueManager{
		applyActionFn: func(_ context.Context, issueID, action, feedback string) (*core.Issue, error) {
			if issueID != "issue-a2a-cancel" {
				t.Fatalf("apply action issue id = %q, want %q", issueID, "issue-a2a-cancel")
			}
			if action != IssueActionAbandon {
				t.Fatalf("apply action = %q, want %q", action, IssueActionAbandon)
			}
			if feedback != "a2a cancel" {
				t.Fatalf("apply feedback = %q, want %q", feedback, "a2a cancel")
			}
			return &core.Issue{
				ID:        issueID,
				ProjectID: project.ID,
				Title:     "a2a cancel",
				Template:  "standard",
				Status:    core.IssueStatusAbandoned,
				State:     core.IssueStateClosed,
			}, nil
		},
	}

	bridge, err := NewA2ABridge(store, manager)
	if err != nil {
		t.Fatalf("NewA2ABridge() error = %v", err)
	}

	task, err := bridge.CancelTask(context.Background(), A2ACancelTaskInput{
		ProjectID: project.ID,
		TaskID:    "issue-a2a-cancel",
	})
	if err != nil {
		t.Fatalf("CancelTask() error = %v", err)
	}
	if task.State != A2ATaskStateCanceled {
		t.Fatalf("task state = %q, want %q", task.State, A2ATaskStateCanceled)
	}
	if manager.applyActionCalls != 1 {
		t.Fatalf("apply action calls = %d, want 1", manager.applyActionCalls)
	}
}

func TestA2ABridge_ProjectScopeMismatchFails(t *testing.T) {
	store := newA2ABridgeTestStore(t)
	mustCreateA2ABridgeProject(t, store, "proj-a2a-scope-a")
	projectB := mustCreateA2ABridgeProject(t, store, "proj-a2a-scope-b")
	mustCreateA2ABridgeIssue(t, store, &core.Issue{
		ID:        "issue-a2a-scope",
		ProjectID: "proj-a2a-scope-a",
		Title:     "scope",
		Template:  "standard",
		Status:    core.IssueStatusExecuting,
	})

	bridge, err := NewA2ABridge(store, &fakeA2AIssueManager{})
	if err != nil {
		t.Fatalf("NewA2ABridge() error = %v", err)
	}

	_, err = bridge.GetTask(context.Background(), A2AGetTaskInput{
		ProjectID: projectB.ID,
		TaskID:    "issue-a2a-scope",
	})
	if !errors.Is(err, ErrA2AProjectScope) {
		t.Fatalf("GetTask() error = %v, want ErrA2AProjectScope", err)
	}
}

type fakeA2AIssueManager struct {
	createIssuesCalls int
	applyActionCalls  int

	createIssuesFn func(ctx context.Context, input CreateIssuesInput) ([]*core.Issue, error)
	applyActionFn  func(ctx context.Context, issueID, action, feedback string) (*core.Issue, error)
}

func (f *fakeA2AIssueManager) CreateIssues(ctx context.Context, input CreateIssuesInput) ([]*core.Issue, error) {
	f.createIssuesCalls++
	if f.createIssuesFn == nil {
		return nil, errors.New("unexpected CreateIssues call")
	}
	return f.createIssuesFn(ctx, input)
}

func (f *fakeA2AIssueManager) ApplyIssueAction(ctx context.Context, issueID, action, feedback string) (*core.Issue, error) {
	f.applyActionCalls++
	if f.applyActionFn == nil {
		return &core.Issue{
			ID:        issueID,
			ProjectID: "proj-default",
			Title:     "fallback",
			Template:  "standard",
			Status:    core.IssueStatusAbandoned,
			State:     core.IssueStateClosed,
		}, nil
	}
	return f.applyActionFn(ctx, issueID, action, feedback)
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

func mustCreateA2ABridgeChatSession(t *testing.T, store core.Store, projectID string, sessionID string) {
	t.Helper()

	if err := store.CreateChatSession(&core.ChatSession{
		ID:        sessionID,
		ProjectID: projectID,
	}); err != nil {
		t.Fatalf("create chat session %q: %v", sessionID, err)
	}
}

func mustCreateA2ABridgeIssue(t *testing.T, store core.Store, issue *core.Issue) {
	t.Helper()
	if err := store.CreateIssue(issue); err != nil {
		t.Fatalf("create issue %q: %v", issue.ID, err)
	}
}
