package bootstrap

import (
	"context"
	"log/slog"
	"os"
	"strings"

	acpproto "github.com/coder/acp-go-sdk"
	llmcollector "github.com/yoke233/ai-workflow/internal/adapters/collector/llm"
	executoradapter "github.com/yoke233/ai-workflow/internal/adapters/executor"
	"github.com/yoke233/ai-workflow/internal/adapters/llm"
	scmadapter "github.com/yoke233/ai-workflow/internal/adapters/scm"
	workspaceprovider "github.com/yoke233/ai-workflow/internal/adapters/workspace/provider"
	flowapp "github.com/yoke233/ai-workflow/internal/application/flow"
	runtimeapp "github.com/yoke233/ai-workflow/internal/application/runtime"
	"github.com/yoke233/ai-workflow/internal/audit"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/platform/config"
	"github.com/yoke233/ai-workflow/internal/platform/configruntime"
	agentruntime "github.com/yoke233/ai-workflow/internal/runtime/agent"
	"github.com/yoke233/ai-workflow/internal/skills"
)

type flowStack struct {
	sessionMode   string
	sessionMgr    runtimeapp.SessionManager
	llmClient     *llm.Client
	engine        *flowapp.WorkItemEngine
	scheduler     *flowapp.WorkItemScheduler
	schedulerStop context.CancelFunc
}

func buildFlowStack(base *bootstrapBase, bootstrapCfg *config.Config, scmTokens SCMTokens, upgradeFn executoradapter.UpgradeFunc) (*flowStack, error) {
	sb := buildSandbox(bootstrapCfg, base.runtimeManager, base.dataDir)
	acpPool := agentruntime.NewACPSessionPool(base.store, base.bus)

	sessionMgr, sessionMode := buildSessionManager(bootstrapCfg, base.store, base.dataDir, acpPool, sb)
	llmClient := buildCollectorClient(bootstrapCfg)
	executor := buildActionExecutor(base.store, base.bus, base.registry, sessionMgr, base.runtimeManager, bootstrapCfg, base.dataDir, scmTokens, upgradeFn, base.signalCfg)
	engine := buildWorkItemEngine(base.store, base.bus, executor, base.runtimeManager, bootstrapCfg, scmTokens, llmClient)
	schedulerCtx, schedulerStop := context.WithCancel(context.Background())
	schedulerCfg := resolveWorkItemSchedulerConfig(bootstrapCfg)
	scheduler := flowapp.NewWorkItemScheduler(engine, base.store, base.bus, schedulerCfg)
	go scheduler.Start(schedulerCtx)

	return &flowStack{
		sessionMode:   sessionMode,
		sessionMgr:    sessionMgr,
		llmClient:     llmClient,
		engine:        engine,
		scheduler:     scheduler,
		schedulerStop: schedulerStop,
	}, nil
}

func buildCollectorClient(bootstrapCfg *config.Config) *llm.Client {
	if bootstrapCfg == nil {
		return nil
	}
	openaiCfg := bootstrapCfg.Runtime.Collector.OpenAI
	if strings.TrimSpace(openaiCfg.APIKey) == "" || strings.TrimSpace(openaiCfg.Model) == "" {
		return nil
	}
	client, err := llm.New(llm.Config{
		BaseURL:    openaiCfg.BaseURL,
		APIKey:     openaiCfg.APIKey,
		Model:      openaiCfg.Model,
		MaxRetries: bootstrapCfg.Runtime.Collector.MaxRetries,
	})
	if err != nil {
		slog.Warn("bootstrap: LLM client disabled (invalid openai config)", "error", err)
		return nil
	}
	slog.Info("bootstrap: LLM client enabled (collector + DAG generator)")
	return client
}

