package core

import "time"

// Project represents a managed codebase that Runs operate on.
type Project struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	RepoPath    string    `json:"repo_path"`
	GitHubOwner string    `json:"github_owner,omitempty"`
	GitHubRepo  string    `json:"github_repo,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
