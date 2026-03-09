package teamleader

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

const (
	IssueActionApprove = "approve"
	IssueActionReject  = "reject"
	IssueActionAbandon = "abandon"
)

type CreateIssueSpec struct {
	ID          string
	Title       string
	Body        string
	Template    string
	AutoMerge   *bool
	Labels      []string
	DependsOn   []string
	Blocks      []string
	Priority    int
	FailPolicy  core.FailurePolicy
	MilestoneID string
	ExternalID  string
}

type CreateIssuesInput struct {
	ProjectID string
	SessionID string
	Issues    []CreateIssueSpec
}

type managerScheduler interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

type managerIssueScheduler interface {
	RecoverExecutingIssues(ctx context.Context) error
	StartIssue(ctx context.Context, issue *core.Issue) error
}

type managerTwoPhaseReview interface {
	SubmitForReview(ctx context.Context, issues []*core.Issue) error
}

type managerIssueReviewSubmitter interface {
	Submit(ctx context.Context, issues []*core.Issue) error
}

type managerIssueRollbackStore interface {
	DeleteIssue(id string) error
}

type ManagerOption func(*Manager)

func WithEventPublisher(pub eventPublisher) ManagerOption {
	return func(m *Manager) {
		if m == nil {
			return
		}
		m.pub = pub
	}
}

func WithReviewGate(gate core.ReviewGate) ManagerOption {
	return func(m *Manager) {
		if m == nil {
			return
		}
		m.reviewGate = gate
	}
}

func WithGateChain(gc *GateChain) ManagerOption {
	return func(m *Manager) {
		if m == nil {
			return
		}
		m.gateChain = gc
	}
}

type Manager struct {
	store      core.Store
	scheduler  managerScheduler
	reviewGate core.ReviewGate
	pub        eventPublisher

	twoPhaseReview  managerTwoPhaseReview
	reviewSubmitter managerIssueReviewSubmitter
	gateChain       *GateChain
}

// SetGateChain installs a GateChain for gate-based issue review.
// When set, submitIssues uses the gate chain path instead of the legacy review path.
func (m *Manager) SetGateChain(gc *GateChain) {
	m.gateChain = gc
}

func NewManager(store core.Store, _ any, review any, scheduler managerScheduler, opts ...ManagerOption) (*Manager, error) {
	if store == nil {
		return nil, errors.New("manager store is required")
	}
	if scheduler == nil {
		return nil, errors.New("manager scheduler is required")
	}

	manager := &Manager{
		store:      store,
		scheduler:  scheduler,
		reviewGate: nil,
	}
	if tp, ok := review.(managerTwoPhaseReview); ok {
		manager.twoPhaseReview = tp
	}
	if submitter, ok := review.(managerIssueReviewSubmitter); ok {
		manager.reviewSubmitter = submitter
	}

	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(manager)
	}
	return manager, nil
}

func (m *Manager) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := m.scheduler.Start(ctx); err != nil {
		return fmt.Errorf("start scheduler: %w", err)
	}

	issueScheduler, err := m.issueScheduler()
	if err != nil {
		return err
	}
	if err := issueScheduler.RecoverExecutingIssues(ctx); err != nil {
		return fmt.Errorf("recover executing issues: %w", err)
	}
	return nil
}

func (m *Manager) Stop(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := m.scheduler.Stop(ctx); err != nil {
		return fmt.Errorf("stop scheduler: %w", err)
	}
	return nil
}

