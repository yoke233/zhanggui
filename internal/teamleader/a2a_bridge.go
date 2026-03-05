package teamleader

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

var (
	ErrA2AInvalidInput = errors.New("a2a invalid input")
	ErrA2ATaskNotFound = errors.New("a2a task not found")
	ErrA2AProjectScope = errors.New("a2a project scope mismatch")
)

type A2ATaskState string

const (
	A2ATaskStateUnknown       A2ATaskState = "unknown"
	A2ATaskStateSubmitted     A2ATaskState = "submitted"
	A2ATaskStateWorking       A2ATaskState = "working"
	A2ATaskStateInputRequired A2ATaskState = "input-required"
	A2ATaskStateCompleted     A2ATaskState = "completed"
	A2ATaskStateFailed        A2ATaskState = "failed"
	A2ATaskStateCanceled      A2ATaskState = "canceled"
)

type A2ASendMessageInput struct {
	ProjectID    string
	SessionID    string
	TaskID       string // non-empty → follow-up to existing INPUT_REQUIRED task
	Conversation string
}

type A2AGetTaskInput struct {
	ProjectID string
	TaskID    string
}

type A2ACancelTaskInput struct {
	ProjectID string
	TaskID    string
}

type A2AListTasksInput struct {
	ProjectID string
	SessionID string
	State     A2ATaskState
	PageSize  int
	PageToken string
}

type A2ATaskSnapshot struct {
	TaskID     string
	ProjectID  string
	SessionID  string
	State      A2ATaskState
	Error      string
	UpdatedAt  time.Time
	BranchName string
	Artifacts  map[string]string // from Run.Artifacts (pr_number, etc.)
}

type A2ATaskList struct {
	Tasks         []*A2ATaskSnapshot
	TotalSize     int
	PageSize      int
	NextPageToken string
}

type A2AIssueManager interface {
	CreateIssues(ctx context.Context, input CreateIssuesInput) ([]*core.Issue, error)
	ApplyIssueAction(ctx context.Context, issueID, action, feedback string) (*core.Issue, error)
}

type A2ABridge struct {
	store   core.Store
	manager A2AIssueManager
}

func NewA2ABridge(store core.Store, manager A2AIssueManager) (*A2ABridge, error) {
	if store == nil {
		return nil, errors.New("a2a bridge store is required")
	}
	if manager == nil {
		return nil, errors.New("a2a bridge manager is required")
	}
	return &A2ABridge{
		store:   store,
		manager: manager,
	}, nil
}

