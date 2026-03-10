package core

import "time"

// ProjectKind categorizes the type of project for workspace strategy selection.
type ProjectKind string

const (
	ProjectDev     ProjectKind = "dev"
	ProjectGeneral ProjectKind = "general"
)

// Project is a pure organizational container for grouping Flows.
// It does NOT store repo/workspace info — use ResourceBinding for that.
type Project struct {
	ID          int64             `json:"id"`
	Name        string            `json:"name"`
	Kind        ProjectKind       `json:"kind"`
	Description string            `json:"description,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}
