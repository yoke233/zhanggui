package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/nats-io/nats.go"
	"github.com/yoke233/ai-workflow/internal/adapters/agent/acpclient"
	chatacp "github.com/yoke233/ai-workflow/internal/adapters/chat/acp"
	llmcollector "github.com/yoke233/ai-workflow/internal/adapters/collector/llm"
	membus "github.com/yoke233/ai-workflow/internal/adapters/events/memory"
	executoradapter "github.com/yoke233/ai-workflow/internal/adapters/executor"
	api "github.com/yoke233/ai-workflow/internal/adapters/http"
	"github.com/yoke233/ai-workflow/internal/adapters/llm"
	llmplanning "github.com/yoke233/ai-workflow/internal/adapters/planning/llm"
	"github.com/yoke233/ai-workflow/internal/adapters/sandbox"
	scmadapter "github.com/yoke233/ai-workflow/internal/adapters/scm"
	"github.com/yoke233/ai-workflow/internal/adapters/store/sqlite"
	workspaceprovider "github.com/yoke233/ai-workflow/internal/adapters/workspace/provider"
	flowapp "github.com/yoke233/ai-workflow/internal/application/flow"
	probeapp "github.com/yoke233/ai-workflow/internal/application/probe"
	runtimeapp "github.com/yoke233/ai-workflow/internal/application/runtime"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/platform/appdata"
	"github.com/yoke233/ai-workflow/internal/platform/config"
	"github.com/yoke233/ai-workflow/internal/platform/configruntime"
	agentruntime "github.com/yoke233/ai-workflow/internal/runtime/agent"
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

type flowStack struct {
	sessionMode   string
	sessionMgr    runtimeapp.SessionManager
	llmClient     *llm.Client
	engine        *flowapp.FlowEngine
	scheduler     *flowapp.FlowScheduler
	schedulerStop context.CancelFunc
}

type apiStack struct {
	leadAgent *chatacp.LeadAgent
	probeSvc  *probeapp.ExecutionProbeService
	registrar func(chi.Router)
}

