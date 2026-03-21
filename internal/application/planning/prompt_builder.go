package planning

import (
	"fmt"
	"strings"

	"github.com/yoke233/zhanggui/internal/core"
	"github.com/yoke233/zhanggui/internal/skills"
)

const (
	defaultPlanningSkillName = "plan-actions"
	defaultPlanningFallback  = `# Planning Guidance

Use this workflow to convert a task description into an executable DAG:

1. Restate the objective in execution terms and infer the expected deliverable.
2. Create only the minimum set of actions needed to deliver the outcome.
3. Use short, unique, lowercase dash-separated action names.
4. Prefer a single focused implementation action over many tiny procedural actions.
5. Insert a gate action when review, approval, or quality validation is required.
6. Use depends_on only for real prerequisites and keep the result topologically ordered.
7. For every action, provide a concise description and at least one concrete acceptance criterion.
8. Choose agent_role and required_capabilities that match available profiles when they are provided.
9. If no profiles are available, default implementation to worker and review to gate.
10. Return only a DAG that is ready to materialize into system actions.`
)

// PromptBuilder assembles the prompt used by DAG generation from reusable planning guidance.
type PromptBuilder struct {
	skillsRoot string
	skillName  string
}

// PromptBuilderOption configures the planning prompt builder.
type PromptBuilderOption func(*PromptBuilder)

// WithPromptSkillsRoot configures the root directory used to load planning skills.
func WithPromptSkillsRoot(root string) PromptBuilderOption {
	return func(b *PromptBuilder) {
		b.skillsRoot = strings.TrimSpace(root)
	}
}

func withPlanningSkillName(name string) PromptBuilderOption {
	return func(b *PromptBuilder) {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			b.skillName = trimmed
		}
	}
}

// NewPromptBuilder creates a planning prompt builder.
func NewPromptBuilder(opts ...PromptBuilderOption) *PromptBuilder {
	builder := &PromptBuilder{
		skillName: defaultPlanningSkillName,
	}
	for _, opt := range opts {
		opt(builder)
	}
	return builder
}

// BuildDAGGenPrompt constructs the prompt for DAG generation using either a planning skill
// or the built-in fallback guidance when the skill is unavailable.
func (b *PromptBuilder) BuildDAGGenPrompt(input GenerateInput, profiles []*core.AgentProfile) string {
	var sb strings.Builder
	sb.WriteString(`You are a software engineering workflow planner. Given a task description, decompose it into a DAG (directed acyclic graph) of execution actions.`)
	sb.WriteString("\n\n")
	sb.WriteString(b.planningGuidance())

	if len(profiles) > 0 {
		sb.WriteString("\n\nAvailable agent profiles:\n")
		for _, p := range profiles {
			caps := "none"
			if len(p.Capabilities) > 0 {
				caps = strings.Join(p.Capabilities, ", ")
			}
			fmt.Fprintf(&sb, "- %q (role: %s, capabilities: [%s])\n", p.ID, p.Role, caps)
		}
		sb.WriteString(`
When assigning agent_role and required_capabilities to an action:
- Set agent_role to one of the roles listed above (e.g. "worker", "gate").
- Set required_capabilities using ONLY capability tags from the profiles above.
- Each action's role + capabilities must match at least one available profile.
`)
	} else {
		sb.WriteString(`
When assigning agent_role:
- Set agent_role to "worker" for implementation actions.
- Set agent_role to "gate" for review or approval actions.
`)
	}

	if input.Description != "" {
		fmt.Fprintf(&sb, `

Task description:
---
%s
---
`, input.Description)
	}

	if len(input.Files) > 0 {
		sb.WriteString("\nReference materials:\n---\n")
		for name, content := range input.Files {
			fmt.Fprintf(&sb, "### %s\n%s\n\n", name, content)
		}
		sb.WriteString("---\n")
	}

	sb.WriteString("\nReturn a JSON object with an \"actions\" array. Actions MUST be ordered so that dependencies always appear before dependents (topological order).")

	return sb.String()
}

func (b *PromptBuilder) planningGuidance() string {
	if skillText := b.loadPlanningSkillText(); skillText != "" {
		return skillText
	}
	return defaultPlanningFallback
}

func (b *PromptBuilder) loadPlanningSkillText() string {
	root := strings.TrimSpace(b.skillsRoot)
	name := strings.TrimSpace(b.skillName)
	if root == "" || name == "" {
		return ""
	}

	parsed, err := skills.InspectSkill(root, name)
	if err != nil || parsed == nil || !parsed.Valid {
		return ""
	}

	return stripSkillFrontmatter(parsed.SkillMD)
}

func stripSkillFrontmatter(content string) string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return strings.TrimSpace(normalized)
	}
	rest := normalized[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		end = strings.Index(rest, "\n---")
	}
	if end < 0 {
		return strings.TrimSpace(normalized)
	}
	return strings.TrimSpace(rest[end+5:])
}
