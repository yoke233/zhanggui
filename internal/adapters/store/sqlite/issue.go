package sqlite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"gorm.io/gorm"
)

func (s *Store) CreateIssue(ctx context.Context, issue *core.Issue) (int64, error) {
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
		issue.Status = core.IssueOpen
	}
	if issue.Priority == "" {
		issue.Priority = core.PriorityMedium
	}

	now := time.Now().UTC()
	model := issueModelFromCore(issue)
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

func (s *Store) GetIssue(ctx context.Context, id int64) (*core.Issue, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}

	var model IssueModel
	err := s.orm.WithContext(ctx).First(&model, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, err
	}
	return model.toCore(), nil
}

func (s *Store) ListIssues(ctx context.Context, filter core.IssueFilter) ([]*core.Issue, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}

	query := s.orm.WithContext(ctx).Model(&IssueModel{})
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

	var models []IssueModel
	if err := query.Order("id DESC").Limit(limit).Offset(offset).Find(&models).Error; err != nil {
		return nil, err
	}

	out := make([]*core.Issue, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) UpdateIssue(ctx context.Context, issue *core.Issue) error {
	if s == nil || s.orm == nil {
		return fmt.Errorf("store is not initialized")
	}
	if issue == nil {
		return fmt.Errorf("issue is nil")
	}

	now := time.Now().UTC()
	model := issueModelFromCore(issue)
	model.UpdatedAt = now

	result := s.orm.WithContext(ctx).Model(&IssueModel{}).
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

func (s *Store) UpdateIssueStatus(ctx context.Context, id int64, status core.IssueStatus) error {
	if s == nil || s.orm == nil {
		return fmt.Errorf("store is not initialized")
	}

	result := s.orm.WithContext(ctx).Model(&IssueModel{}).
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

func (s *Store) UpdateIssueMetadata(ctx context.Context, id int64, metadata map[string]any) error {
	if s == nil || s.orm == nil {
		return fmt.Errorf("store is not initialized")
	}

	result := s.orm.WithContext(ctx).Model(&IssueModel{}).
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

func (s *Store) PrepareIssueRun(ctx context.Context, id int64, queuedStatus core.IssueStatus) error {
	if queuedStatus != core.IssueQueued && queuedStatus != core.IssueRunning {
		return core.ErrInvalidTransition
	}

	result := s.orm.WithContext(ctx).Model(&IssueModel{}).
		Where("id = ? AND status IN ? AND archived_at IS NULL", id, []string{string(core.IssueOpen), string(core.IssueAccepted)}).
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

	if _, err := s.GetIssue(ctx, id); err != nil {
		return err
	}
	return core.ErrInvalidTransition
}

func (s *Store) SetIssueArchived(ctx context.Context, id int64, archived bool) error {
	now := time.Now().UTC()
	query := s.orm.WithContext(ctx).Model(&IssueModel{}).Where("id = ?", id)
	if archived {
		query = query.Where("archived_at IS NULL").Where("status NOT IN ?", []string{
			string(core.IssueQueued),
			string(core.IssueRunning),
			string(core.IssueBlocked),
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

	if _, err := s.GetIssue(ctx, id); err != nil {
		return err
	}
	return core.ErrInvalidTransition
}

func (s *Store) DeleteIssue(ctx context.Context, id int64) error {
	if s == nil || s.orm == nil {
		return fmt.Errorf("store is not initialized")
	}

	result := s.orm.WithContext(ctx).Delete(&IssueModel{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}
