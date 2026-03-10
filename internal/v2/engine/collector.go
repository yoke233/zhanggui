package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/responses"
	"github.com/openai/openai-go/shared"
	"github.com/yoke233/ai-workflow/internal/v2/core"
)

// LLMCollector extracts structured metadata from agent markdown output
// by calling a small LLM to produce JSON (Structured Outputs / JSON schema).
type LLMCollector struct {
	// Complete is the LLM completion function injected by the caller.
	// prompt is the fully assembled extraction prompt.
	// tools carries the JSON schema used to define the expected JSON output.
	// Returns the raw JSON output.
	Complete func(ctx context.Context, prompt string, tools []ToolDef) (json.RawMessage, error)
}

// ToolDef describes a tool for the LLM to call.
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// NewLLMCollector creates a Collector backed by an LLM completion function.
// The Complete function should call a small model (Haiku) with tool_use forced,
// and return the tool input JSON.
func NewLLMCollector(complete func(ctx context.Context, prompt string, tools []ToolDef) (json.RawMessage, error)) *LLMCollector {
	return &LLMCollector{Complete: complete}
}

// Extract implements the Collector interface.
func (c *LLMCollector) Extract(ctx context.Context, stepType core.StepType, markdown string) (map[string]any, error) {
	if c.Complete == nil {
		return nil, fmt.Errorf("LLMCollector.Complete is not set")
	}

	prompt := buildExtractionPrompt(stepType, markdown)
	tools := extractionTools(stepType)

	raw, err := c.Complete(ctx, prompt, tools)
	if err != nil {
		return nil, fmt.Errorf("llm extract for %s: %w", stepType, err)
	}

	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse extraction result: %w", err)
	}
	return result, nil
}

// buildExtractionPrompt creates the system+user prompt for metadata extraction.
func buildExtractionPrompt(stepType core.StepType, markdown string) string {
	var instruction string
	switch stepType {
	case core.StepGate:
		instruction = `You are a metadata extractor. Analyze the following gate review output and extract:
- verdict: "pass" or "reject"
 - reason: a short, human-readable reason (empty string if unclear)
 - reject_targets: list of upstream step IDs to reset when verdict is "reject" (optional; omit if unclear)
Return ONLY a JSON object matching the provided JSON schema.`
	case core.StepComposite:
		instruction = `You are a metadata extractor. Analyze the following composite step output and extract:
- sub_tasks: list of sub-task names/descriptions identified
Return ONLY a JSON object matching the provided JSON schema.`
	default: // exec
		instruction = `You are a metadata extractor. Analyze the following execution output and extract:
- summary: a one-sentence summary of what was accomplished
- files_changed: list of file paths that were modified (empty list if unclear)
- tests_passed: boolean indicating whether tests passed (null if not mentioned)
Return ONLY a JSON object matching the provided JSON schema.`
	}
	return fmt.Sprintf("%s\n\n---\n\n%s", instruction, markdown)
}

// extractionTools returns the tool definitions for a given step type.
func extractionTools(stepType core.StepType) []ToolDef {
	switch stepType {
	case core.StepGate:
		return []ToolDef{{
			Name:        "extract_gate_metadata",
			Description: "Extract structured metadata from a gate review output.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"verdict": map[string]any{
						"type":        "string",
						"enum":        []string{"pass", "reject"},
						"description": "Whether the gate passed or was rejected.",
					},
					"reason": map[string]any{
						"type":        "string",
						"description": "Short, human-readable reason for the verdict.",
					},
					"reject_targets": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "integer"},
						"description": "Upstream step IDs to reset when verdict is reject.",
					},
				},
				"required": []string{"verdict", "reason"},
			},
		}}
	case core.StepComposite:
		return []ToolDef{{
			Name:        "extract_composite_metadata",
			Description: "Extract structured metadata from a composite step output.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"sub_tasks": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "List of sub-task names or descriptions.",
					},
				},
				"required": []string{"sub_tasks"},
			},
		}}
	default: // exec
		return []ToolDef{{
			Name:        "extract_exec_metadata",
			Description: "Extract structured metadata from an execution output.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"summary": map[string]any{
						"type":        "string",
						"description": "One-sentence summary of what was accomplished.",
					},
					"files_changed": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "File paths that were modified.",
					},
					"tests_passed": map[string]any{
						"type":        "boolean",
						"description": "Whether tests passed. Omit if not mentioned.",
					},
				},
				"required": []string{"summary"},
			},
		}}
	}
}

