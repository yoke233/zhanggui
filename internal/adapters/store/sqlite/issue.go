package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yoke233/ai-workflow/internal/core"
)

func (s *Store) CreateIssue(ctx context.Context, issue *core.Issue) (int64, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("store is not initialized")
	}
	if issue == nil {
		return 0, fmt.Errorf("issue is nil")
	}
	title := strings.TrimSpace(issue.Title)
	if title == "" {
		return 0, fmt.Errorf("title is required")
	}

	status := issue.Status
	if status == "" {
		status = core.IssueOpen
	}
	priority := issue.Priority
	if priority == "" {
		priority = core.PriorityMedium
	}

	labelsJSON, err := marshalJSON(issue.Labels)
	if err != nil {
		return 0, fmt.Errorf("marshal labels: %w", err)
	}
	metadataJSON, err := marshalJSON(issue.Metadata)
	if err != nil {
		return 0, fmt.Errorf("marshal metadata: %w", err)
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO issues (project_id, title, body, status, priority, labels, flow_id, metadata, created_at, updated_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		issue.ProjectID, title, issue.Body, string(status), string(priority),
		labelsJSON, issue.FlowID, metadataJSON)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) GetIssue(ctx context.Context, id int64) (*core.Issue, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("store is not initialized")
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, title, body, status, priority, labels, flow_id, metadata, created_at, updated_at
         FROM issues WHERE id = ?`, id)
	return scanIssue(row)
}

func (s *Store) ListIssues(ctx context.Context, filter core.IssueFilter) ([]*core.Issue, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("store is not initialized")
	}

	var where []string
	var args []any

	if filter.ProjectID != nil {
		where = append(where, "project_id = ?")
		args = append(args, *filter.ProjectID)
	}
	if filter.Status != nil {
		where = append(where, "status = ?")
		args = append(args, string(*filter.Status))
	}
	if filter.Priority != nil {
		where = append(where, "priority = ?")
		args = append(args, string(*filter.Priority))
	}

	query := `SELECT id, project_id, title, body, status, priority, labels, flow_id, metadata, created_at, updated_at FROM issues`
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY id DESC"

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	query += " LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*core.Issue
	for rows.Next() {
		issue, err := scanIssueRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, issue)
	}
	return out, rows.Err()
}

func (s *Store) UpdateIssue(ctx context.Context, issue *core.Issue) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store is not initialized")
	}
	if issue == nil {
		return fmt.Errorf("issue is nil")
	}

	labelsJSON, err := marshalJSON(issue.Labels)
	if err != nil {
		return fmt.Errorf("marshal labels: %w", err)
	}
	metadataJSON, err := marshalJSON(issue.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE issues SET project_id = ?, title = ?, body = ?, status = ?, priority = ?,
         labels = ?, flow_id = ?, metadata = ?, updated_at = CURRENT_TIMESTAMP
         WHERE id = ?`,
		issue.ProjectID, issue.Title, issue.Body, string(issue.Status), string(issue.Priority),
		labelsJSON, issue.FlowID, metadataJSON, issue.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (s *Store) UpdateIssueStatus(ctx context.Context, id int64, status core.IssueStatus) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store is not initialized")
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE issues SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		string(status), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteIssue(ctx context.Context, id int64) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store is not initialized")
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM issues WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return core.ErrNotFound
	}
	return nil
}

// scanner is an interface satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanIssueFromScanner(s scanner) (*core.Issue, error) {
	var issue core.Issue
	var projectID sql.NullInt64
	var flowID sql.NullInt64
	var labelsRaw sql.NullString
	var metadataRaw sql.NullString

	if err := s.Scan(
		&issue.ID, &projectID, &issue.Title, &issue.Body,
		&issue.Status, &issue.Priority, &labelsRaw, &flowID,
		&metadataRaw, &issue.CreatedAt, &issue.UpdatedAt,
	); err != nil {
		return nil, err
	}

	if projectID.Valid {
		issue.ProjectID = &projectID.Int64
	}
	if flowID.Valid {
		issue.FlowID = &flowID.Int64
	}
	if labelsRaw.Valid && labelsRaw.String != "" {
		if err := json.Unmarshal([]byte(labelsRaw.String), &issue.Labels); err != nil {
			return nil, fmt.Errorf("unmarshal labels: %w", err)
		}
	}
	if metadataRaw.Valid && metadataRaw.String != "" {
		if err := json.Unmarshal([]byte(metadataRaw.String), &issue.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
	}
	return &issue, nil
}

func scanIssue(row *sql.Row) (*core.Issue, error) {
	issue, err := scanIssueFromScanner(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, core.ErrNotFound
		}
		return nil, err
	}
	return issue, nil
}

func scanIssueRow(rows *sql.Rows) (*core.Issue, error) {
	return scanIssueFromScanner(rows)
}
