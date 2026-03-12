package flow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

// Resolver selects an agent for a step based on role and required capabilities.
type Resolver interface {
	Resolve(ctx context.Context, step *core.Step) (agentID string, err error)
}

// BriefingBuilder assembles a Briefing for a step, injecting upstream context.
type BriefingBuilder interface {
	Build(ctx context.Context, step *core.Step) (*core.Briefing, error)
}

// Collector extracts structured metadata from agent markdown output (via small model + tool_use).
type Collector interface {
	Extract(ctx context.Context, stepType core.StepType, markdown string) (map[string]any, error)
}

// CompositeExpander decomposes a composite step into child steps for its child issue.
type CompositeExpander interface {
	Expand(ctx context.Context, step *core.Step) ([]*core.Step, error)
}

// ExpanderFunc adapts a plain function into a CompositeExpander.
type ExpanderFunc func(ctx context.Context, step *core.Step) ([]*core.Step, error)

func (f ExpanderFunc) Expand(ctx context.Context, step *core.Step) ([]*core.Step, error) {
	return f(ctx, step)
}

// CollectorFunc adapts a plain function into a Collector.
type CollectorFunc func(ctx context.Context, stepType core.StepType, markdown string) (map[string]any, error)

func (f CollectorFunc) Extract(ctx context.Context, stepType core.StepType, markdown string) (map[string]any, error) {
	return f(ctx, stepType, markdown)
}

// prepare resolves agent, builds briefing, and returns values for the Execution record.
func (e *IssueEngine) prepare(ctx context.Context, step *core.Step) (agentID, briefingSnapshot string, err error) {
	if e.resolver != nil {
		agentID, err = e.resolver.Resolve(ctx, step)
		if err != nil {
			return "", "", fmt.Errorf("resolve agent for step %d: %w", step.ID, err)
		}
	}

	if e.briefer != nil {
		briefing, err := e.briefer.Build(ctx, step)
		if err != nil {
			return "", "", fmt.Errorf("build briefing for step %d: %w", step.ID, err)
		}
		if _, err := e.store.CreateBriefing(ctx, briefing); err != nil {
			return "", "", fmt.Errorf("store briefing for step %d: %w", step.ID, err)
		}
		briefingSnapshot = renderBriefingSnapshot(briefing)
	}

	return agentID, briefingSnapshot, nil
}

const (
	maxBriefingRefChars   = 4000
	maxBriefingTotalChars = 12000
)

func renderBriefingSnapshot(briefing *core.Briefing) string {
	if briefing == nil {
		return ""
	}

	objective := truncateBriefingText(strings.TrimSpace(briefing.Objective), maxBriefingTotalChars)
	var sb strings.Builder
	sb.WriteString(objective)

	if len(briefing.ContextRefs) == 0 {
		return strings.TrimSpace(sb.String())
	}

	remaining := maxBriefingTotalChars - len(objective)
	if remaining <= 0 {
		return strings.TrimSpace(sb.String())
	}

	const contextHeader = "\n\n# Context\n"
	wroteContextHeader := false
	for _, ref := range briefing.ContextRefs {
		content := strings.TrimSpace(ref.Inline)
		if content == "" || remaining <= 0 {
			continue
		}
		content = truncateBriefingText(content, maxBriefingRefChars)
		if len(content) > remaining {
			content = truncateBriefingText(content, remaining)
		}
		if strings.TrimSpace(content) == "" {
			continue
		}

		label := strings.TrimSpace(ref.Label)
		if label == "" {
			label = fmt.Sprintf("%s:%d", ref.Type, ref.RefID)
		}

		sectionHeader := "\n\n## " + label + "\n\n"
		available := remaining - len(sectionHeader)
		if !wroteContextHeader {
			available -= len(contextHeader)
		}
		if available <= 0 {
			break
		}
		content = truncateBriefingText(content, minInt(maxBriefingRefChars, available))
		if strings.TrimSpace(content) == "" {
			continue
		}

		if !wroteContextHeader {
			sb.WriteString(contextHeader)
			remaining -= len(contextHeader)
			wroteContextHeader = true
		}
		sb.WriteString(sectionHeader)
		sb.WriteString(content)
		remaining -= len(sectionHeader) + len(content)
	}

	return strings.TrimSpace(sb.String())
}

