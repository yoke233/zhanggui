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
	engine        *flowapp.FlowEngine
	scheduler     *flowapp.FlowScheduler
	schedulerStop context.CancelFunc
}

func buildFlowStack(base *bootstrapBase, bootstrapCfg *config.Config, scmTokens SCMTokens, upgradeFn executoradapter.UpgradeFunc) (*flowStack, error) {
	sb := buildSandbox(bootstrapCfg, base.runtimeManager, base.dataDir)
	acpPool := agentruntime.NewACPSessionPool(base.store, base.bus)

	sessionMgr, sessionMode := buildSessionManager(bootstrapCfg, base.store, base.dataDir, acpPool, sb)
	llmClient := buildCollectorClient(bootstrapCfg)
	executor := buildStepExecutor(base.store, base.bus, base.registry, sessionMgr, base.runtimeManager, bootstrapCfg, scmTokens, upgradeFn, base.signalCfg)
	engine := buildFlowEngine(base.store, base.bus, executor, base.runtimeManager, bootstrapCfg, scmTokens, llmClient)
	schedulerCtx, schedulerStop := context.WithCancel(context.Background())
	scheduler := flowapp.NewFlowScheduler(engine, base.store, base.bus, flowapp.FlowSchedulerConfig{MaxConcurrentFlows: 2})
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

func buildStepExecutor(
	store core.Store,
	bus core.EventBus,
	registry core.AgentRegistry,
	sessionMgr runtimeapp.SessionManager,
	runtimeManager *configruntime.Manager,
	bootstrapCfg *config.Config,
	scmTokens SCMTokens,
	upgradeFn executoradapter.UpgradeFunc,
	signalCfg *AgentSignalConfig,
) flowapp.StepExecutor {
	mockEnabled := bootstrapCfg != nil && bootstrapCfg.Runtime.MockExecutor
	if !mockEnabled {
		mockEnabled = envMockExecutorEnabled()
	}

	var mcpResolver func(string, bool) []acpproto.McpServer
	if runtimeManager != nil {
		mcpResolver = runtimeManager.ResolveMCPServers
	}

	var executor flowapp.StepExecutor
	if mockEnabled {
		slog.Warn("bootstrap: using mock step executor (no ACP processes will be spawned)")
		executor = executoradapter.NewMockStepExecutor(store, bus)
	} else {
		acpCfg := executoradapter.ACPExecutorConfig{
			Registry:                 registry,
			Store:                    store,
			Bus:                      bus,
			SessionManager:           sessionMgr,
			MCPResolver:              mcpResolver,
			ReworkFollowupTemplate:   reworkFollowupTemplate(bootstrapCfg),
			ContinueFollowupTemplate: continueFollowupTemplate(bootstrapCfg),
			StepContextBuilder:       skills.NewStepContextBuilder(store),
		}
		if signalCfg != nil {
			acpCfg.TokenRegistry = signalCfg.TokenRegistry
			acpCfg.ServerAddr = signalCfg.ServerAddr
		}
		executor = executoradapter.NewACPStepExecutor(acpCfg)
	}

	return executoradapter.NewCompositeStepExecutor(executoradapter.CompositeStepExecutorConfig{
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

func buildFlowEngine(
	store core.Store,
	bus core.EventBus,
	executor flowapp.StepExecutor,
	runtimeManager *configruntime.Manager,
	bootstrapCfg *config.Config,
	scmTokens SCMTokens,
	llmClient *llm.Client,
) *flowapp.FlowEngine {
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
		flowapp.WithBriefingBuilder(flowapp.NewBriefingBuilder(store)),
	}
	if llmClient != nil {
		opts = append(opts, flowapp.WithCollector(llmcollector.NewLLMCollector(llmClient.Complete)))
	}
	return flowapp.New(store, bus, executor, opts...)
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

func recoverFlowRuntime(store core.Store, sessionMode string, scheduler *flowapp.FlowScheduler) {
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
