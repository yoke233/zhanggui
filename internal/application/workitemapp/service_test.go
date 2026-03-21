package workitemapp

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

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
	bindingID, err := store.CreateResourceBinding(ctx, &core.ResourceBinding{
		ProjectID:  projectID,
		WorkItemID: &workItemID,
		Kind:       core.ResourceKindAttachment,
		URI:        "D:/tmp/fixture.txt",
		Label:      "fixture.txt",
	})
	if err != nil {
		t.Fatalf("create work item resource binding: %v", err)
	}
	if _, err := store.CreateActionResource(ctx, &core.ActionResource{
		ActionID:          actionID,
		ResourceBindingID: bindingID,
		Direction:         core.ResourceInput,
		Path:              "fixture.txt",
		Required:          true,
	}); err != nil {
		t.Fatalf("create action resource: %v", err)
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

func projectIDPtr(id int64) *int64 {
	return &id
}
