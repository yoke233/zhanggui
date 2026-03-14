package planning

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yoke233/ai-workflow/internal/core"
)

// Service orchestrates DAG generation: prompt building, LLM invocation,
// validation, and materialization into the store.
type Service struct {
	llm      LLMCompleter
	registry core.AgentRegistry
}

// NewService creates a planning Service.
func NewService(llm LLMCompleter, registry core.AgentRegistry) *Service {
	return &Service{llm: llm, registry: registry}
}

// Generate calls the LLM to decompose a task description into a DAG of Steps.
func (s *Service) Generate(ctx context.Context, taskDescription string) (*GeneratedDAG, error) {
	if s.llm == nil {
		return nil, fmt.Errorf("dag_gen: llm completer is nil")
	}

	profiles, err := s.listProfiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("dag_gen: list profiles: %w", err)
	}

	prompt := BuildDAGGenPrompt(taskDescription, profiles)
	tools := BuildDAGGenSchema(profiles)

	raw, err := s.llm.Complete(ctx, prompt, tools)
	if err != nil {
		return nil, fmt.Errorf("dag_gen: llm call failed: %w", err)
	}

	var dag GeneratedDAG
	if err := json.Unmarshal(raw, &dag); err != nil {
		return nil, fmt.Errorf("dag_gen: parse llm output: %w", err)
	}

	if len(dag.Steps) == 0 {
		return nil, fmt.Errorf("dag_gen: llm returned zero steps")
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
func (s *Service) Materialize(ctx context.Context, store core.Store, issueID int64, dag *GeneratedDAG) ([]*core.Action, error) {
	if dag == nil {
		return nil, fmt.Errorf("dag_gen: generated dag is nil")
	}
	if err := ValidateGeneratedDAG(dag); err != nil {
		return nil, fmt.Errorf("dag_gen: %w", err)
	}

	var created []*core.Action

	for i, gs := range dag.Steps {
		stepType := core.ActionType(gs.Type)
		if stepType == "" {
			stepType = core.ActionExec
		}

		step := &core.Action{
			WorkItemID:           issueID,
			Name:                 gs.Name,
			Description:          gs.Description,
			Type:                 stepType,
			Status:               core.ActionPending,
			Position:             i,
			AgentRole:            gs.AgentRole,
			RequiredCapabilities: gs.RequiredCapabilities,
			AcceptanceCriteria:   gs.AcceptanceCriteria,
		}

		id, err := store.CreateAction(ctx, step)
		if err != nil {
			return nil, fmt.Errorf("dag_gen: create step %q: %w", gs.Name, err)
		}
		step.ID = id
		created = append(created, step)
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
// all dependency references are valid, and step types are known.
func ValidateGeneratedDAG(dag *GeneratedDAG) error {
	names := make(map[string]bool, len(dag.Steps))
	for i, s := range dag.Steps {
		if s.Name == "" {
			return fmt.Errorf("step[%d] has empty name", i)
		}
		if names[s.Name] {
			return fmt.Errorf("duplicate step name %q", s.Name)
		}
		names[s.Name] = true
	}

	seen := make(map[string]bool, len(dag.Steps))
	for _, s := range dag.Steps {
		for _, dep := range s.DependsOn {
			if !seen[dep] {
				return fmt.Errorf("step %q depends on %q which is not defined before it", s.Name, dep)
			}
		}
		seen[s.Name] = true
	}

	validTypes := map[string]bool{"exec": true, "gate": true, "composite": true}
	for _, s := range dag.Steps {
		if !validTypes[s.Type] {
			return fmt.Errorf("step %q has invalid type %q", s.Name, s.Type)
		}
	}

	return nil
}

// ValidateCapabilityFit checks that every step's agent_role + required_capabilities
// can be satisfied by at least one available profile.
func ValidateCapabilityFit(dag *GeneratedDAG, profiles []*core.AgentProfile) error {
	for _, gs := range dag.Steps {
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
			return fmt.Errorf("step %q (role=%s, caps=%v) has no matching agent profile",
				gs.Name, gs.AgentRole, gs.RequiredCapabilities)
		}
	}
	return nil
}

// BuildDAGGenPrompt constructs the LLM prompt for DAG generation.
func BuildDAGGenPrompt(taskDescription string, profiles []*core.AgentProfile) string {
	var sb strings.Builder
	sb.WriteString(`You are a software engineering workflow planner. Given a task description, decompose it into a DAG (directed acyclic graph) of execution steps.

Rules:
1. Each step has a unique name (short, lowercase, dash-separated, e.g. "parse-requirements", "implement-api").
2. Step types: "exec" (run code/task), "gate" (review/approval check), "composite" (delegates to sub-workflow).
3. Use "depends_on" to express ordering — list the names of upstream steps.
4. Entry steps (no dependencies) should have an empty depends_on.
5. Include a "gate" step after implementation steps if quality review is needed.
6. Keep it minimal — only create steps that are clearly needed. Prefer fewer, focused steps over many tiny ones.
7. Provide clear acceptance_criteria for each step — what must be true for the step to be considered done.
8. Provide a description for each step — what should be accomplished.
`)

	if len(profiles) > 0 {
		sb.WriteString("\nAvailable agent profiles:\n")
		for _, p := range profiles {
			caps := "none"
			if len(p.Capabilities) > 0 {
				caps = strings.Join(p.Capabilities, ", ")
			}
			sb.WriteString(fmt.Sprintf("- %q (role: %s, capabilities: [%s])\n", p.ID, p.Role, caps))
		}
		sb.WriteString(`
When assigning agent_role and required_capabilities to a step:
- Set agent_role to one of the roles listed above (e.g. "worker", "gate").
- Set required_capabilities using ONLY capability tags from the profiles above.
- Each step's role + capabilities must match at least one available profile.
`)
	} else {
		sb.WriteString(`9. Set agent_role: "worker" for implementation, "gate" for review steps.
`)
	}

	sb.WriteString(fmt.Sprintf(`
Task description:
---
%s
---

Return a JSON object with a "steps" array. Steps MUST be ordered so that dependencies always appear before dependents (topological order).`, taskDescription))

	return sb.String()
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
		Description: "Generate a DAG of workflow steps from a task description.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"steps": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name": map[string]any{
								"type":        "string",
								"description": "Unique step name (lowercase, dash-separated).",
							},
							"type": map[string]any{
								"type":        "string",
								"enum":        []string{"exec", "gate", "composite"},
								"description": "Step type.",
							},
							"depends_on": map[string]any{
								"type":        "array",
								"items":       map[string]any{"type": "string"},
								"description": "Names of upstream steps this depends on.",
							},
							"agent_role": map[string]any{
								"type":        "string",
								"enum":        roleEnum,
								"description": "Agent role for this step.",
							},
							"required_capabilities": map[string]any{
								"type":        "array",
								"items":       capItemSchema,
								"description": "Capability tags the assigned agent must have.",
							},
							"acceptance_criteria": map[string]any{
								"type":        "array",
								"items":       map[string]any{"type": "string"},
								"description": "Conditions that must be met for the step to be done.",
							},
							"description": map[string]any{
								"type":        "string",
								"description": "What this step should accomplish.",
							},
						},
						"required":             []string{"name", "type"},
						"additionalProperties": false,
					},
					"description": "Ordered list of steps forming a DAG (dependencies appear before dependents).",
				},
			},
			"required": []string{"steps"},
		},
	}}
}
