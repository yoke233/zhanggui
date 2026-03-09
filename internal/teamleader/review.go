package teamleader

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/yoke233/ai-workflow/internal/core"
)

const (
	DecisionApprove  = "approve"
	DecisionFix      = "fix"
	DecisionEscalate = "escalate"

	defaultReviewMaxRounds = 2

	phase1ReviewerName       = "demand_reviewer"
	phase2ReviewerName       = "aggregator"
	strictReviewerNamePrefix = "strict_reviewer"
)

// ReviewStore persists review records using IssueID as the primary key.
type ReviewStore interface {
	SaveReviewRecord(record *core.ReviewRecord) error
	GetReviewRecords(issueID string) ([]core.ReviewRecord, error)
}

// DemandReviewer performs per-issue quality review in phase 1.
type DemandReviewer interface {
	Review(ctx context.Context, issue *core.Issue) (core.ReviewVerdict, error)
}

// DependencyAnalyzer performs cross-issue dependency analysis in phase 2.
type DependencyAnalyzer interface {
	Analyze(ctx context.Context, issues []*core.Issue) (*DependencyAnalysis, error)
}

// Reviewer is a compatibility interface for legacy review panel implementations.
type Reviewer interface {
	Name() string
	Review(ctx context.Context, input ReviewerInput) (core.ReviewVerdict, error)
}

// Aggregator is a compatibility interface for legacy review panel implementations.
type Aggregator interface {
	Decide(ctx context.Context, input AggregatorInput) (AggregatorDecision, error)
}

type ReviewerInput struct {
	Plan             any
	Issue            *core.Issue
	Conversation     string
	ProjectContext   string
	PlanFileContents map[string]string
}

type AggregatorInput struct {
	Round    int
	Verdicts []core.ReviewVerdict
}

type AggregatorDecision struct {
	Decision string
	Reason   string
}

type ReviewInput struct {
	Conversation     string
	ProjectContext   string
	PlanFileContents map[string]string
}

type DependencyAnalysis struct {
	Edges      []DependencyEdge     `json:"edges,omitempty"`
	Conflicts  []ConflictInfo       `json:"conflicts,omitempty"`
	Priorities []PrioritySuggestion `json:"priorities,omitempty"`
}

type DependencyEdge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Reason string `json:"reason,omitempty"`
}

type ConflictInfo struct {
	IssueIDs   []string `json:"issue_ids"`
	Resource   string   `json:"resource,omitempty"`
	Suggestion string   `json:"suggestion,omitempty"`
}

type PrioritySuggestion struct {
	IssueID  string `json:"issue_id"`
	Priority int    `json:"priority"`
	Reason   string `json:"reason,omitempty"`
}

type ReviewSessionResult struct {
	Status       string                        `json:"status"`
	Decision     string                        `json:"decision"`
	Verdicts     map[string]core.ReviewVerdict `json:"verdicts"`
	DAG          *DependencyAnalysis           `json:"dag,omitempty"`
	AutoApproved bool                          `json:"auto_approved"`
}

// TwoPhaseReview executes demand review and dependency analysis with Issue semantics.
type TwoPhaseReview struct {
	Store                ReviewStore
	Reviewer             DemandReviewer
	Analyzer             DependencyAnalyzer
	AutoApproveThreshold int
}

// ReviewOrchestrator keeps legacy field names while delegating to TwoPhaseReview.
type ReviewOrchestrator struct {
	Store                ReviewStore
	Reviewer             DemandReviewer
	Analyzer             DependencyAnalyzer
	AutoApproveThreshold int

	Reviewers   []Reviewer
	Aggregator  Aggregator
	MaxRounds   int
	RoleRuntime *ReviewRoleRuntime
}

func (r *ReviewOrchestrator) Run(ctx context.Context, issues []*core.Issue) (*ReviewSessionResult, error) {
	engine := r.toTwoPhaseReview()
	return engine.Run(ctx, issues)
}

func (r *ReviewOrchestrator) SubmitForReview(ctx context.Context, issues []*core.Issue) error {
	_, err := r.Run(ctx, issues)
	return err
}

