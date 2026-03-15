package core

import (
	"context"
	"io"
	"time"
)

// Well-known resource kinds shared by ResourceSpace and legacy migration types.
const (
	ResourceKindGit        = "git"
	ResourceKindLocalFS    = "local_fs"
	ResourceKindS3         = "s3"
	ResourceKindHTTP       = "http"
	ResourceKindWebDAV     = "webdav"
	ResourceKindAttachment = "attachment"
)

// ResourceSpace is a project-scoped external path space (git repo, local dir, bucket).
type ResourceSpace struct {
	ID        int64          `json:"id"`
	ProjectID int64          `json:"project_id"`
	Kind      string         `json:"kind"`
	RootURI   string         `json:"root_uri"`
	Role      string         `json:"role"`
	Label     string         `json:"label,omitempty"`
	Config    map[string]any `json:"config,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// Resource is a concrete file/object belonging to a project and optionally a single owner scope.
type Resource struct {
	ID          int64          `json:"id"`
	ProjectID   int64          `json:"project_id"`
	WorkItemID  *int64         `json:"work_item_id,omitempty"`
	RunID       *int64         `json:"run_id,omitempty"`
	MessageID   *int64         `json:"message_id,omitempty"`
	StorageKind string         `json:"storage_kind"`
	URI         string         `json:"uri"`
	Role        string         `json:"role"`
	FileName    string         `json:"file_name"`
	MimeType    string         `json:"mime_type,omitempty"`
	SizeBytes   int64          `json:"size_bytes,omitempty"`
	Checksum    string         `json:"checksum,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

// IODirection describes whether an ActionIODecl is an input or output declaration.
type IODirection string

const (
	IOInput  IODirection = "input"
	IOOutput IODirection = "output"
)

// ActionIODecl declares action input/output expectations before execution.
type ActionIODecl struct {
	ID          int64       `json:"id"`
	ActionID    int64       `json:"action_id"`
	Direction   IODirection `json:"direction"`
	SpaceID     *int64      `json:"space_id,omitempty"`
	ResourceID  *int64      `json:"resource_id,omitempty"`
	Path        string      `json:"path"`
	MediaType   string      `json:"media_type,omitempty"`
	Description string      `json:"description,omitempty"`
	Required    bool        `json:"required"`
	CreatedAt   time.Time   `json:"created_at"`
}

// ActionResourceDirection is kept as the runtime direction enum used by resolved I/O payloads.
type ActionResourceDirection string

const (
	ResourceInput  ActionResourceDirection = "input"
	ResourceOutput ActionResourceDirection = "output"
)

// ResolvedResource is the materialized form of an action input/output declaration.
type ResolvedResource struct {
	ActionResourceID int64                   `json:"action_resource_id"`
	Direction        ActionResourceDirection `json:"direction"`
	LocalPath        string                  `json:"local_path,omitempty"`
	RemoteURI        string                  `json:"remote_uri"`
	MediaType        string                  `json:"media_type,omitempty"`
	Description      string                  `json:"description,omitempty"`
}

// ResourceSpaceStore persists external path spaces.
type ResourceSpaceStore interface {
	CreateResourceSpace(ctx context.Context, rs *ResourceSpace) (int64, error)
	GetResourceSpace(ctx context.Context, id int64) (*ResourceSpace, error)
	ListResourceSpaces(ctx context.Context, projectID int64) ([]*ResourceSpace, error)
	UpdateResourceSpace(ctx context.Context, rs *ResourceSpace) error
	DeleteResourceSpace(ctx context.Context, id int64) error
}

// ResourceStore persists concrete files/objects.
type ResourceStore interface {
	CreateResource(ctx context.Context, r *Resource) (int64, error)
	GetResource(ctx context.Context, id int64) (*Resource, error)
	ListResourcesByWorkItem(ctx context.Context, workItemID int64) ([]*Resource, error)
	ListResourcesByRun(ctx context.Context, runID int64) ([]*Resource, error)
	ListResourcesByMessage(ctx context.Context, messageID int64) ([]*Resource, error)
	DeleteResource(ctx context.Context, id int64) error
}

// ActionIODeclStore persists action I/O declarations.
type ActionIODeclStore interface {
	CreateActionIODecl(ctx context.Context, decl *ActionIODecl) (int64, error)
	GetActionIODecl(ctx context.Context, id int64) (*ActionIODecl, error)
	ListActionIODecls(ctx context.Context, actionID int64) ([]*ActionIODecl, error)
	ListActionIODeclsByDirection(ctx context.Context, actionID int64, dir IODirection) ([]*ActionIODecl, error)
	DeleteActionIODecl(ctx context.Context, id int64) error
}

// SpaceProvider fetches/deposits files against a path-addressable ResourceSpace.
type SpaceProvider interface {
	Kind() string
	Fetch(ctx context.Context, space *ResourceSpace, path string, destDir string) (localPath string, err error)
	Deposit(ctx context.Context, space *ResourceSpace, path string, localPath string) error
}

// FileStore stores concrete resource files in internal storage.
type FileStore interface {
	Save(ctx context.Context, fileName string, r io.Reader) (uri string, size int64, err error)
	Open(ctx context.Context, uri string) (io.ReadCloser, error)
	Delete(ctx context.Context, uri string) error
}