func (m *Manager) CreateIssues(ctx context.Context, input CreateIssuesInput) ([]*core.Issue, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	projectID := strings.TrimSpace(input.ProjectID)
	if len(input.Issues) == 0 {
		return nil, errors.New("issues are required")
	}

	created := make([]*core.Issue, 0, len(input.Issues))
	createdIDs := make([]string, 0, len(input.Issues))
	usedIDs := make(map[string]struct{}, len(input.Issues))
	rollbackCreated := func(cause error) error {
		if cause == nil || len(createdIDs) == 0 {
			return cause
		}
		rollbackStore, ok := m.store.(managerIssueRollbackStore)
		if !ok {
			return cause
		}
		var rollbackErrs []string
		for i := len(createdIDs) - 1; i >= 0; i-- {
			if err := rollbackStore.DeleteIssue(createdIDs[i]); err != nil && !isManagerNotFoundError(err) {
				rollbackErrs = append(rollbackErrs, fmt.Sprintf("%s: %v", createdIDs[i], err))
			}
		}
		if len(rollbackErrs) == 0 {
			return cause
		}
		return fmt.Errorf("%w (rollback issues: %s)", cause, strings.Join(rollbackErrs, "; "))
	}

	for i := range input.Issues {
		spec := input.Issues[i]
		issueID := strings.TrimSpace(spec.ID)
		if issueID == "" {
			issueID = core.NewIssueID()
		}
		if _, exists := usedIDs[issueID]; exists {
			return nil, rollbackCreated(fmt.Errorf("duplicate issue id %q in create input", issueID))
		}
		usedIDs[issueID] = struct{}{}

		template := strings.TrimSpace(spec.Template)
		if template == "" {
			template = "standard"
		}
		failPolicy := spec.FailPolicy
		if failPolicy == "" {
			failPolicy = core.FailBlock
		}
		autoMerge := true
		if spec.AutoMerge != nil {
			autoMerge = *spec.AutoMerge
		}

		issue := &core.Issue{
			ID:          issueID,
			ProjectID:   projectID,
			SessionID:   strings.TrimSpace(input.SessionID),
			Title:       strings.TrimSpace(spec.Title),
			Body:        spec.Body,
			Labels:      cloneStringSlice(spec.Labels),
			DependsOn:   normalizeIssueRefs(spec.DependsOn),
			Blocks:      normalizeIssueRefs(spec.Blocks),
			Priority:    spec.Priority,
			Template:    template,
			AutoMerge:   autoMerge,
			State:       core.IssueStateOpen,
			Status:      core.IssueStatusDraft,
			MilestoneID: strings.TrimSpace(spec.MilestoneID),
			ExternalID:  strings.TrimSpace(spec.ExternalID),
			FailPolicy:  failPolicy,
		}
		if err := issue.Validate(); err != nil {
			return nil, rollbackCreated(fmt.Errorf("validate issue %s: %w", issueID, err))
		}
		if err := m.store.CreateIssue(issue); err != nil {
			return nil, rollbackCreated(fmt.Errorf("create issue %s: %w", issueID, err))
		}
		createdIDs = append(createdIDs, issueID)
		m.recordTaskStep(issueID, core.StepCreated, "system", "issue created")

		latest, err := m.store.GetIssue(issueID)
		if err != nil {
			created = append(created, cloneManagerIssue(issue))
			continue
		}
		created = append(created, latest)
	}
	return created, nil
}

func isManagerNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "not found")
}
func (m *Manager) ConfirmCreatedIssues(ctx context.Context, issueIDs []string, feedback string) ([]*core.Issue, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	issues, err := m.loadIssues(issueIDs)
	if err != nil {
		return nil, err
	}

	reason := strings.TrimSpace(feedback)
	if reason == "" {
		reason = "confirmed from decompose proposal"
	}

	updatedIssues := make([]*core.Issue, 0, len(issues))
	for _, issue := range issues {
		if issue == nil {
			continue
		}
		switch issue.Status {
		case core.IssueStatusDraft:
			// continue below with the normal draft -> reviewing -> approve flow.
		case core.IssueStatusReviewing:
			approved, err := m.applyIssueApprove(ctx, issue, reason)
			if err != nil {
				return nil, err
			}
			updatedIssues = append(updatedIssues, approved)
			continue
		case core.IssueStatusQueued,
			core.IssueStatusReady,
			core.IssueStatusExecuting,
			core.IssueStatusMerging,
			core.IssueStatusDone,
			core.IssueStatusDecomposing,
			core.IssueStatusDecomposed:
			latest, err := m.loadIssue(issue.ID)
			if err != nil {
				return nil, err
			}
			updatedIssues = append(updatedIssues, latest)
			continue
		default:
			return nil, fmt.Errorf("issue %s cannot be confirmed from status %s", issue.ID, issue.Status)
		}

		reviewing := cloneManagerIssue(issue)
		before := reviewing.Status
		if err := transitionIssueStatus(reviewing, core.IssueStatusReviewing); err != nil {
			return nil, fmt.Errorf("transition issue %s to reviewing: %w", reviewing.ID, err)
		}
		if err := m.store.SaveIssue(reviewing); err != nil {
			return nil, fmt.Errorf("save issue %s as reviewing: %w", reviewing.ID, err)
		}
		m.recordTaskStep(reviewing.ID, core.StepSubmittedForReview, "system", reason)
		if err := m.saveIssueChange(reviewing.ID, "status", string(before), string(reviewing.Status), "decompose_confirm"); err != nil {
			return nil, err
		}

		approved, err := m.applyIssueApprove(ctx, reviewing, reason)
		if err != nil {
			return nil, err
		}
		updatedIssues = append(updatedIssues, approved)
	}
	return updatedIssues, nil
}

