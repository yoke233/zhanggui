package core

import (
	"context"
	"time"
)

// WorkItemAttachment represents a file attached to a WorkItem.
type WorkItemAttachment struct {
	ID         int64     `json:"id"`
	WorkItemID int64     `json:"work_item_id"`
	FileName   string    `json:"file_name"`
	FilePath   string    `json:"-"` // server-side path, not exposed to client
	MimeType   string    `json:"mime_type"`
	Size       int64     `json:"size"`
	CreatedAt  time.Time `json:"created_at"`
}

// WorkItemAttachmentStore persists work item attachments.
type WorkItemAttachmentStore interface {
	CreateWorkItemAttachment(ctx context.Context, att *WorkItemAttachment) (int64, error)
	GetWorkItemAttachment(ctx context.Context, id int64) (*WorkItemAttachment, error)
	ListWorkItemAttachments(ctx context.Context, workItemID int64) ([]*WorkItemAttachment, error)
	DeleteWorkItemAttachment(ctx context.Context, id int64) error
}
