package planning

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/yoke233/zhanggui/internal/core"
)

// Service orchestrates DAG generation: prompt building, LLM invocation,
// validation, and materialization into the store.
type Service struct {
	llm           LLMCompleter
	registry      core.AgentRegistry
	promptBuilder *PromptBuilder
}

// Option configures the planning service.
type Option func(*Service)

// WithPlanningSkillsRoot configures the skills root used by the planning prompt builder.
func WithPlanningSkillsRoot(root string) Option {
	return func(s *Service) {
		builder := s.promptBuilder
		if builder == nil {
			builder = NewPromptBuilder()
		}
		s.promptBuilder = NewPromptBuilder(
			WithPromptSkillsRoot(root),
			withPlanningSkillName(builder.skillName),
		)
	}
}

// NewService creates a planning Service.
func NewService(llm LLMCompleter, registry core.AgentRegistry, opts ...Option) *Service {
	svc := &Service{
		llm:           llm,
		registry:      registry,
		promptBuilder: NewPromptBuilder(),
	}
	for _, opt := range opts {
		opt(svc)
	}
	if svc.promptBuilder == nil {
		svc.promptBuilder = NewPromptBuilder()
	}
	return svc
}

// Generate calls the LLM to decompose a task description into a DAG of actions.
func (s *Service) Generate(ctx context.Context, input GenerateInput) (*GeneratedDAG, error) {
	if s.llm == nil {
		return nil, fmt.Errorf("dag_gen: llm completer is nil")
	}

	profiles, err := s.listProfiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("dag_gen: list profiles: %w", err)
	}

	prompt := s.promptBuilder.BuildDAGGenPrompt(input, profiles)
	tools := BuildDAGGenSchema(profiles)

	raw, err := s.llm.Complete(ctx, prompt, tools)
	if err != nil {
		return nil, fmt.Errorf("dag_gen: llm call failed: %w", err)
	}

	var dag GeneratedDAG
	if err := json.Unmarshal(raw, &dag); err != nil {
		return nil, fmt.Errorf("dag_gen: parse llm output: %w", err)
	}

	if len(dag.Actions) == 0 {
		return nil, fmt.Errorf("dag_gen: llm returned zero actions")
	}

	if err := ValidateGeneratedDAG(&dag); err != nil {
		return nil, fmt.Errorf("dag_gen: %w", err)
	}

	if len(profiles) > 0 {
		if err := ValidateCapabilityFit(&dag, profiles); err != nil {
			return nil, fmt.Errorf("dag_gen: %w", err)
		}
	}

	return &dag, nil
}

// Materialize creates Actions in the store for a given work item from a GeneratedDAG.
// It delegates to the package-level MaterializeDAG function.
func (s *Service) Materialize(ctx context.Context, store core.Store, workItemID int64, dag *GeneratedDAG) ([]*core.Action, error) {
	return MaterializeDAG(ctx, store, workItemID, dag)
}

// MaterializeDAG creates Actions in the store for a given work item from a GeneratedDAG.
// Phase 1: create all Actions (position-ordered) and build a name→ID map.
// Phase 2: resolve name-based DependsOn to action IDs and persist them.
func MaterializeDAG(ctx context.Context, store ActionMaterializer, workItemID int64, dag *GeneratedDAG) ([]*core.Action, error) {
	if dag == nil {
		return nil, fmt.Errorf("dag_gen: generated dag is nil")
	}
	if err := ValidateGeneratedDAG(dag); err != nil {
		return nil, fmt.Errorf("dag_gen: %w", err)
	}

	// Phase 1: create all actions, record name→ID.
	nameToID := make(map[string]int64, len(dag.Actions))
	var created []*core.Action

	for i, gs := range dag.Actions {
		actionType := core.ActionType(gs.Type)
		if actionType == "" {
			actionType = core.ActionExec
		}

		action := &core.Action{
			WorkItemID:           workItemID,
			Name:                 gs.Name,
			Description:          gs.Description,
			Type:                 actionType,
			Status:               core.ActionPending,
			Position:             i,
			AgentRole:            gs.AgentRole,
			RequiredCapabilities: gs.RequiredCapabilities,
			AcceptanceCriteria:   gs.AcceptanceCriteria,
		}

		id, err := store.CreateAction(ctx, action)
		if err != nil {
			return nil, fmt.Errorf("dag_gen: create action %q: %w", gs.Name, err)
		}
		action.ID = id
		nameToID[gs.Name] = id
		created = append(created, action)
	}

	// Phase 2: resolve DependsOn names → IDs and persist.
	for i, gs := range dag.Actions {
		if len(gs.DependsOn) == 0 {
			continue
		}
		resolved := make([]int64, 0, len(gs.DependsOn))
		for _, depName := range gs.DependsOn {
			depID, ok := nameToID[depName]
			if !ok {
				return nil, fmt.Errorf("dag_gen: action %q depends on unknown action %q", gs.Name, depName)
			}
			resolved = append(resolved, depID)
		}
		if err := store.UpdateActionDependsOn(ctx, created[i].ID, resolved); err != nil {
			return nil, fmt.Errorf("dag_gen: update depends_on for action %q: %w", gs.Name, err)
		}
		created[i].DependsOn = resolved
	}

	return created, nil
}

