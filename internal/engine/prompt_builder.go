package engine

import (
	"log/slog"

	"github.com/yoke233/ai-workflow/internal/core"
)

// PromptBuilder assembles a prompt with layered memory context.
type PromptBuilder struct {
	memory core.Memory
}

// NewPromptBuilder creates a PromptBuilder. A nil memory degrades gracefully.
func NewPromptBuilder(memory core.Memory) *PromptBuilder {
	return &PromptBuilder{memory: memory}
}

// Build injects layered memory into prompt variables before rendering.
func (b *PromptBuilder) Build(issueID, runID, stage string, vars PromptVars) (string, error) {
	if b != nil && b.memory != nil {
		if cold, err := b.memory.RecallCold(issueID); err != nil {
			slog.Warn("prompt builder: recall cold context failed", "issue_id", issueID, "error", err)
		} else {
			vars.ColdContext = cold
		}

		if warm, err := b.memory.RecallWarm(issueID); err != nil {
			slog.Warn("prompt builder: recall warm context failed", "issue_id", issueID, "error", err)
		} else {
			vars.WarmContext = warm
		}

		if hot, err := b.memory.RecallHot(issueID, runID); err != nil {
			slog.Warn("prompt builder: recall hot context failed", "issue_id", issueID, "run_id", runID, "error", err)
		} else {
			vars.HotContext = hot
		}
	}

	return RenderPrompt(stage, vars)
}
