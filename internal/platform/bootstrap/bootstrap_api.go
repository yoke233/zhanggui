package bootstrap

import (
	"context"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	chatacp "github.com/yoke233/zhanggui/internal/adapters/chat/acp"
	api "github.com/yoke233/zhanggui/internal/adapters/http"
	llmplanning "github.com/yoke233/zhanggui/internal/adapters/planning/llm"
	scmadapter "github.com/yoke233/zhanggui/internal/adapters/scm"
	inspectionapp "github.com/yoke233/zhanggui/internal/application/inspection"
	planningapp "github.com/yoke233/zhanggui/internal/application/planning"
	probeapp "github.com/yoke233/zhanggui/internal/application/probe"
	"github.com/yoke233/zhanggui/internal/core"
	"github.com/yoke233/zhanggui/internal/platform/config"
	agentruntime "github.com/yoke233/zhanggui/internal/runtime/agent"
)

type apiStack struct {
	leadAgent        *chatacp.LeadAgent
	probeSvc         *probeapp.RunProbeService
	inspectionEngine *inspectionapp.Engine
	registrar        func(chi.Router)
}

func buildAPIStack(
	base *bootstrapBase,
	flow *flowStack,
	bootstrapCfg *config.Config,
) *apiStack {
	sb := buildSandbox(bootstrapCfg, base.runtimeManager, base.dataDir)
	var llmCompleter chatacp.TextCompleter
	if flow.llmClient != nil {
		llmCompleter = flow.llmClient
	}
	var driverResolver chatacp.DriverResolver
	var llmConfigResolver chatacp.LLMConfigResolver
	if base.runtimeManager != nil {
		driverResolver = func(_ context.Context, driverID string) (*core.DriverConfig, error) {
			return base.runtimeManager.ResolveDriverConfig(driverID)
		}
		llmConfigResolver = func(_ context.Context, llmConfigID string) (*config.RuntimeLLMEntryConfig, error) {
			return base.runtimeManager.ResolveLLMConfig(llmConfigID)
		}
	}
	gcCfg := bootstrapCfg.Runtime.Sandbox.GC
	gitPAT := ""
	if bootstrapCfg != nil && bootstrapCfg.GitHub.Token != "" {
		gitPAT = bootstrapCfg.GitHub.Token
	}
	leadAgent := chatacp.NewLeadAgent(chatacp.LeadAgentConfig{
		Registry:               base.registry,
		DriverResolver:         driverResolver,
		LLMConfigResolver:      llmConfigResolver,
		Bus:                    base.bus,
		ResourceSpaceStore:     base.store,
		LLM:                    llmCompleter,
		Sandbox:                sb,
		DataDir:                base.dataDir,
		ChangeRequestProviders: scmadapter.NewChangeRequestProviders,
		GitPAT:                 gitPAT,
		GC: chatacp.GCConfig{
			ArchiveCleanup: gcCfg.ArchiveCleanup,
			StartupCleanup: gcCfg.StartupCleanup,
			Interval:       gcCfg.Interval.Duration,
			RepoMaxAge:     gcCfg.RepoMaxAge.Duration,
		},
	})

	var dagGen api.DAGGenerator
	if flow.llmClient != nil {
		completer := llmplanning.NewCompleter(flow.llmClient)
		skillsRoot := ""
		if base.dataDir != "" {
			skillsRoot = filepath.Join(base.dataDir, "skills")
		}
		dagGen = planningapp.NewService(completer, base.registry, planningapp.WithPlanningSkillsRoot(skillsRoot))
	}

	probeSvc := probeapp.NewRunProbeService(probeapp.RunProbeServiceConfig{
		Store:          base.store,
		Bus:            base.bus,
		SessionManager: flow.sessionMgr,
	})

	// Create ThreadSessionPool for real ACP agent sessions in threads.
	threadPool := agentruntime.NewThreadSessionPool(base.store, base.bus, base.registry, base.dataDir)
	threadPool.SetThreadSharedBootTemplate(bootstrapCfg.Runtime.Prompts.ThreadSharedBootTemplate)
	if base.signalCfg != nil {
		threadPool.SetSignalConfig(base.signalCfg.ServerAddr, base.signalCfg.TokenRegistry)
	}

	apiOpts := buildAPIOptions(bootstrapCfg, base.runtimeManager, leadAgent, flow.scheduler, base.registry, dagGen)
	apiOpts = append(apiOpts, api.WithRunProbeService(probeSvc))
	apiOpts = append(apiOpts, api.WithThreadAgentRuntime(threadPool))
	if base.runtimeManager != nil {
		apiOpts = append(apiOpts, api.WithDriverConfigService(base.runtimeManager))
	}
	if base.dataDir != "" {
		apiOpts = append(apiOpts, api.WithDataDir(base.dataDir))
	}
	if flow.llmClient != nil {
		apiOpts = append(apiOpts, api.WithTextCompleter(flow.llmClient))
		apiOpts = append(apiOpts, api.WithRequirementCompleter(llmplanning.NewCompleter(flow.llmClient)))
	}

	// Inspection engine for self-evolving system inspections.
	inspEngine := inspectionapp.New(base.store, base.bus)
	apiOpts = append(apiOpts, api.WithInspectionEngine(inspEngine))

	handler := api.NewHandler(base.store, base.bus, flow.engine, apiOpts...)

	return &apiStack{
		leadAgent:        leadAgent,
		probeSvc:         probeSvc,
		inspectionEngine: inspEngine,
		registrar:        func(r chi.Router) { handler.Register(r) },
	}
}
