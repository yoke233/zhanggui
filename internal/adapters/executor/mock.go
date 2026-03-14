package executor

import (
	"context"
	"fmt"
	"time"

	flowapp "github.com/yoke233/ai-workflow/internal/application/flow"
	"github.com/yoke233/ai-workflow/internal/core"
)

// NewMockActionExecutor returns a ActionExecutor that does not spawn ACP agents.
// It stores a small markdown artifact and publishes a single "done" agent_output event.
//
// Intended for local smoke tests and CI where external agent credentials are unavailable.
func NewMockActionExecutor(store core.Store, bus core.EventBus) flowapp.ActionExecutor {
	return func(ctx context.Context, step *core.Action, exec *core.Run) error {
		workDir := ""
		if ws := flowapp.WorkspaceFromContext(ctx); ws != nil {
			workDir = ws.Path
		}

		now := time.Now().UTC()
		reply := fmt.Sprintf(
			"## Mock executor\n\n- step_id: %d\n- issue_id: %d\n- step_type: %s\n- agent_role: %s\n- work_dir: %s\n- time_utc: %s\n",
			step.ID, step.WorkItemID, step.Type, step.AgentRole, workDir, now.Format(time.RFC3339),
		)

		// Publish done event with full reply (matches ACP bridge "done" shape).
		if bus != nil {
			bus.Publish(ctx, core.Event{
				Type:       core.EventRunAgentOutput,
				WorkItemID: step.WorkItemID,
				ActionID:   step.ID,
				RunID:      exec.ID,
				Timestamp: now,
				Data: map[string]any{
					"type":    "done",
					"content": reply,
				},
			})
		}

		// Store result inline on the Run.
		exec.ResultMarkdown = reply

		exec.Output = map[string]any{
			"text":        reply,
			"stop_reason": "mock",
		}

		return nil
	}
}
