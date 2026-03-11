package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	acpproto "github.com/coder/acp-go-sdk"
	"github.com/go-chi/chi/v5"
	"github.com/yoke233/ai-workflow/internal/acpclient"
	"github.com/yoke233/ai-workflow/internal/appdata"
	"github.com/yoke233/ai-workflow/internal/config"
	"github.com/yoke233/ai-workflow/internal/configruntime"
	"github.com/yoke233/ai-workflow/internal/teamleader"
	v2api "github.com/yoke233/ai-workflow/internal/v2/api"
	v2core "github.com/yoke233/ai-workflow/internal/v2/core"
	v2engine "github.com/yoke233/ai-workflow/internal/v2/engine"
	v2llm "github.com/yoke233/ai-workflow/internal/v2/llm"
	v2sqlite "github.com/yoke233/ai-workflow/internal/v2/store/sqlite"
)

// seedV2Registry seeds agent drivers and profiles into the SQLite store from TOML config.
// Uses upsert so TOML always acts as the source of truth for configured agents,
// while runtime additions via API are also persisted.
func seedV2Registry(ctx context.Context, store *v2sqlite.Store, cfg *config.Config, _ *acpclient.RoleResolver) {
	if cfg == nil {
		return
	}

	drivers, profiles := configruntime.BuildV2Agents(cfg)
	if len(drivers) == 0 {
		slog.Warn("v2 registry: no agent config to seed")
		return
	}

	for _, d := range drivers {
		if err := store.UpsertDriver(ctx, d); err != nil {
			slog.Warn("v2 registry: seed driver failed", "id", d.ID, "error", err)
		}
	}
	for _, p := range profiles {
		if err := store.UpsertProfile(ctx, p); err != nil {
			slog.Warn("v2 registry: seed profile failed", "id", p.ID, "error", err)
		}
	}
	slog.Info("v2 registry: seeded from config", "drivers", len(drivers), "profiles", len(profiles))
}

type v2GitHubTokens struct {
	CommitPAT string
	MergePAT  string
}

