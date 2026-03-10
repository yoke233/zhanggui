package mcpserver

import (
	"context"

	"github.com/yoke233/ai-workflow/internal/core"
)

// Deps provides business-layer dependencies for MCP tools.
// Store is required; other fields are optional (write tools are skipped when nil).
type Deps struct {
	Store        core.Store
	ContextStore core.ContextStore
	IssueManager IssueManager
	RunExecutor  RunExecutor
}

// IssueManager defines issue lifecycle operations exposed via MCP tools.
type IssueManager interface {
	CreateIssue(ctx context.Context, input CreateIssueInput) (*core.Issue, error)
	UpdateIssue(ctx context.Context, input UpdateIssueInput) (*core.Issue, error)
	ApplyIssueAction(ctx context.Context, issueID, action, feedback string) (*core.Issue, error)
}

// CreateIssueInput contains all fields needed to create an issue.
// ProjectID is optional — the issue can be assigned to a project later via UpdateIssue.
type CreateIssueInput struct {
	ProjectID  string             `json:"project_id,omitempty"`
	SessionID  string             `json:"session_id,omitempty"`
	Title      string             `json:"title"`
	Body       string             `json:"body"`
	Template   string             `json:"template,omitempty"`
	AutoMerge  *bool              `json:"auto_merge,omitempty"`
	Labels     []string           `json:"labels,omitempty"`
	DependsOn  []string           `json:"depends_on,omitempty"`
	Priority   int                `json:"priority,omitempty"`
	FailPolicy core.FailurePolicy `json:"fail_policy,omitempty"`
}

// UpdateIssueInput carries partial updates for an existing issue.
// Only non-zero fields are applied. Allowed only when issue is in draft or reviewing status.
type UpdateIssueInput struct {
	IssueID    string             `json:"issue_id"`
	ProjectID  *string            `json:"project_id,omitempty"`
	Title      string             `json:"title,omitempty"`
	Body       string             `json:"body,omitempty"`
	Template   string             `json:"template,omitempty"`
	Labels     []string           `json:"labels,omitempty"`
	Priority   *int               `json:"priority,omitempty"`
	FailPolicy core.FailurePolicy `json:"fail_policy,omitempty"`
	AutoMerge  *bool              `json:"auto_merge,omitempty"`
	Reason     string             `json:"reason,omitempty"`
}

// RunExecutor defines run action operations exposed via MCP tools.
type RunExecutor interface {
	ApplyAction(ctx context.Context, action core.RunAction) error
}
