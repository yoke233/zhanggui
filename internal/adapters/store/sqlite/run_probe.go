package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"gorm.io/gorm"
)

func (s *Store) CreateRunProbe(ctx context.Context, probe *core.RunProbe) (int64, error) {
	now := time.Now().UTC()
	model := runProbeModelFromCore(probe)
	model.CreatedAt = now
	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, fmt.Errorf("insert execution probe: %w", err)
	}
	probe.ID = model.ID
	probe.CreatedAt = now
	return model.ID, nil
}

func (s *Store) GetRunProbe(ctx context.Context, id int64) (*core.RunProbe, error) {
	var model RunProbeModel
	err := s.orm.WithContext(ctx).First(&model, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, err
	}
	return model.toCore(), nil
}

func (s *Store) ListRunProbesByRun(ctx context.Context, executionID int64) ([]*core.RunProbe, error) {
	var models []RunProbeModel
	err := s.orm.WithContext(ctx).
		Where("execution_id = ?", executionID).
		Order("id ASC").
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	out := make([]*core.RunProbe, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) GetLatestRunProbe(ctx context.Context, executionID int64) (*core.RunProbe, error) {
	var model RunProbeModel
	err := s.orm.WithContext(ctx).
		Where("execution_id = ?", executionID).
		Order("id DESC").
		First(&model).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, err
	}
	return model.toCore(), nil
}

func (s *Store) GetActiveRunProbe(ctx context.Context, executionID int64) (*core.RunProbe, error) {
	var model RunProbeModel
	err := s.orm.WithContext(ctx).
		Where("execution_id = ? AND status IN ?", executionID, []string{
			string(core.RunProbePending),
			string(core.RunProbeSent),
		}).
		Order("id DESC").
		First(&model).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, err
	}
	return model.toCore(), nil
}

func (s *Store) UpdateRunProbe(ctx context.Context, probe *core.RunProbe) error {
	model := runProbeModelFromCore(probe)
	result := s.orm.WithContext(ctx).Model(&RunProbeModel{}).
		Where("id = ?", probe.ID).
		Updates(map[string]any{
			"execution_id":     model.ExecutionID,
			"issue_id":         model.IssueID,
			"step_id":          model.StepID,
			"agent_context_id": model.AgentContextID,
			"session_id":       model.SessionID,
			"owner_id":         model.OwnerID,
			"trigger_source":   model.TriggerSource,
			"question":         model.Question,
			"status":           model.Status,
			"verdict":          model.Verdict,
			"reply_text":       model.ReplyText,
			"error":            model.Error,
			"sent_at":          model.SentAt,
			"answered_at":      model.AnsweredAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (s *Store) GetRunProbeRoute(ctx context.Context, executionID int64) (*core.RunProbeRoute, error) {
	type probeRouteRow struct {
		ExecutionID     int64      `gorm:"column:execution_id"`
		IssueID         int64      `gorm:"column:issue_id"`
		StepID          int64      `gorm:"column:step_id"`
		AgentContextID  *int64     `gorm:"column:agent_context_id"`
		SessionID       string     `gorm:"column:session_id"`
		OwnerID         string     `gorm:"column:owner_id"`
		OwnerLastSeenAt *time.Time `gorm:"column:worker_last_seen_at"`
	}

	var row probeRouteRow
	err := s.orm.WithContext(ctx).
		Table("executions e").
		Select("e.id AS execution_id, e.issue_id, e.step_id, e.agent_context_id, COALESCE(ac.session_id, '') AS session_id, COALESCE(ac.worker_id, '') AS owner_id, ac.worker_last_seen_at").
		Joins("LEFT JOIN agent_contexts ac ON ac.id = e.agent_context_id").
		Where("e.id = ?", executionID).
		Scan(&row).Error
	if err != nil {
		return nil, fmt.Errorf("get execution probe route: %w", err)
	}
	if row.ExecutionID == 0 {
		return nil, core.ErrNotFound
	}
	return &core.RunProbeRoute{
		RunID:           row.ExecutionID,
		WorkItemID:      row.IssueID,
		ActionID:        row.StepID,
		AgentContextID:  row.AgentContextID,
		SessionID:       row.SessionID,
		OwnerID:         row.OwnerID,
		OwnerLastSeenAt: row.OwnerLastSeenAt,
	}, nil
}
