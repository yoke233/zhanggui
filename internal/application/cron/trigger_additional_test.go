package cron

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

type errStore struct {
	*mockStore

	listWorkItemsHook func(core.WorkItemFilter) error
	createWorkItemErr error
	listActionsErr    error
	createActionErr   error
	updateMetadataErr error
}

func (s *errStore) ListWorkItems(ctx context.Context, filter core.WorkItemFilter) ([]*core.WorkItem, error) {
	if s.listWorkItemsHook != nil {
		if err := s.listWorkItemsHook(filter); err != nil {
			return nil, err
		}
	}
	return s.mockStore.ListWorkItems(ctx, filter)
}

func (s *errStore) CreateWorkItem(ctx context.Context, wi *core.WorkItem) (int64, error) {
	if s.createWorkItemErr != nil {
		return 0, s.createWorkItemErr
	}
	return s.mockStore.CreateWorkItem(ctx, wi)
}

func (s *errStore) ListActionsByWorkItem(ctx context.Context, workItemID int64) ([]*core.Action, error) {
	if s.listActionsErr != nil {
		return nil, s.listActionsErr
	}
	return s.mockStore.ListActionsByWorkItem(ctx, workItemID)
}

func (s *errStore) CreateAction(ctx context.Context, action *core.Action) (int64, error) {
	if s.createActionErr != nil {
		return 0, s.createActionErr
	}
	return s.mockStore.CreateAction(ctx, action)
}

func (s *errStore) UpdateWorkItemMetadata(ctx context.Context, id int64, metadata map[string]any) error {
	if s.updateMetadataErr != nil {
		return s.updateMetadataErr
	}
	return s.mockStore.UpdateWorkItemMetadata(ctx, id, metadata)
}

type errScheduler struct {
	mockScheduler
	err error
}

func (s *errScheduler) Submit(ctx context.Context, workItemID int64) error {
	if err := s.err; err != nil {
		return err
	}
	return s.mockScheduler.Submit(ctx, workItemID)
}

func TestParseTemplateAndMetaHelpers(t *testing.T) {
	t.Run("invalid cases", func(t *testing.T) {
		cases := []*core.WorkItem{
			nil,
			{ID: 1},
			{ID: 2, Metadata: map[string]any{MetaEnabled: "false", MetaTemplateID: "true", MetaSchedule: "* * * * *"}},
			{ID: 3, Metadata: map[string]any{MetaEnabled: "true", MetaTemplateID: "false", MetaSchedule: "* * * * *"}},
			{ID: 4, Metadata: map[string]any{MetaEnabled: "true", MetaTemplateID: "true"}},
			{ID: 5, Metadata: map[string]any{MetaEnabled: "true", MetaTemplateID: "true", MetaSchedule: "bad"}},
		}
		for _, wi := range cases {
			if _, ok := parseTemplate(wi); ok {
				t.Fatalf("parseTemplate(%#v) should not match", wi)
			}
		}
	})

	t.Run("valid defaults and parsed fields", func(t *testing.T) {
		now := time.Date(2026, 3, 15, 9, 30, 0, 0, time.UTC)
		tmpl, ok := parseTemplate(&core.WorkItem{
			ID: 9,
			Metadata: map[string]any{
				MetaEnabled:       " yes ",
				MetaTemplateID:    "1",
				MetaSchedule:      "*/5 * * * *",
				MetaMaxInstances:  "not-a-number",
				MetaLastTriggered: now.Format(time.RFC3339),
			},
		})
		if !ok {
			t.Fatal("parseTemplate() should match")
		}
		if tmpl.maxInst != 1 {
			t.Fatalf("maxInst = %d, want 1", tmpl.maxInst)
		}
		if !tmpl.lastFired.Equal(now) {
			t.Fatalf("lastFired = %v, want %v", tmpl.lastFired, now)
		}
		if !tmpl.schedule.matches(now) {
			t.Fatalf("schedule should match %v", now)
		}
	})

	if got := metaString(map[string]any{"value": "  x  "}, "value"); got != "x" {
		t.Fatalf("metaString(trimmed string) = %q, want x", got)
	}
	if got := metaString(map[string]any{"value": 42}, "value"); got != "42" {
		t.Fatalf("metaString(non-string) = %q, want 42", got)
	}
	if got := metaString(nil, "value"); got != "" {
		t.Fatalf("metaString(nil) = %q, want empty", got)
	}
	if !metaBool(map[string]any{"v": "yes"}, "v") || !metaBool(map[string]any{"v": "1"}, "v") || !metaBool(map[string]any{"v": "true"}, "v") {
		t.Fatal("metaBool should accept yes/1/true")
	}
	if metaBool(map[string]any{"v": "no"}, "v") {
		t.Fatal("metaBool(no) should be false")
	}
}

