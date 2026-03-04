package teamleader

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	storesqlite "github.com/yoke233/ai-workflow/internal/plugins/store-sqlite"
)

func TestScheduler_ProfileQueueIgnoresDependencyEdges(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-scheduler-profile-ignore-deps")
	issues := mustCreateIssueSessionWithItems(t, store, project.ID, "session-profile-ignore-deps", core.FailBlock, []core.Issue{
		newIssueWithProfile("issue-a", "A", core.WorkflowProfileStrict, nil),
		newIssueWithProfile("issue-b", "B", core.WorkflowProfileNormal, []string{"issue-a"}),
	})

	runner := &schedulerRunner{}
	bus := &recordingSchedulerBus{}
	s := NewDepScheduler(store, bus, runner.Run, nil, 2)

	if err := s.ScheduleIssues(context.Background(), issues); err != nil {
		t.Fatalf("ScheduleIssues() error = %v", err)
	}

	waitIssueStatus(t, store, "issue-a", core.IssueStatusExecuting, 2*time.Second)
	issueB := waitIssueStatus(t, store, "issue-b", core.IssueStatusExecuting, 2*time.Second)
	if issueB.RunID == "" {
		t.Fatalf("expected issue-b Run assigned even with depends_on edge")
	}

	event, ok := bus.FirstEvent(core.EventIssueReady, "issue-b")
	if !ok {
		t.Fatalf("expected issue-b ready event")
	}
	if got := event.Data["workflow_profile"]; got != string(core.WorkflowProfileNormal) {
		t.Fatalf("issue-b ready profile = %q, want %q", got, core.WorkflowProfileNormal)
	}
}

func TestScheduler_ProfileQueueDispatchesStrictBeforeNormal(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-scheduler-profile-priority")
	issues := mustCreateIssueSessionWithItems(t, store, project.ID, "session-profile-priority", core.FailBlock, []core.Issue{
		newIssueWithProfile("issue-normal", "normal", core.WorkflowProfileNormal, nil),
		newIssueWithProfile("issue-strict", "strict", core.WorkflowProfileStrict, nil),
	})

	runner := &schedulerRunner{}
	s := NewDepScheduler(store, nil, runner.Run, nil, 1)

	if err := s.ScheduleIssues(context.Background(), issues); err != nil {
		t.Fatalf("ScheduleIssues() error = %v", err)
	}

	strictIssue := waitIssueStatus(t, store, "issue-strict", core.IssueStatusExecuting, 2*time.Second)
	normalIssue := waitIssueStatus(t, store, "issue-normal", core.IssueStatusReady, 2*time.Second)
	if strictIssue.RunID == "" {
		t.Fatalf("strict issue Run id should be assigned first")
	}
	if normalIssue.RunID != "" {
		t.Fatalf("normal issue should remain ready before strict completes, got Run=%q", normalIssue.RunID)
	}
}

func TestScheduler_ProfileQueueDispatchesNextAfterRunDone(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-scheduler-profile-done")
	issues := mustCreateIssueSessionWithItems(t, store, project.ID, "session-profile-done", core.FailBlock, []core.Issue{
		newIssueWithProfile("issue-normal", "normal", core.WorkflowProfileNormal, nil),
		newIssueWithProfile("issue-strict", "strict", core.WorkflowProfileStrict, nil),
	})

	runner := &schedulerRunner{}
	s := NewDepScheduler(store, nil, runner.Run, nil, 1)
	if err := s.ScheduleIssues(context.Background(), issues); err != nil {
		t.Fatalf("ScheduleIssues() error = %v", err)
	}

	strictIssue := waitIssueStatus(t, store, "issue-strict", core.IssueStatusExecuting, 2*time.Second)
	if err := s.OnEvent(context.Background(), core.Event{
		Type:      core.EventRunDone,
		RunID:     strictIssue.RunID,
		Timestamp: time.Now(),
	}); err != nil {
		t.Fatalf("OnEvent(done strict) error = %v", err)
	}

	waitIssueStatus(t, store, "issue-strict", core.IssueStatusDone, 2*time.Second)
	normalIssue := waitIssueStatus(t, store, "issue-normal", core.IssueStatusExecuting, 2*time.Second)
	if normalIssue.RunID == "" {
		t.Fatalf("normal issue should be dispatched after strict done")
	}
}

func TestScheduler_FailPolicyBlockFailsRemainingQueuedIssues(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-scheduler-block")
	issues := mustCreateIssueSessionWithItems(t, store, project.ID, "session-block", core.FailBlock, []core.Issue{
		newIssueWithProfile("issue-a", "A", core.WorkflowProfileStrict, nil),
		newIssueWithProfile("issue-b", "B", core.WorkflowProfileNormal, nil),
	})

	s := NewDepScheduler(store, nil, (&schedulerRunner{}).Run, nil, 1)
	if err := s.ScheduleIssues(context.Background(), issues); err != nil {
		t.Fatalf("ScheduleIssues() error = %v", err)
	}

	issueA := waitIssueStatus(t, store, "issue-a", core.IssueStatusExecuting, 2*time.Second)
	if err := s.OnEvent(context.Background(), core.Event{
		Type:      core.EventRunFailed,
		RunID:     issueA.RunID,
		Error:     "boom",
		Timestamp: time.Now(),
	}); err != nil {
		t.Fatalf("OnEvent(failed A) error = %v", err)
	}

	waitIssueStatus(t, store, "issue-a", core.IssueStatusFailed, 2*time.Second)
	issueB := waitIssueStatus(t, store, "issue-b", core.IssueStatusFailed, 2*time.Second)
	if issueB.RunID != "" {
		t.Fatalf("blocked issue should not be dispatched, got Run=%q", issueB.RunID)
	}
}

