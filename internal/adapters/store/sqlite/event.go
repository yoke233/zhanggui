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
	if e.Category == "" {
		e.Category = core.EventCategoryDomain
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
	if filter.WorkItemID != nil {
		query = query.Where("issue_id = ?", *filter.WorkItemID)
	}
	if filter.ActionID != nil {
		query = query.Where("step_id = ?", *filter.ActionID)
	}
	if filter.RunID != nil {
		query = query.Where("exec_id = ?", *filter.RunID)
	}
	if filter.ThreadID != nil {
		query = query.Where("json_extract(data, '$.thread_id') = ?", *filter.ThreadID)
	}
	if strings.TrimSpace(filter.SessionID) != "" {
		query = query.Where("json_extract(data, '$.session_id') = ?", strings.TrimSpace(filter.SessionID))
	}
	if strings.TrimSpace(filter.Category) != "" {
		query = query.Where("category = ?", strings.TrimSpace(filter.Category))
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

func (s *Store) GetLatestRunEventTime(ctx context.Context, execID int64, eventType core.EventType) (*time.Time, error) {
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

// ── Tool Call Audit methods (stored as category='tool_audit' in event_log) ──

func (s *Store) CreateToolCallAudit(ctx context.Context, audit *core.ToolCallAudit) (int64, error) {
	event := audit.ToEvent()
	id, err := s.CreateEvent(ctx, event)
	if err != nil {
		return 0, fmt.Errorf("insert tool call audit: %w", err)
	}
	audit.ID = id
	audit.CreatedAt = event.Timestamp
	return id, nil
}

func (s *Store) GetToolCallAudit(ctx context.Context, id int64) (*core.ToolCallAudit, error) {
	var model EventModel
	if err := s.orm.WithContext(ctx).
		Where("id = ? AND category = ?", id, core.EventCategoryToolAudit).
		First(&model).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("get tool call audit: %w", err)
	}
	event := model.toCore()
	return core.ToolCallAuditFromEvent(event), nil
}

func (s *Store) GetToolCallAuditByToolCallID(ctx context.Context, runID int64, toolCallID string) (*core.ToolCallAudit, error) {
	var model EventModel
	if err := s.orm.WithContext(ctx).
		Where("exec_id = ? AND category = ? AND json_extract(data, '$.tool_call_id') = ?",
			runID, core.EventCategoryToolAudit, toolCallID).
		Order("id DESC").
		First(&model).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("get tool call audit by tool_call_id: %w", err)
	}
	event := model.toCore()
	return core.ToolCallAuditFromEvent(event), nil
}

func (s *Store) ListToolCallAuditsByRun(ctx context.Context, runID int64) ([]*core.ToolCallAudit, error) {
	var models []EventModel
	if err := s.orm.WithContext(ctx).
		Where("exec_id = ? AND category = ?", runID, core.EventCategoryToolAudit).
		Order("id ASC").
		Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list tool call audits by execution: %w", err)
	}
	out := make([]*core.ToolCallAudit, 0, len(models))
	for i := range models {
		event := models[i].toCore()
		out = append(out, core.ToolCallAuditFromEvent(event))
	}
	return out, nil
}

func (s *Store) UpdateToolCallAudit(ctx context.Context, audit *core.ToolCallAudit) error {
	event := audit.ToEvent()
	model := eventModelFromCore(event)
	result := s.orm.WithContext(ctx).Model(&EventModel{}).
		Where("id = ? AND category = ?", audit.ID, core.EventCategoryToolAudit).
		Updates(map[string]any{
			"data":      model.Data,
			"issue_id":  model.IssueID,
			"step_id":   model.StepID,
			"exec_id":   model.ExecID,
			"timestamp": model.Timestamp,
		})
	if result.Error != nil {
		return fmt.Errorf("update tool call audit: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}