func (b *A2ABridge) SendMessage(ctx context.Context, input A2ASendMessageInput) (*A2ATaskSnapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if b == nil {
		return nil, errors.New("a2a bridge is nil")
	}

	// follow-up path: TaskID present means replying to an INPUT_REQUIRED task
	if taskID := strings.TrimSpace(input.TaskID); taskID != "" {
		return b.replyToTask(ctx, input)
	}

	project, err := b.resolveProjectScope(input.ProjectID)
	if err != nil {
		return nil, err
	}

	conversation := strings.TrimSpace(input.Conversation)
	if conversation == "" {
		return nil, fmt.Errorf("%w: conversation is required", ErrA2AInvalidInput)
	}

	autoMerge := true
	issues, err := b.manager.CreateIssues(ctx, CreateIssuesInput{
		ProjectID: project.ID,
		SessionID: strings.TrimSpace(input.SessionID),
		Issues: []CreateIssueSpec{
			{
				Title:      "a2a-message",
				Body:       conversation,
				Template:   "standard",
				Labels:     []string{"a2a"},
				AutoMerge:  &autoMerge,
				FailPolicy: core.FailBlock,
			},
		},
	})
	if err != nil {
		// 当解析失败时保底落地一个待人工处理的 issue，保证 A2A 接口可查询/可取消。
		fallback, fallbackErr := b.createFallbackIssue(project.ID, input.SessionID)
		if fallbackErr != nil {
			return nil, err
		}
		return snapshotFromIssue(fallback), nil
	}

	issue := firstIssue(issues)
	if issue == nil {
		return nil, errors.New("create issues returned empty result")
	}
	if err := b.ensureProjectScope(issue, project.ID); err != nil {
		return nil, err
	}

	// Auto-approve A2A issues so the run starts immediately.
	// The issue is created in draft status; approve requires reviewing first.
	slog.Info("A2A issue created", "id", issue.ID, "status", issue.Status, "auto_merge", issue.AutoMerge)
	if issue.AutoMerge {
		if issue.Status == core.IssueStatusDraft {
			if err := transitionIssueStatus(issue, core.IssueStatusReviewing); err != nil {
				slog.Error("A2A draft->reviewing transition failed", "id", issue.ID, "error", err)
			} else if err := b.store.SaveIssue(issue); err != nil {
				slog.Error("A2A save reviewing issue failed", "id", issue.ID, "error", err)
			}
		}
		approved, approveErr := b.manager.ApplyIssueAction(ctx, issue.ID, IssueActionApprove, "a2a auto-approve")
		if approveErr != nil {
			slog.Error("A2A auto-approve failed", "id", issue.ID, "error", approveErr)
		} else if approved != nil {
			slog.Info("A2A auto-approved", "id", approved.ID, "status", approved.Status, "run_id", approved.RunID)
			issue = approved
		}
	} else {
		slog.Warn("A2A issue auto_merge is false, skipping auto-approve", "id", issue.ID)
	}

	return snapshotFromIssue(issue), nil
}

func (b *A2ABridge) GetTask(ctx context.Context, input A2AGetTaskInput) (*A2ATaskSnapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if b == nil {
		return nil, errors.New("a2a bridge is nil")
	}

	project, err := b.resolveProjectScope(input.ProjectID)
	if err != nil {
		return nil, err
	}
	taskID := strings.TrimSpace(input.TaskID)
	if taskID == "" {
		return nil, fmt.Errorf("%w: task id is required", ErrA2AInvalidInput)
	}

	issue, err := b.store.GetIssue(taskID)
	if err != nil {
		if isNotFoundError(err) {
			return nil, fmt.Errorf("%w: %s", ErrA2ATaskNotFound, taskID)
		}
		return nil, err
	}
	if err := b.ensureProjectScope(issue, project.ID); err != nil {
		return nil, err
	}
	snapshot := snapshotFromIssue(issue)
	b.enrichSnapshotWithRun(snapshot, issue.RunID)
	return snapshot, nil
}

func (b *A2ABridge) CancelTask(ctx context.Context, input A2ACancelTaskInput) (*A2ATaskSnapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if b == nil {
		return nil, errors.New("a2a bridge is nil")
	}

	project, err := b.resolveProjectScope(input.ProjectID)
	if err != nil {
		return nil, err
	}
	taskID := strings.TrimSpace(input.TaskID)
	if taskID == "" {
		return nil, fmt.Errorf("%w: task id is required", ErrA2AInvalidInput)
	}

	issue, err := b.store.GetIssue(taskID)
	if err != nil {
		if isNotFoundError(err) {
			return nil, fmt.Errorf("%w: %s", ErrA2ATaskNotFound, taskID)
		}
		return nil, err
	}
	if err := b.ensureProjectScope(issue, project.ID); err != nil {
		return nil, err
	}

	canceled, err := b.manager.ApplyIssueAction(ctx, taskID, IssueActionAbandon, "a2a cancel")
	if err != nil {
		return nil, err
	}
	if canceled == nil {
		canceled, err = b.store.GetIssue(taskID)
		if err != nil {
			return nil, err
		}
	}
	if err := b.ensureProjectScope(canceled, project.ID); err != nil {
		return nil, err
	}
	return snapshotFromIssue(canceled), nil
}

