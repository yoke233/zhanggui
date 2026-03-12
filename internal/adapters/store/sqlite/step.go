package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"gorm.io/gorm"
)

func (s *Store) CreateStep(ctx context.Context, st *core.Step) (int64, error) {
	if s == nil || s.orm == nil {
		return 0, fmt.Errorf("store is not initialized")
	}

	now := time.Now().UTC()
	model := stepModelFromCore(st)
	model.CreatedAt = now
	model.UpdatedAt = now

	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, fmt.Errorf("insert step: %w", err)
	}
	st.ID = model.ID
	st.CreatedAt = now
	st.UpdatedAt = now
	return model.ID, nil
}

func (s *Store) GetStep(ctx context.Context, id int64) (*core.Step, error) {
	var model StepModel
	err := s.orm.WithContext(ctx).First(&model, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("get step %d: %w", id, err)
	}
	return model.toCore(), nil
}

func (s *Store) ListStepsByIssue(ctx context.Context, issueID int64) ([]*core.Step, error) {
	var models []StepModel
	err := s.orm.WithContext(ctx).
		Where("issue_id = ?", issueID).
		Order("position ASC, id ASC").
		Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("list steps by issue: %w", err)
	}

	steps := make([]*core.Step, 0, len(models))
	for i := range models {
		steps = append(steps, models[i].toCore())
	}
	return steps, nil
}

func (s *Store) UpdateStepStatus(ctx context.Context, id int64, status core.StepStatus) error {
	result := s.orm.WithContext(ctx).Model(&StepModel{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":     string(status),
			"updated_at": time.Now().UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("update step status: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (s *Store) UpdateStep(ctx context.Context, st *core.Step) error {
	now := time.Now().UTC()
	model := stepModelFromCore(st)
	model.UpdatedAt = now

	result := s.orm.WithContext(ctx).Model(&StepModel{}).
		Where("id = ?", st.ID).
		Updates(map[string]any{
			"name":                  model.Name,
			"description":           model.Description,
			"type":                  model.Type,
			"status":                model.Status,
			"position":              model.Position,
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
		return fmt.Errorf("update step: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	st.UpdatedAt = now
	return nil
}

func (s *Store) DeleteStep(ctx context.Context, id int64) error {
	result := s.orm.WithContext(ctx).Delete(&StepModel{}, id)
	if result.Error != nil {
		return fmt.Errorf("delete step %d: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}
