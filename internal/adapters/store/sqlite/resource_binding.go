package sqlite

import (
	"context"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"gorm.io/gorm"
)

func (s *Store) CreateResourceBinding(ctx context.Context, rb *core.ResourceBinding) (int64, error) {
	now := time.Now().UTC()
	model := resourceBindingModelFromCore(rb)
	model.CreatedAt = now
	model.UpdatedAt = now
	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, err
	}
	rb.ID = model.ID
	rb.CreatedAt = now
	rb.UpdatedAt = now
	return model.ID, nil
}

func (s *Store) GetResourceBinding(ctx context.Context, id int64) (*core.ResourceBinding, error) {
	var model ResourceBindingModel
	err := s.orm.WithContext(ctx).First(&model, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, err
	}
	return model.toCore(), nil
}

func (s *Store) ListResourceBindings(ctx context.Context, projectID int64) ([]*core.ResourceBinding, error) {
	var models []ResourceBindingModel
	err := s.orm.WithContext(ctx).
		Where("project_id = ?", projectID).
		Order("id ASC").
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	out := make([]*core.ResourceBinding, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) UpdateResourceBinding(ctx context.Context, rb *core.ResourceBinding) error {
	now := time.Now().UTC()
	model := resourceBindingModelFromCore(rb)
	model.UpdatedAt = now
	result := s.orm.WithContext(ctx).Model(model).Updates(map[string]any{
		"kind":       model.Kind,
		"uri":        model.URI,
		"config":     model.Config,
		"label":      model.Label,
		"updated_at": now,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	rb.UpdatedAt = now
	return nil
}

func (s *Store) ListResourceBindingsByIssue(ctx context.Context, issueID int64, kind string) ([]*core.ResourceBinding, error) {
	var models []ResourceBindingModel
	q := s.orm.WithContext(ctx).Where("issue_id = ?", issueID)
	if kind != "" {
		q = q.Where("kind = ?", kind)
	}
	if err := q.Order("id ASC").Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]*core.ResourceBinding, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) DeleteResourceBinding(ctx context.Context, id int64) error {
	result := s.orm.WithContext(ctx).Delete(&ResourceBindingModel{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}
