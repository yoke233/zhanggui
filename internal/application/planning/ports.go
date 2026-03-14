package planning

import (
	"context"
	"encoding/json"
)

// ToolDef describes a JSON schema tool for structured output extraction.
// Mirrors the adapter-level ToolDef so the application layer can build schemas
// without importing the LLM adapter.
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

// LLMCompleter is the port through which the planning service calls an LLM.
// Implementations live in the adapter layer (e.g. adapters/planning/llm).
type LLMCompleter interface {
	Complete(ctx context.Context, prompt string, tools []ToolDef) (json.RawMessage, error)
}
