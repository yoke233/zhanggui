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

// Metadata keys used in Issue.Metadata to define cron triggers.
const (
	MetaSchedule      = "cron_schedule"        // cron expression, e.g. "0 */6 * * *"
	MetaEnabled       = "cron_enabled"         // "true" to activate
	MetaTemplateID    = "cron_template"        // "true" marks this issue as a template (not submitted itself)
	MetaMaxInstances  = "cron_max_instances"   // max concurrent instances from this template (default 1)
	MetaSourceIssueID = "cron_source_issue_id" // set on cloned issues to trace origin
	MetaLastTriggered = "cron_last_triggered"  // ISO8601 timestamp of last trigger
)

// Store is the persistence port required by the cron trigger.
type Store interface {
	core.IssueStore
	core.StepStore
}

// Scheduler is the issue submission port.
type Scheduler interface {
	Submit(ctx context.Context, issueID int64) error
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
// issue templates and creates+submits new issue instances on schedule.
type Trigger struct {
	store     Store
	scheduler Scheduler
	bus       EventPublisher
	cfg       Config

	mu        sync.Mutex
	schedules map[int64]*templateState // issueID → state
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

type issueTemplate struct {
	issue     *core.Issue
	schedule  cronSchedule
	maxInst   int
	lastFired time.Time
}

func (t *Trigger) loadTemplates(ctx context.Context) ([]issueTemplate, error) {
	const pageSize = 200
	var templates []issueTemplate
	offset := 0

	archived := false
	for {
		issues, err := t.store.ListIssues(ctx, core.IssueFilter{
			Archived: &archived,
			Limit:    pageSize,
			Offset:   offset,
		})
		if err != nil {
			return nil, fmt.Errorf("list issues: %w", err)
		}

		for _, iss := range issues {
			tmpl, ok := parseTemplate(iss)
			if ok {
				templates = append(templates, tmpl)
			}
		}

		if len(issues) < pageSize {
			break
		}
		offset += pageSize
	}
	return templates, nil
}

func parseTemplate(iss *core.Issue) (issueTemplate, bool) {
	if iss == nil || iss.Metadata == nil {
		return issueTemplate{}, false
	}
	if !metaBool(iss.Metadata, MetaEnabled) {
		return issueTemplate{}, false
	}
	if !metaBool(iss.Metadata, MetaTemplateID) {
		return issueTemplate{}, false
	}
	expr := metaString(iss.Metadata, MetaSchedule)
	if expr == "" {
		return issueTemplate{}, false
	}

	sched, err := parseCron(expr)
	if err != nil {
		slog.Warn("cron: invalid schedule", "issue_id", iss.ID, "expr", expr, "error", err)
		return issueTemplate{}, false
	}

	maxInst := 1
	if v := metaString(iss.Metadata, MetaMaxInstances); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxInst = n
		}
	}

	var lastFired time.Time
	if v := metaString(iss.Metadata, MetaLastTriggered); v != "" {
		if parsed, err := time.Parse(time.RFC3339, v); err == nil {
			lastFired = parsed
		}
	}

	return issueTemplate{
		issue:     iss,
		schedule:  sched,
		maxInst:   maxInst,
		lastFired: lastFired,
	}, true
}

