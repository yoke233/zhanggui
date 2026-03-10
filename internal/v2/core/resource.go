package core

import "time"

// ResourceBinding links a Project to an external resource (git repo, local dir, S3, etc.).
type ResourceBinding struct {
	ID        int64          `json:"id"`
	ProjectID int64          `json:"project_id"`
	Kind      string         `json:"kind"`   // "git" | "local_fs" | "s3" | ...
	URI       string         `json:"uri"`
	Config    map[string]any `json:"config,omitempty"`
	Label     string         `json:"label,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}
