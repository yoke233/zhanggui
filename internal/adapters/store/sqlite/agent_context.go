package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"gorm.io/gorm"
)

func (s *Store) CreateAgentContext(ctx context.Context, ac *core.AgentContext) (int64, error) {
	now := time.Now().UTC()
	model := agentContextModelFromCore(ac)
	model.CreatedAt = now
	model.UpdatedAt = now
	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, fmt.Errorf("insert agent_context: %w", err)
	}
	ac.ID = model.ID
	ac.CreatedAt = now
	ac.UpdatedAt = now
	return model.ID, nil
}

func (s *Store) GetAgentContext(ctx context.Context, id int64) (*core.AgentContext, error) {
	var model AgentContextModel
	err := s.orm.WithContext(ctx).First(&model, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("get agent_context %d: %w", id, err)
	}
	return model.toCore(), nil
}

func (s *Store) FindAgentContext(ctx context.Context, agentID string, issueID int64) (*core.AgentContext, error) {
	var model AgentContextModel
	err := s.orm.WithContext(ctx).
		Where("agent_id = ? AND issue_id = ?", agentID, issueID).
		Order("id DESC").
		First(&model).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("find agent_context: %w", err)
	}
	return model.toCore(), nil
}

func (s *Store) UpdateAgentContext(ctx context.Context, ac *core.AgentContext) error {
	now := time.Now().UTC()
	model := agentContextModelFromCore(ac)
	model.UpdatedAt = now
	result := s.orm.WithContext(ctx).Model(&AgentContextModel{}).
		Where("id = ?", ac.ID).
		Updates(map[string]any{
			"agent_id":            model.AgentID,
			"issue_id":            model.IssueID,
			"system_prompt":       model.SystemPrompt,
			"session_id":          model.SessionID,
			"summary":             model.Summary,
			"turn_count":          model.TurnCount,
			"worker_id":           model.WorkerID,
			"worker_last_seen_at": model.WorkerLastSeenAt,
			"updated_at":          model.UpdatedAt,
		})
	if result.Error != nil {
		return fmt.Errorf("update agent_context: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	ac.UpdatedAt = now
	return nil
}
