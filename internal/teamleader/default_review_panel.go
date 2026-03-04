package teamleader

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/yoke233/ai-workflow/internal/acpclient"
	"github.com/yoke233/ai-workflow/internal/core"
)

const (
	defaultDemandReviewerName     = "completeness"
	defaultDependencyAnalyzerName = "dependency"
	defaultTemplateReviewerName   = "feasibility"
	defaultReviewerRoleID         = "reviewer"
	defaultAggregatorRoleID       = "aggregator"
)

// NewDefaultReviewOrchestrator builds a runnable review orchestrator with built-in Issue analyzers.
// This keeps local runtime path working without requiring external AI backends.
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
			newDefaultDemandReviewer(),
			newDefaultDependencyAnalyzer(),
			newDefaultTemplateReviewer(),
		},
		Aggregator:  newDefaultIssueAggregator(),
		MaxRounds:   defaultReviewMaxRounds,
		RoleRuntime: cloneReviewRoleRuntime(effectiveRuntime),
	}
}

func defaultReviewRoleBindings() ReviewRoleBindingInput {
	return ReviewRoleBindingInput{
		Reviewers: map[string]string{
			defaultDemandReviewerName:     defaultReviewerRoleID,
			defaultDependencyAnalyzerName: defaultReviewerRoleID,
			defaultTemplateReviewerName:   defaultReviewerRoleID,
		},
		Aggregator: defaultAggregatorRoleID,
	}
}

func defaultReviewRoleRuntime() *ReviewRoleRuntime {
	runtime, err := ResolveReviewOrchestratorRoles(defaultReviewRoleBindings(), nil)
	if err != nil {
		return &ReviewRoleRuntime{
			ReviewerRoles: map[string]string{
				defaultDemandReviewerName:     defaultReviewerRoleID,
				defaultDependencyAnalyzerName: defaultReviewerRoleID,
				defaultTemplateReviewerName:   defaultReviewerRoleID,
			},
			ReviewerSessionPolicies: map[string]acpclient.SessionPolicy{
				defaultDemandReviewerName:     defaultReviewSessionPolicy,
				defaultDependencyAnalyzerName: defaultReviewSessionPolicy,
				defaultTemplateReviewerName:   defaultReviewSessionPolicy,
			},
			AggregatorRole:          defaultAggregatorRoleID,
			AggregatorSessionPolicy: defaultReviewSessionPolicy,
		}
	}
	return runtime
}

type defaultDemandReviewer struct{}

func newDefaultDemandReviewer() Reviewer {
	return defaultDemandReviewer{}
}

func (r defaultDemandReviewer) Name() string {
	return defaultDemandReviewerName
}

func (r defaultDemandReviewer) Review(_ context.Context, input ReviewerInput) (core.ReviewVerdict, error) {
	if input.Plan == nil {
		return core.ReviewVerdict{}, fmt.Errorf("reviewer %s: plan is nil", r.Name())
	}

	issues := validateDemand(input)
	return buildReviewVerdict(r.Name(), issues), nil
}

