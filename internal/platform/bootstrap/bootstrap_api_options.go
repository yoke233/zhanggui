package bootstrap

import (
	"path/filepath"

	"github.com/yoke233/ai-workflow/internal/adapters/http"
	"github.com/yoke233/ai-workflow/internal/adapters/sandbox"
	flowapp "github.com/yoke233/ai-workflow/internal/application/flow"
	"github.com/yoke233/ai-workflow/internal/core"
	"github.com/yoke233/ai-workflow/internal/platform/appdata"
	"github.com/yoke233/ai-workflow/internal/platform/config"
	"github.com/yoke233/ai-workflow/internal/platform/configruntime"
)

func buildAPIOptions(
	bootstrapCfg *config.Config,
	runtimeManager *configruntime.Manager,
	leadAgent api.LeadChatService,
	scheduler flowapp.Scheduler,
	registry core.AgentRegistry,
	dagGen api.DAGGenerator,
) []api.HandlerOption {
	fallback := config.RuntimeSandboxConfig{}
	if bootstrapCfg != nil {
		fallback = bootstrapCfg.Runtime.Sandbox
	}
	skillsRoot := ""
	if dataDir, err := appdata.ResolveDataDir(); err == nil {
		skillsRoot = filepath.Join(dataDir, "skills")
	}

	// Resolve effective PAT for git tag push.
	gitPAT := ""
	if bootstrapCfg != nil {
		if v := bootstrapCfg.GitHub.Token; v != "" {
			gitPAT = v
		}
	}

	return []api.HandlerOption{
		api.WithLeadAgent(leadAgent),
		api.WithScheduler(scheduler),
		api.WithRegistry(registry),
		api.WithDAGGenerator(dagGen),
		api.WithSandboxController(sandbox.NewRuntimeControlService(runtimeManager, fallback)),
		api.WithSkillsRoot(skillsRoot),
		api.WithGitPAT(gitPAT),
		api.WithPRFlowPromptsProvider(func() flowapp.PRFlowPrompts {
			return currentPRFlowPrompts(runtimeManager, bootstrapCfg)
		}),
	}
}