func (t *Trigger) processTemplate(ctx context.Context, tmpl issueTemplate, now time.Time) {
	t.mu.Lock()
	state, ok := t.schedules[tmpl.issue.ID]
	if !ok {
		state = &templateState{
			schedule:  tmpl.schedule,
			lastFired: tmpl.lastFired,
			maxInst:   tmpl.maxInst,
		}
		t.schedules[tmpl.issue.ID] = state
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
	activeCount := t.countActiveInstances(ctx, tmpl.issue.ID)
	if activeCount >= tmpl.maxInst {
		slog.Debug("cron: skipping trigger, max instances reached",
			"template_issue_id", tmpl.issue.ID,
			"active", activeCount,
			"max", tmpl.maxInst,
		)
		return
	}

	// Clone and submit.
	newIssueID, err := t.cloneAndSubmit(ctx, tmpl.issue)
	if err != nil {
		slog.Error("cron: clone+submit failed", "template_issue_id", tmpl.issue.ID, "error", err)
		return
	}

	// Update in-memory state.
	t.mu.Lock()
	state.lastFired = now
	t.mu.Unlock()

	// Persist lastTriggered back to template metadata.
	tmpl.issue.Metadata[MetaLastTriggered] = now.Format(time.RFC3339)
	if err := t.store.UpdateIssueMetadata(ctx, tmpl.issue.ID, tmpl.issue.Metadata); err != nil {
		slog.Warn("cron: failed to persist last_triggered", "template_issue_id", tmpl.issue.ID, "error", err)
	}

	slog.Info("cron: triggered issue",
		"template_issue_id", tmpl.issue.ID,
		"new_issue_id", newIssueID,
		"schedule", metaString(tmpl.issue.Metadata, MetaSchedule),
	)
}

// countActiveInstances counts non-terminal issues cloned from the given template.
func (t *Trigger) countActiveInstances(ctx context.Context, templateIssueID int64) int {
	sourceID := strconv.FormatInt(templateIssueID, 10)
	count := 0

	// Check active statuses: open, accepted, queued, running, blocked.
	for _, status := range []core.IssueStatus{core.IssueOpen, core.IssueAccepted, core.IssueQueued, core.IssueRunning, core.IssueBlocked} {
		offset := 0
		for {
			issues, err := t.store.ListIssues(ctx, core.IssueFilter{
				Status: &status,
				Limit:  100,
				Offset: offset,
			})
			if err != nil {
				slog.Warn("cron: failed to count active instances", "error", err)
				break
			}
			for _, iss := range issues {
				if metaString(iss.Metadata, MetaSourceIssueID) == sourceID {
					count++
				}
			}
			if len(issues) < 100 {
				break
			}
			offset += 100
		}
	}
	return count
}

func (t *Trigger) cloneAndSubmit(ctx context.Context, source *core.Issue) (int64, error) {
	// 1. Clone issue.
	newIssue := &core.Issue{
		ProjectID:         source.ProjectID,
		ResourceBindingID: source.ResourceBindingID,
		Title:             source.Title + " [cron " + time.Now().UTC().Format("01-02 15:04") + "]",
		Status:            core.IssueOpen,
		Metadata: map[string]any{
			MetaSourceIssueID: strconv.FormatInt(source.ID, 10),
		},
	}
	newIssueID, err := t.store.CreateIssue(ctx, newIssue)
	if err != nil {
		return 0, fmt.Errorf("create issue clone: %w", err)
	}

	// 2. Clone steps.
	steps, err := t.store.ListStepsByIssue(ctx, source.ID)
	if err != nil {
		return 0, fmt.Errorf("list source steps: %w", err)
	}

	for i, s := range steps {
		newStep := &core.Step{
			IssueID:              newIssueID,
			Name:                 s.Name,
			Description:          s.Description,
			Type:                 s.Type,
			Status:               core.StepPending,
			Position:             s.Position,
			AgentRole:            s.AgentRole,
			RequiredCapabilities: s.RequiredCapabilities,
			AcceptanceCriteria:   s.AcceptanceCriteria,
			Timeout:              s.Timeout,
			MaxRetries:           s.MaxRetries,
			Config:               s.Config,
		}
		if newStep.Position < 0 {
			newStep.Position = i
		}
		if _, err := t.store.CreateStep(ctx, newStep); err != nil {
			return 0, fmt.Errorf("clone step %d: %w", s.ID, err)
		}
	}

	// 3. Publish event.
	t.bus.Publish(ctx, core.Event{
		Type:    core.EventIssueQueued,
		IssueID: newIssueID,
		Data: map[string]any{
			"source":      "cron",
			"template_id": source.ID,
		},
		Timestamp: time.Now().UTC(),
	})

	// 4. Submit to scheduler.
	if err := t.scheduler.Submit(ctx, newIssueID); err != nil {
		return 0, fmt.Errorf("submit cloned issue: %w", err)
	}

	return newIssueID, nil
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
