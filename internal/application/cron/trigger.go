package cron

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// Metadata keys used in WorkItem.Metadata to define cron triggers.
const (
	MetaSchedule         = "cron_schedule"           // cron expression, e.g. "0 */6 * * *"
	MetaEnabled          = "cron_enabled"            // "true" to activate
	MetaTemplateID       = "cron_template"           // "true" marks this work item as a template (not submitted itself)
	MetaMaxInstances     = "cron_max_instances"      // max concurrent instances from this template (default 1)
	MetaSourceWorkItemID = "cron_source_workitem_id" // set on cloned work items to trace origin
	MetaLastTriggered    = "cron_last_triggered"     // ISO8601 timestamp of last trigger
)

// Store is the persistence port required by the cron trigger.
type Store interface {
	core.WorkItemStore
	core.ActionStore
}

// Scheduler is the work item submission port.
type Scheduler interface {
	Submit(ctx context.Context, workItemID int64) error
}

// EventPublisher publishes domain events.
type EventPublisher interface {
	Publish(ctx context.Context, event core.Event)
}

// Config configures the cron trigger.
type Config struct {
	Enabled  bool
	Interval time.Duration // how often to scan (default 1m)
}

// Trigger is a background service that periodically scans for cron-enabled
// work item templates and creates+submits new work item instances on schedule.
type Trigger struct {
	store     Store
	scheduler Scheduler
	bus       EventPublisher
	cfg       Config

	mu        sync.Mutex
	schedules map[int64]*templateState // workItemID → state
}

type templateState struct {
	schedule    cronSchedule
	lastFired   time.Time
	maxInst     int
	activeCount int
}

// New creates a CronTrigger.
func New(store Store, scheduler Scheduler, bus EventPublisher, cfg Config) *Trigger {
	if cfg.Interval <= 0 {
		cfg.Interval = time.Minute
	}
	return &Trigger{
		store:     store,
		scheduler: scheduler,
		bus:       bus,
		cfg:       cfg,
		schedules: make(map[int64]*templateState),
	}
}

// Start runs the trigger loop. Blocks until ctx is cancelled.
func (t *Trigger) Start(ctx context.Context) {
	if !t.cfg.Enabled {
		slog.Info("cron: trigger disabled")
		return
	}
	slog.Info("cron: trigger started", "interval", t.cfg.Interval)

	ticker := time.NewTicker(t.cfg.Interval)
	defer ticker.Stop()

	// Run once immediately on startup.
	t.tick(ctx)

	for {
		select {
		case <-ctx.Done():
			slog.Info("cron: trigger stopped")
			return
		case <-ticker.C:
			t.tick(ctx)
		}
	}
}

func (t *Trigger) tick(ctx context.Context) {
	templates, err := t.loadTemplates(ctx)
	if err != nil {
		slog.Error("cron: failed to load templates", "error", err)
		return
	}

	now := time.Now().UTC()
	for _, tmpl := range templates {
		t.processTemplate(ctx, tmpl, now)
	}
}

type workItemTemplate struct {
	workItem  *core.WorkItem
	schedule  cronSchedule
	maxInst   int
	lastFired time.Time
}

func (t *Trigger) loadTemplates(ctx context.Context) ([]workItemTemplate, error) {
	const pageSize = 200
	var templates []workItemTemplate
	offset := 0

	archived := false
	for {
		workItems, err := t.store.ListWorkItems(ctx, core.WorkItemFilter{
			Archived: &archived,
			Limit:    pageSize,
			Offset:   offset,
		})
		if err != nil {
			return nil, fmt.Errorf("list work items: %w", err)
		}

		for _, wi := range workItems {
			tmpl, ok := parseTemplate(wi)
			if ok {
				templates = append(templates, tmpl)
			}
		}

		if len(workItems) < pageSize {
			break
		}
		offset += pageSize
	}
	return templates, nil
}

func parseTemplate(wi *core.WorkItem) (workItemTemplate, bool) {
	if wi == nil || wi.Metadata == nil {
		return workItemTemplate{}, false
	}
	if !metaBool(wi.Metadata, MetaEnabled) {
		return workItemTemplate{}, false
	}
	if !metaBool(wi.Metadata, MetaTemplateID) {
		return workItemTemplate{}, false
	}
	expr := metaString(wi.Metadata, MetaSchedule)
	if expr == "" {
		return workItemTemplate{}, false
	}

	sched, err := parseCron(expr)
	if err != nil {
		slog.Warn("cron: invalid schedule", "workitem_id", wi.ID, "expr", expr, "error", err)
		return workItemTemplate{}, false
	}

	maxInst := 1
	if v := metaString(wi.Metadata, MetaMaxInstances); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxInst = n
		}
	}

	var lastFired time.Time
	if v := metaString(wi.Metadata, MetaLastTriggered); v != "" {
		if parsed, err := time.Parse(time.RFC3339, v); err == nil {
			lastFired = parsed
		}
	}

	return workItemTemplate{
		workItem:  wi,
		schedule:  sched,
		maxInst:   maxInst,
		lastFired: lastFired,
	}, true
}

