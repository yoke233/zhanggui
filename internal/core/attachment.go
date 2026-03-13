package core

import (
	"context"
	"time"
)

// IssueAttachment represents a file attached to an Issue.
type IssueAttachment struct {
	ID        int64     `json:"id"`
	IssueID   int64     `json:"issue_id"`
	FileName  string    `json:"file_name"`
	FilePath  string    `json:"-"` // server-side path, not exposed to client
	MimeType  string    `json:"mime_type"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

// IssueAttachmentStore persists issue attachments.
type IssueAttachmentStore interface {
	CreateIssueAttachment(ctx context.Context, att *IssueAttachment) (int64, error)
	GetIssueAttachment(ctx context.Context, id int64) (*IssueAttachment, error)
	ListIssueAttachments(ctx context.Context, issueID int64) ([]*IssueAttachment, error)
	DeleteIssueAttachment(ctx context.Context, id int64) error
}