func (r *ReviewOrchestrator) DemandReviewer() DemandReviewer {
	engine := r.toTwoPhaseReview()
	if engine == nil {
		return nil
	}
	return engine.Reviewer
}

func (r *ReviewOrchestrator) toTwoPhaseReview() *TwoPhaseReview {
	if r == nil {
		return nil
	}
	out := &TwoPhaseReview{
		Store:                r.Store,
		Reviewer:             r.Reviewer,
		Analyzer:             r.Analyzer,
		AutoApproveThreshold: r.AutoApproveThreshold,
	}
	if out.Reviewer == nil && len(r.Reviewers) > 0 && r.Reviewers[0] != nil {
		out.Reviewer = reviewerAdapter{inner: r.Reviewers[0]}
	}
	return out
}

type reviewerAdapter struct {
	inner Reviewer
}

func (a reviewerAdapter) Review(ctx context.Context, issue *core.Issue) (core.ReviewVerdict, error) {
	if a.inner == nil {
		return core.ReviewVerdict{}, errors.New("reviewer adapter inner reviewer is nil")
	}
	clonedIssue := cloneIssueForReview(issue)
	// Default reviewers consume plan-level issue slices via reflection.
	// Provide a minimal plan envelope so single-issue submit path is compatible.
	plan := struct {
		Issues []*core.Issue
	}{
		Issues: []*core.Issue{clonedIssue},
	}
	return a.inner.Review(ctx, ReviewerInput{
		Plan:  plan,
		Issue: clonedIssue,
	})
}

func (r *TwoPhaseReview) Run(ctx context.Context, issues []*core.Issue) (*ReviewSessionResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := r.validate(); err != nil {
		return nil, err
	}

	normalizedIssues, err := normalizeIssues(issues)
	if err != nil {
		return nil, err
	}
	profile := resolveReviewProfile(normalizedIssues)

	round, err := r.nextRound(normalizedIssues)
	if err != nil {
		return nil, err
	}

	verdicts, phase1NeedsFix, allMeetThreshold, err := r.runPhase1(ctx, normalizedIssues, round, profile)
	if err != nil {
		return nil, err
	}

	analysis, err := r.runPhase2(ctx, normalizedIssues, round, profile, verdicts)
	if err != nil {
		return nil, err
	}

	decision, status, autoApproved := decideSession(phase1NeedsFix, allMeetThreshold, r.AutoApproveThreshold, analysis)
	return &ReviewSessionResult{
		Status:       status,
		Decision:     decision,
		Verdicts:     verdicts,
		DAG:          analysis,
		AutoApproved: autoApproved,
	}, nil
}

func (r *TwoPhaseReview) validate() error {
	if r == nil {
		return errors.New("two phase review is nil")
	}
	if r.Store == nil {
		return errors.New("review store is required")
	}
	if r.Reviewer == nil {
		return errors.New("demand reviewer is required")
	}
	return nil
}

func (r *TwoPhaseReview) nextRound(issues []*core.Issue) (int, error) {
	maxRound := 0
	for i := range issues {
		records, err := r.Store.GetReviewRecords(issues[i].ID)
		if err != nil {
			return 0, fmt.Errorf("load review records for issue %s: %w", issues[i].ID, err)
		}
		for j := range records {
			if records[j].Round > maxRound {
				maxRound = records[j].Round
			}
		}
	}
	return maxRound + 1, nil
}