func (t *Trigger) processTemplate(ctx context.Context, tmpl workItemTemplate, now time.Time) {
	t.mu.Lock()
	state, ok := t.schedules[tmpl.workItem.ID]
	if !ok {
		state = &templateState{
			schedule:  tmpl.schedule,
			lastFired: tmpl.lastFired,
			maxInst:   tmpl.maxInst,
		}
		t.schedules[tmpl.workItem.ID] = state
	} else {
		state.schedule = tmpl.schedule
		state.maxInst = tmpl.maxInst
		if !tmpl.lastFired.IsZero() && tmpl.lastFired.After(state.lastFired) {
			state.lastFired = tmpl.lastFired
		}
	}
	t.mu.Unlock()

	// Check if it's time to fire.
	if !state.schedule.shouldFire(state.lastFired, now) {
		return
	}

	// Check maxInstances: count active (open/queued/running) clones of this template.
	activeCount := t.countActiveInstances(ctx, tmpl.workItem.ID)
	if activeCount >= tmpl.maxInst {
		slog.Debug("cron: skipping trigger, max instances reached",
			"template_workitem_id", tmpl.workItem.ID,
			"active", activeCount,
			"max", tmpl.maxInst,
		)
		return
	}

	// Clone and submit.
	newWorkItemID, err := t.cloneAndSubmit(ctx, tmpl.workItem)
	if err != nil {
		slog.Error("cron: clone+submit failed", "template_workitem_id", tmpl.workItem.ID, "error", err)
		return
	}

	// Update in-memory state.
	t.mu.Lock()
	state.lastFired = now
	t.mu.Unlock()

	// Persist lastTriggered back to template metadata.
	tmpl.workItem.Metadata[MetaLastTriggered] = now.Format(time.RFC3339)
	if err := t.store.UpdateWorkItemMetadata(ctx, tmpl.workItem.ID, tmpl.workItem.Metadata); err != nil {
		slog.Warn("cron: failed to persist last_triggered", "template_workitem_id", tmpl.workItem.ID, "error", err)
	}

	slog.Info("cron: triggered work item",
		"template_workitem_id", tmpl.workItem.ID,
		"new_workitem_id", newWorkItemID,
		"schedule", metaString(tmpl.workItem.Metadata, MetaSchedule),
	)
}

// countActiveInstances counts non-terminal work items cloned from the given template.
func (t *Trigger) countActiveInstances(ctx context.Context, templateWorkItemID int64) int {
	sourceID := strconv.FormatInt(templateWorkItemID, 10)
	count := 0

	// Check active statuses: open, accepted, queued, running, blocked.
	for _, status := range []core.WorkItemStatus{core.WorkItemOpen, core.WorkItemAccepted, core.WorkItemQueued, core.WorkItemRunning, core.WorkItemBlocked} {
		offset := 0
		for {
			workItems, err := t.store.ListWorkItems(ctx, core.WorkItemFilter{
				Status: &status,
				Limit:  100,
				Offset: offset,
			})
			if err != nil {
				slog.Warn("cron: failed to count active instances", "error", err)
				break
			}
			for _, wi := range workItems {
				if metaString(wi.Metadata, MetaSourceWorkItemID) == sourceID {
					count++
				}
			}
			if len(workItems) < 100 {
				break
			}
			offset += 100
		}
	}
	return count
}

func (t *Trigger) cloneAndSubmit(ctx context.Context, source *core.WorkItem) (int64, error) {
	// 1. Clone work item.
	newWorkItem := &core.WorkItem{
		ProjectID:         source.ProjectID,
		ResourceBindingID: source.ResourceBindingID,
		Title:             source.Title + " [cron " + time.Now().UTC().Format("01-02 15:04") + "]",
		Status:            core.WorkItemOpen,
		Metadata: map[string]any{
			MetaSourceWorkItemID: strconv.FormatInt(source.ID, 10),
		},
	}
	newWorkItemID, err := t.store.CreateWorkItem(ctx, newWorkItem)
	if err != nil {
		return 0, fmt.Errorf("create work item clone: %w", err)
	}

	// 2. Clone actions.
	actions, err := t.store.ListActionsByWorkItem(ctx, source.ID)
	if err != nil {
		return 0, fmt.Errorf("list source actions: %w", err)
	}

	for i, a := range actions {
		newAction := &core.Action{
			WorkItemID:           newWorkItemID,
			Name:                 a.Name,
			Description:          a.Description,
			Type:                 a.Type,
			Status:               core.ActionPending,
			Position:             a.Position,
			AgentRole:            a.AgentRole,
			RequiredCapabilities: a.RequiredCapabilities,
			AcceptanceCriteria:   a.AcceptanceCriteria,
			Timeout:              a.Timeout,
			MaxRetries:           a.MaxRetries,
			Config:               a.Config,
		}
		if newAction.Position < 0 {
			newAction.Position = i
		}
		if _, err := t.store.CreateAction(ctx, newAction); err != nil {
			return 0, fmt.Errorf("clone action %d: %w", a.ID, err)
		}
	}

	// 3. Publish event.
	t.bus.Publish(ctx, core.Event{
		Type:       core.EventWorkItemQueued,
		WorkItemID: newWorkItemID,
		Data: map[string]any{
			"source":      "cron",
			"template_id": source.ID,
		},
		Timestamp: time.Now().UTC(),
	})

	// 4. Submit to scheduler.
	if err := t.scheduler.Submit(ctx, newWorkItemID); err != nil {
		return 0, fmt.Errorf("submit cloned work item: %w", err)
	}

	return newWorkItemID, nil
}

func metaBool(m map[string]any, key string) bool {
	v := strings.TrimSpace(strings.ToLower(metaString(m, key)))
	return v == "true" || v == "1" || v == "yes"
}

func metaString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	return strings.TrimSpace(s)
}
