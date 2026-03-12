package core

import (
	"context"
	"time"
)

// ThreadStatus represents the lifecycle state of a Thread.
type ThreadStatus string

const (
	ThreadActive   ThreadStatus = "active"
	ThreadClosed   ThreadStatus = "closed"
	ThreadArchived ThreadStatus = "archived"
)

// Thread is an independent multi-participant discussion container.
// Unlike ChatSession (1:1 direct chat), a Thread supports multiple
// AI agents and multiple human participants in shared discussion.
type Thread struct {
	ID         int64          `json:"id"`
	Title      string         `json:"title"`
	Status     ThreadStatus   `json:"status"`
	OwnerID    string         `json:"owner_id,omitempty"`
	Summary    string         `json:"summary,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

// ThreadFilter constrains Thread queries.
type ThreadFilter struct {
	Status *ThreadStatus
	Limit  int
	Offset int
}

// ThreadStore persists Thread aggregates.
type ThreadStore interface {
	CreateThread(ctx context.Context, thread *Thread) (int64, error)
	GetThread(ctx context.Context, id int64) (*Thread, error)
	ListThreads(ctx context.Context, filter ThreadFilter) ([]*Thread, error)
	UpdateThread(ctx context.Context, thread *Thread) error
	DeleteThread(ctx context.Context, id int64) error
}
