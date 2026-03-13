package sqlite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"gorm.io/gorm"
)

func (s *Store) CreateDAGTemplate(ctx context.Context, t *core.DAGTemplate) (int64, error) {
	now := time.Now().UTC()
	model := dagTemplateModelFromCore(t)
	model.CreatedAt = now
	model.UpdatedAt = now
	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, fmt.Errorf("insert dag_template: %w", err)
	}
	t.ID = model.ID
	t.CreatedAt = now
	t.UpdatedAt = now
	return model.ID, nil
}

func (s *Store) GetDAGTemplate(ctx context.Context, id int64) (*core.DAGTemplate, error) {
	var model DAGTemplateModel
	err := s.orm.WithContext(ctx).First(&model, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("get dag_template %d: %w", id, err)
	}
	t := model.toCore()
	if t.Actions == nil {
		t.Actions = []core.DAGTemplateAction{}
	}
	return t, nil
}

func (s *Store) ListDAGTemplates(ctx context.Context, filter core.DAGTemplateFilter) ([]*core.DAGTemplate, error) {
	query := s.orm.WithContext(ctx).Model(&DAGTemplateModel{})
	if filter.ProjectID != nil {
		query = query.Where("project_id = ?", *filter.ProjectID)
	}
	if filter.Tag != "" {
		query = query.Where("tags LIKE ?", `%\"`+filter.Tag+`\"%`)
	}
	if filter.Search != "" {
		pattern := "%" + strings.TrimSpace(filter.Search) + "%"
		query = query.Where("(name LIKE ? OR description LIKE ?)", pattern, pattern)
	}
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	var models []DAGTemplateModel
	if err := query.Order("id DESC").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list dag_templates: %w", err)
	}
	out := make([]*core.DAGTemplate, 0, len(models))
	for i := range models {
		item := models[i].toCore()
		if item.Actions == nil {
			item.Actions = []core.DAGTemplateAction{}
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Store) UpdateDAGTemplate(ctx context.Context, t *core.DAGTemplate) error {
	now := time.Now().UTC()
	model := dagTemplateModelFromCore(t)
	model.UpdatedAt = now
	result := s.orm.WithContext(ctx).Model(&DAGTemplateModel{}).
		Where("id = ?", t.ID).
		Updates(map[string]any{
			"name":        model.Name,
			"description": model.Description,
			"project_id":  model.ProjectID,
			"tags":        model.Tags,
			"metadata":    model.Metadata,
			"steps":       model.Steps,
			"updated_at":  model.UpdatedAt,
		})
	if result.Error != nil {
		return fmt.Errorf("update dag_template: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	t.UpdatedAt = now
	return nil
}

func (s *Store) DeleteDAGTemplate(ctx context.Context, id int64) error {
	result := s.orm.WithContext(ctx).Delete(&DAGTemplateModel{}, id)
	if result.Error != nil {
		return fmt.Errorf("delete dag_template %d: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}