func (b *A2ABridge) ListTasks(ctx context.Context, input A2AListTasksInput) (*A2ATaskList, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if b == nil {
		return nil, errors.New("a2a bridge is nil")
	}

	project, err := b.resolveProjectScope(input.ProjectID)
	if err != nil {
		return nil, err
	}

	pageSize := normalizeA2APageSize(input.PageSize)
	offset, err := parseA2APageToken(input.PageToken)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid page token", ErrA2AInvalidInput)
	}

	issues, _, err := b.store.ListIssues(project.ID, core.IssueFilter{
		SessionID: strings.TrimSpace(input.SessionID),
	})
	if err != nil {
		return nil, err
	}

	filtered := make([]*A2ATaskSnapshot, 0, len(issues))
	for i := range issues {
		snapshot := snapshotFromIssue(&issues[i])
		if !a2aTaskStateMatches(snapshot.State, input.State) {
			continue
		}
		filtered = append(filtered, snapshot)
	}

	total := len(filtered)
	if offset > total {
		offset = total
	}
	end := offset + pageSize
	if end > total {
		end = total
	}
	page := filtered[offset:end]

	nextToken := ""
	if end < total {
		nextToken = strconv.Itoa(end)
	}
	return &A2ATaskList{
		Tasks:         page,
		TotalSize:     total,
		PageSize:      pageSize,
		NextPageToken: nextToken,
	}, nil
}

func (b *A2ABridge) resolveProjectScope(projectID string) (*core.Project, error) {
	trimmed := strings.TrimSpace(projectID)
	if trimmed != "" {
		project, err := b.store.GetProject(trimmed)
		if err != nil {
			return nil, fmt.Errorf("%w: project %q not found", ErrA2AInvalidInput, trimmed)
		}
		return project, nil
	}

	projects, err := b.store.ListProjects(core.ProjectFilter{})
	if err != nil {
		return nil, err
	}
	if len(projects) == 1 {
		project := projects[0]
		return &project, nil
	}
	if len(projects) == 0 {
		return nil, fmt.Errorf("%w: project id is required", ErrA2AInvalidInput)
	}
	return nil, fmt.Errorf("%w: project id is required when multiple projects exist", ErrA2AInvalidInput)
}

func (b *A2ABridge) ensureProjectScope(issue *core.Issue, projectID string) error {
	if issue == nil {
		return errors.New("issue is nil")
	}
	if strings.TrimSpace(issue.ProjectID) != strings.TrimSpace(projectID) {
		return fmt.Errorf(
			"%w: task %q belongs to project %q, not %q",
			ErrA2AProjectScope,
			strings.TrimSpace(issue.ID),
			strings.TrimSpace(issue.ProjectID),
			strings.TrimSpace(projectID),
		)
	}
	return nil
}

func snapshotFromIssue(issue *core.Issue) *A2ATaskSnapshot {
	state := A2ATaskStateUnknown
	errMsg := ""
	if issue != nil {
		switch issue.Status {
		case core.IssueStatusDraft:
			state = A2ATaskStateSubmitted
		case core.IssueStatusReviewing:
			state = A2ATaskStateInputRequired
		case core.IssueStatusQueued, core.IssueStatusReady, core.IssueStatusExecuting, core.IssueStatusMerging, core.IssueStatusDecomposing, core.IssueStatusDecomposed:
			state = A2ATaskStateWorking
		case core.IssueStatusDone:
			state = A2ATaskStateCompleted
		case core.IssueStatusFailed:
			state = A2ATaskStateFailed
			errMsg = "issue_failed"
		case core.IssueStatusSuperseded, core.IssueStatusAbandoned:
			state = A2ATaskStateCanceled
		default:
			state = A2ATaskStateUnknown
		}
	}

	snapshot := &A2ATaskSnapshot{
		State: state,
		Error: errMsg,
	}
	if issue != nil {
		snapshot.TaskID = strings.TrimSpace(issue.ID)
		snapshot.ProjectID = strings.TrimSpace(issue.ProjectID)
		snapshot.SessionID = strings.TrimSpace(issue.SessionID)
		snapshot.UpdatedAt = issue.UpdatedAt
	}
	return snapshot
}

