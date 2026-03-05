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

func TestScheduler_AutoMergeRunDoneTransitionsToMergingAndHoldsSlot(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-scheduler-automerge-merging")
	autoMerge := newIssueWithProfile("issue-merge", "merge", core.WorkflowProfileStrict, nil)
	autoMerge.AutoMerge = true
	issues := mustCreateIssueSessionWithItems(t, store, project.ID, "session-automerge-merging", core.FailBlock, []core.Issue{
		autoMerge,
		newIssueWithProfile("issue-next", "next", core.WorkflowProfileNormal, nil),
	})

	bus := &recordingSchedulerBus{}
	s := NewDepScheduler(store, bus, (&schedulerRunner{}).Run, nil, 1)
	if err := s.ScheduleIssues(context.Background(), issues); err != nil {
		t.Fatalf("ScheduleIssues() error = %v", err)
	}

	issueMerge := waitIssueStatus(t, store, "issue-merge", core.IssueStatusExecuting, 2*time.Second)
	if err := s.OnEvent(context.Background(), core.Event{
		Type:      core.EventRunDone,
		RunID:     issueMerge.RunID,
		Timestamp: time.Now(),
	}); err != nil {
		t.Fatalf("OnEvent(run_done merge) error = %v", err)
	}

	mergedState := waitIssueStatus(t, store, "issue-merge", core.IssueStatusMerging, 2*time.Second)
	if mergedState.RunID == "" {
		t.Fatalf("merging issue should retain RunID")
	}
	if len(s.sem) != 1 {
		t.Fatalf("expected slot to stay occupied while merging, got %d", len(s.sem))
	}
	if _, ok := s.RunIndex[issueMerge.RunID]; !ok {
		t.Fatalf("expected RunIndex to retain running merge Run")
	}
	nextIssue := waitIssueStatus(t, store, "issue-next", core.IssueStatusReady, 2*time.Second)
	if nextIssue.RunID != "" {
		t.Fatalf("next issue should not dispatch while merge slot occupied, got Run=%q", nextIssue.RunID)
	}
	if _, ok := bus.FirstEvent(core.EventIssueMerging, "issue-merge"); !ok {
		t.Fatalf("expected EventIssueMerging to be published")
	}
}

func TestScheduler_IssueMergedReleasesSlotAndDispatchesNext(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-scheduler-issue-merged")
	autoMerge := newIssueWithProfile("issue-merge", "merge", core.WorkflowProfileStrict, nil)
	autoMerge.AutoMerge = true
	issues := mustCreateIssueSessionWithItems(t, store, project.ID, "session-issue-merged", core.FailBlock, []core.Issue{
		autoMerge,
		newIssueWithProfile("issue-next", "next", core.WorkflowProfileNormal, nil),
	})

	bus := &recordingSchedulerBus{}
	s := NewDepScheduler(store, bus, (&schedulerRunner{}).Run, nil, 1)
	if err := s.ScheduleIssues(context.Background(), issues); err != nil {
		t.Fatalf("ScheduleIssues() error = %v", err)
	}

	issueMerge := waitIssueStatus(t, store, "issue-merge", core.IssueStatusExecuting, 2*time.Second)
	if err := s.OnEvent(context.Background(), core.Event{
		Type:      core.EventRunDone,
		RunID:     issueMerge.RunID,
		Timestamp: time.Now(),
	}); err != nil {
		t.Fatalf("OnEvent(run_done merge) error = %v", err)
	}
	waitIssueStatus(t, store, "issue-merge", core.IssueStatusMerging, 2*time.Second)

	if err := s.OnEvent(context.Background(), core.Event{
		Type:      core.EventIssueMerged,
		IssueID:   "issue-merge",
		Timestamp: time.Now(),
	}); err != nil {
		t.Fatalf("OnEvent(issue_merged) error = %v", err)
	}

	waitIssueStatus(t, store, "issue-merge", core.IssueStatusDone, 2*time.Second)
	nextIssue := waitIssueStatus(t, store, "issue-next", core.IssueStatusExecuting, 2*time.Second)
	if nextIssue.RunID == "" {
		t.Fatalf("next issue should dispatch after merge success")
	}
	if len(s.sem) != 1 {
		t.Fatalf("expected one running issue after dispatching next, got %d", len(s.sem))
	}
	if _, ok := s.RunIndex[issueMerge.RunID]; ok {
		t.Fatalf("expected old merge RunIndex entry to be removed")
	}
	if _, ok := bus.FirstEvent(core.EventIssueDone, "issue-merge"); !ok {
		t.Fatalf("expected EventIssueDone for merged issue")
	}
}

