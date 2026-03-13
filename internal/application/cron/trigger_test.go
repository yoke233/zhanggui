package cron

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// --- mock store ---

type mockStore struct {
	mu         sync.Mutex
	workItems  map[int64]*core.WorkItem
	actions    map[int64]*core.Action
	nextID     int64
}

func newMockStore() *mockStore {
	return &mockStore{
		workItems: make(map[int64]*core.WorkItem),
		actions:   make(map[int64]*core.Action),
	}
}

func (s *mockStore) nextWorkItemID() int64 {
	s.nextID++
	return s.nextID
}

func (s *mockStore) CreateWorkItem(_ context.Context, wi *core.WorkItem) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextWorkItemID()
	wi.ID = id
	wi.CreatedAt = time.Now()
	wi.UpdatedAt = time.Now()
	clone := *wi
	if wi.Metadata != nil {
		clone.Metadata = make(map[string]any, len(wi.Metadata))
		for k, v := range wi.Metadata {
			clone.Metadata[k] = v
		}
	}
	s.workItems[id] = &clone
	return id, nil
}

func (s *mockStore) GetWorkItem(_ context.Context, id int64) (*core.WorkItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	wi, ok := s.workItems[id]
	if !ok {
		return nil, core.ErrNotFound
	}
	clone := *wi
	return &clone, nil
}

func (s *mockStore) ListWorkItems(_ context.Context, filter core.WorkItemFilter) ([]*core.WorkItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []*core.WorkItem
	for _, wi := range s.workItems {
		if filter.Archived != nil {
			isArchived := wi.ArchivedAt != nil
			if *filter.Archived != isArchived {
				continue
			}
		}
		if filter.Status != nil && wi.Status != *filter.Status {
			continue
		}
		result = append(result, wi)
	}
	if filter.Offset > 0 && filter.Offset < len(result) {
		result = result[filter.Offset:]
	} else if filter.Offset >= len(result) {
		return nil, nil
	}
	if filter.Limit > 0 && filter.Limit < len(result) {
		result = result[:filter.Limit]
	}
	return result, nil
}

func (s *mockStore) UpdateWorkItem(_ context.Context, wi *core.WorkItem) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	clone := *wi
	s.workItems[wi.ID] = &clone
	return nil
}

func (s *mockStore) UpdateWorkItemStatus(_ context.Context, id int64, status core.WorkItemStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	wi, ok := s.workItems[id]
	if !ok {
		return core.ErrNotFound
	}
	wi.Status = status
	return nil
}

func (s *mockStore) UpdateWorkItemMetadata(_ context.Context, id int64, metadata map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	wi, ok := s.workItems[id]
	if !ok {
		return core.ErrNotFound
	}
	wi.Metadata = make(map[string]any, len(metadata))
	for k, v := range metadata {
		wi.Metadata[k] = v
	}
	return nil
}

func (s *mockStore) PrepareWorkItemRun(_ context.Context, id int64, _ core.WorkItemStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.workItems[id]
	if !ok {
		return core.ErrNotFound
	}
	return nil
}

func (s *mockStore) SetWorkItemArchived(_ context.Context, _ int64, _ bool) error {
	return nil
}

func (s *mockStore) DeleteWorkItem(_ context.Context, _ int64) error {
	return nil
}

func (s *mockStore) CreateAction(_ context.Context, action *core.Action) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextWorkItemID()
	action.ID = id
	clone := *action
	s.actions[id] = &clone
	return id, nil
}

func (s *mockStore) GetAction(_ context.Context, id int64) (*core.Action, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	action, ok := s.actions[id]
	if !ok {
		return nil, core.ErrNotFound
	}
	clone := *action
	return &clone, nil
}

func (s *mockStore) ListActionsByWorkItem(_ context.Context, workItemID int64) ([]*core.Action, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []*core.Action
	for _, action := range s.actions {
		if action.WorkItemID == workItemID {
			clone := *action
			result = append(result, &clone)
		}
	}
	return result, nil
}

func (s *mockStore) UpdateActionStatus(_ context.Context, _ int64, _ core.ActionStatus) error {
	return nil
}

func (s *mockStore) UpdateAction(_ context.Context, action *core.Action) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	clone := *action
	s.actions[action.ID] = &clone
	return nil
}

