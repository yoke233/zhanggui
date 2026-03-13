package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"gorm.io/gorm"
)

func (s *Store) CreateDeliverable(ctx context.Context, a *core.Deliverable) (int64, error) {
	now := time.Now().UTC()
	model := deliverableModelFromCore(a)
	model.CreatedAt = now

	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, fmt.Errorf("insert artifact: %w", err)
	}
	a.ID = model.ID
	a.CreatedAt = now
	return model.ID, nil
}

func (s *Store) GetDeliverable(ctx context.Context, id int64) (*core.Deliverable, error) {
	var model DeliverableModel
	err := s.orm.WithContext(ctx).First(&model, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("get artifact %d: %w", id, err)
	}
	return model.toCore(), nil
}

func (s *Store) GetLatestDeliverableByAction(ctx context.Context, stepID int64) (*core.Deliverable, error) {
	var model DeliverableModel
	err := s.orm.WithContext(ctx).Where("step_id = ?", stepID).Order("id DESC").First(&model).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("get latest artifact for step %d: %w", stepID, err)
	}
	return model.toCore(), nil
}

func (s *Store) UpdateDeliverable(ctx context.Context, a *core.Deliverable) error {
	model := deliverableModelFromCore(a)
	result := s.orm.WithContext(ctx).Model(&DeliverableModel{}).
		Where("id = ?", a.ID).
		Updates(map[string]any{
			"result_markdown": model.ResultMarkdown,
			"metadata":        model.Metadata,
			"assets":          model.Assets,
		})
	if result.Error != nil {
		return fmt.Errorf("update artifact %d: %w", a.ID, result.Error)
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (s *Store) ListDeliverablesByRun(ctx context.Context, execID int64) ([]*core.Deliverable, error) {
	var models []DeliverableModel
	err := s.orm.WithContext(ctx).Where("execution_id = ?", execID).Order("id ASC").Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("list artifacts by execution: %w", err)
	}

	out := make([]*core.Deliverable, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}
