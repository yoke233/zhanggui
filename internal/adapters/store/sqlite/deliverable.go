package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/yoke233/zhanggui/internal/core"
	"gorm.io/gorm"
)

func (s *Store) CreateDeliverable(ctx context.Context, d *core.Deliverable) (int64, error) {
	if s == nil || s.orm == nil {
		return 0, fmt.Errorf("store is not initialized")
	}
	if err := d.Validate(); err != nil {
		return 0, err
	}

	now := time.Now().UTC()
	model := deliverableModelFromCore(d)
	model.CreatedAt = now
	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, fmt.Errorf("create deliverable: %w", err)
	}
	d.ID = model.ID
	d.CreatedAt = now
	return model.ID, nil
}

func (s *Store) GetDeliverable(ctx context.Context, id int64) (*core.Deliverable, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}

	var model DeliverableModel
	if err := s.orm.WithContext(ctx).First(&model, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("get deliverable %d: %w", id, err)
	}
	return model.toCore(), nil
}

func (s *Store) ListDeliverablesByWorkItem(ctx context.Context, workItemID int64) ([]*core.Deliverable, error) {
	return s.listDeliverables(ctx, "work_item_id = ?", workItemID)
}

func (s *Store) ListDeliverablesByThread(ctx context.Context, threadID int64) ([]*core.Deliverable, error) {
	return s.listDeliverables(ctx, "thread_id = ?", threadID)
}

func (s *Store) ListDeliverablesByProducer(ctx context.Context, producerType core.DeliverableProducerType, producerID int64) ([]*core.Deliverable, error) {
	return s.listDeliverables(ctx, "producer_type = ? AND producer_id = ?", string(producerType), producerID)
}

func (s *Store) listDeliverables(ctx context.Context, query string, args ...any) ([]*core.Deliverable, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}

	var models []DeliverableModel
	if err := s.orm.WithContext(ctx).
		Where(query, args...).
		Order("created_at DESC, id DESC").
		Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list deliverables: %w", err)
	}

	out := make([]*core.Deliverable, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}
