package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"gorm.io/gorm"
)

func (s *Store) CreateWorkItemAttachment(ctx context.Context, att *core.WorkItemAttachment) (int64, error) {
	now := time.Now().UTC()
	model := workItemAttachmentModelFromCore(att)
	model.CreatedAt = now
	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, fmt.Errorf("insert issue attachment: %w", err)
	}
	att.ID = model.ID
	att.CreatedAt = now
	return model.ID, nil
}

func (s *Store) GetWorkItemAttachment(ctx context.Context, id int64) (*core.WorkItemAttachment, error) {
	var model WorkItemAttachmentModel
	err := s.orm.WithContext(ctx).First(&model, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("get issue attachment %d: %w", id, err)
	}
	return model.toCore(), nil
}

func (s *Store) ListWorkItemAttachments(ctx context.Context, issueID int64) ([]*core.WorkItemAttachment, error) {
	var models []WorkItemAttachmentModel
	err := s.orm.WithContext(ctx).Where("issue_id = ?", issueID).Order("id ASC").Find(&models).Error
	if err != nil {
		return nil, fmt.Errorf("list issue attachments for issue %d: %w", issueID, err)
	}
	result := make([]*core.WorkItemAttachment, len(models))
	for i := range models {
		result[i] = models[i].toCore()
	}
	return result, nil
}

func (s *Store) DeleteWorkItemAttachment(ctx context.Context, id int64) error {
	tx := s.orm.WithContext(ctx).Delete(&WorkItemAttachmentModel{}, id)
	if tx.Error != nil {
		return fmt.Errorf("delete issue attachment %d: %w", id, tx.Error)
	}
	if tx.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}
