package workitemapp

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/yoke233/zhanggui/internal/adapters/store/sqlite"
	"github.com/yoke233/zhanggui/internal/core"
)

type sqliteTxAdapter struct {
	base core.TransactionalStore
	wrap func(core.Store) (TxStore, error)
}

func (a sqliteTxAdapter) InTx(ctx context.Context, fn func(ctx context.Context, store TxStore) error) error {
	return a.base.InTx(ctx, func(store core.Store) error {
		txStore, err := a.bind(store)
		if err != nil {
			return err
		}
		return fn(ctx, txStore)
	})
}

func (a sqliteTxAdapter) bind(store core.Store) (TxStore, error) {
	if a.wrap != nil {
		return a.wrap(store)
	}
	txStore, ok := store.(TxStore)
	if !ok {
		return nil, fmt.Errorf("unexpected tx store type %T", store)
	}
	return txStore, nil
}

type bootstrapStub struct {
	err          error
	calls        int
	lastWorkItem int64
}

type runnerStub struct {
	run func(context.Context, int64) error
}

type schedulerStub struct {
	submit func(context.Context, int64) error
}

func (r *runnerStub) Run(ctx context.Context, workItemID int64) error {
	if r.run != nil {
		return r.run(ctx, workItemID)
	}
	return nil
}

func (r *runnerStub) Cancel(context.Context, int64) error { return nil }

func (s *schedulerStub) Submit(ctx context.Context, workItemID int64) error {
	if s.submit != nil {
		return s.submit(ctx, workItemID)
	}
	return nil
}

func (s *schedulerStub) Cancel(context.Context, int64) error { return nil }

func (b *bootstrapStub) BootstrapPRWorkItem(_ context.Context, workItemID int64) error {
	b.calls++
	b.lastWorkItem = workItemID
	return b.err
}

type failingDeleteStore struct {
	*sqlite.Store
	failDeleteRuns bool
}

func (s *failingDeleteStore) DeleteRunsByWorkItem(ctx context.Context, workItemID int64) error {
	if s.failDeleteRuns {
		return errors.New("delete runs failed")
	}
	return s.Store.DeleteRunsByWorkItem(ctx, workItemID)
}

type workItemInitiativeMembershipStore struct {
	*sqlite.Store
	byWorkItem map[int64][]*core.InitiativeItem
}

func (s *workItemInitiativeMembershipStore) ListInitiativeItemsByWorkItem(_ context.Context, workItemID int64) ([]*core.InitiativeItem, error) {
	items := s.byWorkItem[workItemID]
	out := make([]*core.InitiativeItem, len(items))
	copy(out, items)
	return out, nil
}

func newWorkItemAppTestStore(t *testing.T) *sqlite.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "workitemapp-test.db")
	store, err := sqlite.New(dbPath)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func newSQLiteWorkItemService(store Store, tx Tx, bootstrap Bootstrapper) *Service {
	return New(Config{
		Store:       store,
		Tx:          tx,
		BootstrapPR: bootstrap,
	})
}

func newSQLiteTxAdapter(store *sqlite.Store, wrap func(core.Store) (TxStore, error)) Tx {
	return sqliteTxAdapter{
		base: store,
		wrap: wrap,
	}
}

