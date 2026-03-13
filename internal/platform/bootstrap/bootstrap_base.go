package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
	membus "github.com/yoke233/ai-workflow/internal/adapters/events/memory"
	"github.com/yoke233/ai-workflow/internal/adapters/store/sqlite"
	flowapp "github.com/yoke233/ai-workflow/internal/application/flow"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/platform/appdata"
	"github.com/yoke233/ai-workflow/internal/platform/config"
	"github.com/yoke233/ai-workflow/internal/platform/configruntime"
	"github.com/yoke233/ai-workflow/internal/skills"
)

type bootstrapBase struct {
	runtimeDBPath  string
	store          *sqlite.Store
	bus            core.EventBus
	persister      *flowapp.EventPersister
	registry       core.AgentRegistry
	runtimeManager *configruntime.Manager
	dataDir        string
	signalCfg      *AgentSignalConfig
}

func initBootstrapBase(storePath string, roleResolver *acpclient.RoleResolver, bootstrapCfg *config.Config) (*bootstrapBase, error) {
	runtimeDBPath := strings.TrimSuffix(storePath, filepath.Ext(storePath)) + "_runtime.db"
	fmt.Println("[startup] init base: open runtime store")
	store, err := sqlite.New(runtimeDBPath)
	if err != nil {
		return nil, fmt.Errorf("open runtime store %s: %w", runtimeDBPath, err)
	}

	fmt.Println("[startup] init base: create event bus")
	bus := membus.NewBus()
	fmt.Println("[startup] init base: start event persister")
	persister := flowapp.NewEventPersister(store, bus)
	if err := persister.Start(context.Background()); err != nil {
		store.Close()
		return nil, fmt.Errorf("start event persister: %w", err)
	}

	fmt.Println("[startup] init base: seed registry")
	seedRegistry(context.Background(), store, bootstrapCfg, roleResolver)
	fmt.Println("[startup] init base: build runtime manager")
	runtimeManager := buildRuntimeManager(store, runtimeDBPath)

	fmt.Println("[startup] init base: resolve data dir")
	dataDir := ""
	if dd, err := appdata.ResolveDataDir(); err == nil {
		dataDir = dd
	}

	// Extract embedded builtin skills to <dataDir>/skills/ on startup.
	if dataDir != "" {
		fmt.Println("[startup] init base: ensure builtin skills")
		skillsRoot := filepath.Join(dataDir, "skills")
		if err := skills.EnsureBuiltinSkills(skillsRoot); err != nil {
			slog.Warn("bootstrap: failed to extract builtin skills", "error", err)
		}
	}

	return &bootstrapBase{
		runtimeDBPath:  runtimeDBPath,
		store:          store,
		bus:            bus,
		persister:      persister,
		registry:       store,
		runtimeManager: runtimeManager,
		dataDir:        dataDir,
	}, nil
}
