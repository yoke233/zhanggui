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

// Metadata keys used in Flow.Metadata to define cron triggers.
const (
	MetaSchedule      = "cron_schedule"       // cron expression, e.g. "0 */6 * * *"
	MetaEnabled       = "cron_enabled"        // "true" to activate
	MetaTemplateID    = "cron_template"       // "true" marks this flow as a template (not submitted itself)
	MetaMaxInstances  = "cron_max_instances"  // max concurrent instances from this template (default 1)
	MetaSourceFlowID  = "cron_source_flow_id" // set on cloned flows to trace origin
	MetaLastTriggered = "cron_last_triggered" // ISO8601 timestamp of last trigger
)

// Store is the persistence port required by the cron trigger.
type Store interface {
	core.FlowStore
	core.StepStore
}

// Scheduler is the flow submission port.
type Scheduler interface {
	Submit(ctx context.Context, flowID int64) error
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
// flow templates and creates+submits new flow instances on schedule.
type Trigger struct {
	store     Store
	scheduler Scheduler
	bus       EventPublisher
	cfg       Config

	mu        sync.Mutex
	schedules map[int64]*templateState // flowID → state
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

type flowTemplate struct {
	flow        *core.Flow
	schedule    cronSchedule
	maxInst     int
	lastFired   time.Time
}

func (t *Trigger) loadTemplates(ctx context.Context) ([]flowTemplate, error) {
	const pageSize = 200
	var templates []flowTemplate
	offset := 0

	archived := false
	for {
		flows, err := t.store.ListFlows(ctx, core.FlowFilter{
			Archived:       &archived,
			MetadataHasKey: MetaTemplateID,
			Limit:          pageSize,
			Offset:         offset,
		})
		if err != nil {
			return nil, fmt.Errorf("list flows: %w", err)
		}

		for _, f := range flows {
			tmpl, ok := parseTemplate(f)
			if ok {
				templates = append(templates, tmpl)
			}
		}

		if len(flows) < pageSize {
			break
		}
		offset += pageSize
	}
	return templates, nil
}

func parseTemplate(f *core.Flow) (flowTemplate, bool) {
	if f == nil || f.Metadata == nil {
		return flowTemplate{}, false
	}
	if !metaBool(f.Metadata, MetaEnabled) {
		return flowTemplate{}, false
	}
	if !metaBool(f.Metadata, MetaTemplateID) {
		return flowTemplate{}, false
	}
	expr := strings.TrimSpace(f.Metadata[MetaSchedule])
	if expr == "" {
		return flowTemplate{}, false
	}

	sched, err := parseCron(expr)
	if err != nil {
		slog.Warn("cron: invalid schedule", "flow_id", f.ID, "expr", expr, "error", err)
		return flowTemplate{}, false
	}

	maxInst := 1
	if v, ok := f.Metadata[MetaMaxInstances]; ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxInst = n
		}
	}

	var lastFired time.Time
	if v, ok := f.Metadata[MetaLastTriggered]; ok {
		if parsed, err := time.Parse(time.RFC3339, v); err == nil {
			lastFired = parsed
		}
	}

	return flowTemplate{
		flow:     f,
		schedule: sched,
		maxInst:  maxInst,
		lastFired: lastFired,
	}, true
}

func (t *Trigger) processTemplate(ctx context.Context, tmpl flowTemplate, now time.Time) {
	t.mu.Lock()
	state, ok := t.schedules[tmpl.flow.ID]
	if !ok {
		state = &templateState{
			schedule:  tmpl.schedule,
			lastFired: tmpl.lastFired,
			maxInst:   tmpl.maxInst,
		}
		t.schedules[tmpl.flow.ID] = state
	}
	t.mu.Unlock()

	// Check if it's time to fire.
	if !state.schedule.shouldFire(state.lastFired, now) {
		return
	}

	// Check maxInstances: count active (pending/queued/running) clones of this template.
	activeCount := t.countActiveInstances(ctx, tmpl.flow.ID)
	if activeCount >= tmpl.maxInst {
		slog.Debug("cron: skipping trigger, max instances reached",
			"template_flow_id", tmpl.flow.ID,
			"active", activeCount,
			"max", tmpl.maxInst,
		)
		return
	}

	// Clone and submit.
	newFlowID, err := t.cloneAndSubmit(ctx, tmpl.flow)
	if err != nil {
		slog.Error("cron: clone+submit failed", "template_flow_id", tmpl.flow.ID, "error", err)
		return
	}

	// Update in-memory state.
	t.mu.Lock()
	state.lastFired = now
	t.mu.Unlock()

	// Persist lastTriggered back to template metadata.
	tmpl.flow.Metadata[MetaLastTriggered] = now.Format(time.RFC3339)
	if err := t.store.UpdateFlowMetadata(ctx, tmpl.flow.ID, tmpl.flow.Metadata); err != nil {
		slog.Warn("cron: failed to persist last_triggered", "template_flow_id", tmpl.flow.ID, "error", err)
	}

	slog.Info("cron: triggered flow",
		"template_flow_id", tmpl.flow.ID,
		"new_flow_id", newFlowID,
		"schedule", tmpl.flow.Metadata[MetaSchedule],
	)
}

