package sqlite

import (
	"context"
	"fmt"

	"github.com/yoke233/ai-workflow/internal/core"
	"gorm.io/gorm"
)

func (s *Store) CreateToolCallAudit(ctx context.Context, audit *core.ToolCallAudit) (int64, error) {
	model := toolCallAuditModelFromCore(audit)
	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, fmt.Errorf("insert tool call audit: %w", err)
	}
	audit.ID = model.ID
	audit.CreatedAt = model.CreatedAt
	return model.ID, nil
}

func (s *Store) GetToolCallAudit(ctx context.Context, id int64) (*core.ToolCallAudit, error) {
	var model ToolCallAuditModel
	if err := s.orm.WithContext(ctx).First(&model, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("get tool call audit: %w", err)
	}
	return model.toCore(), nil
}

func (s *Store) GetToolCallAuditByToolCallID(ctx context.Context, runID int64, toolCallID string) (*core.ToolCallAudit, error) {
	var model ToolCallAuditModel
	if err := s.orm.WithContext(ctx).
		Where("execution_id = ? AND tool_call_id = ?", runID, toolCallID).
		Order("id DESC").
		First(&model).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("get tool call audit by tool_call_id: %w", err)
	}
	return model.toCore(), nil
}

func (s *Store) ListToolCallAuditsByRun(ctx context.Context, runID int64) ([]*core.ToolCallAudit, error) {
	var models []ToolCallAuditModel
	if err := s.orm.WithContext(ctx).
		Where("execution_id = ?", runID).
		Order("id ASC").
		Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list tool call audits by execution: %w", err)
	}
	out := make([]*core.ToolCallAudit, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) UpdateToolCallAudit(ctx context.Context, audit *core.ToolCallAudit) error {
	model := toolCallAuditModelFromCore(audit)
	result := s.orm.WithContext(ctx).Model(&ToolCallAuditModel{}).
		Where("id = ?", audit.ID).
		Updates(model)
	if result.Error != nil {
		return fmt.Errorf("update tool call audit: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}