func createWorkItemFixture(t *testing.T, store *sqlite.Store) (workItemID int64, actionID int64, featureID int64) {
	t.Helper()
	ctx := context.Background()

	projectID, err := store.CreateProject(ctx, &core.Project{Name: "fixture-project"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	workItemID, err = store.CreateWorkItem(ctx, &core.WorkItem{
		ProjectID: &projectID,
		Title:     "fixture-work-item",
		Body:      "fixture body",
		Status:    core.WorkItemOpen,
		Priority:  core.PriorityMedium,
	})
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}
	actionID, err = store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID,
		Name:       "exec-action",
		Type:       core.ActionExec,
		Status:     core.ActionPending,
		Position:   0,
	})
	if err != nil {
		t.Fatalf("create action: %v", err)
	}
	if _, err := store.CreateRun(ctx, &core.Run{
		ActionID:   actionID,
		WorkItemID: workItemID,
		Status:     core.RunCreated,
		Attempt:    1,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, err := store.CreateActionSignal(ctx, &core.ActionSignal{
		ActionID:   actionID,
		WorkItemID: workItemID,
		Type:       core.SignalComplete,
		Source:     core.SignalSourceAgent,
		Summary:    "done",
	}); err != nil {
		t.Fatalf("create action signal: %v", err)
	}
	if _, err := store.CreateAgentContext(ctx, &core.AgentContext{
		AgentID:      "codex",
		WorkItemID:   workItemID,
		SystemPrompt: "fixture",
	}); err != nil {
		t.Fatalf("create agent context: %v", err)
	}
	spaceID, err := store.CreateResourceSpace(ctx, &core.ResourceSpace{
		ProjectID: projectID,
		Kind:      core.ResourceKindLocalFS,
		RootURI:   "D:/tmp",
		Label:     "fixture-space",
	})
	if err != nil {
		t.Fatalf("create resource space: %v", err)
	}
	if _, err := store.CreateResource(ctx, &core.Resource{
		ProjectID:   projectID,
		WorkItemID:  &workItemID,
		StorageKind: "local",
		URI:         "D:/tmp/fixture.txt",
		Role:        "attachment",
		FileName:    "fixture.txt",
	}); err != nil {
		t.Fatalf("create resource: %v", err)
	}
	if _, err := store.CreateActionIODecl(ctx, &core.ActionIODecl{
		ActionID:  actionID,
		Direction: core.IOInput,
		SpaceID:   &spaceID,
		Path:      "fixture.txt",
		Required:  true,
	}); err != nil {
		t.Fatalf("create action io decl: %v", err)
	}
	threadID, err := store.CreateThread(ctx, &core.Thread{Title: "fixture-thread", Status: core.ThreadActive})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}
	if _, err := store.CreateThreadWorkItemLink(ctx, &core.ThreadWorkItemLink{
		ThreadID:     threadID,
		WorkItemID:   workItemID,
		RelationType: "related",
		IsPrimary:    true,
	}); err != nil {
		t.Fatalf("create thread link: %v", err)
	}
	if _, err := store.CreateEvent(ctx, &core.Event{
		Type:       core.EventWorkItemStarted,
		WorkItemID: workItemID,
	}); err != nil {
		t.Fatalf("create event: %v", err)
	}
	if _, err := store.AppendJournal(ctx, &core.JournalEntry{
		WorkItemID: workItemID,
		Kind:       core.JournalSystem,
		Source:     core.JournalSourceSystem,
		Summary:    "fixture",
	}); err != nil {
		t.Fatalf("append journal: %v", err)
	}
	featureID, err = store.CreateFeatureEntry(ctx, &core.FeatureEntry{
		ProjectID:   projectID,
		Key:         "fixture-feature",
		Description: "fixture feature",
		Status:      core.FeaturePending,
		WorkItemID:  &workItemID,
		ActionID:    &actionID,
	})
	if err != nil {
		t.Fatalf("create feature entry: %v", err)
	}
	return workItemID, actionID, featureID
}

