package secretary

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/user/ai-workflow/internal/acpclient"
	"github.com/user/ai-workflow/internal/core"
)

func TestReviewOrchestratorRunApprovePath(t *testing.T) {
	t.Parallel()

	store := newMockReviewStore()
	panel := ReviewOrchestrator{
		Store: store,
		Reviewers: []Reviewer{
			newStubReviewer("completeness", passVerdict("completeness")),
			newStubReviewer("dependency", passVerdict("dependency")),
			newStubReviewer("feasibility", passVerdict("feasibility")),
		},
		Aggregator: newStubAggregator(func(_ context.Context, input AggregatorInput) (AggregatorDecision, error) {
			if input.Round != 1 {
				return AggregatorDecision{}, errors.New("unexpected round")
			}
			if len(input.Verdicts) != 3 {
				return AggregatorDecision{}, errors.New("aggregator should receive 3 verdicts")
			}
			return AggregatorDecision{Decision: DecisionApprove}, nil
		}),
	}

	result, err := panel.Run(context.Background(), newReviewTestPlan("plan-review-approve"), ReviewInput{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Decision != DecisionApprove {
		t.Fatalf("decision = %q, want %q", result.Decision, DecisionApprove)
	}
	if result.Plan.Status != core.PlanWaitingHuman {
		t.Fatalf("plan status = %q, want %q", result.Plan.Status, core.PlanWaitingHuman)
	}
	if result.Plan.WaitReason != core.WaitFinalApproval {
		t.Fatalf("wait reason = %q, want %q", result.Plan.WaitReason, core.WaitFinalApproval)
	}
	if result.Plan.ReviewRound != 1 {
		t.Fatalf("review round = %d, want 1", result.Plan.ReviewRound)
	}

	records, err := store.GetReviewRecords(result.Plan.ID)
	if err != nil {
		t.Fatalf("GetReviewRecords() error = %v", err)
	}
	if len(records) != 4 {
		t.Fatalf("review record count = %d, want 4", len(records))
	}
	if got := collectReviewers(records); !slices.Equal(got, []string{"aggregator", "completeness", "dependency", "feasibility"}) {
		t.Fatalf("reviewers = %v, want [aggregator completeness dependency feasibility]", got)
	}
}

func TestReviewOrchestratorRunFixThenApprovePath(t *testing.T) {
	t.Parallel()

	store := newMockReviewStore()
	var sawRevisedTask atomic.Bool
	panel := ReviewOrchestrator{
		Store: store,
		Reviewers: []Reviewer{
			newStubReviewer("completeness", func(_ context.Context, input ReviewerInput) (core.ReviewVerdict, error) {
				if input.Round == 2 && len(input.Plan.Tasks) > 0 && input.Plan.Tasks[0].Title == "修正后任务" {
					sawRevisedTask.Store(true)
				}
				return reviewerVerdict("completeness", input.Round), nil
			}),
			newStubReviewer("dependency", func(_ context.Context, input ReviewerInput) (core.ReviewVerdict, error) {
				return reviewerVerdict("dependency", input.Round), nil
			}),
			newStubReviewer("feasibility", func(_ context.Context, input ReviewerInput) (core.ReviewVerdict, error) {
				return reviewerVerdict("feasibility", input.Round), nil
			}),
		},
		Aggregator: newStubAggregator(func(_ context.Context, input AggregatorInput) (AggregatorDecision, error) {
			switch input.Round {
			case 1:
				return AggregatorDecision{
					Decision: DecisionFix,
					RevisedTasks: []core.TaskItem{
						{
							ID:          "task-1",
							Title:       "修正后任务",
							Description: "补齐依赖并增加验收步骤",
							Template:    "standard",
							Status:      core.ItemPending,
						},
					},
					Reason: "补充缺失测试任务",
				}, nil
			case 2:
				return AggregatorDecision{Decision: DecisionApprove}, nil
			default:
				return AggregatorDecision{}, errors.New("unexpected round")
			}
		}),
	}

	result, err := panel.Run(context.Background(), newReviewTestPlan("plan-review-fix"), ReviewInput{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Decision != DecisionApprove {
		t.Fatalf("decision = %q, want %q", result.Decision, DecisionApprove)
	}
	if result.Round != 2 {
		t.Fatalf("round = %d, want 2", result.Round)
	}
	if result.Plan.WaitReason != core.WaitFinalApproval {
		t.Fatalf("wait reason = %q, want %q", result.Plan.WaitReason, core.WaitFinalApproval)
	}
	if !sawRevisedTask.Load() {
		t.Fatal("second review round should receive revised tasks from fix decision")
	}

	records, err := store.GetReviewRecords(result.Plan.ID)
	if err != nil {
		t.Fatalf("GetReviewRecords() error = %v", err)
	}
	if len(records) != 8 {
		t.Fatalf("review record count = %d, want 8", len(records))
	}
}

func TestReviewOrchestratorRunFixFileBasedKeepsTasksAndRecordsSuggestions(t *testing.T) {
	t.Parallel()

	store := newMockReviewStore()
	var sawPlanFileContents atomic.Bool

	panel := ReviewOrchestrator{
		Store: store,
		Reviewers: []Reviewer{
			newStubReviewer("completeness", func(_ context.Context, input ReviewerInput) (core.ReviewVerdict, error) {
				if input.Round == 1 {
					if strings.TrimSpace(input.PlanFileContents["docs/spec.md"]) != "" {
						sawPlanFileContents.Store(true)
					}
				}
				if input.Round == 2 && len(input.Plan.Tasks) != 0 {
					t.Fatalf("file-based fix should keep tasks unchanged, got %d tasks", len(input.Plan.Tasks))
				}
				return reviewerVerdict("completeness", input.Round), nil
			}),
			newStubReviewer("dependency", func(_ context.Context, input ReviewerInput) (core.ReviewVerdict, error) {
				return reviewerVerdict("dependency", input.Round), nil
			}),
			newStubReviewer("feasibility", func(_ context.Context, input ReviewerInput) (core.ReviewVerdict, error) {
				return reviewerVerdict("feasibility", input.Round), nil
			}),
		},
		Aggregator: newStubAggregator(func(_ context.Context, input AggregatorInput) (AggregatorDecision, error) {
			switch input.Round {
			case 1:
				return AggregatorDecision{
					Decision:    DecisionFix,
					Suggestions: "建议先按文件内容抽取模块边界，再进行任务拆分",
				}, nil
			case 2:
				return AggregatorDecision{
					Decision: DecisionApprove,
				}, nil
			default:
				return AggregatorDecision{}, errors.New("unexpected round")
			}
		}),
	}

	plan := &core.TaskPlan{
		ID:         "plan-review-file-fix",
		ProjectID:  "proj-review",
		Name:       "review-file-plan",
		Status:     core.PlanDraft,
		WaitReason: core.WaitNone,
		FailPolicy: core.FailBlock,
		Tasks:      nil,
	}

	result, err := panel.Run(context.Background(), plan, ReviewInput{
		PlanFileContents: map[string]string{
			"docs/spec.md": "Feature spec from files.",
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Decision != DecisionApprove {
		t.Fatalf("decision = %q, want %q", result.Decision, DecisionApprove)
	}
	if result.Round != 2 {
		t.Fatalf("round = %d, want 2", result.Round)
	}
	if len(result.Plan.Tasks) != 0 {
		t.Fatalf("result plan tasks = %d, want 0 for file-based flow", len(result.Plan.Tasks))
	}
	if !sawPlanFileContents.Load() {
		t.Fatal("reviewer should receive plan file contents")
	}

	records, err := store.GetReviewRecords(result.Plan.ID)
	if err != nil {
		t.Fatalf("GetReviewRecords() error = %v", err)
	}
	foundSuggestion := false
	for _, record := range records {
		if record.Reviewer != "aggregator" || record.Round != 1 {
			continue
		}
		for _, fix := range record.Fixes {
			if strings.Contains(fix.Suggestion, "模块边界") {
				foundSuggestion = true
				break
			}
		}
	}
	if !foundSuggestion {
		t.Fatal("round1 aggregator fixes should include suggestion for file-based fix flow")
	}
}

func TestReviewOrchestratorRunEscalatePath(t *testing.T) {
	t.Parallel()

	store := newMockReviewStore()
	panel := ReviewOrchestrator{
		Store: store,
		Reviewers: []Reviewer{
			newStubReviewer("completeness", issueVerdict("completeness", "critical", "缺失核心任务")),
			newStubReviewer("dependency", issueVerdict("dependency", "critical", "存在依赖环")),
			newStubReviewer("feasibility", issueVerdict("feasibility", "warning", "任务过大难以执行")),
		},
		Aggregator: newStubAggregator(func(_ context.Context, _ AggregatorInput) (AggregatorDecision, error) {
			return AggregatorDecision{
				Decision: DecisionEscalate,
				Reason:   "critical 问题无法安全自动修复",
			}, nil
		}),
	}

	result, err := panel.Run(context.Background(), newReviewTestPlan("plan-review-escalate"), ReviewInput{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Decision != DecisionEscalate {
		t.Fatalf("decision = %q, want %q", result.Decision, DecisionEscalate)
	}
	if result.Plan.Status != core.PlanWaitingHuman {
		t.Fatalf("plan status = %q, want %q", result.Plan.Status, core.PlanWaitingHuman)
	}
	if result.Plan.WaitReason != core.WaitFeedbackReq {
		t.Fatalf("wait reason = %q, want %q", result.Plan.WaitReason, core.WaitFeedbackReq)
	}

	records, err := store.GetReviewRecords(result.Plan.ID)
	if err != nil {
		t.Fatalf("GetReviewRecords() error = %v", err)
	}
	if len(records) != 4 {
		t.Fatalf("review record count = %d, want 4", len(records))
	}
}

func TestReviewOrchestratorRunMaxRoundsExceeded(t *testing.T) {
	t.Parallel()

	store := newMockReviewStore()
	panel := ReviewOrchestrator{
		Store: store,
		Reviewers: []Reviewer{
			newStubReviewer("completeness", issueVerdict("completeness", "warning", "覆盖不足")),
			newStubReviewer("dependency", issueVerdict("dependency", "warning", "依赖可优化")),
			newStubReviewer("feasibility", issueVerdict("feasibility", "warning", "任务过粗")),
		},
		Aggregator: newStubAggregator(func(_ context.Context, input AggregatorInput) (AggregatorDecision, error) {
			return AggregatorDecision{
				Decision: DecisionFix,
				RevisedTasks: []core.TaskItem{
					{
						ID:          "task-1",
						Title:       "第2轮仍需修正",
						Description: "补充回归测试和依赖调整",
						Template:    "standard",
						Status:      core.ItemPending,
					},
				},
				Reason: "继续修正",
			}, nil
		}),
		MaxRounds: 2,
	}

	result, err := panel.Run(context.Background(), newReviewTestPlan("plan-review-max-rounds"), ReviewInput{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Decision != DecisionEscalate {
		t.Fatalf("decision = %q, want %q when max rounds reached", result.Decision, DecisionEscalate)
	}
	if result.Plan.ReviewRound != 2 {
		t.Fatalf("review round = %d, want 2", result.Plan.ReviewRound)
	}
	if result.Plan.WaitReason != core.WaitFeedbackReq {
		t.Fatalf("wait reason = %q, want %q", result.Plan.WaitReason, core.WaitFeedbackReq)
	}
	if inputReason := result.Reason; inputReason == "" {
		t.Fatal("max rounds escalation reason should not be empty")
	}

	records, err := store.GetReviewRecords(result.Plan.ID)
	if err != nil {
		t.Fatalf("GetReviewRecords() error = %v", err)
	}
	if len(records) != 8 {
		t.Fatalf("review record count = %d, want 8", len(records))
	}
}

func TestReviewOrchestratorRunReviewersInParallel(t *testing.T) {
	t.Parallel()

	store := newMockReviewStore()
	started := make(chan string, 3)
	release := make(chan struct{})

	newParallelReviewer := func(name string) Reviewer {
		return newStubReviewer(name, func(ctx context.Context, _ ReviewerInput) (core.ReviewVerdict, error) {
			started <- name
			select {
			case <-release:
			case <-ctx.Done():
				return core.ReviewVerdict{}, ctx.Err()
			}
			return core.ReviewVerdict{
				Reviewer: name,
				Status:   "pass",
				Score:    90,
			}, nil
		})
	}

	panel := ReviewOrchestrator{
		Store: store,
		Reviewers: []Reviewer{
			newParallelReviewer("completeness"),
			newParallelReviewer("dependency"),
			newParallelReviewer("feasibility"),
		},
		Aggregator: newStubAggregator(func(_ context.Context, _ AggregatorInput) (AggregatorDecision, error) {
			return AggregatorDecision{Decision: DecisionApprove}, nil
		}),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := panel.Run(ctx, newReviewTestPlan("plan-review-parallel"), ReviewInput{})
		done <- err
	}()

	timeout := time.After(1 * time.Second)
	for i := 0; i < 3; i++ {
		select {
		case <-started:
		case <-timeout:
			t.Fatal("reviewers were not started in parallel")
		}
	}
	close(release)

	if err := <-done; err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestReviewOrchestratorRunApproveDecisionWithWhitespace(t *testing.T) {
	t.Parallel()

	store := newMockReviewStore()
	panel := ReviewOrchestrator{
		Store: store,
		Reviewers: []Reviewer{
			newStubReviewer("completeness", passVerdict("completeness")),
			newStubReviewer("dependency", passVerdict("dependency")),
			newStubReviewer("feasibility", passVerdict("feasibility")),
		},
		Aggregator: newStubAggregator(func(_ context.Context, _ AggregatorInput) (AggregatorDecision, error) {
			return AggregatorDecision{
				Decision: " \n\tapprove \r\n ",
			}, nil
		}),
	}

	result, err := panel.Run(context.Background(), newReviewTestPlan("plan-review-approve-ws"), ReviewInput{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Decision != DecisionApprove {
		t.Fatalf("decision = %q, want %q", result.Decision, DecisionApprove)
	}

	records, err := store.GetReviewRecords(result.Plan.ID)
	if err != nil {
		t.Fatalf("GetReviewRecords() error = %v", err)
	}

	var aggregatorVerdict string
	for _, record := range records {
		if record.Reviewer == "aggregator" {
			aggregatorVerdict = record.Verdict
			break
		}
	}
	if aggregatorVerdict != DecisionApprove {
		t.Fatalf("aggregator verdict = %q, want normalized %q", aggregatorVerdict, DecisionApprove)
	}
}

func TestReviewOrchestratorRunFixDecisionWithWhitespace(t *testing.T) {
	t.Parallel()

	store := newMockReviewStore()
	var sawRevisedTask atomic.Bool
	panel := ReviewOrchestrator{
		Store: store,
		Reviewers: []Reviewer{
			newStubReviewer("completeness", func(_ context.Context, input ReviewerInput) (core.ReviewVerdict, error) {
				if input.Round == 2 && len(input.Plan.Tasks) > 0 && input.Plan.Tasks[0].Title == "修正后任务" {
					sawRevisedTask.Store(true)
				}
				return reviewerVerdict("completeness", input.Round), nil
			}),
			newStubReviewer("dependency", func(_ context.Context, input ReviewerInput) (core.ReviewVerdict, error) {
				return reviewerVerdict("dependency", input.Round), nil
			}),
			newStubReviewer("feasibility", func(_ context.Context, input ReviewerInput) (core.ReviewVerdict, error) {
				return reviewerVerdict("feasibility", input.Round), nil
			}),
		},
		Aggregator: newStubAggregator(func(_ context.Context, input AggregatorInput) (AggregatorDecision, error) {
			switch input.Round {
			case 1:
				return AggregatorDecision{
					Decision: " \nfix\t ",
					RevisedTasks: []core.TaskItem{
						{
							ID:          "task-1",
							Title:       "修正后任务",
							Description: "补齐依赖并增加验收步骤",
							Template:    "standard",
							Status:      core.ItemPending,
						},
					},
				}, nil
			case 2:
				return AggregatorDecision{
					Decision: "\napprove \t",
				}, nil
			default:
				return AggregatorDecision{}, errors.New("unexpected round")
			}
		}),
	}

	result, err := panel.Run(context.Background(), newReviewTestPlan("plan-review-fix-ws"), ReviewInput{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Decision != DecisionApprove {
		t.Fatalf("decision = %q, want %q", result.Decision, DecisionApprove)
	}
	if !sawRevisedTask.Load() {
		t.Fatal("second review round should receive revised tasks from fix decision with whitespace")
	}

	records, err := store.GetReviewRecords(result.Plan.ID)
	if err != nil {
		t.Fatalf("GetReviewRecords() error = %v", err)
	}

	aggregatorByRound := map[int]string{}
	for _, record := range records {
		if record.Reviewer == "aggregator" {
			aggregatorByRound[record.Round] = record.Verdict
		}
	}
	if aggregatorByRound[1] != DecisionFix {
		t.Fatalf("round1 aggregator verdict = %q, want normalized %q", aggregatorByRound[1], DecisionFix)
	}
	if aggregatorByRound[2] != DecisionApprove {
		t.Fatalf("round2 aggregator verdict = %q, want normalized %q", aggregatorByRound[2], DecisionApprove)
	}
}

func TestReviewOrchestratorRunEscalateDecisionWithWhitespace(t *testing.T) {
	t.Parallel()

	store := newMockReviewStore()
	panel := ReviewOrchestrator{
		Store: store,
		Reviewers: []Reviewer{
			newStubReviewer("completeness", issueVerdict("completeness", "critical", "缺失核心任务")),
			newStubReviewer("dependency", issueVerdict("dependency", "critical", "存在依赖环")),
			newStubReviewer("feasibility", issueVerdict("feasibility", "warning", "任务过大难以执行")),
		},
		Aggregator: newStubAggregator(func(_ context.Context, _ AggregatorInput) (AggregatorDecision, error) {
			return AggregatorDecision{
				Decision: " \n escalate\t ",
				Reason:   "critical 问题无法安全自动修复",
			}, nil
		}),
	}

	result, err := panel.Run(context.Background(), newReviewTestPlan("plan-review-escalate-ws"), ReviewInput{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Decision != DecisionEscalate {
		t.Fatalf("decision = %q, want %q", result.Decision, DecisionEscalate)
	}

	records, err := store.GetReviewRecords(result.Plan.ID)
	if err != nil {
		t.Fatalf("GetReviewRecords() error = %v", err)
	}

	var aggregatorVerdict string
	for _, record := range records {
		if record.Reviewer == "aggregator" {
			aggregatorVerdict = record.Verdict
			break
		}
	}
	if aggregatorVerdict != DecisionEscalate {
		t.Fatalf("aggregator verdict = %q, want normalized %q", aggregatorVerdict, DecisionEscalate)
	}
}

func TestReviewOrchestratorRunUnknownDecisionReturnsClearError(t *testing.T) {
	t.Parallel()

	store := newMockReviewStore()
	panel := ReviewOrchestrator{
		Store: store,
		Reviewers: []Reviewer{
			newStubReviewer("completeness", passVerdict("completeness")),
			newStubReviewer("dependency", passVerdict("dependency")),
			newStubReviewer("feasibility", passVerdict("feasibility")),
		},
		Aggregator: newStubAggregator(func(_ context.Context, _ AggregatorInput) (AggregatorDecision, error) {
			return AggregatorDecision{Decision: " \nhold \t"}, nil
		}),
	}

	_, err := panel.Run(context.Background(), newReviewTestPlan("plan-review-unknown-decision"), ReviewInput{})
	if err == nil {
		t.Fatal("Run() expected error for unknown decision, got nil")
	}
	if !strings.Contains(err.Error(), `invalid aggregator decision in round 1`) {
		t.Fatalf("error = %v, want invalid aggregator decision context", err)
	}
	if !strings.Contains(err.Error(), `unsupported decision "hold"`) {
		t.Fatalf("error = %v, want normalized unsupported decision detail", err)
	}
}

func TestReviewOrchestratorUsesRoleBindings(t *testing.T) {
	t.Parallel()

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
				ID:      "reviewer",
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
				ID:      "aggregator",
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

	runtime, err := ResolveReviewOrchestratorRoles(ReviewRoleBindingInput{
		Reviewers: map[string]string{
			"completeness": "reviewer",
			"dependency":   "reviewer",
			"feasibility":  "reviewer",
		},
		Aggregator: "aggregator",
	}, resolver)
	if err != nil {
		t.Fatalf("ResolveReviewOrchestratorRoles() error = %v", err)
	}
	if runtime.AggregatorRole != "aggregator" {
		t.Fatalf("aggregator role = %q, want %q", runtime.AggregatorRole, "aggregator")
	}
	for _, reviewer := range []string{"completeness", "dependency", "feasibility"} {
		if got := runtime.ReviewerRoles[reviewer]; got != "reviewer" {
			t.Fatalf("reviewer role %s = %q, want %q", reviewer, got, "reviewer")
		}
		policy := runtime.ReviewerSessionPolicies[reviewer]
		if !policy.Reuse {
			t.Fatalf("reviewer %s reuse should default true", reviewer)
		}
		if !policy.ResetPrompt {
			t.Fatalf("reviewer %s reset_prompt should default true", reviewer)
		}
	}
	if !runtime.AggregatorSessionPolicy.Reuse {
		t.Fatal("aggregator reuse should default true")
	}
	if !runtime.AggregatorSessionPolicy.ResetPrompt {
		t.Fatal("aggregator reset_prompt should default true")
	}
}

func TestReviewOrchestratorHandleHumanRejectTriggersRegeneration(t *testing.T) {
	t.Parallel()

	store := newMockReviewStore()
	plan := newReviewTestPlan("plan-review-reject")
	plan.Status = core.PlanWaitingHuman
	plan.WaitReason = core.WaitFinalApproval
	plan.ReviewRound = 2

	if err := store.SaveReviewRecord(&core.ReviewRecord{
		PlanID:   plan.ID,
		Round:    1,
		Reviewer: "completeness",
		Verdict:  "issues_found",
		Issues: []core.ReviewIssue{
			{
				Severity:    "warning",
				Description: "coverage_gap",
			},
		},
	}); err != nil {
		t.Fatalf("SaveReviewRecord(round1) error = %v", err)
	}
	if err := store.SaveReviewRecord(&core.ReviewRecord{
		PlanID:   plan.ID,
		Round:    2,
		Reviewer: "aggregator",
		Verdict:  "approve",
	}); err != nil {
		t.Fatalf("SaveReviewRecord(round2) error = %v", err)
	}

	spy := &spyRegenerator{
		result: &core.TaskPlan{
			ID:        plan.ID,
			ProjectID: plan.ProjectID,
			Name:      "regenerated-plan",
			Tasks: []core.TaskItem{
				{
					ID:          "task-r1",
					Title:       "补全遗漏任务",
					Description: "根据人工反馈重建任务清单",
					Template:    "standard",
					Status:      core.ItemPending,
				},
			},
		},
	}

	panel := ReviewOrchestrator{Store: store}
	nextPlan, err := panel.HandleHumanReject(context.Background(), plan, HumanFeedback{
		Category:          FeedbackCoverageGap,
		Detail:            "上一版遗漏验收与回归测试任务，请补齐并明确依赖关系。",
		ExpectedDirection: "补齐测试任务并修正依赖",
	}, spy)
	if err != nil {
		t.Fatalf("HandleHumanReject() error = %v", err)
	}

	if !spy.called {
		t.Fatal("regenerator should be called")
	}
	if spy.request.PlanID != plan.ID {
		t.Fatalf("regeneration plan_id = %q, want %q", spy.request.PlanID, plan.ID)
	}
	if spy.request.RevisionFrom != 2 {
		t.Fatalf("revision_from = %d, want 2", spy.request.RevisionFrom)
	}
	if spy.request.WaitReason != core.WaitFinalApproval {
		t.Fatalf("wait_reason = %q, want %q", spy.request.WaitReason, core.WaitFinalApproval)
	}
	if spy.request.AIReviewSummary.Rounds != 2 {
		t.Fatalf("ai_review_summary.rounds = %d, want 2", spy.request.AIReviewSummary.Rounds)
	}
	if spy.request.AIReviewSummary.LastDecision != DecisionApprove {
		t.Fatalf("ai_review_summary.last_decision = %q, want %q", spy.request.AIReviewSummary.LastDecision, DecisionApprove)
	}
	if len(spy.request.AIReviewSummary.TopIssues) == 0 || spy.request.AIReviewSummary.TopIssues[0] != "coverage_gap" {
		t.Fatalf("ai_review_summary.top_issues = %v, want first issue coverage_gap", spy.request.AIReviewSummary.TopIssues)
	}

	if nextPlan.Status != core.PlanReviewing {
		t.Fatalf("next plan status = %q, want %q", nextPlan.Status, core.PlanReviewing)
	}
	if nextPlan.WaitReason != core.WaitNone {
		t.Fatalf("next plan wait reason = %q, want empty", nextPlan.WaitReason)
	}
	if nextPlan.ReviewRound != 0 {
		t.Fatalf("next plan review_round = %d, want 0", nextPlan.ReviewRound)
	}
}

func TestReviewOrchestratorHandleHumanRejectFeedbackRequiredTriggersRegeneration(t *testing.T) {
	t.Parallel()

	store := newMockReviewStore()
	plan := newReviewTestPlan("plan-review-reject-feedback-required")
	plan.Status = core.PlanWaitingHuman
	plan.WaitReason = core.WaitFeedbackReq
	plan.ReviewRound = 1

	if err := store.SaveReviewRecord(&core.ReviewRecord{
		PlanID:   plan.ID,
		Round:    1,
		Reviewer: "aggregator",
		Verdict:  " \nESCALATE\t ",
		Issues: []core.ReviewIssue{
			{
				Severity:    "critical",
				Description: "coverage_gap",
			},
		},
	}); err != nil {
		t.Fatalf("SaveReviewRecord(round1) error = %v", err)
	}

	spy := &spyRegenerator{
		result: &core.TaskPlan{
			ID:        plan.ID,
			ProjectID: plan.ProjectID,
			Name:      "regenerated-plan-feedback-required",
			Tasks: []core.TaskItem{
				{
					ID:          "task-r1",
					Title:       "补全遗漏任务",
					Description: "根据人工反馈重建任务清单",
					Template:    "standard",
					Status:      core.ItemPending,
				},
			},
		},
	}

	panel := ReviewOrchestrator{Store: store}
	nextPlan, err := panel.HandleHumanReject(context.Background(), plan, HumanFeedback{
		Category:          FeedbackCoverageGap,
		Detail:            "该版本在反馈要求场景中仍遗漏关键验证步骤，请补齐并收敛风险。",
		ExpectedDirection: "聚焦遗漏验证步骤并补全任务",
	}, spy)
	if err != nil {
		t.Fatalf("HandleHumanReject() error = %v", err)
	}

	if !spy.called {
		t.Fatal("regenerator should be called")
	}
	if spy.request.PlanID != plan.ID {
		t.Fatalf("regeneration plan_id = %q, want %q", spy.request.PlanID, plan.ID)
	}
	if spy.request.RevisionFrom != 1 {
		t.Fatalf("revision_from = %d, want 1", spy.request.RevisionFrom)
	}
	if spy.request.WaitReason != core.WaitFeedbackReq {
		t.Fatalf("wait_reason = %q, want %q", spy.request.WaitReason, core.WaitFeedbackReq)
	}
	if spy.request.AIReviewSummary.LastDecision != DecisionEscalate {
		t.Fatalf("ai_review_summary.last_decision = %q, want %q", spy.request.AIReviewSummary.LastDecision, DecisionEscalate)
	}
	if nextPlan.Status != core.PlanReviewing {
		t.Fatalf("next plan status = %q, want %q", nextPlan.Status, core.PlanReviewing)
	}
	if nextPlan.WaitReason != core.WaitNone {
		t.Fatalf("next plan wait reason = %q, want empty", nextPlan.WaitReason)
	}
	if nextPlan.ReviewRound != 0 {
		t.Fatalf("next plan review_round = %d, want 0", nextPlan.ReviewRound)
	}
}

type mockReviewStore struct {
	mu      sync.Mutex
	records []core.ReviewRecord
	plans   []core.TaskPlan
}

func newMockReviewStore() *mockReviewStore {
	return &mockReviewStore{}
}

func (s *mockReviewStore) SaveTaskPlan(plan *core.TaskPlan) error {
	if plan == nil {
		return errors.New("plan is nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.plans = append(s.plans, cloneReviewPlan(*plan))
	return nil
}

func (s *mockReviewStore) SaveReviewRecord(r *core.ReviewRecord) error {
	if r == nil {
		return errors.New("review record is nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	record := *r
	record.Issues = append([]core.ReviewIssue(nil), r.Issues...)
	record.Fixes = append([]core.ProposedFix(nil), r.Fixes...)
	s.records = append(s.records, record)
	return nil
}

func (s *mockReviewStore) GetReviewRecords(planID string) ([]core.ReviewRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var filtered []core.ReviewRecord
	for _, record := range s.records {
		if record.PlanID != planID {
			continue
		}
		cp := record
		cp.Issues = append([]core.ReviewIssue(nil), record.Issues...)
		cp.Fixes = append([]core.ProposedFix(nil), record.Fixes...)
		filtered = append(filtered, cp)
	}
	return filtered, nil
}

type stubReviewer struct {
	name string
	fn   func(ctx context.Context, input ReviewerInput) (core.ReviewVerdict, error)
}

func newStubReviewer(name string, fn func(ctx context.Context, input ReviewerInput) (core.ReviewVerdict, error)) stubReviewer {
	return stubReviewer{name: name, fn: fn}
}

func (r stubReviewer) Name() string { return r.name }

func (r stubReviewer) Review(ctx context.Context, input ReviewerInput) (core.ReviewVerdict, error) {
	if r.fn != nil {
		return r.fn(ctx, input)
	}
	return core.ReviewVerdict{
		Reviewer: r.name,
		Status:   "pass",
		Score:    80,
	}, nil
}

type stubAggregator struct {
	fn func(ctx context.Context, input AggregatorInput) (AggregatorDecision, error)
}

func newStubAggregator(fn func(ctx context.Context, input AggregatorInput) (AggregatorDecision, error)) stubAggregator {
	return stubAggregator{fn: fn}
}

func (a stubAggregator) Decide(ctx context.Context, input AggregatorInput) (AggregatorDecision, error) {
	if a.fn != nil {
		return a.fn(ctx, input)
	}
	return AggregatorDecision{Decision: DecisionApprove}, nil
}

type spyRegenerator struct {
	mu      sync.Mutex
	called  bool
	request RegenerationRequest
	result  *core.TaskPlan
	err     error
}

func (s *spyRegenerator) Regenerate(_ context.Context, req RegenerationRequest) (*core.TaskPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.called = true
	s.request = req
	if s.err != nil {
		return nil, s.err
	}
	if s.result == nil {
		return nil, nil
	}
	cp := cloneReviewPlan(*s.result)
	return &cp, nil
}

func TestReviewAgent_RejectsMissingAcceptance(t *testing.T) {
	t.Parallel()

	store := newMockReviewStore()
	panel := ReviewOrchestrator{
		Store: store,
		Reviewers: []Reviewer{
			newStubReviewer("completeness", passVerdict("completeness")),
			newStubReviewer("dependency", passVerdict("dependency")),
			newStubReviewer("feasibility", passVerdict("feasibility")),
		},
		Aggregator: newStubAggregator(func(_ context.Context, _ AggregatorInput) (AggregatorDecision, error) {
			return AggregatorDecision{Decision: DecisionApprove}, nil
		}),
	}

	plan := newReviewTestPlan("plan-review-structured-missing-acceptance")
	plan.ContractVersion = "v1"
	plan.Tasks[0].Acceptance = nil

	_, err := panel.Run(context.Background(), plan, ReviewInput{})
	if err == nil {
		t.Fatal("expected review run to reject missing acceptance under structured contract")
	}
	if !strings.Contains(err.Error(), "acceptance") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func passVerdict(reviewer string) func(context.Context, ReviewerInput) (core.ReviewVerdict, error) {
	return func(_ context.Context, _ ReviewerInput) (core.ReviewVerdict, error) {
		return core.ReviewVerdict{
			Reviewer: reviewer,
			Status:   "pass",
			Score:    92,
		}, nil
	}
}

func issueVerdict(reviewer, severity, description string) func(context.Context, ReviewerInput) (core.ReviewVerdict, error) {
	return func(_ context.Context, _ ReviewerInput) (core.ReviewVerdict, error) {
		return core.ReviewVerdict{
			Reviewer: reviewer,
			Status:   "issues_found",
			Score:    60,
			Issues: []core.ReviewIssue{
				{
					Severity:    severity,
					TaskID:      "task-1",
					Description: description,
					Suggestion:  "请修正",
				},
			},
		}, nil
	}
}

func reviewerVerdict(reviewer string, round int) core.ReviewVerdict {
	if round == 1 {
		return core.ReviewVerdict{
			Reviewer: reviewer,
			Status:   "issues_found",
			Score:    70,
			Issues: []core.ReviewIssue{
				{
					Severity:    "warning",
					TaskID:      "task-1",
					Description: "首轮发现问题",
					Suggestion:  "请补齐",
				},
			},
		}
	}

	return core.ReviewVerdict{
		Reviewer: reviewer,
		Status:   "pass",
		Score:    90,
	}
}

func newReviewTestPlan(planID string) *core.TaskPlan {
	return &core.TaskPlan{
		ID:         planID,
		ProjectID:  "proj-review",
		Name:       "review-plan",
		Status:     core.PlanDraft,
		WaitReason: core.WaitNone,
		FailPolicy: core.FailBlock,
		Tasks: []core.TaskItem{
			{
				ID:          "task-1",
				PlanID:      planID,
				Title:       "实现功能A",
				Description: "完成功能A并补充测试",
				Template:    "standard",
				Status:      core.ItemPending,
			},
		},
	}
}

func collectReviewers(records []core.ReviewRecord) []string {
	set := map[string]struct{}{}
	for _, record := range records {
		set[record.Reviewer] = struct{}{}
	}

	out := make([]string, 0, len(set))
	for reviewer := range set {
		out = append(out, reviewer)
	}
	slices.Sort(out)
	return out
}
