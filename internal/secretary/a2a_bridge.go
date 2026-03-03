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

type A2APlanManager interface {
	CreateDraft(ctx context.Context, input CreateDraftInput) (*core.TaskPlan, error)
	GetPlan(ctx context.Context, planID string) (*core.TaskPlan, error)
	CancelPlan(ctx context.Context, planID string) (*core.TaskPlan, error)
}

type A2ABridge struct {
	store   core.Store
	manager A2APlanManager
}

func NewA2ABridge(store core.Store, manager A2APlanManager) (*A2ABridge, error) {
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

	plan, err := b.manager.CreateDraft(ctx, CreateDraftInput{
		ProjectID:  project.ID,
		SessionID:  strings.TrimSpace(input.SessionID),
		Name:       "a2a-message",
		FailPolicy: core.FailBlock,
		Request: Request{
			Conversation: conversation,
			ProjectName:  strings.TrimSpace(project.Name),
			RepoPath:     strings.TrimSpace(project.RepoPath),
		},
	})
	if err != nil {
		return nil, err
	}
	if plan == nil {
		return nil, errors.New("create draft returned nil task plan")
	}
	if err := b.ensureProjectScope(plan, project.ID); err != nil {
		return nil, err
	}
	return snapshotFromTaskPlan(plan), nil
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

	plan, err := b.manager.GetPlan(ctx, taskID)
	if err != nil {
		if isNotFoundError(err) {
			return nil, fmt.Errorf("%w: %s", ErrA2ATaskNotFound, taskID)
		}
		return nil, err
	}
	if err := b.ensureProjectScope(plan, project.ID); err != nil {
		return nil, err
	}
	return snapshotFromTaskPlan(plan), nil
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

	plan, err := b.manager.GetPlan(ctx, taskID)
	if err != nil {
		if isNotFoundError(err) {
			return nil, fmt.Errorf("%w: %s", ErrA2ATaskNotFound, taskID)
		}
		return nil, err
	}
	if err := b.ensureProjectScope(plan, project.ID); err != nil {
		return nil, err
	}

	canceled, err := b.manager.CancelPlan(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if canceled == nil {
		return nil, errors.New("cancel plan returned nil task plan")
	}
	return snapshotFromTaskPlan(canceled), nil
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

func (b *A2ABridge) ensureProjectScope(plan *core.TaskPlan, projectID string) error {
	if plan == nil {
		return errors.New("task plan is nil")
	}
	if strings.TrimSpace(plan.ProjectID) != strings.TrimSpace(projectID) {
		return fmt.Errorf(
			"%w: task %q belongs to project %q, not %q",
			ErrA2AProjectScope,
			strings.TrimSpace(plan.ID),
			strings.TrimSpace(plan.ProjectID),
			strings.TrimSpace(projectID),
		)
	}
	return nil
}

func snapshotFromTaskPlan(plan *core.TaskPlan) *A2ATaskSnapshot {
	state := A2ATaskStateUnknown
	errMsg := ""
	if plan != nil {
		switch plan.Status {
		case core.PlanDraft:
			state = A2ATaskStateSubmitted
		case core.PlanReviewing, core.PlanApproved, core.PlanExecuting, core.PlanPartial:
			state = A2ATaskStateWorking
		case core.PlanWaitingHuman:
			state = A2ATaskStateInputRequired
			if plan.WaitReason == core.WaitParseFailed {
				errMsg = string(core.WaitParseFailed)
			}
		case core.PlanDone:
			state = A2ATaskStateCompleted
		case core.PlanFailed:
			state = A2ATaskStateFailed
		case core.PlanAbandoned:
			state = A2ATaskStateCanceled
		default:
			state = A2ATaskStateUnknown
		}
	}

	snapshot := &A2ATaskSnapshot{
		State: state,
		Error: errMsg,
	}
	if plan != nil {
		snapshot.TaskID = strings.TrimSpace(plan.ID)
		snapshot.ProjectID = strings.TrimSpace(plan.ProjectID)
		snapshot.SessionID = strings.TrimSpace(plan.SessionID)
		snapshot.UpdatedAt = plan.UpdatedAt
	}
	return snapshot
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "not found")
}
