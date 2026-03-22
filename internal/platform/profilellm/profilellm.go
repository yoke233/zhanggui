package profilellm

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	ProviderOpenAIChatCompletion = "openai_chat_completion"
	ProviderOpenAIResponse       = "openai_response"
	ProviderAnthropic            = "anthropic"

	// LLMConfigIDSystem is a well-known sentinel value meaning "use whatever
	// API keys / credentials already exist in the system environment".
	// When a profile or chat request uses this value, LLM config resolution
	// and driver-provider compatibility checks are skipped entirely.
	LLMConfigIDSystem = "system"
)

// IsSystemLLMConfig returns true when llmConfigID is empty or the "system"
// sentinel, meaning no explicit LLM config should be resolved.
func IsSystemLLMConfig(llmConfigID string) bool {
	id := strings.TrimSpace(llmConfigID)
	return id == "" || strings.EqualFold(id, LLMConfigIDSystem)
}

type ProviderConfig struct {
	ID                   string
	Type                 string
	BaseURL              string
	APIKey               string
	Model                string
	Temperature          float64
	MaxOutputTokens      int64
	ReasoningEffort      string
	ThinkingBudgetTokens int64
}

func NormalizeProviderType(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

func ValidateDriverProviderCompatibility(driverID, launchCommand string, launchArgs []string, providerType string) error {
	providerType = NormalizeProviderType(providerType)
	if providerType == "" {
		return nil
	}

	driverKind := detectDriverKind(driverID, launchCommand, launchArgs)
	switch driverKind {
	case "codex-acp":
		if providerType != ProviderOpenAIResponse {
			return fmt.Errorf("driver %q only supports provider %q, got %q", normalizedDriverName(driverID, launchCommand), ProviderOpenAIResponse, providerType)
		}
	case "claude-acp":
		if providerType != ProviderAnthropic {
			return fmt.Errorf("driver %q only supports provider %q, got %q", normalizedDriverName(driverID, launchCommand), ProviderAnthropic, providerType)
		}
	case "agentsdk-go":
		switch providerType {
		case ProviderOpenAIChatCompletion, ProviderOpenAIResponse, ProviderAnthropic:
			return nil
		default:
			return fmt.Errorf("driver %q does not support provider %q", normalizedDriverName(driverID, launchCommand), providerType)
		}
	}
	return nil
}

func BuildEnv(cfg ProviderConfig) map[string]string {
	provider := NormalizeProviderType(cfg.Type)
	if provider == "" {
		return nil
	}

	env := map[string]string{
		"AI_WORKFLOW_LLM_PROVIDER": provider,
		"AGENTSDK_PROVIDER":        provider,
	}
	if id := strings.TrimSpace(cfg.ID); id != "" {
		env["AI_WORKFLOW_LLM_CONFIG_ID"] = id
	}
	if baseURL := strings.TrimSpace(cfg.BaseURL); baseURL != "" {
		env["AI_WORKFLOW_LLM_BASE_URL"] = baseURL
		env["AGENTSDK_BASE_URL"] = baseURL
		switch provider {
		case ProviderOpenAIChatCompletion, ProviderOpenAIResponse:
			env["OPENAI_BASE_URL"] = baseURL
		case ProviderAnthropic:
			env["ANTHROPIC_BASE_URL"] = baseURL
		}
	}
	if apiKey := strings.TrimSpace(cfg.APIKey); apiKey != "" {
		env["AI_WORKFLOW_LLM_API_KEY"] = apiKey
		env["AGENTSDK_API_KEY"] = apiKey
		switch provider {
		case ProviderOpenAIChatCompletion, ProviderOpenAIResponse:
			env["OPENAI_API_KEY"] = apiKey
		case ProviderAnthropic:
			env["ANTHROPIC_API_KEY"] = apiKey
			env["ANTHROPIC_AUTH_TOKEN"] = apiKey
		}
	}
	if model := strings.TrimSpace(cfg.Model); model != "" {
		env["AI_WORKFLOW_LLM_MODEL"] = model
		env["AGENTSDK_MODEL"] = model
		switch provider {
		case ProviderOpenAIChatCompletion, ProviderOpenAIResponse:
			env["OPENAI_MODEL"] = model
		case ProviderAnthropic:
			env["ANTHROPIC_MODEL"] = model
		}
	}

	env["AI_WORKFLOW_LLM_TEMPERATURE"] = strconv.FormatFloat(cfg.Temperature, 'f', -1, 64)
	env["AGENTSDK_TEMPERATURE"] = strconv.FormatFloat(cfg.Temperature, 'f', -1, 64)

	if cfg.MaxOutputTokens > 0 {
		value := strconv.FormatInt(cfg.MaxOutputTokens, 10)
		env["AI_WORKFLOW_LLM_MAX_OUTPUT_TOKENS"] = value
		env["AGENTSDK_MAX_OUTPUT_TOKENS"] = value
	}
	if reasoning := strings.TrimSpace(cfg.ReasoningEffort); reasoning != "" {
		env["AI_WORKFLOW_LLM_REASONING_EFFORT"] = reasoning
		env["AGENTSDK_REASONING_EFFORT"] = reasoning
	}
	if cfg.ThinkingBudgetTokens > 0 {
		value := strconv.FormatInt(cfg.ThinkingBudgetTokens, 10)
		env["AI_WORKFLOW_LLM_THINKING_BUDGET_TOKENS"] = value
		env["AGENTSDK_THINKING_BUDGET_TOKENS"] = value
	}
	return env
}

func MergeEnv(llmEnv, driverEnv map[string]string) map[string]string {
	if len(llmEnv) == 0 && len(driverEnv) == 0 {
		return nil
	}
	out := make(map[string]string, len(llmEnv)+len(driverEnv))
	for key, value := range llmEnv {
		out[key] = value
	}
	for key, value := range driverEnv {
		out[key] = value
	}
	return out
}

func detectDriverKind(driverID, launchCommand string, launchArgs []string) string {
	haystackParts := make([]string, 0, len(launchArgs)+2)
	if id := strings.ToLower(strings.TrimSpace(driverID)); id != "" {
		haystackParts = append(haystackParts, id)
	}
	if command := strings.ToLower(strings.TrimSpace(launchCommand)); command != "" {
		haystackParts = append(haystackParts, command)
	}
	for _, arg := range launchArgs {
		if trimmed := strings.ToLower(strings.TrimSpace(arg)); trimmed != "" {
			haystackParts = append(haystackParts, trimmed)
		}
	}
	haystack := strings.Join(haystackParts, " ")

	switch {
	case strings.Contains(haystack, "@zed-industries/codex-acp"), strings.Contains(haystack, "codex-acp"):
		return "codex-acp"
	case strings.Contains(haystack, "@zed-industries/claude-agent-acp"),
		strings.Contains(haystack, "claude-agent-acp"),
		strings.Contains(haystack, "claude-acp"):
		return "claude-acp"
	case strings.Contains(haystack, "agentsdk-go"), strings.Contains(haystack, "agentsdk"):
		return "agentsdk-go"
	default:
		return ""
	}
}

func normalizedDriverName(driverID, launchCommand string) string {
	if driverID = strings.TrimSpace(driverID); driverID != "" {
		return driverID
	}
	if launchCommand = strings.TrimSpace(launchCommand); launchCommand != "" {
		return launchCommand
	}
	return "unknown"
}
