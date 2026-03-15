package bootstrap

import (
	"context"
	"log/slog"

	cronapp "github.com/yoke233/ai-workflow/internal/application/cron"
	flowapp "github.com/yoke233/ai-workflow/internal/application/flow"
	inspectionapp "github.com/yoke233/ai-workflow/internal/application/inspection"
	probeapp "github.com/yoke233/ai-workflow/internal/application/probe"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/platform/config"
	"github.com/yoke233/ai-workflow/internal/platform/configruntime"
)

type bootstrapLifecycle struct {
	runtimeWatchCancel context.CancelFunc
	probeWatchCancel   context.CancelFunc
	cronCancel         context.CancelFunc
	inspectionCancel   context.CancelFunc
	inspectionEngine   *inspectionapp.Engine
}

func startBootstrapLifecycle(
	base *bootstrapBase,
	flow *flowStack,
	apiStack *apiStack,
	bootstrapCfg *config.Config,
) func() {
	lifecycle := &bootstrapLifecycle{}
	startRuntimeWatcher(lifecycle, base.runtimeManager)
	startProbeWatchdog(lifecycle, base.store, apiStack.probeSvc, bootstrapCfg)
	startCronTrigger(lifecycle, base.store, base.bus, flow.scheduler, bootstrapCfg)
	startInspectionScheduler(lifecycle, apiStack.inspectionEngine, base.bus, bootstrapCfg)

	return func() {
		if lifecycle.inspectionCancel != nil {
			lifecycle.inspectionCancel()
		}
		if lifecycle.cronCancel != nil {
			lifecycle.cronCancel()
		}
		if lifecycle.runtimeWatchCancel != nil {
			lifecycle.runtimeWatchCancel()
		}
		if lifecycle.probeWatchCancel != nil {
			lifecycle.probeWatchCancel()
		}
		if base.runtimeManager != nil {
			_ = base.runtimeManager.Close()
		}
		if flow.sessionMgr != nil {
			flow.sessionMgr.Close()
		}
		if apiStack.leadAgent != nil {
			apiStack.leadAgent.Shutdown()
		}
		flow.schedulerStop()
		flow.scheduler.Shutdown()
		base.persister.Stop()
		base.store.Close()
	}
}

func startRuntimeWatcher(lifecycle *bootstrapLifecycle, runtimeManager *configruntime.Manager) {
	if runtimeManager == nil {
		return
	}

	watchCtx, cancel := context.WithCancel(context.Background())
	lifecycle.runtimeWatchCancel = cancel
	if err := runtimeManager.Start(watchCtx); err != nil {
		slog.Warn("bootstrap: config runtime watcher disabled", "error", err)
	}
}

func startProbeWatchdog(
	lifecycle *bootstrapLifecycle,
	store core.Store,
	probeSvc *probeapp.RunProbeService,
	bootstrapCfg *config.Config,
) {
	if bootstrapCfg == nil || !bootstrapCfg.Runtime.ExecutionProbe.Enabled || probeSvc == nil {
		return
	}

	probeWatchdog := probeapp.NewRunProbeWatchdog(store, probeSvc, probeapp.RunProbeWatchdogConfig{
		Enabled:      bootstrapCfg.Runtime.ExecutionProbe.Enabled,
		Interval:     bootstrapCfg.Runtime.ExecutionProbe.Interval.Duration,
		ProbeAfter:   bootstrapCfg.Runtime.ExecutionProbe.After.Duration,
		IdleAfter:    bootstrapCfg.Runtime.ExecutionProbe.IdleAfter.Duration,
		ProbeTimeout: bootstrapCfg.Runtime.ExecutionProbe.Timeout.Duration,
		MaxAttempts:  bootstrapCfg.Runtime.ExecutionProbe.MaxAttempts,
	})
	watchCtx, cancel := context.WithCancel(context.Background())
	lifecycle.probeWatchCancel = cancel
	go probeWatchdog.Start(watchCtx)
}

func startCronTrigger(
	lifecycle *bootstrapLifecycle,
	store core.Store,
	bus core.EventBus,
	scheduler *flowapp.WorkItemScheduler,
	bootstrapCfg *config.Config,
) {
	if bootstrapCfg == nil || !bootstrapCfg.Runtime.Cron.Enabled {
		return
	}

	trigger := cronapp.New(store, scheduler, bus, cronapp.Config{
		Enabled:  true,
		Interval: bootstrapCfg.Runtime.Cron.Interval.Duration,
	})
	ctx, cancel := context.WithCancel(context.Background())
	lifecycle.cronCancel = cancel
	go trigger.Start(ctx)
}

func startInspectionScheduler(
	lifecycle *bootstrapLifecycle,
	engine *inspectionapp.Engine,
	bus core.EventBus,
	bootstrapCfg *config.Config,
) {
	if bootstrapCfg == nil || !bootstrapCfg.Runtime.Inspection.Enabled || engine == nil {
		return
	}

	lifecycle.inspectionEngine = engine

	scheduler := inspectionapp.NewScheduler(engine, bus, inspectionapp.SchedulerConfig{
		Enabled:   true,
		Interval:  bootstrapCfg.Runtime.Inspection.Interval.Duration,
		LookbackH: bootstrapCfg.Runtime.Inspection.LookbackH,
	})
	ctx, cancel := context.WithCancel(context.Background())
	lifecycle.inspectionCancel = cancel
	go scheduler.Start(ctx)
}
