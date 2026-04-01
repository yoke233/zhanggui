package workitemapp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yoke233/zhanggui/internal/core"
)

type initiativeMembershipReader interface {
	ListInitiativeItemsByWorkItem(ctx context.Context, workItemID int64) ([]*core.InitiativeItem, error)
}

type Config struct {
	Store             Store
	Tx                Tx
	Registry          core.AgentRegistry
	Scheduler         Scheduler
	Runner            Runner
	Bus               EventPublisher
	BootstrapPR       Bootstrapper
	BackgroundContext context.Context
}

type Service struct {
	store         Store
	tx            Tx
	registry      core.AgentRegistry
	scheduler     Scheduler
	runner        Runner
	bus           EventPublisher
	bootstrapPR   Bootstrapper
	backgroundCtx context.Context
}

func New(cfg Config) *Service {
	return &Service{
		store:         cfg.Store,
		tx:            cfg.Tx,
		registry:      cfg.Registry,
		scheduler:     cfg.Scheduler,
		runner:        cfg.Runner,
		bus:           cfg.Bus,
		bootstrapPR:   cfg.BootstrapPR,
		backgroundCtx: cfg.BackgroundContext,
	}
}

func (s *Service) CreateWorkItem(ctx context.Context, input CreateWorkItemInput) (*core.WorkItem, error) {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return nil, newError(CodeMissingTitle, "title is required", nil)
	}
	if err := s.validateProject(ctx, input.ProjectID); err != nil {
		return nil, err
	}
	if err := s.validateResourceSpace(ctx, input.ProjectID, input.ResourceSpaceID); err != nil {
		return nil, err
	}
	if err := s.validateDependencies(ctx, 0, input.ProjectID, input.DependsOn); err != nil {
		return nil, err
	}

	priority := core.WorkItemPriority(strings.TrimSpace(input.Priority))
	if priority == "" {
		priority = core.PriorityMedium
	}

	workItem := &core.WorkItem{
		ProjectID:          input.ProjectID,
		ResourceSpaceID:    input.ResourceSpaceID,
		ParentWorkItemID:   input.ParentWorkItemID,
		RootWorkItemID:     input.RootWorkItemID,
		FinalDeliverableID: input.FinalDeliverableID,
		Title:              title,
		Body:               strings.TrimSpace(input.Body),
		Status:             core.WorkItemOpen,
		Priority:           priority,
		ExecutorProfileID:  strings.TrimSpace(input.ExecutorProfileID),
		ReviewerProfileID:  strings.TrimSpace(input.ReviewerProfileID),
		ActiveProfileID:    strings.TrimSpace(input.ActiveProfileID),
		SponsorProfileID:   strings.TrimSpace(input.SponsorProfileID),
		CreatedByProfileID: strings.TrimSpace(input.CreatedByProfileID),
		Labels:             cloneStrings(input.Labels),
		DependsOn:          cloneInt64s(input.DependsOn),
		EscalationPath:     cloneStrings(input.EscalationPath),
		Metadata:           cloneMetadata(input.Metadata),
	}
	if err := s.applyReportingChainDefaults(ctx, workItem); err != nil {
		return nil, err
	}

	id, err := s.store.CreateWorkItem(ctx, workItem)
	if err != nil {
		return nil, err
	}
	workItem.ID = id

	if s.bootstrapPR != nil {
		if err := s.bootstrapPR.BootstrapPRWorkItem(ctx, workItem.ID); err != nil {
			if rollbackErr := s.deleteAggregate(ctx, workItem.ID); rollbackErr != nil {
				return nil, newError(CodeBootstrapPRFailed, fmt.Sprintf("%s; rollback failed: %v", err.Error(), rollbackErr), err)
			}
			return nil, newError(CodeBootstrapPRFailed, err.Error(), err)
		}
	}

	return workItem, nil
}