type bootstrapLifecycle struct {
	runtimeWatchCancel context.CancelFunc
	probeWatchCancel   context.CancelFunc
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

func buildFlowStack(base *bootstrapBase, bootstrapCfg *config.Config, ghTokens gitHubTokens, upgradeFn executoradapter.UpgradeFunc) (*flowStack, error) {
	sb := buildSandbox(bootstrapCfg, base.dataDir)
	acpPool := agentruntime.NewACPSessionPool(base.store, base.bus)

	sessionMgr, sessionMode := buildSessionManager(bootstrapCfg, base.store, base.dataDir, acpPool, sb)
	llmClient := buildCollectorClient(bootstrapCfg)
	executor := buildStepExecutor(base.store, base.bus, base.registry, sessionMgr, bootstrapCfg, ghTokens, upgradeFn)
	engine := buildFlowEngine(base.store, base.bus, executor, base.runtimeManager, bootstrapCfg, ghTokens, llmClient)
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

func buildSessionManager(
	bootstrapCfg *config.Config,
	store core.Store,
	dataDir string,
	acpPool *agentruntime.ACPSessionPool,
	sb sandbox.Sandbox,
) (runtimeapp.SessionManager, string) {
	smMode := ""
	if bootstrapCfg != nil {
		smMode = strings.TrimSpace(strings.ToLower(bootstrapCfg.Runtime.SessionManager.Mode))
	}

	local := func() runtimeapp.SessionManager {
		return agentruntime.NewLocalSessionManager(acpPool, store, sb)
	}

	if smMode == "nats" {
		natsMgr, err := buildNATSSessionManager(bootstrapCfg, store, dataDir)
		if err != nil {
			slog.Error("bootstrap: NATS session manager failed, falling back to local", "error", err)
			slog.Info("bootstrap: using local session manager")
			return local(), smMode
		}
		slog.Info("bootstrap: using NATS session manager")
		return natsMgr, smMode
	}

	slog.Info("bootstrap: using local session manager")
	return local(), smMode
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
	bootstrapCfg *config.Config,
	ghTokens gitHubTokens,
	upgradeFn executoradapter.UpgradeFunc,
) flowapp.StepExecutor {
	mockEnabled := bootstrapCfg != nil && bootstrapCfg.Runtime.MockExecutor
	if !mockEnabled {
		mockEnabled = envMockExecutorEnabled()
	}

	var executor flowapp.StepExecutor
	if mockEnabled {
		slog.Warn("bootstrap: using mock step executor (no ACP processes will be spawned)")
		executor = executoradapter.NewMockStepExecutor(store, bus)
	} else {
		executor = executoradapter.NewACPStepExecutor(executoradapter.ACPExecutorConfig{
			Registry:                 registry,
			Store:                    store,
			Bus:                      bus,
			SessionManager:           sessionMgr,
			ReworkFollowupTemplate:   reworkFollowupTemplate(bootstrapCfg),
			ContinueFollowupTemplate: continueFollowupTemplate(bootstrapCfg),
		})
	}

	return executoradapter.NewCompositeStepExecutor(executoradapter.CompositeStepExecutorConfig{
		Store: store,
		Bus:   bus,
		GitHubTokens: flowapp.GitHubTokens{
			CommitPAT: strings.TrimSpace(ghTokens.CommitPAT),
			MergePAT:  strings.TrimSpace(ghTokens.MergePAT),
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
	ghTokens gitHubTokens,
	llmClient *llm.Client,
) *flowapp.FlowEngine {
	opts := []flowapp.Option{
		flowapp.WithWorkspaceProvider(workspaceprovider.NewCompositeProvider()),
		flowapp.WithGitHubTokens(flowapp.GitHubTokens{
			CommitPAT: strings.TrimSpace(ghTokens.CommitPAT),
			MergePAT:  strings.TrimSpace(ghTokens.MergePAT),
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

func buildAPIStack(
	base *bootstrapBase,
	flow *flowStack,
	bootstrapCfg *config.Config,
) *apiStack {
	sb := buildSandbox(bootstrapCfg, base.dataDir)
	leadAgent := chatacp.NewLeadAgent(chatacp.LeadAgentConfig{
		Registry: base.registry,
		Bus:      base.bus,
		Sandbox:  sb,
	})

	var dagGen api.DAGGenerator
	if flow.llmClient != nil {
		dagGen = llmplanning.NewDAGGenerator(flow.llmClient, base.registry)
	}

	probeSvc := probeapp.NewExecutionProbeService(probeapp.ExecutionProbeServiceConfig{
		Store:          base.store,
		Bus:            base.bus,
		SessionManager: flow.sessionMgr,
	})

	apiOpts := buildAPIOptions(bootstrapCfg, base.runtimeManager, leadAgent, flow.scheduler, base.registry, dagGen)
	apiOpts = append(apiOpts, api.WithExecutionProbeService(probeSvc))
	handler := api.NewHandler(base.store, base.bus, flow.engine, apiOpts...)

	return &apiStack{
		leadAgent: leadAgent,
		probeSvc:  probeSvc,
		registrar: func(r chi.Router) { handler.Register(r) },
	}
}

func startBootstrapLifecycle(
	base *bootstrapBase,
	flow *flowStack,
	apiStack *apiStack,
	bootstrapCfg *config.Config,
) func() {
	lifecycle := &bootstrapLifecycle{}
	startRuntimeWatcher(lifecycle, base.runtimeManager)
	startProbeWatchdog(lifecycle, base.store, apiStack.probeSvc, bootstrapCfg)

	return func() {
		if lifecycle.runtimeWatchCancel != nil {
			lifecycle.runtimeWatchCancel()
		}
		if lifecycle.probeWatchCancel != nil {
			lifecycle.probeWatchCancel()
		}
		if base.runtimeManager != nil {
			_ = base.runtimeManager.Close()
		}
		if flow.sessionMgr != nil {
			flow.sessionMgr.Close()
		}
		if apiStack.leadAgent != nil {
			apiStack.leadAgent.Shutdown()
		}
		flow.schedulerStop()
		flow.scheduler.Shutdown()
		base.persister.Stop()
		base.store.Close()
	}
}

func startRuntimeWatcher(lifecycle *bootstrapLifecycle, runtimeManager *configruntime.Manager) {
	if runtimeManager == nil {
		return
	}

	watchCtx, cancel := context.WithCancel(context.Background())
	lifecycle.runtimeWatchCancel = cancel
	if err := runtimeManager.Start(watchCtx); err != nil {
		slog.Warn("bootstrap: config runtime watcher disabled", "error", err)
	}
}

func startProbeWatchdog(
	lifecycle *bootstrapLifecycle,
	store core.Store,
	probeSvc *probeapp.ExecutionProbeService,
	bootstrapCfg *config.Config,
) {
	if bootstrapCfg == nil || !bootstrapCfg.Runtime.ExecutionProbe.Enabled || probeSvc == nil {
		return
	}

	probeWatchdog := probeapp.NewExecutionProbeWatchdog(store, probeSvc, probeapp.ExecutionProbeWatchdogConfig{
		Enabled:      bootstrapCfg.Runtime.ExecutionProbe.Enabled,
		Interval:     bootstrapCfg.Runtime.ExecutionProbe.Interval.Duration,
		ProbeAfter:   bootstrapCfg.Runtime.ExecutionProbe.After.Duration,
		IdleAfter:    bootstrapCfg.Runtime.ExecutionProbe.IdleAfter.Duration,
		ProbeTimeout: bootstrapCfg.Runtime.ExecutionProbe.Timeout.Duration,
		MaxAttempts:  bootstrapCfg.Runtime.ExecutionProbe.MaxAttempts,
	})
	watchCtx, cancel := context.WithCancel(context.Background())
	lifecycle.probeWatchCancel = cancel
	go probeWatchdog.Start(watchCtx)
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

func buildNATSSessionManager(cfg *config.Config, store core.Store, _ string) (*agentruntime.NATSSessionManager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	natsCfg := cfg.Runtime.SessionManager.NATS

	natsURL := strings.TrimSpace(natsCfg.URL)
	if natsURL == "" && !natsCfg.Embedded {
		return nil, fmt.Errorf("nats.url is required when mode=nats and embedded=false")
	}
	if natsCfg.Embedded && natsURL == "" {
		return nil, fmt.Errorf("embedded NATS not yet implemented; provide nats.url")
	}

	nc, err := natsConnect(natsURL)
	if err != nil {
		return nil, fmt.Errorf("connect to NATS: %w", err)
	}

	prefix := strings.TrimSpace(natsCfg.StreamPrefix)
	if prefix == "" {
		prefix = "aiworkflow"
	}

	return agentruntime.NewNATSSessionManager(agentruntime.NATSSessionManagerConfig{
		NATSConn:     nc,
		StreamPrefix: prefix,
		ServerID:     strings.TrimSpace(cfg.Runtime.SessionManager.ServerID),
		Store:        store,
	})
}

func natsConnect(url string) (*nats.Conn, error) {
	opts := []nats.Option{
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(10),
		nats.ReconnectWait(2 * time.Second),
	}
	if strings.TrimSpace(url) == "" {
		return nil, fmt.Errorf("empty nats url")
	}
	return nats.Connect(url, opts...)
}
