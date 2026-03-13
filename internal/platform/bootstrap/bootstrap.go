package bootstrap

import (
	"fmt"
	"log/slog"

	"github.com/go-chi/chi/v5"
	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
	executoradapter "github.com/yoke233/ai-workflow/internal/adapters/executor"
	httpx "github.com/yoke233/ai-workflow/internal/adapters/http/server"
	"github.com/yoke233/ai-workflow/internal/adapters/store/sqlite"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/platform/config"
	"github.com/yoke233/ai-workflow/internal/platform/configruntime"
)

type SCMTokens struct {
	GitHub string
	Codeup string
}

// AgentSignalConfig holds config for skill-based agent signal injection.
type AgentSignalConfig struct {
	TokenRegistry *httpx.TokenRegistry
	ServerAddr    string // e.g. "http://127.0.0.1:8080"
}

// Build creates the runtime store, event bus, engine, event persister, and API handler.
// Returns the store (for lifecycle), the agent registry, runtime manager, cleanup func, and route registrar.
func Build(storePath string, roleResolver *acpclient.RoleResolver, bootstrapCfg *config.Config, scmTokens SCMTokens, upgradeFn executoradapter.UpgradeFunc, signalCfg *AgentSignalConfig) (*sqlite.Store, core.AgentRegistry, *configruntime.Manager, func(), func(chi.Router)) {
	fmt.Println("[startup] bootstrap: init base")
	base, err := initBootstrapBase(storePath, roleResolver, bootstrapCfg)
	if err != nil {
		slog.Error("bootstrap: init failed", "error", err)
		return nil, nil, nil, nil, nil
	}
	base.signalCfg = signalCfg
	fmt.Println("[startup] bootstrap: build flow stack")
	flow, err := buildFlowStack(base, bootstrapCfg, scmTokens, upgradeFn)
	if err != nil {
		slog.Error("bootstrap: flow assembly failed", "error", err)
		base.persister.Stop()
		base.store.Close()
		return nil, nil, nil, nil, nil
	}
	fmt.Println("[startup] bootstrap: recover flow runtime")
	recoverFlowRuntime(base.store, flow.sessionMode, flow.scheduler)
	fmt.Println("[startup] bootstrap: build api stack")
	apiStack := buildAPIStack(base, flow, bootstrapCfg)
	fmt.Println("[startup] bootstrap: start lifecycle")
	cleanup := startBootstrapLifecycle(base, flow, apiStack, bootstrapCfg)

	slog.Info("engine bootstrapped", "db", base.runtimeDBPath)
	return base.store, base.registry, base.runtimeManager, cleanup, apiStack.registrar
}