func truncateBriefingText(text string, maxChars int) string {
	text = strings.TrimSpace(text)
	if maxChars <= 0 || len(text) <= maxChars {
		return text
	}
	const suffix = "\n\n[truncated]"
	if maxChars <= len(suffix) {
		return text[:maxChars]
	}
	return strings.TrimSpace(text[:maxChars-len(suffix)]) + suffix
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// finalize handles the execution result: failure path or success path.
func (e *IssueEngine) finalize(ctx context.Context, step *core.Step, exec *core.Execution, execErr error) error {
	finished := time.Now().UTC()
	exec.FinishedAt = &finished

	if execErr != nil {
		return e.handleFailure(ctx, step, exec, execErr)
	}
	return e.handleSuccess(ctx, step, exec)
}

// handleFailure classifies the error and decides: retry, block, or fail.
func (e *IssueEngine) handleFailure(ctx context.Context, step *core.Step, exec *core.Execution, execErr error) error {
	exec.Status = core.ExecFailed
	exec.ErrorMessage = execErr.Error()

	// Auto-classify timeout as transient.
	if exec.ErrorKind == "" && errors.Is(execErr, context.DeadlineExceeded) {
		exec.ErrorKind = core.ErrKindTransient
	}

	_ = e.store.UpdateExecution(ctx, exec)

	e.bus.Publish(ctx, core.Event{
		Type:      core.EventExecFailed,
		IssueID:   step.IssueID,
		StepID:    step.ID,
		ExecID:    exec.ID,
		Timestamp: time.Now().UTC(),
		Data:      map[string]any{"error": execErr.Error(), "error_kind": string(exec.ErrorKind)},
	})

	// Permanent → fail immediately, skip retry.
	if exec.ErrorKind == core.ErrKindPermanent {
		_ = e.transitionStep(ctx, step, core.StepFailed)
		return fmt.Errorf("step %d failed (permanent): %w", step.ID, execErr)
	}

	// Need help → block step for external intervention.
	if exec.ErrorKind == core.ErrKindNeedHelp {
		_ = e.transitionStep(ctx, step, core.StepBlocked)
		return nil // Not an engine error; other steps can continue.
	}

	// Transient or unclassified → retry if budget remains.
	if step.RetryCount < step.MaxRetries {
		step.RetryCount++
		step.Status = core.StepPending
		if err := e.store.UpdateStep(ctx, step); err != nil {
			return fmt.Errorf("retry step %d: %w", step.ID, err)
		}
		return nil
	}

	_ = e.transitionStep(ctx, step, core.StepFailed)
	return fmt.Errorf("step %d failed: %w", step.ID, execErr)
}

// handleSuccess processes a successful execution: collect metadata, then gate finalize or step done.
func (e *IssueEngine) handleSuccess(ctx context.Context, step *core.Step, exec *core.Execution) error {
	exec.Status = core.ExecSucceeded
	_ = e.store.UpdateExecution(ctx, exec)

	e.bus.Publish(ctx, core.Event{
		Type:      core.EventExecSucceeded,
		IssueID:   step.IssueID,
		StepID:    step.ID,
		ExecID:    exec.ID,
		Timestamp: time.Now().UTC(),
	})

	// Collect: extract structured metadata from agent output.
	if err := e.collectMetadata(ctx, step); err != nil {
		// Collection failure is non-fatal — log via event but don't fail the step.
		e.bus.Publish(ctx, core.Event{
			Type:      core.EventExecFailed,
			IssueID:   step.IssueID,
			StepID:    step.ID,
			Timestamp: time.Now().UTC(),
			Data:      map[string]any{"collect_error": err.Error()},
		})
	}

	switch step.Type {
	case core.StepGate:
		return e.finalizeGate(ctx, step)
	default:
		return e.transitionStep(ctx, step, core.StepDone)
	}
}

// collectMetadata runs the Collector (if set) to extract structured metadata from the step's latest Artifact.
func (e *IssueEngine) collectMetadata(ctx context.Context, step *core.Step) error {
	if e.collector == nil {
		return nil
	}
	art, err := e.store.GetLatestArtifactByStep(ctx, step.ID)
	if err != nil {
		return nil // no artifact to collect from
	}
	if art.ResultMarkdown == "" {
		return nil
	}

	metadata, err := e.collector.Extract(ctx, step.Type, art.ResultMarkdown)
	if err != nil {
		return fmt.Errorf("collect metadata for step %d: %w", step.ID, err)
	}

	// Merge extracted metadata into existing metadata (don't overwrite).
	if art.Metadata == nil {
		art.Metadata = metadata
	} else {
		for k, v := range metadata {
			if _, exists := art.Metadata[k]; !exists {
				art.Metadata[k] = v
			}
		}
	}

	return e.store.UpdateArtifact(ctx, art)
}
