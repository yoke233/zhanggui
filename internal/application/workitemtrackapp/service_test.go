package workitemtrackapp

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/yoke233/ai-workflow/internal/adapters/store/sqlite"
	"github.com/yoke233/ai-workflow/internal/core"
)

type sqliteTxAdapter struct {
	base core.TransactionalStore
}

func (a sqliteTxAdapter) InTx(ctx context.Context, fn func(ctx context.Context, store TxStore) error) error {
	return a.base.InTx(ctx, func(store core.Store) error {
		txStore, ok := store.(TxStore)
		if !ok {
			return fmt.Errorf("unexpected tx store type %T", store)
		}
		return fn(ctx, txStore)
	})
}

func newWorkItemTrackAppTestStore(t *testing.T) *sqlite.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "workitemtrackapp-test.db")
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func newSQLiteWorkItemTrackService(store Store, tx Tx) *Service {
	return newSQLiteWorkItemTrackServiceWithDeps(store, tx, nil, nil)
}

func newSQLiteWorkItemTrackServiceWithDeps(store Store, tx Tx, bus EventPublisher, executor WorkItemExecutor) *Service {
	return New(Config{
		Store:    store,
		Tx:       tx,
		Bus:      bus,
		Executor: executor,
	})
}

type recordingBus struct {
	events []core.Event
}

func (b *recordingBus) Publish(_ context.Context, event core.Event) {
	b.events = append(b.events, event)
}

type queuedExecutor struct {
	store *sqlite.Store
	calls []int64
}

func (e *queuedExecutor) RunWorkItem(ctx context.Context, workItemID int64) error {
	e.calls = append(e.calls, workItemID)
	if e.store != nil {
		return e.store.UpdateWorkItemStatus(ctx, workItemID, core.WorkItemQueued)
	}
	return nil
}

func TestServiceStartTrack(t *testing.T) {
	store := newWorkItemTrackAppTestStore(t)
	ctx := context.Background()

	threadID, err := store.CreateThread(ctx, &core.Thread{Title: "thread-a"})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}

	svc := newSQLiteWorkItemTrackService(store, sqliteTxAdapter{base: store})
	track, err := svc.StartTrack(ctx, StartTrackInput{
		ThreadID:  threadID,
		Title:     "track alpha",
		Objective: "ship Phase 1",
		CreatedBy: "user-1",
		Metadata:  map[string]any{"source": "manual"},
	})
	if err != nil {
		t.Fatalf("StartTrack: %v", err)
	}
	if track.ID <= 0 || track.PrimaryThreadID == nil || *track.PrimaryThreadID != threadID {
		t.Fatalf("unexpected track: %+v", track)
	}

	links, err := store.ListWorkItemTrackThreads(ctx, track.ID)
	if err != nil {
		t.Fatalf("ListWorkItemTrackThreads: %v", err)
	}
	if len(links) != 1 || links[0].RelationType != core.WorkItemTrackThreadPrimary {
		t.Fatalf("unexpected track links: %+v", links)
	}
}

func TestServiceAttachThreadContext(t *testing.T) {
	store := newWorkItemTrackAppTestStore(t)
	ctx := context.Background()

	threadA, _ := store.CreateThread(ctx, &core.Thread{Title: "thread-a"})
	threadB, _ := store.CreateThread(ctx, &core.Thread{Title: "thread-b"})
	trackID, _ := store.CreateWorkItemTrack(ctx, &core.WorkItemTrack{
		Title:           "track beta",
		Status:          core.WorkItemTrackDraft,
		PrimaryThreadID: &threadA,
	})
	if _, err := store.AttachThreadToWorkItemTrack(ctx, &core.WorkItemTrackThread{
		TrackID:      trackID,
		ThreadID:     threadA,
		RelationType: core.WorkItemTrackThreadPrimary,
	}); err != nil {
		t.Fatalf("attach primary: %v", err)
	}

	svc := newSQLiteWorkItemTrackService(store, sqliteTxAdapter{base: store})
	link, err := svc.AttachThreadContext(ctx, AttachThreadContextInput{
		TrackID:      trackID,
		ThreadID:     threadB,
		RelationType: "context",
	})
	if err != nil {
		t.Fatalf("AttachThreadContext: %v", err)
	}
	if link.ID <= 0 || link.RelationType != core.WorkItemTrackThreadContext {
		t.Fatalf("unexpected track thread link: %+v", link)
	}
}