func TestServiceCreateWorkItemPersistsDependsOnAndRollsBackOnBootstrapFailure(t *testing.T) {
	store := newWorkItemAppTestStore(t)
	ctx := context.Background()

	projectID, err := store.CreateProject(ctx, &core.Project{Name: "project-1"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	dependencyID, err := store.CreateWorkItem(ctx, &core.WorkItem{
		ProjectID: &projectID,
		Title:     "dependency",
		Status:    core.WorkItemOpen,
		Priority:  core.PriorityMedium,
	})
	if err != nil {
		t.Fatalf("create dependency: %v", err)
	}

	bootstrap := &bootstrapStub{err: errors.New("bootstrap failed")}
	svc := newSQLiteWorkItemService(store, newSQLiteTxAdapter(store, nil), bootstrap)

	_, err = svc.CreateWorkItem(ctx, CreateWorkItemInput{
		ProjectID: projectIDPtr(projectID),
		Title:     "new work item",
		DependsOn: []int64{dependencyID},
	})
	if CodeOf(err) != CodeBootstrapPRFailed {
		t.Fatalf("expected %s, got %v", CodeBootstrapPRFailed, err)
	}
	if bootstrap.calls != 1 {
		t.Fatalf("expected bootstrap to be called once, got %d", bootstrap.calls)
	}

	items, err := store.ListWorkItems(ctx, core.WorkItemFilter{Limit: 20})
	if err != nil {
		t.Fatalf("list work items: %v", err)
	}
	if len(items) != 1 || items[0].ID != dependencyID {
		t.Fatalf("expected only dependency to remain after rollback, got %+v", items)
	}

	bootstrap.err = nil
	result, err := svc.CreateWorkItem(ctx, CreateWorkItemInput{
		ProjectID: projectIDPtr(projectID),
		Title:     "successful work item",
		DependsOn: []int64{dependencyID},
	})
	if err != nil {
		t.Fatalf("CreateWorkItem: %v", err)
	}
	if len(result.DependsOn) != 1 || result.DependsOn[0] != dependencyID {
		t.Fatalf("expected depends_on to persist, got %+v", result.DependsOn)
	}

	persisted, err := store.GetWorkItem(ctx, result.ID)
	if err != nil {
		t.Fatalf("get persisted work item: %v", err)
	}
	if len(persisted.DependsOn) != 1 || persisted.DependsOn[0] != dependencyID {
		t.Fatalf("expected persisted depends_on, got %+v", persisted.DependsOn)
	}
}

func TestServiceCreateWorkItemPersistsActiveProfileAndFinalDeliverable(t *testing.T) {
	store := newWorkItemAppTestStore(t)
	svc := newSQLiteWorkItemService(store, newSQLiteTxAdapter(store, nil), nil)
	ctx := context.Background()

	finalDeliverableID := int64(9)
	parentWorkItemID := int64(5)
	rootWorkItemID := int64(1)

	item, err := svc.CreateWorkItem(ctx, CreateWorkItemInput{
		Title:              "Implement login",
		ExecutorProfileID:  "lead",
		ReviewerProfileID:  "ceo",
		ActiveProfileID:    "lead",
		SponsorProfileID:   "ceo",
		CreatedByProfileID: "ceo",
		ParentWorkItemID:   &parentWorkItemID,
		RootWorkItemID:     &rootWorkItemID,
		FinalDeliverableID: &finalDeliverableID,
		EscalationPath:     []string{"lead", "ceo"},
		Metadata:           map[string]any{"source": "task3"},
	})
	if err != nil {
		t.Fatalf("CreateWorkItem: %v", err)
	}
	if item.ActiveProfileID != "lead" {
		t.Fatalf("ActiveProfileID = %q, want lead", item.ActiveProfileID)
	}
	if item.FinalDeliverableID == nil || *item.FinalDeliverableID != finalDeliverableID {
		t.Fatalf("FinalDeliverableID = %v, want %d", item.FinalDeliverableID, finalDeliverableID)
	}
	if len(item.EscalationPath) != 2 || item.EscalationPath[1] != "ceo" {
		t.Fatalf("EscalationPath = %+v", item.EscalationPath)
	}

	persisted, err := store.GetWorkItem(ctx, item.ID)
	if err != nil {
		t.Fatalf("GetWorkItem: %v", err)
	}
	if persisted.ExecutorProfileID != "lead" || persisted.ReviewerProfileID != "ceo" {
		t.Fatalf("executor/reviewer not persisted: %+v", persisted)
	}
	if persisted.ParentWorkItemID == nil || *persisted.ParentWorkItemID != parentWorkItemID {
		t.Fatalf("ParentWorkItemID = %v, want %d", persisted.ParentWorkItemID, parentWorkItemID)
	}
	if persisted.RootWorkItemID == nil || *persisted.RootWorkItemID != rootWorkItemID {
		t.Fatalf("RootWorkItemID = %v, want %d", persisted.RootWorkItemID, rootWorkItemID)
	}
}

func TestServiceAdoptDeliverableSetsFinalDeliverableID(t *testing.T) {
	store := newWorkItemAppTestStore(t)
	svc := newSQLiteWorkItemService(store, newSQLiteTxAdapter(store, nil), nil)
	ctx := context.Background()

	workItemID, err := store.CreateWorkItem(ctx, &core.WorkItem{
		Title:    "Adopt deliverable",
		Status:   core.WorkItemOpen,
		Priority: core.PriorityMedium,
	})
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}

	threadID, err := store.CreateThread(ctx, &core.Thread{
		Title:  "artifact-thread",
		Status: core.ThreadActive,
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}

	deliverableID, err := store.CreateDeliverable(ctx, &core.Deliverable{
		ThreadID:     &threadID,
		Kind:         core.DeliverableDocument,
		Title:        "Login Flow Design",
		Summary:      "thread deliverable",
		ProducerType: core.DeliverableProducerThread,
		ProducerID:   threadID,
		Status:       core.DeliverableFinal,
	})
	if err != nil {
		t.Fatalf("create deliverable: %v", err)
	}

	item, err := svc.AdoptDeliverable(ctx, workItemID, deliverableID)
	if err != nil {
		t.Fatalf("AdoptDeliverable: %v", err)
	}
	if item.FinalDeliverableID == nil || *item.FinalDeliverableID != deliverableID {
		t.Fatalf("FinalDeliverableID = %v, want %d", item.FinalDeliverableID, deliverableID)
	}

	persisted, err := store.GetWorkItem(ctx, workItemID)
	if err != nil {
		t.Fatalf("GetWorkItem: %v", err)
	}
	if persisted.FinalDeliverableID == nil || *persisted.FinalDeliverableID != deliverableID {
		t.Fatalf("persisted FinalDeliverableID = %v, want %d", persisted.FinalDeliverableID, deliverableID)
	}
}

func TestServiceListDeliverablesIncludesAdoptedFinalDeliverable(t *testing.T) {
	store := newWorkItemAppTestStore(t)
	svc := newSQLiteWorkItemService(store, newSQLiteTxAdapter(store, nil), nil)
	ctx := context.Background()

	workItemID, err := store.CreateWorkItem(ctx, &core.WorkItem{
		Title:    "List deliverables",
		Status:   core.WorkItemOpen,
		Priority: core.PriorityMedium,
	})
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}

	threadID, err := store.CreateThread(ctx, &core.Thread{
		Title:  "deliverable-thread",
		Status: core.ThreadActive,
	})
	if err != nil {
		t.Fatalf("create thread: %v", err)
	}

	adoptedID, err := store.CreateDeliverable(ctx, &core.Deliverable{
		ThreadID:     &threadID,
		Kind:         core.DeliverableDocument,
		Title:        "Adopted Design",
		Summary:      "from thread",
		ProducerType: core.DeliverableProducerThread,
		ProducerID:   threadID,
		Status:       core.DeliverableFinal,
	})
	if err != nil {
		t.Fatalf("create adopted deliverable: %v", err)
	}

	ownID, err := store.CreateDeliverable(ctx, &core.Deliverable{
		WorkItemID:   &workItemID,
		Kind:         core.DeliverableCodeChange,
		Title:        "Implementation Patch",
		Summary:      "from work item",
		ProducerType: core.DeliverableProducerWorkItem,
		ProducerID:   workItemID,
		Status:       core.DeliverableFinal,
	})
	if err != nil {
		t.Fatalf("create own deliverable: %v", err)
	}

	if _, err := svc.AdoptDeliverable(ctx, workItemID, adoptedID); err != nil {
		t.Fatalf("AdoptDeliverable: %v", err)
	}

	items, err := svc.ListDeliverables(ctx, workItemID)
	if err != nil {
		t.Fatalf("ListDeliverables: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("ListDeliverables len = %d, want 2", len(items))
	}
	if items[0].ID != adoptedID {
		t.Fatalf("first deliverable id = %d, want adopted %d", items[0].ID, adoptedID)
	}
	if items[1].ID != ownID {
		t.Fatalf("second deliverable id = %d, want own %d", items[1].ID, ownID)
	}
}

func TestServiceUpdateWorkItemAllowsCrossProjectDependenciesInsideSameInitiative(t *testing.T) {
	store := newWorkItemAppTestStore(t)
	ctx := context.Background()

	projectA, err := store.CreateProject(ctx, &core.Project{Name: "project-a"})
	if err != nil {
		t.Fatalf("create project a: %v", err)
	}
	projectB, err := store.CreateProject(ctx, &core.Project{Name: "project-b"})
	if err != nil {
		t.Fatalf("create project b: %v", err)
	}
	currentID, err := store.CreateWorkItem(ctx, &core.WorkItem{
		ProjectID: &projectA,
		Title:     "current",
		Status:    core.WorkItemOpen,
		Priority:  core.PriorityMedium,
	})
	if err != nil {
		t.Fatalf("create current work item: %v", err)
	}
	dependencyID, err := store.CreateWorkItem(ctx, &core.WorkItem{
		ProjectID: &projectB,
		Title:     "dependency",
		Status:    core.WorkItemOpen,
		Priority:  core.PriorityMedium,
	})
	if err != nil {
		t.Fatalf("create dependency work item: %v", err)
	}

	wrapped := &workItemInitiativeMembershipStore{
		Store: store,
		byWorkItem: map[int64][]*core.InitiativeItem{
			currentID:    {{InitiativeID: 7, WorkItemID: currentID}},
			dependencyID: {{InitiativeID: 7, WorkItemID: dependencyID}},
		},
	}
	svc := New(Config{Store: wrapped, Tx: newSQLiteTxAdapter(store, nil)})

	updated, err := svc.UpdateWorkItem(ctx, UpdateWorkItemInput{
		ID:        currentID,
		DependsOn: &[]int64{dependencyID},
	})
	if err != nil {
		t.Fatalf("UpdateWorkItem: %v", err)
	}
	if len(updated.DependsOn) != 1 || updated.DependsOn[0] != dependencyID {
		t.Fatalf("expected depends_on to be updated, got %+v", updated.DependsOn)
	}
}

func TestServiceUpdateWorkItemRejectsCrossProjectDependenciesOutsideInitiative(t *testing.T) {
	store := newWorkItemAppTestStore(t)
	ctx := context.Background()

	projectA, err := store.CreateProject(ctx, &core.Project{Name: "project-a"})
	if err != nil {
		t.Fatalf("create project a: %v", err)
	}
	projectB, err := store.CreateProject(ctx, &core.Project{Name: "project-b"})
	if err != nil {
		t.Fatalf("create project b: %v", err)
	}
	currentID, err := store.CreateWorkItem(ctx, &core.WorkItem{
		ProjectID: &projectA,
		Title:     "current",
		Status:    core.WorkItemOpen,
		Priority:  core.PriorityMedium,
	})
	if err != nil {
		t.Fatalf("create current work item: %v", err)
	}
	dependencyID, err := store.CreateWorkItem(ctx, &core.WorkItem{
		ProjectID: &projectB,
		Title:     "dependency",
		Status:    core.WorkItemOpen,
		Priority:  core.PriorityMedium,
	})
	if err != nil {
		t.Fatalf("create dependency work item: %v", err)
	}

	wrapped := &workItemInitiativeMembershipStore{
		Store:      store,
		byWorkItem: map[int64][]*core.InitiativeItem{},
	}
	svc := New(Config{Store: wrapped, Tx: newSQLiteTxAdapter(store, nil)})

	_, err = svc.UpdateWorkItem(ctx, UpdateWorkItemInput{
		ID:        currentID,
		DependsOn: &[]int64{dependencyID},
	})
	if CodeOf(err) != CodeInvalidWorkItemDependency {
		t.Fatalf("expected %s, got %v", CodeInvalidWorkItemDependency, err)
	}
}

func TestServiceDeleteWorkItemDeletesOwnedDataAndDetachesFeatureEntries(t *testing.T) {
	store := newWorkItemAppTestStore(t)
	workItemID, actionID, featureID := createWorkItemFixture(t, store)
	svc := newSQLiteWorkItemService(store, newSQLiteTxAdapter(store, nil), nil)
	ctx := context.Background()

	if err := svc.DeleteWorkItem(ctx, workItemID); err != nil {
		t.Fatalf("DeleteWorkItem: %v", err)
	}
	if _, err := store.GetWorkItem(ctx, workItemID); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("expected work item to be deleted, got %v", err)
	}
	steps, err := store.ListActionsByWorkItem(ctx, workItemID)
	if err != nil {
		t.Fatalf("list actions: %v", err)
	}
	if len(steps) != 0 {
		t.Fatalf("expected actions deleted, got %d", len(steps))
	}
	runs, err := store.ListRunsByAction(ctx, actionID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected runs deleted, got %d", len(runs))
	}
	signals, err := store.ListActionSignals(ctx, actionID)
	if err != nil {
		t.Fatalf("list signals: %v", err)
	}
	if len(signals) != 0 {
		t.Fatalf("expected signals deleted, got %d", len(signals))
	}
	if _, err := store.FindAgentContext(ctx, "codex", workItemID); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("expected agent context deleted, got %v", err)
	}
	links, err := store.ListThreadsByWorkItem(ctx, workItemID)
	if err != nil {
		t.Fatalf("list thread links: %v", err)
	}
	if len(links) != 0 {
		t.Fatalf("expected thread links deleted, got %d", len(links))
	}
	events, err := store.ListEvents(ctx, core.EventFilter{WorkItemID: &workItemID})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected events deleted, got %d", len(events))
	}
	journalCount, err := store.CountJournal(ctx, core.JournalFilter{WorkItemID: &workItemID})
	if err != nil {
		t.Fatalf("count journal: %v", err)
	}
	if journalCount != 0 {
		t.Fatalf("expected journal deleted, got %d", journalCount)
	}
	feature, err := store.GetFeatureEntry(ctx, featureID)
	if err != nil {
		t.Fatalf("get feature entry: %v", err)
	}
	if feature.WorkItemID != nil || feature.ActionID != nil {
		t.Fatalf("expected feature entry to detach from work item, got %+v", feature)
	}
}

