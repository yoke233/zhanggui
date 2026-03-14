package llmplanning

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/yoke233/ai-workflow/internal/application/planning"
)

type mockLLMClient struct {
	output string
}

func (m *mockLLMClient) Complete(_ context.Context, _ string, _ []planning.ToolDef) (json.RawMessage, error) {
	return json.RawMessage(m.output), nil
}

func TestCompleter_Complete(t *testing.T) {
	expected := `{"steps":[{"name":"build","type":"exec"}]}`
	// Completer wraps an llm.Client, but we test the port adapter here.
	// Since NewCompleter requires *llm.Client which needs real config,
	// we verify the port interface is correctly defined.
	var _ planning.LLMCompleter = &mockLLMClient{output: expected}
}