// OpenAICompleter calls OpenAI Responses API with Structured Outputs (JSON schema)
// and returns the raw JSON text output.
//
// Note: This completer does NOT use tool/function calling. The `tools` parameter is
// only used as a carrier for JSON schema definitions (see extractionTools).
type OpenAICompleter struct {
	client     openai.Client
	model      shared.ResponsesModel
	maxRetries int
	minBackoff time.Duration
	maxBackoff time.Duration
}

type OpenAICompleterConfig struct {
	BaseURL    string
	APIKey     string
	Model      string
	MaxRetries int
	MinBackoff time.Duration
	MaxBackoff time.Duration
}

func NewOpenAICompleter(cfg OpenAICompleterConfig) (*OpenAICompleter, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, fmt.Errorf("openai api_key is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, fmt.Errorf("openai model is required")
	}

	opts := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	if baseURL := strings.TrimSpace(cfg.BaseURL); baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}

	minBackoff := cfg.MinBackoff
	if minBackoff <= 0 {
		minBackoff = 200 * time.Millisecond
	}
	maxBackoff := cfg.MaxBackoff
	if maxBackoff <= 0 {
		maxBackoff = 2 * time.Second
	}

	return &OpenAICompleter{
		client:     openai.NewClient(opts...),
		model:      shared.ResponsesModel(strings.TrimSpace(cfg.Model)),
		maxRetries: max(0, cfg.MaxRetries),
		minBackoff: minBackoff,
		maxBackoff: maxBackoff,
	}, nil
}

func (c *OpenAICompleter) Complete(ctx context.Context, prompt string, tools []ToolDef) (json.RawMessage, error) {
	if c == nil {
		return nil, fmt.Errorf("OpenAICompleter is not initialized")
	}
	if strings.TrimSpace(prompt) == "" {
		return nil, fmt.Errorf("prompt is empty")
	}
	if len(tools) == 0 {
		return nil, fmt.Errorf("no json schema tool definitions provided")
	}

	tool := tools[0]
	name := strings.TrimSpace(tool.Name)
	if name == "" {
		name = "extract_metadata"
	}
	schema := tool.InputSchema
	if schema == nil {
		return nil, fmt.Errorf("tool %q schema is nil", name)
	}
	// Make strict mode friendlier by default: disallow extra top-level keys.
	if _, ok := schema["additionalProperties"]; !ok {
		schema = cloneMap(schema)
		schema["additionalProperties"] = false
	}

	maxAttempts := c.maxRetries + 1
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		resp, err := c.client.Responses.New(ctx, responses.ResponseNewParams{
			Model: shared.ResponsesModel(c.model),
			Input: responses.ResponseNewParamsInputUnion{
				OfString: openai.String(prompt),
			},
			Temperature: openai.Float(0),
			Text: responses.ResponseTextConfigParam{
				Format: responses.ResponseFormatTextConfigUnionParam{
					OfJSONSchema: &responses.ResponseFormatTextJSONSchemaConfigParam{
						Name:        name,
						Schema:      schema,
						Strict:      openai.Bool(true),
						Description: openai.String(strings.TrimSpace(tool.Description)),
					},
				},
			},
		})
		if err == nil {
			out := strings.TrimSpace(resp.OutputText())
			out = stripCodeFences(out)
			if out == "" {
				lastErr = fmt.Errorf("openai returned empty output")
			} else if !json.Valid([]byte(out)) {
				lastErr = fmt.Errorf("openai output is not valid json")
			} else {
				return json.RawMessage(out), nil
			}
		} else {
			lastErr = err
		}

		if attempt == maxAttempts || !isRetryableOpenAIError(lastErr) {
			break
		}
		sleepBackoff(ctx, backoffDelay(attempt, c.minBackoff, c.maxBackoff))
	}
	return nil, fmt.Errorf("openai complete failed after %d attempt(s): %w", maxAttempts, lastErr)
}

func isRetryableOpenAIError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false
	}
	var apierr *openai.Error
	if errors.As(err, &apierr) {
		switch apierr.StatusCode {
		case 408, 409, 425, 429, 500, 502, 503, 504:
			return true
		default:
			return false
		}
	}
	// Network / transient errors that are not wrapped as *openai.Error.
	return true
}

func backoffDelay(attempt int, minBackoff, maxBackoff time.Duration) time.Duration {
	// attempt starts at 1; after first failure we delay for ~minBackoff.
	d := minBackoff << (attempt - 1)
	if d > maxBackoff {
		return maxBackoff
	}
	return d
}

func sleepBackoff(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	// Very small defensive parser: strip ```lang ... ``` if present.
	lines := strings.Split(s, "\n")
	if len(lines) < 2 {
		return s
	}
	if strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
		lines = lines[1:]
	}
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "```" {
		lines = lines[:len(lines)-1]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func cloneMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
