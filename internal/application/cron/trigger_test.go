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
	mu    sync.Mutex
	flows map[int64]*core.Flow
	steps map[int64]*core.Step
	nextID int64
}

func newMockStore() *mockStore {
	return &mockStore{
		flows: make(map[int64]*core.Flow),
		steps: make(map[int64]*core.Step),
	}
}

func (s *mockStore) nextFlowID() int64 {
	s.nextID++
	return s.nextID
}

func (s *mockStore) CreateFlow(_ context.Context, f *core.Flow) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextFlowID()
	f.ID = id
	f.CreatedAt = time.Now()
	f.UpdatedAt = time.Now()
	clone := *f
	if f.Metadata != nil {
		clone.Metadata = make(map[string]string, len(f.Metadata))
		for k, v := range f.Metadata {
			clone.Metadata[k] = v
		}
	}
	s.flows[id] = &clone
	return id, nil
}

func (s *mockStore) GetFlow(_ context.Context, id int64) (*core.Flow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.flows[id]
	if !ok {
		return nil, core.ErrNotFound
	}
	clone := *f
	return &clone, nil
}

func (s *mockStore) ListFlows(_ context.Context, filter core.FlowFilter) ([]*core.Flow, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []*core.Flow
	for _, f := range s.flows {
		if filter.Archived != nil {
			isArchived := f.ArchivedAt != nil
			if *filter.Archived != isArchived {
				continue
			}
		}
		if filter.Status != nil && f.Status != *filter.Status {
			continue
		}
		if filter.MetadataHasKey != "" {
			if f.Metadata == nil {
				continue
			}
			if _, ok := f.Metadata[filter.MetadataHasKey]; !ok {
				continue
			}
		}
		result = append(result, f)
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

func (s *mockStore) UpdateFlowStatus(_ context.Context, id int64, status core.FlowStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.flows[id]
	if !ok {
		return core.ErrNotFound
	}
	f.Status = status
	return nil
}

func (s *mockStore) UpdateFlowMetadata(_ context.Context, id int64, metadata map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.flows[id]
	if !ok {
		return core.ErrNotFound
	}
	f.Metadata = make(map[string]string, len(metadata))
	for k, v := range metadata {
		f.Metadata[k] = v
	}
	return nil
}

func (s *mockStore) PrepareFlowRun(_ context.Context, id int64, _ core.FlowStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.flows[id]
	if !ok {
		return core.ErrNotFound
	}
	return nil
}

func (s *mockStore) SetFlowArchived(_ context.Context, _ int64, _ bool) error {
	return nil
}

func (s *mockStore) CreateStep(_ context.Context, step *core.Step) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextFlowID()
	step.ID = id
	clone := *step
	s.steps[id] = &clone
	return id, nil
}

func (s *mockStore) GetStep(_ context.Context, id int64) (*core.Step, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	step, ok := s.steps[id]
	if !ok {
		return nil, core.ErrNotFound
	}
	clone := *step
	return &clone, nil
}

func (s *mockStore) ListStepsByFlow(_ context.Context, flowID int64) ([]*core.Step, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []*core.Step
	for _, step := range s.steps {
		if step.FlowID == flowID {
			clone := *step
			result = append(result, &clone)
		}
	}
	return result, nil
}

func (s *mockStore) UpdateStepStatus(_ context.Context, _ int64, _ core.StepStatus) error {
	return nil
}

func (s *mockStore) UpdateStep(_ context.Context, step *core.Step) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	clone := *step
	s.steps[step.ID] = &clone
	return nil
}

func (s *mockStore) DeleteStep(_ context.Context, _ int64) error {
	return nil
}

// --- mock scheduler ---

type mockScheduler struct {
	mu        sync.Mutex
	submitted []int64
}

func (s *mockScheduler) Submit(_ context.Context, flowID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.submitted = append(s.submitted, flowID)
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
	meta := map[string]string{
		MetaSchedule:   cronExpr,
		MetaEnabled:    "true",
		MetaTemplateID: "true",
	}
	if maxInst > 0 {
		meta[MetaMaxInstances] = strconv.Itoa(maxInst)
	}
	id, err := store.CreateFlow(context.Background(), &core.Flow{
		Name:     name,
		Status:   core.FlowPending,
		Metadata: meta,
	})
	if err != nil {
		t.Fatalf("create template: %v", err)
	}
	// Add a step so clone has something to copy.
	_, err = store.CreateStep(context.Background(), &core.Step{
		FlowID: id,
		Name:   "step-1",
		Type:   core.StepExec,
		Status: core.StepPending,
	})
	if err != nil {
		t.Fatalf("create step: %v", err)
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
	f, _ := store.GetFlow(ctx, templateID)
	if f.Metadata[MetaLastTriggered] == "" {
		t.Error("expected lastTriggered to be persisted")
	}
}

func TestTrigger_RespectsMaxInstances(t *testing.T) {
	store := newMockStore()
	sched := &mockScheduler{}
	bus := &mockBus{}

	templateID := createTemplate(t, store, "daily", "0 8 * * *", 1)

	// Create an active clone of this template (simulating already running).
	store.CreateFlow(context.Background(), &core.Flow{
		Name:   "daily [cron clone]",
		Status: core.FlowRunning,
		Metadata: map[string]string{
			MetaSourceFlowID: strconv.FormatInt(templateID, 10),
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
		store.CreateFlow(context.Background(), &core.Flow{
			Name:   "daily [clone]",
			Status: core.FlowRunning,
			Metadata: map[string]string{
				MetaSourceFlowID: strconv.FormatInt(templateID, 10),
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

func TestTrigger_ClonesSteps(t *testing.T) {
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

	// The cloned flow should have steps.
	cloneID := submitted[0]
	steps, err := store.ListStepsByFlow(ctx, cloneID)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 {
		t.Errorf("expected 1 cloned step, got %d", len(steps))
	}
}

func TestTrigger_LoadTemplatesFilters(t *testing.T) {
	store := newMockStore()
	sched := &mockScheduler{}
	bus := &mockBus{}

	// Create a template (should be found).
	createTemplate(t, store, "template", "0 8 * * *", 1)

	// Create a normal flow (should NOT be found).
	store.CreateFlow(context.Background(), &core.Flow{
		Name:   "normal-flow",
		Status: core.FlowPending,
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
