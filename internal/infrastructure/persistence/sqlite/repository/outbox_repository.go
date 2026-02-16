package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"zhanggui/internal/errs"
	"zhanggui/internal/infrastructure/persistence/sqlite/model"
	"zhanggui/internal/ports"
)

type OutboxRepository struct {
	db *gorm.DB
}

func NewOutboxRepository(db *gorm.DB) *OutboxRepository {
	return &OutboxRepository{db: db}
}

func (r *OutboxRepository) dbFromContext(ctx context.Context) (*gorm.DB, error) {
	if ctx == nil {
		return nil, errors.New("context is required")
	}

	tx := ports.TxFromContext(ctx)
	if tx == nil {
		return r.db.WithContext(ctx), nil
	}

	gormTx, ok := tx.(*gorm.DB)
	if !ok || gormTx == nil {
		return nil, fmt.Errorf("invalid tx in context: %T", tx)
	}
	return gormTx.WithContext(ctx), nil
}

func (r *OutboxRepository) ListIssues(ctx context.Context, filter ports.OutboxIssueFilter) ([]ports.OutboxIssue, error) {
	db, err := r.dbFromContext(ctx)
	if err != nil {
		return nil, err
	}

	query := db.Model(&model.Issue{})
	if !filter.IncludeClosed {
		query = query.Where("is_closed = ?", false)
	}
	if assignee := strings.TrimSpace(filter.Assignee); assignee != "" {
		query = query.Where("assignee = ?", assignee)
	}
	if len(filter.IncludeLabels) > 0 {
		sub := db.Model(&model.IssueLabel{}).
			Select("issue_id").
			Where("label IN ?", filter.IncludeLabels).
			Group("issue_id").
			Having("count(distinct label) = ?", len(filter.IncludeLabels))
		query = query.Where("issue_id IN (?)", sub)
	}
	if len(filter.ExcludeLabels) > 0 {
		sub := db.Model(&model.IssueLabel{}).
			Select("issue_id").
			Where("label IN ?", filter.ExcludeLabels)
		query = query.Where("issue_id NOT IN (?)", sub)
	}

	var rows []model.Issue
	if err := query.Order("issue_id asc").Find(&rows).Error; err != nil {
		return nil, errs.Wrap(err, "query issues")
	}

	items := make([]ports.OutboxIssue, 0, len(rows))
	for _, row := range rows {
		items = append(items, mapIssue(row))
	}
	return items, nil
}

func (r *OutboxRepository) GetIssue(ctx context.Context, issueID uint64) (ports.OutboxIssue, error) {
	db, err := r.dbFromContext(ctx)
	if err != nil {
		return ports.OutboxIssue{}, err
	}
	return getIssueByID(db, issueID)
}

func (r *OutboxRepository) ListIssueLabels(ctx context.Context, issueID uint64) ([]string, error) {
	db, err := r.dbFromContext(ctx)
	if err != nil {
		return nil, err
	}

	var rows []model.IssueLabel
	if err := db.
		Where("issue_id = ?", issueID).
		Order("label asc").
		Find(&rows).Error; err != nil {
		return nil, errs.Wrap(err, "query issue labels")
	}

	labels := make([]string, 0, len(rows))
	for _, row := range rows {
		labels = append(labels, row.Label)
	}
	return labels, nil
}

func (r *OutboxRepository) ListIssueEvents(ctx context.Context, issueID uint64) ([]ports.OutboxEvent, error) {
	db, err := r.dbFromContext(ctx)
	if err != nil {
		return nil, err
	}
	return listIssueEvents(db, issueID)
}

func (r *OutboxRepository) ListEventsAfter(ctx context.Context, afterEventID uint64, limit int) ([]ports.OutboxEvent, error) {
	db, err := r.dbFromContext(ctx)
	if err != nil {
		return nil, err
	}

	query := db.Model(&model.Event{}).Where("event_id > ?", afterEventID).Order("event_id asc")
	if limit > 0 {
		query = query.Limit(limit)
	}

	var rows []model.Event
	if err := query.Find(&rows).Error; err != nil {
		return nil, errs.Wrap(err, "query events")
	}

	items := make([]ports.OutboxEvent, 0, len(rows))
	for _, row := range rows {
		items = append(items, ports.OutboxEvent{
			EventID:   row.EventID,
			IssueID:   row.IssueID,
			Actor:     row.Actor,
			Body:      row.Body,
			CreatedAt: row.CreatedAt,
		})
	}
	return items, nil
}

