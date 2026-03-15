package workitemtrackapp

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

type Config struct {
	Store    Store
	Tx       Tx
	Bus      EventPublisher
	Executor WorkItemExecutor
}

type Service struct {
	store    Store
	tx       Tx
	bus      EventPublisher
	executor WorkItemExecutor
}

func New(cfg Config) *Service {
	return &Service{
		store:    cfg.Store,
		tx:       cfg.Tx,
		bus:      cfg.Bus,
		executor: cfg.Executor,
	}
}

func (s *Service) StartTrack(ctx context.Context, input StartTrackInput) (*core.WorkItemTrack, error) {
	if input.ThreadID <= 0 {
		return nil, newError(CodeMissingThreadID, "thread_id is required", nil)
	}
	if _, err := s.store.GetThread(ctx, input.ThreadID); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, newError(CodeThreadNotFound, "thread not found", err)
		}
		return nil, err
	}

	title := strings.TrimSpace(input.Title)
	if title == "" {
		return nil, newError(CodeMissingTitle, "title is required", nil)
	}
	track := &core.WorkItemTrack{
		Title:           title,
		Objective:       strings.TrimSpace(input.Objective),
		Status:          core.WorkItemTrackDraft,
		PrimaryThreadID: int64Ptr(input.ThreadID),
		PlannerStatus:   "idle",
		ReviewerStatus:  "idle",
		Metadata:        cloneMetadata(input.Metadata),
		CreatedBy:       strings.TrimSpace(input.CreatedBy),
	}

	createFn := func(ctx context.Context, store TxStore) error {
		id, err := store.CreateWorkItemTrack(ctx, track)
		if err != nil {
			return err
		}
		track.ID = id
		_, err = store.AttachThreadToWorkItemTrack(ctx, &core.WorkItemTrackThread{
			TrackID:      id,
			ThreadID:     input.ThreadID,
			RelationType: core.WorkItemTrackThreadPrimary,
		})
		return err
	}

	if s.tx != nil {
		if err := s.tx.InTx(ctx, createFn); err != nil {
			return nil, err
		}
	} else if err := createFn(ctx, s.store); err != nil {
		return nil, err
	}
	s.publishTrackEvent(ctx, core.EventThreadTrackCreated, track, nil)
	return track, nil
}

func (s *Service) AttachThreadContext(ctx context.Context, input AttachThreadContextInput) (*core.WorkItemTrackThread, error) {
	if _, err := s.store.GetWorkItemTrack(ctx, input.TrackID); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, newError(CodeTrackNotFound, "track not found", err)
		}
		return nil, err
	}
	if _, err := s.store.GetThread(ctx, input.ThreadID); err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, newError(CodeThreadNotFound, "thread not found", err)
		}
		return nil, err
	}

	relation := core.WorkItemTrackThreadSource
	if strings.TrimSpace(input.RelationType) != "" {
		parsed, err := core.ParseWorkItemTrackThreadRelation(input.RelationType)
		if err != nil {
			return nil, newError(CodeInvalidRelationType, err.Error(), err)
		}
		relation = parsed
	}

	link := &core.WorkItemTrackThread{
		TrackID:      input.TrackID,
		ThreadID:     input.ThreadID,
		RelationType: relation,
	}
	id, err := s.store.AttachThreadToWorkItemTrack(ctx, link)
	if err != nil {
		return nil, err
	}
	link.ID = id
	if track, trackErr := s.store.GetWorkItemTrack(ctx, input.TrackID); trackErr == nil {
		s.publishTrackEvent(ctx, core.EventThreadTrackUpdated, track, map[string]any{
			"linked_thread_id":     input.ThreadID,
			"linked_relation_type": string(link.RelationType),
		})
	}
	return link, nil
}

func (s *Service) SubmitForReview(ctx context.Context, input SubmitForReviewInput) (*core.WorkItemTrack, error) {
	return s.updateTrack(ctx, input.TrackID, core.EventThreadTrackUpdated, func(track *core.WorkItemTrack) error {
		switch track.Status {
		case core.WorkItemTrackReviewing:
			return nil
		case core.WorkItemTrackDraft, core.WorkItemTrackPlanning, core.WorkItemTrackPaused:
		default:
			return newError(CodeInvalidState, "track cannot be submitted for review in current state", core.ErrInvalidTransition)
		}

		track.Status = core.WorkItemTrackReviewing
		track.ReviewerStatus = "pending"
		track.AwaitingUserConfirmation = false
		if summary := strings.TrimSpace(input.LatestSummary); summary != "" {
			track.LatestSummary = summary
		}
		if input.PlannerOutput != nil {
			track.PlannerOutput = cloneMetadata(input.PlannerOutput)
		}
		return nil
	})
}

