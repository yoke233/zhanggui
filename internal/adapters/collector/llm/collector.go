package llm

import (
	"context"
	"encoding/json"
	"fmt"

	basellm "github.com/yoke233/ai-workflow/internal/adapters/llm"
	"github.com/yoke233/ai-workflow/internal/core"
)

type LLMCollector struct {
	Complete func(ctx context.Context, prompt string, tools []basellm.ToolDef) (json.RawMessage, error)
}

func NewLLMCollector(complete func(ctx context.Context, prompt string, tools []basellm.ToolDef) (json.RawMessage, error)) *LLMCollector {
	return &LLMCollector{Complete: complete}
}

func (c *LLMCollector) Extract(ctx context.Context, stepType core.ActionType, markdown string) (map[string]any, error) {
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

func buildExtractionPrompt(stepType core.ActionType, markdown string) string {
	var instruction string
	switch stepType {
	case core.ActionGate:
		instruction = `You are a metadata extractor. Analyze the following gate review output and extract:
- verdict: "pass" or "reject"
 - reason: a short, human-readable reason (empty string if unclear)
 - reject_targets: list of upstream step IDs to reset when verdict is "reject" (optional; omit if unclear)
Return ONLY a JSON object matching the provided JSON schema.`
	case core.ActionPlan:
		instruction = `You are a metadata extractor. Analyze the following composite step output and extract:
- sub_tasks: list of sub-task names/descriptions identified
Return ONLY a JSON object matching the provided JSON schema.`
	default:
		instruction = `You are a metadata extractor. Analyze the following execution output and extract:
- summary: a one-sentence summary of what was accomplished
- files_changed: list of file paths that were modified (empty list if unclear)
- tests_passed: boolean indicating whether tests passed (null if not mentioned)
Return ONLY a JSON object matching the provided JSON schema.`
	}
	return fmt.Sprintf("%s\n\n---\n\n%s", instruction, markdown)
}

func extractionTools(stepType core.ActionType) []basellm.ToolDef {
	switch stepType {
	case core.ActionGate:
		return []basellm.ToolDef{{
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
	case core.ActionPlan:
		return []basellm.ToolDef{{
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
	default:
		return []basellm.ToolDef{{
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

type OpenAICompleter = basellm.Client
type OpenAICompleterConfig = basellm.Config

func NewOpenAICompleter(cfg OpenAICompleterConfig) (*OpenAICompleter, error) {
	return basellm.New(cfg)
}

type ToolDef = basellm.ToolDef