func TestTriggerStartDisabledReturnsImmediately(t *testing.T) {
	trigger := New(nil, nil, nil, Config{Enabled: false})
	trigger.Start(context.Background())
}

func TestTriggerStartRunsImmediateTickAndStops(t *testing.T) {
	store := newMockStore()
	scheduler := &mockScheduler{}
	bus := &mockBus{}
	createTemplate(t, store, "always", "* * * * *", 1)

	trigger := New(store, scheduler, bus, Config{Enabled: true, Interval: 10 * time.Millisecond})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		trigger.Start(ctx)
		close(done)
	}()

	deadline := time.Now().Add(500 * time.Millisecond)
	for len(scheduler.Submitted()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Start() did not stop after context cancellation")
	}

	if len(scheduler.Submitted()) == 0 {
		t.Fatal("expected immediate tick to submit a cloned work item")
	}
}

func TestTriggerTickAndLoadTemplatesHandleStoreError(t *testing.T) {
	store := &errStore{
		mockStore:         newMockStore(),
		listWorkItemsHook: func(core.WorkItemFilter) error { return errors.New("boom") },
	}
	trigger := New(store, &mockScheduler{}, &mockBus{}, Config{Enabled: true})

	if _, err := trigger.loadTemplates(context.Background()); err == nil || !strings.Contains(err.Error(), "list work items") {
		t.Fatalf("loadTemplates() error = %v, want wrapped list work items error", err)
	}

	trigger.tick(context.Background())
}

func TestTriggerCountActiveInstancesHandlesPagingAndErrors(t *testing.T) {
	store := &errStore{
		mockStore: newMockStore(),
		listWorkItemsHook: func(filter core.WorkItemFilter) error {
			if filter.Status != nil && *filter.Status == core.WorkItemAccepted {
				return errors.New("accepted unavailable")
			}
			return nil
		},
	}

	templateID := int64(700)
	for i := 0; i < 105; i++ {
		_, err := store.CreateWorkItem(context.Background(), &core.WorkItem{
			Title:  "open clone",
			Status: core.WorkItemOpen,
			Metadata: map[string]any{
				MetaSourceWorkItemID: "700",
			},
		})
		if err != nil {
			t.Fatalf("CreateWorkItem(open): %v", err)
		}
	}
	for i := 0; i < 3; i++ {
		_, err := store.CreateWorkItem(context.Background(), &core.WorkItem{
			Title:  "running clone",
			Status: core.WorkItemRunning,
			Metadata: map[string]any{
				MetaSourceWorkItemID: "700",
			},
		})
		if err != nil {
			t.Fatalf("CreateWorkItem(running): %v", err)
		}
	}
	_, _ = store.CreateWorkItem(context.Background(), &core.WorkItem{
		Title:  "other template clone",
		Status: core.WorkItemBlocked,
		Metadata: map[string]any{
			MetaSourceWorkItemID: "701",
		},
	})

	trigger := New(store, &mockScheduler{}, &mockBus{}, Config{Enabled: true})
	if got := trigger.countActiveInstances(context.Background(), templateID); got != 108 {
		t.Fatalf("countActiveInstances() = %d, want 108", got)
	}
}

func TestTriggerProcessTemplateUpdatesExistingStateAndIgnoresMetadataPersistError(t *testing.T) {
	store := &errStore{
		mockStore:         newMockStore(),
		updateMetadataErr: errors.New("write failed"),
	}
	scheduler := &mockScheduler{}
	trigger := New(store, scheduler, &mockBus{}, Config{Enabled: true})

	templateID := createTemplate(t, store.mockStore, "always", "* * * * *", 2)
	now := time.Date(2026, 3, 15, 9, 45, 0, 0, time.UTC)
	trigger.schedules[templateID] = &templateState{
		schedule:  mustParseCron(t, "* * * * *"),
		lastFired: now.Add(-2 * time.Hour),
		maxInst:   1,
	}

	templates, err := trigger.loadTemplates(context.Background())
	if err != nil {
		t.Fatalf("loadTemplates(): %v", err)
	}
	trigger.processTemplate(context.Background(), templates[0], now)

	if len(scheduler.Submitted()) != 1 {
		t.Fatalf("submitted = %d, want 1", len(scheduler.Submitted()))
	}
	if got := trigger.schedules[templateID].lastFired; !got.Equal(now) {
		t.Fatalf("state.lastFired = %v, want %v", got, now)
	}
}

