package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// ThreadTaskGroup CRUD
// ---------------------------------------------------------------------------

func (s *Store) CreateThreadTaskGroup(ctx context.Context, group *core.ThreadTaskGroup) (int64, error) {
	if s == nil || s.orm == nil {
		return 0, fmt.Errorf("store is not initialized")
	}
	if group == nil {
		return 0, fmt.Errorf("thread task group is nil")
	}
	if group.ThreadID <= 0 {
		return 0, fmt.Errorf("thread_id is required")
	}
	if group.Status == "" {
		group.Status = core.TaskGroupPending
	}
	if !group.Status.Valid() {
		return 0, fmt.Errorf("invalid task group status %q", group.Status)
	}

	now := time.Now().UTC()
	model := threadTaskGroupModelFromCore(group)
	model.CreatedAt = now

	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, err
	}
	group.ID = model.ID
	group.CreatedAt = now
	return model.ID, nil
}

func (s *Store) GetThreadTaskGroup(ctx context.Context, id int64) (*core.ThreadTaskGroup, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}
	var model ThreadTaskGroupModel
	if err := s.orm.WithContext(ctx).First(&model, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, err
	}
	return model.toCore(), nil
}

func (s *Store) ListThreadTaskGroups(ctx context.Context, filter core.ThreadTaskGroupFilter) ([]*core.ThreadTaskGroup, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}
	q := s.orm.WithContext(ctx).Model(&ThreadTaskGroupModel{})
	if filter.ThreadID != nil {
		q = q.Where("thread_id = ?", *filter.ThreadID)
	}
	if filter.Status != nil {
		q = q.Where("status = ?", string(*filter.Status))
	}
	q = q.Order("id ASC")
	if filter.Limit > 0 {
		q = q.Limit(filter.Limit)
	}
	if filter.Offset > 0 {
		q = q.Offset(filter.Offset)
	}

	var models []ThreadTaskGroupModel
	if err := q.Find(&models).Error; err != nil {
		return nil, err
	}
	groups := make([]*core.ThreadTaskGroup, len(models))
	for i := range models {
		groups[i] = models[i].toCore()
	}
	return groups, nil
}

func (s *Store) UpdateThreadTaskGroup(ctx context.Context, group *core.ThreadTaskGroup) error {
	if s == nil || s.orm == nil {
		return fmt.Errorf("store is not initialized")
	}
	if group == nil {
		return fmt.Errorf("thread task group is nil")
	}
	model := threadTaskGroupModelFromCore(group)
	result := s.orm.WithContext(ctx).Save(model)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteThreadTaskGroup(ctx context.Context, id int64) error {
	if s == nil || s.orm == nil {
		return fmt.Errorf("store is not initialized")
	}
	// Delete tasks first, then the group.
	if err := s.orm.WithContext(ctx).Where("group_id = ?", id).Delete(&ThreadTaskModel{}).Error; err != nil {
		return err
	}
	result := s.orm.WithContext(ctx).Delete(&ThreadTaskGroupModel{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------------------
// ThreadTask CRUD
// ---------------------------------------------------------------------------

func (s *Store) CreateThreadTask(ctx context.Context, task *core.ThreadTask) (int64, error) {
	if s == nil || s.orm == nil {
		return 0, fmt.Errorf("store is not initialized")
	}
	if task == nil {
		return 0, fmt.Errorf("thread task is nil")
	}
	if task.GroupID <= 0 {
		return 0, fmt.Errorf("group_id is required")
	}
	if task.ThreadID <= 0 {
		return 0, fmt.Errorf("thread_id is required")
	}
	if task.Assignee == "" {
		return 0, fmt.Errorf("assignee is required")
	}
	if task.Type == "" {
		task.Type = core.TaskTypeWork
	}
	if !task.Type.Valid() {
		return 0, fmt.Errorf("invalid task type %q", task.Type)
	}
	if task.Status == "" {
		task.Status = core.ThreadTaskPending
	}
	if !task.Status.Valid() {
		return 0, fmt.Errorf("invalid thread task status %q", task.Status)
	}

	now := time.Now().UTC()
	model := threadTaskModelFromCore(task)
	model.CreatedAt = now

	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, err
	}
	task.ID = model.ID
	task.CreatedAt = now
	return model.ID, nil
}

func (s *Store) GetThreadTask(ctx context.Context, id int64) (*core.ThreadTask, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}
	var model ThreadTaskModel
	if err := s.orm.WithContext(ctx).First(&model, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, err
	}
	return model.toCore(), nil
}

func (s *Store) ListThreadTasksByGroup(ctx context.Context, groupID int64) ([]*core.ThreadTask, error) {
	if s == nil || s.orm == nil {
		return nil, fmt.Errorf("store is not initialized")
	}
	var models []ThreadTaskModel
	if err := s.orm.WithContext(ctx).Where("group_id = ?", groupID).Order("id ASC").Find(&models).Error; err != nil {
		return nil, err
	}
	tasks := make([]*core.ThreadTask, len(models))
	for i := range models {
		tasks[i] = models[i].toCore()
	}
	return tasks, nil
}

func (s *Store) UpdateThreadTask(ctx context.Context, task *core.ThreadTask) error {
	if s == nil || s.orm == nil {
		return fmt.Errorf("store is not initialized")
	}
	if task == nil {
		return fmt.Errorf("thread task is nil")
	}
	model := threadTaskModelFromCore(task)
	result := s.orm.WithContext(ctx).Save(model)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (s *Store) DeleteThreadTasksByGroup(ctx context.Context, groupID int64) error {
	if s == nil || s.orm == nil {
		return fmt.Errorf("store is not initialized")
	}
	return s.orm.WithContext(ctx).Where("group_id = ?", groupID).Delete(&ThreadTaskModel{}).Error
}
