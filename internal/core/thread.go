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

// ThreadAgentPromptResult captures the raw reply returned by a thread agent
// prompt before the caller decides how to persist or fan it out.
type ThreadAgentPromptResult struct {
	Content      string `json:"content"`
	InputTokens  int64  `json:"input_tokens,omitempty"`
	OutputTokens int64  `json:"output_tokens,omitempty"`
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

// ThreadFocus tracks the project currently emphasized in a thread.
type ThreadFocus struct {
	ProjectID int64 `json:"project_id"`
}

// ContextAccess defines a Thread's access level to mounted project resources.
type ContextAccess string

const (
	ContextAccessRead  ContextAccess = "read"
	ContextAccessCheck ContextAccess = "check"
	ContextAccessWrite ContextAccess = "write"
)

func (a ContextAccess) Valid() bool {
	switch a {
	case ContextAccessRead, ContextAccessCheck, ContextAccessWrite:
		return true
	default:
		return false
	}
}

func ParseContextAccess(raw string) (ContextAccess, error) {
	access := ContextAccess(strings.TrimSpace(raw))
	if !access.Valid() {
		return "", fmt.Errorf("invalid context access %q", raw)
	}
	return access, nil
}

func (a ContextAccess) AllowsCheck() bool {
	return a == ContextAccessCheck || a == ContextAccessWrite
}

func (a ContextAccess) AllowsWrite() bool {
	return a == ContextAccessWrite
}

// ThreadContextRef is a lightweight project context reference attached to a Thread.
type ThreadContextRef struct {
	ID        int64         `json:"id"`
	ThreadID  int64         `json:"thread_id"`
	ProjectID int64         `json:"project_id"`
	Access    ContextAccess `json:"access"`
	Note      string        `json:"note,omitempty"`
	GrantedBy string        `json:"granted_by,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
	ExpiresAt *time.Time    `json:"expires_at,omitempty"`
}

// ThreadWorkspaceMount describes a virtual project mount exposed to a Thread agent.
type ThreadWorkspaceMount struct {
	Path          string        `json:"path"`
	ProjectID     int64         `json:"project_id"`
	Access        ContextAccess `json:"access"`
	CheckCommands []string      `json:"check_commands,omitempty"`
}

// ThreadAttachment is a file or directory uploaded to a thread as discussion material.
type ThreadAttachment struct {
	ID          int64     `json:"id"`
	ThreadID    int64     `json:"thread_id"`
	MessageID   *int64    `json:"message_id,omitempty"`
	FileName    string    `json:"file_name"`
	FilePath    string    `json:"file_path"`
	FileSize    int64     `json:"file_size"`
	ContentType string    `json:"content_type"`
	IsDirectory bool      `json:"is_directory"`
	UploadedBy  string    `json:"uploaded_by,omitempty"`
	Note        string    `json:"note,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// ThreadWorkspaceAttachmentRef is the .context.json representation of an attachment.
type ThreadWorkspaceAttachmentRef struct {
	FileName    string `json:"file_name"`
	FilePath    string `json:"file_path"`
	IsDirectory bool   `json:"is_directory,omitempty"`
	Note        string `json:"note,omitempty"`
}

// ThreadWorkspaceContext is the platform-maintained .context.json payload.
type ThreadWorkspaceContext struct {
	ThreadID    int64                           `json:"thread_id"`
	Workspace   string                          `json:"workspace"`
	Mounts      map[string]ThreadWorkspaceMount `json:"mounts,omitempty"`
	Attachments []ThreadWorkspaceAttachmentRef  `json:"attachments,omitempty"`
	Members     []string                        `json:"members,omitempty"`
	UpdatedAt   time.Time                       `json:"updated_at"`
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

func ReadThreadFocus(thread *Thread) (*ThreadFocus, bool) {
	if thread == nil || len(thread.Metadata) == 0 {
		return nil, false
	}
	raw, ok := thread.Metadata["focus"]
	if !ok || raw == nil {
		return nil, false
	}
	switch value := raw.(type) {
	case ThreadFocus:
		if value.ProjectID > 0 {
			copyValue := value
			return &copyValue, true
		}
	case *ThreadFocus:
		if value != nil && value.ProjectID > 0 {
			copyValue := *value
			return &copyValue, true
		}
	case map[string]any:
		if projectID, ok := parseThreadFocusProjectID(value["project_id"]); ok {
			return &ThreadFocus{ProjectID: projectID}, true
		}
	case map[string]int64:
		if projectID, ok := value["project_id"]; ok && projectID > 0 {
			return &ThreadFocus{ProjectID: projectID}, true
		}
	}
	return nil, false
}

func ReadThreadFocusProjectID(thread *Thread) (int64, bool) {
	focus, ok := ReadThreadFocus(thread)
	if !ok || focus == nil || focus.ProjectID <= 0 {
		return 0, false
	}
	return focus.ProjectID, true
}

func SetThreadFocusProjectID(thread *Thread, projectID int64) {
	if thread == nil || projectID <= 0 {
		return
	}
	if thread.Metadata == nil {
		thread.Metadata = map[string]any{}
	}
	thread.Metadata["focus"] = map[string]any{"project_id": projectID}
}

func ClearThreadFocus(thread *Thread) {
	if thread == nil || len(thread.Metadata) == 0 {
		return
	}
	delete(thread.Metadata, "focus")
	if len(thread.Metadata) == 0 {
		thread.Metadata = nil
	}
}

func parseThreadFocusProjectID(raw any) (int64, bool) {
	switch value := raw.(type) {
	case int:
		if value > 0 {
			return int64(value), true
		}
	case int64:
		if value > 0 {
			return value, true
		}
	case float64:
		if value > 0 {
			return int64(value), true
		}
	}
	return 0, false
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
	DeleteThreadMessagesByThread(ctx context.Context, threadID int64) error

	// ThreadMember CRUD (unified human + agent members).
	AddThreadMember(ctx context.Context, m *ThreadMember) (int64, error)
	ListThreadMembers(ctx context.Context, threadID int64) ([]*ThreadMember, error)
	GetThreadMember(ctx context.Context, id int64) (*ThreadMember, error)
	UpdateThreadMember(ctx context.Context, m *ThreadMember) error
	RemoveThreadMember(ctx context.Context, id int64) error
	RemoveThreadMemberByUser(ctx context.Context, threadID int64, userID string) error
	DeleteThreadMembersByThread(ctx context.Context, threadID int64) error

	CreateThreadWorkItemLink(ctx context.Context, link *ThreadWorkItemLink) (int64, error)
	ListWorkItemsByThread(ctx context.Context, threadID int64) ([]*ThreadWorkItemLink, error)
	ListThreadsByWorkItem(ctx context.Context, workItemID int64) ([]*ThreadWorkItemLink, error)
	DeleteThreadWorkItemLink(ctx context.Context, threadID, workItemID int64) error
	DeleteThreadWorkItemLinksByThread(ctx context.Context, threadID int64) error
	DeleteThreadWorkItemLinksByWorkItem(ctx context.Context, workItemID int64) error

	CreateThreadContextRef(ctx context.Context, ref *ThreadContextRef) (int64, error)
	GetThreadContextRef(ctx context.Context, id int64) (*ThreadContextRef, error)
	ListThreadContextRefs(ctx context.Context, threadID int64) ([]*ThreadContextRef, error)
	UpdateThreadContextRef(ctx context.Context, ref *ThreadContextRef) error
	DeleteThreadContextRef(ctx context.Context, id int64) error
	DeleteThreadContextRefsByThread(ctx context.Context, threadID int64) error

	CreateThreadAttachment(ctx context.Context, att *ThreadAttachment) (int64, error)
	GetThreadAttachment(ctx context.Context, id int64) (*ThreadAttachment, error)
	ListThreadAttachments(ctx context.Context, threadID int64) ([]*ThreadAttachment, error)
	DeleteThreadAttachment(ctx context.Context, id int64) error
	DeleteThreadAttachmentsByThread(ctx context.Context, threadID int64) error
}