func TestServiceMaterializeWorkItem(t *testing.T) {
	store := newWorkItemTrackAppTestStore(t)
	ctx := context.Background()

	threadA, _ := store.CreateThread(ctx, &core.Thread{Title: "thread-a"})
	threadB, _ := store.CreateThread(ctx, &core.Thread{Title: "thread-b"})
	track := &core.WorkItemTrack{
		Title:           "track gamma",
		Objective:       "turn discussion into work item",
		Status:          core.WorkItemTrackAwaitingConfirmation,
		PrimaryThreadID: &threadA,
	}
	trackID, err := store.CreateWorkItemTrack(ctx, track)
	if err != nil {
		t.Fatalf("create track: %v", err)
	}
	if _, err := store.AttachThreadToWorkItemTrack(ctx, &core.WorkItemTrackThread{
		TrackID:      trackID,
		ThreadID:     threadA,
		RelationType: core.WorkItemTrackThreadPrimary,
	}); err != nil {
		t.Fatalf("attach primary: %v", err)
	}
	if _, err := store.AttachThreadToWorkItemTrack(ctx, &core.WorkItemTrackThread{
		TrackID:      trackID,
		ThreadID:     threadB,
		RelationType: core.WorkItemTrackThreadSource,
	}); err != nil {
		t.Fatalf("attach source: %v", err)
	}

	svc := newSQLiteWorkItemTrackService(store, sqliteTxAdapter{base: store})
	result, err := svc.MaterializeWorkItem(ctx, MaterializeWorkItemInput{TrackID: trackID})
	if err != nil {
		t.Fatalf("MaterializeWorkItem: %v", err)
	}
	if result.WorkItem == nil || result.WorkItem.ID <= 0 {
		t.Fatalf("expected persisted work item, got %+v", result.WorkItem)
	}
	if result.Track.WorkItemID == nil || *result.Track.WorkItemID != result.WorkItem.ID {
		t.Fatalf("expected track to point at work item, got %+v", result.Track)
	}
	if result.Track.Status != core.WorkItemTrackMaterialized || result.WorkItem.Status != core.WorkItemAccepted {
		t.Fatalf("unexpected materialized state: track=%+v workItem=%+v", result.Track, result.WorkItem)
	}
	if len(result.Links) != 2 {
		t.Fatalf("expected 2 thread-work item links, got %d", len(result.Links))
	}

	byThread, err := store.ListWorkItemsByThread(ctx, threadA)
	if err != nil {
		t.Fatalf("ListWorkItemsByThread(primary): %v", err)
	}
	if len(byThread) != 1 || !byThread[0].IsPrimary {
		t.Fatalf("expected primary thread link, got %+v", byThread)
	}

	again, err := svc.MaterializeWorkItem(ctx, MaterializeWorkItemInput{TrackID: trackID})
	if err != nil {
		t.Fatalf("MaterializeWorkItem(idempotent): %v", err)
	}
	if again.WorkItem.ID != result.WorkItem.ID {
		t.Fatalf("expected idempotent materialize to reuse work item %d, got %d", result.WorkItem.ID, again.WorkItem.ID)
	}
}

func TestServiceMaterializeRejectsInvalidState(t *testing.T) {
	store := newWorkItemTrackAppTestStore(t)
	ctx := context.Background()

	threadID, _ := store.CreateThread(ctx, &core.Thread{Title: "thread-a"})
	trackID, _ := store.CreateWorkItemTrack(ctx, &core.WorkItemTrack{
		Title:           "track-delta",
		Status:          core.WorkItemTrackExecuting,
		PrimaryThreadID: &threadID,
	})
	if _, err := store.AttachThreadToWorkItemTrack(ctx, &core.WorkItemTrackThread{
		TrackID:      trackID,
		ThreadID:     threadID,
		RelationType: core.WorkItemTrackThreadPrimary,
	}); err != nil {
		t.Fatalf("attach primary: %v", err)
	}

	svc := newSQLiteWorkItemTrackService(store, sqliteTxAdapter{base: store})
	_, err := svc.MaterializeWorkItem(ctx, MaterializeWorkItemInput{TrackID: trackID})
	if CodeOf(err) != CodeInvalidState {
		t.Fatalf("expected invalid state error, got %v", err)
	}
}

func TestServiceReviewLifecycle(t *testing.T) {
	store := newWorkItemTrackAppTestStore(t)
	ctx := context.Background()
	bus := &recordingBus{}

	threadID, err := store.CreateThread(ctx, &core.Thread{Title: "thread-a"})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}

	svc := newSQLiteWorkItemTrackServiceWithDeps(store, sqliteTxAdapter{base: store}, bus, nil)
	track, err := svc.StartTrack(ctx, StartTrackInput{
		ThreadID:  threadID,
		Title:     "track-review",
		Objective: "review workflow",
	})
	if err != nil {
		t.Fatalf("StartTrack: %v", err)
	}

	track, err = svc.SubmitForReview(ctx, SubmitForReviewInput{
		TrackID:       track.ID,
		LatestSummary: "planner done",
		PlannerOutput: map[string]any{"plan": "ok"},
	})
	if err != nil {
		t.Fatalf("SubmitForReview: %v", err)
	}
	if track.Status != core.WorkItemTrackReviewing || track.ReviewerStatus != "pending" {
		t.Fatalf("unexpected review track: %+v", track)
	}

	approved, err := svc.ApproveReview(ctx, ApproveReviewInput{
		TrackID:       track.ID,
		LatestSummary: "ready for user",
		ReviewOutput:  map[string]any{"verdict": "approved"},
	})
	if err != nil {
		t.Fatalf("ApproveReview: %v", err)
	}
	if approved.Status != core.WorkItemTrackAwaitingConfirmation || !approved.AwaitingUserConfirmation {
		t.Fatalf("unexpected approved track: %+v", approved)
	}

	rejectedSource := &core.WorkItemTrack{
		Title:           "track-reject",
		Status:          core.WorkItemTrackReviewing,
		PrimaryThreadID: &threadID,
	}
	rejectedID, err := store.CreateWorkItemTrack(ctx, rejectedSource)
	if err != nil {
		t.Fatalf("create reject track: %v", err)
	}
	if _, err := store.AttachThreadToWorkItemTrack(ctx, &core.WorkItemTrackThread{
		TrackID:      rejectedID,
		ThreadID:     threadID,
		RelationType: core.WorkItemTrackThreadPrimary,
	}); err != nil {
		t.Fatalf("attach reject track: %v", err)
	}

	rejected, err := svc.RejectReview(ctx, RejectReviewInput{
		TrackID:       rejectedID,
		LatestSummary: "needs more work",
		ReviewOutput:  map[string]any{"verdict": "rejected"},
	})
	if err != nil {
		t.Fatalf("RejectReview: %v", err)
	}
	if rejected.Status != core.WorkItemTrackPlanning || rejected.ReviewerStatus != "rejected" {
		t.Fatalf("unexpected rejected track: %+v", rejected)
	}

	if len(bus.events) < 5 {
		t.Fatalf("expected track events to be published, got %d", len(bus.events))
	}
}