// countActiveInstances counts non-terminal flows cloned from the given template.
func (t *Trigger) countActiveInstances(ctx context.Context, templateFlowID int64) int {
	sourceID := strconv.FormatInt(templateFlowID, 10)
	count := 0

	// Check active statuses: pending, queued, running, blocked.
	for _, status := range []core.FlowStatus{core.FlowPending, core.FlowQueued, core.FlowRunning, core.FlowBlocked} {
		flows, err := t.store.ListFlows(ctx, core.FlowFilter{
			Status:         &status,
			MetadataHasKey: MetaSourceFlowID,
			Limit:          100,
		})
		if err != nil {
			slog.Warn("cron: failed to count active instances", "error", err)
			continue
		}
		for _, f := range flows {
			if f.Metadata[MetaSourceFlowID] == sourceID {
				count++
			}
		}
	}
	return count
}

func (t *Trigger) cloneAndSubmit(ctx context.Context, source *core.Flow) (int64, error) {
	// 1. Clone flow.
	newFlow := &core.Flow{
		ProjectID: source.ProjectID,
		Name:      source.Name + " [cron " + time.Now().UTC().Format("01-02 15:04") + "]",
		Status:    core.FlowPending,
		Metadata: map[string]string{
			MetaSourceFlowID: strconv.FormatInt(source.ID, 10),
		},
	}
	newFlowID, err := t.store.CreateFlow(ctx, newFlow)
	if err != nil {
		return 0, fmt.Errorf("create flow clone: %w", err)
	}

	// 2. Clone steps.
	steps, err := t.store.ListStepsByFlow(ctx, source.ID)
	if err != nil {
		return 0, fmt.Errorf("list source steps: %w", err)
	}

	oldToNew := make(map[int64]int64, len(steps))
	for _, s := range steps {
		newStep := &core.Step{
			FlowID:               newFlowID,
			Name:                 s.Name,
			Description:          s.Description,
			Type:                 s.Type,
			Status:               core.StepPending,
			AgentRole:            s.AgentRole,
			RequiredCapabilities: s.RequiredCapabilities,
			AcceptanceCriteria:   s.AcceptanceCriteria,
			Timeout:              s.Timeout,
			MaxRetries:           s.MaxRetries,
			Config:               s.Config,
		}
		newStepID, err := t.store.CreateStep(ctx, newStep)
		if err != nil {
			return 0, fmt.Errorf("clone step %d: %w", s.ID, err)
		}
		oldToNew[s.ID] = newStepID
	}

	// 3. Fix DependsOn references.
	for _, s := range steps {
		if len(s.DependsOn) == 0 {
			continue
		}
		newStepID := oldToNew[s.ID]
		newDeps := make([]int64, 0, len(s.DependsOn))
		for _, dep := range s.DependsOn {
			if newDep, ok := oldToNew[dep]; ok {
				newDeps = append(newDeps, newDep)
			}
		}
		newStep, err := t.store.GetStep(ctx, newStepID)
		if err != nil {
			return 0, fmt.Errorf("get cloned step %d: %w", newStepID, err)
		}
		newStep.DependsOn = newDeps
		if err := t.store.UpdateStep(ctx, newStep); err != nil {
			return 0, fmt.Errorf("update cloned step deps %d: %w", newStepID, err)
		}
	}

	// 4. Publish event.
	t.bus.Publish(ctx, core.Event{
		Type:   core.EventFlowQueued,
		FlowID: newFlowID,
		Data: map[string]any{
			"source":       "cron",
			"template_id":  source.ID,
		},
		Timestamp: time.Now().UTC(),
	})

	// 5. Submit to scheduler.
	if err := t.scheduler.Submit(ctx, newFlowID); err != nil {
		return 0, fmt.Errorf("submit cloned flow: %w", err)
	}

	return newFlowID, nil
}

func metaBool(m map[string]string, key string) bool {
	v := strings.TrimSpace(strings.ToLower(m[key]))
	return v == "true" || v == "1" || v == "yes"
}
