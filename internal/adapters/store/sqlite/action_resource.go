package sqlite

import (
	"context"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"gorm.io/gorm"
)

func (s *Store) CreateActionResource(ctx context.Context, ar *core.ActionResource) (int64, error) {
	now := time.Now().UTC()
	model := actionResourceModelFromCore(ar)
	model.CreatedAt = now
	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, err
	}
	ar.ID = model.ID
	ar.CreatedAt = now
	return model.ID, nil
}

func (s *Store) GetActionResource(ctx context.Context, id int64) (*core.ActionResource, error) {
	var model ActionResourceModel
	err := s.orm.WithContext(ctx).First(&model, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, err
	}
	return model.toCore(), nil
}

func (s *Store) ListActionResources(ctx context.Context, actionID int64) ([]*core.ActionResource, error) {
	var models []ActionResourceModel
	err := s.orm.WithContext(ctx).
		Where("action_id = ?", actionID).
		Order("id ASC").
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	out := make([]*core.ActionResource, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) ListActionResourcesByDirection(ctx context.Context, actionID int64, direction core.ActionResourceDirection) ([]*core.ActionResource, error) {
	var models []ActionResourceModel
	err := s.orm.WithContext(ctx).
		Where("action_id = ? AND direction = ?", actionID, string(direction)).
		Order("id ASC").
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	out := make([]*core.ActionResource, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) DeleteActionResource(ctx context.Context, id int64) error {
	result := s.orm.WithContext(ctx).Delete(&ActionResourceModel{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}
