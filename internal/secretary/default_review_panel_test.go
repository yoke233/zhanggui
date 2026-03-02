package secretary

import (
	"context"
	"testing"

	"github.com/user/ai-workflow/internal/acpclient"
	"github.com/user/ai-workflow/internal/core"
)

func TestNewDefaultReviewOrchestratorApproveFlow(t *testing.T) {
	store := newMockReviewStore()
	panel := NewDefaultReviewOrchestrator(store)

	plan := &core.TaskPlan{
		ID:         "plan-default-review-approve",
		ProjectID:  "proj-1",
		Name:       "demo",
		Status:     core.PlanDraft,
		WaitReason: core.WaitNone,
		FailPolicy: core.FailBlock,
		Tasks: []core.TaskItem{
			{
				ID:          "task-1",
				PlanID:      "plan-default-review-approve",
				Title:       "task one",
				Description: "task one description",
				Template:    "standard",
			},
		},
	}

	result, err := panel.Run(context.Background(), plan, ReviewInput{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Decision != DecisionApprove {
		t.Fatalf("decision = %q, want %q", result.Decision, DecisionApprove)
	}
	if result.Plan.Status != core.PlanWaitingHuman {
		t.Fatalf("status = %q, want %q", result.Plan.Status, core.PlanWaitingHuman)
	}
	if result.Plan.WaitReason != core.WaitFinalApproval {
		t.Fatalf("wait_reason = %q, want %q", result.Plan.WaitReason, core.WaitFinalApproval)
	}
}

func TestNewDefaultReviewOrchestratorEscalateFlow(t *testing.T) {
	store := newMockReviewStore()
	panel := NewDefaultReviewOrchestrator(store)

	plan := &core.TaskPlan{
		ID:         "plan-default-review-escalate",
		ProjectID:  "proj-1",
		Name:       "demo",
		Status:     core.PlanDraft,
		WaitReason: core.WaitNone,
		FailPolicy: core.FailBlock,
		Tasks: []core.TaskItem{
			{
				ID:          "task-1",
				PlanID:      "plan-default-review-escalate",
				Title:       "task one",
				Description: "task one description",
				Template:    "custom-template",
			},
		},
	}

	result, err := panel.Run(context.Background(), plan, ReviewInput{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Decision != DecisionEscalate {
		t.Fatalf("decision = %q, want %q", result.Decision, DecisionEscalate)
	}
	if result.Plan.Status != core.PlanWaitingHuman {
		t.Fatalf("status = %q, want %q", result.Plan.Status, core.PlanWaitingHuman)
	}
	if result.Plan.WaitReason != core.WaitFeedbackReq {
		t.Fatalf("wait_reason = %q, want %q", result.Plan.WaitReason, core.WaitFeedbackReq)
	}
}

func TestNewDefaultReviewOrchestratorFromBindingsUsesResolvedRuntime(t *testing.T) {
	t.Parallel()

	store := newMockReviewStore()
	resolver := acpclient.NewRoleResolver(
		[]acpclient.AgentProfile{
			{
				ID: "codex",
				CapabilitiesMax: acpclient.ClientCapabilities{
					FSRead:   true,
					FSWrite:  true,
					Terminal: true,
				},
			},
		},
		[]acpclient.RoleProfile{
			{
				ID:      "senior-reviewer",
				AgentID: "codex",
				Capabilities: acpclient.ClientCapabilities{
					FSRead:   true,
					FSWrite:  true,
					Terminal: true,
				},
				SessionPolicy: acpclient.SessionPolicy{
					Reuse:       false,
					ResetPrompt: false,
				},
			},
			{
				ID:      "chief-aggregator",
				AgentID: "codex",
				Capabilities: acpclient.ClientCapabilities{
					FSRead:   true,
					FSWrite:  true,
					Terminal: true,
				},
				SessionPolicy: acpclient.SessionPolicy{
					Reuse:       false,
					ResetPrompt: false,
				},
			},
		},
	)

	panel, err := NewDefaultReviewOrchestratorFromBindings(store, ReviewRoleBindingInput{
		Reviewers: map[string]string{
			"completeness": "senior-reviewer",
			"dependency":   "senior-reviewer",
			"feasibility":  "senior-reviewer",
		},
		Aggregator: "chief-aggregator",
	}, resolver)
	if err != nil {
		t.Fatalf("NewDefaultReviewOrchestratorFromBindings() error = %v", err)
	}
	if panel.RoleRuntime == nil {
		t.Fatal("expected role runtime on review panel")
	}

	for _, reviewer := range []string{"completeness", "dependency", "feasibility"} {
		if got := panel.RoleRuntime.ReviewerRoles[reviewer]; got != "senior-reviewer" {
			t.Fatalf("role runtime reviewer role %s = %q, want %q", reviewer, got, "senior-reviewer")
		}
		policy := panel.RoleRuntime.ReviewerSessionPolicies[reviewer]
		if !policy.Reuse {
			t.Fatalf("role runtime reviewer %s should default reuse=true", reviewer)
		}
		if !policy.ResetPrompt {
			t.Fatalf("role runtime reviewer %s should default reset_prompt=true", reviewer)
		}
	}

	if got := panel.RoleRuntime.AggregatorRole; got != "chief-aggregator" {
		t.Fatalf("role runtime aggregator role = %q, want %q", got, "chief-aggregator")
	}
	if !panel.RoleRuntime.AggregatorSessionPolicy.Reuse {
		t.Fatal("role runtime aggregator should default reuse=true")
	}
	if !panel.RoleRuntime.AggregatorSessionPolicy.ResetPrompt {
		t.Fatal("role runtime aggregator should default reset_prompt=true")
	}
}
