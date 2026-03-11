package bootstrap

import (
	"context"
	"fmt"
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
)

type bootstrapBase struct {
	runtimeDBPath  string
	store          *sqlite.Store
	bus            core.EventBus
	persister      *flowapp.EventPersister
	registry       core.AgentRegistry
	runtimeManager *configruntime.Manager
	dataDir        string
}

func initBootstrapBase(storePath string, roleResolver *acpclient.RoleResolver, bootstrapCfg *config.Config) (*bootstrapBase, error) {
	runtimeDBPath := strings.TrimSuffix(storePath, filepath.Ext(storePath)) + "_runtime.db"
	store, err := sqlite.New(runtimeDBPath)
	if err != nil {
		return nil, fmt.Errorf("open runtime store %s: %w", runtimeDBPath, err)
	}

	bus := membus.NewBus()
	persister := flowapp.NewEventPersister(store, bus)
	if err := persister.Start(context.Background()); err != nil {
		store.Close()
		return nil, fmt.Errorf("start event persister: %w", err)
	}

	seedRegistry(context.Background(), store, bootstrapCfg, roleResolver)
	runtimeManager := buildRuntimeManager(store)

	dataDir := ""
	if dd, err := appdata.ResolveDataDir(); err == nil {
		dataDir = dd
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