func (s *Service) ApproveReview(ctx context.Context, input ApproveReviewInput) (*core.WorkItemTrack, error) {
	return s.updateTrack(ctx, input.TrackID, core.EventThreadTrackReviewApproved, func(track *core.WorkItemTrack) error {
		if track.Status == core.WorkItemTrackAwaitingConfirmation {
			return nil
		}
		if track.Status != core.WorkItemTrackReviewing {
			return newError(CodeInvalidState, "track cannot be approved in current state", core.ErrInvalidTransition)
		}

		track.Status = core.WorkItemTrackAwaitingConfirmation
		track.ReviewerStatus = "approved"
		track.AwaitingUserConfirmation = true
		if summary := strings.TrimSpace(input.LatestSummary); summary != "" {
			track.LatestSummary = summary
		}
		if input.ReviewOutput != nil {
			track.ReviewOutput = cloneMetadata(input.ReviewOutput)
		}
		return nil
	})
}

func (s *Service) RejectReview(ctx context.Context, input RejectReviewInput) (*core.WorkItemTrack, error) {
	return s.updateTrack(ctx, input.TrackID, core.EventThreadTrackReviewRejected, func(track *core.WorkItemTrack) error {
		if track.Status == core.WorkItemTrackPlanning && track.ReviewerStatus == "rejected" {
			return nil
		}
		if track.Status != core.WorkItemTrackReviewing {
			return newError(CodeInvalidState, "track cannot be rejected in current state", core.ErrInvalidTransition)
		}

		track.Status = core.WorkItemTrackPlanning
		track.ReviewerStatus = "rejected"
		track.AwaitingUserConfirmation = false
		if summary := strings.TrimSpace(input.LatestSummary); summary != "" {
			track.LatestSummary = summary
		}
		if input.ReviewOutput != nil {
			track.ReviewOutput = cloneMetadata(input.ReviewOutput)
		}
		return nil
	})
}

func (s *Service) PauseTrack(ctx context.Context, input PauseTrackInput) (*core.WorkItemTrack, error) {
	return s.updateTrack(ctx, input.TrackID, core.EventThreadTrackStateChanged, func(track *core.WorkItemTrack) error {
		if track.Status == core.WorkItemTrackPaused {
			return nil
		}
		if !core.CanTransitionWorkItemTrackStatus(track.Status, core.WorkItemTrackPaused) {
			return newError(CodeInvalidState, "track cannot be paused in current state", core.ErrInvalidTransition)
		}
		track.Status = core.WorkItemTrackPaused
		track.AwaitingUserConfirmation = false
		return nil
	})
}

func (s *Service) CancelTrack(ctx context.Context, input CancelTrackInput) (*core.WorkItemTrack, error) {
	return s.updateTrack(ctx, input.TrackID, core.EventThreadTrackStateChanged, func(track *core.WorkItemTrack) error {
		if track.Status == core.WorkItemTrackCancelled {
			return nil
		}
		if !core.CanTransitionWorkItemTrackStatus(track.Status, core.WorkItemTrackCancelled) {
			return newError(CodeInvalidState, "track cannot be cancelled in current state", core.ErrInvalidTransition)
		}
		track.Status = core.WorkItemTrackCancelled
		track.AwaitingUserConfirmation = false
		return nil
	})
}

