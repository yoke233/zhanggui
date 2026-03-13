package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"gorm.io/gorm"
)

func (s *Store) CreateActionSignal(ctx context.Context, sig *core.ActionSignal) (int64, error) {
	now := time.Now().UTC()
	model := actionSignalModelFromCore(sig)
	model.CreatedAt = now
	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, fmt.Errorf("insert step signal: %w", err)
	}
	sig.ID = model.ID
	sig.CreatedAt = now
	return model.ID, nil
}

func (s *Store) GetLatestActionSignal(ctx context.Context, stepID int64, types ...core.SignalType) (*core.ActionSignal, error) {
	var model ActionSignalModel
	q := s.orm.WithContext(ctx).Where("step_id = ?", stepID)
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

func (s *Store) ListActionSignals(ctx context.Context, stepID int64) ([]*core.ActionSignal, error) {
	var models []ActionSignalModel
	err := s.orm.WithContext(ctx).
		Where("step_id = ?", stepID).
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

func (s *Store) ListActionSignalsByType(ctx context.Context, stepID int64, types ...core.SignalType) ([]*core.ActionSignal, error) {
	var models []ActionSignalModel
	q := s.orm.WithContext(ctx).Where("step_id = ?", stepID)
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

func (s *Store) CountActionSignals(ctx context.Context, stepID int64, types ...core.SignalType) (int, error) {
	var count int64
	q := s.orm.WithContext(ctx).Model(&ActionSignalModel{}).Where("step_id = ?", stepID)
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

func (s *Store) ListPendingHumanActions(ctx context.Context, issueID int64) ([]*core.Action, error) {
	var models []ActionModel
	err := s.orm.WithContext(ctx).
		Where("issue_id = ? AND status IN ?", issueID, []string{
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
		Order("issue_id ASC, position ASC").
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
