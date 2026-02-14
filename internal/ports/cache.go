package ports

import (
	"context"
	"time"
)

// Cache defines a generic key-value capability for usecases.
// Adapters may be backed by SQLite/Redis or other stores.
type Cache interface {
	Get(ctx context.Context, key string) (value string, found bool, err error)
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}
