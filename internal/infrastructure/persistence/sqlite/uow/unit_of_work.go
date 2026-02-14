package uow

import (
	"context"

	"gorm.io/gorm"

	"zhanggui/internal/ports"
)

// UnitOfWork implements ports.UnitOfWork with gorm.
type UnitOfWork struct {
	db *gorm.DB
}

func NewUnitOfWork(db *gorm.DB) *UnitOfWork {
	return &UnitOfWork{db: db}
}

func (u *UnitOfWork) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return u.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(ports.WithTxContext(ctx, tx))
	})
}
