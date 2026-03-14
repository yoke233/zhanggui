package bootstrap

import (
	"github.com/go-chi/chi/v5"
	chatacp "github.com/yoke233/ai-workflow/internal/adapters/chat/acp"
	api "github.com/yoke233/ai-workflow/internal/adapters/http"
	llmplanning "github.com/yoke233/ai-workflow/internal/adapters/planning/llm"
	inspectionapp "github.com/yoke233/ai-workflow/internal/application/inspection"
	planningapp "github.com/yoke233/ai-workflow/internal/application/planning"
	probeapp "github.com/yoke233/ai-workflow/internal/application/probe"
	"github.com/yoke233/ai-workflow/internal/platform/config"
	agentruntime "github.com/yoke233/ai-workflow/internal/runtime/agent"
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
	leadAgent := chatacp.NewLeadAgent(chatacp.LeadAgentConfig{
		Registry:             base.registry,
		Bus:                  base.bus,
		ResourceBindingStore: base.store,
		LLM:                  llmCompleter,
		Sandbox:              sb,
		DataDir:              base.dataDir,
	})

	var dagGen api.DAGGenerator
	if flow.llmClient != nil {
		completer := llmplanning.NewCompleter(flow.llmClient)
		dagGen = planningapp.NewService(completer, base.registry)
	}

	probeSvc := probeapp.NewRunProbeService(probeapp.RunProbeServiceConfig{
		Store:          base.store,
		Bus:            base.bus,
		SessionManager: flow.sessionMgr,
	})

	// Create ThreadSessionPool for real ACP agent sessions in threads.
	threadPool := agentruntime.NewThreadSessionPool(base.store, base.bus, base.registry)

	apiOpts := buildAPIOptions(bootstrapCfg, base.runtimeManager, leadAgent, flow.scheduler, base.registry, dagGen)
	apiOpts = append(apiOpts, api.WithRunProbeService(probeSvc))
	apiOpts = append(apiOpts, api.WithThreadAgentRuntime(threadPool))
	if base.dataDir != "" {
		apiOpts = append(apiOpts, api.WithDataDir(base.dataDir))
	}
	if flow.llmClient != nil {
		apiOpts = append(apiOpts, api.WithTextCompleter(flow.llmClient))
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