func (r *TwoPhaseReview) runPhase1(
	ctx context.Context,
	issues []*core.Issue,
	round int,
	profile core.WorkflowProfileType,
) (map[string]core.ReviewVerdict, bool, bool, error) {
	policy := reviewProfilePolicy(profile, r.AutoApproveThreshold)
	threshold := policy.Threshold
	allMeetThreshold := true
	needsFix := false

	out := make(map[string]core.ReviewVerdict, len(issues))
	for i := range issues {
		issue := issues[i]

		aggregated := core.ReviewVerdict{
			Reviewer: phase1ReviewerName,
			Status:   "pass",
			Score:    100,
			Issues:   []core.ReviewIssue{},
		}

		for reviewerIndex := 0; reviewerIndex < policy.ReviewerCount; reviewerIndex++ {
			verdict, err := r.Reviewer.Review(ctx, cloneIssueForReview(issue))
			if err != nil {
				return nil, false, false, fmt.Errorf("phase1 review issue %s: %w", issue.ID, err)
			}

			normalized := normalizeVerdict(issue.ID, verdict)
			recordReviewer := normalizedReviewer(normalized.Reviewer, phase1ReviewerName)
			if policy.ReviewerCount > 1 {
				recordReviewer = fmt.Sprintf("%s_%d", strictReviewerNamePrefix, reviewerIndex+1)
			}

			score := normalized.Score
			record := &core.ReviewRecord{
				IssueID:   issue.ID,
				Round:     round,
				Reviewer:  recordReviewer,
				Verdict:   normalized.Status,
				Summary:   strings.TrimSpace(normalized.Summary),
				RawOutput: strings.TrimSpace(normalized.RawOutput),
				Issues:    append([]core.ReviewIssue(nil), normalized.Issues...),
				Score:     &score,
			}
			if err := r.Store.SaveReviewRecord(record); err != nil {
				return nil, false, false, fmt.Errorf("persist phase1 review record for issue %s: %w", issue.ID, err)
			}

			if verdictNeedsFix(normalized) {
				aggregated.Status = "issues_found"
				aggregated.Issues = append(aggregated.Issues, normalized.Issues...)
			}
			if normalized.Score < aggregated.Score {
				aggregated.Score = normalized.Score
			}
		}
		aggregated.Summary = defaultVerdictSummary(aggregated.Status, aggregated.Score, len(aggregated.Issues))
		aggregated.RawOutput = fmt.Sprintf("profile=%s reviewers=%d", profile, policy.ReviewerCount)
		out[issue.ID] = aggregated

		if verdictNeedsFix(aggregated) {
			needsFix = true
		}
		if threshold > 0 && aggregated.Score < threshold {
			allMeetThreshold = false
		}
	}
	return out, needsFix, allMeetThreshold, nil
}

func (r *TwoPhaseReview) runPhase2(
	ctx context.Context,
	issues []*core.Issue,
	round int,
	profile core.WorkflowProfileType,
	phase1Verdicts map[string]core.ReviewVerdict,
) (*DependencyAnalysis, error) {
	analysis := &DependencyAnalysis{}
	if len(issues) > 1 && r.Analyzer != nil && profile != core.WorkflowProfileFastRelease {
		out, err := r.Analyzer.Analyze(ctx, cloneIssueListForReview(issues))
		if err != nil {
			return nil, fmt.Errorf("phase2 dependency analyze: %w", err)
		}
		if out != nil {
			analysis = cloneDependencyAnalysis(out)
		}
	}

	for i := range issues {
		issueID := issues[i].ID
		phase1 := phase1Verdicts[issueID]
		analysisIssues := dependencyIssuesForIssue(issueID, analysis)
		verdict := "pass"
		score := 100
		if verdictNeedsFix(phase1) {
			verdict = "issues_found"
			score = 60
		}
		analysisIssues = append(analysisIssues, phase1.Issues...)
		if len(analysisIssues) > 0 {
			verdict = "issues_found"
			score = 60
		}
		summary := dependencyReviewSummary(verdict, len(analysisIssues))
		if profile == core.WorkflowProfileFastRelease && verdict == "pass" {
			summary = "fast_release profile quick pass"
		}
		rawOutput := dependencyReviewRawOutput(issueID, verdict, score, analysis, analysisIssues, summary)

		record := &core.ReviewRecord{
			IssueID:   issueID,
			Round:     round,
			Reviewer:  phase2ReviewerName,
			Verdict:   verdict,
			Summary:   summary,
			RawOutput: rawOutput,
			Issues:    analysisIssues,
			Score:     &score,
		}
		if err := r.Store.SaveReviewRecord(record); err != nil {
			return nil, fmt.Errorf("persist phase2 review record for issue %s: %w", issueID, err)
		}
	}

	return analysis, nil
}

type profileReviewPolicy struct {
	ReviewerCount int
	Threshold     int
}

