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

// ThreadMessage is a single message within a Thread.
type ThreadMessage struct {
	ID        int64          `json:"id"`
	ThreadID  int64          `json:"thread_id"`
	SenderID  string         `json:"sender_id"`
	Role      string         `json:"role"` // "human" or "agent"
	Content   string         `json:"content"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// ThreadParticipant represents a participant in a Thread.
type ThreadParticipant struct {
	ID       int64     `json:"id"`
	ThreadID int64     `json:"thread_id"`
	UserID   string    `json:"user_id"`
	Role     string    `json:"role"` // "owner", "member", "agent"
	JoinedAt time.Time `json:"joined_at"`
}

// ThreadWorkItemLink represents an explicit link between a Thread and a WorkItem (Issue).
type ThreadWorkItemLink struct {
	ID           int64     `json:"id"`
	ThreadID     int64     `json:"thread_id"`
	WorkItemID   int64     `json:"work_item_id"`
	RelationType string    `json:"relation_type"` // "related", "drives", "blocks"
	IsPrimary    bool      `json:"is_primary"`
	CreatedAt    time.Time `json:"created_at"`
}

// ThreadAgentSession represents an AI agent session within a Thread.
type ThreadAgentSession struct {
	ID               int64          `json:"id"`
	ThreadID         int64          `json:"thread_id"`
	AgentProfileID   string         `json:"agent_profile_id"`
	ACPSessionID     string         `json:"acp_session_id"`
	Status           string         `json:"status"` // "joining", "booting", "active", "paused", "left", "failed"
	TurnCount        int            `json:"turn_count"`
	TotalInputTokens int64          `json:"total_input_tokens"`
	TotalOutputTokens int64         `json:"total_output_tokens"`
	ProgressSummary  string         `json:"progress_summary,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	JoinedAt         time.Time      `json:"joined_at"`
	LastActiveAt     time.Time      `json:"last_active_at"`
}

// ThreadAgentSession status constants.
const (
	ThreadAgentJoining = "joining"
	ThreadAgentBooting = "booting"
	ThreadAgentActive  = "active"
	ThreadAgentPaused  = "paused"
	ThreadAgentLeft    = "left"
	ThreadAgentFailed  = "failed"
)

// ThreadStore persists Thread aggregates.
type ThreadStore interface {
	CreateThread(ctx context.Context, thread *Thread) (int64, error)
	GetThread(ctx context.Context, id int64) (*Thread, error)
	ListThreads(ctx context.Context, filter ThreadFilter) ([]*Thread, error)
	UpdateThread(ctx context.Context, thread *Thread) error
	DeleteThread(ctx context.Context, id int64) error

	CreateThreadMessage(ctx context.Context, msg *ThreadMessage) (int64, error)
	ListThreadMessages(ctx context.Context, threadID int64, limit, offset int) ([]*ThreadMessage, error)

	AddThreadParticipant(ctx context.Context, p *ThreadParticipant) (int64, error)
	ListThreadParticipants(ctx context.Context, threadID int64) ([]*ThreadParticipant, error)
	RemoveThreadParticipant(ctx context.Context, threadID int64, userID string) error

	CreateThreadWorkItemLink(ctx context.Context, link *ThreadWorkItemLink) (int64, error)
	ListWorkItemsByThread(ctx context.Context, threadID int64) ([]*ThreadWorkItemLink, error)
	ListThreadsByWorkItem(ctx context.Context, workItemID int64) ([]*ThreadWorkItemLink, error)
	DeleteThreadWorkItemLink(ctx context.Context, threadID, workItemID int64) error
	DeleteThreadWorkItemLinksByThread(ctx context.Context, threadID int64) error
	DeleteThreadWorkItemLinksByWorkItem(ctx context.Context, workItemID int64) error

	CreateThreadAgentSession(ctx context.Context, s *ThreadAgentSession) (int64, error)
	GetThreadAgentSession(ctx context.Context, id int64) (*ThreadAgentSession, error)
	ListThreadAgentSessions(ctx context.Context, threadID int64) ([]*ThreadAgentSession, error)
	UpdateThreadAgentSession(ctx context.Context, s *ThreadAgentSession) error
	DeleteThreadAgentSession(ctx context.Context, id int64) error
}
