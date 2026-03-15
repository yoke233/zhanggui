package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/yoke233/ai-workflow/internal/core"
	"gorm.io/gorm"
)

func (s *Store) CreateActionIODecl(ctx context.Context, decl *core.ActionIODecl) (int64, error) {
	if err := validateActionIODecl(decl); err != nil {
		return 0, err
	}
	now := time.Now().UTC()
	model := actionIODeclModelFromCore(decl)
	model.CreatedAt = now
	if err := s.orm.WithContext(ctx).Create(model).Error; err != nil {
		return 0, err
	}
	decl.ID = model.ID
	decl.CreatedAt = now
	return model.ID, nil
}

func (s *Store) GetActionIODecl(ctx context.Context, id int64) (*core.ActionIODecl, error) {
	var model ActionIODeclModel
	err := s.orm.WithContext(ctx).First(&model, id).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, core.ErrNotFound
		}
		return nil, err
	}
	return model.toCore(), nil
}

func (s *Store) ListActionIODecls(ctx context.Context, actionID int64) ([]*core.ActionIODecl, error) {
	var models []ActionIODeclModel
	if err := s.orm.WithContext(ctx).
		Where("action_id = ?", actionID).
		Order("id ASC").
		Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]*core.ActionIODecl, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) ListActionIODeclsByDirection(ctx context.Context, actionID int64, dir core.IODirection) ([]*core.ActionIODecl, error) {
	var models []ActionIODeclModel
	if err := s.orm.WithContext(ctx).
		Where("action_id = ? AND direction = ?", actionID, string(dir)).
		Order("id ASC").
		Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]*core.ActionIODecl, 0, len(models))
	for i := range models {
		out = append(out, models[i].toCore())
	}
	return out, nil
}

func (s *Store) DeleteActionIODecl(ctx context.Context, id int64) error {
	result := s.orm.WithContext(ctx).Delete(&ActionIODeclModel{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return core.ErrNotFound
	}
	return nil
}

func validateActionIODecl(decl *core.ActionIODecl) error {
	if decl == nil {
		return fmt.Errorf("action io decl is required")
	}
	if decl.ActionID == 0 {
		return fmt.Errorf("action io decl requires action_id")
	}
	if decl.Direction != core.IOInput && decl.Direction != core.IOOutput {
		return fmt.Errorf("action io decl requires valid direction")
	}
	if (decl.SpaceID == nil) == (decl.ResourceID == nil) {
		return fmt.Errorf("action io decl must reference exactly one of space_id or resource_id")
	}
	return nil
}
