// Package llmplanning provides an LLM-backed adapter for the planning port.
// It translates the application-layer LLMCompleter interface into calls
// to the shared llm.Client.
package llmplanning

import (
	"context"
	"encoding/json"

	"github.com/yoke233/ai-workflow/internal/adapters/llm"
	planningapp "github.com/yoke233/ai-workflow/internal/application/planning"
)

// Completer adapts an llm.Client to the planning.LLMCompleter port.
type Completer struct {
	client *llm.Client
}

// NewCompleter creates an LLM completer adapter.
func NewCompleter(client *llm.Client) *Completer {
	return &Completer{client: client}
}

// Complete calls the LLM with the given prompt and tool schema,
// converting between the application-layer ToolDef and the adapter-layer ToolDef.
func (c *Completer) Complete(ctx context.Context, prompt string, tools []planningapp.ToolDef) (json.RawMessage, error) {
	adapterTools := make([]llm.ToolDef, len(tools))
	for i, t := range tools {
		adapterTools[i] = llm.ToolDef{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		}
	}
	return c.client.Complete(ctx, prompt, adapterTools)
}
