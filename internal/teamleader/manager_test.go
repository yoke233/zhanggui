package teamleader

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/yoke233/ai-workflow/internal/core"
	storesqlite "github.com/yoke233/ai-workflow/internal/plugins/store-sqlite"
)

func TestManager_StartCallsRecoverExecutingIssues(t *testing.T) {
	t.Parallel()

	store := newManagerTestStore(t)
	t.Cleanup(func() { _ = store.Close() })

	scheduler := &fakeManagerScheduler{}
	manager, err := NewManager(store, nil, nil, scheduler)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if !scheduler.startCalled {
		t.Fatal("scheduler.Start should be called")
	}
	if !scheduler.recoverCalled {
		t.Fatal("scheduler.RecoverExecutingIssues should be called")
	}
}

func TestManager_CreateIssuesPersistsDraftWithDefaults(t *testing.T) {
	t.Parallel()

	store := newManagerTestStore(t)
	t.Cleanup(func() { _ = store.Close() })

	project := mustCreateManagerProject(t, store, "proj-manager-create")
	const sessionID = "sess-manager-create"
	if err := store.CreateChatSession(&core.ChatSession{
		ID:        sessionID,
		ProjectID: project.ID,
		Messages:  []core.ChatMessage{},
	}); err != nil {
		t.Fatalf("CreateChatSession() error = %v", err)
	}

	manager, err := NewManager(store, nil, nil, &fakeManagerScheduler{})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	created, err := manager.CreateIssues(context.Background(), CreateIssuesInput{
		ProjectID: project.ID,
		SessionID: sessionID,
		Issues: []CreateIssueSpec{
			{
				ID:        "issue-manager-create",
				Title:     "Fill release regression steps",
				Body:      "Ensure full regression checklist before release.",
				Labels:    []string{"release", "qa"},
				DependsOn: []string{" issue-prep ", "issue-prep", ""},
				Blocks:    []string{"issue-deploy", "issue-deploy"},
				Priority:  3,
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateIssues() error = %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("CreateIssues() returned %d issues, want 1", len(created))
	}

	issue := created[0]
	if issue.Status != core.IssueStatusDraft {
		t.Fatalf("created status = %q, want %q", issue.Status, core.IssueStatusDraft)
	}
	if issue.State != core.IssueStateOpen {
		t.Fatalf("created state = %q, want %q", issue.State, core.IssueStateOpen)
	}
	if issue.Template != "standard" {
		t.Fatalf("created template = %q, want %q", issue.Template, "standard")
	}
	if !issue.AutoMerge {
		t.Fatalf("created auto_merge = %t, want true", issue.AutoMerge)
	}
	if issue.FailPolicy != core.FailBlock {
		t.Fatalf("created fail_policy = %q, want %q", issue.FailPolicy, core.FailBlock)
	}
	if got, want := issue.DependsOn, []string{"issue-prep"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("created depends_on = %#v, want %#v", got, want)
	}
	if got, want := issue.Blocks, []string{"issue-deploy"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("created blocks = %#v, want %#v", got, want)
	}

	persisted, err := store.GetIssue(issue.ID)
	if err != nil {
		t.Fatalf("GetIssue(%s) error = %v", issue.ID, err)
	}
	if persisted.Status != core.IssueStatusDraft {
		t.Fatalf("persisted status = %q, want %q", persisted.Status, core.IssueStatusDraft)
	}
	if !persisted.AutoMerge {
		t.Fatalf("persisted auto_merge = %t, want true", persisted.AutoMerge)
	}
	if persisted.SessionID != sessionID {
		t.Fatalf("persisted session_id = %q, want %q", persisted.SessionID, sessionID)
	}
	if got, want := persisted.DependsOn, []string{"issue-prep"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("persisted depends_on = %#v, want %#v", got, want)
	}
	if got, want := persisted.Blocks, []string{"issue-deploy"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("persisted blocks = %#v, want %#v", got, want)
	}
}

func TestManager_ConfirmCreatedIssuesQueuesAndDispatches(t *testing.T) {
	t.Parallel()

	store := newManagerTestStore(t)
	t.Cleanup(func() { _ = store.Close() })

	project := mustCreateManagerProject(t, store, "proj-manager-confirm-created")
	issue := mustCreateManagerIssue(t, store, project.ID, "issue-confirm-created", core.IssueStatusDraft, core.IssueStateOpen)

	scheduler := &fakeManagerScheduler{}
	manager, err := NewManager(store, nil, nil, scheduler)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	confirmed, err := manager.ConfirmCreatedIssues(context.Background(), []string{issue.ID}, "confirmed from test")
	if err != nil {
		t.Fatalf("ConfirmCreatedIssues() error = %v", err)
	}
	if len(confirmed) != 1 || confirmed[0] == nil {
		t.Fatalf("confirmed issues = %#v", confirmed)
	}
	if confirmed[0].Status != core.IssueStatusQueued {
		t.Fatalf("confirmed status = %q, want %q", confirmed[0].Status, core.IssueStatusQueued)
	}

	persisted, err := store.GetIssue(issue.ID)
	if err != nil {
		t.Fatalf("GetIssue(%s) error = %v", issue.ID, err)
	}
	if persisted.Status != core.IssueStatusQueued {
		t.Fatalf("persisted status = %q, want %q", persisted.Status, core.IssueStatusQueued)
	}
	if scheduler.startIssueCalls != 1 {
		t.Fatalf("scheduler startIssueCalls = %d, want 1", scheduler.startIssueCalls)
	}
	if len(scheduler.startedIssues) != 1 || scheduler.startedIssues[0].ID != issue.ID {
		t.Fatalf("scheduler started issues = %#v, want [%s]", scheduler.startedIssues, issue.ID)
	}
}

func TestManager_CreateIssuesRejectsDuplicateIssueIDs(t *testing.T) {
	t.Parallel()

	store := newManagerTestStore(t)
	t.Cleanup(func() { _ = store.Close() })

	project := mustCreateManagerProject(t, store, "proj-manager-dup")
	manager, err := NewManager(store, nil, nil, &fakeManagerScheduler{})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	_, err = manager.CreateIssues(context.Background(), CreateIssuesInput{
		ProjectID: project.ID,
		Issues: []CreateIssueSpec{
			{ID: "issue-dup", Title: "first"},
			{ID: "issue-dup", Title: "second"},
		},
	})
	if err == nil {
		t.Fatal("CreateIssues() should fail for duplicate issue IDs")
	}
	if !strings.Contains(err.Error(), `duplicate issue id "issue-dup"`) {
		t.Fatalf("error = %v, want duplicate issue id message", err)
	}
}

func TestManager_CreateIssuesRollsBackCreatedIssuesWhenLaterCreateFails(t *testing.T) {
	t.Parallel()

	store := newManagerTestStore(t)
	t.Cleanup(func() { _ = store.Close() })

	project := mustCreateManagerProject(t, store, "proj-manager-create-rollback")
	const sessionID = "sess-manager-create-rollback"
	if err := store.CreateChatSession(&core.ChatSession{
		ID:        sessionID,
		ProjectID: project.ID,
		Messages:  []core.ChatMessage{},
	}); err != nil {
		t.Fatalf("CreateChatSession() error = %v", err)
	}
	if err := store.CreateIssue(&core.Issue{
		ID:         "issue-existing",
		ProjectID:  project.ID,
		SessionID:  sessionID,
		Title:      "existing",
		Template:   "standard",
		State:      core.IssueStateOpen,
		Status:     core.IssueStatusDraft,
		FailPolicy: core.FailBlock,
	}); err != nil {
		t.Fatalf("seed existing issue: %v", err)
	}

	manager, err := NewManager(store, nil, nil, &fakeManagerScheduler{})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	_, err = manager.CreateIssues(context.Background(), CreateIssuesInput{
		ProjectID: project.ID,
		SessionID: sessionID,
		Issues: []CreateIssueSpec{
			{ID: "issue-first", Title: "first"},
			{ID: "issue-existing", Title: "duplicate existing"},
		},
	})
	if err == nil {
		t.Fatal("CreateIssues() should fail when later issue create fails")
	}
	if !strings.Contains(err.Error(), `create issue issue-existing`) {
		t.Fatalf("error = %v, want create issue context", err)
	}

	if _, getErr := store.GetIssue("issue-first"); getErr == nil {
		t.Fatal("issue-first should be rolled back after batch create failure")
	}
	steps, stepsErr := store.ListTaskSteps("issue-first")
	if stepsErr != nil {
		t.Fatalf("ListTaskSteps(issue-first) error = %v", stepsErr)
	}
	if len(steps) != 0 {
		t.Fatalf("ListTaskSteps(issue-first) len = %d, want 0 after rollback", len(steps))
	}

	persisted, getErr := store.GetIssue("issue-existing")
	if getErr != nil {
		t.Fatalf("GetIssue(issue-existing) error = %v", getErr)
	}
	if persisted == nil || persisted.ID != "issue-existing" {
		t.Fatalf("persisted existing issue = %#v", persisted)
	}
}

func TestManager_SubmitForReviewMarksIssueReviewingViaTwoPhase(t *testing.T) {
	t.Parallel()

	store := newManagerTestStore(t)
	t.Cleanup(func() { _ = store.Close() })

	project := mustCreateManagerProject(t, store, "proj-manager-submit-two-phase")
	issue := mustCreateManagerIssue(t, store, project.ID, "issue-submit-two-phase", core.IssueStatusDraft, core.IssueStateOpen)

	review := &fakeManagerTwoPhaseReview{}
	manager, err := NewManager(store, nil, review, &fakeManagerScheduler{})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	err = manager.SubmitForReview(context.Background(), []string{" " + issue.ID + " ", issue.ID, ""})
	if err != nil {
		t.Fatalf("SubmitForReview() error = %v", err)
	}
	if review.submitCalls != 1 {
		t.Fatalf("two-phase SubmitForReview calls = %d, want 1", review.submitCalls)
	}
	if len(review.lastSubmitted) != 1 || review.lastSubmitted[0].ID != issue.ID {
		t.Fatalf("two-phase submitted issues = %#v, want [%s]", review.lastSubmitted, issue.ID)
	}

	updated, err := store.GetIssue(issue.ID)
	if err != nil {
		t.Fatalf("GetIssue(%s) error = %v", issue.ID, err)
	}
	if updated.Status != core.IssueStatusReviewing {
		t.Fatalf("updated status = %q, want %q", updated.Status, core.IssueStatusReviewing)
	}
	if updated.State != core.IssueStateOpen {
		t.Fatalf("updated state = %q, want %q", updated.State, core.IssueStateOpen)
	}

	change := mustLatestIssueChange(t, store, issue.ID)
	if change.Field != "status" {
		t.Fatalf("change field = %q, want %q", change.Field, "status")
	}
	if change.OldValue != string(core.IssueStatusDraft) {
		t.Fatalf("change old_value = %q, want %q", change.OldValue, core.IssueStatusDraft)
	}
	if change.NewValue != string(core.IssueStatusReviewing) {
		t.Fatalf("change new_value = %q, want %q", change.NewValue, core.IssueStatusReviewing)
	}
	if change.Reason != "submit_for_review" {
		t.Fatalf("change reason = %q, want %q", change.Reason, "submit_for_review")
	}
}

func TestManager_SubmitForReviewAutoApprovesWhenAutoMergeEnabledAndRoundPasses(t *testing.T) {
	t.Parallel()

	store := newManagerTestStore(t)
	t.Cleanup(func() { _ = store.Close() })

	project := mustCreateManagerProject(t, store, "proj-manager-auto-approve")
	issue := mustCreateManagerIssue(t, store, project.ID, "issue-auto-approve", core.IssueStatusDraft, core.IssueStateOpen)
	issue.AutoMerge = true
	if err := store.SaveIssue(issue); err != nil {
		t.Fatalf("SaveIssue(%s) error = %v", issue.ID, err)
	}

	review := &fakeManagerTwoPhaseReview{
		submitHook: func(issues []*core.Issue) error {
			for i := range issues {
				if err := store.SaveReviewRecord(&core.ReviewRecord{
					IssueID:   issues[i].ID,
					Round:     1,
					Reviewer:  "auto-reviewer",
					Verdict:   "pass",
					Issues:    nil,
					Fixes:     nil,
					RawOutput: "all checks passed",
				}); err != nil {
					return err
				}
			}
			return nil
		},
	}
	scheduler := &fakeManagerScheduler{}
	manager, err := NewManager(store, nil, review, scheduler)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if err := manager.SubmitForReview(context.Background(), []string{issue.ID}); err != nil {
		t.Fatalf("SubmitForReview() error = %v", err)
	}
	if scheduler.startIssueCalls != 1 {
		t.Fatalf("scheduler StartIssue calls = %d, want 1", scheduler.startIssueCalls)
	}

	updated, err := store.GetIssue(issue.ID)
	if err != nil {
		t.Fatalf("GetIssue(%s) error = %v", issue.ID, err)
	}
	if updated.Status != core.IssueStatusQueued {
		t.Fatalf("updated status = %q, want %q", updated.Status, core.IssueStatusQueued)
	}
}

func TestManager_SubmitForReviewDoesNotAutoApproveWhenAutoMergeDisabled(t *testing.T) {
	t.Parallel()

	store := newManagerTestStore(t)
	t.Cleanup(func() { _ = store.Close() })

	project := mustCreateManagerProject(t, store, "proj-manager-auto-approve-disabled")
	issue := mustCreateManagerIssue(t, store, project.ID, "issue-auto-approve-disabled", core.IssueStatusDraft, core.IssueStateOpen)
	issue.AutoMerge = false
	if err := store.SaveIssue(issue); err != nil {
		t.Fatalf("SaveIssue(%s) error = %v", issue.ID, err)
	}

	review := &fakeManagerTwoPhaseReview{
		submitHook: func(issues []*core.Issue) error {
			for i := range issues {
				if err := store.SaveReviewRecord(&core.ReviewRecord{
					IssueID:  issues[i].ID,
					Round:    1,
					Reviewer: "auto-reviewer",
					Verdict:  "pass",
				}); err != nil {
					return err
				}
			}
			return nil
		},
	}
	scheduler := &fakeManagerScheduler{}
	manager, err := NewManager(store, nil, review, scheduler)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if err := manager.SubmitForReview(context.Background(), []string{issue.ID}); err != nil {
		t.Fatalf("SubmitForReview() error = %v", err)
	}
	if scheduler.startIssueCalls != 0 {
		t.Fatalf("scheduler StartIssue calls = %d, want 0", scheduler.startIssueCalls)
	}

	updated, err := store.GetIssue(issue.ID)
	if err != nil {
		t.Fatalf("GetIssue(%s) error = %v", issue.ID, err)
	}
	if updated.Status != core.IssueStatusReviewing {
		t.Fatalf("updated status = %q, want %q", updated.Status, core.IssueStatusReviewing)
	}
}

func TestManager_SubmitForReviewDoesNotAutoApproveWhenRoundNotPass(t *testing.T) {
	t.Parallel()

	store := newManagerTestStore(t)
	t.Cleanup(func() { _ = store.Close() })

	project := mustCreateManagerProject(t, store, "proj-manager-auto-approve-no-pass")
	issue := mustCreateManagerIssue(t, store, project.ID, "issue-auto-approve-no-pass", core.IssueStatusDraft, core.IssueStateOpen)
	issue.AutoMerge = true
	if err := store.SaveIssue(issue); err != nil {
		t.Fatalf("SaveIssue(%s) error = %v", issue.ID, err)
	}

	review := &fakeManagerTwoPhaseReview{
		submitHook: func(issues []*core.Issue) error {
			for i := range issues {
				if err := store.SaveReviewRecord(&core.ReviewRecord{
					IssueID:  issues[i].ID,
					Round:    1,
					Reviewer: "auto-reviewer",
					Verdict:  "fix",
					Issues: []core.ReviewIssue{
						{
							Severity:    "high",
							IssueID:     "rv-1",
							Description: "need rework",
							Suggestion:  "add missing rollback flow",
						},
					},
				}); err != nil {
					return err
				}
			}
			return nil
		},
	}
	scheduler := &fakeManagerScheduler{}
	manager, err := NewManager(store, nil, review, scheduler)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if err := manager.SubmitForReview(context.Background(), []string{issue.ID}); err != nil {
		t.Fatalf("SubmitForReview() error = %v", err)
	}
	if scheduler.startIssueCalls != 0 {
		t.Fatalf("scheduler StartIssue calls = %d, want 0", scheduler.startIssueCalls)
	}

	updated, err := store.GetIssue(issue.ID)
	if err != nil {
		t.Fatalf("GetIssue(%s) error = %v", issue.ID, err)
	}
	if updated.Status != core.IssueStatusReviewing {
		t.Fatalf("updated status = %q, want %q", updated.Status, core.IssueStatusReviewing)
	}
}

func TestManager_SubmitForReviewWithGateChainApprovesAndPersistsGateData(t *testing.T) {
	t.Parallel()

	store := newManagerTestStore(t)
	t.Cleanup(func() { _ = store.Close() })

	project := mustCreateManagerProject(t, store, "proj-manager-gate-pass")
	issue := mustCreateManagerIssue(t, store, project.ID, "issue-gate-pass", core.IssueStatusDraft, core.IssueStateOpen)

	scheduler := &fakeManagerScheduler{}
	manager, err := NewManager(
		store,
		nil,
		nil,
		scheduler,
		WithGateChain(&GateChain{
			Store: store,
			Runners: map[core.GateType]core.GateRunner{
				core.GateTypeAuto: &AutoGateRunner{
					Reviewer: &gateStubDemandReviewer{
						verdict: core.ReviewVerdict{Status: "pass", Score: 95, Summary: "ready"},
					},
				},
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if err := manager.SubmitForReview(context.Background(), []string{issue.ID}); err != nil {
		t.Fatalf("SubmitForReview() error = %v", err)
	}
	if scheduler.startIssueCalls != 1 {
		t.Fatalf("scheduler StartIssue calls = %d, want 1", scheduler.startIssueCalls)
	}

	updated, err := store.GetIssue(issue.ID)
	if err != nil {
		t.Fatalf("GetIssue(%s) error = %v", issue.ID, err)
	}
	if updated.Status != core.IssueStatusQueued {
		t.Fatalf("updated status = %q, want %q", updated.Status, core.IssueStatusQueued)
	}

	checks, err := store.GetGateChecks(issue.ID)
	if err != nil {
		t.Fatalf("GetGateChecks(%s) error = %v", issue.ID, err)
	}
	if len(checks) != 1 {
		t.Fatalf("gate checks len = %d, want 1", len(checks))
	}
	if checks[0].Status != core.GateStatusPassed {
		t.Fatalf("gate status = %q, want %q", checks[0].Status, core.GateStatusPassed)
	}
	if strings.TrimSpace(checks[0].DecisionID) == "" {
		t.Fatal("expected saved gate check to reference a decision")
	}

	decisions, err := store.ListDecisions(issue.ID)
	if err != nil {
		t.Fatalf("ListDecisions(%s) error = %v", issue.ID, err)
	}
	if len(decisions) != 1 {
		t.Fatalf("decisions len = %d, want 1", len(decisions))
	}
	if decisions[0].Action != "pass" {
		t.Fatalf("decision action = %q, want pass", decisions[0].Action)
	}
}

func TestManager_SubmitForReviewWithGateChainPendingLeavesIssueReviewing(t *testing.T) {
	t.Parallel()

	store := newManagerTestStore(t)
	t.Cleanup(func() { _ = store.Close() })

	project := mustCreateManagerProject(t, store, "proj-manager-gate-pending")
	issue := mustCreateManagerIssue(t, store, project.ID, "issue-gate-pending", core.IssueStatusDraft, core.IssueStateOpen)
	issue.Labels = []string{"profile:strict"}
	if err := store.SaveIssue(issue); err != nil {
		t.Fatalf("SaveIssue(%s) error = %v", issue.ID, err)
	}

	scheduler := &fakeManagerScheduler{}
	manager, err := NewManager(
		store,
		nil,
		nil,
		scheduler,
		WithGateChain(&GateChain{
			Store: store,
			Runners: map[core.GateType]core.GateRunner{
				core.GateTypeAuto: &AutoGateRunner{
					Reviewer: &gateStubDemandReviewer{
						verdict: core.ReviewVerdict{Status: "pass", Score: 95, Summary: "ready"},
					},
				},
				core.GateTypePeerReview: &PeerReviewRunner{},
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if err := manager.SubmitForReview(context.Background(), []string{issue.ID}); err != nil {
		t.Fatalf("SubmitForReview() error = %v", err)
	}
	if scheduler.startIssueCalls != 0 {
		t.Fatalf("scheduler StartIssue calls = %d, want 0", scheduler.startIssueCalls)
	}

	updated, err := store.GetIssue(issue.ID)
	if err != nil {
		t.Fatalf("GetIssue(%s) error = %v", issue.ID, err)
	}
	if updated.Status != core.IssueStatusReviewing {
		t.Fatalf("updated status = %q, want %q", updated.Status, core.IssueStatusReviewing)
	}

	latest, err := store.GetLatestGateCheck(issue.ID, "peer_review")
	if err != nil {
		t.Fatalf("GetLatestGateCheck(peer_review) error = %v", err)
	}
	if latest.Status != core.GateStatusPending {
		t.Fatalf("peer_review latest status = %q, want %q", latest.Status, core.GateStatusPending)
	}
}

func TestManager_ResolveGatePassContinuesChainAndApprovesIssue(t *testing.T) {
	t.Parallel()

	store := newManagerTestStore(t)
	t.Cleanup(func() { _ = store.Close() })

	project := mustCreateManagerProject(t, store, "proj-manager-resolve-pass")
	issue := mustCreateManagerIssue(t, store, project.ID, "issue-resolve-pass", core.IssueStatusDraft, core.IssueStateOpen)
	issue.Labels = []string{"profile:strict"}
	if err := store.SaveIssue(issue); err != nil {
		t.Fatalf("SaveIssue(%s) error = %v", issue.ID, err)
	}

	scheduler := &fakeManagerScheduler{}
	manager, err := NewManager(
		store,
		nil,
		nil,
		scheduler,
		WithGateChain(&GateChain{
			Store: store,
			Runners: map[core.GateType]core.GateRunner{
				core.GateTypeAuto: &AutoGateRunner{
					Reviewer: &gateStubDemandReviewer{
						verdict: core.ReviewVerdict{Status: "pass", Score: 95, Summary: "ready"},
					},
				},
				core.GateTypePeerReview: &PeerReviewRunner{},
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if err := manager.SubmitForReview(context.Background(), []string{issue.ID}); err != nil {
		t.Fatalf("SubmitForReview() error = %v", err)
	}

	updated, err := manager.ResolveGate(context.Background(), issue.ID, "peer_review", "pass", "peer approved")
	if err != nil {
		t.Fatalf("ResolveGate(pass) error = %v", err)
	}
	if updated.Status != core.IssueStatusQueued {
		t.Fatalf("resolved status = %q, want %q", updated.Status, core.IssueStatusQueued)
	}
	if scheduler.startIssueCalls != 1 {
		t.Fatalf("scheduler StartIssue calls = %d, want 1", scheduler.startIssueCalls)
	}
}

func TestManager_ResolveGateFailRejectsIssue(t *testing.T) {
	t.Parallel()

	store := newManagerTestStore(t)
	t.Cleanup(func() { _ = store.Close() })

	project := mustCreateManagerProject(t, store, "proj-manager-resolve-fail")
	issue := mustCreateManagerIssue(t, store, project.ID, "issue-resolve-fail", core.IssueStatusDraft, core.IssueStateOpen)
	issue.Labels = []string{"profile:strict"}
	if err := store.SaveIssue(issue); err != nil {
		t.Fatalf("SaveIssue(%s) error = %v", issue.ID, err)
	}

	manager, err := NewManager(
		store,
		nil,
		nil,
		&fakeManagerScheduler{},
		WithGateChain(&GateChain{
			Store: store,
			Runners: map[core.GateType]core.GateRunner{
				core.GateTypeAuto: &AutoGateRunner{
					Reviewer: &gateStubDemandReviewer{
						verdict: core.ReviewVerdict{Status: "pass", Score: 95, Summary: "ready"},
					},
				},
				core.GateTypePeerReview: &PeerReviewRunner{},
			},
		}),
	)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if err := manager.SubmitForReview(context.Background(), []string{issue.ID}); err != nil {
		t.Fatalf("SubmitForReview() error = %v", err)
	}

	updated, err := manager.ResolveGate(context.Background(), issue.ID, "peer_review", "fail", "needs changes")
	if err != nil {
		t.Fatalf("ResolveGate(fail) error = %v", err)
	}
	if updated.Status != core.IssueStatusDraft {
		t.Fatalf("resolved status = %q, want %q", updated.Status, core.IssueStatusDraft)
	}
}

func TestManager_SubmitForReviewUsesReviewGateWhenConfigured(t *testing.T) {
	t.Parallel()

	store := newManagerTestStore(t)
	t.Cleanup(func() { _ = store.Close() })

	project := mustCreateManagerProject(t, store, "proj-manager-submit-gate")
	issue := mustCreateManagerIssue(t, store, project.ID, "issue-submit-gate", core.IssueStatusDraft, core.IssueStateOpen)

	gate := &fakeManagerReviewGate{}
	manager, err := NewManager(store, nil, nil, &fakeManagerScheduler{}, WithReviewGate(gate))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if err := manager.SubmitForReview(context.Background(), []string{issue.ID}); err != nil {
		t.Fatalf("SubmitForReview() error = %v", err)
	}
	if gate.submitCalls != 1 {
		t.Fatalf("review gate Submit calls = %d, want 1", gate.submitCalls)
	}
	if len(gate.lastSubmitted) != 1 || gate.lastSubmitted[0].ID != issue.ID {
		t.Fatalf("review gate submitted issues = %#v, want [%s]", gate.lastSubmitted, issue.ID)
	}

	updated, err := store.GetIssue(issue.ID)
	if err != nil {
		t.Fatalf("GetIssue(%s) error = %v", issue.ID, err)
	}
	if updated.Status != core.IssueStatusReviewing {
		t.Fatalf("updated status = %q, want %q", updated.Status, core.IssueStatusReviewing)
	}
}

func TestManager_SubmitForReviewFailsWithoutSubmitter(t *testing.T) {
	t.Parallel()

	store := newManagerTestStore(t)
	t.Cleanup(func() { _ = store.Close() })

	project := mustCreateManagerProject(t, store, "proj-manager-submit-no-review")
	issue := mustCreateManagerIssue(t, store, project.ID, "issue-submit-no-review", core.IssueStatusDraft, core.IssueStateOpen)

	manager, err := NewManager(store, nil, nil, &fakeManagerScheduler{})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	err = manager.SubmitForReview(context.Background(), []string{issue.ID})
	if err == nil {
		t.Fatal("SubmitForReview() should fail when no review submitter is configured")
	}
	if !strings.Contains(err.Error(), "no issue review submitter configured") {
		t.Fatalf("error = %v, want no review submitter message", err)
	}
}

func TestManager_ApplyIssueActionApproveQueuesAndStartsIssue(t *testing.T) {
	t.Parallel()

	store := newManagerTestStore(t)
	t.Cleanup(func() { _ = store.Close() })

	project := mustCreateManagerProject(t, store, "proj-manager-approve")
	issue := mustCreateManagerIssue(t, store, project.ID, "issue-approve", core.IssueStatusReviewing, core.IssueStateOpen)

	scheduler := &fakeManagerScheduler{}
	manager, err := NewManager(store, nil, nil, scheduler)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	updated, err := manager.ApplyIssueAction(context.Background(), issue.ID, " APPROVE ", "ship it")
	if err != nil {
		t.Fatalf("ApplyIssueAction(approve) error = %v", err)
	}
	if updated.Status != core.IssueStatusQueued {
		t.Fatalf("updated status = %q, want %q", updated.Status, core.IssueStatusQueued)
	}
	if updated.State != core.IssueStateOpen {
		t.Fatalf("updated state = %q, want %q", updated.State, core.IssueStateOpen)
	}
	if updated.ClosedAt != nil {
		t.Fatal("updated closed_at should be nil after approve")
	}
	if scheduler.startIssueCalls != 1 {
		t.Fatalf("scheduler StartIssue calls = %d, want 1", scheduler.startIssueCalls)
	}
	if len(scheduler.startedIssues) != 1 || scheduler.startedIssues[0].ID != issue.ID {
		t.Fatalf("scheduler started issues = %#v, want [%s]", scheduler.startedIssues, issue.ID)
	}

	change := mustLatestIssueChange(t, store, issue.ID)
	if change.OldValue != string(core.IssueStatusReviewing) {
		t.Fatalf("change old_value = %q, want %q", change.OldValue, core.IssueStatusReviewing)
	}
	if change.NewValue != string(core.IssueStatusQueued) {
		t.Fatalf("change new_value = %q, want %q", change.NewValue, core.IssueStatusQueued)
	}
	if change.Reason != "ship it" {
		t.Fatalf("change reason = %q, want %q", change.Reason, "ship it")
	}
}

func TestManager_ApplyIssueActionApproveStartIssueFailureMarksIssueFailed(t *testing.T) {
	t.Parallel()

	store := newManagerTestStore(t)
	t.Cleanup(func() { _ = store.Close() })

	project := mustCreateManagerProject(t, store, "proj-manager-approve-start-fail")
	issue := mustCreateManagerIssue(t, store, project.ID, "issue-approve-start-fail", core.IssueStatusReviewing, core.IssueStateOpen)

	startErr := errors.New("scheduler unavailable")
	scheduler := &fakeManagerScheduler{
		startIssueErr: startErr,
	}
	manager, err := NewManager(store, nil, nil, scheduler)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	updated, err := manager.ApplyIssueAction(context.Background(), issue.ID, IssueActionApprove, "ship it")
	if err == nil {
		t.Fatal("ApplyIssueAction(approve) should fail when scheduler StartIssue fails")
	}
	if updated != nil {
		t.Fatalf("ApplyIssueAction(approve) result = %#v, want nil on failure", updated)
	}
	if !strings.Contains(err.Error(), "start issue scheduler for "+issue.ID) {
		t.Fatalf("error = %v, want issue scheduler start context", err)
	}
	if !strings.Contains(err.Error(), startErr.Error()) {
		t.Fatalf("error = %v, want root scheduler error %q", err, startErr)
	}
	if scheduler.startIssueCalls != 1 {
		t.Fatalf("scheduler StartIssue calls = %d, want 1", scheduler.startIssueCalls)
	}

	persisted, err := store.GetIssue(issue.ID)
	if err != nil {
		t.Fatalf("GetIssue(%s) error = %v", issue.ID, err)
	}
	if persisted.Status != core.IssueStatusFailed {
		t.Fatalf("persisted status = %q, want %q", persisted.Status, core.IssueStatusFailed)
	}
	if persisted.State != core.IssueStateOpen {
		t.Fatalf("persisted state = %q, want %q", persisted.State, core.IssueStateOpen)
	}
	if persisted.ClosedAt != nil {
		t.Fatal("persisted closed_at should be nil after approve dispatch failure")
	}
	if persisted.RunID != "" {
		t.Fatalf("persisted run_id = %q, want empty when dispatch fails", persisted.RunID)
	}

	changes, err := store.GetIssueChanges(issue.ID)
	if err != nil {
		t.Fatalf("GetIssueChanges(%s) error = %v", issue.ID, err)
	}
	var hasApproveQueuedChange bool
	var hasDispatchFailedChange bool
	for _, change := range changes {
		if change.OldValue == string(core.IssueStatusReviewing) &&
			change.NewValue == string(core.IssueStatusQueued) &&
			change.Reason == "ship it" {
			hasApproveQueuedChange = true
		}
		if change.OldValue == string(core.IssueStatusQueued) &&
			change.NewValue == string(core.IssueStatusFailed) &&
			strings.Contains(change.Reason, "approve dispatch failed") &&
			strings.Contains(change.Reason, startErr.Error()) {
			hasDispatchFailedChange = true
		}
	}
	if !hasApproveQueuedChange {
		t.Fatalf("missing approve queued issue change, got %#v", changes)
	}
	if !hasDispatchFailedChange {
		t.Fatalf("missing dispatch failure issue change, got %#v", changes)
	}
}

func TestManager_ApplyIssueActionRejectRequiresFeedback(t *testing.T) {
	t.Parallel()

	store := newManagerTestStore(t)
	t.Cleanup(func() { _ = store.Close() })

	project := mustCreateManagerProject(t, store, "proj-manager-reject-no-feedback")
	issue := mustCreateManagerIssue(t, store, project.ID, "issue-reject-no-feedback", core.IssueStatusReviewing, core.IssueStateOpen)

	manager, err := NewManager(store, nil, nil, &fakeManagerScheduler{})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	_, err = manager.ApplyIssueAction(context.Background(), issue.ID, IssueActionReject, "   ")
	if err == nil {
		t.Fatal("ApplyIssueAction(reject) should fail when feedback is empty")
	}
	if !strings.Contains(err.Error(), "reject action requires feedback") {
		t.Fatalf("error = %v, want reject feedback requirement", err)
	}

	persisted, err := store.GetIssue(issue.ID)
	if err != nil {
		t.Fatalf("GetIssue(%s) error = %v", issue.ID, err)
	}
	if persisted.Status != core.IssueStatusReviewing {
		t.Fatalf("persisted status = %q, want unchanged %q", persisted.Status, core.IssueStatusReviewing)
	}
}

func TestManager_ApplyIssueActionRejectMovesIssueToDraft(t *testing.T) {
	t.Parallel()

	store := newManagerTestStore(t)
	t.Cleanup(func() { _ = store.Close() })

	project := mustCreateManagerProject(t, store, "proj-manager-reject")
	issue := mustCreateManagerIssue(t, store, project.ID, "issue-reject", core.IssueStatusReviewing, core.IssueStateOpen)

	manager, err := NewManager(store, nil, nil, &fakeManagerScheduler{})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	updated, err := manager.ApplyIssueAction(context.Background(), issue.ID, IssueActionReject, "need more details")
	if err != nil {
		t.Fatalf("ApplyIssueAction(reject) error = %v", err)
	}
	if updated.Status != core.IssueStatusDraft {
		t.Fatalf("updated status = %q, want %q", updated.Status, core.IssueStatusDraft)
	}
	if updated.State != core.IssueStateOpen {
		t.Fatalf("updated state = %q, want %q", updated.State, core.IssueStateOpen)
	}
	if updated.ClosedAt != nil {
		t.Fatal("updated closed_at should be nil after reject")
	}

	change := mustLatestIssueChange(t, store, issue.ID)
	if change.Reason != "need more details" {
		t.Fatalf("change reason = %q, want %q", change.Reason, "need more details")
	}
}

func TestManager_ApplyIssueActionAbandonClosesIssue(t *testing.T) {
	t.Parallel()

	store := newManagerTestStore(t)
	t.Cleanup(func() { _ = store.Close() })

	project := mustCreateManagerProject(t, store, "proj-manager-abandon")
	issue := mustCreateManagerIssue(t, store, project.ID, "issue-abandon", core.IssueStatusReviewing, core.IssueStateOpen)

	manager, err := NewManager(store, nil, nil, &fakeManagerScheduler{})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	updated, err := manager.ApplyIssueAction(context.Background(), issue.ID, IssueActionAbandon, "")
	if err != nil {
		t.Fatalf("ApplyIssueAction(abandon) error = %v", err)
	}
	if updated.Status != core.IssueStatusAbandoned {
		t.Fatalf("updated status = %q, want %q", updated.Status, core.IssueStatusAbandoned)
	}
	if updated.State != core.IssueStateClosed {
		t.Fatalf("updated state = %q, want %q", updated.State, core.IssueStateClosed)
	}
	if updated.ClosedAt == nil {
		t.Fatal("updated closed_at should be set after abandon")
	}

	change := mustLatestIssueChange(t, store, issue.ID)
	if change.Reason != "human abandon" {
		t.Fatalf("change reason = %q, want %q", change.Reason, "human abandon")
	}
}

func newManagerTestStore(t *testing.T) core.Store {
	t.Helper()

	store, err := storesqlite.New(":memory:")
	if err != nil {
		t.Fatalf("storesqlite.New() error = %v", err)
	}
	return store
}

func mustCreateManagerProject(t *testing.T, store core.Store, id string) *core.Project {
	t.Helper()

	project := &core.Project{
		ID:       id,
		Name:     id,
		RepoPath: t.TempDir(),
	}
	if err := store.CreateProject(project); err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	return project
}

func mustCreateManagerIssue(
	t *testing.T,
	store core.Store,
	projectID string,
	issueID string,
	status core.IssueStatus,
	state core.IssueState,
) *core.Issue {
	t.Helper()

	if status == "" {
		status = core.IssueStatusDraft
	}
	if state == "" {
		state = core.IssueStateOpen
	}

	issue := &core.Issue{
		ID:         issueID,
		ProjectID:  projectID,
		Title:      issueID,
		Body:       "manager test issue",
		Template:   "standard",
		Status:     status,
		State:      state,
		FailPolicy: core.FailBlock,
	}
	if err := store.CreateIssue(issue); err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}

	persisted, err := store.GetIssue(issueID)
	if err != nil {
		t.Fatalf("GetIssue(%s) error = %v", issueID, err)
	}
	return persisted
}

func mustLatestIssueChange(t *testing.T, store core.Store, issueID string) core.IssueChange {
	t.Helper()

	changes, err := store.GetIssueChanges(issueID)
	if err != nil {
		t.Fatalf("GetIssueChanges(%s) error = %v", issueID, err)
	}
	if len(changes) == 0 {
		t.Fatalf("GetIssueChanges(%s) returned no changes", issueID)
	}
	return changes[len(changes)-1]
}

type fakeManagerScheduler struct {
	mu sync.Mutex

	startCalled   bool
	stopCalled    bool
	recoverCalled bool

	startIssueCalls int
	startedIssues   []*core.Issue

	startErr      error
	stopErr       error
	recoverErr    error
	startIssueErr error
}

func (s *fakeManagerScheduler) Start(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startCalled = true
	return s.startErr
}

func (s *fakeManagerScheduler) Stop(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopCalled = true
	return s.stopErr
}

func (s *fakeManagerScheduler) RecoverExecutingIssues(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recoverCalled = true
	return s.recoverErr
}

func (s *fakeManagerScheduler) StartIssue(_ context.Context, issue *core.Issue) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.startIssueCalls++
	s.startedIssues = append(s.startedIssues, cloneManagerIssue(issue))
	return s.startIssueErr
}

type fakeManagerTwoPhaseReview struct {
	mu sync.Mutex

	submitCalls   int
	lastSubmitted []*core.Issue
	submitHook    func(issues []*core.Issue) error
	submitErr     error
}

func (r *fakeManagerTwoPhaseReview) SubmitForReview(_ context.Context, issues []*core.Issue) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.submitCalls++
	r.lastSubmitted = cloneManagerIssues(issues)
	if r.submitHook != nil {
		if err := r.submitHook(cloneManagerIssues(issues)); err != nil {
			return err
		}
	}
	return r.submitErr
}

type fakeManagerReviewGate struct {
	mu sync.Mutex

	submitCalls   int
	lastSubmitted []*core.Issue
	submitErr     error
}

func (g *fakeManagerReviewGate) Name() string {
	return "fake-manager-review-gate"
}

func (g *fakeManagerReviewGate) Init(context.Context) error {
	return nil
}

func (g *fakeManagerReviewGate) Close() error {
	return nil
}

func (g *fakeManagerReviewGate) Submit(_ context.Context, issues []*core.Issue) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.submitCalls++
	g.lastSubmitted = cloneManagerIssues(issues)
	return "review-manager-test", g.submitErr
}

func (g *fakeManagerReviewGate) Check(_ context.Context, _ string) (*core.ReviewResult, error) {
	return &core.ReviewResult{
		Status:   core.ReviewStatusPending,
		Decision: core.ReviewDecisionPending,
	}, nil
}

func (g *fakeManagerReviewGate) Cancel(_ context.Context, _ string) error {
	return nil
}
