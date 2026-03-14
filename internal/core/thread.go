package core

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ThreadStatus represents the lifecycle state of a Thread.
type ThreadStatus string

const (
	ThreadActive   ThreadStatus = "active"
	ThreadClosed   ThreadStatus = "closed"
	ThreadArchived ThreadStatus = "archived"
)

func (s ThreadStatus) Valid() bool {
	switch s {
	case ThreadActive, ThreadClosed, ThreadArchived:
		return true
	default:
		return false
	}
}

func ParseThreadStatus(raw string) (ThreadStatus, error) {
	status := ThreadStatus(strings.TrimSpace(raw))
	if !status.Valid() {
		return "", fmt.Errorf("invalid thread status %q", raw)
	}
	return status, nil
}

func CanTransitionThreadStatus(from, to ThreadStatus) bool {
	if !from.Valid() || !to.Valid() {
		return false
	}
	if from == to {
		return true
	}
	switch from {
	case ThreadActive:
		return to == ThreadClosed || to == ThreadArchived
	case ThreadClosed:
		return to == ThreadArchived
	case ThreadArchived:
		return false
	default:
		return false
	}
}

// Thread is an independent multi-participant discussion container.
// Unlike ChatSession (1:1 direct chat), a Thread supports multiple
// AI agents and multiple human participants in shared discussion.
type Thread struct {
	ID        int64          `json:"id"`
	Title     string         `json:"title"`
	Status    ThreadStatus   `json:"status"`
	OwnerID   string         `json:"owner_id,omitempty"`
	Summary   string         `json:"summary,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// ThreadFilter constrains Thread queries.
type ThreadFilter struct {
	Status *ThreadStatus
	Limit  int
	Offset int
}

// ThreadMessage is a single message within a Thread.
type ThreadMessage struct {
	ID               int64          `json:"id"`
	ThreadID         int64          `json:"thread_id"`
	SenderID         string         `json:"sender_id"`
	Role             string         `json:"role"` // "human" or "agent"
	Content          string         `json:"content"`
	ReplyToMessageID *int64         `json:"reply_to_msg_id,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
}

// ThreadMemberKind constants.
const (
	ThreadMemberKindHuman = "human"
	ThreadMemberKindAgent = "agent"
)

// ThreadMember represents a unified member (human or agent) in a Thread.
type ThreadMember struct {
	ID             int64             `json:"id"`
	ThreadID       int64             `json:"thread_id"`
	Kind           string            `json:"kind"` // "human" or "agent"
	UserID         string            `json:"user_id,omitempty"`
	AgentProfileID string            `json:"agent_profile_id,omitempty"`
	Role           string            `json:"role"`
	Status         ThreadAgentStatus `json:"status,omitempty"`
	AgentData      map[string]any    `json:"agent_data,omitempty"` // acp_session_id, turn_count, tokens, progress, metadata
	JoinedAt       time.Time         `json:"joined_at"`
	LastActiveAt   time.Time         `json:"last_active_at"`
}

// ThreadWorkItemLink represents an explicit link between a Thread and a WorkItem.
type ThreadWorkItemLink struct {
	ID           int64     `json:"id"`
	ThreadID     int64     `json:"thread_id"`
	WorkItemID   int64     `json:"work_item_id"`
	RelationType string    `json:"relation_type"` // "related", "drives", "blocks"
	IsPrimary    bool      `json:"is_primary"`
	CreatedAt    time.Time `json:"created_at"`
}

// ThreadAgentStatus represents the lifecycle status of an agent thread member.
type ThreadAgentStatus string

// ThreadAgentStatus constants.
const (
	ThreadAgentJoining ThreadAgentStatus = "joining"
	ThreadAgentBooting ThreadAgentStatus = "booting"
	ThreadAgentActive  ThreadAgentStatus = "active"
	ThreadAgentPaused  ThreadAgentStatus = "paused"
	ThreadAgentLeft    ThreadAgentStatus = "left"
	ThreadAgentFailed  ThreadAgentStatus = "failed"
)

func (s ThreadAgentStatus) Valid() bool {
	switch s {
	case ThreadAgentJoining, ThreadAgentBooting, ThreadAgentActive, ThreadAgentPaused, ThreadAgentLeft, ThreadAgentFailed:
		return true
	default:
		return false
	}
}

func ParseThreadAgentStatus(raw string) (ThreadAgentStatus, error) {
	status := ThreadAgentStatus(strings.TrimSpace(raw))
	if !status.Valid() {
		return "", fmt.Errorf("invalid thread agent status %q", raw)
	}
	return status, nil
}

func CanTransitionThreadAgentStatus(from, to ThreadAgentStatus) bool {
	if !from.Valid() || !to.Valid() {
		return false
	}
	if from == to {
		return true
	}
	switch from {
	case ThreadAgentJoining:
		return to == ThreadAgentBooting || to == ThreadAgentFailed || to == ThreadAgentLeft
	case ThreadAgentBooting:
		return to == ThreadAgentActive || to == ThreadAgentFailed || to == ThreadAgentLeft
	case ThreadAgentActive:
		return to == ThreadAgentPaused || to == ThreadAgentFailed || to == ThreadAgentLeft
	case ThreadAgentPaused:
		return to == ThreadAgentBooting || to == ThreadAgentFailed || to == ThreadAgentLeft
	case ThreadAgentLeft, ThreadAgentFailed:
		return false
	default:
		return false
	}
}

// ThreadStore persists Thread aggregates.
type ThreadStore interface {
	CreateThread(ctx context.Context, thread *Thread) (int64, error)
	GetThread(ctx context.Context, id int64) (*Thread, error)
	ListThreads(ctx context.Context, filter ThreadFilter) ([]*Thread, error)
	UpdateThread(ctx context.Context, thread *Thread) error
	DeleteThread(ctx context.Context, id int64) error

	CreateThreadMessage(ctx context.Context, msg *ThreadMessage) (int64, error)
	ListThreadMessages(ctx context.Context, threadID int64, limit, offset int) ([]*ThreadMessage, error)

	// ThreadMember CRUD (unified human + agent members).
	AddThreadMember(ctx context.Context, m *ThreadMember) (int64, error)
	ListThreadMembers(ctx context.Context, threadID int64) ([]*ThreadMember, error)
	GetThreadMember(ctx context.Context, id int64) (*ThreadMember, error)
	UpdateThreadMember(ctx context.Context, m *ThreadMember) error
	RemoveThreadMember(ctx context.Context, id int64) error
	RemoveThreadMemberByUser(ctx context.Context, threadID int64, userID string) error

	CreateThreadWorkItemLink(ctx context.Context, link *ThreadWorkItemLink) (int64, error)
	ListWorkItemsByThread(ctx context.Context, threadID int64) ([]*ThreadWorkItemLink, error)
	ListThreadsByWorkItem(ctx context.Context, workItemID int64) ([]*ThreadWorkItemLink, error)
	DeleteThreadWorkItemLink(ctx context.Context, threadID, workItemID int64) error
	DeleteThreadWorkItemLinksByThread(ctx context.Context, threadID int64) error
	DeleteThreadWorkItemLinksByWorkItem(ctx context.Context, workItemID int64) error

}
