package sqlite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"gorm.io/gorm"
)

func (s *Store) CreateThread(ctx context.Context, thread *core.Thread) (int64, error) {
	if s == nil || s.orm == nil {
		return 0, fmt.Errorf("store is not initialized")
	}
	if thread == nil {
		return 0, fmt.Errorf("thread is nil")
	}

	title := strings.TrimSpace(thread.Title)
	if title == "" {
		return 0, fmt.Errorf("title is required")
	}

	if thread.Status == "" {
		thread.Status = core.ThreadActive
	}

	now := time.Now().UTC()
	model := threadModelFromCore(thread)
	model.Title = title
	model.CreatedAt = now
	model.UpdatedAt = now

	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, err
	}
	thread.ID = model.ID
	thread.Title = title
	thread.CreatedAt = now
	thread.UpdatedAt = now
	return model.ID, nil
}

func (s *Store) GetThread(ctx context.Context, id int64) (*core.Thread, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}

	var model ThreadModel
	err := s.orm.WithContext(ctx).First(&model, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, err
	}
	return model.toCore(), nil
}

func (s *Store) ListThreads(ctx context.Context, filter core.ThreadFilter) ([]*core.Thread, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}

	query := s.orm.WithContext(ctx).Model(&ThreadModel{})

	if filter.Status != nil {
		query = query.Where("status = ?", string(*filter.Status))
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	var models []ThreadModel
	if err := query.Order("id DESC").Limit(limit).Offset(offset).Find(&models).Error; err != nil {
		return nil, err
	}

	out := make([]*core.Thread, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) UpdateThread(ctx context.Context, thread *core.Thread) error {
	if s == nil || s.orm == nil {
		return fmt.Errorf("store is not initialized")
	}
	if thread == nil {
		return fmt.Errorf("thread is nil")
	}

	now := time.Now().UTC()
	model := threadModelFromCore(thread)
	model.UpdatedAt = now

	result := s.orm.WithContext(ctx).Model(&ThreadModel{}).
		Where("id = ?", thread.ID).
		Updates(map[string]any{
			"title":      model.Title,
			"status":     model.Status,
			"owner_id":   model.OwnerID,
			"summary":    model.Summary,
			"metadata":   model.Metadata,
			"updated_at": model.UpdatedAt,
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}

	thread.UpdatedAt = now
	return nil
}

func (s *Store) DeleteThread(ctx context.Context, id int64) error {
	if s == nil || s.orm == nil {
		return fmt.Errorf("store is not initialized")
	}

	result := s.orm.WithContext(ctx).Delete(&ThreadModel{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}
