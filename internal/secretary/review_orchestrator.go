package secretary

import (
	"fmt"
	"strings"

	"github.com/user/ai-workflow/internal/acpclient"
)

var requiredReviewerRoles = []string{
	"completeness",
	"dependency",
	"feasibility",
}

var defaultReviewSessionPolicy = acpclient.SessionPolicy{
	Reuse:       true,
	ResetPrompt: true,
}

type ReviewRoleBindingInput struct {
	Reviewers  map[string]string
	Aggregator string
}

type ReviewRoleRuntime struct {
	ReviewerRoles           map[string]string
	ReviewerSessionPolicies map[string]acpclient.SessionPolicy
	AggregatorRole          string
	AggregatorSessionPolicy acpclient.SessionPolicy
}

func ResolveReviewOrchestratorRoles(bindings ReviewRoleBindingInput, resolver *acpclient.RoleResolver) (*ReviewRoleRuntime, error) {
	out := &ReviewRoleRuntime{
		ReviewerRoles:           make(map[string]string, len(requiredReviewerRoles)),
		ReviewerSessionPolicies: make(map[string]acpclient.SessionPolicy, len(requiredReviewerRoles)),
	}

	for _, reviewer := range requiredReviewerRoles {
		roleID := strings.TrimSpace(bindings.Reviewers[reviewer])
		if roleID == "" {
			return nil, fmt.Errorf("review role binding is required for reviewer %q", reviewer)
		}

		policy := defaultReviewSessionPolicy
		if resolver != nil {
			_, role, err := resolver.Resolve(roleID)
			if err != nil {
				return nil, fmt.Errorf("resolve review_orchestrator reviewer %q role %q: %w", reviewer, roleID, err)
			}
			policy = withDefaultReviewSessionPolicy(role.SessionPolicy)
		}

		out.ReviewerRoles[reviewer] = roleID
		out.ReviewerSessionPolicies[reviewer] = policy
	}

	aggregatorRole := strings.TrimSpace(bindings.Aggregator)
	if aggregatorRole == "" {
		return nil, fmt.Errorf("review role binding is required for aggregator")
	}

	aggregatorPolicy := defaultReviewSessionPolicy
	if resolver != nil {
		_, role, err := resolver.Resolve(aggregatorRole)
		if err != nil {
			return nil, fmt.Errorf("resolve review_orchestrator aggregator role %q: %w", aggregatorRole, err)
		}
		aggregatorPolicy = withDefaultReviewSessionPolicy(role.SessionPolicy)
	}
	out.AggregatorRole = aggregatorRole
	out.AggregatorSessionPolicy = aggregatorPolicy
	return out, nil
}

func withDefaultReviewSessionPolicy(policy acpclient.SessionPolicy) acpclient.SessionPolicy {
	out := policy
	if !out.Reuse {
		out.Reuse = true
	}
	if !out.ResetPrompt {
		out.ResetPrompt = true
	}
	return out
}

func cloneReviewRoleRuntime(in *ReviewRoleRuntime) *ReviewRoleRuntime {
	if in == nil {
		return nil
	}
	out := &ReviewRoleRuntime{
		ReviewerRoles:           make(map[string]string, len(in.ReviewerRoles)),
		ReviewerSessionPolicies: make(map[string]acpclient.SessionPolicy, len(in.ReviewerSessionPolicies)),
		AggregatorRole:          in.AggregatorRole,
		AggregatorSessionPolicy: in.AggregatorSessionPolicy,
	}
	for k, v := range in.ReviewerRoles {
		out.ReviewerRoles[k] = v
	}
	for k, v := range in.ReviewerSessionPolicies {
		out.ReviewerSessionPolicies[k] = v
	}
	return out
}
