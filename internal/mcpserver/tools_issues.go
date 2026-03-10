package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yoke233/ai-workflow/internal/core"
)

func registerIssueTools(server *mcp.Server, mgr IssueManager, store core.Store) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_issue",
		Description: "Create a new issue. ProjectID is optional — assign later via update_issue. Starts in draft status.",
	}, createIssueHandler(mgr, store))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_issue",
		Description: "Update an existing issue (title, body, labels, project, priority, etc). Only allowed when issue is in draft or reviewing status.",
	}, updateIssueHandler(mgr, store))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "apply_issue_action",
		Description: "Apply an action to an issue: approve (start execution), reject (send back to draft), or abandon (cancel)",
	}, applyIssueActionHandler(mgr))

	if store != nil {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "add_issue_attachment",
			Description: "Add a requirement file or content attachment to an issue. Only allowed when issue is in draft or reviewing status.",
		}, addIssueAttachmentHandler(store))
	}
}

// --- create_issue ---

type CreateIssueToolInput struct {
	ProjectID   string   `json:"project_id,omitempty" jsonschema:"Project ID (optional, can be assigned later)"`
	ProjectName string   `json:"project_name,omitempty" jsonschema:"Project name (alternative to project_id)"`
	Title       string   `json:"title" jsonschema:"Issue title (required)"`
	Body        string   `json:"body" jsonschema:"Detailed description"`
	Template    string   `json:"template,omitempty" jsonschema:"Pipeline template: standard, full, quick, hotfix (default: standard)"`
	Labels      []string `json:"labels,omitempty" jsonschema:"Tags for the issue"`
	AutoMerge   *bool    `json:"auto_merge,omitempty" jsonschema:"Auto-merge on completion (default: true)"`
	FailPolicy  string   `json:"fail_policy,omitempty" jsonschema:"Failure handling: block, skip, human (default: block)"`
	SessionID   string   `json:"session_id,omitempty" jsonschema:"Chat session ID to group issues"`
	DependsOn   []string `json:"depends_on,omitempty" jsonschema:"IDs of issues this depends on"`
	Priority    int      `json:"priority,omitempty" jsonschema:"Scheduling priority (higher = sooner)"`
}

func createIssueHandler(mgr IssueManager, store core.Store) func(context.Context, *mcp.CallToolRequest, CreateIssueToolInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in CreateIssueToolInput) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(in.Title) == "" {
			return errorResult("title is required")
		}

		projectID := strings.TrimSpace(in.ProjectID)
		if projectID == "" && strings.TrimSpace(in.ProjectName) != "" && store != nil {
			pid, err := resolveProjectID(store, "", in.ProjectName)
			if err != nil {
				return errorResult(err.Error())
			}
			projectID = pid
		}

		template := strings.TrimSpace(in.Template)
		if template == "" {
			template = "standard"
		}
		failPolicy := core.FailurePolicy(strings.TrimSpace(in.FailPolicy))
		if failPolicy == "" {
			failPolicy = core.FailBlock
		}

		issue, err := mgr.CreateIssue(ctx, CreateIssueInput{
			ProjectID:  projectID,
			SessionID:  strings.TrimSpace(in.SessionID),
			Title:      strings.TrimSpace(in.Title),
			Body:       strings.TrimSpace(in.Body),
			Template:   template,
			Labels:     in.Labels,
			AutoMerge:  in.AutoMerge,
			DependsOn:  in.DependsOn,
			FailPolicy: failPolicy,
			Priority:   in.Priority,
		})
		if err != nil {
			return errorResult(fmt.Sprintf("create issue: %v", err))
		}
		return jsonResult(issue)
	}
}

// --- update_issue ---

type UpdateIssueToolInput struct {
	IssueID     string   `json:"issue_id" jsonschema:"Issue ID (required)"`
	ProjectID   *string  `json:"project_id,omitempty" jsonschema:"Assign or change project"`
	ProjectName string   `json:"project_name,omitempty" jsonschema:"Project name (alternative to project_id)"`
	Title       string   `json:"title,omitempty" jsonschema:"New title"`
	Body        string   `json:"body,omitempty" jsonschema:"New description (replaces existing)"`
	Template    string   `json:"template,omitempty" jsonschema:"New pipeline template"`
	Labels      []string `json:"labels,omitempty" jsonschema:"New labels (replaces existing)"`
	Priority    *int     `json:"priority,omitempty" jsonschema:"New priority"`
	FailPolicy  string   `json:"fail_policy,omitempty" jsonschema:"New failure policy: block, skip, human"`
	AutoMerge   *bool    `json:"auto_merge,omitempty" jsonschema:"New auto-merge setting"`
	Reason      string   `json:"reason,omitempty" jsonschema:"Reason for the update (recorded in change history)"`
}

