package sqlite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"gorm.io/gorm"
)

// --- FeatureEntry CRUD ---

func (s *Store) CreateFeatureEntry(ctx context.Context, entry *core.FeatureEntry) (int64, error) {
	if s == nil || s.orm == nil {
		return 0, fmt.Errorf("store is not initialized")
	}
	if entry == nil {
		return 0, fmt.Errorf("entry is nil")
	}
	if strings.TrimSpace(entry.Key) == "" {
		return 0, fmt.Errorf("key is required")
	}
	if entry.Status == "" {
		entry.Status = core.FeaturePending
	}

	now := time.Now().UTC()
	model := featureEntryModelFromCore(entry)
	model.Key = strings.TrimSpace(model.Key)
	model.CreatedAt = now
	model.UpdatedAt = now

	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return 0, core.ErrDuplicateEntryKey
		}
		return 0, err
	}
	entry.ID = model.ID
	entry.CreatedAt = now
	entry.UpdatedAt = now
	return model.ID, nil
}

func (s *Store) GetFeatureEntry(ctx context.Context, id int64) (*core.FeatureEntry, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}
	var model FeatureEntryModel
	if err := s.orm.WithContext(ctx).First(&model, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, err
	}
	return model.toCore(), nil
}

func (s *Store) GetFeatureEntryByKey(ctx context.Context, projectID int64, key string) (*core.FeatureEntry, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}
	var model FeatureEntryModel
	if err := s.orm.WithContext(ctx).Where("project_id = ? AND key = ?", projectID, key).First(&model).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, err
	}
	return model.toCore(), nil
}

func (s *Store) ListFeatureEntries(ctx context.Context, filter core.FeatureEntryFilter) ([]*core.FeatureEntry, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}

	query := s.orm.WithContext(ctx).Model(&FeatureEntryModel{})
	if filter.ProjectID > 0 {
		query = query.Where("project_id = ?", filter.ProjectID)
	}
	if filter.Status != nil {
		query = query.Where("status = ?", string(*filter.Status))
	}
	if filter.WorkItemID != nil {
		query = query.Where("issue_id = ?", *filter.WorkItemID)
	}
	// Tags filter: match entries whose JSON tags column contains ALL requested tags.
	for _, tag := range filter.Tags {
		query = query.Where("tags LIKE ?", "%"+tag+"%")
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 200
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	var models []FeatureEntryModel
	if err := query.Order("id ASC").Limit(limit).Offset(offset).Find(&models).Error; err != nil {
		return nil, err
	}

	out := make([]*core.FeatureEntry, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) UpdateFeatureEntry(ctx context.Context, entry *core.FeatureEntry) error {
	if s == nil || s.orm == nil {
		return fmt.Errorf("store is not initialized")
	}
	if entry == nil {
		return fmt.Errorf("entry is nil")
	}

	now := time.Now().UTC()
	model := featureEntryModelFromCore(entry)
	result := s.orm.WithContext(ctx).Model(&FeatureEntryModel{}).
		Where("id = ?", entry.ID).
		Updates(map[string]any{
			"key":         model.Key,
			"description": model.Description,
			"status":      model.Status,
			"issue_id":    model.IssueID,
			"step_id":     model.StepID,
			"tags":        model.Tags,
			"metadata":    model.Metadata,
			"updated_at":  now,
		})
	if result.Error != nil {
		if strings.Contains(result.Error.Error(), "UNIQUE constraint failed") {
			return core.ErrDuplicateEntryKey
		}
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	entry.UpdatedAt = now
	return nil
}

func (s *Store) UpdateFeatureEntryStatus(ctx context.Context, id int64, status core.FeatureStatus) error {
	if s == nil || s.orm == nil {
		return fmt.Errorf("store is not initialized")
	}

	result := s.orm.WithContext(ctx).Model(&FeatureEntryModel{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":     string(status),
			"updated_at": time.Now().UTC(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteFeatureEntry(ctx context.Context, id int64) error {
	if s == nil || s.orm == nil {
		return fmt.Errorf("store is not initialized")
	}
	result := s.orm.WithContext(ctx).Delete(&FeatureEntryModel{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (s *Store) CountFeatureEntriesByStatus(ctx context.Context, projectID int64) (map[core.FeatureStatus]int, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}

	type statusCount struct {
		Status string
		Count  int
	}

	var rows []statusCount
	err := s.orm.WithContext(ctx).Model(&FeatureEntryModel{}).
		Select("status, COUNT(*) as count").
		Where("project_id = ?", projectID).
		Group("status").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	result := make(map[core.FeatureStatus]int, len(rows))
	for _, r := range rows {
		result[core.FeatureStatus(r.Status)] = r.Count
	}
	return result, nil
}
