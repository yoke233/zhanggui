package bootstrap

import (
	"github.com/go-chi/chi/v5"
	chatacp "github.com/yoke233/ai-workflow/internal/adapters/chat/acp"
	api "github.com/yoke233/ai-workflow/internal/adapters/http"
	llmplanning "github.com/yoke233/ai-workflow/internal/adapters/planning/llm"
	probeapp "github.com/yoke233/ai-workflow/internal/application/probe"
	"github.com/yoke233/ai-workflow/internal/platform/config"
	agentruntime "github.com/yoke233/ai-workflow/internal/runtime/agent"
)

type apiStack struct {
	leadAgent *chatacp.LeadAgent
	probeSvc  *probeapp.ExecutionProbeService
	registrar func(chi.Router)
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
		dagGen = llmplanning.NewDAGGenerator(flow.llmClient, base.registry)
	}

	probeSvc := probeapp.NewExecutionProbeService(probeapp.ExecutionProbeServiceConfig{
		Store:          base.store,
		Bus:            base.bus,
		SessionManager: flow.sessionMgr,
	})

	// Create ThreadSessionPool for real ACP agent sessions in threads.
	threadPool := agentruntime.NewThreadSessionPool(base.store, base.bus, base.registry)

	apiOpts := buildAPIOptions(bootstrapCfg, base.runtimeManager, leadAgent, flow.scheduler, base.registry, dagGen)
	apiOpts = append(apiOpts, api.WithExecutionProbeService(probeSvc))
	apiOpts = append(apiOpts, api.WithThreadAgentRuntime(threadPool))
	if base.dataDir != "" {
		apiOpts = append(apiOpts, api.WithDataDir(base.dataDir))
	}
	if flow.llmClient != nil {
		apiOpts = append(apiOpts, api.WithTextCompleter(flow.llmClient))
	}
	handler := api.NewHandler(base.store, base.bus, flow.engine, apiOpts...)

	return &apiStack{
		leadAgent: leadAgent,
		probeSvc:  probeSvc,
		registrar: func(r chi.Router) { handler.Register(r) },
	}
}
