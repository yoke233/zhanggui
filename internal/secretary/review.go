package secretary

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/user/ai-workflow/internal/core"
)

const (
	DecisionApprove  = "approve"
	DecisionFix      = "fix"
	DecisionEscalate = "escalate"

	defaultReviewMaxRounds = 2
	minFeedbackDetailRunes = 20
)

type ReviewStore interface {
	SaveTaskPlan(plan *core.TaskPlan) error
	SaveReviewRecord(record *core.ReviewRecord) error
	GetReviewRecords(planID string) ([]core.ReviewRecord, error)
}

type Reviewer interface {
	Name() string
	Review(ctx context.Context, input ReviewerInput) (core.ReviewVerdict, error)
}

type Aggregator interface {
	Decide(ctx context.Context, input AggregatorInput) (AggregatorDecision, error)
}

type ReviewerInput struct {
	Plan             *core.TaskPlan
	Round            int
	Conversation     string
	ProjectContext   string
	PlanFileContents map[string]string
}

type AggregatorInput struct {
	Plan     *core.TaskPlan
	Round    int
	Verdicts []core.ReviewVerdict
}

type AggregatorDecision struct {
	Decision     string
	RevisedTasks []core.TaskItem
	Fixes        []core.ProposedFix
	Suggestions  string
	Reason       string
}

type ReviewInput struct {
	Conversation     string
	ProjectContext   string
	PlanFileContents map[string]string
}

type ReviewResult struct {
	Plan     *core.TaskPlan
	Decision string
	Reason   string
	Round    int
}

type FeedbackCategory string

const (
	FeedbackMissingNode    FeedbackCategory = "missing_node"
	FeedbackCycle          FeedbackCategory = "cycle"
	FeedbackSelfDependency FeedbackCategory = "self_dependency"
	FeedbackBadGranularity FeedbackCategory = "bad_granularity"
	FeedbackCoverageGap    FeedbackCategory = "coverage_gap"
	FeedbackOther          FeedbackCategory = "other"
)

var allowedFeedbackCategories = map[FeedbackCategory]struct{}{
	FeedbackMissingNode:    {},
	FeedbackCycle:          {},
	FeedbackSelfDependency: {},
	FeedbackBadGranularity: {},
	FeedbackCoverageGap:    {},
	FeedbackOther:          {},
}

type HumanFeedback struct {
	Category          FeedbackCategory `json:"category"`
	Detail            string           `json:"detail"`
	ExpectedDirection string           `json:"expected_direction,omitempty"`
}

type AIReviewSummary struct {
	Rounds       int      `json:"rounds"`
	LastDecision string   `json:"last_decision"`
	TopIssues    []string `json:"top_issues"`
}

type RegenerationRequest struct {
	PlanID          string          `json:"plan_id"`
	RevisionFrom    int             `json:"revision_from"`
	WaitReason      core.WaitReason `json:"wait_reason"`
	Feedback        HumanFeedback   `json:"feedback"`
	AIReviewSummary AIReviewSummary `json:"ai_review_summary"`
}

type Regenerator interface {
	Regenerate(ctx context.Context, req RegenerationRequest) (*core.TaskPlan, error)
}

type ReviewOrchestrator struct {
	Store       ReviewStore
	Reviewers   []Reviewer
	Aggregator  Aggregator
	MaxRounds   int
	RoleRuntime *ReviewRoleRuntime
}