func (r *OutboxRepository) ListQualityEvents(ctx context.Context, issueID uint64, limit int) ([]ports.OutboxQualityEvent, error) {
	db, err := r.dbFromContext(ctx)
	if err != nil {
		return nil, err
	}

	query := db.Model(&model.QualityEvent{}).Where("issue_id = ?", issueID).Order("quality_event_id desc")
	if limit > 0 {
		query = query.Limit(limit)
	}

	var rows []model.QualityEvent
	if err := query.Find(&rows).Error; err != nil {
		return nil, errs.Wrap(err, "query quality events")
	}

	items := make([]ports.OutboxQualityEvent, 0, len(rows))
	for _, row := range rows {
		items = append(items, ports.OutboxQualityEvent{
			QualityEventID:  row.QualityEventID,
			IssueID:         row.IssueID,
			IdempotencyKey:  row.IdempotencyKey,
			Source:          row.Source,
			ExternalEventID: row.ExternalEventID,
			Category:        row.Category,
			Result:          row.Result,
			Actor:           row.Actor,
			Summary:         row.Summary,
			EvidenceJSON:    row.EvidenceJSON,
			PayloadJSON:     row.PayloadJSON,
			IngestedAt:      row.IngestedAt,
		})
	}
	return items, nil
}

func (r *OutboxRepository) CreateIssue(ctx context.Context, issue ports.OutboxIssue, labels []string) (ports.OutboxIssue, error) {
	if ports.TxFromContext(ctx) != nil {
		db, err := r.dbFromContext(ctx)
		if err != nil {
			return ports.OutboxIssue{}, err
		}

		row := model.Issue{
			Title:     issue.Title,
			Body:      issue.Body,
			Assignee:  issue.Assignee,
			IsClosed:  issue.IsClosed,
			CreatedAt: issue.CreatedAt,
			UpdatedAt: issue.UpdatedAt,
			ClosedAt:  issue.ClosedAt,
		}
		if err := db.Create(&row).Error; err != nil {
			return ports.OutboxIssue{}, errs.Wrap(err, "insert issue")
		}

		if len(labels) > 0 {
			labelRows := make([]model.IssueLabel, 0, len(labels))
			for _, label := range labels {
				labelRows = append(labelRows, model.IssueLabel{
					IssueID: row.IssueID,
					Label:   label,
				})
			}

			if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&labelRows).Error; err != nil {
				return ports.OutboxIssue{}, errs.Wrap(err, "insert issue labels")
			}
		}

		return mapIssue(row), nil
	}

	var created ports.OutboxIssue
	if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txCtx := ports.WithTxContext(ctx, tx)
		row, err := r.CreateIssue(txCtx, issue, labels)
		if err != nil {
			return err
		}
		created = row
		return nil
	}); err != nil {
		return ports.OutboxIssue{}, err
	}
	return created, nil
}

func (r *OutboxRepository) SetIssueAssignee(ctx context.Context, issueID uint64, assignee string, updatedAt string) error {
	db, err := r.dbFromContext(ctx)
	if err != nil {
		return err
	}

	if err := db.Model(&model.Issue{}).
		Where("issue_id = ?", issueID).
		Updates(map[string]any{
			"assignee":   assignee,
			"updated_at": updatedAt,
		}).Error; err != nil {
		return errs.Wrap(err, "update issue assignee")
	}
	return nil
}

func (r *OutboxRepository) UpdateIssueUpdatedAt(ctx context.Context, issueID uint64, updatedAt string) error {
	db, err := r.dbFromContext(ctx)
	if err != nil {
		return err
	}

	if err := db.Model(&model.Issue{}).
		Where("issue_id = ?", issueID).
		Update("updated_at", updatedAt).Error; err != nil {
		return errs.Wrap(err, "update issue updated_at")
	}
	return nil
}

func (r *OutboxRepository) MarkIssueClosed(ctx context.Context, issueID uint64, closedAt string) error {
	db, err := r.dbFromContext(ctx)
	if err != nil {
		return err
	}

	if err := db.Model(&model.Issue{}).
		Where("issue_id = ?", issueID).
		Updates(map[string]any{
			"is_closed":  true,
			"closed_at":  closedAt,
			"updated_at": closedAt,
		}).Error; err != nil {
		return errs.Wrap(err, "close issue")
	}
	return nil
}

func (r *OutboxRepository) ReplaceStateLabel(ctx context.Context, issueID uint64, stateLabel string) error {
	if strings.TrimSpace(stateLabel) == "" {
		return nil
	}

	if ports.TxFromContext(ctx) != nil {
		db, err := r.dbFromContext(ctx)
		if err != nil {
			return err
		}

		if err := db.Where("issue_id = ? AND label LIKE ?", issueID, "state:%").Delete(&model.IssueLabel{}).Error; err != nil {
			return errs.Wrap(err, "delete old state labels")
		}

		row := model.IssueLabel{
			IssueID: issueID,
			Label:   stateLabel,
		}
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return errs.Wrap(err, "insert state label")
		}
		return nil
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txCtx := ports.WithTxContext(ctx, tx)
		return r.ReplaceStateLabel(txCtx, issueID, stateLabel)
	})
}

