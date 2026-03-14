package core

import (
	"context"
	"time"
)

// Well-known ResourceBinding kinds.
const (
	ResourceKindGit     = "git"
	ResourceKindLocalFS = "local_fs"
	ResourceKindS3      = "s3"
	ResourceKindHTTP    = "http"
	ResourceKindWebDAV  = "webdav"
)

// ResourceBinding is the unified representation of an external resource
// associated with a Project. It serves two roles:
//
//  1. Workspace source — WorkspaceProvider uses it to prepare the agent's
//     working directory (e.g. git worktree, local dir).
//  2. I/O storage — ActionResource references it for per-action file
//     fetch/deposit (e.g. shared drive, S3 bucket, CDN).
//
// The Kind field determines which provider handles it; the URI field
// can be a local path ("/home/user/repo"), a remote URL
// ("https://github.com/org/repo.git"), or a storage URI ("s3://bucket/prefix").
type ResourceBinding struct {
	ID        int64          `json:"id"`
	ProjectID int64          `json:"project_id"`
	Kind      string         `json:"kind"` // "git" | "local_fs" | "s3" | "http" | "webdav" | ...
	URI       string         `json:"uri"`  // local path, remote URL, or storage URI
	Config    map[string]any `json:"config,omitempty"`
	Label     string         `json:"label,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// ---------------------------------------------------------------------------
// Action Resources — per-action input/output resource declarations
// ---------------------------------------------------------------------------

// ActionResourceDirection indicates whether an action reads or writes a resource.
type ActionResourceDirection string

const (
	ResourceInput  ActionResourceDirection = "input"
	ResourceOutput ActionResourceDirection = "output"
)

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
	ID                int64                  `json:"id"`
	ActionID          int64                  `json:"action_id"`
	ResourceBindingID int64                  `json:"resource_binding_id"`
	Direction         ActionResourceDirection `json:"direction"` // "input" | "output"
	Path              string                 `json:"path"`       // relative path within the binding's URI
	MediaType         string                 `json:"media_type,omitempty"`
	Description       string                 `json:"description,omitempty"`
	Required          bool                   `json:"required"` // if true, missing input causes action failure
	Metadata          map[string]any         `json:"metadata,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
}

// ResolvedResource is the materialized form of an ActionResource — the actual
// local path or URL that the agent should use during execution.
type ResolvedResource struct {
	ActionResourceID int64                  `json:"action_resource_id"`
	Direction        ActionResourceDirection `json:"direction"`
	LocalPath        string                 `json:"local_path,omitempty"` // local file path (for inputs fetched to disk)
	RemoteURI        string                 `json:"remote_uri"`           // full resolved URI
	MediaType        string                 `json:"media_type,omitempty"`
	Description      string                 `json:"description,omitempty"`
}

// ---------------------------------------------------------------------------
// Store interfaces
// ---------------------------------------------------------------------------

// ResourceBindingStore persists ResourceBinding records.
type ResourceBindingStore interface {
	CreateResourceBinding(ctx context.Context, rb *ResourceBinding) (int64, error)
	GetResourceBinding(ctx context.Context, id int64) (*ResourceBinding, error)
	ListResourceBindings(ctx context.Context, projectID int64) ([]*ResourceBinding, error)
	UpdateResourceBinding(ctx context.Context, rb *ResourceBinding) error
	DeleteResourceBinding(ctx context.Context, id int64) error
}

// ActionResourceStore persists ActionResource records.
type ActionResourceStore interface {
	CreateActionResource(ctx context.Context, ar *ActionResource) (int64, error)
	GetActionResource(ctx context.Context, id int64) (*ActionResource, error)
	ListActionResources(ctx context.Context, actionID int64) ([]*ActionResource, error)
	ListActionResourcesByDirection(ctx context.Context, actionID int64, direction ActionResourceDirection) ([]*ActionResource, error)
	DeleteActionResource(ctx context.Context, id int64) error
}

// ResourceProvider is the pluggable interface for fetching and depositing resources.
// Each resource Kind has a corresponding ResourceProvider implementation.
type ResourceProvider interface {
	// Kind returns the resource kind this provider handles.
	Kind() string

	// Fetch downloads/copies a resource to a local path. Returns the local path.
	Fetch(ctx context.Context, binding *ResourceBinding, path string, destDir string) (localPath string, err error)

	// Deposit uploads/copies a local file to the resource location.
	Deposit(ctx context.Context, binding *ResourceBinding, path string, localPath string) error
}
