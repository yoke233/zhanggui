package sqlite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"gorm.io/gorm"
)

func (s *Store) CreateEvent(ctx context.Context, e *core.Event) (int64, error) {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	model := eventModelFromCore(e)
	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, fmt.Errorf("insert event: %w", err)
	}
	e.ID = model.ID
	return model.ID, nil
}

func (s *Store) ListEvents(ctx context.Context, filter core.EventFilter) ([]*core.Event, error) {
	query := s.orm.WithContext(ctx).Model(&EventModel{})
	if filter.IssueID != nil {
		query = query.Where("issue_id = ?", *filter.IssueID)
	}
	if filter.StepID != nil {
		query = query.Where("step_id = ?", *filter.StepID)
	}
	if filter.ExecID != nil {
		query = query.Where("exec_id = ?", *filter.ExecID)
	}
	if strings.TrimSpace(filter.SessionID) != "" {
		query = query.Where("json_extract(data, '$.session_id') = ?", strings.TrimSpace(filter.SessionID))
	}
	if len(filter.Types) > 0 {
		types := make([]string, 0, len(filter.Types))
		for _, t := range filter.Types {
			types = append(types, string(t))
		}
		query = query.Where("type IN ?", types)
	}
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	var models []EventModel
	if err := query.Order("id ASC").Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	events := make([]*core.Event, 0, len(models))
	for i := range models {
		events = append(events, models[i].toCore())
	}
	return events, nil
}

func (s *Store) GetLatestExecutionEventTime(ctx context.Context, execID int64, eventType core.EventType) (*time.Time, error) {
	var model EventModel
	err := s.orm.WithContext(ctx).
		Where("exec_id = ? AND type = ?", execID, string(eventType)).
		Order("timestamp DESC").
		Limit(1).
		First(&model).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("get latest execution event time: %w", err)
	}
	return &model.Timestamp, nil
}