func (m *Manager) SubmitForReview(ctx context.Context, issueIDs []string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	issues, err := m.loadIssues(issueIDs)
	if err != nil {
		return err
	}
	if m.gateChain != nil {
		return m.submitIssuesWithGateChain(ctx, issues)
	}

	roundBeforeSubmit := make(map[string]int, len(issues))
	for _, issue := range issues {
		records, getErr := m.store.GetReviewRecords(issue.ID)
		if getErr != nil {
			return fmt.Errorf("load review records for issue %s before submit: %w", issue.ID, getErr)
		}
		roundBeforeSubmit[issue.ID] = maxReviewRound(records)
	}

	if err := m.submitIssues(ctx, issues); err != nil {
		return err
	}

	for _, issue := range issues {
		before := issue.Status
		updated := cloneManagerIssue(issue)
		if err := transitionIssueStatus(updated, core.IssueStatusReviewing); err != nil {
			return fmt.Errorf("transition issue %s to reviewing: %w", updated.ID, err)
		}
		if updated.State == "" {
			updated.State = core.IssueStateOpen
		}

		if err := m.store.SaveIssue(updated); err != nil {
			return fmt.Errorf("save issue %s as reviewing: %w", updated.ID, err)
		}
		m.recordTaskStep(updated.ID, core.StepSubmittedForReview, "system", "submitted for review")
		if err := m.saveIssueChange(updated.ID, "status", string(before), string(updated.Status), "submit_for_review"); err != nil {
			return err
		}
		if !updated.AutoMerge {
			continue
		}
		autoApproved, autoApproveErr := m.shouldAutoApproveIssue(updated.ID, roundBeforeSubmit[updated.ID])
		if autoApproveErr != nil {
			return autoApproveErr
		}
		if !autoApproved {
			continue
		}
		if _, autoApproveErr := m.applyIssueApprove(ctx, updated, "auto approve after review pass"); autoApproveErr != nil {
			return autoApproveErr
		}
	}
	return nil
}

func (m *Manager) ApplyIssueAction(ctx context.Context, issueID, action, feedback string) (*core.Issue, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	issue, err := m.loadIssue(issueID)
	if err != nil {
		return nil, err
	}

	switch normalizeIssueAction(action) {
	case IssueActionApprove:
		return m.applyIssueApprove(ctx, issue, feedback)
	case IssueActionReject:
		return m.applyIssueReject(ctx, issue, feedback)
	case IssueActionAbandon:
		return m.applyIssueAbandon(ctx, issue, feedback)
	default:
		return nil, fmt.Errorf("unsupported issue action %q", strings.TrimSpace(action))
	}
}

func (m *Manager) submitIssues(ctx context.Context, issues []*core.Issue) error {
	switch {
	case m.twoPhaseReview != nil:
		if err := m.twoPhaseReview.SubmitForReview(ctx, cloneManagerIssues(issues)); err != nil {
			return fmt.Errorf("submit issues via two-phase review: %w", err)
		}
		return nil
	case m.reviewSubmitter != nil:
		if err := m.reviewSubmitter.Submit(ctx, cloneManagerIssues(issues)); err != nil {
			return fmt.Errorf("submit issues via review submitter: %w", err)
		}
		return nil
	case m.reviewGate != nil:
		if _, err := m.reviewGate.Submit(ctx, cloneManagerIssues(issues)); err != nil {
			return fmt.Errorf("submit issues via review gate: %w", err)
		}
		return nil
	default:
		return errors.New("no issue review submitter configured")
	}
}

