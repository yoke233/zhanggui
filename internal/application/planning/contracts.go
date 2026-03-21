package planning

import (
	"context"

	"github.com/yoke233/zhanggui/internal/core"
)

// ActionMaterializer is the minimal store contract for materializing a DAG into Actions.
type ActionMaterializer interface {
	CreateAction(ctx context.Context, a *core.Action) (int64, error)
	UpdateActionDependsOn(ctx context.Context, id int64, dependsOn []int64) error
}

// GenerateInput is the input for DAG generation.
type GenerateInput struct {
	Description string            // task description (required for entry B, optional for entry A)
	Files       map[string]string // filename → content (optional)
}

// GeneratedAction is the planner output for a single action in a generated DAG.
type GeneratedAction struct {
	Name                 string   `json:"name"`
	Type                 string   `json:"type"`
	DependsOn            []string `json:"depends_on,omitempty"`
	AgentRole            string   `json:"agent_role,omitempty"`
	RequiredCapabilities []string `json:"required_capabilities,omitempty"`
	AcceptanceCriteria   []string `json:"acceptance_criteria,omitempty"`
	Description          string   `json:"description,omitempty"`
}

// GeneratedDAG is the planner output for the full DAG.
type GeneratedDAG struct {
	Actions []GeneratedAction `json:"actions"`
}
