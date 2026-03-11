package bootstrap

import (
	"github.com/go-chi/chi/v5"
	chatacp "github.com/yoke233/ai-workflow/internal/adapters/chat/acp"
	api "github.com/yoke233/ai-workflow/internal/adapters/http"
	llmplanning "github.com/yoke233/ai-workflow/internal/adapters/planning/llm"
	probeapp "github.com/yoke233/ai-workflow/internal/application/probe"
	"github.com/yoke233/ai-workflow/internal/platform/config"
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
