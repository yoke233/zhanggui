package sqlite

import (
	"context"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"gorm.io/gorm"
)

func (s *Store) CreateResourceSpace(ctx context.Context, rs *core.ResourceSpace) (int64, error) {
	now := time.Now().UTC()
	model := resourceSpaceModelFromCore(rs)
	model.CreatedAt = now
	model.UpdatedAt = now
	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, err
	}
	rs.ID = model.ID
	rs.CreatedAt = now
	rs.UpdatedAt = now
	return model.ID, nil
}

func (s *Store) GetResourceSpace(ctx context.Context, id int64) (*core.ResourceSpace, error) {
	var model ResourceSpaceModel
	err := s.orm.WithContext(ctx).First(&model, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, err
	}
	return model.toCore(), nil
}

func (s *Store) ListResourceSpaces(ctx context.Context, projectID int64) ([]*core.ResourceSpace, error) {
	var models []ResourceSpaceModel
	if err := s.orm.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("id ASC").
		Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]*core.ResourceSpace, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) UpdateResourceSpace(ctx context.Context, rs *core.ResourceSpace) error {
	now := time.Now().UTC()
	model := resourceSpaceModelFromCore(rs)
	model.UpdatedAt = now
	result := s.orm.WithContext(ctx).Model(model).Updates(map[string]any{
		"kind":       model.Kind,
		"root_uri":   model.RootURI,
		"role":       model.Role,
		"label":      model.Label,
		"config":     model.Config,
		"updated_at": now,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	rs.UpdatedAt = now
	return nil
}

func (s *Store) DeleteResourceSpace(ctx context.Context, id int64) error {
	result := s.orm.WithContext(ctx).Delete(&ResourceSpaceModel{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}
