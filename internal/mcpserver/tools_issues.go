package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yoke233/ai-workflow/internal/core"
)

func registerIssueTools(server *mcp.Server, mgr IssueManager) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_issue",
		Description: "Create a new issue for a project. The issue starts in draft status and must be explicitly approved to begin execution.",
	}, createIssueHandler(mgr))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "apply_issue_action",
		Description: "Apply an action to an issue: approve (start execution), reject (send back to draft), or abandon (cancel)",
	}, applyIssueActionHandler(mgr))
}

type CreateIssueInput struct {
	ProjectID  string   `json:"project_id" jsonschema:"Project ID (required)"`
	Title      string   `json:"title" jsonschema:"Issue title (required)"`
	Body       string   `json:"body" jsonschema:"Detailed description"`
	Template   string   `json:"template,omitempty" jsonschema:"Pipeline template: standard, full, quick, hotfix (default: standard)"`
	Labels     []string `json:"labels,omitempty" jsonschema:"Tags for the issue"`
	AutoMerge  *bool    `json:"auto_merge,omitempty" jsonschema:"Auto-merge on completion (default: true)"`
	FailPolicy string   `json:"fail_policy,omitempty" jsonschema:"Failure handling: block, skip, human (default: block)"`
	SessionID  string   `json:"session_id,omitempty" jsonschema:"Chat session ID to group issues"`
	ParentID   string   `json:"parent_id,omitempty" jsonschema:"Parent issue ID for decomposition"`
	Priority   int      `json:"priority,omitempty" jsonschema:"Scheduling priority (higher = sooner)"`
}

func createIssueHandler(mgr IssueManager) func(context.Context, *mcp.CallToolRequest, CreateIssueInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in CreateIssueInput) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(in.ProjectID) == "" {
			return errorResult("project_id is required")
		}
		if strings.TrimSpace(in.Title) == "" {
			return errorResult("title is required")
		}

		template := strings.TrimSpace(in.Template)
		if template == "" {
			template = "standard"
		}
		failPolicy := core.FailurePolicy(strings.TrimSpace(in.FailPolicy))
		if failPolicy == "" {
			failPolicy = core.FailBlock
		}

		spec := CreateIssueSpec{
			Title:      strings.TrimSpace(in.Title),
			Body:       strings.TrimSpace(in.Body),
			Template:   template,
			Labels:     in.Labels,
			AutoMerge:  in.AutoMerge,
			FailPolicy: failPolicy,
			ParentID:   strings.TrimSpace(in.ParentID),
			Priority:   in.Priority,
		}

		issues, err := mgr.CreateIssues(ctx, CreateIssuesInput{
			ProjectID: strings.TrimSpace(in.ProjectID),
			SessionID: strings.TrimSpace(in.SessionID),
			Issues:    []CreateIssueSpec{spec},
		})
		if err != nil {
			return nil, nil, fmt.Errorf("create issue: %w", err)
		}
		if len(issues) == 0 {
			return errorResult("no issues created")
		}
		return jsonResult(issues[0])
	}
}

type ApplyIssueActionInput struct {
	IssueID  string `json:"issue_id" jsonschema:"Issue ID (required)"`
	Action   string `json:"action" jsonschema:"Action: approve, reject, abandon (required)"`
	Feedback string `json:"feedback,omitempty" jsonschema:"Optional feedback message"`
}

func applyIssueActionHandler(mgr IssueManager) func(context.Context, *mcp.CallToolRequest, ApplyIssueActionInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in ApplyIssueActionInput) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(in.IssueID) == "" {
			return errorResult("issue_id is required")
		}
		action := strings.ToLower(strings.TrimSpace(in.Action))
		if action == "" {
			return errorResult("action is required")
		}
		switch action {
		case "approve", "reject", "abandon":
		default:
			return errorResult("invalid action: " + action + " (must be approve, reject, or abandon)")
		}

		issue, err := mgr.ApplyIssueAction(ctx, strings.TrimSpace(in.IssueID), action, strings.TrimSpace(in.Feedback))
		if err != nil {
			return nil, nil, fmt.Errorf("apply issue action: %w", err)
		}
		if issue == nil {
			return errorResult("issue not found after action")
		}
		return jsonResult(issue)
	}
}
