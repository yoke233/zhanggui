package executor

import (
	"context"
	"fmt"
	"time"

	flowapp "github.com/yoke233/ai-workflow/internal/application/flow"
	"github.com/yoke233/ai-workflow/internal/core"
)

// NewMockStepExecutor returns a StepExecutor that does not spawn ACP agents.
// It stores a small markdown artifact and publishes a single "done" agent_output event.
//
// Intended for local smoke tests and CI where external agent credentials are unavailable.
func NewMockStepExecutor(store core.Store, bus core.EventBus) flowapp.StepExecutor {
	return func(ctx context.Context, step *core.Step, exec *core.Execution) error {
		workDir := ""
		if ws := flowapp.WorkspaceFromContext(ctx); ws != nil {
			workDir = ws.Path
		}

		now := time.Now().UTC()
		reply := fmt.Sprintf(
			"## Mock executor\n\n- step_id: %d\n- flow_id: %d\n- step_type: %s\n- agent_role: %s\n- work_dir: %s\n- time_utc: %s\n",
			step.ID, step.FlowID, step.Type, step.AgentRole, workDir, now.Format(time.RFC3339),
		)

		// Publish done event with full reply (matches ACP bridge "done" shape).
		if bus != nil {
			bus.Publish(ctx, core.Event{
				Type:      core.EventExecAgentOutput,
				FlowID:    step.FlowID,
				StepID:    step.ID,
				ExecID:    exec.ID,
				Timestamp: now,
				Data: map[string]any{
					"type":    "done",
					"content": reply,
				},
			})
		}

		// Store artifact.
		if store != nil {
			art := &core.Artifact{
				ExecutionID:    exec.ID,
				StepID:         step.ID,
				FlowID:         step.FlowID,
				ResultMarkdown: reply,
			}
			artID, err := store.CreateArtifact(ctx, art)
			if err != nil {
				return fmt.Errorf("store artifact: %w", err)
			}
			exec.ArtifactID = &artID
		}

		exec.Output = map[string]any{
			"text":        reply,
			"stop_reason": "mock",
		}

		return nil
	}
}