func reviewProfilePolicy(profile core.WorkflowProfileType, autoApproveThreshold int) profileReviewPolicy {
	switch profile {
	case core.WorkflowProfileStrict:
		threshold := clampThreshold(autoApproveThreshold)
		if threshold < 85 {
			threshold = 85
		}
		return profileReviewPolicy{
			ReviewerCount: 3,
			Threshold:     threshold,
		}
	case core.WorkflowProfileFastRelease:
		return profileReviewPolicy{
			ReviewerCount: 1,
			Threshold:     0,
		}
	default:
		return profileReviewPolicy{
			ReviewerCount: 1,
			Threshold:     clampThreshold(autoApproveThreshold),
		}
	}
}

func resolveReviewProfile(issues []*core.Issue) core.WorkflowProfileType {
	resolved := core.WorkflowProfileFastRelease
	for i := range issues {
		profile := workflowProfileFromIssue(issues[i])
		switch profile {
		case core.WorkflowProfileStrict:
			return core.WorkflowProfileStrict
		case core.WorkflowProfileNormal:
			resolved = core.WorkflowProfileNormal
		}
	}
	if resolved.Validate() == nil {
		return resolved
	}
	return core.WorkflowProfileNormal
}

func decideSession(phase1NeedsFix bool, allMeetThreshold bool, threshold int, analysis *DependencyAnalysis) (decision string, status string, autoApproved bool) {
	hasConflicts := hasDependencyConflicts(analysis)
	thresholdEnabled := clampThreshold(threshold) > 0

	switch {
	case hasConflicts:
		return DecisionEscalate, core.ReviewStatusRejected, false
	case phase1NeedsFix:
		return DecisionFix, core.ReviewStatusChangesRequested, false
	case thresholdEnabled && !allMeetThreshold:
		return DecisionFix, core.ReviewStatusChangesRequested, false
	default:
		return DecisionApprove, core.ReviewStatusApproved, true
	}
}

func normalizeIssues(issues []*core.Issue) ([]*core.Issue, error) {
	if len(issues) == 0 {
		return nil, errors.New("issues are required")
	}

	out := make([]*core.Issue, 0, len(issues))
	seen := make(map[string]struct{}, len(issues))
	for i := range issues {
		issue := issues[i]
		if issue == nil {
			return nil, fmt.Errorf("issue[%d] is nil", i)
		}
		issueID := strings.TrimSpace(issue.ID)
		if issueID == "" {
			return nil, fmt.Errorf("issue[%d] id is required", i)
		}
		if _, exists := seen[issueID]; exists {
			return nil, fmt.Errorf("duplicate issue id %q", issueID)
		}
		seen[issueID] = struct{}{}

		cloned := cloneIssueForReview(issue)
		cloned.ID = issueID
		out = append(out, cloned)
	}
	return out, nil
}

func cloneIssueForReview(issue *core.Issue) *core.Issue {
	if issue == nil {
		return nil
	}
	out := *issue
	out.Labels = append([]string(nil), issue.Labels...)
	out.Attachments = append([]string(nil), issue.Attachments...)
	out.DependsOn = append([]string(nil), issue.DependsOn...)
	out.Blocks = append([]string(nil), issue.Blocks...)
	return &out
}

func cloneIssueListForReview(issues []*core.Issue) []*core.Issue {
	out := make([]*core.Issue, 0, len(issues))
	for i := range issues {
		out = append(out, cloneIssueForReview(issues[i]))
	}
	return out
}

func normalizeVerdict(issueID string, verdict core.ReviewVerdict) core.ReviewVerdict {
	status := normalizeVerdictStatus(verdict.Status, len(verdict.Issues))
	reviewer := normalizedReviewer(verdict.Reviewer, phase1ReviewerName)
	score := verdict.Score
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	if score == 0 && status == "pass" {
		score = 100
	}

	issues := make([]core.ReviewIssue, 0, len(verdict.Issues))
	for i := range verdict.Issues {
		item := verdict.Issues[i]
		item.IssueID = strings.TrimSpace(item.IssueID)
		if item.IssueID == "" {
			item.IssueID = issueID
		}
		issues = append(issues, item)
	}
	summary := strings.TrimSpace(verdict.Summary)
	if summary == "" {
		summary = defaultVerdictSummary(status, score, len(issues))
	}
	rawOutput := strings.TrimSpace(verdict.RawOutput)
	if rawOutput == "" {
		rawOutput = formatReviewRawOutput(summary, status, score, issues)
	}

	return core.ReviewVerdict{
		Reviewer:  reviewer,
		Status:    status,
		Summary:   summary,
		RawOutput: rawOutput,
		Issues:    issues,
		Score:     score,
	}
}

