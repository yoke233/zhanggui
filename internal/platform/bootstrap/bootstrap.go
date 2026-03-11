package bootstrap

import (
	"context"
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
	executoradapter "github.com/yoke233/ai-workflow/internal/adapters/executor"
	"github.com/yoke233/ai-workflow/internal/adapters/store/sqlite"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/platform/config"
	"github.com/yoke233/ai-workflow/internal/platform/configruntime"
)

// seedRegistry seeds agent drivers and profiles into the SQLite store from TOML config.
// Uses upsert so TOML always acts as the source of truth for configured agents,
// while runtime additions via API are also persisted.
func seedRegistry(ctx context.Context, store *sqlite.Store, cfg *config.Config, _ *acpclient.RoleResolver) {
	if cfg == nil {
		return
	}

	drivers, profiles := configruntime.BuildAgents(cfg)
	if len(drivers) == 0 {
		slog.Warn("registry: no agent config to seed")
		return
	}

	for _, d := range drivers {
		if err := store.UpsertDriver(ctx, d); err != nil {
			slog.Warn("registry: seed driver failed", "id", d.ID, "error", err)
		}
	}
	for _, p := range profiles {
		if err := store.UpsertProfile(ctx, p); err != nil {
			slog.Warn("registry: seed profile failed", "id", p.ID, "error", err)
		}
	}
	slog.Info("registry: seeded from config", "drivers", len(drivers), "profiles", len(profiles))
}

type gitHubTokens struct {
	CommitPAT string
	MergePAT  string
}

// bootstrap creates the runtime store, event bus, engine, event persister, and API handler.
// Returns the store (for lifecycle), the agent registry, runtime manager, cleanup func, and route registrar.
func bootstrap(storePath string, roleResolver *acpclient.RoleResolver, bootstrapCfg *config.Config, ghTokens gitHubTokens, upgradeFn executoradapter.UpgradeFunc) (*sqlite.Store, core.AgentRegistry, *configruntime.Manager, func(), func(chi.Router)) {
	base, err := initBootstrapBase(storePath, roleResolver, bootstrapCfg)
	if err != nil {
		slog.Error("bootstrap: init failed", "error", err)
		return nil, nil, nil, nil, nil
	}
	flow, err := buildFlowStack(base, bootstrapCfg, ghTokens, upgradeFn)
	if err != nil {
		slog.Error("bootstrap: flow assembly failed", "error", err)
		base.persister.Stop()
		base.store.Close()
		return nil, nil, nil, nil, nil
	}
	recoverFlowRuntime(base.store, flow.sessionMode, flow.scheduler)
	apiStack := buildAPIStack(base, flow, bootstrapCfg)
	cleanup := startBootstrapLifecycle(base, flow, apiStack, bootstrapCfg)

	slog.Info("engine bootstrapped", "db", base.runtimeDBPath)
	return base.store, base.registry, base.runtimeManager, cleanup, apiStack.registrar
}