func firstIssue(issues []*core.Issue) *core.Issue {
	for i := range issues {
		if issues[i] != nil {
			return issues[i]
		}
	}
	return nil
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}

func (b *A2ABridge) enrichSnapshotWithRun(snapshot *A2ATaskSnapshot, runID string) {
	if snapshot == nil || strings.TrimSpace(runID) == "" {
		return
	}
	run, err := b.store.GetRun(runID)
	if err != nil || run == nil {
		return
	}
	snapshot.BranchName = strings.TrimSpace(run.BranchName)
	if len(run.Artifacts) > 0 {
		snapshot.Artifacts = make(map[string]string, len(run.Artifacts))
		for k, v := range run.Artifacts {
			snapshot.Artifacts[k] = v
		}
	}
}

// replyToTask handles the follow-up path: TaskID is set and the issue must be in
// INPUT_REQUIRED (reviewing) state. The conversation text is used as approve feedback.
func (b *A2ABridge) replyToTask(ctx context.Context, input A2ASendMessageInput) (*A2ATaskSnapshot, error) {
	taskID := strings.TrimSpace(input.TaskID)
	conversation := strings.TrimSpace(input.Conversation)
	if conversation == "" {
		return nil, fmt.Errorf("%w: conversation is required for follow-up", ErrA2AInvalidInput)
	}

	issue, err := b.store.GetIssue(taskID)
	if err != nil {
		if isNotFoundError(err) {
			return nil, fmt.Errorf("%w: %s", ErrA2ATaskNotFound, taskID)
		}
		return nil, err
	}

	// Validate project scope when caller provided an explicit projectID.
	if pid := strings.TrimSpace(input.ProjectID); pid != "" {
		if err := b.ensureProjectScope(issue, pid); err != nil {
			return nil, err
		}
	}

	if issue.Status != core.IssueStatusReviewing {
		return nil, fmt.Errorf("%w: task %q is not in input-required state (status=%s)",
			ErrA2AInvalidInput, taskID, issue.Status)
	}

	updated, err := b.manager.ApplyIssueAction(ctx, taskID, IssueActionApprove, conversation)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		updated, err = b.store.GetIssue(taskID)
		if err != nil {
			return nil, err
		}
	}
	return snapshotFromIssue(updated), nil
}

func (b *A2ABridge) createFallbackIssue(projectID string, sessionID string) (*core.Issue, error) {
	resolvedSessionID := ""
	trimmedSessionID := strings.TrimSpace(sessionID)
	if trimmedSessionID != "" {
		if _, err := b.store.GetChatSession(trimmedSessionID); err == nil {
			resolvedSessionID = trimmedSessionID
		}
	}

	fallback := &core.Issue{
		ID:         core.NewIssueID(),
		ProjectID:  strings.TrimSpace(projectID),
		SessionID:  resolvedSessionID,
		Title:      "a2a-message",
		Body:       "a2a request fallback: requires manual review",
		Template:   "standard",
		State:      core.IssueStateOpen,
		Status:     core.IssueStatusReviewing,
		FailPolicy: core.FailBlock,
	}
	if err := b.store.CreateIssue(fallback); err != nil {
		return nil, fmt.Errorf("create fallback a2a issue: %w", err)
	}
	return b.store.GetIssue(fallback.ID)
}

func normalizeA2APageSize(size int) int {
	switch {
	case size <= 0:
		return 50
	case size > 100:
		return 100
	default:
		return size
	}
}

func parseA2APageToken(token string) (int, error) {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(trimmed)
	if err != nil || offset < 0 {
		return 0, fmt.Errorf("invalid offset %q", trimmed)
	}
	return offset, nil
}

func a2aTaskStateMatches(actual A2ATaskState, expected A2ATaskState) bool {
	if strings.TrimSpace(string(expected)) == "" {
		return true
	}
	return actual == expected
}
