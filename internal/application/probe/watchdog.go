package probe

import (
	"context"
	"log/slog"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

type RunProbeWatchdogConfig struct {
	Enabled      bool
	Interval     time.Duration
	ProbeAfter   time.Duration
	IdleAfter    time.Duration
	ProbeTimeout time.Duration
	MaxAttempts  int
}

type RunProbeWatchdog struct {
	store   Store
	service *RunProbeService
	cfg     RunProbeWatchdogConfig
}

func NewRunProbeWatchdog(store Store, service *RunProbeService, cfg RunProbeWatchdogConfig) *RunProbeWatchdog {
	return &RunProbeWatchdog{store: store, service: service, cfg: cfg}
}

func (w *RunProbeWatchdog) Start(ctx context.Context) {
	if w == nil || !w.cfg.Enabled || w.service == nil || w.store == nil {
		return
	}

	interval := w.cfg.Interval
	if interval <= 0 {
		interval = time.Minute
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		w.runOnce(ctx)

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *RunProbeWatchdog) runOnce(ctx context.Context) {
	running, err := w.store.ListRunsByStatus(ctx, core.RunRunning)
	if err != nil {
		slog.Warn("run probe watchdog: list running runs failed", "error", err)
		return
	}

	now := time.Now().UTC()
	for _, runRec := range running {
		if runRec == nil {
			continue
		}
		if !w.shouldProbeRun(ctx, now, runRec) {
			continue
		}
		if _, err := w.service.RequestRunProbe(ctx, runRec.ID, core.RunProbeTriggerWatchdog, "", w.cfg.ProbeTimeout); err != nil && err != ErrRunProbeConflict && err != ErrRunNotRunning {
			slog.Warn("run probe watchdog: request probe failed", "run_id", runRec.ID, "error", err)
		}
	}
}

func (w *RunProbeWatchdog) shouldProbeRun(ctx context.Context, now time.Time, runRec *core.Run) bool {
	startedAt := runRec.CreatedAt
	if runRec.StartedAt != nil {
		startedAt = *runRec.StartedAt
	}
	if w.cfg.ProbeAfter > 0 && now.Sub(startedAt) < w.cfg.ProbeAfter {
		return false
	}

	if active, err := w.store.GetActiveRunProbe(ctx, runRec.ID); err == nil && active != nil {
		return false
	} else if err != nil && err != core.ErrNotFound {
		slog.Warn("run probe watchdog: read active probe failed", "run_id", runRec.ID, "error", err)
		return false
	}

	probes, err := w.store.ListRunProbesByRun(ctx, runRec.ID)
	if err != nil {
		slog.Warn("run probe watchdog: list probes failed", "run_id", runRec.ID, "error", err)
		return false
	}
	if w.cfg.MaxAttempts > 0 && len(probes) >= w.cfg.MaxAttempts {
		return false
	}

	lastActivity := startedAt
	latestEventAt, err := w.store.GetLatestRunEventTime(ctx, runRec.ID, core.EventRunAgentOutput)
	if err != nil {
		slog.Warn("run probe watchdog: latest activity lookup failed", "run_id", runRec.ID, "error", err)
		return false
	}
	if latestEventAt != nil {
		lastActivity = *latestEventAt
	}
	if w.cfg.IdleAfter > 0 && now.Sub(lastActivity) < w.cfg.IdleAfter {
		return false
	}

	return true
}