func normalizeVerdictStatus(raw string, issueCount int) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "", "ok", "approved":
		if issueCount > 0 {
			return "issues_found"
		}
		return "pass"
	case "pass", "issues_found", "pending", "rejected", "changes_requested":
		return value
	default:
		if issueCount > 0 {
			return "issues_found"
		}
		return "pass"
	}
}

func normalizedReviewer(name string, fallback string) string {
	value := strings.TrimSpace(name)
	if value == "" {
		return fallback
	}
	return value
}

func verdictNeedsFix(verdict core.ReviewVerdict) bool {
	if len(verdict.Issues) > 0 {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(verdict.Status)) {
	case "pass", "approved":
		return false
	default:
		return true
	}
}

func hasDependencyConflicts(analysis *DependencyAnalysis) bool {
	if analysis == nil {
		return false
	}
	return len(analysis.Conflicts) > 0
}

func dependencyIssuesForIssue(issueID string, analysis *DependencyAnalysis) []core.ReviewIssue {
	if analysis == nil {
		return nil
	}

	out := make([]core.ReviewIssue, 0)
	for i := range analysis.Conflicts {
		conflict := analysis.Conflicts[i]
		if !containsIssueID(conflict.IssueIDs, issueID) {
			continue
		}
		desc := "dependency conflict detected"
		if resource := strings.TrimSpace(conflict.Resource); resource != "" {
			desc = fmt.Sprintf("dependency conflict on %s", resource)
		}
		out = append(out, core.ReviewIssue{
			Severity:    "warning",
			IssueID:     issueID,
			Description: desc,
			Suggestion:  strings.TrimSpace(conflict.Suggestion),
		})
	}
	return out
}

func defaultVerdictSummary(status string, score int, issueCount int) string {
	if issueCount > 0 {
		return fmt.Sprintf("发现 %d 个待处理问题（score=%d）", issueCount, score)
	}
	normalizedStatus := strings.ToLower(strings.TrimSpace(status))
	if normalizedStatus == "pass" || normalizedStatus == "approved" {
		return fmt.Sprintf("评审通过（score=%d）", score)
	}
	return fmt.Sprintf("评审状态=%s（score=%d）", strings.TrimSpace(status), score)
}