func updateIssueHandler(mgr IssueManager, store core.Store) func(context.Context, *mcp.CallToolRequest, UpdateIssueToolInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in UpdateIssueToolInput) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(in.IssueID) == "" {
			return errorResult("issue_id is required")
		}

		// Resolve project_name to project_id if needed.
		if in.ProjectID == nil && strings.TrimSpace(in.ProjectName) != "" && store != nil {
			pid, err := resolveProjectID(store, "", in.ProjectName)
			if err != nil {
				return errorResult(err.Error())
			}
			in.ProjectID = &pid
		}

		var fp core.FailurePolicy
		if v := strings.TrimSpace(in.FailPolicy); v != "" {
			fp = core.FailurePolicy(v)
		}

		issue, err := mgr.UpdateIssue(ctx, UpdateIssueInput{
			IssueID:    strings.TrimSpace(in.IssueID),
			ProjectID:  in.ProjectID,
			Title:      strings.TrimSpace(in.Title),
			Body:       strings.TrimSpace(in.Body),
			Template:   strings.TrimSpace(in.Template),
			Labels:     in.Labels,
			Priority:   in.Priority,
			FailPolicy: fp,
			AutoMerge:  in.AutoMerge,
			Reason:     strings.TrimSpace(in.Reason),
		})
		if err != nil {
			return errorResult(fmt.Sprintf("update issue: %v", err))
		}
		return jsonResult(issue)
	}
}

// --- apply_issue_action ---

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
			return errorResult(fmt.Sprintf("apply issue action: %v", err))
		}
		if issue == nil {
			return errorResult("issue not found after action")
		}
		return jsonResult(issue)
	}
}

// --- add_issue_attachment ---

type AddIssueAttachmentInput struct {
	IssueID   string `json:"issue_id" jsonschema:"Issue ID (required)"`
	Path      string `json:"path" jsonschema:"Attachment name or file path (required)"`
	Content   string `json:"content,omitempty" jsonschema:"Attachment content (provide content or url)"`
	URL       string `json:"url,omitempty" jsonschema:"URL to fetch content from (alternative to content)"`
	MediaType string `json:"media_type,omitempty" jsonschema:"MIME type of the attachment"`
}

var issueEditableStatuses = map[core.IssueStatus]bool{
	core.IssueStatusDraft:     true,
	core.IssueStatusReviewing: true,
}

func addIssueAttachmentHandler(store core.Store) func(context.Context, *mcp.CallToolRequest, AddIssueAttachmentInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in AddIssueAttachmentInput) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(in.IssueID) == "" {
			return errorResult("issue_id is required")
		}
		if strings.TrimSpace(in.Path) == "" {
			return errorResult("path is required")
		}

		content := strings.TrimSpace(in.Content)
		rawURL := strings.TrimSpace(in.URL)
		if content == "" && rawURL == "" {
			return errorResult("content or url is required")
		}

		issue, err := store.GetIssue(strings.TrimSpace(in.IssueID))
		if err != nil {
			return errorResult(fmt.Sprintf("get issue: %v", err))
		}
		if issue == nil {
			return errorResult("issue not found: " + in.IssueID)
		}
		if !issueEditableStatuses[issue.Status] {
			return errorResult(fmt.Sprintf("cannot add attachment: issue is in %q status (must be draft or reviewing)", issue.Status))
		}

		sourceURL := ""
		mediaType := strings.TrimSpace(in.MediaType)
		if rawURL != "" {
			fetched, fetchedMediaType, fetchErr := fetchURLContent(ctx, rawURL, 1<<20)
			if fetchErr != nil {
				return errorResult(fmt.Sprintf("fetch url: %v", fetchErr))
			}
			content = string(fetched)
			sourceURL = rawURL
			if mediaType == "" {
				mediaType = fetchedMediaType
			}
		}

		att := &core.IssueAttachment{
			IssueID:   issue.ID,
			Path:      strings.TrimSpace(in.Path),
			Content:   content,
			SourceURL: sourceURL,
			MediaType: mediaType,
		}
		if err := store.SaveIssueAttachment(att); err != nil {
			return errorResult(fmt.Sprintf("save attachment: %v", err))
		}

		attachments, err := store.GetIssueAttachments(issue.ID)
		if err != nil {
			return errorResult(fmt.Sprintf("get attachments: %v", err))
		}
		return jsonResult(map[string]any{
			"issue_id":    issue.ID,
			"attachments": attachments,
		})
	}
}
