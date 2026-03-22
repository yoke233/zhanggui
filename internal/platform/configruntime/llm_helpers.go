package configruntime

import (
	"github.com/yoke233/zhanggui/internal/platform/config"
	"github.com/yoke233/zhanggui/internal/platform/profilellm"
)

// NewProviderConfigFromEntry converts a config.RuntimeLLMEntryConfig to a
// profilellm.ProviderConfig. This helper lives in configruntime (rather than
// profilellm) to avoid a circular import: config already imports profilellm.
func NewProviderConfigFromEntry(cfg *config.RuntimeLLMEntryConfig) profilellm.ProviderConfig {
	return profilellm.ProviderConfig{
		ID:                   cfg.ID,
		Type:                 cfg.Type,
		BaseURL:              cfg.BaseURL,
		APIKey:               cfg.APIKey,
		Model:                cfg.Model,
		Temperature:          cfg.Temperature,
		MaxOutputTokens:      cfg.MaxOutputTokens,
		ReasoningEffort:      cfg.ReasoningEffort,
		ThinkingBudgetTokens: cfg.ThinkingBudgetTokens,
	}
}
