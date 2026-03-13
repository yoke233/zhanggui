package core

import (
	"context"
	"time"
)

// ResourceLocatorKind classifies the storage backend for a resource locator.
type ResourceLocatorKind string

const (
	LocatorLocalFS ResourceLocatorKind = "local_fs"
	LocatorS3      ResourceLocatorKind = "s3"
	LocatorHTTP    ResourceLocatorKind = "http"
	LocatorGit     ResourceLocatorKind = "git"
	LocatorWebDAV  ResourceLocatorKind = "webdav"
)

// ResourceLocator describes a storage location that can be used to fetch or deposit resources.
// It is a project-level registry entry — actions reference locators by ID.
type ResourceLocator struct {
	ID        int64               `json:"id"`
	ProjectID int64               `json:"project_id"`
	Kind      ResourceLocatorKind `json:"kind"`
	Label     string              `json:"label"`
	BaseURI   string              `json:"base_uri"`             // e.g. "s3://bucket/prefix", "/mnt/shared", "https://cdn.example.com"
	Config    map[string]any      `json:"config,omitempty"`     // auth, region, headers, etc.
	CreatedAt time.Time           `json:"created_at"`
	UpdatedAt time.Time           `json:"updated_at"`
}

// ActionResourceDirection indicates whether an action reads or writes a resource.
type ActionResourceDirection string

const (
	ResourceInput  ActionResourceDirection = "input"
	ResourceOutput ActionResourceDirection = "output"
)

// ActionResource binds an Action to a ResourceLocator, declaring what file/object
// the action should fetch (input) or deposit (output) during execution.
//
// Example flow:
//
//	Action "write-article"  → output: locator=shared-drive, path="articles/draft.md"
//	Action "make-script"    → input:  locator=shared-drive, path="articles/draft.md"
//	                        → output: locator=shared-drive, path="scripts/video-script.md"
//	Action "edit-video"     → input:  locator=shared-drive, path="scripts/video-script.md"
//	                        → output: locator=s3-media,     path="videos/final.mp4"
type ActionResource struct {
	ID          int64                  `json:"id"`
	ActionID    int64                  `json:"action_id"`
	LocatorID   int64                  `json:"locator_id"`
	Direction   ActionResourceDirection `json:"direction"` // "input" | "output"
	Path        string                 `json:"path"`       // relative path within the locator's BaseURI
	MediaType   string                 `json:"media_type,omitempty"`
	Description string                 `json:"description,omitempty"`
	Required    bool                   `json:"required"` // if true, missing input causes action failure
	Metadata    map[string]any         `json:"metadata,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
}

// ResolvedResource is the materialized form of an ActionResource — the actual
// local path or URL that the agent should use during execution.
type ResolvedResource struct {
	ActionResourceID int64  `json:"action_resource_id"`
	Direction        ActionResourceDirection `json:"direction"`
	LocalPath        string `json:"local_path,omitempty"` // local file path (for inputs fetched to disk)
	RemoteURI        string `json:"remote_uri"`           // full resolved URI
	MediaType        string `json:"media_type,omitempty"`
	Description      string `json:"description,omitempty"`
}

// ResourceLocatorStore persists ResourceLocator records.
type ResourceLocatorStore interface {
	CreateResourceLocator(ctx context.Context, loc *ResourceLocator) (int64, error)
	GetResourceLocator(ctx context.Context, id int64) (*ResourceLocator, error)
	ListResourceLocators(ctx context.Context, projectID int64) ([]*ResourceLocator, error)
	UpdateResourceLocator(ctx context.Context, loc *ResourceLocator) error
	DeleteResourceLocator(ctx context.Context, id int64) error
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
// Each ResourceLocatorKind has a corresponding ResourceProvider implementation.
type ResourceProvider interface {
	// Kind returns the locator kind this provider handles.
	Kind() ResourceLocatorKind

	// Fetch downloads/copies a resource to a local path. Returns the local path.
	Fetch(ctx context.Context, locator *ResourceLocator, path string, destDir string) (localPath string, err error)

	// Deposit uploads/copies a local file to the resource location.
	Deposit(ctx context.Context, locator *ResourceLocator, path string, localPath string) error
}
