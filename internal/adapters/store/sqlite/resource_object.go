package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"gorm.io/gorm"
)

func (s *Store) CreateResource(ctx context.Context, r *core.Resource) (int64, error) {
	if err := validateResource(r); err != nil {
		return 0, err
	}
	now := time.Now().UTC()
	model := resourceModelFromCore(r)
	model.CreatedAt = now
	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, err
	}
	r.ID = model.ID
	r.CreatedAt = now
	return model.ID, nil
}

func (s *Store) GetResource(ctx context.Context, id int64) (*core.Resource, error) {
	var model ResourceModel
	err := s.orm.WithContext(ctx).First(&model, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, err
	}
	return model.toCore(), nil
}

func (s *Store) ListResourcesByWorkItem(ctx context.Context, workItemID int64) ([]*core.Resource, error) {
	return s.listResources(ctx, "work_item_id = ?", workItemID)
}

func (s *Store) ListResourcesByRun(ctx context.Context, runID int64) ([]*core.Resource, error) {
	return s.listResources(ctx, "run_id = ?", runID)
}

func (s *Store) ListResourcesByMessage(ctx context.Context, messageID int64) ([]*core.Resource, error) {
	return s.listResources(ctx, "message_id = ?", messageID)
}

func (s *Store) DeleteResource(ctx context.Context, id int64) error {
	result := s.orm.WithContext(ctx).Delete(&ResourceModel{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (s *Store) listResources(ctx context.Context, query string, value int64) ([]*core.Resource, error) {
	var models []ResourceModel
	if err := s.orm.WithContext(ctx).
		Where(query, value).
		Order("id ASC").
		Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]*core.Resource, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func validateResource(r *core.Resource) error {
	if r == nil {
		return fmt.Errorf("resource is required")
	}
	owners := 0
	if r.WorkItemID != nil {
		owners++
	}
	if r.RunID != nil {
		owners++
	}
	if r.MessageID != nil {
		owners++
	}
	if owners > 1 {
		return fmt.Errorf("resource must have at most one owner")
	}
	if r.URI == "" {
		return fmt.Errorf("resource requires uri")
	}
	if r.FileName == "" {
		return fmt.Errorf("resource requires file_name")
	}
	if r.StorageKind == "" {
		return fmt.Errorf("resource requires storage_kind")
	}
	return nil
}
