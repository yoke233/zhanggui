package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/yoke233/zhanggui/internal/core"
	"gorm.io/gorm"
)

func (s *Store) CreateAction(ctx context.Context, st *core.Action) (int64, error) {
	if s == nil || s.orm == nil {
		return 0, fmt.Errorf("store is not initialized")
	}

	now := time.Now().UTC()
	model := actionModelFromCore(st)
	model.CreatedAt = now
	model.UpdatedAt = now

	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, fmt.Errorf("insert action: %w", err)
	}
	st.ID = model.ID
	st.CreatedAt = now
	st.UpdatedAt = now
	return model.ID, nil
}

func (s *Store) GetAction(ctx context.Context, id int64) (*core.Action, error) {
	var model ActionModel
	err := s.orm.WithContext(ctx).First(&model, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("get action %d: %w", id, err)
	}
	return model.toCore(), nil
}

func (s *Store) ListActionsByWorkItem(ctx context.Context, issueID int64) ([]*core.Action, error) {
	var models []ActionModel
	err := s.orm.WithContext(ctx).
		Where("work_item_id = ?", issueID).
		Order("position ASC, id ASC").
		Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("list actions by work item: %w", err)
	}

	actions := make([]*core.Action, 0, len(models))
	for i := range models {
		actions = append(actions, models[i].toCore())
	}
	return actions, nil
}

func (s *Store) UpdateActionStatus(ctx context.Context, id int64, status core.ActionStatus) error {
	result := s.orm.WithContext(ctx).Model(&ActionModel{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":     string(status),
			"updated_at": time.Now().UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("update action status: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (s *Store) UpdateAction(ctx context.Context, st *core.Action) error {
	now := time.Now().UTC()
	model := actionModelFromCore(st)
	model.UpdatedAt = now

	result := s.orm.WithContext(ctx).Model(&ActionModel{}).
		Where("id = ?", st.ID).
		Updates(map[string]any{
			"name":                  model.Name,
			"description":           model.Description,
			"type":                  model.Type,
			"status":                model.Status,
			"position":              model.Position,
			"depends_on":            model.DependsOn,
			"agent_role":            model.AgentRole,
			"required_capabilities": model.RequiredCapabilities,
			"acceptance_criteria":   model.AcceptanceCriteria,
			"timeout_ms":            model.TimeoutMs,
			"config":                model.Config,
			"max_retries":           model.MaxRetries,
			"retry_count":           model.RetryCount,
			"updated_at":            model.UpdatedAt,
		})
	if result.Error != nil {
		return fmt.Errorf("update action: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	st.UpdatedAt = now
	return nil
}

func (s *Store) DeleteAction(ctx context.Context, id int64) error {
	result := s.orm.WithContext(ctx).Delete(&ActionModel{}, id)
	if result.Error != nil {
		return fmt.Errorf("delete action %d: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (s *Store) BatchCreateActions(ctx context.Context, actions []*core.Action) error {
	if len(actions) == 0 {
		return nil
	}
	now := time.Now().UTC()
	models := make([]ActionModel, 0, len(actions))
	for _, a := range actions {
		m := *actionModelFromCore(a)
		m.CreatedAt = now
		m.UpdatedAt = now
		models = append(models, m)
	}
	if err := s.orm.WithContext(ctx).Create(&models).Error; err != nil {
		return fmt.Errorf("batch insert actions: %w", err)
	}
	for i := range models {
		actions[i].ID = models[i].ID
		actions[i].CreatedAt = now
		actions[i].UpdatedAt = now
	}
	return nil
}

func (s *Store) UpdateActionDependsOn(ctx context.Context, id int64, dependsOn []int64) error {
	result := s.orm.WithContext(ctx).Model(&ActionModel{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"depends_on": JSONField[[]int64]{Data: dependsOn},
			"updated_at": time.Now().UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("update action depends_on: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}
