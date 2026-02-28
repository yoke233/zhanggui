package core

import "time"

// Project represents a managed codebase that pipelines operate on.
type Project struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	RootPath    string         `json:"root_path"`
	RepoURL     string         `json:"repo_url,omitempty"`
	Config      map[string]any `json:"config,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}
