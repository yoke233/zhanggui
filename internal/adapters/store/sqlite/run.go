package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"gorm.io/gorm"
)

func (s *Store) CreateRun(ctx context.Context, e *core.Run) (int64, error) {
	now := time.Now().UTC()
	model := runModelFromCore(e)
	model.CreatedAt = now

	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, fmt.Errorf("insert execution: %w", err)
	}
	e.ID = model.ID
	e.CreatedAt = now
	return model.ID, nil
}

func (s *Store) GetRun(ctx context.Context, id int64) (*core.Run, error) {
	var model RunModel
	err := s.orm.WithContext(ctx).First(&model, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("get execution %d: %w", id, err)
	}
	return model.toCore(), nil
}

func (s *Store) ListRunsByAction(ctx context.Context, stepID int64) ([]*core.Run, error) {
	var models []RunModel
	err := s.orm.WithContext(ctx).
		Where("step_id = ?", stepID).
		Order("attempt ASC").
		Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("list executions by step: %w", err)
	}

	out := make([]*core.Run, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) ListRunsByStatus(ctx context.Context, status core.RunStatus) ([]*core.Run, error) {
	var models []RunModel
	err := s.orm.WithContext(ctx).
		Where("status = ?", string(status)).
		Order("id ASC").
		Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("list executions by status: %w", err)
	}

	out := make([]*core.Run, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) UpdateRun(ctx context.Context, e *core.Run) error {
	model := runModelFromCore(e)
	result := s.orm.WithContext(ctx).Model(&RunModel{}).
		Where("id = ?", e.ID).
		Updates(map[string]any{
			"status":            model.Status,
			"agent_id":          model.AgentID,
			"agent_context_id":  model.AgentContextID,
			"briefing_snapshot": model.BriefingSnapshot,
			"input":             model.Input,
			"output":            model.Output,
			"error_message":     model.ErrorMessage,
			"error_kind":        model.ErrorKind,
			"started_at":        model.StartedAt,
			"finished_at":       model.FinishedAt,
			"result_markdown":   model.ResultMarkdown,
			"result_metadata":   model.ResultMetadata,
		})
	if result.Error != nil {
		return fmt.Errorf("update execution: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (s *Store) GetLatestRunWithResult(ctx context.Context, actionID int64) (*core.Run, error) {
	var model RunModel
	err := s.orm.WithContext(ctx).
		Where("step_id = ? AND result_markdown != '' AND result_markdown IS NOT NULL", actionID).
		Order("id DESC").
		First(&model).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("get latest run with result for action %d: %w", actionID, err)
	}
	return model.toCore(), nil
}