func (s *Service) listProfiles(ctx context.Context) ([]*core.AgentProfile, error) {
	if s.registry == nil {
		return nil, nil
	}
	return s.registry.ListProfiles(ctx)
}

// ValidateGeneratedDAG checks that the generated DAG has no duplicate names,
// all dependency references are valid, and action types are known.
func ValidateGeneratedDAG(dag *GeneratedDAG) error {
	names := make(map[string]bool, len(dag.Actions))
	for i, s := range dag.Actions {
		if s.Name == "" {
			return fmt.Errorf("action[%d] has empty name", i)
		}
		if names[s.Name] {
			return fmt.Errorf("duplicate action name %q", s.Name)
		}
		names[s.Name] = true
	}

	seen := make(map[string]bool, len(dag.Actions))
	for _, s := range dag.Actions {
		for _, dep := range s.DependsOn {
			if !seen[dep] {
				return fmt.Errorf("action %q depends on %q which is not defined before it", s.Name, dep)
			}
		}
		seen[s.Name] = true
	}

	validTypes := map[string]bool{"exec": true, "gate": true, "composite": true}
	for _, s := range dag.Actions {
		if !validTypes[s.Type] {
			return fmt.Errorf("action %q has invalid type %q", s.Name, s.Type)
		}
	}

	return nil
}

// ValidateCapabilityFit checks that every action's agent_role + required_capabilities
// can be satisfied by at least one available profile.
func ValidateCapabilityFit(dag *GeneratedDAG, profiles []*core.AgentProfile) error {
	for _, gs := range dag.Actions {
		role := core.AgentRole(gs.AgentRole)
		matched := false
		for _, p := range profiles {
			if role != "" && p.Role != role {
				continue
			}
			if p.MatchesRequirements(gs.RequiredCapabilities) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("action %q (role=%s, caps=%v) has no matching agent profile",
				gs.Name, gs.AgentRole, gs.RequiredCapabilities)
		}
	}
	return nil
}

// BuildDAGGenPrompt constructs the LLM prompt for DAG generation.
func BuildDAGGenPrompt(input GenerateInput, profiles []*core.AgentProfile) string {
	return NewPromptBuilder().BuildDAGGenPrompt(input, profiles)
}

// BuildDAGGenSchema returns the tool schema for structured LLM output.
func BuildDAGGenSchema(profiles []*core.AgentProfile) []ToolDef {
	roleSet := map[string]bool{}
	capSet := map[string]bool{}
	for _, p := range profiles {
		roleSet[string(p.Role)] = true
		for _, c := range p.Capabilities {
			capSet[c] = true
		}
	}

	roleEnum := []string{"worker", "gate", "lead", "support"}
	if len(roleSet) > 0 {
		roleEnum = roleEnum[:0]
		for r := range roleSet {
			roleEnum = append(roleEnum, r)
		}
	}

	capItemSchema := map[string]any{"type": "string"}
	if len(capSet) > 0 {
		caps := make([]string, 0, len(capSet))
		for c := range capSet {
			caps = append(caps, c)
		}
		capItemSchema["enum"] = caps
	}

	return []ToolDef{{
		Name:        "generate_dag",
		Description: "Generate a DAG of workflow actions from a task description.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"actions": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name": map[string]any{
								"type":        "string",
								"description": "Unique action name (lowercase, dash-separated).",
							},
							"type": map[string]any{
								"type":        "string",
								"enum":        []string{"exec", "gate", "composite"},
								"description": "Action type.",
							},
							"depends_on": map[string]any{
								"type":        "array",
								"items":       map[string]any{"type": "string"},
								"description": "Names of upstream actions this depends on.",
							},
							"agent_role": map[string]any{
								"type":        "string",
								"enum":        roleEnum,
								"description": "Agent role for this action.",
							},
							"required_capabilities": map[string]any{
								"type":        "array",
								"items":       capItemSchema,
								"description": "Capability tags the assigned agent must have.",
							},
							"acceptance_criteria": map[string]any{
								"type":        "array",
								"items":       map[string]any{"type": "string"},
								"description": "Conditions that must be met for the action to be done.",
							},
							"description": map[string]any{
								"type":        "string",
								"description": "What this action should accomplish.",
							},
						},
						"required":             []string{"name", "type"},
						"additionalProperties": false,
					},
					"description": "Ordered list of actions forming a DAG (dependencies appear before dependents).",
				},
			},
			"required": []string{"actions"},
		},
	}}
}