func (r *OutboxRepository) AddIssueLabel(ctx context.Context, issueID uint64, label string) error {
	label = strings.TrimSpace(label)
	if label == "" {
		return nil
	}

	if ports.TxFromContext(ctx) != nil {
		db, err := r.dbFromContext(ctx)
		if err != nil {
			return err
		}

		row := model.IssueLabel{
			IssueID: issueID,
			Label:   label,
		}
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
			return errs.Wrap(err, "insert issue label")
		}
		return nil
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txCtx := ports.WithTxContext(ctx, tx)
		return r.AddIssueLabel(txCtx, issueID, label)
	})
}

func (r *OutboxRepository) RemoveIssueLabel(ctx context.Context, issueID uint64, label string) error {
	label = strings.TrimSpace(label)
	if label == "" {
		return nil
	}

	if ports.TxFromContext(ctx) != nil {
		db, err := r.dbFromContext(ctx)
		if err != nil {
			return err
		}

		if err := db.Where("issue_id = ? AND label = ?", issueID, label).Delete(&model.IssueLabel{}).Error; err != nil {
			return errs.Wrap(err, "delete issue label")
		}
		return nil
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txCtx := ports.WithTxContext(ctx, tx)
		return r.RemoveIssueLabel(txCtx, issueID, label)
	})
}

func (r *OutboxRepository) HasIssueLabel(ctx context.Context, issueID uint64, label string) (bool, error) {
	db, err := r.dbFromContext(ctx)
	if err != nil {
		return false, err
	}

	var count int64
	if err := db.Model(&model.IssueLabel{}).
		Where("issue_id = ? AND label = ?", issueID, label).
		Count(&count).Error; err != nil {
		return false, errs.Wrap(err, "count issue label")
	}
	return count > 0, nil
}

func (r *OutboxRepository) AppendEvent(ctx context.Context, input ports.OutboxEventCreate) error {
	db, err := r.dbFromContext(ctx)
	if err != nil {
		return err
	}

	row := model.Event{
		IssueID:   input.IssueID,
		Actor:     input.Actor,
		Body:      input.Body,
		CreatedAt: input.CreatedAt,
	}
	if err := db.Create(&row).Error; err != nil {
		return errs.Wrap(err, "insert event")
	}
	return nil
}

func (r *OutboxRepository) CreateQualityEvent(ctx context.Context, input ports.OutboxQualityEventCreate) (bool, error) {
	if ports.TxFromContext(ctx) != nil {
		db, err := r.dbFromContext(ctx)
		if err != nil {
			return false, err
		}

		row := model.QualityEvent{
			IssueID:         input.IssueID,
			IdempotencyKey:  input.IdempotencyKey,
			Source:          input.Source,
			ExternalEventID: input.ExternalEventID,
			Category:        input.Category,
			Result:          input.Result,
			Actor:           input.Actor,
			Summary:         input.Summary,
			EvidenceJSON:    input.EvidenceJSON,
			PayloadJSON:     input.PayloadJSON,
			IngestedAt:      input.IngestedAt,
		}
		result := db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "idempotency_key"}},
			DoNothing: true,
		}).Create(&row)
		if result.Error != nil {
			return false, errs.Wrap(result.Error, "insert quality event")
		}
		return result.RowsAffected > 0, nil
	}

	inserted := false
	if err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txCtx := ports.WithTxContext(ctx, tx)
		ok, err := r.CreateQualityEvent(txCtx, input)
		if err != nil {
			return err
		}
		inserted = ok
		return nil
	}); err != nil {
		return false, err
	}
	return inserted, nil
}

func getIssueByID(db *gorm.DB, issueID uint64) (ports.OutboxIssue, error) {
	var row model.Issue
	if err := db.Where("issue_id = ?", issueID).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ports.OutboxIssue{}, ports.ErrIssueNotFound
		}
		return ports.OutboxIssue{}, errs.Wrap(err, "query issue")
	}
	return mapIssue(row), nil
}

func listIssueEvents(db *gorm.DB, issueID uint64) ([]ports.OutboxEvent, error) {
	var rows []model.Event
	if err := db.
		Where("issue_id = ?", issueID).
		Order("event_id asc").
		Find(&rows).Error; err != nil {
		return nil, errs.Wrap(err, "query events")
	}

	items := make([]ports.OutboxEvent, 0, len(rows))
	for _, row := range rows {
		items = append(items, ports.OutboxEvent{
			EventID:   row.EventID,
			IssueID:   row.IssueID,
			Actor:     row.Actor,
			Body:      row.Body,
			CreatedAt: row.CreatedAt,
		})
	}
	return items, nil
}

func mapIssue(row model.Issue) ports.OutboxIssue {
	return ports.OutboxIssue{
		IssueID:   row.IssueID,
		Title:     row.Title,
		Body:      row.Body,
		Assignee:  row.Assignee,
		IsClosed:  row.IsClosed,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
		ClosedAt:  row.ClosedAt,
	}
}
