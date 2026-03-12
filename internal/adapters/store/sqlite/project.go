package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"gorm.io/gorm"
)

func (s *Store) CreateProject(ctx context.Context, p *core.Project) (int64, error) {
	if s == nil || s.orm == nil {
		return 0, fmt.Errorf("store is not initialized")
	}
	if p == nil {
		return 0, fmt.Errorf("project is nil")
	}
	if p.Kind == "" {
		p.Kind = core.ProjectGeneral
	}
	now := time.Now().UTC()
	model := projectModelFromCore(p)
	model.CreatedAt = now
	model.UpdatedAt = now
	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, err
	}
	p.ID = model.ID
	p.CreatedAt = now
	p.UpdatedAt = now
	return model.ID, nil
}

func (s *Store) GetProject(ctx context.Context, id int64) (*core.Project, error) {
	var model ProjectModel
	err := s.orm.WithContext(ctx).First(&model, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, err
	}
	return model.toCore(), nil
}

func (s *Store) ListProjects(ctx context.Context, limit, offset int) ([]*core.Project, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	var models []ProjectModel
	err := s.orm.WithContext(ctx).Order("id DESC").Limit(limit).Offset(offset).Find(&models).Error
	if err != nil {
		return nil, err
	}
	out := make([]*core.Project, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) UpdateProject(ctx context.Context, p *core.Project) error {
	if p == nil {
		return fmt.Errorf("project is nil")
	}
	now := time.Now().UTC()
	model := projectModelFromCore(p)
	model.UpdatedAt = now
	result := s.orm.WithContext(ctx).Model(&ProjectModel{}).
		Where("id = ?", p.ID).
		Updates(map[string]any{
			"name":        model.Name,
			"kind":        model.Kind,
			"description": model.Description,
			"metadata":    model.Metadata,
			"updated_at":  model.UpdatedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	p.UpdatedAt = now
	return nil
}

func (s *Store) DeleteProject(ctx context.Context, id int64) error {
	result := s.orm.WithContext(ctx).Delete(&ProjectModel{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}
