package sqlite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"gorm.io/gorm"
)

func (s *Store) CreateWorkItemTrack(ctx context.Context, track *core.WorkItemTrack) (int64, error) {
	if s == nil || s.orm == nil {
		return 0, fmt.Errorf("store is not initialized")
	}
	if track == nil {
		return 0, fmt.Errorf("work item track is nil")
	}

	title := strings.TrimSpace(track.Title)
	if title == "" {
		return 0, fmt.Errorf("title is required")
	}
	if track.Status == "" {
		track.Status = core.WorkItemTrackDraft
	}
	if !track.Status.Valid() {
		return 0, fmt.Errorf("invalid work item track status %q", track.Status)
	}

	now := time.Now().UTC()
	model := workItemTrackModelFromCore(track)
	model.Title = title
	model.CreatedAt = now
	model.UpdatedAt = now
	if model.PlannerStatus == "" {
		model.PlannerStatus = "idle"
	}
	if model.ReviewerStatus == "" {
		model.ReviewerStatus = "idle"
	}

	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, err
	}
	track.ID = model.ID
	track.Title = title
	track.CreatedAt = now
	track.UpdatedAt = now
	if track.PlannerStatus == "" {
		track.PlannerStatus = "idle"
	}
	if track.ReviewerStatus == "" {
		track.ReviewerStatus = "idle"
	}
	return model.ID, nil
}

func (s *Store) GetWorkItemTrack(ctx context.Context, id int64) (*core.WorkItemTrack, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}

	var model WorkItemTrackModel
	err := s.orm.WithContext(ctx).First(&model, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, err
	}
	return model.toCore(), nil
}

func (s *Store) ListWorkItemTracks(ctx context.Context, filter core.WorkItemTrackFilter) ([]*core.WorkItemTrack, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}

	query := s.orm.WithContext(ctx).Model(&WorkItemTrackModel{})
	if filter.Status != nil {
		query = query.Where("status = ?", string(*filter.Status))
	}
	if filter.PrimaryThreadID != nil {
		query = query.Where("primary_thread_id = ?", *filter.PrimaryThreadID)
	}
	if filter.WorkItemID != nil {
		query = query.Where("work_item_id = ?", *filter.WorkItemID)
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	var models []WorkItemTrackModel
	if err := query.Order("id DESC").Limit(limit).Offset(offset).Find(&models).Error; err != nil {
		return nil, err
	}

	out := make([]*core.WorkItemTrack, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) UpdateWorkItemTrack(ctx context.Context, track *core.WorkItemTrack) error {
	if s == nil || s.orm == nil {
		return fmt.Errorf("store is not initialized")
	}
	if track == nil {
		return fmt.Errorf("work item track is nil")
	}

	title := strings.TrimSpace(track.Title)
	if title == "" {
		return fmt.Errorf("title is required")
	}
	if !track.Status.Valid() {
		return fmt.Errorf("invalid work item track status %q", track.Status)
	}

	now := time.Now().UTC()
	model := workItemTrackModelFromCore(track)
	model.Title = title
	model.UpdatedAt = now
	if model.PlannerStatus == "" {
		model.PlannerStatus = "idle"
	}
	if model.ReviewerStatus == "" {
		model.ReviewerStatus = "idle"
	}

	result := s.orm.WithContext(ctx).Model(&WorkItemTrackModel{}).
		Where("id = ?", track.ID).
		Updates(map[string]any{
			"title":                      model.Title,
			"objective":                  model.Objective,
			"status":                     model.Status,
			"primary_thread_id":          model.PrimaryThreadID,
			"work_item_id":               model.WorkItemID,
			"planner_status":             model.PlannerStatus,
			"reviewer_status":            model.ReviewerStatus,
			"awaiting_user_confirmation": model.AwaitingUserConfirmation,
			"latest_summary":             model.LatestSummary,
			"planner_output_json":        model.PlannerOutput,
			"review_output_json":         model.ReviewOutput,
			"metadata_json":              model.Metadata,
			"created_by":                 model.CreatedBy,
			"updated_at":                 model.UpdatedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	track.Title = title
	track.UpdatedAt = now
	return nil
}

func (s *Store) UpdateWorkItemTrackStatus(ctx context.Context, id int64, status core.WorkItemTrackStatus) error {
	if s == nil || s.orm == nil {
		return fmt.Errorf("store is not initialized")
	}
	if !status.Valid() {
		return fmt.Errorf("invalid work item track status %q", status)
	}

	result := s.orm.WithContext(ctx).Model(&WorkItemTrackModel{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":     string(status),
			"updated_at": time.Now().UTC(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (s *Store) AttachThreadToWorkItemTrack(ctx context.Context, link *core.WorkItemTrackThread) (int64, error) {
	if s == nil || s.orm == nil {
		return 0, fmt.Errorf("store is not initialized")
	}
	if link == nil {
		return 0, fmt.Errorf("work item track thread is nil")
	}
	if link.TrackID <= 0 || link.ThreadID <= 0 {
		return 0, fmt.Errorf("track_id and thread_id are required")
	}
	if link.RelationType == "" {
		link.RelationType = core.WorkItemTrackThreadSource
	}
	if !link.RelationType.Valid() {
		return 0, fmt.Errorf("invalid work item track thread relation %q", link.RelationType)
	}

	now := time.Now().UTC()
	model := workItemTrackThreadModelFromCore(link)
	model.CreatedAt = now

	if err := s.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(model).Error; err != nil {
			return err
		}
		if link.RelationType == core.WorkItemTrackThreadPrimary {
			return tx.Model(&WorkItemTrackModel{}).
				Where("id = ?", link.TrackID).
				Updates(map[string]any{
					"primary_thread_id": link.ThreadID,
					"updated_at":        now,
				}).Error
		}
		return nil
	}); err != nil {
		return 0, err
	}

	link.ID = model.ID
	link.CreatedAt = now
	return model.ID, nil
}

func (s *Store) ListWorkItemTrackThreads(ctx context.Context, trackID int64) ([]*core.WorkItemTrackThread, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}

	var models []WorkItemTrackThreadModel
	if err := s.orm.WithContext(ctx).
		Where("track_id = ?", trackID).
		Order("id ASC").
		Find(&models).Error; err != nil {
		return nil, err
	}

	out := make([]*core.WorkItemTrackThread, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) ListWorkItemTracksByThread(ctx context.Context, threadID int64) ([]*core.WorkItemTrack, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}

	var models []WorkItemTrackModel
	if err := s.orm.WithContext(ctx).
		Model(&WorkItemTrackModel{}).
		Joins("JOIN work_item_track_threads ON work_item_track_threads.track_id = work_item_tracks.id").
		Where("work_item_track_threads.thread_id = ?", threadID).
		Order("work_item_tracks.id DESC").
		Find(&models).Error; err != nil {
		return nil, err
	}

	out := make([]*core.WorkItemTrack, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) ListWorkItemTracksByWorkItem(ctx context.Context, workItemID int64) ([]*core.WorkItemTrack, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}

	var models []WorkItemTrackModel
	if err := s.orm.WithContext(ctx).
		Where("work_item_id = ?", workItemID).
		Order("id DESC").
		Find(&models).Error; err != nil {
		return nil, err
	}

	out := make([]*core.WorkItemTrack, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}
