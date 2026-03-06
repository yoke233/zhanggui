package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yoke233/ai-workflow/internal/core"
)

func registerProjectTools(server *mcp.Server, store core.Store) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_project",
		Description: "Create a new project",
	}, createProjectHandler(store))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_project",
		Description: "Update an existing project",
	}, updateProjectHandler(store))

	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_project",
		Description: "Delete a project by ID",
	}, deleteProjectHandler(store))
}

type CreateProjectInput struct {
	Name          string `json:"name" jsonschema:"Project name (required)"`
	RepoPath      string `json:"repo_path,omitempty" jsonschema:"Local repository path"`
	DefaultBranch string `json:"default_branch,omitempty" jsonschema:"Default git branch (default: main)"`
	GitHubOwner   string `json:"github_owner,omitempty" jsonschema:"GitHub owner"`
	GitHubRepo    string `json:"github_repo,omitempty" jsonschema:"GitHub repository name"`
}

func createProjectHandler(store core.Store) func(context.Context, *mcp.CallToolRequest, CreateProjectInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in CreateProjectInput) (*mcp.CallToolResult, any, error) {
		name := strings.TrimSpace(in.Name)
		if name == "" {
			return errorResult("name is required")
		}
		branch := strings.TrimSpace(in.DefaultBranch)
		if branch == "" {
			branch = "main"
		}
		now := time.Now().UTC()
		p := &core.Project{
			ID:            uuid.NewString(),
			Name:          name,
			RepoPath:      strings.TrimSpace(in.RepoPath),
			DefaultBranch: branch,
			GitHubOwner:   strings.TrimSpace(in.GitHubOwner),
			GitHubRepo:    strings.TrimSpace(in.GitHubRepo),
			CreatedAt:     now,
			UpdatedAt:     now,
		}
		if err := store.CreateProject(p); err != nil {
			return errorResult(fmt.Sprintf("create project: %v", err))
		}
		return jsonResult(p)
	}
}

type UpdateProjectInput struct {
	ProjectID     string `json:"project_id,omitempty" jsonschema:"Project ID"`
	ProjectName   string `json:"project_name,omitempty" jsonschema:"Project name (alternative to project_id)"`
	Name          string `json:"name,omitempty" jsonschema:"New project name"`
	RepoPath      string `json:"repo_path,omitempty" jsonschema:"New repository path"`
	DefaultBranch string `json:"default_branch,omitempty" jsonschema:"New default branch"`
	GitHubOwner   string `json:"github_owner,omitempty" jsonschema:"New GitHub owner"`
	GitHubRepo    string `json:"github_repo,omitempty" jsonschema:"New GitHub repository name"`
}

func updateProjectHandler(store core.Store) func(context.Context, *mcp.CallToolRequest, UpdateProjectInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in UpdateProjectInput) (*mcp.CallToolResult, any, error) {
		pid, err := resolveProjectID(store, in.ProjectID, in.ProjectName)
		if err != nil {
			return errorResult(err.Error())
		}
		p, err := store.GetProject(pid)
		if err != nil {
			return errorResult(fmt.Sprintf("get project: %v", err))
		}
		if p == nil {
			return errorResult("project not found: " + in.ProjectID)
		}
		if v := strings.TrimSpace(in.Name); v != "" {
			p.Name = v
		}
		if v := strings.TrimSpace(in.RepoPath); v != "" {
			p.RepoPath = v
		}
		if v := strings.TrimSpace(in.DefaultBranch); v != "" {
			p.DefaultBranch = v
		}
		if v := strings.TrimSpace(in.GitHubOwner); v != "" {
			p.GitHubOwner = v
		}
		if v := strings.TrimSpace(in.GitHubRepo); v != "" {
			p.GitHubRepo = v
		}
		p.UpdatedAt = time.Now().UTC()
		if err := store.UpdateProject(p); err != nil {
			return errorResult(fmt.Sprintf("update project: %v", err))
		}
		return jsonResult(p)
	}
}

type DeleteProjectInput struct {
	ProjectID   string `json:"project_id,omitempty" jsonschema:"Project ID"`
	ProjectName string `json:"project_name,omitempty" jsonschema:"Project name (alternative to project_id)"`
}

func deleteProjectHandler(store core.Store) func(context.Context, *mcp.CallToolRequest, DeleteProjectInput) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in DeleteProjectInput) (*mcp.CallToolResult, any, error) {
		pid, err := resolveProjectID(store, in.ProjectID, in.ProjectName)
		if err != nil {
			return errorResult(err.Error())
		}
		if err := store.DeleteProject(pid); err != nil {
			return errorResult(fmt.Sprintf("delete project: %v", err))
		}
		return jsonResult(map[string]string{"deleted": pid})
	}
}
