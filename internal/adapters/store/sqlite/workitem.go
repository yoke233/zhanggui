package sqlite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"gorm.io/gorm"
)

func (s *Store) CreateWorkItem(ctx context.Context, issue *core.WorkItem) (int64, error) {
	if s == nil || s.orm == nil {
		return 0, fmt.Errorf("store is not initialized")
	}
	if issue == nil {
		return 0, fmt.Errorf("issue is nil")
	}

	title := strings.TrimSpace(issue.Title)
	if title == "" {
		return 0, fmt.Errorf("title is required")
	}

	if issue.Status == "" {
		issue.Status = core.WorkItemOpen
	}
	if issue.Priority == "" {
		issue.Priority = core.PriorityMedium
	}

	now := time.Now().UTC()
	model := workItemModelFromCore(issue)
	model.Title = title
	model.CreatedAt = now
	model.UpdatedAt = now

	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, err
	}
	issue.ID = model.ID
	issue.Title = title
	issue.CreatedAt = now
	issue.UpdatedAt = now
	return model.ID, nil
}

func (s *Store) GetWorkItem(ctx context.Context, id int64) (*core.WorkItem, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}

	var model WorkItemModel
	err := s.orm.WithContext(ctx).First(&model, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, err
	}
	return model.toCore(), nil
}

func (s *Store) ListWorkItems(ctx context.Context, filter core.WorkItemFilter) ([]*core.WorkItem, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}

	query := s.orm.WithContext(ctx).Model(&WorkItemModel{})
	if filter.ProjectID != nil {
		query = query.Where("project_id = ?", *filter.ProjectID)
	}
	if filter.Status != nil {
		query = query.Where("status = ?", string(*filter.Status))
	}
	if filter.Priority != nil {
		query = query.Where("priority = ?", string(*filter.Priority))
	}
	if filter.Archived != nil {
		if *filter.Archived {
			query = query.Where("archived_at IS NOT NULL")
		} else {
			query = query.Where("archived_at IS NULL")
		}
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	var models []WorkItemModel
	if err := query.Order("id DESC").Limit(limit).Offset(offset).Find(&models).Error; err != nil {
		return nil, err
	}

	out := make([]*core.WorkItem, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) UpdateWorkItem(ctx context.Context, issue *core.WorkItem) error {
	if s == nil || s.orm == nil {
		return fmt.Errorf("store is not initialized")
	}
	if issue == nil {
		return fmt.Errorf("issue is nil")
	}

	now := time.Now().UTC()
	model := workItemModelFromCore(issue)
	model.UpdatedAt = now

	result := s.orm.WithContext(ctx).Model(&WorkItemModel{}).
		Where("id = ?", issue.ID).
		Updates(map[string]any{
			"project_id":          model.ProjectID,
			"resource_binding_id": model.ResourceBindingID,
			"title":               model.Title,
			"body":                model.Body,
			"status":              model.Status,
			"priority":            model.Priority,
			"labels":              model.Labels,
			"depends_on":          model.DependsOn,
			"metadata":            model.Metadata,
			"updated_at":          model.UpdatedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	issue.UpdatedAt = now
	return nil
}

func (s *Store) UpdateWorkItemStatus(ctx context.Context, id int64, status core.WorkItemStatus) error {
	if s == nil || s.orm == nil {
		return fmt.Errorf("store is not initialized")
	}

	result := s.orm.WithContext(ctx).Model(&WorkItemModel{}).
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

func (s *Store) UpdateWorkItemMetadata(ctx context.Context, id int64, metadata map[string]any) error {
	if s == nil || s.orm == nil {
		return fmt.Errorf("store is not initialized")
	}

	result := s.orm.WithContext(ctx).Model(&WorkItemModel{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"metadata":   JSONField[map[string]any]{Data: metadata},
			"updated_at": time.Now().UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("update issue metadata: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (s *Store) PrepareWorkItemRun(ctx context.Context, id int64, queuedStatus core.WorkItemStatus) error {
	if queuedStatus != core.WorkItemQueued && queuedStatus != core.WorkItemRunning {
		return core.ErrInvalidTransition
	}

	result := s.orm.WithContext(ctx).Model(&WorkItemModel{}).
		Where("id = ? AND status IN ? AND archived_at IS NULL", id, []string{string(core.WorkItemOpen), string(core.WorkItemAccepted)}).
		Updates(map[string]any{
			"status":     string(queuedStatus),
			"updated_at": time.Now().UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("prepare issue run: %w", result.Error)
	}
	if result.RowsAffected > 0 {
		return nil
	}

	if _, err := s.GetWorkItem(ctx, id); err != nil {
		return err
	}
	return core.ErrInvalidTransition
}

func (s *Store) SetWorkItemArchived(ctx context.Context, id int64, archived bool) error {
	now := time.Now().UTC()
	query := s.orm.WithContext(ctx).Model(&WorkItemModel{}).Where("id = ?", id)
	if archived {
		query = query.Where("archived_at IS NULL").Where("status NOT IN ?", []string{
			string(core.WorkItemQueued),
			string(core.WorkItemRunning),
			string(core.WorkItemBlocked),
		})
	} else {
		query = query.Where("archived_at IS NOT NULL")
	}

	var updates map[string]any
	if archived {
		updates = map[string]any{"archived_at": now, "updated_at": now}
	} else {
		updates = map[string]any{"archived_at": nil, "updated_at": now}
	}

	result := query.Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("set issue archived: %w", result.Error)
	}
	if result.RowsAffected > 0 {
		return nil
	}

	if _, err := s.GetWorkItem(ctx, id); err != nil {
		return err
	}
	return core.ErrInvalidTransition
}

func (s *Store) DeleteWorkItem(ctx context.Context, id int64) error {
	if s == nil || s.orm == nil {
		return fmt.Errorf("store is not initialized")
	}

	result := s.orm.WithContext(ctx).Delete(&WorkItemModel{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}