func validateDemand(input ReviewerInput) []core.ReviewIssue {
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

	planIssues := collectPlanIssueViews(plan)
	if len(planIssues) == 0 {
		return []core.ReviewIssue{
			{
				Severity:    "error",
				Description: "issue plan has no issues",
				Suggestion:  "add at least one executable issue",
			},
		}
	}

	for i := range planIssues {
		issue := planIssues[i]
		issueID := strings.TrimSpace(issue.ID)

		title := strings.TrimSpace(issue.Title)
		if title == "" {
			issues = append(issues, core.ReviewIssue{
				Severity:    "error",
				IssueID:     issueID,
				Description: "issue title is required",
				Suggestion:  "provide a clear issue title",
			})
		}

		body := strings.TrimSpace(issue.Body)
		if body == "" {
			issues = append(issues, core.ReviewIssue{
				Severity:    "error",
				IssueID:     issueID,
				Description: "issue body is required",
				Suggestion:  "describe scope, acceptance, and constraints in body",
			})
		}

		template := strings.TrimSpace(issue.Template)
		if template == "" {
			issues = append(issues, core.ReviewIssue{
				Severity:    "error",
				IssueID:     issueID,
				Description: "issue template is required",
				Suggestion:  "set a valid template name",
			})
		} else if strings.ContainsAny(template, " \t\r\n") {
			issues = append(issues, core.ReviewIssue{
				Severity:    "error",
				IssueID:     issueID,
				Description: "issue template must not contain spaces",
				Suggestion:  "use one token template id such as standard/full/quick/hotfix",
			})
		}

		seenAttachments := make(map[string]struct{}, len(issue.Attachments))
		for idx, rawAttachment := range issue.Attachments {
			attachment := strings.TrimSpace(rawAttachment)
			if attachment == "" {
				issues = append(issues, core.ReviewIssue{
					Severity:    "error",
					IssueID:     issueID,
					Description: fmt.Sprintf("issue attachment[%d] is empty", idx),
					Suggestion:  "remove empty attachment or provide a valid path/url",
				})
				continue
			}
			if _, duplicated := seenAttachments[attachment]; duplicated {
				issues = append(issues, core.ReviewIssue{
					Severity:    "warning",
					IssueID:     issueID,
					Description: fmt.Sprintf("issue attachment is duplicated: %s", attachment),
					Suggestion:  "deduplicate repeated attachment entries",
				})
				continue
			}
			seenAttachments[attachment] = struct{}{}
		}
	}

	return issues
}

type defaultDependencyAnalyzer struct{}

func newDefaultDependencyAnalyzer() Reviewer {
	return defaultDependencyAnalyzer{}
}

func (r defaultDependencyAnalyzer) Name() string {
	return defaultDependencyAnalyzerName
}

func (r defaultDependencyAnalyzer) Review(_ context.Context, input ReviewerInput) (core.ReviewVerdict, error) {
	if input.Plan == nil {
		return core.ReviewVerdict{}, fmt.Errorf("reviewer %s: plan is nil", r.Name())
	}
	if isFileBasedPlan(input.Plan, input.PlanFileContents) {
		return buildReviewVerdict(r.Name(), nil), nil
	}

	planIssues := collectPlanIssueViews(input.Plan)
	if len(planIssues) == 0 {
		return buildReviewVerdict(r.Name(), nil), nil
	}

	issues := make([]core.ReviewIssue, 0)
	for i := range planIssues {
		issue := planIssues[i]
		issueID := strings.TrimSpace(issue.ID)
		if issueID == "" {
			issues = append(issues, core.ReviewIssue{
				Severity:    "error",
				Description: "issue id is required for dependency analysis",
				Suggestion:  "assign stable issue IDs before declaring dependencies",
			})
			continue
		}
	}

	// V2 no longer uses runtime DAG dependencies. Dependency analyzer only
	// keeps minimal input sanity checks to avoid blocking issue-level flow.
	return buildReviewVerdict(r.Name(), issues), nil
}

type defaultTemplateReviewer struct{}

func newDefaultTemplateReviewer() Reviewer {
	return defaultTemplateReviewer{}
}

func (r defaultTemplateReviewer) Name() string {
	return defaultTemplateReviewerName
}

func (r defaultTemplateReviewer) Review(_ context.Context, input ReviewerInput) (core.ReviewVerdict, error) {
	if input.Plan == nil {
		return core.ReviewVerdict{}, fmt.Errorf("reviewer %s: plan is nil", r.Name())
	}
	if isFileBasedPlan(input.Plan, input.PlanFileContents) {
		return buildReviewVerdict(r.Name(), nil), nil
	}

	planIssues := collectPlanIssueViews(input.Plan)
	issues := make([]core.ReviewIssue, 0)
	for i := range planIssues {
		issue := planIssues[i]
		template := strings.TrimSpace(issue.Template)
		if template == "" {
			continue
		}
		if _, ok := allowedRunTemplates[template]; ok {
			continue
		}
		issues = append(issues, core.ReviewIssue{
			Severity:    "warning",
			IssueID:     strings.TrimSpace(issue.ID),
			Description: fmt.Sprintf("unknown template %q", template),
			Suggestion:  "use one of: full/standard/quick/hotfix",
		})
	}

	return buildReviewVerdict(r.Name(), issues), nil
}

