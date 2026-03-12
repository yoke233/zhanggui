package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
)

func (s *Store) CreateEvent(ctx context.Context, e *core.Event) (int64, error) {
	data, err := marshalJSON(e.Data)
	if err != nil {
		return 0, fmt.Errorf("marshal event data: %w", err)
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO events (type, issue_id, step_id, exec_id, data, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		e.Type, nilIfZero(e.IssueID), nilIfZero(e.StepID), nilIfZero(e.ExecID), data, e.Timestamp,
	)
	if err != nil {
		return 0, fmt.Errorf("insert event: %w", err)
	}
	id, _ := res.LastInsertId()
	e.ID = id
	return id, nil
}

func (s *Store) ListEvents(ctx context.Context, filter core.EventFilter) ([]*core.Event, error) {
	query := `SELECT id, type, issue_id, step_id, exec_id, data, timestamp FROM events`
	var conditions []string
	var args []any

	if filter.IssueID != nil {
		conditions = append(conditions, "issue_id = ?")
		args = append(args, *filter.IssueID)
	}
	if filter.StepID != nil {
		conditions = append(conditions, "step_id = ?")
		args = append(args, *filter.StepID)
	}
	if filter.ExecID != nil {
		conditions = append(conditions, "exec_id = ?")
		args = append(args, *filter.ExecID)
	}
	if strings.TrimSpace(filter.SessionID) != "" {
		conditions = append(conditions, "json_extract(data, '$.session_id') = ?")
		args = append(args, strings.TrimSpace(filter.SessionID))
	}
	if len(filter.Types) > 0 {
		placeholders := make([]string, len(filter.Types))
		for i, t := range filter.Types {
			placeholders[i] = "?"
			args = append(args, t)
		}
		conditions = append(conditions, fmt.Sprintf("type IN (%s)", strings.Join(placeholders, ",")))
	}

	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY id"
	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}
	if filter.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", filter.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	var events []*core.Event
	for rows.Next() {
		e := &core.Event{}
		var issueID, stepID, execID sql.NullInt64
		var data sql.NullString
		if err := rows.Scan(&e.ID, &e.Type, &issueID, &stepID, &execID, &data, &e.Timestamp); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		if issueID.Valid {
			e.IssueID = issueID.Int64
		}
		if stepID.Valid {
			e.StepID = stepID.Int64
		}
		if execID.Valid {
			e.ExecID = execID.Int64
		}
		if data.Valid {
			_ = json.Unmarshal([]byte(data.String), &e.Data)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (s *Store) GetLatestExecutionEventTime(ctx context.Context, execID int64, eventType core.EventType) (*time.Time, error) {
	var raw sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT MAX(timestamp) FROM events WHERE exec_id = ? AND type = ?`,
		execID, eventType,
	).Scan(&raw)
	if err != nil {
		return nil, fmt.Errorf("get latest execution event time: %w", err)
	}
	if !raw.Valid || strings.TrimSpace(raw.String) == "" {
		return nil, nil
	}
	value, err := time.Parse(time.RFC3339Nano, raw.String)
	if err != nil {
		value, err = time.Parse("2006-01-02 15:04:05Z07:00", raw.String)
	}
	if err != nil {
		value, err = time.Parse("2006-01-02 15:04:05", raw.String)
	}
	if err != nil {
		value, err = time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", raw.String)
	}
	if err != nil {
		return nil, fmt.Errorf("parse latest execution event time %q: %w", raw.String, err)
	}
	return &value, nil
}

func nilIfZero(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