func (p *ReviewOrchestrator) Run(ctx context.Context, plan *core.TaskPlan, input ReviewInput) (*ReviewResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := p.validateForRun(plan, input.PlanFileContents); err != nil {
		return nil, err
	}

	working := cloneReviewPlan(*plan)
	maxRounds := p.effectiveMaxRounds()
	working.Status = core.PlanReviewing
	working.WaitReason = core.WaitNone
	working.ReviewRound = 0
	if err := p.Store.SaveTaskPlan(&working); err != nil {
		return nil, fmt.Errorf("save plan before review: %w", err)
	}

	for round := 1; round <= maxRounds; round++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		working.ReviewRound = round
		if err := p.Store.SaveTaskPlan(&working); err != nil {
			return nil, fmt.Errorf("save plan round %d: %w", round, err)
		}

		verdicts, err := p.runReviewersParallel(ctx, &working, round, input)
		if err != nil {
			return nil, err
		}
		if err := p.persistReviewerRecords(working.ID, round, verdicts); err != nil {
			return nil, err
		}

		decision, err := p.Aggregator.Decide(ctx, AggregatorInput{
			Plan:     cloneReviewPlanPtr(&working),
			Round:    round,
			Verdicts: cloneVerdicts(verdicts),
		})
		if err != nil {
			return nil, fmt.Errorf("aggregator decide round %d: %w", round, err)
		}
		if err := validateAggregatorDecision(&decision); err != nil {
			return nil, fmt.Errorf("invalid aggregator decision in round %d: %w", round, err)
		}
		if err := p.persistAggregatorRecord(working.ID, round, decision, verdicts); err != nil {
			return nil, err
		}

		switch decision.Decision {
		case DecisionApprove:
			working.Status = core.PlanWaitingHuman
			working.WaitReason = core.WaitFinalApproval
			working.ReviewRound = round
			if err := p.Store.SaveTaskPlan(&working); err != nil {
				return nil, fmt.Errorf("save approved plan: %w", err)
			}
			return &ReviewResult{
				Plan:     cloneReviewPlanPtr(&working),
				Decision: DecisionApprove,
				Round:    round,
			}, nil
		case DecisionEscalate:
			working.Status = core.PlanWaitingHuman
			working.WaitReason = core.WaitFeedbackReq
			working.ReviewRound = round
			if err := p.Store.SaveTaskPlan(&working); err != nil {
				return nil, fmt.Errorf("save escalated plan: %w", err)
			}
			return &ReviewResult{
				Plan:     cloneReviewPlanPtr(&working),
				Decision: DecisionEscalate,
				Reason:   strings.TrimSpace(decision.Reason),
				Round:    round,
			}, nil
		case DecisionFix:
			if round >= maxRounds {
				working.Status = core.PlanWaitingHuman
				working.WaitReason = core.WaitFeedbackReq
				working.ReviewRound = round
				if err := p.Store.SaveTaskPlan(&working); err != nil {
					return nil, fmt.Errorf("save max-rounds plan: %w", err)
				}
				reason := strings.TrimSpace(decision.Reason)
				if reason == "" {
					reason = "max_rounds_exceeded"
				}
				return &ReviewResult{
					Plan:     cloneReviewPlanPtr(&working),
					Decision: DecisionEscalate,
					Reason:   reason,
					Round:    round,
				}, nil
			}
			if !hasPendingFileContents(&working, input.PlanFileContents) {
				if len(decision.RevisedTasks) == 0 {
					return nil, fmt.Errorf("invalid revised tasks in round %d: fix decision requires revised_tasks", round)
				}
				if err := validateStructuredTasks(working.ContractVersion, decision.RevisedTasks); err != nil {
					return nil, fmt.Errorf("invalid revised tasks in round %d: %w", round, err)
				}
				working.Tasks = cloneTaskItems(decision.RevisedTasks)
			}
			working.Status = core.PlanReviewing
			working.WaitReason = core.WaitNone
		}
	}

	return nil, errors.New("review orchestrator reached unreachable state")
}