var allowedRunTemplates = map[string]struct{}{
	"full":     {},
	"standard": {},
	"quick":    {},
	"hotfix":   {},
}

type defaultIssueAggregator struct{}

func newDefaultIssueAggregator() Aggregator {
	return defaultIssueAggregator{}
}

func (a defaultIssueAggregator) Decide(_ context.Context, input AggregatorInput) (AggregatorDecision, error) {
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

type issueView struct {
	ID          string
	Title       string
	Body        string
	Template    string
	Attachments []string
	DependsOn   []string
}

func collectPlanIssueViews(plan any) []issueView {
	if plan == nil {
		return nil
	}

	planValue, ok := derefStructValue(reflect.ValueOf(plan))
	if !ok {
		return nil
	}

	issueSlice := planValue.FieldByName("Tasks")
	if !issueSlice.IsValid() {
		issueSlice = planValue.FieldByName("Issues")
	}
	if !issueSlice.IsValid() || issueSlice.Kind() != reflect.Slice {
		return nil
	}

	out := make([]issueView, 0, issueSlice.Len())
	for i := 0; i < issueSlice.Len(); i++ {
		itemValue, ok := derefStructValue(issueSlice.Index(i))
		if !ok {
			continue
		}

		out = append(out, issueView{
			ID:          readStructStringField(itemValue, "ID"),
			Title:       readStructStringField(itemValue, "Title"),
			Body:        coalesceStringField(itemValue, "Body", "Description"),
			Template:    readStructStringField(itemValue, "Template"),
			Attachments: readStructStringSliceField(itemValue, "Attachments"),
			DependsOn:   readStructStringSliceField(itemValue, "DependsOn"),
		})
	}

	return out
}

func derefStructValue(v reflect.Value) (reflect.Value, bool) {
	for v.IsValid() {
		switch v.Kind() {
		case reflect.Interface, reflect.Pointer:
			if v.IsNil() {
				return reflect.Value{}, false
			}
			v = v.Elem()
		default:
			if v.Kind() != reflect.Struct {
				return reflect.Value{}, false
			}
			return v, true
		}
	}
	return reflect.Value{}, false
}

func readStructStringField(v reflect.Value, name string) string {
	field := v.FieldByName(name)
	if !field.IsValid() || field.Kind() != reflect.String {
		return ""
	}
	return field.String()
}

func coalesceStringField(v reflect.Value, names ...string) string {
	for _, name := range names {
		value := strings.TrimSpace(readStructStringField(v, name))
		if value != "" {
			return value
		}
	}
	return ""
}

func readStructStringSliceField(v reflect.Value, name string) []string {
	field := v.FieldByName(name)
	if !field.IsValid() || field.Kind() != reflect.Slice {
		return nil
	}
	out := make([]string, 0, field.Len())
	for i := 0; i < field.Len(); i++ {
		item := field.Index(i)
		if item.Kind() == reflect.String {
			out = append(out, item.String())
		}
	}
	return out
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if _, duplicated := seen[value]; duplicated {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func buildReviewVerdict(name string, issues []core.ReviewIssue) core.ReviewVerdict {
	status := "pass"
	score := 100
	if len(issues) > 0 {
		status = "issues_found"
		score = 60
	}
	summary := defaultVerdictSummary(status, score, len(issues))
	return core.ReviewVerdict{
		Reviewer:  name,
		Status:    status,
		Summary:   summary,
		RawOutput: formatReviewRawOutput(summary, status, score, issues),
		Issues:    issues,
		Score:     score,
	}
}