func (s *Service) UpdateWorkItem(ctx context.Context, input UpdateWorkItemInput) (*core.WorkItem, error) {
	workItem, err := s.store.GetWorkItem(ctx, input.ID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, newError(CodeWorkItemNotFound, "work item not found", err)
		}
		return nil, err
	}

	if input.ProjectID != nil {
		if err := s.validateProject(ctx, input.ProjectID); err != nil {
			return nil, err
		}
		workItem.ProjectID = input.ProjectID
	}

	targetProjectID := workItem.ProjectID
	if input.ProjectID != nil {
		targetProjectID = input.ProjectID
	}

	if input.ResourceSpaceID != nil {
		if err := s.validateResourceSpace(ctx, targetProjectID, input.ResourceSpaceID); err != nil {
			return nil, err
		}
		workItem.ResourceSpaceID = input.ResourceSpaceID
	}
	if input.ParentWorkItemID != nil {
		workItem.ParentWorkItemID = input.ParentWorkItemID
	}
	if input.RootWorkItemID != nil {
		workItem.RootWorkItemID = input.RootWorkItemID
	}
	if input.FinalDeliverableID != nil {
		workItem.FinalDeliverableID = input.FinalDeliverableID
	}
	if input.Title != nil {
		workItem.Title = strings.TrimSpace(*input.Title)
		if workItem.Title == "" {
			return nil, newError(CodeMissingTitle, "title is required", nil)
		}
	}
	if input.Body != nil {
		workItem.Body = strings.TrimSpace(*input.Body)
	}
	if input.Status != nil {
		workItem.Status = core.WorkItemStatus(strings.TrimSpace(*input.Status))
	}
	if input.Priority != nil {
		workItem.Priority = core.WorkItemPriority(strings.TrimSpace(*input.Priority))
	}
	if input.ExecutorProfileID != nil {
		workItem.ExecutorProfileID = strings.TrimSpace(*input.ExecutorProfileID)
	}
	if input.ReviewerProfileID != nil {
		workItem.ReviewerProfileID = strings.TrimSpace(*input.ReviewerProfileID)
	}
	if input.ActiveProfileID != nil {
		workItem.ActiveProfileID = strings.TrimSpace(*input.ActiveProfileID)
	}
	if input.SponsorProfileID != nil {
		workItem.SponsorProfileID = strings.TrimSpace(*input.SponsorProfileID)
	}
	if input.CreatedByProfileID != nil {
		workItem.CreatedByProfileID = strings.TrimSpace(*input.CreatedByProfileID)
	}
	if input.Labels != nil {
		workItem.Labels = cloneStrings(*input.Labels)
	}
	if input.DependsOn != nil {
		if err := s.validateDependencies(ctx, workItem.ID, targetProjectID, *input.DependsOn); err != nil {
			return nil, err
		}
		workItem.DependsOn = cloneInt64s(*input.DependsOn)
	}
	if input.EscalationPath != nil {
		workItem.EscalationPath = cloneStrings(*input.EscalationPath)
	}
	if input.Metadata != nil {
		workItem.Metadata = cloneMetadata(input.Metadata)
	}
	if err := s.applyReportingChainDefaults(ctx, workItem); err != nil {
		return nil, err
	}

	if err := s.store.UpdateWorkItem(ctx, workItem); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, newError(CodeWorkItemNotFound, "work item not found", err)
		}
		return nil, err
	}
	return workItem, nil
}

func (s *Service) DeleteWorkItem(ctx context.Context, workItemID int64) error {
	if _, err := s.store.GetWorkItem(ctx, workItemID); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return newError(CodeWorkItemNotFound, "work item not found", err)
		}
		return err
	}
	return s.deleteAggregate(ctx, workItemID)
}

func (s *Service) SetArchived(ctx context.Context, workItemID int64, archived bool) (*core.WorkItem, error) {
	workItem, err := s.store.GetWorkItem(ctx, workItemID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, newError(CodeWorkItemNotFound, "work item not found", err)
		}
		return nil, err
	}
	if archived {
		switch workItem.Status {
		case core.WorkItemQueued, core.WorkItemInExecution, core.WorkItemPendingReview, core.WorkItemEscalated:
			return nil, newError(CodeInvalidState, "active work item cannot be archived", core.ErrInvalidTransition)
		}
	}

	if err := s.store.SetWorkItemArchived(ctx, workItemID, archived); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, newError(CodeWorkItemNotFound, "work item not found", err)
		}
		if errors.Is(err, core.ErrInvalidTransition) {
			return nil, newError(CodeInvalidState, "work item cannot be archived in current state", err)
		}
		return nil, err
	}
	return s.store.GetWorkItem(ctx, workItemID)
}

func (s *Service) RunWorkItem(ctx context.Context, workItemID int64) (*RunWorkItemResult, error) {
	actions, err := s.store.ListActionsByWorkItem(ctx, workItemID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, newError(CodeWorkItemNotFound, "work item not found", err)
		}
		return nil, err
	}
	if len(actions) == 0 {
		return nil, newError(CodeNoActions, "work item has no actions; add at least one action before running", nil)
	}
	if err := s.resetRetryableActions(ctx, actions); err != nil {
		return nil, err
	}

	if s.scheduler != nil {
		if err := s.scheduler.Submit(ctx, workItemID); err != nil {
			return nil, mapRunError(err)
		}
		return &RunWorkItemResult{
			Queued:  true,
			Message: "work item queued for run",
		}, nil
	}

	if err := s.store.PrepareWorkItemRun(ctx, workItemID, core.WorkItemQueued); err != nil {
		return nil, mapRunError(err)
	}
	if s.runner == nil {
		return nil, fmt.Errorf("runner is not configured")
	}
	go s.runInBackground(s.backgroundContext(), workItemID)

	return &RunWorkItemResult{
		Queued:  false,
		Message: "work item run started",
	}, nil
}

