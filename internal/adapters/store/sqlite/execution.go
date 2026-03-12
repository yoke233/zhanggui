package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"gorm.io/gorm"
)

func (s *Store) CreateExecution(ctx context.Context, e *core.Execution) (int64, error) {
	now := time.Now().UTC()
	model := executionModelFromCore(e)
	model.CreatedAt = now

	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, fmt.Errorf("insert execution: %w", err)
	}
	e.ID = model.ID
	e.CreatedAt = now
	return model.ID, nil
}

func (s *Store) GetExecution(ctx context.Context, id int64) (*core.Execution, error) {
	var model ExecutionModel
	err := s.orm.WithContext(ctx).First(&model, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("get execution %d: %w", id, err)
	}
	return model.toCore(), nil
}

func (s *Store) ListExecutionsByStep(ctx context.Context, stepID int64) ([]*core.Execution, error) {
	var models []ExecutionModel
	err := s.orm.WithContext(ctx).
		Where("step_id = ?", stepID).
		Order("attempt ASC").
		Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("list executions by step: %w", err)
	}

	out := make([]*core.Execution, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) ListExecutionsByStatus(ctx context.Context, status core.ExecutionStatus) ([]*core.Execution, error) {
	var models []ExecutionModel
	err := s.orm.WithContext(ctx).
		Where("status = ?", string(status)).
		Order("id ASC").
		Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("list executions by status: %w", err)
	}

	out := make([]*core.Execution, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) UpdateExecution(ctx context.Context, e *core.Execution) error {
	model := executionModelFromCore(e)
	result := s.orm.WithContext(ctx).Model(&ExecutionModel{}).
		Where("id = ?", e.ID).
		Updates(map[string]any{
			"status":            model.Status,
			"agent_id":          model.AgentID,
			"agent_context_id":  model.AgentContextID,
			"briefing_snapshot": model.BriefingSnapshot,
			"artifact_id":       model.ArtifactID,
			"output":            model.Output,
			"error_message":     model.ErrorMessage,
			"error_kind":        model.ErrorKind,
			"started_at":        model.StartedAt,
			"finished_at":       model.FinishedAt,
		})
	if result.Error != nil {
		return fmt.Errorf("update execution: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}
