package secretary

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/user/ai-workflow/internal/acpclient"
	"github.com/user/ai-workflow/internal/core"
)

const (
	defaultCompletenessReviewerName = "completeness"
	defaultDependencyReviewerName   = "dependency"
	defaultFeasibilityReviewerName  = "feasibility"
	defaultReviewerRoleID           = "reviewer"
	defaultAggregatorRoleID         = "aggregator"
)

// NewDefaultReviewOrchestrator builds a runnable review orchestrator with rule-based reviewers.
// This keeps P2 production path working without requiring external AI backends.
func NewDefaultReviewOrchestrator(store ReviewStore) *ReviewOrchestrator {
	return newDefaultReviewOrchestrator(store, defaultReviewRoleRuntime())
}

func NewDefaultReviewOrchestratorFromBindings(
	store ReviewStore,
	bindings ReviewRoleBindingInput,
	resolver *acpclient.RoleResolver,
) (*ReviewOrchestrator, error) {
	runtime, err := ResolveReviewOrchestratorRoles(bindings, resolver)
	if err != nil {
		return nil, err
	}
	return newDefaultReviewOrchestrator(store, runtime), nil
}

func newDefaultReviewOrchestrator(store ReviewStore, runtime *ReviewRoleRuntime) *ReviewOrchestrator {
	effectiveRuntime := runtime
	if effectiveRuntime == nil {
		effectiveRuntime = defaultReviewRoleRuntime()
	}
	return &ReviewOrchestrator{
		Store: store,
		Reviewers: []Reviewer{
			newCompletenessReviewer(),
			newDependencyReviewer(),
			newFeasibilityReviewer(),
		},
		Aggregator:  newRuleAggregator(),
		MaxRounds:   defaultReviewMaxRounds,
		RoleRuntime: cloneReviewRoleRuntime(effectiveRuntime),
	}
}

func defaultReviewRoleBindings() ReviewRoleBindingInput {
	return ReviewRoleBindingInput{
		Reviewers: map[string]string{
			defaultCompletenessReviewerName: defaultReviewerRoleID,
			defaultDependencyReviewerName:   defaultReviewerRoleID,
			defaultFeasibilityReviewerName:  defaultReviewerRoleID,
		},
		Aggregator: defaultAggregatorRoleID,
	}
}

func defaultReviewRoleRuntime() *ReviewRoleRuntime {
	runtime, err := ResolveReviewOrchestratorRoles(defaultReviewRoleBindings(), nil)
	if err != nil {
		return &ReviewRoleRuntime{
			ReviewerRoles: map[string]string{
				defaultCompletenessReviewerName: defaultReviewerRoleID,
				defaultDependencyReviewerName:   defaultReviewerRoleID,
				defaultFeasibilityReviewerName:  defaultReviewerRoleID,
			},
			ReviewerSessionPolicies: map[string]acpclient.SessionPolicy{
				defaultCompletenessReviewerName: defaultReviewSessionPolicy,
				defaultDependencyReviewerName:   defaultReviewSessionPolicy,
				defaultFeasibilityReviewerName:  defaultReviewSessionPolicy,
			},
			AggregatorRole:          defaultAggregatorRoleID,
			AggregatorSessionPolicy: defaultReviewSessionPolicy,
		}
	}
	return runtime
}

type ruleReviewer struct {
	name string
	run  func(input ReviewerInput) []core.ReviewIssue
}

func (r ruleReviewer) Name() string {
	return r.name
}

func (r ruleReviewer) Review(_ context.Context, input ReviewerInput) (core.ReviewVerdict, error) {
	if input.Plan == nil {
		return core.ReviewVerdict{}, fmt.Errorf("reviewer %s: plan is nil", r.name)
	}
	issues := r.run(input)
	status := "pass"
	score := 100
	if len(issues) > 0 {
		status = "issues_found"
		score = 60
	}
	return core.ReviewVerdict{
		Reviewer: r.name,
		Status:   status,
		Issues:   issues,
		Score:    score,
	}, nil
}

