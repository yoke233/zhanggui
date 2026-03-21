package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/yoke233/zhanggui/internal/core"
	"gorm.io/gorm"
)

func (s *Store) CreateActionSignal(ctx context.Context, sig *core.ActionSignal) (int64, error) {
	now := time.Now().UTC()
	model := actionSignalModelFromCore(sig)
	model.CreatedAt = now
	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, fmt.Errorf("insert action signal: %w", err)
	}
	sig.ID = model.ID
	sig.CreatedAt = now

	// Dual-write to activity_journal.
	if entry := core.ActionSignalToJournalEntry(sig); entry != nil {
		_, _ = s.AppendJournal(ctx, entry)
	}
	return model.ID, nil
}

func (s *Store) GetLatestActionSignal(ctx context.Context, actionID int64, types ...core.SignalType) (*core.ActionSignal, error) {
	var model ActionSignalModel
	q := s.orm.WithContext(ctx).Where("action_id = ?", actionID)
	if len(types) > 0 {
		strs := make([]string, len(types))
		for i, t := range types {
			strs[i] = string(t)
		}
		q = q.Where("type IN ?", strs)
	}
	err := q.Order("id DESC").First(&model).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil // not found is normal — signals are optional
		}
		return nil, err
	}
	return model.toCore(), nil
}

func (s *Store) ListActionSignals(ctx context.Context, actionID int64) ([]*core.ActionSignal, error) {
	var models []ActionSignalModel
	err := s.orm.WithContext(ctx).
		Where("action_id = ?", actionID).
		Order("id ASC").
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	out := make([]*core.ActionSignal, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) ListActionSignalsByType(ctx context.Context, actionID int64, types ...core.SignalType) ([]*core.ActionSignal, error) {
	var models []ActionSignalModel
	q := s.orm.WithContext(ctx).Where("action_id = ?", actionID)
	if len(types) > 0 {
		strs := make([]string, len(types))
		for i, t := range types {
			strs[i] = string(t)
		}
		q = q.Where("type IN ?", strs)
	}
	err := q.Order("id ASC").Find(&models).Error
	if err != nil {
		return nil, err
	}
	out := make([]*core.ActionSignal, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) CountActionSignals(ctx context.Context, actionID int64, types ...core.SignalType) (int, error) {
	var count int64
	q := s.orm.WithContext(ctx).Model(&ActionSignalModel{}).Where("action_id = ?", actionID)
	if len(types) > 0 {
		strs := make([]string, len(types))
		for i, t := range types {
			strs[i] = string(t)
		}
		q = q.Where("type IN ?", strs)
	}
	if err := q.Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

func (s *Store) ListPendingHumanActions(ctx context.Context, workItemID int64) ([]*core.Action, error) {
	var models []ActionModel
	err := s.orm.WithContext(ctx).
		Where("work_item_id = ? AND status IN ?", workItemID, []string{
			string(core.ActionBlocked),
			string(core.ActionWaitingGate),
		}).
		Order("position ASC").
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	out := make([]*core.Action, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) ListAllPendingHumanActions(ctx context.Context) ([]*core.Action, error) {
	var models []ActionModel
	err := s.orm.WithContext(ctx).
		Where("status IN ?", []string{
			string(core.ActionBlocked),
			string(core.ActionWaitingGate),
		}).
		Order("work_item_id ASC, position ASC").
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	out := make([]*core.Action, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

// ── Probe signal queries (merged from RunProbeStore) ──

var probeSignalTypes = []string{
	string(core.SignalProbeRequest),
	string(core.SignalProbeResponse),
}

func (s *Store) ListProbeSignalsByRun(ctx context.Context, runID int64) ([]*core.ActionSignal, error) {
	var models []ActionSignalModel
	err := s.orm.WithContext(ctx).
		Where("run_id = ? AND type IN ?", runID, probeSignalTypes).
		Order("id ASC").
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	out := make([]*core.ActionSignal, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) GetLatestProbeSignal(ctx context.Context, runID int64) (*core.ActionSignal, error) {
	var model ActionSignalModel
	err := s.orm.WithContext(ctx).
		Where("run_id = ? AND type IN ?", runID, probeSignalTypes).
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

func (s *Store) GetActiveProbeSignal(ctx context.Context, runID int64) (*core.ActionSignal, error) {
	var model ActionSignalModel
	err := s.orm.WithContext(ctx).
		Where("run_id = ? AND type = ? AND json_extract(payload, '$.status') NOT IN (?, ?, ?, ?)",
			runID,
			string(core.SignalProbeRequest),
			"answered", "timeout", "unreachable", "failed",
		).
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

func (s *Store) UpdateProbeSignal(ctx context.Context, sig *core.ActionSignal) error {
	model := actionSignalModelFromCore(sig)
	result := s.orm.WithContext(ctx).Model(&ActionSignalModel{}).
		Where("id = ?", sig.ID).
		Updates(map[string]any{
			"type":    model.Type,
			"source":  model.Source,
			"summary": model.Summary,
			"content": model.Content,
			"payload": model.Payload,
			"actor":   model.Actor,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (s *Store) GetRunProbeRoute(ctx context.Context, runID int64) (*core.RunProbeRoute, error) {
	type probeRouteRow struct {
		RunID           int64      `gorm:"column:run_id"`
		WorkItemID      int64      `gorm:"column:work_item_id"`
		ActionID        int64      `gorm:"column:action_id"`
		AgentContextID  *int64     `gorm:"column:agent_context_id"`
		SessionID       string     `gorm:"column:session_id"`
		OwnerID         string     `gorm:"column:owner_id"`
		OwnerLastSeenAt *time.Time `gorm:"column:worker_last_seen_at"`
	}

	var row probeRouteRow
	err := s.orm.WithContext(ctx).
		Table("runs e").
		Select("e.id AS run_id, e.work_item_id, e.action_id, e.agent_context_id, COALESCE(ac.session_id, '') AS session_id, COALESCE(ac.worker_id, '') AS owner_id, ac.worker_last_seen_at").
		Joins("LEFT JOIN agent_contexts ac ON ac.id = e.agent_context_id").
		Where("e.id = ?", runID).
		Scan(&row).Error
	if err != nil {
		return nil, fmt.Errorf("get run probe route: %w", err)
	}
	if row.RunID == 0 {
		return nil, core.ErrNotFound
	}
	return &core.RunProbeRoute{
		RunID:           row.RunID,
		WorkItemID:      row.WorkItemID,
		ActionID:        row.ActionID,
		AgentContextID:  row.AgentContextID,
		SessionID:       row.SessionID,
		OwnerID:         row.OwnerID,
		OwnerLastSeenAt: row.OwnerLastSeenAt,
	}, nil
}
