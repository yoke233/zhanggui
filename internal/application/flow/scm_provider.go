package flow

import (
	"context"
	"fmt"
)

// ChangeRequestProvider abstracts PR/MR operations (GitHub PR, GitLab MR, etc.).
// It must not log or persist access tokens.
type ChangeRequestProvider interface {
	Kind() string
	Detect(ctx context.Context, originURL string) (ChangeRequestRepo, bool, error)
	EnsureOpen(ctx context.Context, repo ChangeRequestRepo, input EnsureOpenInput) (ChangeRequest, bool, error)
	Merge(ctx context.Context, repo ChangeRequestRepo, number int, input MergeInput) error
	// GetState returns the current state of a change request: "open", "merged", or "closed".
	GetState(ctx context.Context, repo ChangeRequestRepo, number int) (string, error)
}

// ChangeRequestRepo identifies a repository in a provider-specific way.
type ChangeRequestRepo struct {
	Kind      string
	Host      string
	Namespace string
	Name      string
}

// EnsureOpenInput describes parameters for creating/finding an open change request.
type EnsureOpenInput struct {
	Head  string
	Base  string
	Title string
	Body  string
	Draft bool
	Extra map[string]any
}

// ChangeRequest is a provider-agnostic view of a PR/MR.
type ChangeRequest struct {
	Number   int
	URL      string
	HeadSHA  string
	Metadata map[string]any
}

// MergeInput holds merge parameters.
type MergeInput struct {
	Method        string // squash/merge/rebase (provider-dependent)
	CommitTitle   string
	CommitMessage string
	SHA           string
	Extra         map[string]any
}

// MergeError captures provider-specific merge failure details and keeps them
// provider-agnostic enough for gate feedback and retry decisions.
type MergeError struct {
	Provider       string
	Repo           ChangeRequestRepo
	Number         int
	URL            string
	Message        string
	MergeableState string
	AlreadyMerged  bool
}

func (e *MergeError) Error() string {
	if e == nil {
		return ""
	}
	msg := e.Message
	if msg == "" {
		msg = "merge failed"
	}
	if e.Number > 0 {
		return fmt.Sprintf("%s PR #%d failed: %s", e.ProviderLabel(), e.Number, msg)
	}
	return fmt.Sprintf("%s merge failed: %s", e.ProviderLabel(), msg)
}

func (e *MergeError) ProviderLabel() string {
	if e == nil || e.Provider == "" {
		return "change request"
	}
	return e.Provider
}
