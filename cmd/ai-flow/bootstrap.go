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
	"github.com/yoke233/ai-workflow/internal/teamleader"
)

var (
	recoveryOnce          sync.Once
	missingConfigHintOnce sync.Once
)

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
	exec.SetRoleResolver(bootstrapSet.RoleResolver)
	exec.SetWorkspace(bootstrapSet.Workspace)
	exec.SetRunstageRoles(cfg.RoleBinds.Run.StageRoles)
	exec.SetACPHandlerFactory(&acpHandlerFactoryAdapter{})
	mcpEnv := teamleader.MCPEnvConfig{
		DBPath: expandStorePath(cfg.Store.Path),
	}
	exec.SetMCPServerResolver(func(role acpclient.RoleProfile) []acpproto.McpServer {
		return teamleader.MCPToolsFromRoleConfig(role, mcpEnv)
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

func loadBootstrapConfig() (*config.Config, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	dataDir := filepath.Join(cwd, ".ai-workflow")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	cfgPath := filepath.Join(dataDir, "config.yaml")
	if _, statErr := os.Stat(cfgPath); statErr == nil {
		return config.LoadGlobal(cfgPath)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return nil, statErr
	}
	missingConfigHintOnce.Do(func() {
		fmt.Fprintf(os.Stderr, "config not found at %s, using built-in defaults (run `ai-flow config init` to create it)\n", cfgPath)
	})

	cfg := config.Defaults()
	if err := config.ApplyEnvOverrides(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
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