func (p *ReviewOrchestrator) HandleHumanReject(ctx context.Context, plan *core.TaskPlan, feedback HumanFeedback, regenerator Regenerator) (*core.TaskPlan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p == nil {
		return nil, errors.New("review orchestrator is nil")
	}
	if p.Store == nil {
		return nil, errors.New("review store is required")
	}
	if regenerator == nil {
		return nil, errors.New("regenerator is required")
	}
	if plan == nil {
		return nil, errors.New("task plan is required")
	}

	planID := strings.TrimSpace(plan.ID)
	if planID == "" {
		return nil, errors.New("task plan id is required")
	}
	if plan.Status != core.PlanWaitingHuman {
		return nil, fmt.Errorf("human reject requires waiting_human plan, got %s", plan.Status)
	}
	if plan.WaitReason != core.WaitFinalApproval && plan.WaitReason != core.WaitFeedbackReq {
		return nil, fmt.Errorf("human reject requires final_approval/feedback_required, got %s", plan.WaitReason)
	}
	if err := feedback.Validate(); err != nil {
		return nil, err
	}

	records, err := p.Store.GetReviewRecords(planID)
	if err != nil {
		return nil, fmt.Errorf("load review records: %w", err)
	}
	req := RegenerationRequest{
		PlanID:       planID,
		RevisionFrom: plan.ReviewRound,
		WaitReason:   plan.WaitReason,
		Feedback: HumanFeedback{
			Category:          feedback.Category,
			Detail:            strings.TrimSpace(feedback.Detail),
			ExpectedDirection: strings.TrimSpace(feedback.ExpectedDirection),
		},
		AIReviewSummary: summarizeAIReview(records),
	}
	if req.RevisionFrom <= 0 {
		req.RevisionFrom = req.AIReviewSummary.Rounds
	}

	nextPlan, err := regenerator.Regenerate(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("regenerate task plan: %w", err)
	}
	if nextPlan == nil {
		return nil, errors.New("regenerator returned nil task plan")
	}

	updated := cloneReviewPlan(*nextPlan)
	if strings.TrimSpace(updated.ID) == "" {
		updated.ID = planID
	}
	if strings.TrimSpace(updated.ProjectID) == "" {
		updated.ProjectID = plan.ProjectID
	}
	updated.Status = core.PlanReviewing
	updated.WaitReason = core.WaitNone
	updated.ReviewRound = 0

	if err := p.Store.SaveTaskPlan(&updated); err != nil {
		return nil, fmt.Errorf("save regenerated plan: %w", err)
	}
	return &updated, nil
}

func (f HumanFeedback) Validate() error {
	if _, ok := allowedFeedbackCategories[f.Category]; !ok {
		return fmt.Errorf("invalid feedback category %q", f.Category)
	}

	detail := strings.TrimSpace(f.Detail)
	if utf8.RuneCountInString(detail) < minFeedbackDetailRunes {
		return fmt.Errorf("feedback detail must be at least %d characters", minFeedbackDetailRunes)
	}
	return nil
}

func (p *ReviewOrchestrator) validateForRun(plan *core.TaskPlan, planFileContents map[string]string) error {
	if p == nil {
		return errors.New("review orchestrator is nil")
	}
	if p.Store == nil {
		return errors.New("review store is required")
	}
	if plan == nil {
		return errors.New("task plan is required")
	}
	if strings.TrimSpace(plan.ID) == "" {
		return errors.New("task plan id is required")
	}
	if len(p.Reviewers) != 3 {
		return fmt.Errorf("review orchestrator requires exactly 3 reviewers, got %d", len(p.Reviewers))
	}
	for i, reviewer := range p.Reviewers {
		if reviewer == nil {
			return fmt.Errorf("reviewer %d is nil", i+1)
		}
		if strings.TrimSpace(reviewer.Name()) == "" {
			return fmt.Errorf("reviewer %d has empty name", i+1)
		}
	}
	if p.Aggregator == nil {
		return errors.New("aggregator is required")
	}
	if !hasPendingFileContents(plan, planFileContents) {
		if err := validateStructuredTasks(plan.ContractVersion, plan.Tasks); err != nil {
			return err
		}
	}
	return nil
}

func validateStructuredTasks(contractVersion string, tasks []core.TaskItem) error {
	if strings.TrimSpace(contractVersion) == "" {
		return nil
	}
	for i, task := range tasks {
		if err := task.Validate(true); err != nil {
			taskID := strings.TrimSpace(task.ID)
			if taskID == "" {
				taskID = fmt.Sprintf("#%d", i+1)
			}
			return fmt.Errorf("task %s: %w", taskID, err)
		}
	}
	return nil
}

func (p *ReviewOrchestrator) effectiveMaxRounds() int {
	if p == nil || p.MaxRounds <= 0 {
		return defaultReviewMaxRounds
	}
	return p.MaxRounds
}

