package sqlite

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"gorm.io/gorm"
)

func (s *Store) CreateThread(ctx context.Context, thread *core.Thread) (int64, error) {
	if s == nil || s.orm == nil {
		return 0, fmt.Errorf("store is not initialized")
	}
	if thread == nil {
		return 0, fmt.Errorf("thread is nil")
	}

	title := strings.TrimSpace(thread.Title)
	if title == "" {
		return 0, fmt.Errorf("title is required")
	}

	if thread.Status == "" {
		thread.Status = core.ThreadActive
	}
	if !thread.Status.Valid() {
		return 0, fmt.Errorf("invalid thread status %q", thread.Status)
	}

	now := time.Now().UTC()
	model := threadModelFromCore(thread)
	model.Title = title
	model.CreatedAt = now
	model.UpdatedAt = now

	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, err
	}
	thread.ID = model.ID
	thread.Title = title
	thread.CreatedAt = now
	thread.UpdatedAt = now
	return model.ID, nil
}

func (s *Store) GetThread(ctx context.Context, id int64) (*core.Thread, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}

	var model ThreadModel
	err := s.orm.WithContext(ctx).First(&model, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, err
	}
	return model.toCore(), nil
}

func (s *Store) ListThreads(ctx context.Context, filter core.ThreadFilter) ([]*core.Thread, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}

	query := s.orm.WithContext(ctx).Model(&ThreadModel{})

	if filter.Status != nil {
		query = query.Where("status = ?", string(*filter.Status))
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	var models []ThreadModel
	if err := query.Order("id DESC").Limit(limit).Offset(offset).Find(&models).Error; err != nil {
		return nil, err
	}

	out := make([]*core.Thread, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) UpdateThread(ctx context.Context, thread *core.Thread) error {
	if s == nil || s.orm == nil {
		return fmt.Errorf("store is not initialized")
	}
	if thread == nil {
		return fmt.Errorf("thread is nil")
	}
	if thread.Status == "" {
		thread.Status = core.ThreadActive
	}
	if !thread.Status.Valid() {
		return fmt.Errorf("invalid thread status %q", thread.Status)
	}

	now := time.Now().UTC()
	model := threadModelFromCore(thread)
	model.UpdatedAt = now

	result := s.orm.WithContext(ctx).Model(&ThreadModel{}).
		Where("id = ?", thread.ID).
		Updates(map[string]any{
			"title":      model.Title,
			"status":     model.Status,
			"owner_id":   model.OwnerID,
			"summary":    model.Summary,
			"metadata":   model.Metadata,
			"updated_at": model.UpdatedAt,
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}

	thread.UpdatedAt = now
	return nil
}

func (s *Store) DeleteThread(ctx context.Context, id int64) error {
	if s == nil || s.orm == nil {
		return fmt.Errorf("store is not initialized")
	}

	result := s.orm.WithContext(ctx).Delete(&ThreadModel{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (s *Store) CreateThreadWithParticipants(ctx context.Context, thread *core.Thread, participants []*core.ThreadMember) error {
	if s == nil || s.orm == nil {
		return fmt.Errorf("store is not initialized")
	}
	if thread == nil {
		return fmt.Errorf("thread is nil")
	}

	title := strings.TrimSpace(thread.Title)
	if title == "" {
		return fmt.Errorf("title is required")
	}
	if thread.Status == "" {
		thread.Status = core.ThreadActive
	}
	if !thread.Status.Valid() {
		return fmt.Errorf("invalid thread status %q", thread.Status)
	}

	now := time.Now().UTC()
	model := threadModelFromCore(thread)
	model.Title = title
	model.CreatedAt = now
	model.UpdatedAt = now

	err := s.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(model).Error; err != nil {
			return err
		}

		seen := make(map[string]bool)
		for _, participant := range participants {
			if participant == nil {
				continue
			}
			userID := strings.TrimSpace(participant.UserID)
			if userID == "" || seen[userID] {
				continue
			}
			seen[userID] = true

			role := strings.TrimSpace(participant.Role)
			if role == "" {
				role = "member"
			}
			kind := participant.Kind
			if kind == "" {
				kind = "human"
			}
			memberModel := &ThreadMemberModel{
				ThreadID:     model.ID,
				Kind:         kind,
				UserID:       userID,
				Role:         role,
				JoinedAt:     now,
				LastActiveAt: now,
			}
			if err := tx.Create(memberModel).Error; err != nil {
				return err
			}
			participant.ID = memberModel.ID
			participant.ThreadID = model.ID
			participant.Kind = kind
			participant.UserID = userID
			participant.Role = role
			participant.JoinedAt = now
			participant.LastActiveAt = now
		}
		return nil
	})
	if err != nil {
		return err
	}

	thread.ID = model.ID
	thread.Title = title
	thread.CreatedAt = now
	thread.UpdatedAt = now
	return nil
}

// ---------------------------------------------------------------------------
// ThreadMessage CRUD
// ---------------------------------------------------------------------------

func (s *Store) CreateThreadMessage(ctx context.Context, msg *core.ThreadMessage) (int64, error) {
	if s == nil || s.orm == nil {
		return 0, fmt.Errorf("store is not initialized")
	}
	if msg == nil {
		return 0, fmt.Errorf("message is nil")
	}

	now := time.Now().UTC()
	model := &ThreadMessageModel{
		ThreadID:         msg.ThreadID,
		SenderID:         strings.TrimSpace(msg.SenderID),
		Role:             msg.Role,
		Content:          msg.Content,
		ReplyToMessageID: msg.ReplyToMessageID,
		Metadata:         JSONField[map[string]any]{Data: msg.Metadata},
		CreatedAt:        now,
	}
	if model.Role == "" {
		model.Role = "human"
	}

	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, err
	}
	msg.ID = model.ID
	msg.CreatedAt = now
	return model.ID, nil
}

func (s *Store) GetThreadMessage(ctx context.Context, id int64) (*core.ThreadMessage, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}

	var model ThreadMessageModel
	err := s.orm.WithContext(ctx).First(&model, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, err
	}
	return model.toCore(), nil
}

func (s *Store) ListThreadMessages(ctx context.Context, threadID int64, limit, offset int) ([]*core.ThreadMessage, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	var models []ThreadMessageModel
	if err := s.orm.WithContext(ctx).
		Where("thread_id = ?", threadID).
		Order("id ASC").
		Limit(limit).
		Offset(offset).
		Find(&models).Error; err != nil {
		return nil, err
	}

	out := make([]*core.ThreadMessage, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// ThreadMember CRUD (unified human + agent members)
// ---------------------------------------------------------------------------

func (s *Store) AddThreadMember(ctx context.Context, m *core.ThreadMember) (int64, error) {
	if s == nil || s.orm == nil {
		return 0, fmt.Errorf("store is not initialized")
	}
	if m == nil {
		return 0, fmt.Errorf("member is nil")
	}

	now := time.Now().UTC()
	if m.Kind == "" {
		m.Kind = core.ThreadMemberKindHuman
	}
	model := threadMemberModelFromCore(m)
	if model.Role == "" {
		model.Role = "member"
	}
	model.JoinedAt = now
	model.LastActiveAt = now

	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, err
	}
	m.ID = model.ID
	m.JoinedAt = now
	m.LastActiveAt = now
	return model.ID, nil
}

func (s *Store) ListThreadMembers(ctx context.Context, threadID int64) ([]*core.ThreadMember, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}

	var models []ThreadMemberModel
	if err := s.orm.WithContext(ctx).
		Where("thread_id = ?", threadID).
		Order("id ASC").
		Find(&models).Error; err != nil {
		return nil, err
	}

	out := make([]*core.ThreadMember, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) GetThreadMember(ctx context.Context, id int64) (*core.ThreadMember, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}

	var model ThreadMemberModel
	err := s.orm.WithContext(ctx).First(&model, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, err
	}
	return model.toCore(), nil
}

func (s *Store) UpdateThreadMember(ctx context.Context, m *core.ThreadMember) error {
	if s == nil || s.orm == nil {
		return fmt.Errorf("store is not initialized")
	}
	if m == nil {
		return fmt.Errorf("member is nil")
	}

	now := time.Now().UTC()
	result := s.orm.WithContext(ctx).Model(&ThreadMemberModel{}).
		Where("id = ?", m.ID).
		Updates(map[string]any{
			"kind":             m.Kind,
			"user_id":          m.UserID,
			"agent_profile_id": m.AgentProfileID,
			"role":             m.Role,
			"status":           string(m.Status),
			"agent_data":       JSONField[map[string]any]{Data: m.AgentData},
			"last_active_at":   now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	m.LastActiveAt = now
	return nil
}

func (s *Store) RemoveThreadMember(ctx context.Context, id int64) error {
	if s == nil || s.orm == nil {
		return fmt.Errorf("store is not initialized")
	}

	result := s.orm.WithContext(ctx).Delete(&ThreadMemberModel{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (s *Store) RemoveThreadMemberByUser(ctx context.Context, threadID int64, userID string) error {
	if s == nil || s.orm == nil {
		return fmt.Errorf("store is not initialized")
	}

	result := s.orm.WithContext(ctx).
		Where("thread_id = ? AND user_id = ?", threadID, strings.TrimSpace(userID)).
		Delete(&ThreadMemberModel{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// ThreadWorkItemLink CRUD
// ---------------------------------------------------------------------------

func (s *Store) CreateThreadWorkItemLink(ctx context.Context, link *core.ThreadWorkItemLink) (int64, error) {
	if s == nil || s.orm == nil {
		return 0, fmt.Errorf("store is not initialized")
	}
	if link == nil {
		return 0, fmt.Errorf("link is nil")
	}
	if link.RelationType == "" {
		link.RelationType = "related"
	}

	now := time.Now().UTC()
	model := &ThreadWorkItemLinkModel{
		ThreadID:     link.ThreadID,
		WorkItemID:   link.WorkItemID,
		RelationType: link.RelationType,
		IsPrimary:    link.IsPrimary,
		CreatedAt:    now,
	}

	err := s.orm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if link.IsPrimary {
			if err := tx.Model(&ThreadWorkItemLinkModel{}).
				Where("thread_id = ? AND is_primary = ?", link.ThreadID, true).
				Update("is_primary", false).Error; err != nil {
				return err
			}
		}
		return tx.Create(model).Error
	})
	if err != nil {
		return 0, err
	}
	link.ID = model.ID
	link.CreatedAt = now
	return model.ID, nil
}

func (s *Store) ListWorkItemsByThread(ctx context.Context, threadID int64) ([]*core.ThreadWorkItemLink, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}

	var models []ThreadWorkItemLinkModel
	if err := s.orm.WithContext(ctx).
		Where("thread_id = ?", threadID).
		Order("id ASC").
		Find(&models).Error; err != nil {
		return nil, err
	}

	out := make([]*core.ThreadWorkItemLink, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) ListThreadsByWorkItem(ctx context.Context, workItemID int64) ([]*core.ThreadWorkItemLink, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}

	var models []ThreadWorkItemLinkModel
	if err := s.orm.WithContext(ctx).
		Where("work_item_id = ?", workItemID).
		Order("id ASC").
		Find(&models).Error; err != nil {
		return nil, err
	}

	out := make([]*core.ThreadWorkItemLink, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) DeleteThreadWorkItemLink(ctx context.Context, threadID, workItemID int64) error {
	if s == nil || s.orm == nil {
		return fmt.Errorf("store is not initialized")
	}

	result := s.orm.WithContext(ctx).
		Where("thread_id = ? AND work_item_id = ?", threadID, workItemID).
		Delete(&ThreadWorkItemLinkModel{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteThreadWorkItemLinksByThread(ctx context.Context, threadID int64) error {
	if s == nil || s.orm == nil {
		return fmt.Errorf("store is not initialized")
	}
	return s.orm.WithContext(ctx).
		Where("thread_id = ?", threadID).
		Delete(&ThreadWorkItemLinkModel{}).Error
}

func (s *Store) DeleteThreadWorkItemLinksByWorkItem(ctx context.Context, workItemID int64) error {
	if s == nil || s.orm == nil {
		return fmt.Errorf("store is not initialized")
	}
	return s.orm.WithContext(ctx).
		Where("work_item_id = ?", workItemID).
		Delete(&ThreadWorkItemLinkModel{}).Error
}