func formatReviewRawOutput(summary string, status string, score int, issues []core.ReviewIssue) string {
	lines := make([]string, 0, len(issues)+4)
	if text := strings.TrimSpace(summary); text != "" {
		lines = append(lines, "summary: "+text)
	}
	lines = append(lines, "status: "+strings.TrimSpace(status))
	lines = append(lines, fmt.Sprintf("score: %d", score))
	if len(issues) == 0 {
		lines = append(lines, "issues: none")
		return strings.Join(lines, "\n")
	}

	lines = append(lines, "issues:")
	for i := range issues {
		item := issues[i]
		severity := strings.TrimSpace(item.Severity)
		if severity == "" {
			severity = "unknown"
		}
		desc := strings.TrimSpace(item.Description)
		if desc == "" {
			desc = "(empty)"
		}
		line := fmt.Sprintf("%d. [%s] %s", i+1, severity, desc)
		if suggestion := strings.TrimSpace(item.Suggestion); suggestion != "" {
			line += " | suggestion: " + suggestion
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func dependencyReviewSummary(verdict string, issueCount int) string {
	normalizedVerdict := strings.ToLower(strings.TrimSpace(verdict))
	if issueCount == 0 {
		if normalizedVerdict != "" && normalizedVerdict != "pass" {
			return fmt.Sprintf("依赖关系检查状态=%s，可继续执行", normalizedVerdict)
		}
		return "依赖关系检查通过，可继续执行"
	}
	return fmt.Sprintf("依赖关系检查发现 %d 个问题，建议先修复", issueCount)
}

func dependencyReviewRawOutput(
	issueID string,
	verdict string,
	score int,
	analysis *DependencyAnalysis,
	analysisIssues []core.ReviewIssue,
	summary string,
) string {
	lines := []string{
		"summary: " + strings.TrimSpace(summary),
		"verdict: " + strings.TrimSpace(verdict),
		fmt.Sprintf("score: %d", score),
		fmt.Sprintf("issue_id: %s", strings.TrimSpace(issueID)),
	}
	if len(analysisIssues) > 0 {
		lines = append(lines, "detected_issues:")
		for i := range analysisIssues {
			item := analysisIssues[i]
			line := fmt.Sprintf("%d. %s", i+1, strings.TrimSpace(item.Description))
			if suggestion := strings.TrimSpace(item.Suggestion); suggestion != "" {
				line += " | suggestion: " + suggestion
			}
			lines = append(lines, line)
		}
	} else {
		lines = append(lines, "detected_issues: none")
	}

	if analysis == nil {
		return strings.Join(lines, "\n")
	}

	if len(analysis.Edges) > 0 {
		lines = append(lines, "dependency_edges:")
		for i := range analysis.Edges {
			edge := analysis.Edges[i]
			lines = append(lines, fmt.Sprintf(
				"%d. %s -> %s | reason: %s",
				i+1,
				strings.TrimSpace(edge.From),
				strings.TrimSpace(edge.To),
				strings.TrimSpace(edge.Reason),
			))
		}
	}
	if len(analysis.Conflicts) > 0 {
		lines = append(lines, "conflicts:")
		for i := range analysis.Conflicts {
			conflict := analysis.Conflicts[i]
			lines = append(lines, fmt.Sprintf(
				"%d. issues=%s | resource=%s | suggestion=%s",
				i+1,
				strings.Join(conflict.IssueIDs, ","),
				strings.TrimSpace(conflict.Resource),
				strings.TrimSpace(conflict.Suggestion),
			))
		}
	}
	if len(analysis.Priorities) > 0 {
		lines = append(lines, "priority_suggestions:")
		for i := range analysis.Priorities {
			priority := analysis.Priorities[i]
			lines = append(lines, fmt.Sprintf(
				"%d. issue=%s priority=%d reason=%s",
				i+1,
				strings.TrimSpace(priority.IssueID),
				priority.Priority,
				strings.TrimSpace(priority.Reason),
			))
		}
	}
	return strings.Join(lines, "\n")
}

func containsIssueID(ids []string, target string) bool {
	for i := range ids {
		if strings.TrimSpace(ids[i]) == target {
			return true
		}
	}
	return false
}

func clampThreshold(raw int) int {
	if raw <= 0 {
		return 0
	}
	if raw > 100 {
		return 100
	}
	return raw
}

func cloneDependencyAnalysis(in *DependencyAnalysis) *DependencyAnalysis {
	if in == nil {
		return &DependencyAnalysis{}
	}
	out := &DependencyAnalysis{
		Edges:      make([]DependencyEdge, 0, len(in.Edges)),
		Conflicts:  make([]ConflictInfo, 0, len(in.Conflicts)),
		Priorities: make([]PrioritySuggestion, 0, len(in.Priorities)),
	}

	out.Edges = append(out.Edges, in.Edges...)
	for i := range in.Conflicts {
		conflict := in.Conflicts[i]
		conflict.IssueIDs = append([]string(nil), conflict.IssueIDs...)
		out.Conflicts = append(out.Conflicts, conflict)
	}
	out.Priorities = append(out.Priorities, in.Priorities...)
	return out
}

func collectAllIssues(verdicts []core.ReviewVerdict) []core.ReviewIssue {
	if len(verdicts) == 0 {
		return nil
	}
	out := make([]core.ReviewIssue, 0)
	for i := range verdicts {
		out = append(out, verdicts[i].Issues...)
	}
	return out
}

func loadPlanFileContents(_ any) map[string]string {
	return nil
}

func isFileBasedPlan(_ any, planFileContents map[string]string) bool {
	return len(planFileContents) > 0
}