func newCompletenessReviewer() Reviewer {
	return ruleReviewer{
		name: defaultCompletenessReviewerName,
		run: func(input ReviewerInput) []core.ReviewIssue {
			plan := input.Plan
			issues := make([]core.ReviewIssue, 0)
			if isFileBasedPlan(plan, input.PlanFileContents) {
				fileContents := cloneStringMap(input.PlanFileContents)
				if len(fileContents) == 0 {
					fileContents = loadPlanFileContents(plan)
				}
				if len(fileContents) == 0 {
					return []core.ReviewIssue{
						{
							Severity:    "error",
							Description: "file-based plan has no file contents",
							Suggestion:  "provide non-empty file contents for parser input",
						},
					}
				}
				paths := make([]string, 0, len(fileContents))
				for path := range fileContents {
					paths = append(paths, path)
				}
				slices.Sort(paths)
				for _, path := range paths {
					if strings.TrimSpace(fileContents[path]) != "" {
						continue
					}
					issues = append(issues, core.ReviewIssue{
						Severity:    "error",
						Description: fmt.Sprintf("file content is empty: %s", path),
						Suggestion:  "provide non-empty file content for each source file",
					})
				}
				return issues
			}
			if len(plan.Tasks) == 0 {
				return []core.ReviewIssue{
					{
						Severity:    "error",
						Description: "task plan has no tasks",
						Suggestion:  "add at least one executable task",
						TaskID:      "",
					},
				}
			}

			for i := range plan.Tasks {
				task := plan.Tasks[i]
				if strings.TrimSpace(task.Title) == "" {
					issues = append(issues, core.ReviewIssue{
						Severity:    "error",
						Description: "task title is required",
						Suggestion:  "provide a clear task title",
						TaskID:      strings.TrimSpace(task.ID),
					})
				}
				if strings.TrimSpace(task.Description) == "" {
					issues = append(issues, core.ReviewIssue{
						Severity:    "error",
						Description: "task description is required",
						Suggestion:  "provide acceptance criteria in description",
						TaskID:      strings.TrimSpace(task.ID),
					})
				}
			}

			return issues
		},
	}
}

func newDependencyReviewer() Reviewer {
	return ruleReviewer{
		name: defaultDependencyReviewerName,
		run: func(input ReviewerInput) []core.ReviewIssue {
			if isFileBasedPlan(input.Plan, input.PlanFileContents) {
				return nil
			}
			plan := input.Plan
			dag := Build(plan.Tasks)
			if err := dag.Validate(); err != nil {
				return []core.ReviewIssue{
					{
						Severity:    "error",
						Description: err.Error(),
						Suggestion:  "fix dependency graph to satisfy DAG constraints",
					},
				}
			}
			return nil
		},
	}
}

func newFeasibilityReviewer() Reviewer {
	return ruleReviewer{
		name: defaultFeasibilityReviewerName,
		run: func(input ReviewerInput) []core.ReviewIssue {
			if isFileBasedPlan(input.Plan, input.PlanFileContents) {
				return nil
			}
			plan := input.Plan
			issues := make([]core.ReviewIssue, 0)
			for i := range plan.Tasks {
				task := plan.Tasks[i]
				template := strings.TrimSpace(task.Template)
				if template == "" {
					continue
				}
				if _, ok := allowedPipelineTemplates[template]; ok {
					continue
				}
				issues = append(issues, core.ReviewIssue{
					Severity:    "warning",
					Description: fmt.Sprintf("unknown template %q", template),
					Suggestion:  "use one of: full/standard/quick/hotfix",
					TaskID:      strings.TrimSpace(task.ID),
				})
			}
			return issues
		},
	}
}

var allowedPipelineTemplates = map[string]struct{}{
	"full":     {},
	"standard": {},
	"quick":    {},
	"hotfix":   {},
}

type ruleAggregator struct{}

func newRuleAggregator() Aggregator {
	return ruleAggregator{}
}

func (a ruleAggregator) Decide(_ context.Context, input AggregatorInput) (AggregatorDecision, error) {
	allIssues := collectAllIssues(input.Verdicts)
	if len(allIssues) == 0 {
		return AggregatorDecision{
			Decision: DecisionApprove,
		}, nil
	}

	reason := "review issues found"
	for i := range allIssues {
		issue := allIssues[i]
		if strings.TrimSpace(issue.Description) != "" {
			reason = issue.Description
			break
		}
	}

	// V1 runtime path: when issues exist we escalate to human feedback directly.
	return AggregatorDecision{
		Decision: DecisionEscalate,
		Reason:   reason,
	}, nil
}
