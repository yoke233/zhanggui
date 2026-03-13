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
		ThreadID:  msg.ThreadID,
		SenderID:  strings.TrimSpace(msg.SenderID),
		Role:      msg.Role,
		Content:   msg.Content,
		Metadata:  JSONField[map[string]any]{Data: msg.Metadata},
		CreatedAt: now,
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
// ThreadParticipant CRUD
// ---------------------------------------------------------------------------

func (s *Store) AddThreadParticipant(ctx context.Context, p *core.ThreadParticipant) (int64, error) {
	if s == nil || s.orm == nil {
		return 0, fmt.Errorf("store is not initialized")
	}
	if p == nil {
		return 0, fmt.Errorf("participant is nil")
	}

	now := time.Now().UTC()
	model := &ThreadParticipantModel{
		ThreadID: p.ThreadID,
		UserID:   strings.TrimSpace(p.UserID),
		Role:     p.Role,
		JoinedAt: now,
	}
	if model.Role == "" {
		model.Role = "member"
	}

	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, err
	}
	p.ID = model.ID
	p.JoinedAt = now
	return model.ID, nil
}

func (s *Store) ListThreadParticipants(ctx context.Context, threadID int64) ([]*core.ThreadParticipant, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}

	var models []ThreadParticipantModel
	if err := s.orm.WithContext(ctx).
		Where("thread_id = ?", threadID).
		Order("id ASC").
		Find(&models).Error; err != nil {
		return nil, err
	}

	out := make([]*core.ThreadParticipant, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
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

// ---------------------------------------------------------------------------
// ThreadAgentSession CRUD
// ---------------------------------------------------------------------------

func (s *Store) CreateThreadAgentSession(ctx context.Context, sess *core.ThreadAgentSession) (int64, error) {
	if s == nil || s.orm == nil {
		return 0, fmt.Errorf("store is not initialized")
	}
	if sess == nil {
		return 0, fmt.Errorf("session is nil")
	}

	now := time.Now().UTC()
	model := &ThreadAgentSessionModel{
		ThreadID:       sess.ThreadID,
		AgentProfileID: strings.TrimSpace(sess.AgentProfileID),
		ACPSessionID:   sess.ACPSessionID,
		Status:         sess.Status,
		JoinedAt:       now,
		LastActiveAt:   now,
	}
	if model.Status == "" {
		model.Status = "joining"
	}

	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, err
	}
	sess.ID = model.ID
	sess.JoinedAt = now
	sess.LastActiveAt = now
	return model.ID, nil
}

func (s *Store) GetThreadAgentSession(ctx context.Context, id int64) (*core.ThreadAgentSession, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}

	var model ThreadAgentSessionModel
	err := s.orm.WithContext(ctx).First(&model, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, err
	}
	return model.toCore(), nil
}

func (s *Store) ListThreadAgentSessions(ctx context.Context, threadID int64) ([]*core.ThreadAgentSession, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}

	var models []ThreadAgentSessionModel
	if err := s.orm.WithContext(ctx).
		Where("thread_id = ?", threadID).
		Order("id ASC").
		Find(&models).Error; err != nil {
		return nil, err
	}

	out := make([]*core.ThreadAgentSession, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) UpdateThreadAgentSession(ctx context.Context, sess *core.ThreadAgentSession) error {
	if s == nil || s.orm == nil {
		return fmt.Errorf("store is not initialized")
	}
	if sess == nil {
		return fmt.Errorf("session is nil")
	}

	now := time.Now().UTC()
	result := s.orm.WithContext(ctx).Model(&ThreadAgentSessionModel{}).
		Where("id = ?", sess.ID).
		Updates(map[string]any{
			"status":              sess.Status,
			"acp_session_id":      sess.ACPSessionID,
			"turn_count":          sess.TurnCount,
			"total_input_tokens":  sess.TotalInputTokens,
			"total_output_tokens": sess.TotalOutputTokens,
			"progress_summary":    sess.ProgressSummary,
			"metadata":            JSONField[map[string]any]{Data: sess.Metadata},
			"last_active_at":      now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	sess.LastActiveAt = now
	return nil
}

func (s *Store) DeleteThreadAgentSession(ctx context.Context, id int64) error {
	if s == nil || s.orm == nil {
		return fmt.Errorf("store is not initialized")
	}

	result := s.orm.WithContext(ctx).Delete(&ThreadAgentSessionModel{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (s *Store) RemoveThreadParticipant(ctx context.Context, threadID int64, userID string) error {
	if s == nil || s.orm == nil {
		return fmt.Errorf("store is not initialized")
	}

	result := s.orm.WithContext(ctx).
		Where("thread_id = ? AND user_id = ?", threadID, strings.TrimSpace(userID)).
		Delete(&ThreadParticipantModel{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}