func (m *Manager) submitIssuesWithGateChain(ctx context.Context, issues []*core.Issue) error {
	for _, issue := range issues {
		before := issue.Status
		reviewing := cloneManagerIssue(issue)
		if err := transitionIssueStatus(reviewing, core.IssueStatusReviewing); err != nil {
			return fmt.Errorf("transition issue %s to reviewing: %w", reviewing.ID, err)
		}
		if reviewing.State == "" {
			reviewing.State = core.IssueStateOpen
		}
		if err := m.store.SaveIssue(reviewing); err != nil {
			return fmt.Errorf("save issue %s as reviewing: %w", reviewing.ID, err)
		}
		m.recordTaskStep(reviewing.ID, core.StepSubmittedForReview, "system", "submitted for review")
		if err := m.saveIssueChange(reviewing.ID, "status", string(before), string(reviewing.Status), "submit_for_review"); err != nil {
			return err
		}

		wp := core.WorkflowProfile{Type: workflowProfileFromIssue(reviewing), SLAMinutes: 10}
		result, err := m.gateChain.Run(ctx, reviewing, wp.ResolveGates())
		if err != nil {
			return fmt.Errorf("gate chain for issue %s: %w", reviewing.ID, err)
		}
		switch {
		case result.AllPassed:
			if _, err := m.applyIssueApprove(ctx, reviewing, "approved by gate chain"); err != nil {
				return err
			}
		case result.PendingGate != "":
			continue
		case result.FailedCheck != nil:
			reason := strings.TrimSpace(result.FailedCheck.Reason)
			if reason == "" {
				reason = fmt.Sprintf("gate %s failed", result.FailedCheck.GateName)
			} else {
				reason = fmt.Sprintf("gate %s failed: %s", result.FailedCheck.GateName, reason)
			}
			if _, err := m.applyIssueReject(ctx, reviewing, reason); err != nil {
				return err
			}
		default:
			return fmt.Errorf("gate chain for issue %s returned no outcome", reviewing.ID)
		}
	}
	return nil
}

func (m *Manager) ResolveGate(ctx context.Context, issueID, gateName, action, reason string) (*core.Issue, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m.gateChain == nil {
		return nil, errors.New("gate chain is not configured")
	}

	issue, err := m.loadIssue(issueID)
	if err != nil {
		return nil, err
	}
	resolvedAction := strings.ToLower(strings.TrimSpace(action))
	if resolvedAction != "pass" && resolvedAction != "fail" {
		return nil, fmt.Errorf("unsupported gate action %q", strings.TrimSpace(action))
	}
	gateName = strings.TrimSpace(gateName)
	if gateName == "" {
		return nil, errors.New("gate name is required")
	}

	wp := core.WorkflowProfile{Type: workflowProfileFromIssue(issue), SLAMinutes: 10}
	gates := wp.ResolveGates()
	gateIndex := -1
	var gate core.Gate
	for i := range gates {
		if gates[i].Name == gateName {
			gateIndex = i
			gate = gates[i]
			break
		}
	}
	if gateIndex < 0 {
		return nil, fmt.Errorf("gate %s is not configured for issue %s", gateName, issue.ID)
	}

	latest, err := m.store.GetLatestGateCheck(issue.ID, gateName)
	if err != nil {
		return nil, fmt.Errorf("load latest gate check for %s: %w", gateName, err)
	}
	if latest.Status != core.GateStatusPending {
		return nil, fmt.Errorf("gate %s is not pending", gateName)
	}

	resolutionReason := strings.TrimSpace(reason)
	if resolutionReason == "" {
		if resolvedAction == "pass" {
			resolutionReason = "approved by human gate resolution"
		} else {
			resolutionReason = "rejected by human gate resolution"
		}
	}

	status := core.GateStatusPassed
	taskAction := core.StepGatePassed
	if resolvedAction == "fail" {
		status = core.GateStatusFailed
		taskAction = core.StepGateFailed
	}

	check := &core.GateCheck{
		ID:        core.NewGateCheckID(),
		IssueID:   issue.ID,
		GateName:  gate.Name,
		GateType:  gate.Type,
		Attempt:   latest.Attempt + 1,
		Status:    status,
		Reason:    resolutionReason,
		CheckedBy: "human",
		CreatedAt: time.Now(),
	}
	if err := m.saveGateDecision(issue, gate, check); err != nil {
		return nil, err
	}
	if err := m.store.SaveGateCheck(check); err != nil {
		return nil, fmt.Errorf("save resolved gate check %s: %w", gateName, err)
	}
	if _, err := m.store.SaveTaskStep(&core.TaskStep{
		ID:        core.NewTaskStepID(),
		IssueID:   issue.ID,
		Action:    taskAction,
		Note:      fmt.Sprintf("[gate:%s] %s", gateName, resolutionReason),
		RefID:     check.ID,
		RefType:   "gate_check",
		CreatedAt: time.Now(),
	}); err != nil {
		return nil, fmt.Errorf("save gate resolution task step %s: %w", gateName, err)
	}

	if resolvedAction == "fail" {
		return m.applyIssueReject(ctx, issue, resolutionReason)
	}

	remaining := gates[gateIndex+1:]
	if len(remaining) == 0 {
		return m.applyIssueApprove(ctx, issue, resolutionReason)
	}
	result, err := m.gateChain.Run(ctx, issue, remaining)
	if err != nil {
		return nil, fmt.Errorf("continue gate chain for issue %s: %w", issue.ID, err)
	}
	switch {
	case result.AllPassed:
		return m.applyIssueApprove(ctx, issue, resolutionReason)
	case result.PendingGate != "":
		return m.loadIssue(issue.ID)
	case result.FailedCheck != nil:
		rejectReason := strings.TrimSpace(result.FailedCheck.Reason)
		if rejectReason == "" {
			rejectReason = fmt.Sprintf("gate %s failed", result.FailedCheck.GateName)
		} else {
			rejectReason = fmt.Sprintf("gate %s failed: %s", result.FailedCheck.GateName, rejectReason)
		}
		return m.applyIssueReject(ctx, issue, rejectReason)
	default:
		return nil, fmt.Errorf("continue gate chain for issue %s returned no outcome", issue.ID)
	}
}