func TestServiceDeleteWorkItemRollsBackWhenAggregateDeleteFails(t *testing.T) {
	base := newWorkItemAppTestStore(t)
	workItemID, actionID, featureID := createWorkItemFixture(t, base)
	store := &failingDeleteStore{Store: base, failDeleteRuns: true}
	tx := newSQLiteTxAdapter(base, func(txStore core.Store) (TxStore, error) {
		sqliteStore, ok := txStore.(*sqlite.Store)
		if !ok {
			return nil, fmt.Errorf("unexpected tx store type %T", txStore)
		}
		return &failingDeleteStore{Store: sqliteStore, failDeleteRuns: true}, nil
	})
	svc := newSQLiteWorkItemService(store, tx, nil)
	ctx := context.Background()

	if err := svc.DeleteWorkItem(ctx, workItemID); err == nil {
		t.Fatal("expected delete work item to fail")
	}

	if _, err := base.GetWorkItem(ctx, workItemID); err != nil {
		t.Fatalf("expected work item to remain after rollback: %v", err)
	}
	runs, err := base.ListRunsByAction(ctx, actionID)
	if err != nil {
		t.Fatalf("list runs after rollback: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected runs to roll back, got %d", len(runs))
	}
	feature, err := base.GetFeatureEntry(ctx, featureID)
	if err != nil {
		t.Fatalf("get feature after rollback: %v", err)
	}
	if feature.WorkItemID == nil || *feature.WorkItemID != workItemID {
		t.Fatalf("expected feature entry work item link to roll back, got %+v", feature)
	}
}