func TestServiceConfirmExecutionCreatesDefaultAction(t *testing.T) {
	store := newWorkItemTrackAppTestStore(t)
	ctx := context.Background()
	bus := &recordingBus{}
	executor := &queuedExecutor{store: store}

	threadID, err := store.CreateThread(ctx, &core.Thread{Title: "thread-a"})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	trackID, err := store.CreateWorkItemTrack(ctx, &core.WorkItemTrack{
		Title:                    "track-exec",
		Objective:                "run this",
		Status:                   core.WorkItemTrackAwaitingConfirmation,
		PrimaryThreadID:          &threadID,
		AwaitingUserConfirmation: true,
	})
	if err != nil {
		t.Fatalf("create track: %v", err)
	}
	if _, err := store.AttachThreadToWorkItemTrack(ctx, &core.WorkItemTrackThread{
		TrackID:      trackID,
		ThreadID:     threadID,
		RelationType: core.WorkItemTrackThreadPrimary,
	}); err != nil {
		t.Fatalf("attach track: %v", err)
	}

	svc := newSQLiteWorkItemTrackServiceWithDeps(store, sqliteTxAdapter{base: store}, bus, executor)
	result, err := svc.ConfirmExecution(ctx, ConfirmExecutionInput{TrackID: trackID})
	if err != nil {
		t.Fatalf("ConfirmExecution: %v", err)
	}
	if result.Track.Status != core.WorkItemTrackExecuting {
		t.Fatalf("expected executing track, got %+v", result.Track)
	}
	if result.WorkItem.Status != core.WorkItemQueued {
		t.Fatalf("expected queued work item, got %+v", result.WorkItem)
	}
	if len(executor.calls) != 1 {
		t.Fatalf("expected executor to be called once, got %d", len(executor.calls))
	}

	actions, err := store.ListActionsByWorkItem(ctx, result.WorkItem.ID)
	if err != nil {
		t.Fatalf("ListActionsByWorkItem: %v", err)
	}
	if len(actions) != 1 || actions[0].Type != core.ActionExec {
		t.Fatalf("expected one default exec action, got %+v", actions)
	}
}

func TestServiceSyncTrackStatusFromWorkItem(t *testing.T) {
	store := newWorkItemTrackAppTestStore(t)
	ctx := context.Background()
	bus := &recordingBus{}

	threadID, err := store.CreateThread(ctx, &core.Thread{Title: "thread-a"})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	workItemID, err := store.CreateWorkItem(ctx, &core.WorkItem{
		Title:    "work item",
		Status:   core.WorkItemRunning,
		Priority: core.PriorityMedium,
	})
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}
	trackID, err := store.CreateWorkItemTrack(ctx, &core.WorkItemTrack{
		Title:           "track-sync",
		Status:          core.WorkItemTrackExecuting,
		PrimaryThreadID: &threadID,
		WorkItemID:      &workItemID,
	})
	if err != nil {
		t.Fatalf("create track: %v", err)
	}
	if _, err := store.AttachThreadToWorkItemTrack(ctx, &core.WorkItemTrackThread{
		TrackID:      trackID,
		ThreadID:     threadID,
		RelationType: core.WorkItemTrackThreadPrimary,
	}); err != nil {
		t.Fatalf("attach track: %v", err)
	}

	svc := newSQLiteWorkItemTrackServiceWithDeps(store, sqliteTxAdapter{base: store}, bus, nil)
	updated, err := svc.SyncTrackStatusFromWorkItem(ctx, workItemID, core.WorkItemDone)
	if err != nil {
		t.Fatalf("SyncTrackStatusFromWorkItem: %v", err)
	}
	if len(updated) != 1 || updated[0].Status != core.WorkItemTrackDone {
		t.Fatalf("unexpected synced tracks: %+v", updated)
	}
}