func (s *Service) CancelWorkItem(ctx context.Context, workItemID int64) error {
	var err error
	if s.scheduler != nil {
		err = s.scheduler.Cancel(ctx, workItemID)
	} else {
		if s.runner == nil {
			return fmt.Errorf("runner is not configured")
		}
		err = s.runner.Cancel(ctx, workItemID)
	}
	if err == nil {
		return nil
	}
	if errors.Is(err, core.ErrInvalidTransition) {
		return newError(CodeInvalidState, "work item cannot be cancelled in current state", err)
	}
	if errors.Is(err, core.ErrNotFound) {
		return newError(CodeWorkItemNotFound, "work item not found", err)
	}
	return err
}

func (s *Service) runInBackground(ctx context.Context, workItemID int64) {
	if err := s.runner.Run(ctx, workItemID); err != nil && s.bus != nil {
		s.bus.Publish(ctx, core.Event{
			Type:       core.EventWorkItemFailed,
			WorkItemID: workItemID,
			Timestamp:  time.Now().UTC(),
			Data:       map[string]any{"error": err.Error()},
		})
	}
}

func (s *Service) backgroundContext() context.Context {
	if s != nil && s.backgroundCtx != nil {
		return s.backgroundCtx
	}
	return context.Background()
}

func (s *Service) resetRetryableActions(ctx context.Context, actions []*core.Action) error {
	for _, action := range actions {
		if action == nil {
			continue
		}
		switch action.Status {
		case core.ActionFailed, core.ActionBlocked:
			action.Status = core.ActionPending
			if err := s.store.UpdateAction(ctx, action); err != nil {
				if errors.Is(err, core.ErrNotFound) {
					return newError(CodeWorkItemNotFound, "action not found while resetting for rerun", err)
				}
				return err
			}
		}
	}
	return nil
}

func (s *Service) deleteAggregate(ctx context.Context, workItemID int64) error {
	if s.tx != nil {
		return s.tx.InTx(ctx, func(ctx context.Context, txStore TxStore) error {
			return deleteAggregateData(ctx, txStore, workItemID)
		})
	}
	return deleteAggregateData(ctx, s.store, workItemID)
}

func deleteAggregateData(ctx context.Context, store TxStore, workItemID int64) error {
	if err := store.DetachFeatureEntriesByWorkItem(ctx, workItemID); err != nil {
		return err
	}
	if err := store.DeleteJournalByWorkItem(ctx, workItemID); err != nil {
		return err
	}
	if err := store.DeleteEventsByWorkItem(ctx, workItemID); err != nil {
		return err
	}
	if err := store.DeleteAgentContextsByWorkItem(ctx, workItemID); err != nil {
		return err
	}
	if err := store.DeleteActionSignalsByWorkItem(ctx, workItemID); err != nil {
		return err
	}
	if err := store.DeleteRunsByWorkItem(ctx, workItemID); err != nil {
		return err
	}
	if err := store.DeleteResourcesByWorkItem(ctx, workItemID); err != nil {
		return err
	}
	if err := store.DeleteActionIODeclsByWorkItem(ctx, workItemID); err != nil {
		return err
	}
	if err := store.DeleteThreadWorkItemLinksByWorkItem(ctx, workItemID); err != nil {
		return err
	}
	if err := store.DeleteActionsByWorkItem(ctx, workItemID); err != nil {
		return err
	}
	if err := store.DeleteWorkItem(ctx, workItemID); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return newError(CodeWorkItemNotFound, "work item not found", err)
		}
		return err
	}
	return nil
}

func (s *Service) validateProject(ctx context.Context, projectID *int64) error {
	if projectID == nil {
		return nil
	}
	if _, err := s.store.GetProject(ctx, *projectID); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return newError(CodeProjectNotFound, "project not found", err)
		}
		return err
	}
	return nil
}

func (s *Service) validateResourceSpace(ctx context.Context, projectID *int64, spaceID *int64) error {
	if spaceID == nil {
		return nil
	}
	if projectID == nil {
		return newError(CodeInvalidResourceSpace, "resource space requires project_id", nil)
	}
	space, err := s.store.GetResourceSpace(ctx, *spaceID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return newError(CodeResourceSpaceNotFound, "resource space not found", err)
		}
		return err
	}
	if space.ProjectID != *projectID {
		return newError(CodeInvalidResourceSpace, fmt.Sprintf("resource space %d does not belong to project %d", *spaceID, *projectID), nil)
	}
	return nil
}

