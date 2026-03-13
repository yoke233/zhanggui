package bootstrap

import (
	"context"
	"path/filepath"
	"testing"

	membus "github.com/yoke233/ai-workflow/internal/adapters/events/memory"
	"github.com/yoke233/ai-workflow/internal/adapters/store/sqlite"
	flowapp "github.com/yoke233/ai-workflow/internal/application/flow"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/platform/config"
)

func TestResolveFlowSchedulerConfigUsesConfiguredProjectRuns(t *testing.T) {
	t.Parallel()

	cfg := config.Defaults()
	cfg.Scheduler.MaxProjectRuns = 5

	got := resolveFlowSchedulerConfig(&cfg)
	if got.MaxConcurrentIssues != 5 {
		t.Fatalf("MaxConcurrentIssues = %d, want 5", got.MaxConcurrentIssues)
	}
	if got.MaxConcurrentFlows != 5 {
		t.Fatalf("MaxConcurrentFlows = %d, want 5", got.MaxConcurrentFlows)
	}
}

func TestResolveFlowSchedulerConfigDefaults(t *testing.T) {
	t.Parallel()

	got := resolveFlowSchedulerConfig(nil)
	if got.MaxConcurrentIssues != 2 {
		t.Fatalf("MaxConcurrentIssues = %d, want 2", got.MaxConcurrentIssues)
	}
	if got.MaxConcurrentFlows != 2 {
		t.Fatalf("MaxConcurrentFlows = %d, want 2", got.MaxConcurrentFlows)
	}
}

func TestBuildFlowEngineAppliesConfiguredAgentConcurrency(t *testing.T) {
	t.Parallel()

	store := newBootstrapTestStore(t)
	bus := membus.NewBus()
	cfg := config.Defaults()
	cfg.Scheduler.MaxGlobalAgents = 6

	engine := buildFlowEngine(store, bus, noopStepExecutor, nil, &cfg, SCMTokens{}, nil)
	if got := engine.MaxConcurrency(); got != 6 {
		t.Fatalf("engine.MaxConcurrency() = %d, want 6", got)
	}
}

func TestBuildFlowEngineUsesDefaultConcurrencyWhenUnset(t *testing.T) {
	t.Parallel()

	store := newBootstrapTestStore(t)
	bus := membus.NewBus()
	cfg := config.Defaults()
	cfg.Scheduler.MaxGlobalAgents = 0

	engine := buildFlowEngine(store, bus, noopStepExecutor, nil, &cfg, SCMTokens{}, nil)
	if got := engine.MaxConcurrency(); got != 4 {
		t.Fatalf("engine.MaxConcurrency() = %d, want 4", got)
	}
}

func newBootstrapTestStore(t *testing.T) *sqlite.Store {
	t.Helper()

	store, err := sqlite.New(filepath.Join(t.TempDir(), "bootstrap-test.db"))
	if err != nil {
		t.Fatalf("sqlite.New() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}

func noopStepExecutor(context.Context, *core.Step, *core.Execution) error {
	return nil
}

var _ flowapp.StepExecutor = noopStepExecutor
