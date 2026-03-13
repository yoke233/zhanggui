package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"gorm.io/gorm"
)

func (s *Store) CreateStepSignal(ctx context.Context, sig *core.StepSignal) (int64, error) {
	now := time.Now().UTC()
	model := stepSignalModelFromCore(sig)
	model.CreatedAt = now
	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, fmt.Errorf("insert step signal: %w", err)
	}
	sig.ID = model.ID
	sig.CreatedAt = now
	return model.ID, nil
}

func (s *Store) GetLatestStepSignal(ctx context.Context, stepID int64, types ...core.SignalType) (*core.StepSignal, error) {
	var model StepSignalModel
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

func (s *Store) ListStepSignals(ctx context.Context, stepID int64) ([]*core.StepSignal, error) {
	var models []StepSignalModel
	err := s.orm.WithContext(ctx).
		Where("step_id = ?", stepID).
		Order("id ASC").
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	out := make([]*core.StepSignal, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) ListStepSignalsByType(ctx context.Context, stepID int64, types ...core.SignalType) ([]*core.StepSignal, error) {
	var models []StepSignalModel
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
	out := make([]*core.StepSignal, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) CountStepSignals(ctx context.Context, stepID int64, types ...core.SignalType) (int, error) {
	var count int64
	q := s.orm.WithContext(ctx).Model(&StepSignalModel{}).Where("step_id = ?", stepID)
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

func (s *Store) ListPendingHumanSteps(ctx context.Context, issueID int64) ([]*core.Step, error) {
	var models []StepModel
	err := s.orm.WithContext(ctx).
		Where("issue_id = ? AND status IN ?", issueID, []string{
			string(core.StepBlocked),
			string(core.StepWaitingGate),
		}).
		Order("position ASC").
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	out := make([]*core.Step, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) ListAllPendingHumanSteps(ctx context.Context) ([]*core.Step, error) {
	var models []StepModel
	err := s.orm.WithContext(ctx).
		Where("status IN ?", []string{
			string(core.StepBlocked),
			string(core.StepWaitingGate),
		}).
		Order("issue_id ASC, position ASC").
		Find(&models).Error
	if err != nil {
		return nil, err
	}
	out := make([]*core.Step, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}
