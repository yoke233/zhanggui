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
	depsJSON, err := marshalJSON(issue.DependsOn)
	if err != nil {
		return 0, fmt.Errorf("marshal depends_on: %w", err)
	}
	metadataJSON, err := marshalJSON(issue.Metadata)
	if err != nil {
		return 0, fmt.Errorf("marshal metadata: %w", err)
	}

	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO issues (project_id, resource_binding_id, title, body, status, priority, labels, depends_on, metadata, archived_at, created_at, updated_at)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		issue.ProjectID, issue.ResourceBindingID, title, issue.Body, string(status), string(priority),
		labelsJSON, depsJSON, metadataJSON, issue.ArchivedAt, now, now)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	issue.ID = id
	issue.CreatedAt = now
	issue.UpdatedAt = now
	return id, nil
}

func (s *Store) GetIssue(ctx context.Context, id int64) (*core.Issue, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("store is not initialized")
	}
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, resource_binding_id, title, body, status, priority, labels, depends_on, metadata, archived_at, created_at, updated_at
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
	if filter.Archived != nil {
		if *filter.Archived {
			where = append(where, `archived_at IS NOT NULL`)
		} else {
			where = append(where, `archived_at IS NULL`)
		}
	}

	query := `SELECT id, project_id, resource_binding_id, title, body, status, priority, labels, depends_on, metadata, archived_at, created_at, updated_at FROM issues`
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
	depsJSON, err := marshalJSON(issue.DependsOn)
	if err != nil {
		return fmt.Errorf("marshal depends_on: %w", err)
	}
	metadataJSON, err := marshalJSON(issue.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE issues SET project_id = ?, resource_binding_id = ?, title = ?, body = ?, status = ?, priority = ?,
         labels = ?, depends_on = ?, metadata = ?, updated_at = ?
         WHERE id = ?`,
		issue.ProjectID, issue.ResourceBindingID, issue.Title, issue.Body, string(issue.Status), string(issue.Priority),
		labelsJSON, depsJSON, metadataJSON, now, issue.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return core.ErrNotFound
	}
	issue.UpdatedAt = now
	return nil
}

func (s *Store) UpdateIssueStatus(ctx context.Context, id int64, status core.IssueStatus) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("store is not initialized")
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE issues SET status = ?, updated_at = ? WHERE id = ?`,
		string(status), time.Now().UTC(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (s *Store) UpdateIssueMetadata(ctx context.Context, id int64, metadata map[string]any) error {
	meta, err := marshalJSON(metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE issues SET metadata = ?, updated_at = ? WHERE id = ?`,
		meta, time.Now().UTC(), id,
	)
	if err != nil {
		return fmt.Errorf("update issue metadata: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (s *Store) PrepareIssueRun(ctx context.Context, id int64, queuedStatus core.IssueStatus) error {
	if queuedStatus != core.IssueQueued && queuedStatus != core.IssueRunning {
		return core.ErrInvalidTransition
	}

	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE issues
		 SET status = ?, updated_at = ?
		 WHERE id = ? AND status IN (?, ?) AND archived_at IS NULL`,
		queuedStatus, now, id, core.IssueOpen, core.IssueAccepted,
	)
	if err != nil {
		return fmt.Errorf("prepare issue run: %w", err)
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		return nil
	}

	if _, err := s.GetIssue(ctx, id); err != nil {
		return err
	}
	return core.ErrInvalidTransition
}

func (s *Store) SetIssueArchived(ctx context.Context, id int64, archived bool) error {
	now := time.Now().UTC()
	var archivedAt any
	var res sql.Result
	var err error
	if archived {
		archivedAt = now
		res, err = s.db.ExecContext(ctx,
			`UPDATE issues
			 SET archived_at = ?, updated_at = ?
			 WHERE id = ? AND archived_at IS NULL AND status NOT IN (?, ?, ?)`,
			archivedAt, now, id, core.IssueQueued, core.IssueRunning, core.IssueBlocked,
		)
	} else {
		res, err = s.db.ExecContext(ctx,
			`UPDATE issues
			 SET archived_at = NULL, updated_at = ?
			 WHERE id = ? AND archived_at IS NOT NULL`,
			now, id,
		)
	}
	if err != nil {
		return fmt.Errorf("set issue archived: %w", err)
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		return nil
	}

	if _, err := s.GetIssue(ctx, id); err != nil {
		return err
	}
	return core.ErrInvalidTransition
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
	var resourceBindingID sql.NullInt64
	var labelsRaw sql.NullString
	var depsRaw sql.NullString
	var metadataRaw sql.NullString
	var archivedAt sql.NullTime

	if err := s.Scan(
		&issue.ID, &projectID, &resourceBindingID, &issue.Title, &issue.Body,
		&issue.Status, &issue.Priority, &labelsRaw, &depsRaw,
		&metadataRaw, &archivedAt, &issue.CreatedAt, &issue.UpdatedAt,
	); err != nil {
		return nil, err
	}

	if projectID.Valid {
		issue.ProjectID = &projectID.Int64
	}
	if resourceBindingID.Valid {
		issue.ResourceBindingID = &resourceBindingID.Int64
	}
	if labelsRaw.Valid && labelsRaw.String != "" {
		if err := json.Unmarshal([]byte(labelsRaw.String), &issue.Labels); err != nil {
			return nil, fmt.Errorf("unmarshal labels: %w", err)
		}
	}
	if depsRaw.Valid && depsRaw.String != "" {
		if err := json.Unmarshal([]byte(depsRaw.String), &issue.DependsOn); err != nil {
			return nil, fmt.Errorf("unmarshal depends_on: %w", err)
		}
	}
	if metadataRaw.Valid && metadataRaw.String != "" {
		if err := json.Unmarshal([]byte(metadataRaw.String), &issue.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
	}
	if archivedAt.Valid {
		t := archivedAt.Time
		issue.ArchivedAt = &t
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
