package mcpserver

import (
	"context"

	"github.com/yoke233/ai-workflow/internal/core"
)

// Deps provides business-layer dependencies for MCP tools.
// Store is required; other fields are optional (write tools are skipped when nil).
type Deps struct {
	Store        core.Store
	IssueManager IssueManager
	RunExecutor  RunExecutor
}

// IssueManager defines issue lifecycle operations exposed via MCP tools.
type IssueManager interface {
	CreateIssues(ctx context.Context, input CreateIssuesInput) ([]*core.Issue, error)
	ApplyIssueAction(ctx context.Context, issueID, action, feedback string) (*core.Issue, error)
}

// CreateIssueSpec mirrors teamleader.CreateIssueSpec to avoid import cycle.
type CreateIssueSpec struct {
	Title      string             `json:"title"`
	Body       string             `json:"body"`
	Template   string             `json:"template,omitempty"`
	AutoMerge  *bool              `json:"auto_merge,omitempty"`
	Labels     []string           `json:"labels,omitempty"`
	DependsOn  []string           `json:"depends_on,omitempty"`
	Priority   int                `json:"priority,omitempty"`
	FailPolicy core.FailurePolicy `json:"fail_policy,omitempty"`
	ParentID   string             `json:"parent_id,omitempty"`
}

// CreateIssuesInput mirrors teamleader.CreateIssuesInput.
type CreateIssuesInput struct {
	ProjectID string
	SessionID string
	Issues    []CreateIssueSpec
}

// RunExecutor defines run action operations exposed via MCP tools.
type RunExecutor interface {
	ApplyAction(ctx context.Context, action core.RunAction) error
}
