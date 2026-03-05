package teamleader

import (
	"context"
	"errors"
	"fmt"
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

type Manager struct {
	store      core.Store
	scheduler  managerScheduler
	reviewGate core.ReviewGate
	pub        eventPublisher

	twoPhaseReview  managerTwoPhaseReview
	reviewSubmitter managerIssueReviewSubmitter
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
	if projectID == "" {
		return nil, errors.New("project id is required")
	}
	if len(input.Issues) == 0 {
		return nil, errors.New("issues are required")
	}

	created := make([]*core.Issue, 0, len(input.Issues))
	usedIDs := make(map[string]struct{}, len(input.Issues))

	for i := range input.Issues {
		spec := input.Issues[i]
		issueID := strings.TrimSpace(spec.ID)
		if issueID == "" {
			issueID = core.NewIssueID()
		}
		if _, exists := usedIDs[issueID]; exists {
			return nil, fmt.Errorf("duplicate issue id %q in create input", issueID)
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
			ID:        issueID,
			ProjectID: projectID,
			SessionID: strings.TrimSpace(input.SessionID),
			Title:     strings.TrimSpace(spec.Title),
			Body:      spec.Body,
			Labels:    cloneStringSlice(spec.Labels),
			// V2 removes runtime dependency graph; dependency fields are ignored.
			DependsOn:   nil,
			Blocks:      nil,
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
			return nil, fmt.Errorf("validate issue %s: %w", issueID, err)
		}
		if err := m.store.CreateIssue(issue); err != nil {
			return nil, fmt.Errorf("create issue %s: %w", issueID, err)
		}

		latest, err := m.store.GetIssue(issueID)
		if err != nil {
			created = append(created, cloneManagerIssue(issue))
			continue
		}
		created = append(created, latest)
	}
	return created, nil
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
	if err := m.saveIssueChange(updated.ID, "status", string(before), string(updated.Status), reason); err != nil {
		return nil, err
	}
	return m.loadIssue(updated.ID)
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
