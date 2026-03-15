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
	resourceprovider "github.com/yoke233/ai-workflow/internal/adapters/resource/provider"
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
	engine := buildWorkItemEngine(base.store, base.bus, executor, base.registry, base.runtimeManager, bootstrapCfg, base.dataDir, scmTokens, llmClient)
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
	cfg, source, ok := resolveFlowLLMConfig(bootstrapCfg)
	if !ok {
		return nil
	}
	client, err := llm.New(cfg)
	if err != nil {
		slog.Warn("bootstrap: LLM client disabled (invalid config)", "source", source, "error", err)
		return nil
	}
	slog.Info("bootstrap: LLM client enabled (collector + DAG generator)", "source", source)
	return client
}

func resolveFlowLLMConfig(bootstrapCfg *config.Config) (llm.Config, string, bool) {
	if bootstrapCfg == nil {
		return llm.Config{}, "", false
	}
	maxRetries := bootstrapCfg.Runtime.Collector.MaxRetries
	if cfg, ok := resolveRuntimeLLMConfig(bootstrapCfg.Runtime.LLM, maxRetries); ok {
		return cfg, "runtime.llm", true
	}
	return llm.Config{}, "", false
}

func resolveRuntimeLLMConfig(cfg config.RuntimeLLMConfig, maxRetries int) (llm.Config, bool) {
	defaultID := strings.TrimSpace(cfg.DefaultConfigID)
	if defaultID != "" {
		for _, item := range cfg.Configs {
			if strings.TrimSpace(item.ID) != defaultID {
				continue
			}
			return llmConfigFromRuntimeEntry(item, maxRetries)
		}
		return llm.Config{}, false
	}
	for _, item := range cfg.Configs {
		if llmCfg, ok := llmConfigFromRuntimeEntry(item, maxRetries); ok {
			return llmCfg, true
		}
	}
	return llm.Config{}, false
}

func llmConfigFromRuntimeEntry(item config.RuntimeLLMEntryConfig, maxRetries int) (llm.Config, bool) {
	provider := strings.ToLower(strings.TrimSpace(item.Type))
	switch provider {
	case "", llm.ProviderOpenAIResponse, llm.ProviderOpenAIChatCompletion:
	default:
		return llm.Config{}, false
	}
	apiKey := strings.TrimSpace(item.APIKey)
	model := strings.TrimSpace(item.Model)
	if apiKey == "" || model == "" {
		return llm.Config{}, false
	}
	return llm.Config{
		Provider:   provider,
		BaseURL:    strings.TrimSpace(item.BaseURL),
		APIKey:     apiKey,
		Model:      model,
		MaxRetries: maxRetries,
	}, true
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
	registry core.AgentRegistry,
	runtimeManager *configruntime.Manager,
	bootstrapCfg *config.Config,
	_ string, // dataDir reserved for future use
	scmTokens SCMTokens,
	llmClient *llm.Client,
) *flowapp.WorkItemEngine {
	// Build InputBuilder with optional registry + skills root for context injection.
	inputBuilderOpts := []flowapp.InputBuilderOption{}
	if registry != nil {
		inputBuilderOpts = append(inputBuilderOpts, flowapp.WithRegistry(registry))
	}
	if skillsRoot, err := skills.ResolveSkillsRoot(); err == nil {
		inputBuilderOpts = append(inputBuilderOpts, flowapp.WithSkillsRoot(skillsRoot))
	}

	opts := []flowapp.Option{
		flowapp.WithWorkspaceProvider(workspaceprovider.NewCompositeProvider()),
		flowapp.WithResourceResolver(flowapp.NewActionIOResolver(store, resourceprovider.NewDefaultRegistry())),
		flowapp.WithSCMTokens(flowapp.SCMTokens{
			GitHub: strings.TrimSpace(scmTokens.GitHub),
			Codeup: strings.TrimSpace(scmTokens.Codeup),
		}),
		flowapp.WithPRFlowPromptsProvider(func() flowapp.PRFlowPrompts {
			return currentPRFlowPrompts(runtimeManager, bootstrapCfg)
		}),
		flowapp.WithChangeRequestProviders(scmadapter.NewChangeRequestProviders),
		flowapp.WithInputBuilder(flowapp.NewInputBuilder(store, inputBuilderOpts...)),
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
