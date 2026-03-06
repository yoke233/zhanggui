package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yoke233/ai-workflow/internal/core"
)

type ProjectDashboardInput struct {
	ProjectID   string `json:"project_id,omitempty" jsonschema:"Project ID"`
	ProjectName string `json:"project_name,omitempty" jsonschema:"Project name (alternative to project_id)"`
}

type ProjectDashboardResult struct {
	Project      core.Project `json:"project"`
	Stats        ProjectStats `json:"stats"`
	ActiveIssues []core.Issue `json:"active_issues"`
	RecentRuns   []core.Run   `json:"recent_runs"`
	NeedsAction  []ActionItem `json:"needs_action"`
}

type ActionItem struct {
	Type    string `json:"type"`    // review_needed / run_failed / action_required
	IssueID string `json:"issue_id"`
	RunID   string `json:"run_id,omitempty"`
	Summary string `json:"summary"`
}

func registerDashboardTools(server *mcp.Server, store core.Store) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "project_dashboard",
		Description: "Get a comprehensive project overview in a single call: project info, stats, active issues, recent runs, and items needing action",
	}, projectDashboardHandler(store))
}

func projectDashboardHandler(store core.Store) func(context.Context, *mcp.CallToolRequest, ProjectDashboardInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in ProjectDashboardInput) (*mcp.CallToolResult, any, error) {
		pid, err := resolveProjectID(store, in.ProjectID, in.ProjectName)
		if err != nil {
			return errorResult(err.Error())
		}

		project, err := store.GetProject(pid)
		if err != nil {
			return nil, nil, fmt.Errorf("get project: %w", err)
		}
		if project == nil {
			return errorResult("project not found")
		}

		stats, err := computeProjectStats(store, pid)
		if err != nil {
			return nil, nil, fmt.Errorf("compute stats: %w", err)
		}

		activeIssues, _, err := store.ListIssues(pid, core.IssueFilter{State: "open", Limit: 10})
		if err != nil {
			return nil, nil, fmt.Errorf("list active issues: %w", err)
		}
		if activeIssues == nil {
			activeIssues = []core.Issue{}
		}

		recentRuns, err := store.ListRuns(pid, core.RunFilter{Limit: 5})
		if err != nil {
			return nil, nil, fmt.Errorf("list recent runs: %w", err)
		}
		if recentRuns == nil {
			recentRuns = []core.Run{}
		}

		needsAction := buildActionItems(activeIssues, recentRuns)

		return jsonResult(ProjectDashboardResult{
			Project:      *project,
			Stats:        stats,
			ActiveIssues: activeIssues,
			RecentRuns:   recentRuns,
			NeedsAction:  needsAction,
		})
	}
}

func computeProjectStats(store core.Store, projectID string) (ProjectStats, error) {
	issues, totalIssues, err := store.ListIssues(projectID, core.IssueFilter{})
	if err != nil {
		return ProjectStats{}, fmt.Errorf("list issues: %w", err)
	}
	openCount := 0
	for _, iss := range issues {
		if iss.State == core.IssueStateOpen {
			openCount++
		}
	}

	runs, err := store.ListRuns(projectID, core.RunFilter{})
	if err != nil {
		return ProjectStats{}, fmt.Errorf("list runs: %w", err)
	}
	completedCount := 0
	successCount := 0
	for _, r := range runs {
		if r.Status == core.StatusCompleted {
			completedCount++
			if r.Conclusion == core.ConclusionSuccess {
				successCount++
			}
		}
	}

	var successRate float64
	if completedCount > 0 {
		successRate = float64(successCount) / float64(completedCount)
	}

	return ProjectStats{
		TotalIssues:   totalIssues,
		OpenIssues:    openCount,
		ClosedIssues:  totalIssues - openCount,
		TotalRuns:     len(runs),
		CompletedRuns: completedCount,
		SuccessRate:   successRate,
	}, nil
}

func buildActionItems(issues []core.Issue, runs []core.Run) []ActionItem {
	var items []ActionItem

	for _, iss := range issues {
		switch iss.Status {
		case core.IssueStatusReviewing:
			items = append(items, ActionItem{
				Type:    "review_needed",
				IssueID: iss.ID,
				Summary: fmt.Sprintf("Issue %q is waiting for review", iss.Title),
			})
		case core.IssueStatusFailed:
			items = append(items, ActionItem{
				Type:    "run_failed",
				IssueID: iss.ID,
				RunID:   iss.RunID,
				Summary: fmt.Sprintf("Issue %q has failed", iss.Title),
			})
		}
	}

	for _, r := range runs {
		if r.Status == core.StatusActionRequired {
			items = append(items, ActionItem{
				Type:    "action_required",
				IssueID: r.IssueID,
				RunID:   r.ID,
				Summary: fmt.Sprintf("Run %q requires human action", r.Name),
			})
		}
		if r.Status == core.StatusCompleted && r.Conclusion == core.ConclusionFailure {
			items = append(items, ActionItem{
				Type:    "run_failed",
				IssueID: r.IssueID,
				RunID:   r.ID,
				Summary: fmt.Sprintf("Run %q failed", r.Name),
			})
		}
	}

	return items
}