// bootstrapV2 creates the v2 store, event bus, engine, event persister, and API handler.
// Returns the v2 store (for lifecycle), the agent registry, runtime manager, cleanup func, and route registrar.
func bootstrapV2(v1StorePath string, roleResolver *acpclient.RoleResolver, bootstrapCfg *config.Config, mcpEnv teamleader.MCPEnvConfig, ghTokens v2GitHubTokens, upgradeFn v2engine.UpgradeFunc) (*v2sqlite.Store, v2core.AgentRegistry, *configruntime.Manager, func(), func(chi.Router)) {
	v2DBPath := strings.TrimSuffix(v1StorePath, filepath.Ext(v1StorePath)) + "_v2.db"
	v2Store, err := v2sqlite.New(v2DBPath)
	if err != nil {
		slog.Error("v2 bootstrap: failed to open store", "path", v2DBPath, "error", err)
		return nil, nil, nil, nil, nil
	}

	v2Bus := v2engine.NewMemBus()
	acpPool := v2engine.NewACPSessionPool(v2Store, v2Bus)

	persister := v2engine.NewEventPersister(v2Store, v2Bus)
	if err := persister.Start(context.Background()); err != nil {
		slog.Error("v2 bootstrap: failed to start event persister", "error", err)
		v2Store.Close()
		return nil, nil, nil, nil, nil
	}

	seedV2Registry(context.Background(), v2Store, bootstrapCfg, roleResolver)
	runtimeManager := buildV2RuntimeManager(v2Store, mcpEnv)

	var registry v2core.AgentRegistry = v2Store

	dataDir := ""
	if dd, err := appdata.ResolveDataDir(); err == nil {
		dataDir = dd
	}
	sb := buildV2Sandbox(bootstrapCfg, dataDir)

	mockEnabled := bootstrapCfg != nil && bootstrapCfg.V2.MockExecutor
	if !mockEnabled {
		if raw := strings.TrimSpace(os.Getenv("AI_WORKFLOW_V2_MOCK_EXECUTOR")); raw != "" {
			switch strings.ToLower(raw) {
			case "1", "true", "yes", "on":
				mockEnabled = true
			}
		}
	}

	var executor v2engine.StepExecutor
	if mockEnabled {
		slog.Warn("v2 bootstrap: using mock step executor (no ACP processes will be spawned)")
		executor = v2engine.NewMockStepExecutor(v2Store, v2Bus)
	} else {
		executor = v2engine.NewACPStepExecutor(v2engine.ACPExecutorConfig{
			Registry: registry,
			Store:    v2Store,
			Bus:      v2Bus,
			MCPEnv:   mcpEnv,
			MCPResolver: func(profileID string, agentSupportsSSE bool) []acpproto.McpServer {
				if runtimeManager == nil {
					return nil
				}
				return runtimeManager.ResolveMCPServers(profileID, agentSupportsSSE)
			},
			SessionPool: acpPool,
			Sandbox:     sb,
			ReworkFollowupTemplate: func() string {
				if bootstrapCfg == nil {
					return ""
				}
				return bootstrapCfg.V2.Prompts.ReworkFollowup
			}(),
			ContinueFollowupTemplate: func() string {
				if bootstrapCfg == nil {
					return ""
				}
				return bootstrapCfg.V2.Prompts.ContinueFollowup
			}(),
		})
	}

	wsProvider := v2engine.NewCompositeProvider()
	var llmClient *v2llm.Client
	engOpts := []v2engine.Option{
		v2engine.WithWorkspaceProvider(wsProvider),
		v2engine.WithGitHubTokens(v2engine.GitHubTokens{
			CommitPAT: strings.TrimSpace(ghTokens.CommitPAT),
			MergePAT:  strings.TrimSpace(ghTokens.MergePAT),
		}),
		v2engine.WithPRFlowPromptsProvider(func() v2engine.PRFlowPrompts {
			return currentV2PRFlowPrompts(runtimeManager, bootstrapCfg)
		}),
	}

	if bootstrapCfg != nil {
		openaiCfg := bootstrapCfg.V2.Collector.OpenAI
		if strings.TrimSpace(openaiCfg.APIKey) != "" && strings.TrimSpace(openaiCfg.Model) != "" {
			c, err := v2llm.New(v2llm.Config{
				BaseURL:    openaiCfg.BaseURL,
				APIKey:     openaiCfg.APIKey,
				Model:      openaiCfg.Model,
				MaxRetries: bootstrapCfg.V2.Collector.MaxRetries,
			})
			if err != nil {
				slog.Warn("v2 bootstrap: LLM client disabled (invalid openai config)", "error", err)
			} else {
				llmClient = c
				engOpts = append(engOpts, v2engine.WithCollector(v2engine.NewLLMCollector(llmClient.Complete)))
				slog.Info("v2 bootstrap: LLM client enabled (collector + DAG generator)")
			}
		}
	}

	executor = v2engine.NewCompositeStepExecutor(v2engine.CompositeStepExecutorConfig{
		Store: v2Store,
		Bus:   v2Bus,
		GitHubTokens: v2engine.GitHubTokens{
			CommitPAT: strings.TrimSpace(ghTokens.CommitPAT),
			MergePAT:  strings.TrimSpace(ghTokens.MergePAT),
		},
		UpgradeFunc: upgradeFn,
		ACPExecutor: executor,
	})

	engOpts = append(engOpts, v2engine.WithBriefingBuilder(v2engine.NewBriefingBuilder(v2Store)))
	eng := v2engine.New(v2Store, v2Bus, executor, engOpts...)

	scheduler := v2engine.NewFlowScheduler(eng, v2Store, v2Bus, v2engine.FlowSchedulerConfig{MaxConcurrentFlows: 2})
	schedCtx, schedCancel := context.WithCancel(context.Background())
	go scheduler.Start(schedCtx)

	if n, err := v2engine.RecoverInterruptedFlows(context.Background(), v2Store, scheduler); err != nil {
		slog.Warn("v2 bootstrap: flow recovery error", "error", err)
	} else if n > 0 {
		slog.Info("v2 bootstrap: recovered interrupted flows", "count", n)
	}

	leadAgent := v2engine.NewLeadAgent(v2engine.LeadAgentConfig{
		Registry: registry,
		Bus:      v2Bus,
		Sandbox:  sb,
	})

	var dagGen *v2engine.DAGGenerator
	if llmClient != nil {
		dagGen = v2engine.NewDAGGenerator(llmClient, registry)
	}

	handler := v2api.NewHandler(v2Store, v2Bus, eng, buildV2APIOptions(bootstrapCfg, runtimeManager, leadAgent, scheduler, registry, dagGen)...)
	registrar := func(r chi.Router) { handler.Register(r) }

	var runtimeWatchCancel context.CancelFunc
	cleanup := func() {
		if runtimeWatchCancel != nil {
			runtimeWatchCancel()
		}
		if runtimeManager != nil {
			_ = runtimeManager.Close()
		}
		if acpPool != nil {
			acpPool.Close()
		}
		if leadAgent != nil {
			leadAgent.Shutdown()
		}
		schedCancel()
		scheduler.Shutdown()
		persister.Stop()
		v2Store.Close()
	}

	if runtimeManager != nil {
		watchCtx, cancel := context.WithCancel(context.Background())
		runtimeWatchCancel = cancel
		if err := runtimeManager.Start(watchCtx); err != nil {
			slog.Warn("v2 bootstrap: config runtime watcher disabled", "error", err)
		}
	}

	slog.Info("v2 engine bootstrapped", "db", v2DBPath)
	return v2Store, registry, runtimeManager, cleanup, registrar
}