func (m *Manager) applyIssueApprove(ctx context.Context, issue *core.Issue, feedback string) (*core.Issue, error) {
	updated := cloneManagerIssue(issue)
	before := updated.Status
	updated.State = core.IssueStateOpen
	updated.ClosedAt = nil

	reason := strings.TrimSpace(feedback)
	if reason == "" {
		reason = "human approve"
	}

	// Epic/decomposable issues go to decomposing instead of scheduling.
	if updated.NeedsDecomposition() && updated.ParentID == "" {
		if err := transitionIssueStatus(updated, core.IssueStatusDecomposing); err != nil {
			return nil, fmt.Errorf("transition issue %s to decomposing: %w", updated.ID, err)
		}
		if err := m.store.SaveIssue(updated); err != nil {
			return nil, fmt.Errorf("save decomposing issue %s: %w", updated.ID, err)
		}
		m.recordTaskStep(updated.ID, core.StepDecomposeStarted, "system", "approved for decomposition")
		if err := m.saveIssueChange(updated.ID, "status", string(before), string(updated.Status), reason); err != nil {
			return nil, err
		}
		if m.pub != nil {
			m.pub.Publish(ctx, core.Event{
				Type:      core.EventIssueDecomposing,
				IssueID:   updated.ID,
				ProjectID: updated.ProjectID,
				Timestamp: time.Now(),
			})
		}
		return m.loadIssue(updated.ID)
	}

	// Normal path: queue for execution.
	issueScheduler, err := m.issueScheduler()
	if err != nil {
		return nil, err
	}
	if err := transitionIssueStatus(updated, core.IssueStatusQueued); err != nil {
		return nil, fmt.Errorf("transition issue %s to queued: %w", updated.ID, err)
	}
	if err := m.store.SaveIssue(updated); err != nil {
		return nil, fmt.Errorf("save approved issue %s: %w", updated.ID, err)
	}
	m.recordTaskStep(updated.ID, core.StepReviewApproved, "system", "approved by review gate")
	m.recordTaskStep(updated.ID, core.StepQueued, "system", "queued for execution")
	if err := m.saveIssueChange(updated.ID, "status", string(before), string(updated.Status), reason); err != nil {
		return nil, err
	}
	if err := issueScheduler.StartIssue(ctx, cloneManagerIssue(updated)); err != nil {
		if markErr := m.markApproveDispatchFailure(updated, err); markErr != nil {
			return nil, fmt.Errorf("start issue scheduler for %s: %w (mark issue failed: %v)", updated.ID, err, markErr)
		}
		return nil, fmt.Errorf("start issue scheduler for %s: %w", updated.ID, err)
	}
	return m.loadIssue(updated.ID)
}