func (s *Service) MaterializeWorkItem(ctx context.Context, input MaterializeWorkItemInput) (*MaterializeWorkItemResult, error) {
	track, err := s.store.GetWorkItemTrack(ctx, input.TrackID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, newError(CodeTrackNotFound, "track not found", err)
		}
		return nil, err
	}

	if track.WorkItemID != nil {
		workItem, err := s.store.GetWorkItem(ctx, *track.WorkItemID)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				return nil, newError(CodeWorkItemNotFound, "work item not found", err)
			}
			return nil, err
		}
		links, err := s.buildThreadLinks(ctx, track, workItem.ID)
		if err != nil {
			return nil, err
		}
		return &MaterializeWorkItemResult{Track: track, WorkItem: workItem, Links: links}, nil
	}

	if track.Status != core.WorkItemTrackAwaitingConfirmation && track.Status != core.WorkItemTrackReviewing && track.Status != core.WorkItemTrackDraft {
		return nil, newError(CodeInvalidState, "track cannot materialize in current state", core.ErrInvalidTransition)
	}

	var workItem *core.WorkItem
	var links []*core.ThreadWorkItemLink
	materializeFn := func(ctx context.Context, store TxStore) error {
		threadLinks, err := store.ListWorkItemTrackThreads(ctx, track.ID)
		if err != nil {
			return err
		}

		workItem = &core.WorkItem{
			ProjectID: input.ProjectID,
			Title:     track.Title,
			Body:      strings.TrimSpace(track.Objective),
			Status:    core.WorkItemAccepted,
			Priority:  core.PriorityMedium,
			Metadata: map[string]any{
				"source_track_id": track.ID,
			},
		}
		if track.PrimaryThreadID != nil {
			workItem.Metadata["source_thread_id"] = *track.PrimaryThreadID
		}

		workItemID, err := store.CreateWorkItem(ctx, workItem)
		if err != nil {
			return err
		}
		workItem.ID = workItemID

		links = make([]*core.ThreadWorkItemLink, 0, len(threadLinks))
		for _, threadLink := range threadLinks {
			if threadLink == nil {
				continue
			}
			link := &core.ThreadWorkItemLink{
				ThreadID:     threadLink.ThreadID,
				WorkItemID:   workItemID,
				RelationType: "related",
				IsPrimary:    threadLink.RelationType == core.WorkItemTrackThreadPrimary,
			}
			if link.IsPrimary {
				link.RelationType = "drives"
			}
			id, err := store.CreateThreadWorkItemLink(ctx, link)
			if err != nil {
				return err
			}
			link.ID = id
			links = append(links, link)
		}

		track.WorkItemID = int64Ptr(workItemID)
		track.Status = core.WorkItemTrackMaterialized
		if err := store.UpdateWorkItemTrack(ctx, track); err != nil {
			return err
		}
		return nil
	}

	if s.tx != nil {
		if err := s.tx.InTx(ctx, materializeFn); err != nil {
			return nil, err
		}
	} else if err := materializeFn(ctx, s.store); err != nil {
		return nil, err
	}

	result := &MaterializeWorkItemResult{
		Track:    track,
		WorkItem: workItem,
		Links:    links,
	}
	s.publishTrackEvent(ctx, core.EventThreadTrackMaterialized, track, map[string]any{
		"work_item_id": workItem.ID,
		"link_count":   len(links),
	})
	return result, nil
}

func (s *Service) buildThreadLinks(ctx context.Context, track *core.WorkItemTrack, workItemID int64) ([]*core.ThreadWorkItemLink, error) {
	threadLinks, err := s.store.ListWorkItemTrackThreads(ctx, track.ID)
	if err != nil {
		return nil, err
	}
	links := make([]*core.ThreadWorkItemLink, 0, len(threadLinks))
	for _, threadLink := range threadLinks {
		if threadLink == nil {
			continue
		}
		link := &core.ThreadWorkItemLink{
			ThreadID:     threadLink.ThreadID,
			WorkItemID:   workItemID,
			RelationType: "related",
			IsPrimary:    threadLink.RelationType == core.WorkItemTrackThreadPrimary,
		}
		if link.IsPrimary {
			link.RelationType = "drives"
		}
		links = append(links, link)
	}
	return links, nil
}

