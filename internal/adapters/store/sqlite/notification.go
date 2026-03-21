package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/yoke233/zhanggui/internal/core"
	"gorm.io/gorm"
)

// NotificationModel is the GORM model for the notifications table.
type NotificationModel struct {
	ID         int64                                 `gorm:"column:id;primaryKey;autoIncrement"`
	Level      string                                `gorm:"column:level;not null"`
	Title      string                                `gorm:"column:title;not null"`
	Body       string                                `gorm:"column:body;not null;default:''"`
	Category   string                                `gorm:"column:category;not null;default:''"`
	ActionURL  string                                `gorm:"column:action_url;not null;default:''"`
	ProjectID  *int64                                `gorm:"column:project_id"`
	WorkItemID *int64                                `gorm:"column:work_item_id"`
	RunID      *int64                                `gorm:"column:run_id"`
	Channels   JSONField[[]core.NotificationChannel] `gorm:"column:channels;type:text"`
	Read       bool                                  `gorm:"column:read;not null;default:false"`
	ReadAt     *time.Time                            `gorm:"column:read_at"`
	CreatedAt  time.Time                             `gorm:"column:created_at"`
}

func (NotificationModel) TableName() string { return "notifications" }

func notificationModelFromCore(n *core.Notification) *NotificationModel {
	if n == nil {
		return nil
	}
	return &NotificationModel{
		ID:         n.ID,
		Level:      string(n.Level),
		Title:      n.Title,
		Body:       n.Body,
		Category:   n.Category,
		ActionURL:  n.ActionURL,
		ProjectID:  n.ProjectID,
		WorkItemID: n.WorkItemID,
		RunID:      n.RunID,
		Channels:   JSONField[[]core.NotificationChannel]{Data: n.Channels},
		Read:       n.Read,
		ReadAt:     n.ReadAt,
		CreatedAt:  n.CreatedAt,
	}
}

func (m *NotificationModel) toCore() *core.Notification {
	if m == nil {
		return nil
	}
	return &core.Notification{
		ID:         m.ID,
		Level:      core.NotificationLevel(m.Level),
		Title:      m.Title,
		Body:       m.Body,
		Category:   m.Category,
		ActionURL:  m.ActionURL,
		ProjectID:  m.ProjectID,
		WorkItemID: m.WorkItemID,
		RunID:      m.RunID,
		Channels:   m.Channels.Data,
		Read:       m.Read,
		ReadAt:     m.ReadAt,
		CreatedAt:  m.CreatedAt,
	}
}

func (s *Store) CreateNotification(ctx context.Context, n *core.Notification) (int64, error) {
	now := time.Now().UTC()
	model := notificationModelFromCore(n)
	model.CreatedAt = now
	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, fmt.Errorf("insert notification: %w", err)
	}
	n.ID = model.ID
	n.CreatedAt = now
	return model.ID, nil
}

func (s *Store) GetNotification(ctx context.Context, id int64) (*core.Notification, error) {
	var model NotificationModel
	err := s.orm.WithContext(ctx).First(&model, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("get notification %d: %w", id, err)
	}
	return model.toCore(), nil
}

func (s *Store) ListNotifications(ctx context.Context, filter core.NotificationFilter) ([]*core.Notification, error) {
	q := s.orm.WithContext(ctx).Model(&NotificationModel{})
	if filter.ProjectID != nil {
		q = q.Where("project_id = ?", *filter.ProjectID)
	}
	if filter.WorkItemID != nil {
		q = q.Where("work_item_id = ?", *filter.WorkItemID)
	}
	if filter.Category != "" {
		q = q.Where("category = ?", filter.Category)
	}
	if filter.Level != nil {
		q = q.Where("level = ?", string(*filter.Level))
	}
	if filter.Read != nil {
		q = q.Where("read = ?", *filter.Read)
	}
	q = q.Order("id DESC")
	if filter.Offset > 0 {
		q = q.Offset(filter.Offset)
	}
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	q = q.Limit(limit)

	var models []NotificationModel
	if err := q.Find(&models).Error; err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	result := make([]*core.Notification, len(models))
	for i := range models {
		result[i] = models[i].toCore()
	}
	return result, nil
}

func (s *Store) MarkNotificationRead(ctx context.Context, id int64) error {
	now := time.Now().UTC()
	res := s.orm.WithContext(ctx).Model(&NotificationModel{}).Where("id = ?", id).
		Updates(map[string]any{"read": true, "read_at": now})
	if res.Error != nil {
		return fmt.Errorf("mark notification read %d: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (s *Store) MarkAllNotificationsRead(ctx context.Context) error {
	now := time.Now().UTC()
	err := s.orm.WithContext(ctx).Model(&NotificationModel{}).Where("read = ?", false).
		Updates(map[string]any{"read": true, "read_at": now}).Error
	if err != nil {
		return fmt.Errorf("mark all notifications read: %w", err)
	}
	return nil
}

func (s *Store) DeleteNotification(ctx context.Context, id int64) error {
	res := s.orm.WithContext(ctx).Delete(&NotificationModel{}, id)
	if res.Error != nil {
		return fmt.Errorf("delete notification %d: %w", id, res.Error)
	}
	if res.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (s *Store) CountUnreadNotifications(ctx context.Context) (int, error) {
	var count int64
	err := s.orm.WithContext(ctx).Model(&NotificationModel{}).Where("read = ?", false).Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count unread notifications: %w", err)
	}
	return int(count), nil
}