func (p *ReviewOrchestrator) runReviewersParallel(ctx context.Context, plan *core.TaskPlan, round int, input ReviewInput) ([]core.ReviewVerdict, error) {
	type reviewResult struct {
		index   int
		verdict core.ReviewVerdict
		err     error
	}

	ch := make(chan reviewResult, len(p.Reviewers))
	var wg sync.WaitGroup

	for idx, reviewer := range p.Reviewers {
		wg.Add(1)
		go func(i int, rv Reviewer) {
			defer wg.Done()
			verdict, err := rv.Review(ctx, ReviewerInput{
				Plan:             cloneReviewPlanPtr(plan),
				Round:            round,
				Conversation:     input.Conversation,
				ProjectContext:   input.ProjectContext,
				PlanFileContents: cloneStringMap(input.PlanFileContents),
			})
			if err != nil {
				ch <- reviewResult{index: i, err: fmt.Errorf("reviewer %s: %w", rv.Name(), err)}
				return
			}
			if strings.TrimSpace(verdict.Reviewer) == "" {
				verdict.Reviewer = rv.Name()
			}
			verdict.Status = normalizeVerdictStatus(verdict.Status)
			ch <- reviewResult{index: i, verdict: verdict}
		}(idx, reviewer)
	}

	wg.Wait()
	close(ch)

	verdicts := make([]core.ReviewVerdict, len(p.Reviewers))
	var firstErr error
	for item := range ch {
		if item.err != nil && firstErr == nil {
			firstErr = item.err
			continue
		}
		verdicts[item.index] = item.verdict
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return verdicts, nil
}

func (p *ReviewOrchestrator) persistReviewerRecords(planID string, round int, verdicts []core.ReviewVerdict) error {
	for _, verdict := range verdicts {
		score := verdict.Score
		record := &core.ReviewRecord{
			PlanID:   planID,
			Round:    round,
			Reviewer: strings.TrimSpace(verdict.Reviewer),
			Verdict:  normalizeVerdictStatus(verdict.Status),
			Issues:   cloneIssues(verdict.Issues),
			Score:    &score,
		}
		if err := p.Store.SaveReviewRecord(record); err != nil {
			return fmt.Errorf("save reviewer record round %d reviewer %s: %w", round, verdict.Reviewer, err)
		}
	}
	return nil
}

func (p *ReviewOrchestrator) persistAggregatorRecord(planID string, round int, decision AggregatorDecision, verdicts []core.ReviewVerdict) error {
	normalizedDecision := normalizeDecision(decision.Decision)
	record := &core.ReviewRecord{
		PlanID:   planID,
		Round:    round,
		Reviewer: "aggregator",
		Verdict:  normalizedDecision,
		Issues:   collectAllIssues(verdicts),
		Fixes:    cloneFixes(decision.Fixes),
	}
	if normalizedDecision == DecisionFix && len(record.Fixes) == 0 {
		if len(decision.RevisedTasks) > 0 {
			record.Fixes = tasksToFixes(decision.RevisedTasks)
		} else {
			record.Fixes = suggestionsToFixes(decision.Suggestions)
		}
	}
	if err := p.Store.SaveReviewRecord(record); err != nil {
		return fmt.Errorf("save aggregator record round %d: %w", round, err)
	}
	return nil
}

func validateAggregatorDecision(decision *AggregatorDecision) error {
	if decision == nil {
		return errors.New("decision is nil")
	}

	decision.Decision = normalizeDecision(decision.Decision)
	switch decision.Decision {
	case DecisionApprove:
		return nil
	case DecisionFix:
		if len(decision.RevisedTasks) == 0 && len(decision.Fixes) == 0 && strings.TrimSpace(decision.Suggestions) == "" {
			return errors.New("fix decision requires revised_tasks/fixes/suggestions")
		}
		return nil
	case DecisionEscalate:
		if strings.TrimSpace(decision.Reason) == "" {
			return errors.New("escalate decision requires reason")
		}
		return nil
	default:
		return fmt.Errorf("unsupported decision %q, expected one of: %s, %s, %s", decision.Decision, DecisionApprove, DecisionFix, DecisionEscalate)
	}
}

func normalizeVerdictStatus(status string) string {
	value := strings.ToLower(strings.TrimSpace(status))
	switch value {
	case "pass", "issues_found":
		return value
	default:
		return value
	}
}

func collectAllIssues(verdicts []core.ReviewVerdict) []core.ReviewIssue {
	out := make([]core.ReviewIssue, 0, len(verdicts))
	for _, verdict := range verdicts {
		out = append(out, cloneIssues(verdict.Issues)...)
	}
	return out
}

func tasksToFixes(tasks []core.TaskItem) []core.ProposedFix {
	if len(tasks) == 0 {
		return nil
	}
	out := make([]core.ProposedFix, 0, len(tasks))
	for _, task := range tasks {
		description := strings.TrimSpace(task.Title)
		if description == "" {
			description = "replace task"
		}
		out = append(out, core.ProposedFix{
			TaskID:      strings.TrimSpace(task.ID),
			Description: description,
			Suggestion:  strings.TrimSpace(task.Description),
		})
	}
	return out
}

func suggestionsToFixes(suggestions string) []core.ProposedFix {
	trimmed := strings.TrimSpace(suggestions)
	if trimmed == "" {
		return nil
	}
	return []core.ProposedFix{
		{
			Description: "review suggestions",
			Suggestion:  trimmed,
		},
	}
}

func summarizeAIReview(records []core.ReviewRecord) AIReviewSummary {
	summary := AIReviewSummary{
		TopIssues: []string{},
	}
	if len(records) == 0 {
		return summary
	}

	maxRound := 0
	lastDecision := ""
	issueCounts := map[string]int{}

	for _, record := range records {
		if record.Round > maxRound {
			maxRound = record.Round
		}
		if strings.EqualFold(strings.TrimSpace(record.Reviewer), "aggregator") {
			lastDecision = normalizeDecision(record.Verdict)
		}
		for _, issue := range record.Issues {
			key := strings.TrimSpace(issue.Description)
			if key == "" {
				key = strings.TrimSpace(issue.Severity)
			}
			if key == "" {
				continue
			}
			issueCounts[key]++
		}
	}

	if lastDecision == "" {
		lastDecision = normalizeDecision(records[len(records)-1].Verdict)
	}

	type issueStat struct {
		issue string
		count int
	}
	stats := make([]issueStat, 0, len(issueCounts))
	for issue, count := range issueCounts {
		stats = append(stats, issueStat{issue: issue, count: count})
	}
	slices.SortFunc(stats, func(a, b issueStat) int {
		if a.count == b.count {
			return strings.Compare(a.issue, b.issue)
		}
		if a.count > b.count {
			return -1
		}
		return 1
	})

	topIssues := make([]string, 0, min(3, len(stats)))
	for i := 0; i < len(stats) && i < 3; i++ {
		topIssues = append(topIssues, stats[i].issue)
	}

	summary.Rounds = maxRound
	summary.LastDecision = lastDecision
	summary.TopIssues = topIssues
	return summary
}

func normalizeDecision(decision string) string {
	value := strings.ToLower(strings.TrimSpace(decision))
	switch value {
	case DecisionApprove, DecisionFix, DecisionEscalate:
		return value
	default:
		return value
	}
}

func cloneVerdicts(in []core.ReviewVerdict) []core.ReviewVerdict {
	if len(in) == 0 {
		return nil
	}
	out := make([]core.ReviewVerdict, len(in))
	for i, verdict := range in {
		out[i] = core.ReviewVerdict{
			Reviewer: verdict.Reviewer,
			Status:   verdict.Status,
			Issues:   cloneIssues(verdict.Issues),
			Score:    verdict.Score,
		}
	}
	return out
}

func cloneIssues(in []core.ReviewIssue) []core.ReviewIssue {
	if len(in) == 0 {
		return nil
	}
	out := make([]core.ReviewIssue, len(in))
	copy(out, in)
	return out
}

func cloneFixes(in []core.ProposedFix) []core.ProposedFix {
	if len(in) == 0 {
		return nil
	}
	out := make([]core.ProposedFix, len(in))
	copy(out, in)
	return out
}

func cloneTaskItems(in []core.TaskItem) []core.TaskItem {
	if len(in) == 0 {
		return nil
	}
	out := make([]core.TaskItem, len(in))
	for i, item := range in {
		out[i] = item
		out[i].Labels = append([]string(nil), item.Labels...)
		out[i].DependsOn = append([]string(nil), item.DependsOn...)
	}
	return out
}

func cloneReviewPlan(plan core.TaskPlan) core.TaskPlan {
	cp := plan
	cp.Tasks = cloneTaskItems(plan.Tasks)
	copyPlanOptionalFileFields(&cp, &plan)
	return cp
}

func cloneReviewPlanPtr(plan *core.TaskPlan) *core.TaskPlan {
	if plan == nil {
		return nil
	}
	cp := cloneReviewPlan(*plan)
	return &cp
}