func TestScheduler_RecoverExecutingIssuesReplaysDoneAndDispatchesNext(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-scheduler-recover")
	mustCreateIssueSessionWithItems(t, store, project.ID, "session-recover", core.FailBlock, []core.Issue{
		{
			ID:       "issue-a",
			Title:    "A",
			Body:     "A",
			Status:   core.IssueStatusExecuting,
			RunID:    "Run-recover-done",
			Template: "strict",
			Labels:   []string{"profile:strict"},
		},
		newIssueWithProfile("issue-b", "B", core.WorkflowProfileNormal, nil),
	})

	if err := store.SaveRun(&core.Run{
		ID:         "Run-recover-done",
		ProjectID:  project.ID,
		Name:       "Run-recover-done",
		Status:     core.StatusCompleted,
		Conclusion: core.ConclusionSuccess,
		IssueID:    "issue-a",
	}); err != nil {
		t.Fatalf("SaveRun(done) error = %v", err)
	}

	s := NewDepScheduler(store, nil, (&schedulerRunner{}).Run, nil, 1)
	if err := s.RecoverExecutingIssues(context.Background(), ""); err != nil {
		t.Fatalf("RecoverExecutingIssues() error = %v", err)
	}

	waitIssueStatus(t, store, "issue-a", core.IssueStatusDone, 2*time.Second)
	issueB := waitIssueStatus(t, store, "issue-b", core.IssueStatusExecuting, 2*time.Second)
	if issueB.RunID == "" {
		t.Fatalf("expected issue-b dispatched after replaying done event")
	}
}

func TestDepScheduler_ScheduleIssuesRejectsEmptyPayload(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	s := NewDepScheduler(store, nil, (&schedulerRunner{}).Run, nil, 1)
	err := s.ScheduleIssues(context.Background(), []*core.Issue{nil})
	if err == nil {
		t.Fatalf("ScheduleIssues() expected error for empty payload")
	}
	if got := strings.ToLower(err.Error()); !strings.Contains(got, "no issues provided") {
		t.Fatalf("ScheduleIssues() error = %v, want contains no issues provided", err)
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

func (b *recordingSchedulerBus) FirstEvent(eventType core.EventType, issueID string) (core.Event, bool) {
	for _, evt := range b.Events() {
		if evt.Type == eventType && evt.IssueID == issueID {
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

func (r *schedulerRunner) Run(_ context.Context, RunID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, RunID)
	return nil
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

func mustCreateIssueSessionWithItems(
	t *testing.T,
	store core.Store,
	projectID string,
	sessionID string,
	failPolicy core.FailurePolicy,
	items []core.Issue,
) []*core.Issue {
	t.Helper()

	trimmedSessionID := strings.TrimSpace(sessionID)
	if trimmedSessionID != "" {
		err := store.CreateChatSession(&core.ChatSession{
			ID:        trimmedSessionID,
			ProjectID: projectID,
			Messages:  []core.ChatMessage{},
		})
		if err != nil && !strings.Contains(strings.ToLower(err.Error()), "unique") {
			t.Fatalf("CreateChatSession(%s) error = %v", trimmedSessionID, err)
		}
	}

	issues := make([]*core.Issue, 0, len(items))
	for _, item := range items {
		issue := item
		issue.ProjectID = projectID
		issue.SessionID = trimmedSessionID
		if issue.Template == "" {
			issue.Template = "standard"
		}
		if issue.State == "" {
			issue.State = core.IssueStateOpen
		}
		if issue.Status == "" {
			issue.Status = core.IssueStatusQueued
		}
		if issue.FailPolicy == "" {
			issue.FailPolicy = failPolicy
		}
		if err := store.CreateIssue(&issue); err != nil {
			t.Fatalf("CreateIssue(%s) error = %v", issue.ID, err)
		}
		issues = append(issues, &issue)
	}
	return issues
}

func newIssueWithProfile(id, title string, profile core.WorkflowProfileType, dependsOn []string) core.Issue {
	return core.Issue{
		ID:        id,
		Title:     title,
		Body:      title,
		DependsOn: dependsOn,
		Labels:    []string{"profile:" + string(profile)},
		Status:    core.IssueStatusQueued,
		State:     core.IssueStateOpen,
		Template:  "standard",
	}
}

func waitIssueStatus(t *testing.T, store core.Store, issueID string, want core.IssueStatus, timeout time.Duration) *core.Issue {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		issue, err := store.GetIssue(issueID)
		if err != nil {
			t.Fatalf("GetIssue(%s) error = %v", issueID, err)
		}
		if issue.Status == want {
			return issue
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting issue %s status %q, got %q", issueID, want, issue.Status)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
