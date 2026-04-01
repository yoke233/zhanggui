package workitemapp

import (
	"context"
	"errors"
	"time"

	"github.com/yoke233/zhanggui/internal/core"
)

func (s *Service) AdoptDeliverable(ctx context.Context, workItemID, deliverableID int64) (*core.WorkItem, error) {
	store := s.store
	var (
		workItem        *core.WorkItem
		statusCompleted bool
		err             error
	)
	if s.tx != nil {
		err = s.tx.InTx(ctx, func(ctx context.Context, txStore TxStore) error {
			store = txStore
			workItem, statusCompleted, err = adoptDeliverableInStore(ctx, txStore, workItemID, deliverableID)
			return err
		})
	} else {
		workItem, statusCompleted, err = adoptDeliverableInStore(ctx, store, workItemID, deliverableID)
	}
	if err != nil {
		return nil, err
	}
	if statusCompleted && s != nil && s.bus != nil {
		s.bus.Publish(ctx, core.Event{
			Type:       core.EventWorkItemCompleted,
			WorkItemID: workItem.ID,
			Timestamp:  time.Now().UTC(),
		})
	}
	return workItem, nil
}

func adoptDeliverableInStore(ctx context.Context, store Store, workItemID, deliverableID int64) (*core.WorkItem, bool, error) {
	workItem, err := store.GetWorkItem(ctx, workItemID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, false, newError(CodeWorkItemNotFound, "work item not found", err)
		}
		return nil, false, err
	}

	deliverable, err := store.GetDeliverable(ctx, deliverableID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, false, newError(CodeDeliverableNotFound, "deliverable not found", err)
		}
		return nil, false, err
	}
	if err := validateDeliverableForAdoption(ctx, store, workItemID, deliverable); err != nil {
		return nil, false, err
	}

	actions, err := store.ListActionsByWorkItem(ctx, workItemID)
	if err != nil {
		return nil, false, err
	}
	for _, action := range actions {
		if action == nil {
			continue
		}
		if action.Status == core.ActionRunning {
			return nil, false, newError(CodeInvalidState, "cannot adopt deliverable while actions are running", core.ErrInvalidTransition)
		}
		if !isActionTerminal(action.Status) {
			action.Status = core.ActionCancelled
			if err := store.UpdateAction(ctx, action); err != nil {
				if errors.Is(err, core.ErrNotFound) {
					return nil, false, newError(CodeWorkItemNotFound, "action not found while closing work item", err)
				}
				return nil, false, err
			}
		}
	}

	wasCompleted := workItem.Status == core.WorkItemCompleted
	workItem.FinalDeliverableID = &deliverable.ID
	if workItem.Status != core.WorkItemCancelled {
		workItem.Status = core.WorkItemCompleted
	}
	if err := store.UpdateWorkItem(ctx, workItem); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, false, newError(CodeWorkItemNotFound, "work item not found", err)
		}
		return nil, false, err
	}
	return workItem, workItem.Status == core.WorkItemCompleted && !wasCompleted, nil
}

func validateDeliverableForAdoption(ctx context.Context, store Store, workItemID int64, deliverable *core.Deliverable) error {
	if deliverable == nil {
		return newError(CodeDeliverableNotFound, "deliverable not found", core.ErrNotFound)
	}
	if deliverable.Status != core.DeliverableFinal {
		return newError(CodeInvalidState, "only final deliverables can be adopted", core.ErrInvalidTransition)
	}
	if !deliverable.HasContent() {
		return newError(CodeInvalidState, "deliverable must include content before adoption", core.ErrInvalidTransition)
	}
	if deliverable.WorkItemID != nil {
		if *deliverable.WorkItemID != workItemID {
			return newError(CodeInvalidState, "deliverable belongs to a different work item", core.ErrInvalidTransition)
		}
		return nil
	}
	if deliverable.ThreadID == nil {
		return newError(CodeInvalidState, "deliverable is not attached to the work item", core.ErrInvalidTransition)
	}

	links, err := store.ListThreadsByWorkItem(ctx, workItemID)
	if err != nil {
		return err
	}
	for _, link := range links {
		if link == nil {
			continue
		}
		if link.ThreadID == *deliverable.ThreadID {
			return nil
		}
	}
	return newError(CodeInvalidState, "deliverable thread is not linked to the work item", core.ErrInvalidTransition)
}

func isActionTerminal(status core.ActionStatus) bool {
	switch status {
	case core.ActionDone, core.ActionCancelled:
		return true
	default:
		return false
	}
}

func (s *Service) ListDeliverables(ctx context.Context, workItemID int64) ([]*core.Deliverable, error) {
	workItem, err := s.store.GetWorkItem(ctx, workItemID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, newError(CodeWorkItemNotFound, "work item not found", err)
		}
		return nil, err
	}

	items, err := s.store.ListDeliverablesByWorkItem(ctx, workItemID)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 && workItem.FinalDeliverableID == nil {
		return []*core.Deliverable{}, nil
	}

	result := make([]*core.Deliverable, 0, len(items)+1)
	seen := make(map[int64]struct{}, len(items)+1)
	appendUnique := func(item *core.Deliverable) {
		if item == nil {
			return
		}
		if _, exists := seen[item.ID]; exists {
			return
		}
		seen[item.ID] = struct{}{}
		result = append(result, item)
	}

	if workItem.FinalDeliverableID != nil {
		finalDeliverable, err := s.store.GetDeliverable(ctx, *workItem.FinalDeliverableID)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				return nil, newError(CodeDeliverableNotFound, "deliverable not found", err)
			}
			return nil, err
		}
		appendUnique(finalDeliverable)
	}
	for _, item := range items {
		appendUnique(item)
	}
	return result, nil
}
