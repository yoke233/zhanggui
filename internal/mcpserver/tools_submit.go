package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yoke233/ai-workflow/internal/core"
)

type SubmitTaskInput struct {
	ProjectID   string           `json:"project_id,omitempty" jsonschema:"Project ID"`
	ProjectName string           `json:"project_name,omitempty" jsonschema:"Project name (alternative to project_id)"`
	Description string           `json:"description" jsonschema:"Task description (required). First line becomes the title."`
	Files       []FileAttachment `json:"files,omitempty" jsonschema:"Attachments: provide content or url for each"`
	Template    string           `json:"template,omitempty" jsonschema:"Pipeline template (default: standard)"`
	AutoApprove *bool            `json:"auto_approve,omitempty" jsonschema:"Auto-approve the issue (default: true)"`
}

type FileAttachment struct {
	Path      string `json:"path" jsonschema:"File name or path (required)"`
	Content   string `json:"content,omitempty" jsonschema:"File content"`
	URL       string `json:"url,omitempty" jsonschema:"URL to fetch content from"`
	MediaType string `json:"media_type,omitempty" jsonschema:"MIME type"`
}

type SubmitTaskResult struct {
	Issue       *core.Issue            `json:"issue"`
	Attachments []core.IssueAttachment `json:"attachments"`
	Status      string                 `json:"status"`
}

func registerSubmitTaskTool(server *mcp.Server, mgr IssueManager, store core.Store) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "submit_task",
		Description: "Create and optionally auto-approve a task in one call. Combines create_issue + add_attachments + approve into a single operation.",
	}, submitTaskHandler(mgr, store))
}

func submitTaskHandler(mgr IssueManager, store core.Store) func(context.Context, *mcp.CallToolRequest, SubmitTaskInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in SubmitTaskInput) (*mcp.CallToolResult, any, error) {
		description := strings.TrimSpace(in.Description)
		if description == "" {
			return errorResult("description is required")
		}

		// Resolve project.
		projectID := ""
		if in.ProjectID != "" || in.ProjectName != "" {
			pid, err := resolveProjectID(store, in.ProjectID, in.ProjectName)
			if err != nil {
				return errorResult(err.Error())
			}
			projectID = pid
		}

		// Extract title from first line, capped at 80 chars.
		title, body := extractTitleBody(description)

		template := strings.TrimSpace(in.Template)
		if template == "" {
			template = "standard"
		}

		issue, err := mgr.CreateIssue(ctx, CreateIssueInput{
			ProjectID:  projectID,
			Title:      title,
			Body:       body,
			Template:   template,
			FailPolicy: core.FailBlock,
		})
		if err != nil {
			return errorResult(fmt.Sprintf("create issue: %v", err))
		}

		// Attach files. On failure, abandon the issue to avoid orphans.
		if attachErr := attachFiles(ctx, store, issue.ID, in.Files); attachErr != nil {
			_, _ = mgr.ApplyIssueAction(ctx, issue.ID, "abandon", "attachment failed: "+attachErr.Error())
			return errorResult(fmt.Sprintf("attach files (issue abandoned): %v", attachErr))
		}

		// Auto-approve (default true).
		autoApprove := true
		if in.AutoApprove != nil {
			autoApprove = *in.AutoApprove
		}

		status := string(issue.Status)
		if autoApprove {
			approved, approveErr := mgr.ApplyIssueAction(ctx, issue.ID, "approve", "")
			if approveErr != nil {
				status = fmt.Sprintf("created (approve failed: %v)", approveErr)
			} else if approved != nil {
				issue = approved
				status = string(approved.Status)
			}
		}

		attachments, _ := store.GetIssueAttachments(issue.ID)
		if attachments == nil {
			attachments = []core.IssueAttachment{}
		}

		return jsonResult(SubmitTaskResult{
			Issue:       issue,
			Attachments: attachments,
			Status:      status,
		})
	}
}

func attachFiles(ctx context.Context, store core.Store, issueID string, files []FileAttachment) error {
	for _, f := range files {
		path := strings.TrimSpace(f.Path)
		if path == "" {
			return fmt.Errorf("file path is required")
		}
		content := f.Content
		sourceURL := ""
		mediaType := strings.TrimSpace(f.MediaType)

		rawURL := strings.TrimSpace(f.URL)
		if rawURL != "" {
			fetched, fetchedMediaType, err := fetchURLContent(ctx, rawURL, 1<<20)
			if err != nil {
				return fmt.Errorf("fetch %s: %w", path, err)
			}
			content = string(fetched)
			sourceURL = rawURL
			if mediaType == "" {
				mediaType = fetchedMediaType
			}
		} else if strings.TrimSpace(content) == "" {
			return fmt.Errorf("file %q: content or url is required", path)
		}

		if err := store.SaveIssueAttachment(&core.IssueAttachment{
			IssueID:   issueID,
			Path:      path,
			Content:   content,
			SourceURL: sourceURL,
			MediaType: mediaType,
		}); err != nil {
			return fmt.Errorf("save %s: %w", path, err)
		}
	}
	return nil
}

func extractTitleBody(description string) (title, body string) {
	lines := strings.SplitN(description, "\n", 2)
	title = strings.TrimSpace(lines[0])
	runes := []rune(title)
	if len(runes) > 80 {
		title = string(runes[:80])
	}
	body = description
	return title, body
}