func TestServiceRunWorkItemUsesBackgroundContext(t *testing.T) {
	store := newWorkItemAppTestStore(t)
	ctx := context.Background()

	projectID, err := store.CreateProject(ctx, &core.Project{Name: "project-run"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	workItemID, err := store.CreateWorkItem(ctx, &core.WorkItem{
		ProjectID: &projectID,
		Title:     "run me",
		Status:    core.WorkItemOpen,
		Priority:  core.PriorityMedium,
	})
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}
	if _, err := store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID,
		Name:       "exec",
		Type:       core.ActionExec,
		Status:     core.ActionPending,
		Position:   0,
	}); err != nil {
		t.Fatalf("create action: %v", err)
	}

	backgroundCtx, backgroundCancel := context.WithCancel(context.Background())
	defer backgroundCancel()

	reqCtx, reqCancel := context.WithCancel(context.Background())
	defer reqCancel()

	observed := make(chan context.Context, 1)
	done := make(chan struct{})
	runner := &runnerStub{
		run: func(runCtx context.Context, workItemID int64) error {
			if workItemID == 0 {
				t.Fatal("expected work item ID")
			}
			observed <- runCtx
			<-runCtx.Done()
			close(done)
			return runCtx.Err()
		},
	}

	svc := New(Config{
		Store:             store,
		Runner:            runner,
		BackgroundContext: backgroundCtx,
	})

	result, err := svc.RunWorkItem(reqCtx, workItemID)
	if err != nil {
		t.Fatalf("RunWorkItem: %v", err)
	}
	if result == nil || result.Queued {
		t.Fatalf("unexpected result: %+v", result)
	}

	var runCtx context.Context
	select {
	case runCtx = <-observed:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for runner context")
	}

	reqCancel()
	time.Sleep(20 * time.Millisecond)
	if err := runCtx.Err(); err != nil {
		t.Fatalf("runner context should outlive request context, got %v", err)
	}

	backgroundCancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for background context cancellation")
	}
}

