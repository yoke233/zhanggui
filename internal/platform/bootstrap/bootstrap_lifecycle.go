package bootstrap

import (
	"context"
	"log/slog"

	probeapp "github.com/yoke233/ai-workflow/internal/application/probe"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/platform/config"
	"github.com/yoke233/ai-workflow/internal/platform/configruntime"
)

type bootstrapLifecycle struct {
	runtimeWatchCancel context.CancelFunc
	probeWatchCancel   context.CancelFunc
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

	return func() {
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
	probeSvc *probeapp.ExecutionProbeService,
	bootstrapCfg *config.Config,
) {
	if bootstrapCfg == nil || !bootstrapCfg.Runtime.ExecutionProbe.Enabled || probeSvc == nil {
		return
	}

	probeWatchdog := probeapp.NewExecutionProbeWatchdog(store, probeSvc, probeapp.ExecutionProbeWatchdogConfig{
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