func TestScheduler_IssueMergeConflictKeepsMergingAndSlot(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-scheduler-merge-conflict")
	autoMerge := newIssueWithProfile("issue-merge", "merge", core.WorkflowProfileStrict, nil)
	autoMerge.AutoMerge = true
	issues := mustCreateIssueSessionWithItems(t, store, project.ID, "session-merge-conflict", core.FailBlock, []core.Issue{
		autoMerge,
		newIssueWithProfile("issue-next", "next", core.WorkflowProfileNormal, nil),
	})

	s := NewDepScheduler(store, nil, (&schedulerRunner{}).Run, nil, 1)
	if err := s.ScheduleIssues(context.Background(), issues); err != nil {
		t.Fatalf("ScheduleIssues() error = %v", err)
	}

	issueMerge := waitIssueStatus(t, store, "issue-merge", core.IssueStatusExecuting, 2*time.Second)
	if err := s.OnEvent(context.Background(), core.Event{
		Type:      core.EventRunDone,
		RunID:     issueMerge.RunID,
		Timestamp: time.Now(),
	}); err != nil {
		t.Fatalf("OnEvent(run_done merge) error = %v", err)
	}
	waitIssueStatus(t, store, "issue-merge", core.IssueStatusMerging, 2*time.Second)

	if err := s.OnEvent(context.Background(), core.Event{
		Type:      core.EventIssueMergeConflict,
		IssueID:   "issue-merge",
		Timestamp: time.Now(),
	}); err != nil {
		t.Fatalf("OnEvent(issue_merge_conflict) error = %v", err)
	}

	conflictIssue := waitIssueStatus(t, store, "issue-merge", core.IssueStatusMerging, 2*time.Second)
	if conflictIssue.RunID == "" {
		t.Fatalf("conflict issue should retain RunID while waiting triage")
	}
	if len(s.sem) != 1 {
		t.Fatalf("expected slot occupied on merge conflict, got %d", len(s.sem))
	}
	nextIssue := waitIssueStatus(t, store, "issue-next", core.IssueStatusReady, 2*time.Second)
	if nextIssue.RunID != "" {
		t.Fatalf("next issue should remain pending while merge conflict unresolved")
	}
}

func TestScheduler_IssueMergeRetryReleasesOldRunAndRedispatches(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-scheduler-merge-retry")
	autoMerge := newIssueWithProfile("issue-merge", "merge", core.WorkflowProfileStrict, nil)
	autoMerge.AutoMerge = true
	issues := mustCreateIssueSessionWithItems(t, store, project.ID, "session-merge-retry", core.FailBlock, []core.Issue{
		autoMerge,
		newIssueWithProfile("issue-next", "next", core.WorkflowProfileNormal, nil),
	})

	s := NewDepScheduler(store, nil, (&schedulerRunner{}).Run, nil, 1)
	if err := s.ScheduleIssues(context.Background(), issues); err != nil {
		t.Fatalf("ScheduleIssues() error = %v", err)
	}

	issueMerge := waitIssueStatus(t, store, "issue-merge", core.IssueStatusExecuting, 2*time.Second)
	originalRunID := issueMerge.RunID
	if err := s.OnEvent(context.Background(), core.Event{
		Type:      core.EventRunDone,
		RunID:     originalRunID,
		Timestamp: time.Now(),
	}); err != nil {
		t.Fatalf("OnEvent(run_done merge) error = %v", err)
	}
	waitIssueStatus(t, store, "issue-merge", core.IssueStatusMerging, 2*time.Second)

	queued := waitIssueStatus(t, store, "issue-merge", core.IssueStatusMerging, 2*time.Second)
	queued.Status = core.IssueStatusQueued
	queued.RunID = ""
	if err := store.SaveIssue(queued); err != nil {
		t.Fatalf("SaveIssue(queued retry) error = %v", err)
	}

	if err := s.OnEvent(context.Background(), core.Event{
		Type:      core.EventIssueMergeRetry,
		IssueID:   "issue-merge",
		Timestamp: time.Now(),
	}); err != nil {
		t.Fatalf("OnEvent(issue_merge_retry) error = %v", err)
	}

	retried := waitIssueStatus(t, store, "issue-merge", core.IssueStatusExecuting, 2*time.Second)
	if retried.RunID == "" || retried.RunID == originalRunID {
		t.Fatalf("expected re-dispatched RunID, got %q (original %q)", retried.RunID, originalRunID)
	}
	if _, ok := s.RunIndex[originalRunID]; ok {
		t.Fatalf("expected old merge RunIndex entry to be removed after retry")
	}
}

