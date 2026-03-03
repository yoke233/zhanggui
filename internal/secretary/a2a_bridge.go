package secretary

import (
	"context"
	"errors"
	"fmt"
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

type A2ATaskSnapshot struct {
	TaskID    string
	ProjectID string
	SessionID string
	State     A2ATaskState
	Error     string
	UpdatedAt time.Time
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

	project, err := b.resolveProjectScope(input.ProjectID)
	if err != nil {
		return nil, err
	}

	conversation := strings.TrimSpace(input.Conversation)
	if conversation == "" {
		return nil, fmt.Errorf("%w: conversation is required", ErrA2AInvalidInput)
	}

	issues, err := b.manager.CreateIssues(ctx, CreateIssuesInput{
		ProjectID: project.ID,
		SessionID: strings.TrimSpace(input.SessionID),
		Issues: []CreateIssueSpec{
			{
				Title:      "a2a-message",
				Body:       conversation,
				Template:   "standard",
				Labels:     []string{"a2a"},
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
	return snapshotFromIssue(issue), nil
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
		case core.IssueStatusQueued, core.IssueStatusReady, core.IssueStatusExecuting:
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