func (s *Service) validateDependencies(ctx context.Context, workItemID int64, projectID *int64, deps []int64) error {
	seen := make(map[int64]struct{}, len(deps))
	for _, depID := range deps {
		if depID <= 0 {
			return newError(CodeInvalidWorkItemDependency, "dependency work item id must be positive", nil)
		}
		if depID == workItemID && workItemID != 0 {
			return newError(CodeInvalidWorkItemDependency, "work item cannot depend on itself", nil)
		}
		if _, ok := seen[depID]; ok {
			return newError(CodeInvalidWorkItemDependency, fmt.Sprintf("duplicate dependency work item id %d", depID), nil)
		}
		seen[depID] = struct{}{}

		depWorkItem, err := s.store.GetWorkItem(ctx, depID)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				return newError(CodeWorkItemDependencyNotFound, "dependency work item not found", err)
			}
			return err
		}
		if projectID != nil && depWorkItem.ProjectID != nil && *depWorkItem.ProjectID != *projectID {
			allowed, allowErr := s.allowCrossProjectDependency(ctx, workItemID, depID)
			if allowErr != nil {
				return allowErr
			}
			if !allowed {
				return newError(CodeInvalidWorkItemDependency, fmt.Sprintf("dependency work item %d belongs to a different project; cross-project dependencies require both work items to belong to the same initiative", depID), nil)
			}
		}
	}
	return nil
}

func (s *Service) allowCrossProjectDependency(ctx context.Context, workItemID int64, depID int64) (bool, error) {
	reader, ok := s.store.(initiativeMembershipReader)
	if !ok || workItemID == 0 {
		return false, nil
	}
	currentItems, err := reader.ListInitiativeItemsByWorkItem(ctx, workItemID)
	if err != nil {
		return false, err
	}
	if len(currentItems) == 0 {
		return false, nil
	}
	depItems, err := reader.ListInitiativeItemsByWorkItem(ctx, depID)
	if err != nil {
		return false, err
	}
	if len(depItems) == 0 {
		return false, nil
	}
	currentInitiatives := make(map[int64]struct{}, len(currentItems))
	for _, item := range currentItems {
		if item == nil || item.InitiativeID <= 0 {
			continue
		}
		currentInitiatives[item.InitiativeID] = struct{}{}
	}
	for _, item := range depItems {
		if item == nil {
			continue
		}
		if _, ok := currentInitiatives[item.InitiativeID]; ok {
			return true, nil
		}
	}
	return false, nil
}

func mapRunError(err error) error {
	switch {
	case errors.Is(err, core.ErrNotFound):
		return newError(CodeWorkItemNotFound, "work item not found", err)
	case errors.Is(err, core.ErrInvalidTransition):
		return newError(CodeInvalidState, "work item is not in a runnable state", err)
	default:
		return err
	}
}

func cloneMetadata(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneInt64s(in []int64) []int64 {
	if len(in) == 0 {
		return nil
	}
	out := make([]int64, len(in))
	copy(out, in)
	return out
}

func (s *Service) applyReportingChainDefaults(ctx context.Context, workItem *core.WorkItem) error {
	if s == nil || s.registry == nil || workItem == nil {
		return nil
	}

	if workItem.ReviewerProfileID == "" && workItem.ExecutorProfileID != "" {
		reviewer, err := DefaultReviewerProfileID(ctx, workItem.ExecutorProfileID, s.registry)
		if err != nil {
			return err
		}
		workItem.ReviewerProfileID = reviewer
	}
	if workItem.ActiveProfileID == "" {
		workItem.ActiveProfileID = firstNonEmpty(workItem.ExecutorProfileID, workItem.ReviewerProfileID)
	}
	if len(workItem.EscalationPath) == 0 && workItem.ActiveProfileID != "" {
		path, err := BuildEscalationPath(ctx, workItem.ActiveProfileID, s.registry)
		if err != nil {
			return err
		}
		workItem.EscalationPath = path
	}
	if workItem.SponsorProfileID == "" {
		workItem.SponsorProfileID = resolveSponsorProfileID(workItem.EscalationPath, workItem.ReviewerProfileID)
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func resolveSponsorProfileID(escalationPath []string, reviewerProfileID string) string {
	for i := len(escalationPath) - 1; i >= 0; i-- {
		candidate := strings.TrimSpace(escalationPath[i])
		if candidate != "" && candidate != HumanEscalationTarget {
			return candidate
		}
	}
	return strings.TrimSpace(reviewerProfileID)
}
