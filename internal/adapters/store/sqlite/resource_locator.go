package sqlite

import (
	"context"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"gorm.io/gorm"
)

func (s *Store) CreateResourceLocator(ctx context.Context, loc *core.ResourceLocator) (int64, error) {
	now := time.Now().UTC()
	model := resourceLocatorModelFromCore(loc)
	model.CreatedAt = now
	model.UpdatedAt = now
	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, err
	}
	loc.ID = model.ID
	loc.CreatedAt = now
	loc.UpdatedAt = now
	return model.ID, nil
}

func (s *Store) GetResourceLocator(ctx context.Context, id int64) (*core.ResourceLocator, error) {
	var model ResourceLocatorModel
	err := s.orm.WithContext(ctx).First(&model, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, err
	}
	return model.toCore(), nil
}

func (s *Store) ListResourceLocators(ctx context.Context, projectID int64) ([]*core.ResourceLocator, error) {
	var models []ResourceLocatorModel
	err := s.orm.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("id ASC").
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	out := make([]*core.ResourceLocator, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) UpdateResourceLocator(ctx context.Context, loc *core.ResourceLocator) error {
	now := time.Now().UTC()
	model := resourceLocatorModelFromCore(loc)
	model.UpdatedAt = now
	result := s.orm.WithContext(ctx).Model(model).Updates(map[string]any{
		"kind":       model.Kind,
		"label":      model.Label,
		"base_uri":   model.BaseURI,
		"config":     model.Config,
		"updated_at": now,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	loc.UpdatedAt = now
	return nil
}

func (s *Store) DeleteResourceLocator(ctx context.Context, id int64) error {
	result := s.orm.WithContext(ctx).Delete(&ResourceLocatorModel{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}