func (m *Manager) markApproveDispatchFailure(issue *core.Issue, dispatchErr error) error {
	if issue == nil {
		return nil
	}

	failed := cloneManagerIssue(issue)
	before := failed.Status
	if err := transitionIssueStatus(failed, core.IssueStatusFailed); err != nil {
		return fmt.Errorf("transition issue %s to failed: %w", failed.ID, err)
	}
	failed.State = core.IssueStateOpen
	failed.RunID = ""
	failed.ClosedAt = nil

	if err := m.store.SaveIssue(failed); err != nil {
		return fmt.Errorf("save failed issue %s after approve dispatch error: %w", failed.ID, err)
	}
	m.recordTaskStep(failed.ID, core.StepFailed, "system", "approve dispatch failed")

	reason := "approve dispatch failed"
	if dispatchErr != nil {
		detail := strings.TrimSpace(dispatchErr.Error())
		if detail != "" {
			reason = reason + ": " + detail
		}
	}
	if err := m.saveIssueChange(failed.ID, "status", string(before), string(failed.Status), reason); err != nil {
		return err
	}
	return nil
}

func (m *Manager) applyIssueReject(_ context.Context, issue *core.Issue, feedback string) (*core.Issue, error) {
	reason := strings.TrimSpace(feedback)
	if reason == "" {
		return nil, errors.New("reject action requires feedback")
	}

	updated := cloneManagerIssue(issue)
	before := updated.Status
	if err := transitionIssueStatus(updated, core.IssueStatusDraft); err != nil {
		return nil, fmt.Errorf("transition issue %s to draft: %w", updated.ID, err)
	}
	updated.State = core.IssueStateOpen
	updated.ClosedAt = nil

	if err := m.store.SaveIssue(updated); err != nil {
		return nil, fmt.Errorf("save rejected issue %s: %w", updated.ID, err)
	}
	m.recordTaskStep(updated.ID, core.StepReviewRejected, "system", reason)
	if err := m.saveIssueChange(updated.ID, "status", string(before), string(updated.Status), reason); err != nil {
		return nil, err
	}
	return m.loadIssue(updated.ID)
}

func (m *Manager) applyIssueAbandon(_ context.Context, issue *core.Issue, feedback string) (*core.Issue, error) {
	updated := cloneManagerIssue(issue)
	before := updated.Status
	if err := transitionIssueStatus(updated, core.IssueStatusAbandoned); err != nil {
		return nil, fmt.Errorf("transition issue %s to abandoned: %w", updated.ID, err)
	}
	updated.State = core.IssueStateClosed
	now := time.Now()
	updated.ClosedAt = &now

	if err := m.store.SaveIssue(updated); err != nil {
		return nil, fmt.Errorf("save abandoned issue %s: %w", updated.ID, err)
	}

	reason := strings.TrimSpace(feedback)
	if reason == "" {
		reason = "human abandon"
	}
	m.recordTaskStep(updated.ID, core.StepAbandoned, "system", reason)
	if err := m.saveIssueChange(updated.ID, "status", string(before), string(updated.Status), reason); err != nil {
		return nil, err
	}
	return m.loadIssue(updated.ID)
}

func (m *Manager) recordTaskStep(issueID string, action core.TaskStepAction, agentID, note string) {
	if m == nil || m.store == nil {
		return
	}
	issueID = strings.TrimSpace(issueID)
	if issueID == "" {
		return
	}
	if _, err := m.store.SaveTaskStep(&core.TaskStep{
		ID:        core.NewTaskStepID(),
		IssueID:   issueID,
		Action:    action,
		AgentID:   strings.TrimSpace(agentID),
		Note:      strings.TrimSpace(note),
		CreatedAt: time.Now(),
	}); err != nil {
		slog.Warn("failed to save task step", "error", err, "issue", issueID, "action", action)
	}
}

