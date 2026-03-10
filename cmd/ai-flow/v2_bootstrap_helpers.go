package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/yoke233/ai-workflow/internal/appdata"
	"github.com/yoke233/ai-workflow/internal/config"
	"github.com/yoke233/ai-workflow/internal/configruntime"
	"github.com/yoke233/ai-workflow/internal/teamleader"
	v2api "github.com/yoke233/ai-workflow/internal/v2/api"
	v2core "github.com/yoke233/ai-workflow/internal/v2/core"
	v2engine "github.com/yoke233/ai-workflow/internal/v2/engine"
	v2sandbox "github.com/yoke233/ai-workflow/internal/v2/sandbox"
	v2sqlite "github.com/yoke233/ai-workflow/internal/v2/store/sqlite"
)

func buildV2Sandbox(cfg *config.Config, dataDir string) v2sandbox.Sandbox {
	if cfg == nil || !cfg.V2.Sandbox.Enabled {
		return v2sandbox.NoopSandbox{}
	}

	requireAuth := false
	if raw := strings.ToLower(strings.TrimSpace(os.Getenv("AI_WORKFLOW_CODEX_REQUIRE_AUTH"))); raw != "" {
		switch raw {
		case "1", "true", "yes", "on":
			requireAuth = true
		}
	}

	homeSandbox := v2sandbox.HomeDirSandbox{
		DataDir:          dataDir,
		SkillsRoot:       filepath.Join(dataDir, "skills"),
		RequireCodexAuth: requireAuth,
	}

	switch strings.ToLower(strings.TrimSpace(cfg.V2.Sandbox.Provider)) {
	case "", "home_dir":
		return homeSandbox
	case "litebox":
		return v2sandbox.LiteBoxSandbox{
			Base:          homeSandbox,
			BridgeCommand: strings.TrimSpace(cfg.V2.Sandbox.LiteBox.BridgeCommand),
			BridgeArgs:    append([]string(nil), cfg.V2.Sandbox.LiteBox.BridgeArgs...),
			RunnerPath:    strings.TrimSpace(cfg.V2.Sandbox.LiteBox.RunnerPath),
			RunnerArgs:    append([]string(nil), cfg.V2.Sandbox.LiteBox.RunnerArgs...),
		}
	default:
		slog.Warn("v2 sandbox: unknown provider, fallback to home_dir", "provider", cfg.V2.Sandbox.Provider)
		return homeSandbox
	}
}

func buildV2RuntimeManager(v2Store *v2sqlite.Store, mcpEnv teamleader.MCPEnvConfig) *configruntime.Manager {
	dataDir, err := appdata.ResolveDataDir()
	if err != nil {
		return nil
	}

	cfgPath := filepath.Join(dataDir, "config.toml")
	secretsPath := secretsFilePath(dataDir)
	runtimeManager, err := configruntime.NewManager(cfgPath, secretsPath, mcpEnv, slog.Default(), func(ctx context.Context, snap *configruntime.Snapshot) error {
		return configruntime.SyncRegistry(ctx, v2Store, snap)
	})
	if err != nil {
		slog.Warn("v2 bootstrap: config runtime disabled", "error", err)
		return nil
	}
	return runtimeManager
}

func buildV2APIOptions(
	bootstrapCfg *config.Config,
	leadAgent *v2engine.LeadAgent,
	scheduler *v2engine.FlowScheduler,
	registry v2core.AgentRegistry,
	dagGen *v2engine.DAGGenerator,
) []v2api.HandlerOption {
	enabled := bootstrapCfg != nil && bootstrapCfg.V2.Sandbox.Enabled
	provider := ""
	if bootstrapCfg != nil {
		provider = bootstrapCfg.V2.Sandbox.Provider
	}
	skillsRoot := ""
	if dataDir, err := appdata.ResolveDataDir(); err == nil {
		skillsRoot = filepath.Join(dataDir, "skills")
	}

	return []v2api.HandlerOption{
		v2api.WithLeadAgent(leadAgent),
		v2api.WithScheduler(scheduler),
		v2api.WithRegistry(registry),
		v2api.WithDAGGenerator(dagGen),
		v2api.WithSandboxInspector(v2sandbox.NewDefaultSupportInspector(enabled, provider)),
		v2api.WithSkillsRoot(skillsRoot),
	}
}