func (s *Service) ConfirmExecution(ctx context.Context, input ConfirmExecutionInput) (*ConfirmExecutionResult, error) {
	if s.executor == nil {
		return nil, newError(CodeExecutionUnavailable, "work item execution is not configured", nil)
	}

	materialized, err := s.MaterializeWorkItem(ctx, MaterializeWorkItemInput{
		TrackID:   input.TrackID,
		ProjectID: input.ProjectID,
	})
	if err != nil {
		return nil, err
	}

	track := materialized.Track
	workItem := materialized.WorkItem
	if workItem == nil || track == nil || track.WorkItemID == nil {
		return nil, newError(CodeWorkItemNotFound, "work item not found", core.ErrNotFound)
	}

	if err := s.ensureDefaultExecutionAction(ctx, workItem.ID, track); err != nil {
		return nil, err
	}

	status := "accepted"
	switch workItem.Status {
	case core.WorkItemQueued, core.WorkItemRunning:
		status = "already_running"
	default:
		if err := s.executor.RunWorkItem(ctx, workItem.ID); err != nil {
			return nil, err
		}
		status = "queued"
	}

	updatedTrack, err := s.updateTrack(ctx, track.ID, core.EventThreadTrackExecutionConfirmed, func(track *core.WorkItemTrack) error {
		if track.Status == core.WorkItemTrackExecuting {
			return nil
		}
		if !core.CanTransitionWorkItemTrackStatus(track.Status, core.WorkItemTrackExecuting) {
			return newError(CodeInvalidState, "track cannot enter executing state", core.ErrInvalidTransition)
		}
		track.Status = core.WorkItemTrackExecuting
		track.AwaitingUserConfirmation = false
		return nil
	})
	if err != nil {
		return nil, err
	}

	refreshedWorkItem, err := s.store.GetWorkItem(ctx, workItem.ID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, newError(CodeWorkItemNotFound, "work item not found", err)
		}
		return nil, err
	}

	return &ConfirmExecutionResult{
		Track:    updatedTrack,
		WorkItem: refreshedWorkItem,
		Status:   status,
	}, nil
}

func (s *Service) SyncTrackStatusFromWorkItem(ctx context.Context, workItemID int64, workItemStatus core.WorkItemStatus) ([]*core.WorkItemTrack, error) {
	target, ok := mapTrackStatusFromWorkItem(workItemStatus)
	if !ok {
		return nil, nil
	}

	tracks, err := s.store.ListWorkItemTracksByWorkItem(ctx, workItemID)
	if err != nil {
		return nil, err
	}
	updated := make([]*core.WorkItemTrack, 0, len(tracks))
	for _, track := range tracks {
		if track == nil {
			continue
		}
		next, changed, err := s.applyTrackUpdate(ctx, track.ID, core.EventThreadTrackStateChanged, func(track *core.WorkItemTrack) error {
			if track.Status == target {
				return nil
			}
			if !core.CanTransitionWorkItemTrackStatus(track.Status, target) {
				return nil
			}
			track.Status = target
			track.AwaitingUserConfirmation = false
			return nil
		})
		if err != nil {
			return nil, err
		}
		if changed && next != nil {
			updated = append(updated, next)
		}
	}
	return updated, nil
}

func (s *Service) ensureDefaultExecutionAction(ctx context.Context, workItemID int64, track *core.WorkItemTrack) error {
	actions, err := s.store.ListActionsByWorkItem(ctx, workItemID)
	if err != nil {
		return err
	}
	if len(actions) > 0 {
		return nil
	}

	input := strings.TrimSpace(track.Objective)
	if input == "" {
		input = strings.TrimSpace(track.Title)
	}

	action := &core.Action{
		WorkItemID:  workItemID,
		Name:        "execute-track",
		Description: strings.TrimSpace(track.Objective),
		Type:        core.ActionExec,
		Status:      core.ActionPending,
		Position:    0,
		Input:       input,
		AgentRole:   "worker",
		MaxRetries:  1,
	}
	_, err = s.store.CreateAction(ctx, action)
	return err
}

func (s *Service) updateTrack(ctx context.Context, trackID int64, eventType core.EventType, mutate func(track *core.WorkItemTrack) error) (*core.WorkItemTrack, error) {
	track, _, err := s.applyTrackUpdate(ctx, trackID, eventType, mutate)
	return track, err
}

func (s *Service) applyTrackUpdate(ctx context.Context, trackID int64, eventType core.EventType, mutate func(track *core.WorkItemTrack) error) (*core.WorkItemTrack, bool, error) {
	track, err := s.store.GetWorkItemTrack(ctx, trackID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, false, newError(CodeTrackNotFound, "track not found", err)
		}
		return nil, false, err
	}

	before := snapshotTrack(track)
	if err := mutate(track); err != nil {
		return nil, false, err
	}
	changed := trackChanged(before, track)
	if !changed {
		return track, false, nil
	}

	saveFn := func(ctx context.Context, updater interface {
		UpdateWorkItemTrack(ctx context.Context, track *core.WorkItemTrack) error
	}) error {
		return updater.UpdateWorkItemTrack(ctx, track)
	}
	if s.tx != nil {
		if err := s.tx.InTx(ctx, func(ctx context.Context, store TxStore) error {
			return saveFn(ctx, store)
		}); err != nil {
			return nil, false, err
		}
	} else if err := saveFn(ctx, s.store); err != nil {
		return nil, false, err
	}

	extra := map[string]any{}
	if before.Status != track.Status {
		extra["previous_status"] = string(before.Status)
	}
	if eventType != "" {
		s.publishTrackEvent(ctx, eventType, track, extra)
	}
	if before.Status != track.Status && eventType != core.EventThreadTrackStateChanged {
		s.publishTrackEvent(ctx, core.EventThreadTrackStateChanged, track, extra)
	}
	return track, true, nil
}