func TestCloneAndSubmitErrorPaths(t *testing.T) {
	t.Run("create work item fails", func(t *testing.T) {
		trigger := New(&errStore{
			mockStore:         newMockStore(),
			createWorkItemErr: errors.New("create failed"),
		}, &mockScheduler{}, &mockBus{}, Config{Enabled: true})

		_, err := trigger.cloneAndSubmit(context.Background(), &core.WorkItem{ID: 1, Title: "tmpl"})
		if err == nil || !strings.Contains(err.Error(), "create work item clone") {
			t.Fatalf("cloneAndSubmit() error = %v", err)
		}
	})

	t.Run("list actions fails", func(t *testing.T) {
		store := &errStore{
			mockStore:      newMockStore(),
			listActionsErr: errors.New("list failed"),
		}
		trigger := New(store, &mockScheduler{}, &mockBus{}, Config{Enabled: true})

		source := &core.WorkItem{ID: 1, Title: "tmpl"}
		_, err := trigger.cloneAndSubmit(context.Background(), source)
		if err == nil || !strings.Contains(err.Error(), "list source actions") {
			t.Fatalf("cloneAndSubmit() error = %v", err)
		}
	})

	t.Run("create action fails", func(t *testing.T) {
		store := &errStore{
			mockStore:       newMockStore(),
			createActionErr: errors.New("action failed"),
		}
		sourceID, err := store.mockStore.CreateWorkItem(context.Background(), &core.WorkItem{Title: "tmpl"})
		if err != nil {
			t.Fatalf("CreateWorkItem(source): %v", err)
		}
		_, err = store.mockStore.CreateAction(context.Background(), &core.Action{
			WorkItemID: sourceID,
			ID:         88,
			Name:       "run",
			Type:       core.ActionExec,
			Status:     core.ActionPending,
		})
		if err != nil {
			t.Fatalf("CreateAction(source): %v", err)
		}

		trigger := New(store, &mockScheduler{}, &mockBus{}, Config{Enabled: true})
		_, err = trigger.cloneAndSubmit(context.Background(), &core.WorkItem{ID: sourceID, Title: "tmpl"})
		if err == nil || !strings.Contains(err.Error(), "clone action") {
			t.Fatalf("cloneAndSubmit() error = %v", err)
		}
	})

	t.Run("submit fails after cloning actions", func(t *testing.T) {
		store := newMockStore()
		sourceID, err := store.CreateWorkItem(context.Background(), &core.WorkItem{
			Title: "tmpl",
		})
		if err != nil {
			t.Fatalf("CreateWorkItem(source): %v", err)
		}
		_, err = store.CreateAction(context.Background(), &core.Action{
			WorkItemID: sourceID,
			ID:         90,
			Name:       "run",
			Type:       core.ActionExec,
			Status:     core.ActionPending,
			Position:   -1,
		})
		if err != nil {
			t.Fatalf("CreateAction(source): %v", err)
		}

		scheduler := &errScheduler{err: errors.New("submit failed")}
		trigger := New(store, scheduler, &mockBus{}, Config{Enabled: true})

		newID, err := trigger.cloneAndSubmit(context.Background(), &core.WorkItem{
			ID:    sourceID,
			Title: "tmpl",
		})
		if err == nil || !strings.Contains(err.Error(), "submit cloned work item") {
			t.Fatalf("cloneAndSubmit() error = %v", err)
		}
		if newID != 0 {
			t.Fatalf("newID = %d, want 0 on submit failure", newID)
		}

		actions, err := store.ListActionsByWorkItem(context.Background(), 3)
		if err != nil {
			t.Fatalf("ListActionsByWorkItem(clone): %v", err)
		}
		if len(actions) != 1 || actions[0].Position != 0 {
			t.Fatalf("cloned actions = %#v, want one action with normalized position 0", actions)
		}
	})
}
