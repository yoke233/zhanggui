package bootstrap

import (
	"context"
	"log/slog"
	"path/filepath"

	"github.com/yoke233/ai-workflow/internal/adapters/store/sqlite"
	"github.com/yoke233/ai-workflow/internal/platform/appdata"
	"github.com/yoke233/ai-workflow/internal/platform/configruntime"
)

func buildRuntimeManager(store *sqlite.Store, runtimeDBPath string) *configruntime.Manager {
	dataDir, err := appdata.ResolveDataDir()
	if err != nil {
		return nil
	}

	cfgPath := filepath.Join(dataDir, "config.toml")
	secretsPath := resolveSecretsFilePath(dataDir)
	mcpEnv := configruntime.MCPEnvConfig{
		DBPath: runtimeDBPath,
	}
	runtimeManager, err := configruntime.NewManager(cfgPath, secretsPath, mcpEnv, slog.Default(), func(ctx context.Context, snap *configruntime.Snapshot) error {
		return configruntime.SyncRegistry(ctx, store, snap)
	})
	if err != nil {
		slog.Warn("bootstrap: config runtime disabled", "error", err)
		return nil
	}
	return runtimeManager
}
