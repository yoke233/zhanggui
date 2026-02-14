package cache

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"zhanggui/internal/errs"
	"zhanggui/internal/infrastructure/persistence/sqlite/model"
	"zhanggui/internal/ports"
)

type SQLiteCache struct {
	db *gorm.DB
}

var _ ports.Cache = (*SQLiteCache)(nil)

func NewSQLiteCache(db *gorm.DB) *SQLiteCache {
	return &SQLiteCache{db: db}
}

func (c *SQLiteCache) Get(ctx context.Context, key string) (string, bool, error) {
	if ctx == nil {
		return "", false, errors.New("context is required")
	}
	if err := ctx.Err(); err != nil {
		return "", false, errs.Wrap(err, "check context")
	}

	trimmedKey := strings.TrimSpace(key)
	if trimmedKey == "" {
		return "", false, errors.New("key is required")
	}

	var row model.OutboxKV
	if err := c.db.WithContext(ctx).Where("key = ?", trimmedKey).Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", false, nil
		}
		return "", false, errs.Wrap(err, "query cache by key")
	}

	return row.Value, true, nil
}

func (c *SQLiteCache) Set(ctx context.Context, key string, value string, _ time.Duration) error {
	if ctx == nil {
		return errors.New("context is required")
	}
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "check context")
	}

	trimmedKey := strings.TrimSpace(key)
	if trimmedKey == "" {
		return errors.New("key is required")
	}

	row := model.OutboxKV{
		Key:       trimmedKey,
		Value:     value,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}

	if err := c.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "key"}},
		DoUpdates: clause.Assignments(map[string]any{
			"value":      row.Value,
			"updated_at": row.UpdatedAt,
		}),
	}).Create(&row).Error; err != nil {
		return errs.Wrap(err, "upsert cache key")
	}

	return nil
}

func (c *SQLiteCache) Delete(ctx context.Context, key string) error {
	if ctx == nil {
		return errors.New("context is required")
	}
	if err := ctx.Err(); err != nil {
		return errs.Wrap(err, "check context")
	}

	trimmedKey := strings.TrimSpace(key)
	if trimmedKey == "" {
		return errors.New("key is required")
	}

	if err := c.db.WithContext(ctx).Where("key = ?", trimmedKey).Delete(&model.OutboxKV{}).Error; err != nil {
		return errs.Wrap(err, "delete cache key")
	}
	return nil
}