func (s *mockStore) DeleteAction(_ context.Context, _ int64) error {
	return nil
}

func (s *mockStore) BatchCreateActions(_ context.Context, _ []*core.Action) error {
	return nil
}

func (s *mockStore) UpdateActionDependsOn(_ context.Context, _ int64, _ []int64) error {
	return nil
}

// --- mock scheduler ---

type mockScheduler struct {
	mu        sync.Mutex
	submitted []int64
}

func (s *mockScheduler) Submit(_ context.Context, workItemID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.submitted = append(s.submitted, workItemID)
	return nil
}

func (s *mockScheduler) Submitted() []int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]int64, len(s.submitted))
	copy(out, s.submitted)
	return out
}

// --- mock bus ---

type mockBus struct {
	mu     sync.Mutex
	events []core.Event
}

func (b *mockBus) Publish(_ context.Context, e core.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, e)
}

// --- helpers ---

func createTemplate(t *testing.T, store *mockStore, name, cronExpr string, maxInst int) int64 {
	t.Helper()
	meta := map[string]any{
		MetaSchedule:   cronExpr,
		MetaEnabled:    "true",
		MetaTemplateID: "true",
	}
	if maxInst > 0 {
		meta[MetaMaxInstances] = strconv.Itoa(maxInst)
	}
	id, err := store.CreateWorkItem(context.Background(), &core.WorkItem{
		Title:    name,
		Status:   core.WorkItemOpen,
		Metadata: meta,
	})
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	// Add an action so clone has something to copy.
	_, err = store.CreateAction(context.Background(), &core.Action{
		WorkItemID: id,
		Name:       "action-1",
		Type:       core.ActionExec,
		Status:     core.ActionPending,
	})
	if err != nil {
		t.Fatalf("create action: %v", err)
	}
	return id
}

// --- tests ---

func TestTrigger_FiresOnSchedule(t *testing.T) {
	store := newMockStore()
	sched := &mockScheduler{}
	bus := &mockBus{}

	templateID := createTemplate(t, store, "daily-8am", "0 8 * * *", 1)

	trigger := New(store, sched, bus, Config{Enabled: true, Interval: time.Minute})

	// Set lastFired to yesterday 08:00.
	trigger.mu.Lock()
	trigger.schedules[templateID] = &templateState{
		schedule:  mustParseCron(t, "0 8 * * *"),
		lastFired: time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC),
		maxInst:   1,
	}
	trigger.mu.Unlock()

	// Simulate tick at 08:01 today — should fire.
	ctx := context.Background()
	templates, err := trigger.loadTemplates(ctx)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 3, 11, 8, 1, 0, 0, time.UTC)
	for _, tmpl := range templates {
		trigger.processTemplate(ctx, tmpl, now)
	}

	submitted := sched.Submitted()
	if len(submitted) != 1 {
		t.Fatalf("expected 1 submission, got %d", len(submitted))
	}

	// Verify lastTriggered was persisted.
	wi, _ := store.GetWorkItem(ctx, templateID)
	if metaString(wi.Metadata, MetaLastTriggered) == "" {
		t.Error("expected lastTriggered to be persisted")
	}
}

func TestTrigger_RespectsMaxInstances(t *testing.T) {
	store := newMockStore()
	sched := &mockScheduler{}
	bus := &mockBus{}

	templateID := createTemplate(t, store, "daily", "0 8 * * *", 1)

	// Create an active clone of this template (simulating already running).
	store.CreateWorkItem(context.Background(), &core.WorkItem{
		Title:  "daily [cron clone]",
		Status: core.WorkItemRunning,
		Metadata: map[string]any{
			MetaSourceWorkItemID: strconv.FormatInt(templateID, 10),
		},
	})

	trigger := New(store, sched, bus, Config{Enabled: true, Interval: time.Minute})

	ctx := context.Background()
	templates, err := trigger.loadTemplates(ctx)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 3, 11, 8, 1, 0, 0, time.UTC)
	for _, tmpl := range templates {
		trigger.processTemplate(ctx, tmpl, now)
	}

	submitted := sched.Submitted()
	if len(submitted) != 0 {
		t.Errorf("expected 0 submissions (max instances reached), got %d", len(submitted))
	}
}

