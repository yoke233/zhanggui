package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"gorm.io/gorm"
)

func (s *Store) AppendJournal(ctx context.Context, entry *core.JournalEntry) (int64, error) {
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	model := journalModelFromCore(entry)
	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, fmt.Errorf("insert journal entry: %w", err)
	}
	entry.ID = model.ID
	return model.ID, nil
}

func (s *Store) BatchAppendJournal(ctx context.Context, entries []*core.JournalEntry) error {
	if len(entries) == 0 {
		return nil
	}
	now := time.Now().UTC()
	models := make([]*JournalModel, 0, len(entries))
	for _, e := range entries {
		if e.CreatedAt.IsZero() {
			e.CreatedAt = now
		}
		models = append(models, journalModelFromCore(e))
	}
	if err := s.orm.WithContext(ctx).CreateInBatches(models, 50).Error; err != nil {
		return fmt.Errorf("batch insert journal entries: %w", err)
	}
	for i, m := range models {
		entries[i].ID = m.ID
	}
	return nil
}

func (s *Store) ListJournal(ctx context.Context, filter core.JournalFilter) ([]*core.JournalEntry, error) {
	q := s.orm.WithContext(ctx).Model(&JournalModel{})
	q = applyJournalFilter(q, filter)
	q = q.Order("created_at ASC, id ASC")
	if filter.Limit > 0 {
		q = q.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		q = q.Offset(filter.Offset)
	}

	var models []JournalModel
	if err := q.Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list journal: %w", err)
	}
	out := make([]*core.JournalEntry, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) CountJournal(ctx context.Context, filter core.JournalFilter) (int, error) {
	q := s.orm.WithContext(ctx).Model(&JournalModel{})
	q = applyJournalFilter(q, filter)
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count journal: %w", err)
	}
	return int(count), nil
}

func (s *Store) GetLatestSignal(ctx context.Context, actionID int64, signalTypes ...string) (*core.JournalEntry, error) {
	q := s.orm.WithContext(ctx).
		Where("action_id = ? AND kind = ?", actionID, string(core.JournalSignal))
	if len(signalTypes) > 0 {
		q = q.Where("json_extract(payload, '$.signal_type') IN ?", signalTypes)
	}
	var model JournalModel
	if err := q.Order("id DESC").First(&model).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("get latest signal: %w", err)
	}
	return model.toCore(), nil
}

func (s *Store) CountSignals(ctx context.Context, actionID int64, signalTypes ...string) (int, error) {
	q := s.orm.WithContext(ctx).Model(&JournalModel{}).
		Where("action_id = ? AND kind = ?", actionID, string(core.JournalSignal))
	if len(signalTypes) > 0 {
		q = q.Where("json_extract(payload, '$.signal_type') IN ?", signalTypes)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count signals: %w", err)
	}
	return int(count), nil
}

func applyJournalFilter(q *gorm.DB, f core.JournalFilter) *gorm.DB {
	if f.WorkItemID != nil {
		q = q.Where("work_item_id = ?", *f.WorkItemID)
	}
	if f.ActionID != nil {
		q = q.Where("action_id = ?", *f.ActionID)
	}
	if f.RunID != nil {
		q = q.Where("run_id = ?", *f.RunID)
	}
	if len(f.Kinds) > 0 {
		strs := make([]string, len(f.Kinds))
		for i, k := range f.Kinds {
			strs[i] = string(k)
		}
		q = q.Where("kind IN ?", strs)
	}
	if len(f.Sources) > 0 {
		strs := make([]string, len(f.Sources))
		for i, s := range f.Sources {
			strs[i] = string(s)
		}
		q = q.Where("source IN ?", strs)
	}
	if f.Since != nil {
		q = q.Where("created_at >= ?", *f.Since)
	}
	if f.Until != nil {
		q = q.Where("created_at <= ?", *f.Until)
	}
	return q
}
