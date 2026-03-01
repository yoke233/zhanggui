package reviewgithubpr

import (
	"fmt"

	"github.com/user/ai-workflow/internal/config"
	"github.com/user/ai-workflow/internal/core"
	githubsvc "github.com/user/ai-workflow/internal/github"
)

func Module() core.PluginModule {
	return core.PluginModule{
		Name: "review-github-pr",
		Slot: core.SlotReviewGate,
		Factory: func(cfg map[string]any) (core.Plugin, error) {
			if cfg == nil {
				return nil, fmt.Errorf("review-github-pr requires store dependency")
			}
			rawStore, ok := cfg["store"]
			if !ok {
				return nil, fmt.Errorf("review-github-pr requires store dependency")
			}
			store, ok := rawStore.(core.Store)
			if !ok || store == nil {
				return nil, fmt.Errorf("review-github-pr requires valid store dependency")
			}

			var client prClient
			if githubCfg, ok := cfg["github"].(config.GitHubConfig); ok {
				ghClient, err := githubsvc.NewClient(githubCfg)
				if err != nil {
					return nil, fmt.Errorf("review-github-pr build github client: %w", err)
				}
				service, err := githubsvc.NewGitHubService(ghClient, githubCfg.Owner, githubCfg.Repo)
				if err != nil {
					return nil, fmt.Errorf("review-github-pr build github service: %w", err)
				}
				client = service
			}

			return New(store, client), nil
		},
	}
}