func TestScheduler_FailPolicyBlockSkipsMergingIssue(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-scheduler-block-skip-merging")
	autoMerge := newIssueWithProfile("issue-merge", "merge", core.WorkflowProfileStrict, nil)
	autoMerge.AutoMerge = true
	issues := mustCreateIssueSessionWithItems(t, store, project.ID, "session-block-skip-merging", core.FailBlock, []core.Issue{
		autoMerge,
		newIssueWithProfile("issue-fail", "fail", core.WorkflowProfileNormal, nil),
	})

	s := NewDepScheduler(store, nil, (&schedulerRunner{}).Run, nil, 2)
	if err := s.ScheduleIssues(context.Background(), issues); err != nil {
		t.Fatalf("ScheduleIssues() error = %v", err)
	}

	mergeIssue := waitIssueStatus(t, store, "issue-merge", core.IssueStatusExecuting, 2*time.Second)
	failIssue := waitIssueStatus(t, store, "issue-fail", core.IssueStatusExecuting, 2*time.Second)

	if err := s.OnEvent(context.Background(), core.Event{
		Type:      core.EventRunDone,
		RunID:     mergeIssue.RunID,
		Timestamp: time.Now(),
	}); err != nil {
		t.Fatalf("OnEvent(run_done merge) error = %v", err)
	}
	merging := waitIssueStatus(t, store, "issue-merge", core.IssueStatusMerging, 2*time.Second)

	if err := s.OnEvent(context.Background(), core.Event{
		Type:      core.EventRunFailed,
		RunID:     failIssue.RunID,
		Error:     "boom",
		Timestamp: time.Now(),
	}); err != nil {
		t.Fatalf("OnEvent(run_failed fail) error = %v", err)
	}
	waitIssueStatus(t, store, "issue-fail", core.IssueStatusFailed, 2*time.Second)

	after := waitIssueStatus(t, store, "issue-merge", core.IssueStatusMerging, 2*time.Second)
	if after.RunID == "" || after.RunID != merging.RunID {
		t.Fatalf("merging issue should keep RunID after block policy, got %q", after.RunID)
	}
}

func TestScheduler_RecoverExecutingIssuesKeepsMergingAsRunning(t *testing.T) {
	store := newSchedulerTestStore(t)
	defer store.Close()

	project := mustCreateSchedulerProject(t, store, "proj-scheduler-recover-merging")
	mustCreateIssueSessionWithItems(t, store, project.ID, "session-recover-merging", core.FailBlock, []core.Issue{
		{
			ID:        "issue-merge",
			Title:     "merge",
			Body:      "merge",
			Status:    core.IssueStatusMerging,
			RunID:     "Run-recover-merging",
			Template:  "strict",
			Labels:    []string{"profile:strict"},
			AutoMerge: true,
		},
	})

	if err := store.SaveRun(&core.Run{
		ID:         "Run-recover-merging",
		ProjectID:  project.ID,
		Name:       "Run-recover-merging",
		Status:     core.StatusInProgress,
		Conclusion: "",
		IssueID:    "issue-merge",
	}); err != nil {
		t.Fatalf("SaveRun(in_progress) error = %v", err)
	}

	s := NewDepScheduler(store, nil, (&schedulerRunner{}).Run, nil, 1)
	if err := s.RecoverExecutingIssues(context.Background(), project.ID); err != nil {
		t.Fatalf("RecoverExecutingIssues() error = %v", err)
	}

	recovered := waitIssueStatus(t, store, "issue-merge", core.IssueStatusMerging, 2*time.Second)
	if recovered.RunID != "Run-recover-merging" {
		t.Fatalf("expected merge RunID preserved, got %q", recovered.RunID)
	}
	if _, ok := s.RunIndex["Run-recover-merging"]; !ok {
		t.Fatalf("expected recovered merging Run to remain indexed")
	}
	if len(s.sem) != 1 {
		t.Fatalf("expected recovered merging issue to occupy one slot, got %d", len(s.sem))
	}
}

func TestBuildRunFromIssue_AddsMergeConflictHintOnRetry(t *testing.T) {
	issue := &core.Issue{
		ID:           "issue-hint-retry",
		ProjectID:    "proj-hint-retry",
		Title:        "hint retry",
		Body:         "hint retry body",
		Template:     "standard",
		MergeRetries: 1,
	}

	run, err := buildRunFromIssue(issue, core.WorkflowProfileStrict, nil)
	if err != nil {
		t.Fatalf("buildRunFromIssue() error = %v", err)
	}
	if run.Config == nil {
		t.Fatalf("run config should not be nil")
	}
	hint, _ := run.Config["merge_conflict_hint"].(string)
	if strings.TrimSpace(hint) == "" {
		t.Fatalf("expected merge_conflict_hint in run config")
	}
}

func TestBuildRunFromIssue_NoMergeConflictHintWithoutRetry(t *testing.T) {
	issue := &core.Issue{
		ID:        "issue-hint-none",
		ProjectID: "proj-hint-none",
		Title:     "hint none",
		Body:      "hint none body",
		Template:  "standard",
	}

	run, err := buildRunFromIssue(issue, core.WorkflowProfileStrict, nil)
	if err != nil {
		t.Fatalf("buildRunFromIssue() error = %v", err)
	}
	if run.Config == nil {
		t.Fatalf("run config should not be nil")
	}
	if _, ok := run.Config["merge_conflict_hint"]; ok {
		t.Fatalf("merge_conflict_hint should not be present when MergeRetries=0")
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

func (b *recordingSchedulerBus) Subscribe(_ ...core.SubOption) (*core.Subscription, error) {
	ch := make(chan core.Event, 1)
	return &core.Subscription{
		C:        ch,
		CancelFn: func() { close(ch) },
	}, nil
}

func (b *recordingSchedulerBus) Close() error {
	return nil
}

func (b *recordingSchedulerBus) Publish(_ context.Context, evt core.Event) error {
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
	return nil
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
