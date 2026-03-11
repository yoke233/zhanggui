package scm

import (
	"context"

	flowapp "github.com/yoke233/ai-workflow/internal/application/flow"
)

func NewChangeRequestProviders(token string) []flowapp.ChangeRequestProvider {
	return []flowapp.ChangeRequestProvider{
		NewGitHubProvider(token),
		NewCodeupProvider(CodeupProviderConfig{
			Token: token,
		}),
	}
}

func DetectChangeRequestProvider(ctx context.Context, originURL string, providers []flowapp.ChangeRequestProvider) (flowapp.ChangeRequestProvider, flowapp.ChangeRequestRepo, bool, error) {
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		repo, ok, err := provider.Detect(ctx, originURL)
		if err != nil {
			return nil, flowapp.ChangeRequestRepo{}, false, err
		}
		if ok {
			return provider, repo, true, nil
		}
	}
	return nil, flowapp.ChangeRequestRepo{}, false, nil
}
