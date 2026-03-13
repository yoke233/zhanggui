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

func TestResolveWorkItemSchedulerConfigUsesConfiguredProjectRuns(t *testing.T) {
	t.Parallel()

	cfg := config.Defaults()
	cfg.Scheduler.MaxProjectRuns = 5

	got := resolveWorkItemSchedulerConfig(&cfg)
	if got.MaxConcurrentWorkItems != 5 {
		t.Fatalf("MaxConcurrentWorkItems = %d, want 5", got.MaxConcurrentWorkItems)
	}
}

func TestResolveWorkItemSchedulerConfigDefaults(t *testing.T) {
	t.Parallel()

	got := resolveWorkItemSchedulerConfig(nil)
	if got.MaxConcurrentWorkItems != 2 {
		t.Fatalf("MaxConcurrentWorkItems = %d, want 2", got.MaxConcurrentWorkItems)
	}
}

func TestBuildWorkItemEngineAppliesConfiguredAgentConcurrency(t *testing.T) {
	t.Parallel()

	store := newBootstrapTestStore(t)
	bus := membus.NewBus()
	cfg := config.Defaults()
	cfg.Scheduler.MaxGlobalAgents = 6

	engine := buildWorkItemEngine(store, bus, noopActionExecutor, nil, &cfg, SCMTokens{}, nil)
	if got := engine.MaxConcurrency(); got != 6 {
		t.Fatalf("engine.MaxConcurrency() = %d, want 6", got)
	}
}

func TestBuildWorkItemEngineUsesDefaultConcurrencyWhenUnset(t *testing.T) {
	t.Parallel()

	store := newBootstrapTestStore(t)
	bus := membus.NewBus()
	cfg := config.Defaults()
	cfg.Scheduler.MaxGlobalAgents = 0

	engine := buildWorkItemEngine(store, bus, noopActionExecutor, nil, &cfg, SCMTokens{}, nil)
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

func noopActionExecutor(context.Context, *core.Action, *core.Run) error {
	return nil
}

var _ flowapp.ActionExecutor = noopActionExecutor
