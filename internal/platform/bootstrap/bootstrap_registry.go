package bootstrap

import (
	"context"
	"log/slog"

	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
	"github.com/yoke233/ai-workflow/internal/adapters/store/sqlite"
	"github.com/yoke233/ai-workflow/internal/platform/config"
	"github.com/yoke233/ai-workflow/internal/platform/configruntime"
)

// seedRegistry seeds agent profiles into the SQLite store from TOML config.
// Uses upsert so TOML always acts as the source of truth for configured agents,
// while runtime additions via API are also persisted.
func seedRegistry(ctx context.Context, store *sqlite.Store, cfg *config.Config, _ *acpclient.RoleResolver) {
	if cfg == nil {
		return
	}

	profiles := configruntime.BuildAgents(cfg)
	if len(profiles) == 0 {
		slog.Warn("registry: no agent config to seed")
		return
	}

	for _, p := range profiles {
		if err := store.UpsertProfile(ctx, p); err != nil {
			slog.Warn("registry: seed profile failed", "id", p.ID, "error", err)
		}
	}
	slog.Info("registry: seeded from config", "profiles", len(profiles))
}

func SeedRegistry(ctx context.Context, store *sqlite.Store, cfg *config.Config) {
	seedRegistry(ctx, store, cfg, nil)
}