func TestServiceRunWorkItemResetsFailedActionsBeforeQueueing(t *testing.T) {
	store := newWorkItemAppTestStore(t)
	ctx := context.Background()

	workItemID, err := store.CreateWorkItem(ctx, &core.WorkItem{
		Title:    "rerun me",
		Status:   core.WorkItemNeedsRework,
		Priority: core.PriorityMedium,
	})
	if err != nil {
		t.Fatalf("create work item: %v", err)
	}
	actionID, err := store.CreateAction(ctx, &core.Action{
		WorkItemID: workItemID,
		Name:       "exec",
		Type:       core.ActionExec,
		Status:     core.ActionFailed,
		Position:   0,
	})
	if err != nil {
		t.Fatalf("create action: %v", err)
	}

	var queuedID int64
	svc := New(Config{
		Store: store,
		Scheduler: &schedulerStub{
			submit: func(_ context.Context, workItemID int64) error {
				queuedID = workItemID
				return nil
			},
		},
	})

	result, err := svc.RunWorkItem(ctx, workItemID)
	if err != nil {
		t.Fatalf("RunWorkItem: %v", err)
	}
	if result == nil || !result.Queued {
		t.Fatalf("unexpected result: %+v", result)
	}
	if queuedID != workItemID {
		t.Fatalf("queued work item id = %d, want %d", queuedID, workItemID)
	}

	action, err := store.GetAction(ctx, actionID)
	if err != nil {
		t.Fatalf("GetAction() error = %v", err)
	}
	if action.Status != core.ActionPending {
		t.Fatalf("action.Status = %q, want %q", action.Status, core.ActionPending)
	}
}

func projectIDPtr(id int64) *int64 {
	return &id
}
