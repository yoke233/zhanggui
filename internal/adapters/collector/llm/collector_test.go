package llm

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	basellm "github.com/yoke233/ai-workflow/internal/adapters/llm"
	"github.com/yoke233/ai-workflow/internal/core"
)

func TestLLMCollector_ExecExtraction(t *testing.T) {
	// Simulate LLM returning tool_use JSON.
	collector := NewLLMCollector(func(ctx context.Context, prompt string, tools []basellm.ToolDef) (json.RawMessage, error) {
		if len(tools) != 1 || tools[0].Name != "extract_exec_metadata" {
			t.Fatalf("expected extract_exec_metadata tool, got %v", tools)
		}
		return json.RawMessage(`{"summary":"Added login endpoint","files_changed":["api/auth.go","api/auth_test.go"],"tests_passed":true}`), nil
	})

	result, err := collector.Extract(context.Background(), core.ActionExec, "## Changes\nAdded login endpoint in api/auth.go\nAll tests pass.")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if result["summary"] != "Added login endpoint" {
		t.Errorf("summary = %v, want 'Added login endpoint'", result["summary"])
	}
	files, ok := result["files_changed"].([]any)
	if !ok || len(files) != 2 {
		t.Errorf("files_changed = %v, want 2 files", result["files_changed"])
	}
}

func TestLLMCollector_GateExtraction(t *testing.T) {
	collector := NewLLMCollector(func(ctx context.Context, prompt string, tools []basellm.ToolDef) (json.RawMessage, error) {
		if tools[0].Name != "extract_gate_metadata" {
			t.Fatalf("expected extract_gate_metadata tool, got %s", tools[0].Name)
		}
		return json.RawMessage(`{"verdict":"reject","reason":"Missing error handling"}`), nil
	})

	result, err := collector.Extract(context.Background(), core.ActionGate, "Review: code lacks error handling and tests.")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if result["verdict"] != "reject" {
		t.Errorf("verdict = %v, want reject", result["verdict"])
	}
	if reason, ok := result["reason"].(string); !ok || reason == "" {
		t.Errorf("reason = %v, want non-empty string", result["reason"])
	}
}

func TestLLMCollector_CompositeExtraction(t *testing.T) {
	collector := NewLLMCollector(func(ctx context.Context, prompt string, tools []basellm.ToolDef) (json.RawMessage, error) {
		if tools[0].Name != "extract_composite_metadata" {
			t.Fatalf("expected extract_composite_metadata tool, got %s", tools[0].Name)
		}
		return json.RawMessage(`{"sub_tasks":["implement API","write tests","update docs"]}`), nil
	})

	result, err := collector.Extract(context.Background(), core.ActionPlan, "Decomposed into: implement API, write tests, update docs.")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	tasks, ok := result["sub_tasks"].([]any)
	if !ok || len(tasks) != 3 {
		t.Errorf("sub_tasks = %v, want 3 items", result["sub_tasks"])
	}
}

func TestLLMCollector_NilComplete(t *testing.T) {
	collector := &LLMCollector{}
	_, err := collector.Extract(context.Background(), core.ActionExec, "some markdown")
	if err == nil {
		t.Fatal("expected error when Complete is nil")
	}
}

func TestBuildExtractionPrompt(t *testing.T) {
	prompt := buildExtractionPrompt(core.ActionGate, "review content")
	if len(prompt) == 0 {
		t.Fatal("prompt should not be empty")
	}
	// Should contain the markdown content.
	if !strings.Contains(prompt, "review content") {
		t.Error("prompt should contain the markdown input")
	}
	// Should mention gate-specific instructions.
	if !strings.Contains(prompt, "verdict") {
		t.Error("gate prompt should mention verdict")
	}
}

func TestExtractionTools(t *testing.T) {
	tests := []struct {
		stepType core.ActionType
		toolName string
	}{
		{core.ActionExec, "extract_exec_metadata"},
		{core.ActionGate, "extract_gate_metadata"},
		{core.ActionPlan, "extract_composite_metadata"},
	}
	for _, tt := range tests {
		tools := extractionTools(tt.stepType)
		if len(tools) != 1 {
			t.Errorf("%s: expected 1 tool, got %d", tt.stepType, len(tools))
			continue
		}
		if tools[0].Name != tt.toolName {
			t.Errorf("%s: tool name = %s, want %s", tt.stepType, tools[0].Name, tt.toolName)
		}
	}
}
