package inspection

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// SchedulerConfig configures the inspection scheduler.
type SchedulerConfig struct {
	Enabled   bool
	Interval  time.Duration // how often to run inspections (default 24h)
	LookbackH int           // hours of data to inspect (default 24)
	ProjectID *int64        // optional: scope to a specific project
}

// Scheduler is a background service that periodically triggers inspection runs.
type Scheduler struct {
	engine *Engine
	bus    EventPublisher
	cfg    SchedulerConfig

	mu      sync.Mutex
	running bool
}

// NewScheduler creates a new inspection scheduler.
func NewScheduler(engine *Engine, bus EventPublisher, cfg SchedulerConfig) *Scheduler {
	if cfg.Interval == 0 {
		cfg.Interval = 24 * time.Hour
	}
	if cfg.LookbackH == 0 {
		cfg.LookbackH = 24
	}
	return &Scheduler{
		engine: engine,
		bus:    bus,
		cfg:    cfg,
	}
}

// Start begins the periodic inspection loop. Blocks until ctx is cancelled.
func (s *Scheduler) Start(ctx context.Context) {
	if !s.cfg.Enabled {
		slog.Info("inspection scheduler disabled")
		return
	}

	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	slog.Info("inspection scheduler started",
		"interval", s.cfg.Interval,
		"lookback_hours", s.cfg.LookbackH,
	)

	// Run immediately on startup, then on interval.
	s.runOnce(ctx)

	ticker := time.NewTicker(s.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("inspection scheduler stopped")
			return
		case <-ticker.C:
			s.runOnce(ctx)
		}
	}
}

func (s *Scheduler) runOnce(ctx context.Context) {
	now := time.Now()
	periodStart := now.Add(-time.Duration(s.cfg.LookbackH) * time.Hour)

	slog.Info("inspection scheduler: running inspection",
		"period_start", periodStart,
		"period_end", now,
	)

	report, err := s.engine.RunInspection(ctx, core.InspectionTriggerCron, s.cfg.ProjectID, periodStart, now)
	if err != nil {
		slog.Error("inspection scheduler: inspection failed", "error", err)
		return
	}

	slog.Info("inspection scheduler: inspection completed",
		"id", report.ID,
		"findings", len(report.Findings),
		"insights", len(report.Insights),
		"suggested_skills", len(report.SuggestedSkills),
	)
}