func (s *Service) publishTrackEvent(ctx context.Context, eventType core.EventType, track *core.WorkItemTrack, extra map[string]any) {
	if s.bus == nil || track == nil || eventType == "" {
		return
	}

	threadLinks, err := s.store.ListWorkItemTrackThreads(ctx, track.ID)
	if err != nil {
		return
	}

	threadIDs := make(map[int64]struct{}, len(threadLinks)+1)
	for _, link := range threadLinks {
		if link == nil || link.ThreadID <= 0 {
			continue
		}
		threadIDs[link.ThreadID] = struct{}{}
	}
	if track.PrimaryThreadID != nil && *track.PrimaryThreadID > 0 {
		threadIDs[*track.PrimaryThreadID] = struct{}{}
	}

	for threadID := range threadIDs {
		data := map[string]any{
			"thread_id": threadID,
			"track_id":  track.ID,
			"status":    string(track.Status),
			"title":     track.Title,
			"objective": track.Objective,
			"track":     cloneTrack(track),
		}
		if track.WorkItemID != nil {
			data["work_item_id"] = *track.WorkItemID
		}
		for k, v := range extra {
			data[k] = v
		}
		s.bus.Publish(ctx, core.Event{
			Type:      eventType,
			Data:      data,
			Timestamp: time.Now().UTC(),
		})
	}
}

func snapshotTrack(track *core.WorkItemTrack) *core.WorkItemTrack {
	return cloneTrack(track)
}

func cloneTrack(track *core.WorkItemTrack) *core.WorkItemTrack {
	if track == nil {
		return nil
	}
	cloned := *track
	if track.PrimaryThreadID != nil {
		cloned.PrimaryThreadID = int64Ptr(*track.PrimaryThreadID)
	}
	if track.WorkItemID != nil {
		cloned.WorkItemID = int64Ptr(*track.WorkItemID)
	}
	cloned.PlannerOutput = cloneMetadata(track.PlannerOutput)
	cloned.ReviewOutput = cloneMetadata(track.ReviewOutput)
	cloned.Metadata = cloneMetadata(track.Metadata)
	return &cloned
}

func trackChanged(before, after *core.WorkItemTrack) bool {
	if before == nil || after == nil {
		return before != after
	}
	if before.Title != after.Title ||
		before.Objective != after.Objective ||
		before.Status != after.Status ||
		before.PlannerStatus != after.PlannerStatus ||
		before.ReviewerStatus != after.ReviewerStatus ||
		before.AwaitingUserConfirmation != after.AwaitingUserConfirmation ||
		before.LatestSummary != after.LatestSummary ||
		before.CreatedBy != after.CreatedBy {
		return true
	}
	if !sameInt64Ptr(before.PrimaryThreadID, after.PrimaryThreadID) || !sameInt64Ptr(before.WorkItemID, after.WorkItemID) {
		return true
	}
	if !reflect.DeepEqual(before.PlannerOutput, after.PlannerOutput) ||
		!reflect.DeepEqual(before.ReviewOutput, after.ReviewOutput) ||
		!reflect.DeepEqual(before.Metadata, after.Metadata) {
		return true
	}
	return false
}

func sameInt64Ptr(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func mapTrackStatusFromWorkItem(status core.WorkItemStatus) (core.WorkItemTrackStatus, bool) {
	switch status {
	case core.WorkItemQueued, core.WorkItemRunning:
		return core.WorkItemTrackExecuting, true
	case core.WorkItemDone:
		return core.WorkItemTrackDone, true
	case core.WorkItemFailed:
		return core.WorkItemTrackFailed, true
	default:
		return "", false
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

func int64Ptr(v int64) *int64 {
	return &v
}