func buildActionExecutor(
	store core.Store,
	bus core.EventBus,
	registry core.AgentRegistry,
	sessionMgr runtimeapp.SessionManager,
	runtimeManager *configruntime.Manager,
	bootstrapCfg *config.Config,
	dataDir string,
	scmTokens SCMTokens,
	upgradeFn executoradapter.UpgradeFunc,
	signalCfg *AgentSignalConfig,
) flowapp.ActionExecutor {
	mockEnabled := bootstrapCfg != nil && bootstrapCfg.Runtime.MockExecutor
	if !mockEnabled {
		mockEnabled = envMockExecutorEnabled()
	}

	var mcpResolver func(string, bool) []acpproto.McpServer
	if runtimeManager != nil {
		mcpResolver = runtimeManager.ResolveMCPServers
	}

	var executor flowapp.ActionExecutor
	if mockEnabled {
		slog.Warn("bootstrap: using mock action executor (no ACP processes will be spawned)")
		executor = executoradapter.NewMockActionExecutor(store, bus)
	} else {
		var auditLogger *audit.Logger
		if bootstrapCfg != nil && bootstrapCfg.Audit.Enabled {
			auditLogger = audit.NewLogger(store, audit.Config{
				Enabled:        bootstrapCfg.Audit.Enabled,
				RootDir:        audit.ResolveRootDir(dataDir, bootstrapCfg.Audit.FallbackDir),
				RedactionLevel: bootstrapCfg.Audit.RedactionLevel,
			})
		}
		acpCfg := executoradapter.ACPExecutorConfig{
			Registry:                 registry,
			Store:                    store,
			Bus:                      bus,
			SessionManager:           sessionMgr,
			MCPResolver:              mcpResolver,
			ReworkFollowupTemplate:   reworkFollowupTemplate(bootstrapCfg),
			ContinueFollowupTemplate: continueFollowupTemplate(bootstrapCfg),
			StepContextBuilder:       skills.NewActionContextBuilder(store),
			AuditLogger:              auditLogger,
		}
		if signalCfg != nil {
			acpCfg.TokenRegistry = signalCfg.TokenRegistry
			acpCfg.ServerAddr = signalCfg.ServerAddr
		}
		executor = executoradapter.NewACPActionExecutor(acpCfg)
	}

	return executoradapter.NewCompositeActionExecutor(executoradapter.CompositeStepExecutorConfig{
		Store: store,
		Bus:   bus,
		SCMTokens: flowapp.SCMTokens{
			GitHub: strings.TrimSpace(scmTokens.GitHub),
			Codeup: strings.TrimSpace(scmTokens.Codeup),
		},
		UpgradeFunc: upgradeFn,
		ACPExecutor: executor,
	})
}

func buildWorkItemEngine(
	store core.Store,
	bus core.EventBus,
	executor flowapp.ActionExecutor,
	runtimeManager *configruntime.Manager,
	bootstrapCfg *config.Config,
	scmTokens SCMTokens,
	llmClient *llm.Client,
) *flowapp.WorkItemEngine {
	opts := []flowapp.Option{
		flowapp.WithWorkspaceProvider(workspaceprovider.NewCompositeProvider()),
		flowapp.WithSCMTokens(flowapp.SCMTokens{
			GitHub: strings.TrimSpace(scmTokens.GitHub),
			Codeup: strings.TrimSpace(scmTokens.Codeup),
		}),
		flowapp.WithPRFlowPromptsProvider(func() flowapp.PRFlowPrompts {
			return currentPRFlowPrompts(runtimeManager, bootstrapCfg)
		}),
		flowapp.WithChangeRequestProviders(scmadapter.NewChangeRequestProviders),
		flowapp.WithInputBuilder(flowapp.NewInputBuilder(store)),
	}
	if llmClient != nil {
		opts = append(opts, flowapp.WithCollector(llmcollector.NewLLMCollector(llmClient.Complete)))
	}
	if bootstrapCfg != nil && bootstrapCfg.Scheduler.MaxGlobalAgents > 0 {
		opts = append(opts, flowapp.WithConcurrency(bootstrapCfg.Scheduler.MaxGlobalAgents))
	}
	return flowapp.New(store, bus, executor, opts...)
}

func resolveWorkItemSchedulerConfig(bootstrapCfg *config.Config) flowapp.WorkItemSchedulerConfig {
	schedulerCfg := flowapp.WorkItemSchedulerConfig{
		MaxConcurrentWorkItems: 2,
	}
	if bootstrapCfg != nil && bootstrapCfg.Scheduler.MaxProjectRuns > 0 {
		schedulerCfg.MaxConcurrentWorkItems = bootstrapCfg.Scheduler.MaxProjectRuns
	}
	return schedulerCfg
}

func reworkFollowupTemplate(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	return cfg.Runtime.Prompts.ReworkFollowup
}

func continueFollowupTemplate(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	return cfg.Runtime.Prompts.ContinueFollowup
}

func envMockExecutorEnabled() bool {
	raw := strings.TrimSpace(os.Getenv("AI_WORKFLOW_MOCK_EXECUTOR"))
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func recoverFlowRuntime(store core.Store, sessionMode string, scheduler *flowapp.WorkItemScheduler) {
	recoverFlows := flowapp.RecoverInterruptedFlows
	recoveryLogLabel := "interrupted flows"
	if sessionMode == "nats" {
		recoverFlows = flowapp.RecoverQueuedFlows
		recoveryLogLabel = "queued flows"
		slog.Warn("bootstrap: skipping running-flow recovery in NATS mode until execution recovery is implemented")
	}
	if n, err := recoverFlows(context.Background(), store, scheduler); err != nil {
		slog.Warn("bootstrap: flow recovery error", "error", err)
	} else if n > 0 {
		slog.Info("bootstrap: recovered flows", "kind", recoveryLogLabel, "count", n)
	}
}
