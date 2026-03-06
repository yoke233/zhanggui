package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yoke233/ai-workflow/internal/core"
)

func registerRunTools(server *mcp.Server, executor RunExecutor) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "apply_run_action",
		Description: "Apply an action to a run: approve, reject, skip, rerun, abort, pause, resume, modify, change_role",
	}, applyRunActionHandler(executor))
}

type ApplyRunActionInput struct {
	RunID   string `json:"run_id" jsonschema:"Run ID (required)"`
	Action  string `json:"action" jsonschema:"Action type: approve, reject, skip, rerun, abort, pause, resume, modify, change_role (required)"`
	Stage   string `json:"stage,omitempty" jsonschema:"Target stage name"`
	Message string `json:"message,omitempty" jsonschema:"Action message or description update"`
	Role    string `json:"role,omitempty" jsonschema:"New role (for change_role action)"`
}

func applyRunActionHandler(executor RunExecutor) func(context.Context, *mcp.CallToolRequest, ApplyRunActionInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in ApplyRunActionInput) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(in.RunID) == "" {
			return errorResult("run_id is required")
		}
		action := strings.ToLower(strings.TrimSpace(in.Action))
		if action == "" {
			return errorResult("action is required")
		}

		runAction := core.RunAction{
			RunID:   strings.TrimSpace(in.RunID),
			Type:    core.HumanActionType(action),
			Stage:   core.StageID(strings.TrimSpace(in.Stage)),
			Message: strings.TrimSpace(in.Message),
			Role:    strings.TrimSpace(in.Role),
		}
		if err := runAction.Validate(); err != nil {
			return errorResult("invalid action: " + err.Error())
		}
		if err := executor.ApplyAction(ctx, runAction); err != nil {
			return errorResult(fmt.Sprintf("apply run action: %v", err))
		}
		return jsonResult(map[string]string{
			"run_id": runAction.RunID,
			"action": action,
			"status": "applied",
		})
	}
}
