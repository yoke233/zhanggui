package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"gorm.io/gorm"
)

func (s *Store) CreateBriefing(ctx context.Context, b *core.Briefing) (int64, error) {
	now := time.Now().UTC()
	model := briefingModelFromCore(b)
	model.CreatedAt = now
	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, fmt.Errorf("insert briefing: %w", err)
	}
	b.ID = model.ID
	b.CreatedAt = now
	return model.ID, nil
}

func (s *Store) GetBriefing(ctx context.Context, id int64) (*core.Briefing, error) {
	var model BriefingModel
	err := s.orm.WithContext(ctx).First(&model, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("get briefing %d: %w", id, err)
	}
	return model.toCore(), nil
}

func (s *Store) GetBriefingByStep(ctx context.Context, stepID int64) (*core.Briefing, error) {
	var model BriefingModel
	err := s.orm.WithContext(ctx).Where("step_id = ?", stepID).Order("id DESC").First(&model).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("get briefing by step %d: %w", stepID, err)
	}
	return model.toCore(), nil
}
