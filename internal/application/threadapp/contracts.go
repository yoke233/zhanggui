package threadapp

import (
	"context"
	"time"

	"github.com/yoke233/zhanggui/internal/core"
)

type ThreadReader interface {
	GetThread(ctx context.Context, id int64) (*core.Thread, error)
}

type WorkItemReader interface {
	GetWorkItem(ctx context.Context, id int64) (*core.WorkItem, error)
}

type ProjectReader interface {
	GetProject(ctx context.Context, id int64) (*core.Project, error)
}

type ThreadWriter interface {
	CreateThread(ctx context.Context, thread *core.Thread) (int64, error)
	UpdateThread(ctx context.Context, thread *core.Thread) error
	DeleteThread(ctx context.Context, id int64) error
}

type ThreadMessageWriter interface {
	DeleteResourcesByThread(ctx context.Context, threadID int64) error
	DeleteThreadMessagesByThread(ctx context.Context, threadID int64) error
}

type ThreadAttachmentWriter interface {
	DeleteThreadAttachmentsByThread(ctx context.Context, threadID int64) error
}

type ThreadMemberWriter interface {
	AddThreadMember(ctx context.Context, member *core.ThreadMember) (int64, error)
	DeleteThreadMembersByThread(ctx context.Context, threadID int64) error
}

type ThreadMemberReader interface {
	ListThreadMembers(ctx context.Context, threadID int64) ([]*core.ThreadMember, error)
}

type ThreadLinkWriter interface {
	CreateThreadWorkItemLink(ctx context.Context, link *core.ThreadWorkItemLink) (int64, error)
	DeleteThreadWorkItemLink(ctx context.Context, threadID, workItemID int64) error
	DeleteThreadWorkItemLinksByThread(ctx context.Context, threadID int64) error
}

type ThreadLinkReader interface {
	ListThreadsByWorkItem(ctx context.Context, workItemID int64) ([]*core.ThreadWorkItemLink, error)
}

type ThreadContextRefStore interface {
	CreateThreadContextRef(ctx context.Context, ref *core.ThreadContextRef) (int64, error)
	GetThreadContextRef(ctx context.Context, id int64) (*core.ThreadContextRef, error)
	ListThreadContextRefs(ctx context.Context, threadID int64) ([]*core.ThreadContextRef, error)
	UpdateThreadContextRef(ctx context.Context, ref *core.ThreadContextRef) error
	DeleteThreadContextRef(ctx context.Context, id int64) error
	DeleteThreadContextRefsByThread(ctx context.Context, threadID int64) error

	ListThreadAttachments(ctx context.Context, threadID int64) ([]*core.ThreadAttachment, error)
}

type WorkItemWriter interface {
	CreateWorkItem(ctx context.Context, workItem *core.WorkItem) (int64, error)
	DeleteWorkItem(ctx context.Context, id int64) error
}

// Store is the application-facing persistence port for thread workflows.
type Store interface {
	ThreadReader
	WorkItemReader
	ProjectReader
	ThreadWriter
	ThreadMessageWriter
	ThreadAttachmentWriter
	ThreadMemberReader
	ThreadMemberWriter
	ThreadLinkReader
	ThreadLinkWriter
	ThreadContextRefStore
	WorkItemWriter
	core.ResourceSpaceStore
}

type TxStore interface {
	Store
}

type Tx interface {
	InTx(ctx context.Context, fn func(ctx context.Context, store TxStore) error) error
}

// Runtime is the optional runtime port for cleaning up live thread sessions.
type Runtime interface {
	CleanupThread(ctx context.Context, threadID int64) error
}

type WorkspaceManager interface {
	EnsureThreadWorkspace(ctx context.Context, threadID int64) error
	SyncThreadWorkspaceContext(ctx context.Context, threadID int64) error
}

type CreateThreadInput struct {
	Title              string
	OwnerID            string
	Metadata           map[string]any
	ParticipantUserIDs []string
}

type CreateThreadResult struct {
	Thread       *core.Thread
	Participants []*core.ThreadMember
}

type CreateThreadContextRefInput struct {
	ThreadID  int64
	ProjectID int64
	Access    string
	Note      string
	GrantedBy string
	ExpiresAt *time.Time
}

type UpdateThreadContextRefInput struct {
	ThreadID  int64
	RefID     int64
	Access    string
	Note      *string
	GrantedBy string
	ExpiresAt *time.Time
}

type LinkThreadWorkItemInput struct {
	ThreadID     int64
	WorkItemID   int64
	RelationType string
	IsPrimary    bool
}

type CreateWorkItemFromThreadInput struct {
	ThreadID      int64
	WorkItemTitle string
	WorkItemBody  string
	ProjectID     *int64
}

type CreateWorkItemFromThreadResult struct {
	Thread   *core.Thread
	WorkItem *core.WorkItem
	Link     *core.ThreadWorkItemLink
}
