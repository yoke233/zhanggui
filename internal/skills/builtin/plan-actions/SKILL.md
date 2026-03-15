---
name: plan-actions
description: A reusable planning playbook for turning a task description into an executable DAG of ai-workflow steps. Use this guidance when the goal is to decompose work, define dependencies, insert review gates, and map steps to agent roles and capabilities.
---

# Plan Actions

Use this skill when you need to convert an ambiguous request into an executable workflow.

## Planning Objectives

1. Restate the user's goal in execution terms.
2. Infer the expected deliverable or artifact.
3. Produce the minimum viable DAG needed to achieve the outcome.
4. Keep the result executable by existing agent roles and capabilities.

## Step Design Rules

1. Each step must have a short, unique, lowercase dash-separated name.
2. Prefer fewer, outcome-oriented steps over many tiny procedural steps.
3. Use `depends_on` only for real prerequisites.
4. Keep steps in topological order so every dependency appears before the dependent step.
5. Insert a `gate` step whenever review, approval, or quality validation is required.
6. Use `composite` only when the work should expand into a subordinate workflow rather than execute directly.

## Role And Capability Mapping

1. Choose `agent_role` based on the nature of the work, not on a preferred implementation sequence.
2. Only require capabilities that are genuinely needed for the step to complete.
3. If agent profiles are provided, stay within those known roles and capability tags.
4. If no profiles are provided, default implementation work to `worker` and review work to `gate`.

## Quality Rules

1. Every step should include a clear description of what must be accomplished.
2. Every step should include at least one concrete acceptance criterion.
3. The final DAG should be immediately materializable into system actions without extra interpretation.
4. Avoid speculative or optional steps unless they are necessary to satisfy the stated objective.

## Output Standard

Return only a structured DAG plan that is ready to be validated and materialized by the system.
