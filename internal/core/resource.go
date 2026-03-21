package core

import (
	"context"
	"time"
)

// ResourceBinding is the legacy pre-unification representation of an external resource
// associated with a Project. It remains only for migration/compatibility paths.
// New runtime code should use ResourceSpace / Resource / ActionIODecl instead.
//
// Historically it served two roles:
//
//  1. Workspace source — WorkspaceProvider uses it to prepare the agent's
//     working directory (e.g. git worktree, local dir).
//  2. I/O storage — ActionResource references it for per-action file
//     fetch/deposit (e.g. shared drive, S3 bucket, CDN).
//
// When Kind is "attachment", the binding represents a work-item file
// attachment (WorkItemID is set, ProjectID may be zero).
//
// The Kind field determines which provider handles it; the URI field
// can be a local path ("/home/user/repo"), a remote URL
// ("https://github.com/org/repo.git"), or a storage URI ("s3://bucket/prefix").
type ResourceBinding struct {
	ID         int64          `json:"id"`
	ProjectID  int64          `json:"project_id"`
	WorkItemID *int64         `json:"work_item_id,omitempty"`
	Kind       string         `json:"kind"` // "git" | "local_fs" | "s3" | "http" | "webdav" | "attachment" | ...
	URI        string         `json:"uri"`  // local path, remote URL, or storage URI
	Config     map[string]any `json:"config,omitempty"`
	Label      string         `json:"label,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

// ---------------------------------------------------------------------------
// Action Resources — per-action input/output resource declarations
// ---------------------------------------------------------------------------

// ActionResource binds an Action to a ResourceBinding, declaring what file/object
// the action should fetch (input) or deposit (output) during execution.
//
// Example flow:
//
//	Action "write-article"  → output: binding=shared-drive, path="articles/draft.md"
//	Action "make-script"    → input:  binding=shared-drive, path="articles/draft.md"
//	                        → output: binding=shared-drive, path="scripts/video-script.md"
//	Action "edit-video"     → input:  binding=shared-drive, path="scripts/video-script.md"
//	                        → output: binding=s3-media,     path="videos/final.mp4"
type ActionResource struct {
	ID                int64                   `json:"id"`
	ActionID          int64                   `json:"action_id"`
	ResourceBindingID int64                   `json:"resource_binding_id"`
	Direction         ActionResourceDirection `json:"direction"` // "input" | "output"
	Path              string                  `json:"path"`      // relative path within the binding's URI
	MediaType         string                  `json:"media_type,omitempty"`
	Description       string                  `json:"description,omitempty"`
	Required          bool                    `json:"required"` // if true, missing input causes action failure
	Metadata          map[string]any          `json:"metadata,omitempty"`
	CreatedAt         time.Time               `json:"created_at"`
}

// ---------------------------------------------------------------------------
// Store interfaces
// ---------------------------------------------------------------------------

// ResourceBindingStore persists legacy ResourceBinding records.
type ResourceBindingStore interface {
	CreateResourceBinding(ctx context.Context, rb *ResourceBinding) (int64, error)
	GetResourceBinding(ctx context.Context, id int64) (*ResourceBinding, error)
	ListResourceBindings(ctx context.Context, projectID int64) ([]*ResourceBinding, error)
	ListResourceBindingsByWorkItem(ctx context.Context, workItemID int64, kind string) ([]*ResourceBinding, error)
	UpdateResourceBinding(ctx context.Context, rb *ResourceBinding) error
	DeleteResourceBinding(ctx context.Context, id int64) error
}

// ActionResourceStore persists legacy ActionResource records.
type ActionResourceStore interface {
	CreateActionResource(ctx context.Context, ar *ActionResource) (int64, error)
	GetActionResource(ctx context.Context, id int64) (*ActionResource, error)
	ListActionResources(ctx context.Context, actionID int64) ([]*ActionResource, error)
	ListActionResourcesByDirection(ctx context.Context, actionID int64, direction ActionResourceDirection) ([]*ActionResource, error)
	DeleteActionResource(ctx context.Context, id int64) error
}

// ResourceProvider is the legacy binding-based provider contract.
type ResourceProvider interface {
	// Kind returns the resource kind this provider handles.
	Kind() string

	// Fetch downloads/copies a resource to a local path. Returns the local path.
	Fetch(ctx context.Context, binding *ResourceBinding, path string, destDir string) (localPath string, err error)

	// Deposit uploads/copies a local file to the resource location.
	Deposit(ctx context.Context, binding *ResourceBinding, path string, localPath string) error
}

// NewAttachmentBinding creates a ResourceBinding that represents a work-item file attachment.
func NewAttachmentBinding(workItemID int64, fileName, filePath, mimeType string, size int64) *ResourceBinding {
	return &ResourceBinding{
		WorkItemID: &workItemID,
		Kind:       ResourceKindAttachment,
		URI:        filePath,
		Label:      fileName,
		Config: map[string]any{
			"mime_type": mimeType,
			"size":      size,
		},
	}
}

// AttachmentFileName returns the display name for an attachment binding.
func (rb *ResourceBinding) AttachmentFileName() string { return rb.Label }

// AttachmentFilePath returns the on-disk path for an attachment binding.
func (rb *ResourceBinding) AttachmentFilePath() string { return rb.URI }

// AttachmentMimeType extracts the MIME type from an attachment binding's Config.
func (rb *ResourceBinding) AttachmentMimeType() string {
	if rb.Config == nil {
		return ""
	}
	s, _ := rb.Config["mime_type"].(string)
	return s
}

// AttachmentSize extracts the file size from an attachment binding's Config.
func (rb *ResourceBinding) AttachmentSize() int64 {
	if rb.Config == nil {
		return 0
	}
	switch v := rb.Config["size"].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	}
	return 0
}