func TestTrigger_MaxInstancesAllowsMore(t *testing.T) {
	store := newMockStore()
	sched := &mockScheduler{}
	bus := &mockBus{}

	templateID := createTemplate(t, store, "daily", "0 8 * * *", 3)

	// Create 2 active clones — maxInst is 3, so one more should be allowed.
	for i := 0; i < 2; i++ {
		store.CreateWorkItem(context.Background(), &core.WorkItem{
			Title:  "daily [clone]",
			Status: core.WorkItemRunning,
			Metadata: map[string]any{
				MetaSourceWorkItemID: strconv.FormatInt(templateID, 10),
			},
		})
	}

	trigger := New(store, sched, bus, Config{Enabled: true, Interval: time.Minute})

	// Pre-set lastFired to yesterday so shouldFire will fire at 08:01 today.
	trigger.mu.Lock()
	trigger.schedules[templateID] = &templateState{
		schedule:  mustParseCron(t, "0 8 * * *"),
		lastFired: time.Date(2026, 3, 10, 8, 0, 0, 0, time.UTC),
		maxInst:   3,
	}
	trigger.mu.Unlock()

	ctx := context.Background()
	templates, err := trigger.loadTemplates(ctx)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 3, 11, 8, 1, 0, 0, time.UTC)
	for _, tmpl := range templates {
		trigger.processTemplate(ctx, tmpl, now)
	}

	if len(sched.Submitted()) != 1 {
		t.Errorf("expected 1 submission, got %d", len(sched.Submitted()))
	}
}

func TestTrigger_DoesNotFireBeforeSchedule(t *testing.T) {
	store := newMockStore()
	sched := &mockScheduler{}
	bus := &mockBus{}

	createTemplate(t, store, "daily-8am", "0 8 * * *", 1)

	trigger := New(store, sched, bus, Config{Enabled: true, Interval: time.Minute})

	ctx := context.Background()
	templates, err := trigger.loadTemplates(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Now is 07:59 — should not fire (never fired before, minute doesn't match).
	now := time.Date(2026, 3, 11, 7, 59, 0, 0, time.UTC)
	for _, tmpl := range templates {
		trigger.processTemplate(ctx, tmpl, now)
	}

	if len(sched.Submitted()) != 0 {
		t.Errorf("expected 0 submissions at 07:59, got %d", len(sched.Submitted()))
	}
}

func TestTrigger_ClonesActions(t *testing.T) {
	store := newMockStore()
	sched := &mockScheduler{}
	bus := &mockBus{}

	createTemplate(t, store, "daily", "0 8 * * *", 1)

	trigger := New(store, sched, bus, Config{Enabled: true, Interval: time.Minute})

	ctx := context.Background()
	templates, err := trigger.loadTemplates(ctx)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 3, 11, 8, 0, 0, 0, time.UTC)
	for _, tmpl := range templates {
		trigger.processTemplate(ctx, tmpl, now)
	}

	submitted := sched.Submitted()
	if len(submitted) != 1 {
		t.Fatalf("expected 1 submission, got %d", len(submitted))
	}

	// The cloned work item should have actions.
	cloneID := submitted[0]
	actions, err := store.ListActionsByWorkItem(ctx, cloneID)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 {
		t.Errorf("expected 1 cloned action, got %d", len(actions))
	}
}

func TestTrigger_LoadTemplatesFilters(t *testing.T) {
	store := newMockStore()
	sched := &mockScheduler{}
	bus := &mockBus{}

	// Create a template (should be found).
	createTemplate(t, store, "template", "0 8 * * *", 1)

	// Create a normal work item (should NOT be found).
	store.CreateWorkItem(context.Background(), &core.WorkItem{
		Title:  "normal-workitem",
		Status: core.WorkItemOpen,
	})

	trigger := New(store, sched, bus, Config{Enabled: true})

	templates, err := trigger.loadTemplates(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) != 1 {
		t.Errorf("expected 1 template, got %d", len(templates))
	}
}

func mustParseCron(t *testing.T, expr string) cronSchedule {
	t.Helper()
	s, err := parseCron(expr)
	if err != nil {
		t.Fatalf("parseCron(%q): %v", expr, err)
	}
	return s
}
