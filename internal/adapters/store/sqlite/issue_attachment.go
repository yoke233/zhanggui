package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"gorm.io/gorm"
)

func (s *Store) CreateIssueAttachment(ctx context.Context, att *core.IssueAttachment) (int64, error) {
	now := time.Now().UTC()
	model := issueAttachmentModelFromCore(att)
	model.CreatedAt = now
	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, fmt.Errorf("insert issue attachment: %w", err)
	}
	att.ID = model.ID
	att.CreatedAt = now
	return model.ID, nil
}

func (s *Store) GetIssueAttachment(ctx context.Context, id int64) (*core.IssueAttachment, error) {
	var model IssueAttachmentModel
	err := s.orm.WithContext(ctx).First(&model, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("get issue attachment %d: %w", id, err)
	}
	return model.toCore(), nil
}

func (s *Store) ListIssueAttachments(ctx context.Context, issueID int64) ([]*core.IssueAttachment, error) {
	var models []IssueAttachmentModel
	err := s.orm.WithContext(ctx).Where("issue_id = ?", issueID).Order("id ASC").Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("list issue attachments for issue %d: %w", issueID, err)
	}
	result := make([]*core.IssueAttachment, len(models))
	for i := range models {
		result[i] = models[i].toCore()
	}
	return result, nil
}

func (s *Store) DeleteIssueAttachment(ctx context.Context, id int64) error {
	tx := s.orm.WithContext(ctx).Delete(&IssueAttachmentModel{}, id)
	if tx.Error != nil {
		return fmt.Errorf("delete issue attachment %d: %w", id, tx.Error)
	}
	if tx.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}
