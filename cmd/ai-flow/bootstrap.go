package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/yoke233/ai-workflow/internal/acpclient"
	"github.com/yoke233/ai-workflow/internal/config"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/engine"
	"github.com/yoke233/ai-workflow/internal/eventbus"
	pluginfactory "github.com/yoke233/ai-workflow/internal/plugins/factory"
	storesqlite "github.com/yoke233/ai-workflow/internal/plugins/store-sqlite"
	"github.com/yoke233/ai-workflow/internal/teamleader"
)

var recoveryOnce sync.Once

func bootstrap() (*engine.Executor, core.Store, error) {
	exec, bootstrapSet, _, err := bootstrapWithEventBus()
	if err != nil {
		return nil, nil, err
	}
	return exec, bootstrapSet.Store, nil
}

func bootstrapWithEventBus() (*engine.Executor, *pluginfactory.BootstrapSet, core.EventBus, error) {
	cfg, err := loadBootstrapConfig()
	if err != nil {
		return nil, nil, nil, err
	}

	bootstrapSet, err := pluginfactory.BuildFromConfig(*cfg)
	if err != nil {
		return nil, nil, nil, err
	}
	if bootstrapSet.Workspace == nil {
		return nil, nil, nil, errors.New("workspace plugin is not configured in bootstrap set")
	}

	bus := eventbus.New()
	logger := slog.Default()
	exec := engine.NewExecutor(bootstrapSet.Store, bus, logger)
	if memory := memoryFromStore(bootstrapSet.Store); memory != nil {
		exec.SetMemory(memory)
	}
	exec.SetRoleResolver(bootstrapSet.RoleResolver)
	exec.SetWorkspace(bootstrapSet.Workspace)
	exec.SetRunstageRoles(cfg.RoleBinds.Run.StageRoles)
	exec.SetACPHandlerFactory(&acpHandlerFactoryAdapter{})
	mcpEnv := teamleader.MCPEnvConfig{
		DBPath: expandStorePath(cfg.Store.Path),
	}
	exec.SetMCPServerResolver(func(role acpclient.RoleProfile, sseSupported bool) []acpproto.McpServer {
		return teamleader.MCPToolsFromRoleConfig(role, mcpEnv, sseSupported)
	})

	recoveryOnce.Do(func() {
		go func() {
			if recErr := exec.RecoverActiveRuns(context.Background()); recErr != nil {
				logger.Error("recovery failed", "error", recErr)
			}
		}()
	})

	return exec, bootstrapSet, bus, nil
}

func memoryFromStore(store core.Store) core.Memory {
	sqliteStore, ok := store.(*storesqlite.SQLiteStore)
	if !ok || sqliteStore == nil {
		return nil
	}
	return storesqlite.NewSQLiteMemory(sqliteStore)
}

// resolveDataDir returns the data directory for config, secrets, and database.
// Priority: $AI_WORKFLOW_DATA_DIR > $CWD/.ai-workflow
func resolveDataDir() (string, error) {
	if env := os.Getenv("AI_WORKFLOW_DATA_DIR"); env != "" {
		return filepath.Abs(env)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(cwd, ".ai-workflow"), nil
}

func loadBootstrapConfig() (*config.Config, error) {
	dataDir, err := resolveDataDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	cfgPath := filepath.Join(dataDir, "config.toml")
	secretsPath := secretsFilePath(dataDir)

	// Ensure secrets exist (auto-generate tokens if needed).
	secrets, err := config.LoadSecrets(secretsPath)
	if err != nil {
		return nil, err
	}
	if config.EnsureSecrets(secrets) {
		if saveErr := config.SaveSecrets(secretsPath, secrets); saveErr != nil {
			return nil, fmt.Errorf("save bootstrap secrets: %w", saveErr)
		}
	}

	// Auto-generate config if missing (equivalent to `config init`).
	if _, statErr := os.Stat(cfgPath); errors.Is(statErr, os.ErrNotExist) {
		content, genErr := loadDefaultConfigTemplate()
		if genErr != nil {
			return nil, fmt.Errorf("generate default config: %w", genErr)
		}
		if writeErr := os.WriteFile(cfgPath, content, 0o644); writeErr != nil {
			return nil, fmt.Errorf("write default config: %w", writeErr)
		}
		fmt.Fprintf(os.Stderr, "config auto-initialized: %s\n", cfgPath)
	} else if statErr != nil {
		return nil, statErr
	}

	cfg, err := config.LoadGlobal(cfgPath, secretsPath)
	if err != nil {
		return nil, err
	}

	// Auto-generate missing required secrets and persist.
	if config.EnsureSecrets(secrets) {
		config.ApplySecrets(cfg, secrets)
		if saveErr := config.SaveSecrets(secretsPath, secrets); saveErr != nil {
			slog.Warn("failed to save secrets", "error", saveErr)
		} else {
			slog.Info("generated secrets saved", "path", secretsPath, "admin_token", secrets.AdminToken())
		}
	}

	// Re-validate after secrets are applied.
	if err := config.Validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func mergeBootstrapProjectConfig(base *config.Config, repoPath string) (*config.Config, error) {
	if base == nil {
		return nil, errors.New("base config is required")
	}
	trimmedRepoPath := strings.TrimSpace(repoPath)
	if trimmedRepoPath == "" {
		return config.MergeForRun(base, nil, nil)
	}
	projectLayer, err := config.LoadProject(trimmedRepoPath)
	if err != nil {
		return nil, err
	}
	return config.MergeForRun(base, projectLayer, nil)
}

func expandStorePath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return trimmed
	}
	if strings.HasPrefix(trimmed, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			trimmed = filepath.Join(home, trimmed[2:])
		}
	}
	if !filepath.IsAbs(trimmed) {
		if abs, err := filepath.Abs(trimmed); err == nil {
			return abs
		}
	}
	return trimmed
}
