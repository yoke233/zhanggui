package cache

import (
	"context"
	"testing"

	gormsqlite "github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"zhanggui/internal/infrastructure/persistence/sqlite/model"
)

func setupSQLiteCache(t *testing.T) *SQLiteCache {
	t.Helper()

	db, err := gorm.Open(gormsqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	if err := db.AutoMigrate(&model.OutboxKV{}); err != nil {
		t.Fatalf("auto migrate outbox_kv: %v", err)
	}

	return NewSQLiteCache(db)
}

func TestSQLiteCacheSetGetDelete(t *testing.T) {
	cache := setupSQLiteCache(t)
	ctx := context.Background()

	if err := cache.Set(ctx, "active_run:local#1", "2026-02-14-backend-0001", 0); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	value, found, err := cache.Get(ctx, "active_run:local#1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !found {
		t.Fatalf("Get() expected found=true")
	}
	if value != "2026-02-14-backend-0001" {
		t.Fatalf("Get() value = %q", value)
	}

	if err := cache.Set(ctx, "active_run:local#1", "2026-02-14-backend-0002", 0); err != nil {
		t.Fatalf("Set(update) error = %v", err)
	}

	value, found, err = cache.Get(ctx, "active_run:local#1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !found || value != "2026-02-14-backend-0002" {
		t.Fatalf("Get() after update = %q, found=%v", value, found)
	}

	if err := cache.Delete(ctx, "active_run:local#1"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, found, err = cache.Get(ctx, "active_run:local#1")
	if err != nil {
		t.Fatalf("Get() after delete error = %v", err)
	}
	if found {
		t.Fatalf("Get() expected found=false after delete")
	}
}

func TestSQLiteCacheRejectsEmptyKey(t *testing.T) {
	cache := setupSQLiteCache(t)
	ctx := context.Background()

	if err := cache.Set(ctx, "", "v", 0); err == nil {
		t.Fatalf("Set() expected error for empty key")
	}
	if _, _, err := cache.Get(ctx, ""); err == nil {
		t.Fatalf("Get() expected error for empty key")
	}
	if err := cache.Delete(ctx, ""); err == nil {
		t.Fatalf("Delete() expected error for empty key")
	}
}