func (m *Manager) issueScheduler() (managerIssueScheduler, error) {
	issueScheduler, ok := m.scheduler.(managerIssueScheduler)
	if !ok {
		return nil, errors.New("manager scheduler does not implement issue scheduler interface")
	}
	return issueScheduler, nil
}

func (m *Manager) loadIssue(issueID string) (*core.Issue, error) {
	id := strings.TrimSpace(issueID)
	if id == "" {
		return nil, errors.New("issue id is required")
	}

	issue, err := m.store.GetIssue(id)
	if err != nil {
		return nil, fmt.Errorf("get issue %s: %w", id, err)
	}
	return issue, nil
}

func (m *Manager) loadIssues(issueIDs []string) ([]*core.Issue, error) {
	if len(issueIDs) == 0 {
		return nil, errors.New("issue ids are required")
	}

	seen := map[string]struct{}{}
	issues := make([]*core.Issue, 0, len(issueIDs))
	for _, rawID := range issueIDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}

		issue, err := m.loadIssue(id)
		if err != nil {
			return nil, err
		}
		issues = append(issues, issue)
	}

	if len(issues) == 0 {
		return nil, errors.New("issue ids are required")
	}
	return issues, nil
}

func (m *Manager) saveIssueChange(issueID, field, oldValue, newValue, reason string) error {
	change := &core.IssueChange{
		IssueID:  issueID,
		Field:    field,
		OldValue: oldValue,
		NewValue: newValue,
		Reason:   reason,
	}
	if err := m.store.SaveIssueChange(change); err != nil {
		return fmt.Errorf("save issue change for %s: %w", issueID, err)
	}
	return nil
}

func normalizeIssueAction(action string) string {
	return strings.ToLower(strings.TrimSpace(action))
}

func maxReviewRound(records []core.ReviewRecord) int {
	maxRound := 0
	for i := range records {
		if records[i].Round > maxRound {
			maxRound = records[i].Round
		}
	}
	return maxRound
}

func normalizeReviewVerdict(verdict string) string {
	normalized := strings.ToLower(strings.TrimSpace(verdict))
	switch normalized {
	case "approve":
		return "approved"
	case "ok":
		return "pass"
	case "canceled":
		return "cancelled"
	default:
		return normalized
	}
}

func (m *Manager) saveGateDecision(issue *core.Issue, gate core.Gate, check *core.GateCheck) error {
	decision, err := buildGateDecision(issue, gate, check)
	if err != nil {
		return err
	}
	if err := m.store.SaveDecision(decision); err != nil {
		return fmt.Errorf("save gate decision %s: %w", gate.Name, err)
	}
	check.DecisionID = decision.ID
	return nil
}

func (m *Manager) shouldAutoApproveIssue(issueID string, baselineRound int) (bool, error) {
	records, err := m.store.GetReviewRecords(issueID)
	if err != nil {
		return false, fmt.Errorf("load review records for issue %s: %w", issueID, err)
	}
	latestRound := maxReviewRound(records)
	if latestRound <= baselineRound {
		return false, nil
	}

	foundCurrentRoundRecord := false
	for i := range records {
		record := records[i]
		if record.Round != latestRound {
			continue
		}
		foundCurrentRoundRecord = true
		if len(record.Issues) > 0 {
			return false, nil
		}

		switch normalizeReviewVerdict(record.Verdict) {
		case "pass", "approved":
			// keep evaluating, any non-pass verdict in this round should block auto approve.
		default:
			return false, nil
		}
	}
	return foundCurrentRoundRecord, nil
}

func normalizeIssueRefs(refs []string) []string {
	if len(refs) == 0 {
		return nil
	}
	out := make([]string, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		id := strings.TrimSpace(ref)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func cloneManagerIssue(issue *core.Issue) *core.Issue {
	if issue == nil {
		return nil
	}
	cp := *issue
	cp.Labels = cloneStringSlice(issue.Labels)
	cp.DependsOn = cloneStringSlice(issue.DependsOn)
	cp.Blocks = cloneStringSlice(issue.Blocks)
	cp.Attachments = cloneStringSlice(issue.Attachments)
	return &cp
}

func cloneManagerIssues(issues []*core.Issue) []*core.Issue {
	if len(issues) == 0 {
		return nil
	}
	out := make([]*core.Issue, len(issues))
	for i := range issues {
		out[i] = cloneManagerIssue(issues[i])
	}
	return out
}
